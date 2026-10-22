package booking_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adverax/booking-demo/internal/booking"
)

// Property tests of the idempotent replay (FR-006, INV-003, ADR-0011) against the real store: the
// key's uniqueness is held by the schema (ADR-0008) and the classification reads what is stored,
// so a fake would prove nothing. Seeds are fixed, so a failure reproduces.

// movableClock is an injected clock (ADR-0013) a test can move, also while requests run.
type movableClock struct{ v atomic.Pointer[time.Time] }

func newMovableClock(t time.Time) *movableClock {
	c := &movableClock{}
	c.set(t)
	return c
}

func (c *movableClock) set(t time.Time)     { c.v.Store(&t) }
func (c *movableClock) now() time.Time      { return *c.v.Load() }
func (c *movableClock) add(d time.Duration) { c.set(c.now().Add(d)) }

func (e *env) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// stored reads booking id as the store holds it, for comparing before and after.
func (e *env) stored(t *testing.T, id string) booking.Booking {
	t.Helper()
	b, err := e.svc.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// nameVariants are customer names, each a distinct byte sequence, some of which read alike: case, whitespace, and two Unicode-equivalent but byte-different spellings of "Renée".
var nameVariants = []string{"Ann", "ann", "ANN", "Ann ", " Ann", "An n", "Ann\t", "Bob", "Ren\u00e9e", "Rene\u0301e"}

// EC-002: an identical request repeated with its key any number of times, at any time (also after
// the slot started, and after cancellation), creates no second booking and returns the original
// in its current state.
func TestProperty_Booking_IdempotentUnderRetry(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 5))
	clock := newMovableClock(fixedClock())
	e := newEnv(t, clock.now)
	ctx := context.Background()
	for iter := range 40 {
		clock.set(fixedClock())
		resourceID := e.newResource(t)
		req := booking.Request{
			ResourceID:     resourceID,
			SlotStart:      inAnyZone(rng, base.Add(time.Duration(rng.IntN(8))*booking.SlotLength)),
			CustomerName:   nameVariants[rng.IntN(len(nameVariants))],
			IdempotencyKey: fmt.Sprintf("retry-%d", iter),
		}
		orig, replayed, err := e.svc.Book(ctx, req)
		if err != nil || replayed {
			t.Fatalf("iter %d first request: replayed %v, err %v", iter, replayed, err)
		}
		want := orig
		for r := range 1 + rng.IntN(12) {
			switch rng.IntN(4) {
			case 0: // time passes, possibly far past the slot start
				clock.add(time.Duration(rng.Int64N(int64(30 * 24 * time.Hour))))
			case 1: // the customer cancels in between
				if _, err := e.svc.Cancel(ctx, orig.ID); err != nil {
					t.Fatal(err)
				}
				want.Status = booking.StatusCancelled
			}
			again := req
			again.SlotStart = inAnyZone(rng, booking.CanonicalSlotStart(req.SlotStart)) // another spelling of the same slot
			got, replayed, err := e.svc.Book(ctx, again)
			if err != nil || !replayed || got != want {
				t.Fatalf("iter %d repeat %d (clock %v): got %+v, replayed %v, err %v; want %+v replayed", iter, r, clock.now(), got, replayed, err, want)
			}
		}
		if n := e.count(t, "SELECT count(*) FROM booking WHERE resource_id = ?", resourceID); n != 1 {
			t.Fatalf("iter %d: %d bookings, want 1", iter, n)
		}
	}
}

// bookAllAtOnce releases every request at the same moment and returns each one's outcome.
func (e *env) bookAllAtOnce(reqs []booking.Request) ([]booking.Booking, []bool, []error) {
	bs, replays, errs := make([]booking.Booking, len(reqs)), make([]bool, len(reqs)), make([]error, len(reqs))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, req := range reqs {
		wg.Go(func() {
			<-start
			bs[i], replays[i], errs[i] = e.svc.Book(context.Background(), req)
		})
	}
	close(start)
	wg.Wait()
	return bs, replays, errs
}

// EC-003: for any number of simultaneous identical requests sharing one key, exactly one booking is
// created and every response carries its id.
func TestProperty_Booking_ConcurrentSameKeyYieldsOneBooking(t *testing.T) {
	rng := rand.New(rand.NewPCG(6, 6))
	e := newEnv(t, fixedClock)
	sizes := []int{2, 10, 50}
	for range 9 {
		sizes = append(sizes, 2+rng.IntN(49))
	}
	for iter, n := range sizes {
		resourceID := e.newResource(t)
		req := booking.Request{ResourceID: resourceID, SlotStart: base, CustomerName: "Ann", IdempotencyKey: fmt.Sprintf("same-%d", iter)}
		reqs := make([]booking.Request, n)
		for i := range reqs {
			reqs[i] = req
		}
		bs, replays, errs := e.bookAllAtOnce(reqs)
		created, ids := 0, map[string]bool{}
		for i := range reqs {
			if errs[i] != nil {
				t.Fatalf("%d requests, request %d: %v", n, i, errs[i])
			}
			if !replays[i] {
				created++
			}
			ids[bs[i].ID] = true
		}
		if created != 1 || len(ids) != 1 {
			t.Fatalf("%d requests: %d created, %d distinct ids; want 1 and 1", n, created, len(ids))
		}
		if c := e.count(t, "SELECT count(*) FROM booking WHERE resource_id = ?", resourceID); c != 1 {
			t.Fatalf("%d requests: %d bookings, want 1", n, c)
		}
	}
}

// otherPayload returns req with at least one of resource id, slot start and customer name
// changed; resources are the resource ids to choose from.
func otherPayload(rng *rand.Rand, req booking.Request, resources []string) booking.Request {
	out := req
	for out.ResourceID == req.ResourceID && booking.CanonicalSlotStart(out.SlotStart).Equal(booking.CanonicalSlotStart(req.SlotStart)) &&
		out.CustomerName == req.CustomerName {
		if rng.IntN(2) == 0 {
			out.ResourceID = resources[rng.IntN(len(resources))]
		}
		if rng.IntN(2) == 0 {
			out.SlotStart = inAnyZone(rng, base.Add(time.Duration(rng.IntN(8))*booking.SlotLength))
		}
		if rng.IntN(2) == 0 {
			out.CustomerName = nameVariants[rng.IntN(len(nameVariants))]
		}
	}
	return out
}

// EC-004: a request repeating a stored key with any differing resource id, slot start or customer
// name, also for a slot that already started, is a key conflict, creates nothing and changes
// nothing.
func TestProperty_Booking_KeyReuseWithDifferentPayloadCreatesNothing(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 7))
	clock := newMovableClock(fixedClock())
	e := newEnv(t, clock.now)
	ctx := context.Background()
	for iter := range 150 {
		clock.set(fixedClock())
		resources := []string{e.newResource(t), e.newResource(t)}
		req := booking.Request{
			ResourceID:     resources[rng.IntN(2)],
			SlotStart:      inAnyZone(rng, base.Add(time.Duration(rng.IntN(8))*booking.SlotLength)),
			CustomerName:   "Ann",
			IdempotencyKey: fmt.Sprintf("reuse-%d", iter),
		}
		orig, _, err := e.svc.Book(ctx, req)
		if err != nil {
			t.Fatalf("iter %d first: %v", iter, err)
		}
		if rng.IntN(3) == 0 {
			if orig, err = e.svc.Cancel(ctx, orig.ID); err != nil {
				t.Fatal(err)
			}
		}
		if rng.IntN(2) == 0 { // the slot has started
			clock.set(orig.SlotStart.Add(time.Duration(1 + rng.Int64N(int64(48*time.Hour)))))
		}
		total := e.count(t, "SELECT count(*) FROM booking")

		other := otherPayload(rng, req, resources)
		got, replayed, err := e.svc.Book(ctx, other)
		if !errors.Is(err, booking.ErrKeyConflict) || replayed || got != (booking.Booking{}) {
			t.Fatalf("iter %d: %+v then %+v: got %+v, replayed %v, err %v; want ErrKeyConflict", iter, req, other, got, replayed, err)
		}
		if n := e.count(t, "SELECT count(*) FROM booking"); n != total {
			t.Fatalf("iter %d: %d bookings after the conflict, %d before", iter, n, total)
		}
		if after := e.stored(t, orig.ID); after != orig {
			t.Fatalf("iter %d: stored booking changed from %+v to %+v", iter, orig, after)
		}
	}
}

// ADR-0012/R1: for any number of concurrent replays of one stored key, with the same or with
// different payloads, every request ends as the original booking or a classified key conflict,
// never as a lock or busy error.
func TestProperty_Booking_ConcurrentReplaysNeverSeeLockErrors(t *testing.T) {
	rng := rand.New(rand.NewPCG(8, 8))
	e := newEnv(t, fixedClock)
	resources := []string{e.newResource(t), e.newResource(t)}
	sizes := []int{2, 50}
	for range 10 {
		sizes = append(sizes, 2+rng.IntN(49))
	}
	for iter, n := range sizes {
		req := booking.Request{ResourceID: resources[0], SlotStart: base.Add(time.Duration(iter) * booking.SlotLength), CustomerName: "Ann", IdempotencyKey: fmt.Sprintf("lock-%d", iter)}
		orig, _, err := e.svc.Book(context.Background(), req)
		if err != nil {
			t.Fatalf("iter %d first: %v", iter, err)
		}
		reqs := make([]booking.Request, n)
		same := make([]bool, n)
		for i := range reqs {
			reqs[i], same[i] = req, rng.IntN(2) == 0
			if !same[i] {
				reqs[i] = otherPayload(rng, req, resources)
			}
		}
		bs, replays, errs := e.bookAllAtOnce(reqs)
		for i := range reqs {
			switch {
			case same[i] && (errs[i] != nil || !replays[i] || bs[i] != orig):
				t.Fatalf("%d replays, request %d (same payload): %+v, replayed %v, err %v", n, i, bs[i], replays[i], errs[i])
			case !same[i] && !errors.Is(errs[i], booking.ErrKeyConflict):
				t.Fatalf("%d replays, request %d (other payload): err %v, want ErrKeyConflict", n, i, errs[i])
			}
		}
		if c := e.count(t, "SELECT count(*) FROM booking WHERE idempotency_key = ?", req.IdempotencyKey); c != 1 {
			t.Fatalf("iter %d: %d bookings under the key, want 1", iter, c)
		}
	}
}

// ADR-0015/R1: for any stored booking and any repeat of its key, the repeat is classified as the
// same payload exactly when resource id, canonical slot start and the bytes of the customer name
// are all equal.
func TestProperty_Booking_PayloadComparedByteForByte(t *testing.T) {
	rng := rand.New(rand.NewPCG(9, 9))
	e := newEnv(t, fixedClock)
	ctx := context.Background()
	var resources []string
	pick := func() booking.Request {
		return booking.Request{
			ResourceID:   resources[rng.IntN(2)],
			SlotStart:    inAnyZone(rng, base.Add(time.Duration(rng.IntN(3))*booking.SlotLength)),
			CustomerName: nameVariants[rng.IntN(len(nameVariants))],
		}
	}
	sameCount := 0
	for iter := range 300 {
		resources = []string{e.newResource(t), e.newResource(t)}
		first := pick()
		first.IdempotencyKey = fmt.Sprintf("bytes-%d", iter)
		orig, _, err := e.svc.Book(ctx, first)
		if err != nil {
			t.Fatalf("iter %d first: %v", iter, err)
		}
		if rng.IntN(2) == 0 { // free the slot, so a taken slot cannot hide the classification
			if _, err := e.svc.Cancel(ctx, orig.ID); err != nil {
				t.Fatal(err)
			}
		}
		repeat := pick()
		if rng.IntN(3) == 0 { // bias toward near misses and exact repeats
			repeat.ResourceID, repeat.SlotStart = first.ResourceID, inAnyZone(rng, orig.SlotStart)
		}
		repeat.IdempotencyKey = first.IdempotencyKey
		same := repeat.ResourceID == first.ResourceID &&
			booking.CanonicalSlotStart(repeat.SlotStart).Equal(booking.CanonicalSlotStart(first.SlotStart)) &&
			repeat.CustomerName == first.CustomerName // Go compares strings byte for byte

		_, replayed, err := e.svc.Book(ctx, repeat)
		if same {
			sameCount++
			if err != nil || !replayed {
				t.Fatalf("iter %d: %+v then %+v (same payload): replayed %v, err %v", iter, first, repeat, replayed, err)
			}
		} else if !errors.Is(err, booking.ErrKeyConflict) {
			t.Fatalf("iter %d: %+v then %+v (different payload): err %v, want ErrKeyConflict", iter, first, repeat, err)
		}
	}
	if sameCount == 0 {
		t.Fatal("the generator produced no same-payload repeat; the property was not exercised")
	}
}
