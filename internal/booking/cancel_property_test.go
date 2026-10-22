package booking_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/adverax/booking-demo/internal/booking"
)

// Property tests of cancellation (FR-007, FR-008) against the real store: idempotency of a cancel
// (EC-005, ADR-0003) and finality of cancelled (INV-004, ADR-0010/R1), which the schema holds.

// bookAt books slot i of Monday (base + i half hours) under a key unique to the trial.
func (e *env) bookAt(t *testing.T, resourceID string, i int, key string) booking.Booking {
	t.Helper()
	b, _, err := e.svc.Book(context.Background(), booking.Request{
		ResourceID: resourceID, SlotStart: base.Add(time.Duration(i) * booking.SlotLength), CustomerName: "Ann", IdempotencyKey: key,
	})
	if err != nil {
		t.Fatalf("book slot %d: %v", i, err)
	}
	return b
}

func (e *env) storedStatus(t *testing.T, id string) string {
	t.Helper()
	var status string
	if err := e.db.QueryRow("SELECT status FROM booking WHERE id = ?", id).Scan(&status); err != nil {
		t.Fatalf("read status of %s: %v", id, err)
	}
	return status
}

// EC-005: cancelling a booking any number of times, one after another or all at once, leaves it
// cancelled and returns it as a success every time, identical to the first answer; nothing else in
// the database changes.
func TestProperty_Booking_RepeatedCancelIsIdempotent(t *testing.T) {
	e := newEnv(t, fixedClock)
	resourceID := e.newResource(t)
	rng := rand.New(rand.NewPCG(4, 5))
	ctx := context.Background()

	for trial := range 20 { // 20 distinct Monday slots from 09:00
		b := e.bookAt(t, resourceID, trial, fmt.Sprintf("key-%d", trial))
		want := b
		want.Status = booking.StatusCancelled
		n := 1 + rng.IntN(10)
		concurrent := rng.IntN(2) == 1

		results := make([]booking.Booking, n)
		errs := make([]error, n)
		if concurrent {
			var wg sync.WaitGroup
			for i := range n {
				wg.Go(func() { results[i], errs[i] = e.svc.Cancel(ctx, b.ID) })
			}
			wg.Wait()
		} else {
			for i := range n {
				results[i], errs[i] = e.svc.Cancel(ctx, b.ID)
			}
		}
		for i := range n {
			if errs[i] != nil || results[i] != want {
				t.Fatalf("trial %d (%d cancels, concurrent %v): cancel %d = %+v, %v; want %+v", trial, n, concurrent, i, results[i], errs[i], want)
			}
		}
		if got, err := e.svc.Get(ctx, b.ID); err != nil || got != want {
			t.Fatalf("trial %d: read back %+v, %v; want %+v", trial, got, err, want)
		}
	}
	var rows int
	if err := e.db.QueryRow("SELECT count(*) FROM booking").Scan(&rows); err != nil || rows != 20 {
		t.Fatalf("%d booking rows (err %v), want 20: a cancel must not create rows", rows, err)
	}
}

// reactivationAttempts are the ways a status update can be attempted on cancelled booking b, through
// the domain code, the store's port, or directly in the database. Each reports whether it was
// rejected. The cancelling ones are included so a sequence mixes allowed and forbidden updates.
var reactivationAttempts = []struct {
	name      string
	activates bool
	run       func(e *env, b booking.Booking) error
}{
	{"domain WithStatus(active)", true, func(_ *env, b booking.Booking) error {
		_, err := b.WithStatus(booking.StatusActive)
		return err
	}},
	{"domain WithStatus(cancelled)", false, func(_ *env, b booking.Booking) error {
		_, err := b.WithStatus(booking.StatusCancelled)
		return err
	}},
	{"store UpdateBookingStatus(active)", true, func(e *env, b booking.Booking) error {
		_, err := e.store.UpdateBookingStatus(context.Background(), b.ID, booking.StatusActive)
		if err != nil && !errors.Is(err, booking.ErrBookingCancelled) {
			return fmt.Errorf("rejected with %w, want ErrBookingCancelled", err)
		}
		return err
	}},
	{"service Cancel", false, func(e *env, b booking.Booking) error {
		_, err := e.svc.Cancel(context.Background(), b.ID)
		return err
	}},
	{"SQL UPDATE by id", true, func(e *env, b booking.Booking) error {
		_, err := e.db.Exec("UPDATE booking SET status = 'active' WHERE id = ?", b.ID)
		return err
	}},
	{"SQL UPDATE of every cancelled booking of the resource", true, func(e *env, b booking.Booking) error {
		_, err := e.db.Exec("UPDATE booking SET status = 'active' WHERE resource_id = ? AND status = 'cancelled'", b.ResourceID)
		return err
	}},
	{"SQL UPDATE of status and name together", true, func(e *env, b booking.Booking) error {
		_, err := e.db.Exec("UPDATE booking SET customer_name = 'Mallory', status = 'active' WHERE id = ?", b.ID)
		return err
	}},
	{"SQL upsert DO UPDATE", true, func(e *env, b booking.Booking) error {
		_, err := e.db.Exec(`INSERT INTO booking (id, resource_id, slot_start, customer_name, idempotency_key, status)
			VALUES (?, ?, ?, ?, ?, 'active') ON CONFLICT (id) DO UPDATE SET status = 'active'`,
			b.ID, b.ResourceID, b.SlotStart.Unix(), b.CustomerName, b.IdempotencyKey)
		return err
	}},
	{"SQL UPDATE to cancelled", false, func(e *env, b booking.Booking) error {
		_, err := e.db.Exec("UPDATE booking SET status = 'cancelled' WHERE id = ?", b.ID)
		return err
	}},
}

// EC(ADR-0010/R1), INV-004: for any cancelled booking, whether its slot is free or taken again,
// and any sequence of status updates attempted on it through the domain code, the store or
// directly in the database, every attempt to make it active is rejected and its status stays
// cancelled; the allowed updates (to cancelled) succeed.
func TestProperty_Booking_CancelledNeverBecomesActive(t *testing.T) {
	e := newEnv(t, fixedClock)
	resourceID := e.newResource(t)
	rng := rand.New(rand.NewPCG(6, 7))
	ctx := context.Background()

	for trial := range 30 { // 30 distinct Monday slots from 09:00
		b := e.bookAt(t, resourceID, trial, fmt.Sprintf("key-%d", trial))
		b, err := e.svc.Cancel(ctx, b.ID)
		if err != nil {
			t.Fatal(err)
		}
		slotTaken := rng.IntN(2) == 1
		if slotTaken {
			e.bookAt(t, resourceID, trial, fmt.Sprintf("rebook-%d", trial))
		}
		steps := 1 + rng.IntN(12)
		for step := range steps {
			a := reactivationAttempts[rng.IntN(len(reactivationAttempts))]
			err := a.run(e, b)
			if a.activates && err == nil {
				t.Fatalf("trial %d step %d: %s on a cancelled booking (slot taken %v) was accepted", trial, step, a.name, slotTaken)
			}
			if !a.activates && err != nil {
				t.Fatalf("trial %d step %d: %s failed: %v", trial, step, a.name, err)
			}
			if got := e.storedStatus(t, b.ID); got != string(booking.StatusCancelled) {
				t.Fatalf("trial %d step %d: after %s the stored status is %q, want cancelled", trial, step, a.name, got)
			}
		}
		if got := e.activeAt(t, resourceID, b.SlotStart); got != map[bool]int{false: 0, true: 1}[slotTaken] {
			t.Fatalf("trial %d: %d active bookings at the slot (slot taken %v)", trial, got, slotTaken)
		}
	}
}
