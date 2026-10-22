package booking

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/adverax/booking-demo/internal/catalog"
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
// InsertBooking calls, keyReads the GetBookingByIdempotencyKey calls.
type fakeRepo struct {
	err       error
	insertErr error
	saved     []Booking
	inserts   int
	keyReads  int
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
	r.keyReads++
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

// fakeResources is the catalog port: resources open every day 00:00-24:00 by id, unless hours
// names other hours; any other id is catalog.ErrNotFound; err fails every read.
type fakeResources struct {
	ids   []string
	hours []catalog.Interval
	err   error
}

func (f fakeResources) Get(_ context.Context, id string) (catalog.Resource, error) {
	if f.err != nil {
		return catalog.Resource{}, f.err
	}
	for _, known := range f.ids {
		if known == id {
			hours := f.hours
			if hours == nil {
				for d := time.Sunday; d <= time.Saturday; d++ {
					hours = append(hours, catalog.Interval{Weekday: d, Start: 0, End: catalog.EndOfDay})
				}
			}
			return catalog.Resource{ID: id, Name: id, WeeklyHours: hours}, nil
		}
	}
	return catalog.Resource{}, fmt.Errorf("catalog: get resource %q: %w", id, catalog.ErrNotFound)
}

// openResources knows r1 and r2, open around the clock.
var openResources = fakeResources{ids: []string{"r1", "r2"}}

var clock = func() time.Time { return time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC) }

func TestService_BookStoresAnActiveBookingAtTheCanonicalStart(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, openResources, clock)
	// 11:00 at +02:00 is the Monday 09:00 UTC slot, spelled in another zone.
	req := Request{ResourceID: "r1", SlotStart: time.Date(2026, 11, 2, 11, 0, 0, 0, time.FixedZone("+02", 7200)), CustomerName: " Ann ", IdempotencyKey: "K-1 "}

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
	s := NewService(&fakeRepo{insertErr: fmt.Errorf("store: %w", ErrNotUnique)}, openResources, clock)
	_, _, err := s.Book(context.Background(), Request{ResourceID: "r1", SlotStart: clock().Add(time.Hour), CustomerName: "Ann", IdempotencyKey: "k"})
	if !errors.Is(err, ErrSlotTaken) {
		t.Fatalf("err = %v, want ErrSlotTaken", err)
	}
}

func TestService_BookPassesOtherStoreErrorsThrough(t *testing.T) {
	boom := errors.New("disk I/O error")
	s := NewService(&fakeRepo{err: boom}, openResources, clock)
	_, _, err := s.Book(context.Background(), Request{ResourceID: "r1", SlotStart: clock().Add(time.Hour), CustomerName: "Ann", IdempotencyKey: "k"})
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
		{"same payload, another zone's spelling of the slot", Request{"r1", start.In(time.FixedZone("+02", 7200)), "Ann", "K1"}, true, nil},
		{"other name", Request{"r1", start, "Bob", "K1"}, false, ErrKeyConflict},
		{"name differs by case", Request{"r1", start, "ann", "K1"}, false, ErrKeyConflict},
		{"name differs by a trailing space", Request{"r1", start, "Ann ", "K1"}, false, ErrKeyConflict},
		{"other slot", Request{"r1", start.Add(SlotLength), "Ann", "K1"}, false, ErrKeyConflict},
		{"other resource", Request{"r2", start, "Ann", "K1"}, false, ErrKeyConflict},
		{"no booking under the key", Request{"r1", start, "Ann", "K2"}, false, ErrSlotTaken},
	} {
		t.Run(c.name, func(t *testing.T) {
			repo := &fakeRepo{insertErr: notUnique, saved: []Booking{stored}}
			b, replayed, err := NewService(repo, openResources, clock).Book(context.Background(), c.req)
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
	_, replayed, err := NewService(repo, openResources, clock).Book(context.Background(), Request{ResourceID: "r1", SlotStart: clock(), CustomerName: "Ann", IdempotencyKey: "K1"})
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
	_, _, err := NewService(repo, openResources, clock).Book(context.Background(), Request{ResourceID: "r1", SlotStart: clock(), CustomerName: "Ann"})
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
	b, err := NewService(repo, openResources, clock).Cancel(context.Background(), "b1")
	if err != nil || b.ID != "b1" || b.Status != StatusCancelled || repo.saved[0].Status != StatusCancelled {
		t.Fatalf("Cancel = %+v, %v; stored %+v", b, err, repo.saved)
	}
}

func TestService_CancelAndGetReportUnknownIdAsNotFound(t *testing.T) {
	s := NewService(&fakeRepo{}, openResources, clock)
	if _, err := s.Cancel(context.Background(), "missing-id"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Cancel: err = %v, want ErrNotFound", err)
	}
	if _, err := s.Get(context.Background(), "missing-id"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get: err = %v, want ErrNotFound", err)
	}
}

func TestService_CancelPassesTheSchemaGuardThrough(t *testing.T) {
	s := NewService(&fakeRepo{err: fmt.Errorf("store: %w", ErrBookingCancelled)}, openResources, clock)
	if _, err := s.Cancel(context.Background(), "b1"); !errors.Is(err, ErrBookingCancelled) {
		t.Fatalf("err = %v, want ErrBookingCancelled", err)
	}
}

// Validation order (ADR-0011, ADR-0020): shape, resource, working hours, past. Every rejection
// stores nothing, and none but the past slot reads the key; the first failing check names the
// error. Each case also fails the checks after its own, so a reordering shows.
func TestService_BookValidatesInOrderAndOnlyThePastSlotReadsTheKey(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC) // Monday
	monday9 := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	sunday9 := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC) // closed and past
	nineToFive := []catalog.Interval{{Weekday: time.Monday, Start: 9 * 60, End: 17 * 60}}
	resources := fakeResources{ids: []string{"r1"}, hours: nineToFive}
	for _, c := range []struct {
		name    string
		req     Request
		wantErr error
	}{
		{"empty key on an unknown resource in the past", Request{"missing", sunday9, "Ann", ""}, ErrInvalidRequest},
		{"empty name on an unknown resource in the past", Request{"missing", sunday9, "", "K1"}, ErrInvalidRequest},
		{"non-aligned start on an unknown resource in the past", Request{"missing", sunday9.Add(15 * time.Minute), "Ann", "K1"}, ErrInvalidRequest},
		{"sub-second start", Request{"r1", monday9.Add(time.Hour).Add(time.Nanosecond), "Ann", "K1"}, ErrInvalidRequest},
		{"unknown resource, closed day, in the past", Request{"missing", sunday9, "Ann", "K1"}, ErrUnknownResource},
		{"closed day in the past", Request{"r1", sunday9, "Ann", "K1"}, ErrOutsideWorkingHours},
		{"before opening, in the past", Request{"r1", monday9.Add(-30 * time.Minute), "Ann", "K1"}, ErrOutsideWorkingHours},
		{"slot ending after closing", Request{"r1", monday9.Add(8 * time.Hour), "Ann", "K1"}, ErrOutsideWorkingHours},
		{"inside hours, in the past, key not stored", Request{"r1", monday9, "Ann", "K1"}, ErrSlotInPast},
	} {
		t.Run(c.name, func(t *testing.T) {
			repo := &fakeRepo{}
			_, _, err := NewService(repo, resources, func() time.Time { return now }).Book(context.Background(), c.req)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			wantReads := 0
			if c.wantErr == ErrSlotInPast {
				wantReads = 1
			}
			if repo.inserts != 0 || len(repo.saved) != 0 || repo.keyReads != wantReads {
				t.Fatalf("inserts %d, saved %d, key reads %d; want 0, 0, %d", repo.inserts, len(repo.saved), repo.keyReads, wantReads)
			}
		})
	}
}

// ADR-0020 rejects lookup_first: a new booking, and a taken slot until its insert is rejected,
// make no key read before the insert.
func TestService_BookMakesNoKeyReadOnTheNormalPath(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, openResources, clock)
	if _, _, err := s.Book(context.Background(), Request{"r1", clock(), "Ann", "K1"}); err != nil {
		t.Fatal(err)
	}
	if repo.inserts != 1 || repo.keyReads != 0 {
		t.Fatalf("new booking: inserts %d, key reads %d; want 1 and 0", repo.inserts, repo.keyReads)
	}
	taken := &fakeRepo{insertErr: fmt.Errorf("store: %w", ErrNotUnique)}
	if _, _, err := NewService(taken, openResources, clock).Book(context.Background(), Request{"r1", clock(), "Bob", "K2"}); !errors.Is(err, ErrSlotTaken) {
		t.Fatalf("err = %v, want ErrSlotTaken", err)
	}
	if taken.inserts != 1 || taken.keyReads != 1 {
		t.Fatalf("taken slot: inserts %d, key reads %d; want the insert, then one classifying read", taken.inserts, taken.keyReads)
	}
}

// ADR-0020: a past slot is resolved from the stored key, with one read and never an insert, by the
// same comparison as a rejected insert.
func TestService_BookResolvesAPastSlotFromTheStoredKey(t *testing.T) {
	start := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	now := func() time.Time { return start.Add(time.Second) }
	stored := Booking{ID: "b1", ResourceID: "r1", SlotStart: start, CustomerName: "Ann", IdempotencyKey: "K1", Status: StatusActive}
	for _, c := range []struct {
		name         string
		req          Request
		wantReplayed bool
		wantErr      error
	}{
		{"same payload", Request{"r1", start, "Ann", "K1"}, true, nil},
		{"other name", Request{"r1", start, "Bob", "K1"}, false, ErrKeyConflict},
		{"other past slot", Request{"r1", start.Add(-SlotLength), "Ann", "K1"}, false, ErrKeyConflict},
		{"key not stored", Request{"r1", start, "Ann", "K9"}, false, ErrSlotInPast},
	} {
		t.Run(c.name, func(t *testing.T) {
			repo := &fakeRepo{saved: []Booking{stored}}
			b, replayed, err := NewService(repo, openResources, now).Book(context.Background(), c.req)
			if !errors.Is(err, c.wantErr) || replayed != c.wantReplayed || (c.wantReplayed && b != stored) {
				t.Fatalf("Book = %+v, replayed %v, err %v; want replayed %v, err %v", b, replayed, err, c.wantReplayed, c.wantErr)
			}
			if repo.inserts != 0 || repo.keyReads != 1 || len(repo.saved) != 1 {
				t.Fatalf("inserts %d, key reads %d, saved %d; want 0, 1, 1", repo.inserts, repo.keyReads, len(repo.saved))
			}
		})
	}
}

func TestService_BookPassesResourceAndKeyReadErrorsThrough(t *testing.T) {
	boom := errors.New("disk I/O error")
	_, _, err := NewService(&fakeRepo{}, fakeResources{err: boom}, clock).Book(context.Background(), Request{"r1", clock(), "Ann", "K1"})
	if !errors.Is(err, boom) || errors.Is(err, ErrUnknownResource) {
		t.Fatalf("resource read: err = %v, want the read error only", err)
	}
	past := func() time.Time { return clock().Add(time.Hour) }
	_, _, err = NewService(&failingReadRepo{readErr: boom}, openResources, past).Book(context.Background(), Request{"r1", clock(), "Ann", "K1"})
	if !errors.Is(err, boom) || errors.Is(err, ErrSlotInPast) {
		t.Fatalf("past key read: err = %v, want the read error only", err)
	}
}

// ADR-0004 as read from the catalog's Interval: half-open in minutes of the UTC day, 24:00 the end
// of the day; the weekday is the UTC one.
func TestInsideWorkingHours(t *testing.T) {
	hours := []catalog.Interval{
		{Weekday: time.Monday, Start: 9 * 60, End: 17 * 60},
		{Weekday: time.Tuesday, Start: 22 * 60, End: catalog.EndOfDay},
	}
	at := func(day, h, m int) time.Time { return time.Date(2026, 11, day, h, m, 0, 0, time.UTC) } // 2 Nov 2026 is a Monday
	for _, c := range []struct {
		start time.Time
		want  bool
	}{
		{at(2, 9, 0), true},
		{at(2, 16, 30), true},
		{at(2, 8, 30), false},
		{at(2, 17, 0), false},
		{at(3, 23, 30), true}, // ends at 24:00
		{at(3, 21, 30), false},
		{at(4, 0, 0), false}, // Wednesday is closed
		{at(1, 9, 0), false}, // Sunday is closed
		{at(2, 9, 0).In(time.FixedZone("-10", -10*3600)), true}, // Sunday 23:00 local is Monday 09:00 UTC
	} {
		if got := insideWorkingHours(hours, c.start); got != c.want {
			t.Errorf("insideWorkingHours(%v) = %v, want %v", c.start, got, c.want)
		}
	}
}

// ADR-0016, ADR-0013: in the past means start earlier than now in whole epoch seconds.
func TestInPast(t *testing.T) {
	start := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		now  time.Time
		want bool
	}{
		{start, false},
		{start.Add(-time.Second), false},
		{start.Add(999 * time.Millisecond), false}, // still the same epoch second
		{start.Add(time.Second), true},
		{start.Add(time.Second).In(time.FixedZone("+05:45", 5*3600+45*60)), true},
	} {
		if got := inPast(start, c.now); got != c.want {
			t.Errorf("inPast(%v, %v) = %v, want %v", start, c.now, got, c.want)
		}
	}
}
