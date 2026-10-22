package store

import (
	"context"
	"slices"
	"testing"
	"time"
)

// Integration test of the availability read against the real SQLite schema: only active bookings
// of the given resource whose start lies in [from, to) are returned, ordered by start, as UTC.
func TestStore_ActiveBookingStartsReadsActiveBookingsOfOneResourceInWindow(t *testing.T) {
	s := openTestStore(t)
	insertResource(t, s.db, "r1")
	insertResource(t, s.db, "r2")
	const half = int64(1800)
	for _, b := range []struct {
		id, resource string
		start        int64
		status       string
	}{
		{"before", "r1", slot - half, "active"},
		{"at-from", "r1", slot, "active"},
		{"later", "r1", slot + 4*half, "active"},
		{"inside", "r1", slot + 2*half, "active"},
		{"cancelled", "r1", slot + half, "cancelled"},
		{"other-resource", "r2", slot + 3*half, "active"},
		{"at-to", "r1", slot + 5*half, "active"},
	} {
		if err := insertBooking(s.db, b.id, b.resource, b.start, "Ann", "k-"+b.id, b.status); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.ActiveBookingStarts(context.Background(), "r1", time.Unix(slot, 0), time.Unix(slot+5*half, 0).In(time.FixedZone("z", 3600)))
	if err != nil {
		t.Fatal(err)
	}
	want := []time.Time{time.Unix(slot, 0).UTC(), time.Unix(slot+2*half, 0).UTC(), time.Unix(slot+4*half, 0).UTC()}
	if !slices.Equal(got, want) {
		t.Fatalf("starts = %v, want %v", got, want)
	}
}

func TestStore_ActiveBookingStartsOfResourceWithoutBookingsIsEmpty(t *testing.T) {
	s := openTestStore(t)
	got, err := s.ActiveBookingStarts(context.Background(), "nope", time.Unix(slot, 0), time.Unix(slot+86400, 0))
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got %v, %v; want an empty slice", got, err)
	}
}
