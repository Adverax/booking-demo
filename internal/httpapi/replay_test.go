package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adverax/booking-demo/internal/httpapi"
)

// API tests of the idempotent replay (FR-006, INV-003): insert-then-classify (ADR-0011) through the
// real handler over the real store, every response validated against api/openapi.yaml.

// monday930 is the free half-hour slot after monday9.
var monday930 = monday9.Add(30 * time.Minute)

// replayEnv is an apiEnv with a clock the test can move (ADR-0013) and one resource open
// Mon-Fri 09:00-17:00 UTC.
type replayEnv struct {
	*apiEnv
	now      atomic.Pointer[time.Time]
	resource httpapi.Resource
}

func newReplayEnv(t *testing.T) *replayEnv {
	t.Helper()
	e := &replayEnv{}
	e.setNow(fixedNow)
	e.apiEnv = newAPIEnvWithClock(t, func() time.Time { return *e.now.Load() })
	e.resource = e.createResource("Dr. Lee", weekdayHours(monToFri, "09:00", "17:00"))
	return e
}

func (e *replayEnv) setNow(t time.Time) { e.now.Store(&t) }

// mustBook books and fails the test unless the answer is 201; it returns the new booking.
func (e *replayEnv) mustBook(resourceID string, start time.Time, name, key string) httpapi.Booking {
	e.t.Helper()
	rec := e.book(resourceID, start, name, key)
	if rec.Code != http.StatusCreated {
		e.t.Fatalf("book %s %v %q %q: %d %s, want 201", resourceID, start, name, key, rec.Code, rec.Body)
	}
	return decodeBooking(e.t, rec)
}

// assertReplay checks that rec is 200 with exactly the booking want.
func (e *replayEnv) assertReplay(rec *httptest.ResponseRecorder, want httpapi.Booking) {
	e.t.Helper()
	if rec.Code != http.StatusOK {
		e.t.Fatalf("replay: %d %s, want 200 with the original booking", rec.Code, rec.Body)
	}
	if got := decodeBooking(e.t, rec); got != want {
		e.t.Fatalf("replay returned %+v, want %+v", got, want)
	}
}

func (e *replayEnv) bookings() int { return e.count("SELECT count(*) FROM booking") }

func (e *replayEnv) bookingsUnderKey(key string) int {
	return e.count("SELECT count(*) FROM booking WHERE idempotency_key = ?", key)
}

// assertUnchanged checks that the stored row of b still holds b's payload and status.
func (e *replayEnv) assertUnchanged(b httpapi.Booking, key string) {
	e.t.Helper()
	n := e.count(`SELECT count(*) FROM booking WHERE id = ? AND resource_id = ? AND slot_start = ?
		AND customer_name = ? AND idempotency_key = ? AND status = ?`,
		b.Id, b.ResourceId, b.SlotStart.Unix(), b.CustomerName, key, string(b.Status))
	if n != 1 {
		e.t.Fatalf("stored booking %s changed", b.Id)
	}
}

// AC-053 (ADR-0018): an empty key is a 4xx JSON error with a stable code, and nothing is stored.
func TestBookSlot_RejectsEmptyIdempotencyKey(t *testing.T) {
	e := newReplayEnv(t)
	assertError(t, e.book(e.resource.Id, monday9, "Ann", ""), http.StatusBadRequest, httpapi.ErrorCodeValidationError)
	if n := e.bookings(); n != 0 {
		t.Fatalf("%d bookings stored", n)
	}
}

// AC-021: an identical repeat returns the original booking with the same id.
func TestBookSlot_RepeatReturnsOriginalBooking(t *testing.T) {
	e := newReplayEnv(t)
	b1 := e.mustBook(e.resource.Id, monday9, "Ann", "K1")
	e.assertReplay(e.book(e.resource.Id, monday9, "Ann", "K1"), b1)
}

// AC-022: an identical repeat leaves exactly one booking.
func TestBookSlot_RepeatCreatesNothingNew(t *testing.T) {
	e := newReplayEnv(t)
	b1 := e.mustBook(e.resource.Id, monday9, "Ann", "K1")
	e.assertReplay(e.book(e.resource.Id, monday9, "Ann", "K1"), b1)
	if n := e.bookings(); n != 1 {
		t.Fatalf("%d bookings, want 1", n)
	}
}

// AC-023: 10 simultaneous identical requests with one key yield one booking, and all 10 responses
// carry its id.
func TestBookSlot_ConcurrentRepeatsYieldOneBooking(t *testing.T) {
	e := newReplayEnv(t)
	body, err := json.Marshal(httpapi.BookSlotRequest{ResourceId: e.resource.Id, SlotStart: monday9, CustomerName: "Ann", IdempotencyKey: "K1"})
	if err != nil {
		t.Fatal(err)
	}
	recs := make([]*httptest.ResponseRecorder, 10)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range recs {
		wg.Go(func() {
			<-start
			recs[i] = serve(e.handler, http.MethodPost, "/bookings", string(body))
		})
	}
	close(start)
	wg.Wait()

	created, ids := 0, map[string]bool{}
	for i, rec := range recs {
		e.validate(http.MethodPost, "/bookings", rec)
		switch rec.Code {
		case http.StatusCreated:
			created++
		case http.StatusOK:
		default:
			t.Fatalf("request %d: %d %s, want 201 or 200", i, rec.Code, rec.Body)
		}
		ids[decodeBooking(t, rec).Id] = true
	}
	if created != 1 || len(ids) != 1 {
		t.Fatalf("%d created, %d distinct ids; want 1 and 1", created, len(ids))
	}
	if n := e.bookings(); n != 1 {
		t.Fatalf("%d bookings, want 1", n)
	}
}

// AC-038, AC-045, AC-046 (ADR-0001, ADR-0015): the same key and slot with a customer name that is
// not byte-for-byte equal is a key conflict, and the stored booking is unchanged.
func TestBookSlot_RepeatWithDifferentNameIsKeyConflict(t *testing.T) {
	assertNameIsKeyConflict(t, "Bob")
}

func TestBookSlot_CustomerNameComparisonIsCaseSensitive(t *testing.T) {
	assertNameIsKeyConflict(t, "ann")
}

func TestBookSlot_CustomerNameComparisonDoesNotTrim(t *testing.T) {
	assertNameIsKeyConflict(t, "Ann ")
}

func assertNameIsKeyConflict(t *testing.T, name string) {
	t.Helper()
	e := newReplayEnv(t)
	b1 := e.mustBook(e.resource.Id, monday9, "Ann", "K1")
	assertError(t, e.book(e.resource.Id, monday9, name, "K1"), http.StatusConflict, httpapi.ErrorCodeIdempotencyKeyConflict)
	e.assertUnchanged(b1, "K1")
	if n := e.bookings(); n != 1 {
		t.Fatalf("%d bookings, want 1", n)
	}
}

// AC-039: the key repeated for another free slot of the same resource is a key conflict, and that
// slot stays free.
func TestBookSlot_RepeatWithDifferentSlotIsKeyConflict(t *testing.T) {
	e := newReplayEnv(t)
	b1 := e.mustBook(e.resource.Id, monday9, "Ann", "K1")
	assertError(t, e.book(e.resource.Id, monday930, "Ann", "K1"), http.StatusConflict, httpapi.ErrorCodeIdempotencyKeyConflict)
	if n := e.activeBookings(e.resource.Id, monday930); n != 0 {
		t.Fatalf("Monday 09:30 has %d active bookings, want 0", n)
	}
	e.assertUnchanged(b1, "K1")
}

// AC-040: the key repeated for the same start on another resource is a key conflict, and that
// resource has no booking.
func TestBookSlot_RepeatWithDifferentResourceIsKeyConflict(t *testing.T) {
	e := newReplayEnv(t)
	r2 := e.createResource("R2", weekdayHours(monToFri, "09:00", "17:00"))
	b1 := e.mustBook(e.resource.Id, monday9, "Ann", "K1")
	assertError(t, e.book(r2.Id, monday9, "Ann", "K1"), http.StatusConflict, httpapi.ErrorCodeIdempotencyKeyConflict)
	if n := e.count("SELECT count(*) FROM booking WHERE resource_id = ?", r2.Id); n != 0 {
		t.Fatalf("R2 has %d bookings, want 0", n)
	}
	e.assertUnchanged(b1, "K1")
}

// AC-041 (ADR-0002): an identical repeat after cancellation returns the original booking with its
// id and status cancelled, and creates nothing, although the slot is free again.
func TestBookSlot_RepeatAfterCancelReturnsCancelledOriginal(t *testing.T) {
	e := newReplayEnv(t)
	b1 := e.mustBook(e.resource.Id, monday9, "Ann", "K1")
	if rec := e.cancel(b1.Id); rec.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body)
	}
	want := b1
	want.Status = httpapi.BookingStatusCancelled
	e.assertReplay(e.book(e.resource.Id, monday9, "Ann", "K1"), want)
	if n := e.bookings(); n != 1 {
		t.Fatalf("%d bookings, want 1", n)
	}
}

// AC-043, AC-044 (ADR-0015): keys that differ only by case or by a trailing space are different
// keys, so the request books the free slot as a new booking.
func TestBookSlot_KeysDifferingOnlyByCaseAreDifferentKeys(t *testing.T) {
	assertDifferentKey(t, "Key-1", "key-1")
}

func TestBookSlot_KeysDifferingOnlyByTrailingSpaceAreDifferentKeys(t *testing.T) {
	assertDifferentKey(t, "Key-1", "Key-1 ")
}

func assertDifferentKey(t *testing.T, first, second string) {
	t.Helper()
	e := newReplayEnv(t)
	b1 := e.mustBook(e.resource.Id, monday9, "Ann", first)
	b2 := e.mustBook(e.resource.Id, monday930, "Ann", second)
	if b2.Id == b1.Id || !b2.SlotStart.Equal(monday930) {
		t.Fatalf("second booking %+v, want a new id for Monday 09:30", b2)
	}
	if e.bookingsUnderKey(first) != 1 || e.bookingsUnderKey(second) != 1 {
		t.Fatalf("want one booking under %q and one under %q", first, second)
	}
}

// AC-047: a new key for a slot that is actively booked under another key is slot_taken, and
// nothing is stored under the new key.
func TestBookSlot_SlotTakenStoresNothingUnderNewKey(t *testing.T) {
	e := newReplayEnv(t)
	e.mustBook(e.resource.Id, monday9, "Ann", "K1")
	assertError(t, e.book(e.resource.Id, monday9, "Bob", "K2"), http.StatusConflict, httpapi.ErrorCodeSlotTaken)
	if n := e.bookingsUnderKey("K2"); n != 0 {
		t.Fatalf("%d bookings under K2, want 0", n)
	}
	if n := e.activeBookings(e.resource.Id, monday9); n != 1 {
		t.Fatalf("%d active bookings for the slot, want 1", n)
	}
}

// Late replay (ADR-0020): B1 is booked while the clock reads Monday 08:00 UTC, then the clock
// moves to Monday 09:00:01 UTC, after the slot started.
func (e *replayEnv) bookThenSlotStarts() httpapi.Booking {
	e.t.Helper()
	e.setNow(time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC))
	b1 := e.mustBook(e.resource.Id, monday9, "Ann", "K1")
	e.setNow(time.Date(2026, 11, 2, 9, 0, 1, 0, time.UTC))
	return b1
}

// AC-055: an identical repeat after the slot started returns B1, not a past-slot error.
func TestBookSlot_RepeatAfterSlotStartedReturnsOriginalBooking(t *testing.T) {
	e := newReplayEnv(t)
	b1 := e.bookThenSlotStarts()
	e.assertReplay(e.book(e.resource.Id, monday9, "Ann", "K1"), b1)
}

// AC-058: an identical repeat after the slot started leaves exactly one booking.
func TestBookSlot_RepeatAfterSlotStartedCreatesNothingNew(t *testing.T) {
	e := newReplayEnv(t)
	b1 := e.bookThenSlotStarts()
	e.assertReplay(e.book(e.resource.Id, monday9, "Ann", "K1"), b1)
	if n := e.bookings(); n != 1 {
		t.Fatalf("%d bookings, want 1", n)
	}
}

// AC-056: the key repeated for the same past slot with another name is the key conflict of
// ADR-0001, not the past-slot code.
func TestBookSlot_PastSlotRepeatWithDifferentPayloadIsKeyConflict(t *testing.T) {
	e := newReplayEnv(t)
	b1 := e.bookThenSlotStarts()
	assertError(t, e.book(e.resource.Id, monday9, "Bob", "K1"), http.StatusConflict, httpapi.ErrorCodeIdempotencyKeyConflict)
	e.assertUnchanged(b1, "K1")
	if n := e.bookings(); n != 1 {
		t.Fatalf("%d bookings, want 1", n)
	}
}

// The replay answer is byte-identical to the first answer apart from the status line, so a client
// that lost the 201 reads the same booking from the 200.
func TestBookSlot_ReplayBodyEqualsTheOriginalBody(t *testing.T) {
	e := newReplayEnv(t)
	first := e.book(e.resource.Id, monday9, "Ann", "K1")
	again := e.book(e.resource.Id, monday9, "Ann", "K1")
	if first.Code != http.StatusCreated || again.Code != http.StatusOK || first.Body.String() != again.Body.String() {
		t.Fatalf("first %d %s, replay %d %s", first.Code, first.Body, again.Code, again.Body)
	}
}
