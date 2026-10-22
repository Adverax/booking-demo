package booking_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adverax/booking-demo/internal/booking"
	"github.com/adverax/booking-demo/internal/catalog"
)

// Property tests of booking validation (FR-005, INV-002) against the real store. Seeds are fixed, so
// a failure reproduces.

// EC(ADR-0013/R1): for any slot and any injected clock value, the past-slot decision reads the
// current time only from the injected clock and compares integer UTC epoch seconds: a request is
// rejected as in the past exactly when the slot start's epoch second is earlier than now's, and
// then nothing is stored under its key. Clock values are drawn around the slot start (one second
// either side, the same second with a sub-second part, and up to a day away) and in any zone, and
// lie decades from the real time, so a decision taken from the system clock would show.
func TestProperty_Booking_PastCheckUsesInjectedClock(t *testing.T) {
	rng := rand.New(rand.NewPCG(10, 10))
	var now atomic.Pointer[time.Time]
	var reads atomic.Int64
	e := newEnv(t, func() time.Time { reads.Add(1); return *now.Load() })
	resourceID := e.newResource(t)
	ctx := context.Background()
	pastSeen, acceptedSeen := 0, 0
	// All draws share one resource, so a slot is drawn at most once: a repeated slot would be
	// rejected as taken, a failure unrelated to the clock.
	drawn := make(map[int64]bool)
	for iter := range 400 {
		// 1975 .. 2095, on the half hour; far from the real clock in most draws.
		var slot time.Time
		for {
			slot = time.Unix((157_766_400+rng.Int64N(3_786_912_000))/1800*1800, 0)
			if !drawn[slot.Unix()] {
				break
			}
		}
		drawn[slot.Unix()] = true
		var offset time.Duration
		switch rng.IntN(4) {
		case 0:
			offset = time.Second * time.Duration(rng.IntN(3)-1) // -1 s, 0, +1 s
		case 1:
			offset = time.Duration(rng.Int64N(int64(time.Second))) // the same second, sub-second part
		default:
			offset = time.Duration(rng.Int64N(int64(48*time.Hour))) - 24*time.Hour
		}
		n := slot.Add(offset).In(time.FixedZone("z", (rng.IntN(27)-12)*3600+rng.IntN(4)*900))
		now.Store(&n)
		key := fmt.Sprintf("past-%d", iter)
		before := reads.Load()

		b, replayed, err := e.svc.Book(ctx, booking.Request{ResourceID: resourceID, SlotStart: inAnyZone(rng, slot), CustomerName: "Ann", IdempotencyKey: key})
		if reads.Load() == before {
			t.Fatalf("iter %d: the injected clock was not read", iter)
		}
		wantPast := slot.Unix() < n.Unix()
		stored := e.count(t, "SELECT count(*) FROM booking WHERE idempotency_key = ?", key)
		if wantPast {
			pastSeen++
			if !errors.Is(err, booking.ErrSlotInPast) || replayed || stored != 0 {
				t.Fatalf("iter %d: slot %v, now %v: err %v, replayed %v, %d stored; want ErrSlotInPast and nothing stored", iter, slot.UTC(), n, err, replayed, stored)
			}
			continue
		}
		acceptedSeen++
		if err != nil || replayed || stored != 1 || !b.SlotStart.Equal(slot) {
			t.Fatalf("iter %d: slot %v, now %v: %+v, replayed %v, err %v, %d stored; want a new booking", iter, slot.UTC(), n, b, replayed, err, stored)
		}
	}
	if pastSeen == 0 || acceptedSeen == 0 {
		t.Fatalf("generator covered one side only: %d past, %d accepted", pastSeen, acceptedSeen)
	}
	assertNoSystemClock(t)
}

// INV-002: for any weekly working hours and any slot of a week, a request with a new key is
// accepted exactly when the slot lies inside the working interval of its UTC day and does not start
// before now; outside hours is reported before past, and a rejection stores nothing. The expected
// outcome is restated independently, in epoch seconds of the slot and of the interval's bounds.
func TestProperty_Booking_AcceptedOnlyInsideWorkingHoursAndNotInPast(t *testing.T) {
	rng := rand.New(rand.NewPCG(11, 11))
	var now time.Time
	e := newEnv(t, func() time.Time { return now })
	ctx := context.Background()
	week := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC) // a Sunday
	outcomes := map[string]int{}
	for iter := range 300 {
		var hours []catalog.Interval
		for d := time.Sunday; d <= time.Saturday; d++ {
			if rng.IntN(3) == 0 {
				continue // closed
			}
			start := rng.IntN(48)
			hours = append(hours, catalog.Interval{Weekday: d, Start: start * 30, End: (start + 1 + rng.IntN(48-start)) * 30})
		}
		if len(hours) == 0 {
			hours = append(hours, catalog.Interval{Weekday: time.Monday, Start: 0, End: catalog.EndOfDay})
		}
		r, err := e.catalog.Create(ctx, "R", hours)
		if err != nil {
			t.Fatalf("iter %d: %v", iter, err)
		}
		slot := week.Add(time.Duration(rng.IntN(7*48)) * booking.SlotLength)
		now = slot.Add(time.Duration(rng.IntN(5)-2) * booking.SlotLength)

		inside := false
		for _, iv := range hours {
			day := week.Add(time.Duration(iv.Weekday) * 24 * time.Hour).Unix()
			if slot.Unix() >= day+int64(iv.Start)*60 && slot.Unix()+int64(booking.SlotLength/time.Second) <= day+int64(iv.End)*60 {
				inside = true
			}
		}
		key := fmt.Sprintf("hours-%d", iter)
		_, _, err = e.svc.Book(ctx, booking.Request{ResourceID: r.ID, SlotStart: inAnyZone(rng, slot), CustomerName: "Ann", IdempotencyKey: key})
		var want error
		switch {
		case !inside:
			want = booking.ErrOutsideWorkingHours
		case slot.Before(now):
			want = booking.ErrSlotInPast
		}
		if !errors.Is(err, want) || (want == nil && err != nil) {
			t.Fatalf("iter %d: hours %+v, slot %v, now %v: err %v, want %v", iter, hours, slot, now, err, want)
		}
		stored := e.count(t, "SELECT count(*) FROM booking WHERE idempotency_key = ?", key)
		if (want == nil) != (stored == 1) || stored > 1 {
			t.Fatalf("iter %d: %d stored under the key for outcome %v", iter, stored, want)
		}
		outcomes[fmt.Sprint(want)]++
	}
	if len(outcomes) != 3 {
		t.Fatalf("generator did not reach every outcome: %v", outcomes)
	}
}
