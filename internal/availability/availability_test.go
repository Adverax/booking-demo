package availability_test

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adverax/booking-demo/internal/availability"
	"github.com/adverax/booking-demo/internal/booking"
	"github.com/adverax/booking-demo/internal/catalog"
)

// Unit tests of the availability logic over in-memory ports. The API tests in internal/httpapi run
// it over the real store.

type fakeResources map[string]catalog.Resource

func (f fakeResources) Get(_ context.Context, id string) (catalog.Resource, error) {
	r, ok := f[id]
	if !ok {
		return catalog.Resource{}, catalog.ErrNotFound
	}
	return r, nil
}

// fakeBookings holds active booking starts per resource and counts its reads.
type fakeBookings struct {
	starts map[string][]time.Time
	reads  atomic.Int64
}

func (f *fakeBookings) ActiveBookingStarts(_ context.Context, resourceID string, from, to time.Time) ([]time.Time, error) {
	f.reads.Add(1)
	var out []time.Time
	for _, s := range f.starts[resourceID] {
		if !s.Before(from) && s.Before(to) {
			out = append(out, s)
		}
	}
	return out, nil
}

// monday is Monday 2026-11-02 UTC.
var monday = time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)

func mustResource(t *testing.T, id string, hours ...catalog.Interval) catalog.Resource {
	t.Helper()
	r, err := catalog.NewResource(id, "Dr. Lee", hours)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func monToFri(start, end int) []catalog.Interval {
	var h []catalog.Interval
	for d := time.Monday; d <= time.Friday; d++ {
		h = append(h, catalog.Interval{Weekday: d, Start: start, End: end})
	}
	return h
}

func newService(t *testing.T, r catalog.Resource, booked ...time.Time) (*availability.Service, *fakeBookings) {
	t.Helper()
	b := &fakeBookings{starts: map[string][]time.Time{r.ID: booked}}
	return availability.NewService(fakeResources{r.ID: r}, b), b
}

func starts(slots []availability.Slot) []time.Time {
	var out []time.Time
	for _, s := range slots {
		out = append(out, s.Start)
	}
	return out
}

// The component keeps its own slot length, so that it does not depend on Slot Booking; both must
// name the same 30 minutes (FR-003).
func TestAvailability_SlotLengthEqualsBookingSlotLength(t *testing.T) {
	if availability.SlotLength != booking.SlotLength {
		t.Fatalf("availability.SlotLength = %v, booking.SlotLength = %v", availability.SlotLength, booking.SlotLength)
	}
}

func TestFreeSlots_EnumeratesWorkingHoursMinusActiveBookings(t *testing.T) {
	r := mustResource(t, "r1", monToFri(9*60, 10*60+30)...)
	svc, _ := newService(t, r, monday.Add(9*time.Hour+30*time.Minute), monday.Add(9*time.Hour+30*time.Minute+24*time.Hour))
	got, err := svc.FreeSlots(context.Background(), "r1", monday, monday.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	want := []time.Time{
		monday.Add(9 * time.Hour), monday.Add(10 * time.Hour),
		monday.Add(33 * time.Hour), monday.Add(34 * time.Hour),
	}
	if !slices.Equal(starts(got), want) {
		t.Fatalf("starts = %v, want %v", starts(got), want)
	}
	for _, s := range got {
		if s.End.Sub(s.Start) != 30*time.Minute || s.Start.Location() != time.UTC || s.End.Location() != time.UTC {
			t.Fatalf("slot %+v is not a 30-minute UTC slot", s)
		}
	}
}

// An interval ending at 24:00 yields its last slot 23:30-24:00 and nothing of the next day
// (ADR-0004).
func TestFreeSlots_IntervalEndingAtTwentyFourHundred(t *testing.T) {
	r := mustResource(t, "r1", catalog.Interval{Weekday: time.Monday, Start: 23 * 60, End: catalog.EndOfDay})
	svc, _ := newService(t, r)
	got, err := svc.FreeSlots(context.Background(), "r1", monday, monday.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	want := []availability.Slot{
		{Start: monday.Add(23 * time.Hour), End: monday.Add(23*time.Hour + 30*time.Minute)},
		{Start: monday.Add(23*time.Hour + 30*time.Minute), End: monday.Add(24 * time.Hour)},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("slots = %v, want %v", got, want)
	}
}

// Only the UTC calendar day of from and to counts: a time of day or another zone does not shift
// the range.
func TestFreeSlots_UsesTheUtcDayOfTheBounds(t *testing.T) {
	r := mustResource(t, "r1", monToFri(9*60, 10*60)...)
	svc, _ := newService(t, r)
	zone := time.FixedZone("plus2", 2*3600)
	from := time.Date(2026, 11, 2, 1, 0, 0, 0, zone) // 2026-11-01 23:00 UTC: Sunday
	to := monday.Add(23*time.Hour + 59*time.Minute)  // still Monday
	got, err := svc.FreeSlots(context.Background(), "r1", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if want := []time.Time{monday.Add(9 * time.Hour), monday.Add(9*time.Hour + 30*time.Minute)}; !slices.Equal(starts(got), want) {
		t.Fatalf("starts = %v, want %v", starts(got), want)
	}
}

// A one-day range (from equal to to) is one day, as the existing range rules treat it (NG-3).
func TestFreeSlots_RangeRules(t *testing.T) {
	r := mustResource(t, "r1", monToFri(9*60, 10*60)...)
	for name, c := range map[string]struct {
		to      time.Time
		from    time.Time
		invalid bool
	}{
		"one day":           {from: monday, to: monday},
		"14 days":           {from: monday, to: monday.AddDate(0, 0, 13)},
		"15 days":           {from: monday, to: monday.AddDate(0, 0, 14), invalid: true},
		"from one day late": {from: monday.AddDate(0, 0, 1), to: monday, invalid: true},
	} {
		svc, b := newService(t, r)
		_, err := svc.FreeSlots(context.Background(), "r1", c.from, c.to)
		if got := errors.Is(err, availability.ErrInvalidRange); got != c.invalid {
			t.Errorf("%s: err = %v, want invalid %v", name, err, c.invalid)
		}
		if c.invalid && b.reads.Load() != 0 {
			t.Errorf("%s: bookings were read for an invalid range", name)
		}
	}
}

// An invalid range is rejected before the resource is read too: it is not reported as not found.
func TestFreeSlots_InvalidRangeOfUnknownResourceIsInvalidRange(t *testing.T) {
	svc := availability.NewService(fakeResources{}, &fakeBookings{})
	_, err := svc.FreeSlots(context.Background(), "missing-id", monday.AddDate(0, 0, 2), monday)
	if !errors.Is(err, availability.ErrInvalidRange) {
		t.Fatalf("err = %v, want ErrInvalidRange", err)
	}
}

func TestFreeSlots_UnknownResourceReadsNoBookings(t *testing.T) {
	b := &fakeBookings{}
	svc := availability.NewService(fakeResources{}, b)
	_, err := svc.FreeSlots(context.Background(), "missing-id", monday, monday)
	if !errors.Is(err, availability.ErrUnknownResource) || !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("err = %v, want ErrUnknownResource wrapping catalog.ErrNotFound", err)
	}
	if b.reads.Load() != 0 {
		t.Fatal("bookings were read for an unknown resource")
	}
}

type failingBookings struct{}

var errStore = errors.New("store down")

func (failingBookings) ActiveBookingStarts(context.Context, string, time.Time, time.Time) ([]time.Time, error) {
	return nil, errStore
}

func TestFreeSlots_StoreErrorIsReturned(t *testing.T) {
	r := mustResource(t, "r1", monToFri(9*60, 10*60)...)
	svc := availability.NewService(fakeResources{"r1": r}, failingBookings{})
	if _, err := svc.FreeSlots(context.Background(), "r1", monday, monday); !errors.Is(err, errStore) {
		t.Fatalf("err = %v, want the store error", err)
	}
}
