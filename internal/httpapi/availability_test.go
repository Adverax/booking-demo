package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adverax/booking-demo/internal/availability"
	"github.com/adverax/booking-demo/internal/booking"
	"github.com/adverax/booking-demo/internal/catalog"
	"github.com/adverax/booking-demo/internal/httpapi"
)

// API tests of availability search (FR-003, FR-004): the real handler over the real store, every
// response validated against the getAvailability operation of api/openapi.yaml.

// mondayDate is the UTC day of monday9, Monday 2026-11-02.
const mondayDate = "2026-11-02"

func availabilityTarget(resourceID, from, to string) string {
	return "/resources/" + resourceID + "/availability?from=" + from + "&to=" + to
}

// availabilityOn queries h and validates the answer against the contract.
func (e *apiEnv) availabilityOn(h http.Handler, resourceID, from, to string) *httptest.ResponseRecorder {
	e.t.Helper()
	target := availabilityTarget(resourceID, from, to)
	rec := serve(h, http.MethodGet, target, "")
	e.validateAt(http.MethodGet, "/resources/{resourceId}/availability", target, rec)
	return rec
}

func (e *apiEnv) availability(resourceID, from, to string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.availabilityOn(e.handler, resourceID, from, to)
}

// freeStarts queries the range and returns the starts of its free slots; anything but 200 fails.
func (e *apiEnv) freeStarts(resourceID, from, to string) []time.Time {
	e.t.Helper()
	rec := e.availability(resourceID, from, to)
	if rec.Code != http.StatusOK {
		e.t.Fatalf("availability: %d %s", rec.Code, rec.Body)
	}
	var a httpapi.Availability
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
		e.t.Fatalf("body is not an availability: %v; body %s", err, rec.Body)
	}
	if a.ResourceId != resourceID || a.From.String() != from || a.To.String() != to {
		e.t.Fatalf("availability echoes %s %s..%s, want %s %s..%s", a.ResourceId, a.From, a.To, resourceID, from, to)
	}
	starts := make([]time.Time, 0, len(a.Slots))
	for _, s := range a.Slots {
		if s.End.Sub(s.Start) != 30*time.Minute {
			e.t.Fatalf("slot %v-%v is not 30 minutes long", s.Start, s.End)
		}
		starts = append(starts, s.Start)
	}
	return starts
}

func (e *apiEnv) mondayNineToTen() httpapi.Resource {
	e.t.Helper()
	return e.createResource("Dr. Lee", weekdayHours(monToFri, "09:00", "10:00"))
}

// AC-007: a resource open Mon 09:00-10:00 UTC with no bookings has exactly the 09:00 and 09:30
// slots on that Monday, each 30 minutes long, as RFC 3339 UTC instants.
func TestAvailability_ReturnsHalfHourSlotsInsideWorkingHours(t *testing.T) {
	e := newAPIEnv(t)
	r := e.mondayNineToTen()
	got := e.freeStarts(r.Id, mondayDate, mondayDate)
	if want := []time.Time{monday9, monday930}; !slices.Equal(got, want) {
		t.Fatalf("free starts = %v, want %v", got, want)
	}
	body := e.availability(r.Id, mondayDate, mondayDate).Body.String()
	want := `{"from":"2026-11-02","resourceId":"` + r.Id + `","slots":[{"end":"2026-11-02T09:30:00Z","start":"2026-11-02T09:00:00Z"},{"end":"2026-11-02T10:00:00Z","start":"2026-11-02T09:30:00Z"}],"to":"2026-11-02"}` + "\n"
	if body != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// AC-008: with an active booking for 09:00, only the 09:30 slot is returned.
func TestAvailability_ExcludesActiveBookedSlot(t *testing.T) {
	e := newAPIEnv(t)
	r := e.mondayNineToTen()
	if rec := e.book(r.Id, monday9, "Ann", "key-ann"); rec.Code != http.StatusCreated {
		t.Fatalf("book: %d %s", rec.Code, rec.Body)
	}
	if got, want := e.freeStarts(r.Id, mondayDate, mondayDate), []time.Time{monday930}; !slices.Equal(got, want) {
		t.Fatalf("free starts = %v, want %v", got, want)
	}
}

// AC-009: no returned slot starts before 09:00 or ends after 10:00.
func TestAvailability_ExcludesSlotsOutsideWorkingHours(t *testing.T) {
	e := newAPIEnv(t)
	r := e.mondayNineToTen()
	got := e.freeStarts(r.Id, mondayDate, mondayDate)
	if len(got) == 0 {
		t.Fatal("no slots returned")
	}
	for _, s := range got {
		if s.Before(monday9) || s.Add(30*time.Minute).After(monday9.Add(time.Hour)) {
			t.Fatalf("slot %v lies outside 09:00-10:00", s)
		}
	}
}

// AC-010: a resource open Mon-Fri only has 0 slots on a Saturday.
func TestAvailability_NoSlotsOnNonWorkingDay(t *testing.T) {
	e := newAPIEnv(t)
	r := e.createResource("Dr. Lee", weekdayHours(monToFri, "09:00", "17:00"))
	if got := e.freeStarts(r.Id, "2026-11-07", "2026-11-07"); len(got) != 0 {
		t.Fatalf("Saturday has %d slots, want 0: %v", len(got), got)
	}
}

// AC-011: a range of exactly 14 days succeeds with slots for every day of the range.
func TestAvailability_AcceptsRangeOf14Days(t *testing.T) {
	e := newAPIEnv(t)
	everyDay := append(slices.Clone(monToFri), httpapi.WeekdaySaturday, httpapi.WeekdaySunday)
	r := e.createResource("Dr. Lee", weekdayHours(everyDay, "09:00", "10:00"))
	got := e.freeStarts(r.Id, mondayDate, "2026-11-15")
	var want []time.Time
	for d := range 14 {
		day := monday9.AddDate(0, 0, d)
		want = append(want, day, day.Add(30*time.Minute))
	}
	if !slices.Equal(got, want) {
		t.Fatalf("free starts = %v, want %v", got, want)
	}
}

// AC-012: a range of 15 days is 400 validation_error.
func TestAvailability_RejectsRangeOver14Days(t *testing.T) {
	e := newAPIEnv(t)
	r := e.mondayNineToTen()
	assertError(t, e.availability(r.Id, mondayDate, "2026-11-16"), http.StatusBadRequest, httpapi.ErrorCodeValidationError)
}

// AC-013: an unknown resource is 404 not_found.
func TestAvailability_UnknownResourceNotFound(t *testing.T) {
	e := newAPIEnv(t)
	assertError(t, e.availability("missing-id", mondayDate, mondayDate), http.StatusNotFound, httpapi.ErrorCodeNotFound)
}

// countingBookings counts the reads of active bookings that reach the store.
type countingBookings struct {
	availability.Bookings
	reads atomic.Int64
}

func (c *countingBookings) ActiveBookingStarts(ctx context.Context, resourceID string, from, to time.Time) ([]time.Time, error) {
	c.reads.Add(1)
	return c.Bookings.ActiveBookingStarts(ctx, resourceID, from, to)
}

// AC-054 (ADR-0018): a range whose from is 2 days after its to is 400 validation_error, answered
// before any booking is read. The same handler reads bookings for a valid range, so the count is
// not vacuous.
func TestAvailability_RejectsRangeStartAfterEnd(t *testing.T) {
	e := newAPIEnv(t)
	r := e.mondayNineToTen()
	cat := catalog.New(e.store)
	reads := &countingBookings{Bookings: e.store}
	h := httpapi.NewHandler(httpapi.NewServer(cat, availability.NewService(cat, reads), booking.NewService(e.store, cat, func() time.Time { return fixedNow })))

	assertError(t, e.availabilityOn(h, r.Id, "2026-11-04", mondayDate), http.StatusBadRequest, httpapi.ErrorCodeValidationError)
	if n := reads.reads.Load(); n != 0 {
		t.Fatalf("%d booking reads for a reversed range, want 0", n)
	}
	if rec := e.availabilityOn(h, r.Id, mondayDate, mondayDate); rec.Code != http.StatusOK || reads.reads.Load() != 1 {
		t.Fatalf("valid range: %d, %d booking reads; want 200 and 1 read", rec.Code, reads.reads.Load())
	}
}

// A malformed or missing date is 400 validation_error from the generated binding.
func TestAvailability_MalformedDateIsValidationError(t *testing.T) {
	e := newAPIEnv(t)
	r := e.mondayNineToTen()
	for _, target := range []string{
		availabilityTarget(r.Id, "2026-13-01", mondayDate),
		"/resources/" + r.Id + "/availability?from=" + mondayDate,
	} {
		rec := serve(e.handler, http.MethodGet, target, "")
		e.validateAt(http.MethodGet, "/resources/{resourceId}/availability", target, rec)
		assertError(t, rec, http.StatusBadRequest, httpapi.ErrorCodeValidationError)
	}
}

// AC-025 (FR-007): once the Monday 09:00 booking is cancelled, the next availability query lists
// the slot as free.
func TestCancelBooking_FreesSlotImmediately(t *testing.T) {
	e := newAPIEnv(t)
	b := e.bookAnn()
	if slices.Contains(e.freeStarts(b.ResourceId, mondayDate, mondayDate), monday9) {
		t.Fatal("booked Monday 09:00 is listed as free")
	}
	if rec := e.cancel(b.Id); rec.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body)
	}
	if !slices.Contains(e.freeStarts(b.ResourceId, mondayDate, mondayDate), monday9) {
		t.Fatal("cancelled Monday 09:00 is not listed as free")
	}
}

// AC-042 (FR-006, INV-004): repeating the request of a cancelled booking with its key K1 creates
// nothing: there is still 1 booking, and Monday 09:00 is still listed as free.
func TestBookSlot_RepeatAfterCancelCreatesNoNewBooking(t *testing.T) {
	e := newReplayEnv(t)
	b1 := e.mustBook(e.resource.Id, monday9, "Ann", "K1")
	if rec := e.cancel(b1.Id); rec.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body)
	}
	if rec := e.book(e.resource.Id, monday9, "Ann", "K1"); rec.Code != http.StatusOK {
		t.Fatalf("repeat: %d %s, want 200 with the original booking", rec.Code, rec.Body)
	}
	if n := e.bookings(); n != 1 {
		t.Fatalf("%d bookings, want 1", n)
	}
	if !slices.Contains(e.freeStarts(e.resource.Id, mondayDate, mondayDate), monday9) {
		t.Fatal("Monday 09:00 is not listed as free after the repeat")
	}
}
