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

// fakeRepo answers from saved. err fails every operation; insertErr fails InsertBooking only,
// which is how a uniqueness rejection followed by a classifying read is faked. inserts counts the
// InsertBooking calls.
type fakeRepo struct {
	err       error
	insertErr error
	saved     []Booking
	inserts   int
}

func (r *fakeRepo) InsertBooking(_ context.Context, b Booking) error {
	r.inserts++
	if r.err != nil {
		return r.err
	}
	if r.insertErr != nil {
		return r.insertErr
	}
	r.saved = append(r.saved, b)
	return nil
}

func (r *fakeRepo) GetBookingByIdempotencyKey(_ context.Context, key string) (Booking, error) {
	if r.err != nil {
		return Booking{}, r.err
	}
	for _, b := range r.saved {
		if b.IdempotencyKey == key {
			return b, nil
		}
	}
	return Booking{}, ErrNotFound
}

// UpdateBookingStatus and GetBooking answer from the saved bookings; the schema's guard is not
// faked, the store's tests and the property tests prove it against SQLite.
func (r *fakeRepo) UpdateBookingStatus(_ context.Context, id string, to Status) (Booking, error) {
	if r.err != nil {
		return Booking{}, r.err
	}
	for i := range r.saved {
		if r.saved[i].ID == id {
			r.saved[i].Status = to
			return r.saved[i], nil
		}
	}
	return Booking{}, ErrNotFound
}

func (r *fakeRepo) GetBooking(_ context.Context, id string) (Booking, error) {
	if r.err != nil {
		return Booking{}, r.err
	}
	for _, b := range r.saved {
		if b.ID == id {
			return b, nil
		}
	}
	return Booking{}, ErrNotFound
}

var clock = func() time.Time { return time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC) }

func TestService_BookStoresAnActiveBookingAtTheCanonicalStart(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, clock)
	req := Request{ResourceID: "r1", SlotStart: time.Date(2026, 11, 2, 9, 10, 0, 0, time.UTC), CustomerName: " Ann ", IdempotencyKey: "K-1 "}

	b, replayed, err := s.Book(context.Background(), req)
	if err != nil || replayed {
		t.Fatalf("Book: replayed %v, err %v", replayed, err)
	}
	want := Booking{ID: b.ID, ResourceID: "r1", SlotStart: time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC),
		CustomerName: " Ann ", IdempotencyKey: "K-1 ", Status: StatusActive}
	if b.ID == "" || b != want || len(repo.saved) != 1 || repo.saved[0] != want {
		t.Fatalf("booked %+v, saved %+v; want %+v", b, repo.saved, want)
	}
	if !b.SlotEnd().Equal(want.SlotStart.Add(30 * time.Minute)) {
		t.Errorf("SlotEnd = %v", b.SlotEnd())
	}
	req.IdempotencyKey = "K-2"
	b2, _, _ := s.Book(context.Background(), req)
	if b2.ID == b.ID {
		t.Error("two bookings got the same id")
	}
}

// A uniqueness rejection with nothing stored under the request's key is a taken slot.
func TestService_BookMapsAUniquenessRejectionToSlotTaken(t *testing.T) {
	s := NewService(&fakeRepo{insertErr: fmt.Errorf("store: %w", ErrNotUnique)}, clock)
	_, _, err := s.Book(context.Background(), Request{ResourceID: "r1", SlotStart: clock().Add(time.Hour), IdempotencyKey: "k"})
	if !errors.Is(err, ErrSlotTaken) {
		t.Fatalf("err = %v, want ErrSlotTaken", err)
	}
}

func TestService_BookPassesOtherStoreErrorsThrough(t *testing.T) {
	boom := errors.New("disk I/O error")
	s := NewService(&fakeRepo{err: boom}, clock)
	_, _, err := s.Book(context.Background(), Request{ResourceID: "r1", SlotStart: clock().Add(time.Hour), IdempotencyKey: "k"})
	if !errors.Is(err, boom) || errors.Is(err, ErrSlotTaken) {
		t.Fatalf("err = %v, want the store error and not ErrSlotTaken", err)
	}
}

// The classifier (ADR-0011): after a uniqueness rejection the booking stored under the key decides
// the outcome, whatever index fired.
func TestService_BookClassifiesARejectedInsertFromTheStoredKey(t *testing.T) {
	start := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	stored := Booking{ID: "b1", ResourceID: "r1", SlotStart: start, CustomerName: "Ann", IdempotencyKey: "K1", Status: StatusCancelled}
	notUnique := fmt.Errorf("store: %w", ErrNotUnique)
	for _, c := range []struct {
		name         string
		req          Request
		wantReplayed bool
		wantErr      error
	}{
		{"same payload, non-canonical spelling of the slot", Request{"r1", start.Add(17 * time.Minute).In(time.FixedZone("+02", 7200)), "Ann", "K1"}, true, nil},
		{"other name", Request{"r1", start, "Bob", "K1"}, false, ErrKeyConflict},
		{"name differs by case", Request{"r1", start, "ann", "K1"}, false, ErrKeyConflict},
		{"name differs by a trailing space", Request{"r1", start, "Ann ", "K1"}, false, ErrKeyConflict},
		{"other slot", Request{"r1", start.Add(SlotLength), "Ann", "K1"}, false, ErrKeyConflict},
		{"other resource", Request{"r2", start, "Ann", "K1"}, false, ErrKeyConflict},
		{"no booking under the key", Request{"r1", start, "Ann", "K2"}, false, ErrSlotTaken},
	} {
		t.Run(c.name, func(t *testing.T) {
			repo := &fakeRepo{insertErr: notUnique, saved: []Booking{stored}}
			b, replayed, err := NewService(repo, clock).Book(context.Background(), c.req)
			if !errors.Is(err, c.wantErr) || replayed != c.wantReplayed {
				t.Fatalf("Book = %+v, replayed %v, err %v; want replayed %v, err %v", b, replayed, err, c.wantReplayed, c.wantErr)
			}
			if c.wantReplayed && b != stored {
				t.Fatalf("replay returned %+v, want the stored booking %+v in its current state", b, stored)
			}
			if !c.wantReplayed && b != (Booking{}) {
				t.Fatalf("rejected request returned %+v", b)
			}
		})
	}
}

// A failure of the classifying read is passed through and is not mistaken for any outcome.
func TestService_BookPassesAClassifyingReadErrorThrough(t *testing.T) {
	boom := errors.New("disk I/O error")
	repo := &failingReadRepo{fakeRepo: fakeRepo{insertErr: fmt.Errorf("store: %w", ErrNotUnique)}, readErr: boom}
	_, replayed, err := NewService(repo, clock).Book(context.Background(), Request{ResourceID: "r1", SlotStart: clock(), CustomerName: "Ann", IdempotencyKey: "K1"})
	if !errors.Is(err, boom) || errors.Is(err, ErrSlotTaken) || errors.Is(err, ErrKeyConflict) || replayed {
		t.Fatalf("replayed %v, err %v; want the read error only", replayed, err)
	}
}

type failingReadRepo struct {
	fakeRepo
	readErr error
}

func (r *failingReadRepo) GetBookingByIdempotencyKey(context.Context, string) (Booking, error) {
	return Booking{}, r.readErr
}

// ADR-0018, AC-053: an empty key is rejected before any statement, so it never reaches the
// classifier.
func TestService_BookRejectsAnEmptyKeyBeforeInserting(t *testing.T) {
	repo := &fakeRepo{}
	_, _, err := NewService(repo, clock).Book(context.Background(), Request{ResourceID: "r1", SlotStart: clock(), CustomerName: "Ann"})
	if !errors.Is(err, ErrInvalidRequest) || repo.inserts != 0 || len(repo.saved) != 0 {
		t.Fatalf("err %v, inserts %d; want ErrInvalidRequest and no insert", err, repo.inserts)
	}
}

// The aggregate's transition rule (INV-004): every status may become cancelled, an active booking
// may stay active, and a cancelled booking may become nothing but cancelled.
func TestBooking_WithStatusKeepsCancelledFinal(t *testing.T) {
	active := Booking{ID: "b1", Status: StatusActive}
	cancelled := Booking{ID: "b1", Status: StatusCancelled}
	for _, c := range []struct {
		from    Booking
		to      Status
		want    Status
		wantErr error
	}{
		{active, StatusCancelled, StatusCancelled, nil},
		{active, StatusActive, StatusActive, nil},
		{cancelled, StatusCancelled, StatusCancelled, nil},
		{cancelled, StatusActive, StatusCancelled, ErrBookingCancelled},
		{cancelled, Status("pending"), StatusCancelled, ErrBookingCancelled},
	} {
		got, err := c.from.WithStatus(c.to)
		if !errors.Is(err, c.wantErr) || got.Status != c.want || got.ID != "b1" {
			t.Errorf("%s.WithStatus(%s) = %+v, %v; want status %s, err %v", c.from.Status, c.to, got, err, c.want, c.wantErr)
		}
	}
}

func TestService_CancelSetsCancelledThroughTheRepository(t *testing.T) {
	repo := &fakeRepo{saved: []Booking{{ID: "b1", Status: StatusActive}}}
	b, err := NewService(repo, clock).Cancel(context.Background(), "b1")
	if err != nil || b.ID != "b1" || b.Status != StatusCancelled || repo.saved[0].Status != StatusCancelled {
		t.Fatalf("Cancel = %+v, %v; stored %+v", b, err, repo.saved)
	}
}

func TestService_CancelAndGetReportUnknownIdAsNotFound(t *testing.T) {
	s := NewService(&fakeRepo{}, clock)
	if _, err := s.Cancel(context.Background(), "missing-id"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Cancel: err = %v, want ErrNotFound", err)
	}
	if _, err := s.Get(context.Background(), "missing-id"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get: err = %v, want ErrNotFound", err)
	}
}

func TestService_CancelPassesTheSchemaGuardThrough(t *testing.T) {
	s := NewService(&fakeRepo{err: fmt.Errorf("store: %w", ErrBookingCancelled)}, clock)
	if _, err := s.Cancel(context.Background(), "b1"); !errors.Is(err, ErrBookingCancelled) {
		t.Fatalf("err = %v, want ErrBookingCancelled", err)
	}
}
