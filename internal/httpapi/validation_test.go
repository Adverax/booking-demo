package httpapi_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/adverax/booking-demo/internal/httpapi"
)

// API tests of booking validation (FR-005, INV-002, CARD-007) through the real handler over the real
// store, every response validated against api/openapi.yaml. The resource is open Mon-Fri
// 09:00-17:00 UTC; the clock reads fixedNow (Monday 2026-10-26 08:00 UTC) unless a test moves it.

// AC-015: a future Monday 08:00 slot, before opening, is a 4xx JSON error and nothing is stored.
func TestBookSlot_RejectsSlotOutsideWorkingHours(t *testing.T) {
	e := newReplayEnv(t)
	assertError(t, e.book(e.resource.Id, monday9.Add(-time.Hour), "Ann", "K1"), http.StatusUnprocessableEntity, httpapi.ErrorCodeSlotOutsideWorkingHours)
	if n := e.bookings(); n != 0 {
		t.Fatalf("%d bookings stored, want none", n)
	}
}

// AC-016: a slot inside working hours that started 1 hour ago is a 4xx JSON error and nothing is
// stored.
func TestBookSlot_RejectsSlotInThePast(t *testing.T) {
	e := newReplayEnv(t)
	e.setNow(monday9.Add(time.Hour))
	assertError(t, e.book(e.resource.Id, monday9, "Ann", "K1"), http.StatusUnprocessableEntity, httpapi.ErrorCodeSlotInPast)
	if n := e.bookings(); n != 0 {
		t.Fatalf("%d bookings stored, want none", n)
	}
}

// AC-017: a start of 09:15 UTC, not on the half hour, is a 4xx JSON error and nothing is stored;
// the start is not floored to 09:00 (which CARD-003 did before).
func TestBookSlot_RejectsNonAlignedStart(t *testing.T) {
	e := newReplayEnv(t)
	assertError(t, e.book(e.resource.Id, monday9.Add(15*time.Minute), "Ann", "K1"), http.StatusBadRequest, httpapi.ErrorCodeValidationError)
	assertError(t, e.book(e.resource.Id, monday9.Add(time.Second), "Ann", "K2"), http.StatusBadRequest, httpapi.ErrorCodeValidationError)
	if n := e.bookings(); n != 0 {
		t.Fatalf("%d bookings stored, want none", n)
	}
}

// AC-020: a booking of a resource that does not exist is a not-found JSON error.
func TestBookSlot_UnknownResourceNotFound(t *testing.T) {
	e := newReplayEnv(t)
	assertError(t, e.book("missing-id", monday9, "Ann", "K1"), http.StatusNotFound, httpapi.ErrorCodeNotFound)
	if n := e.bookings(); n != 0 {
		t.Fatalf("%d bookings stored, want none", n)
	}
}

// AC-050: with the clock at Monday 09:00:01 UTC, the Monday 09:00 slot started 1 second ago: a 4xx
// JSON error, nothing stored.
func TestBookSlot_RejectsSlotStartingOneSecondBeforeNow(t *testing.T) {
	e := newReplayEnv(t)
	e.setNow(monday9.Add(time.Second))
	assertError(t, e.book(e.resource.Id, monday9, "Ann", "K1"), http.StatusUnprocessableEntity, httpapi.ErrorCodeSlotInPast)
	if n := e.bookings(); n != 0 {
		t.Fatalf("%d bookings stored, want none", n)
	}
}

// AC-051: with the clock at exactly Monday 09:00:00 UTC, "Ann" books the Monday 09:00 slot, whose
// start equals now, and the booking is stored.
func TestBookSlot_AcceptsSlotStartingExactlyNow(t *testing.T) {
	e := newReplayEnv(t)
	e.setNow(monday9)
	b := e.mustBook(e.resource.Id, monday9, "Ann", "K1")
	if !b.SlotStart.Equal(monday9) || b.CustomerName != "Ann" || b.Status != httpapi.BookingStatusActive {
		t.Fatalf("booked %+v", b)
	}
	if n := e.count("SELECT count(*) FROM booking WHERE id = ? AND slot_start = ? AND customer_name = 'Ann'", b.Id, monday9.Unix()); n != 1 {
		t.Fatalf("stored rows for the booking = %d, want 1", n)
	}
}

// AC-052: an empty customer name for a free future slot and a new key is a 4xx JSON error and
// nothing is stored.
func TestBookSlot_RejectsEmptyCustomerName(t *testing.T) {
	e := newReplayEnv(t)
	assertError(t, e.book(e.resource.Id, monday9, "", "K1"), http.StatusBadRequest, httpapi.ErrorCodeValidationError)
	if n := e.bookings(); n != 0 {
		t.Fatalf("%d bookings stored, want none", n)
	}
}

// AC-057: with the clock at Monday 09:00:01 UTC and nothing stored under K9, booking the Monday
// 09:00 slot with K9 is the past-slot error, and nothing is stored under K9.
func TestBookSlot_UnknownKeyForPastSlotIsRejectedAsPast(t *testing.T) {
	e := newReplayEnv(t)
	e.setNow(monday9.Add(time.Second))
	assertError(t, e.book(e.resource.Id, monday9, "Ann", "K9"), http.StatusUnprocessableEntity, httpapi.ErrorCodeSlotInPast)
	if n := e.bookingsUnderKey("K9"); n != 0 {
		t.Fatalf("%d bookings under K9, want none", n)
	}
}

// The order of validation (ADR-0020: the key is looked up only when the slot is in the past): with
// K1 stored for the Monday 09:00 slot and the clock past it, a request under K1 that is malformed,
// names an unknown resource or lies outside working hours is answered as such, not as a replay or a
// key conflict; only the past-slot rejection consults K1.
func TestBookSlot_OnlyThePastSlotRejectionConsultsTheStoredKey(t *testing.T) {
	e := newReplayEnv(t)
	b1 := e.bookThenSlotStarts()
	for _, c := range []struct {
		name     string
		resource string
		start    time.Time
		cname    string
		status   int
		code     httpapi.ErrorCode
	}{
		{"empty name", e.resource.Id, monday9, "", http.StatusBadRequest, httpapi.ErrorCodeValidationError},
		{"start off the half hour", e.resource.Id, monday9.Add(15 * time.Minute), "Ann", http.StatusBadRequest, httpapi.ErrorCodeValidationError},
		{"unknown resource", "missing-id", monday9, "Ann", http.StatusNotFound, httpapi.ErrorCodeNotFound},
		{"before opening", e.resource.Id, monday9.Add(-time.Hour), "Ann", http.StatusUnprocessableEntity, httpapi.ErrorCodeSlotOutsideWorkingHours},
	} {
		t.Run(c.name, func(t *testing.T) {
			assertError(t, e.book(c.resource, c.start, c.cname, "K1"), c.status, c.code)
		})
	}
	e.assertReplay(e.book(e.resource.Id, monday9, "Ann", "K1"), b1)
	e.assertUnchanged(b1, "K1")
	if n := e.bookings(); n != 1 {
		t.Fatalf("%d bookings, want 1", n)
	}
}
