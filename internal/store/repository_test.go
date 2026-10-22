package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/adverax/booking-demo/internal/booking"
	"github.com/adverax/booking-demo/internal/catalog"
)

func newBooking(id, resourceID string, start time.Time, key string) booking.Booking {
	return booking.Booking{ID: id, ResourceID: resourceID, SlotStart: start, CustomerName: "Ann", IdempotencyKey: key, Status: booking.StatusActive}
}

var slotTime = time.Unix(slot, 0).UTC()

func TestStore_InsertBookingStoresEpochSeconds(t *testing.T) {
	s := openTestStore(t)
	insertResource(t, s.db, "r1")
	if err := s.InsertBooking(context.Background(), newBooking("b1", "r1", slotTime, "k1")); err != nil {
		t.Fatal(err)
	}
	var start int64
	var typ, status string
	if err := s.db.QueryRow("SELECT slot_start, typeof(slot_start), status FROM booking WHERE id = 'b1'").Scan(&start, &typ, &status); err != nil {
		t.Fatal(err)
	}
	if start != slot || typ != "integer" || status != "active" {
		t.Fatalf("stored %d (%s) %s", start, typ, status)
	}
}

// Either unique index rejects the insert as booking.ErrNotUnique; which one is not said.
func TestStore_InsertBookingReportsUniqueRejectionsAsNotUnique(t *testing.T) {
	s := openTestStore(t)
	insertResource(t, s.db, "r1")
	ctx := context.Background()
	if err := s.InsertBooking(ctx, newBooking("b1", "r1", slotTime, "k1")); err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string]booking.Booking{
		"same active slot": newBooking("b2", "r1", slotTime, "k2"),
		"same key":         newBooking("b3", "r1", slotTime.Add(time.Hour), "k1"),
	} {
		if err := s.InsertBooking(ctx, b); !errors.Is(err, booking.ErrNotUnique) {
			t.Errorf("%s: err = %v, want ErrNotUnique", name, err)
		}
	}
}

// Other rejections, such as an unknown resource, are not uniqueness rejections.
func TestStore_InsertBookingOfUnknownResourceIsNotNotUnique(t *testing.T) {
	s := openTestStore(t)
	err := s.InsertBooking(context.Background(), newBooking("b1", "nope", slotTime, "k1"))
	if err == nil || errors.Is(err, booking.ErrNotUnique) {
		t.Fatalf("err = %v, want a non-uniqueness error", err)
	}
}

func TestStore_InsertResourceStoresNameAndHours(t *testing.T) {
	s := openTestStore(t)
	r := catalog.Resource{ID: "r1", Name: "Dr. Lee", WeeklyHours: []catalog.Interval{
		{Weekday: time.Monday, Start: 540, End: 1020},
		{Weekday: time.Sunday, Start: 1320, End: 1440},
	}}
	if err := s.InsertResource(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM resource_working_hours WHERE resource_id = 'r1' AND
		((weekday = 1 AND start_minute = 540 AND end_minute = 1020) OR (weekday = 0 AND start_minute = 1320 AND end_minute = 1440))`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("stored %d of 2 intervals", n)
	}
}

// A failure after the resource row was written leaves nothing behind: never a resource without
// its working hours.
func TestStore_InsertResourceIsAllOrNothing(t *testing.T) {
	s := openTestStore(t)
	r := catalog.Resource{ID: "r1", Name: "Dr. Lee", WeeklyHours: []catalog.Interval{
		{Weekday: time.Monday, Start: 540, End: 1020},
		{Weekday: time.Monday, Start: 600, End: 660}, // second interval on one weekday: the primary key rejects it
	}}
	if err := s.InsertResource(context.Background(), r); err == nil {
		t.Fatal("second interval on a weekday accepted by the schema")
	}
	var resources, hours int
	if err := s.db.QueryRow("SELECT (SELECT count(*) FROM resource), (SELECT count(*) FROM resource_working_hours)").Scan(&resources, &hours); err != nil {
		t.Fatal(err)
	}
	if resources != 0 || hours != 0 {
		t.Fatalf("after a failed insert: %d resources, %d interval rows", resources, hours)
	}
}
