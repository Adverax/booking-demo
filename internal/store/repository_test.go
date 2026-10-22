package store

import (
	"context"
	"errors"
	"reflect"
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

func TestStore_ListResourcesReturnsAllInCreationOrderWithHoursByWeekday(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if rs, err := s.ListResources(ctx); err != nil || rs == nil || len(rs) != 0 {
		t.Fatalf("empty store: %#v, %v; want an empty, non-nil list", rs, err)
	}
	lee := catalog.Resource{ID: "z-lee", Name: "Dr. Lee", WeeklyHours: []catalog.Interval{
		{Weekday: time.Sunday, Start: 1320, End: 1440},
		{Weekday: time.Monday, Start: 540, End: 1020},
	}}
	room := catalog.Resource{ID: "a-room", Name: "Room A", WeeklyHours: []catalog.Interval{
		{Weekday: time.Saturday, Start: 0, End: 30},
	}}
	for _, r := range []catalog.Resource{lee, room} {
		if err := s.InsertResource(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	rs, err := s.ListResources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []catalog.Resource{lee, room}; !reflect.DeepEqual(rs, want) {
		t.Fatalf("listed %+v, want %+v", rs, want)
	}
}

func TestStore_GetResourceReturnsTheResourceOrNotFound(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	r := catalog.Resource{ID: "r1", Name: "Room A", WeeklyHours: []catalog.Interval{
		{Weekday: time.Monday, Start: 540, End: 1020},
		{Weekday: time.Friday, Start: 600, End: 1440},
	}}
	if err := s.InsertResource(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertResource(ctx, catalog.Resource{ID: "r2", Name: "Other", WeeklyHours: []catalog.Interval{{Weekday: time.Monday, Start: 0, End: 60}}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetResource(ctx, "r1")
	if err != nil || !reflect.DeepEqual(got, r) {
		t.Fatalf("got %+v, %v; want %+v", got, err, r)
	}
	if _, err := s.GetResource(ctx, "missing-id"); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("unknown id: err = %v, want catalog.ErrNotFound", err)
	}
}

// A closed store reports its failure as an error, not as "not found" or an empty list.
func TestStore_ResourceReadsReportStoreFailures(t *testing.T) {
	s := openTestStore(t)
	s.Close()
	if _, err := s.ListResources(context.Background()); err == nil {
		t.Error("ListResources on a closed store succeeded")
	}
	if _, err := s.GetResource(context.Background(), "r1"); err == nil || errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("GetResource on a closed store: err = %v", err)
	}
}

func TestStore_UpdateBookingStatusReturnsTheUpdatedBooking(t *testing.T) {
	s := openTestStore(t)
	insertResource(t, s.db, "r1")
	ctx := context.Background()
	stored := newBooking("b1", "r1", slotTime, "k1")
	if err := s.InsertBooking(ctx, stored); err != nil {
		t.Fatal(err)
	}

	got, err := s.UpdateBookingStatus(ctx, "b1", booking.StatusCancelled)
	want := stored
	want.Status = booking.StatusCancelled
	if err != nil || got != want {
		t.Fatalf("UpdateBookingStatus = %+v, %v; want %+v", got, err, want)
	}
	if read, err := s.GetBooking(ctx, "b1"); err != nil || read != want {
		t.Fatalf("GetBooking = %+v, %v; want %+v", read, err, want)
	}
}

// A cancelled booking frees its slot at once: a new active booking of the slot is accepted.
func TestStore_CancelledBookingFreesTheSlot(t *testing.T) {
	s := openTestStore(t)
	insertResource(t, s.db, "r1")
	ctx := context.Background()
	if err := s.InsertBooking(ctx, newBooking("b1", "r1", slotTime, "k1")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateBookingStatus(ctx, "b1", booking.StatusCancelled); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertBooking(ctx, newBooking("b2", "r1", slotTime, "k2")); err != nil {
		t.Fatalf("slot of the cancelled booking is not free: %v", err)
	}
}

// The schema's guard reaches the port as booking.ErrBookingCancelled, and nothing is changed.
func TestStore_UpdateBookingStatusMapsTheSchemaGuard(t *testing.T) {
	s := openTestStore(t)
	insertResource(t, s.db, "r1")
	if err := insertBooking(s.db, "b1", "r1", slot, "Ann", "k1", "cancelled"); err != nil {
		t.Fatal(err)
	}
	_, err := s.UpdateBookingStatus(context.Background(), "b1", booking.StatusActive)
	if !errors.Is(err, booking.ErrBookingCancelled) {
		t.Fatalf("err = %v, want ErrBookingCancelled", err)
	}
	if got := bookingStatus(t, s.db, "b1"); got != "cancelled" {
		t.Fatalf("status = %q, want cancelled", got)
	}
}

func TestStore_UnknownBookingIsNotFound(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if _, err := s.UpdateBookingStatus(ctx, "missing-id", booking.StatusCancelled); !errors.Is(err, booking.ErrNotFound) {
		t.Errorf("UpdateBookingStatus: err = %v, want ErrNotFound", err)
	}
	if _, err := s.GetBooking(ctx, "missing-id"); !errors.Is(err, booking.ErrNotFound) {
		t.Errorf("GetBooking: err = %v, want ErrNotFound", err)
	}
}
