package booking

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestCanonicalSlotStart(t *testing.T) {
	plus2 := time.FixedZone("+02:00", 2*3600)
	minus330 := time.FixedZone("-03:30", -(3*3600 + 30*60))
	cases := []struct {
		in, want time.Time
	}{
		{time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC), time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)},
		{time.Date(2026, 11, 2, 9, 29, 59, 999_999_999, time.UTC), time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)},
		{time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC), time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC)},
		{time.Date(2026, 11, 2, 11, 15, 0, 0, plus2), time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)},
		// A -03:30 offset is a half hour off the UTC grid in local terms; the slot is UTC's.
		{time.Date(2026, 11, 2, 5, 45, 0, 0, minus330), time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)},
		{time.Date(1969, 12, 31, 23, 59, 59, 0, time.UTC), time.Date(1969, 12, 31, 23, 30, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		got := CanonicalSlotStart(c.in)
		if !got.Equal(c.want) || got.Location() != time.UTC {
			t.Errorf("CanonicalSlotStart(%v) = %v, want %v in UTC", c.in, got, c.want)
		}
	}
}

type fakeRepo struct {
	err   error
	saved []Booking
}

func (r *fakeRepo) InsertBooking(_ context.Context, b Booking) error {
	if r.err != nil {
		return r.err
	}
	r.saved = append(r.saved, b)
	return nil
}

var clock = func() time.Time { return time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC) }

func TestService_BookStoresAnActiveBookingAtTheCanonicalStart(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, clock)
	req := Request{ResourceID: "r1", SlotStart: time.Date(2026, 11, 2, 9, 10, 0, 0, time.UTC), CustomerName: " Ann ", IdempotencyKey: "K-1 "}

	b, err := s.Book(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	want := Booking{ID: b.ID, ResourceID: "r1", SlotStart: time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC),
		CustomerName: " Ann ", IdempotencyKey: "K-1 ", Status: StatusActive}
	if b.ID == "" || b != want || len(repo.saved) != 1 || repo.saved[0] != want {
		t.Fatalf("booked %+v, saved %+v; want %+v", b, repo.saved, want)
	}
	if !b.SlotEnd().Equal(want.SlotStart.Add(30 * time.Minute)) {
		t.Errorf("SlotEnd = %v", b.SlotEnd())
	}
	b2, _ := s.Book(context.Background(), req)
	if b2.ID == b.ID {
		t.Error("two bookings got the same id")
	}
}

func TestService_BookMapsAUniquenessRejectionToSlotTaken(t *testing.T) {
	s := NewService(&fakeRepo{err: fmt.Errorf("store: %w", ErrNotUnique)}, clock)
	_, err := s.Book(context.Background(), Request{ResourceID: "r1", SlotStart: clock().Add(time.Hour)})
	if !errors.Is(err, ErrSlotTaken) {
		t.Fatalf("err = %v, want ErrSlotTaken", err)
	}
}

func TestService_BookPassesOtherStoreErrorsThrough(t *testing.T) {
	boom := errors.New("disk I/O error")
	s := NewService(&fakeRepo{err: boom}, clock)
	_, err := s.Book(context.Background(), Request{ResourceID: "r1", SlotStart: clock().Add(time.Hour)})
	if !errors.Is(err, boom) || errors.Is(err, ErrSlotTaken) {
		t.Fatalf("err = %v, want the store error and not ErrSlotTaken", err)
	}
}
