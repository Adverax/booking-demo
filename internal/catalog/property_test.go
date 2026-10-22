package catalog_test

import (
	"context"
	"errors"
	"math/rand/v2"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/adverax/booking-demo/internal/catalog"
	"github.com/adverax/booking-demo/internal/store"
)

// Property tests of INV-005 on the Resource aggregate. The generator uses a fixed seed, so every
// run checks the same cases and a failure reproduces.

// inv005 is the rule of INV-005 (ADR-0004) restated independently of the implementation: a name,
// at least one interval, at most one per weekday, each on the half hour with
// 0 <= start < end <= 24:00.
func inv005(name string, hours []catalog.Interval) bool {
	if name == "" || len(hours) == 0 {
		return false
	}
	days := map[time.Weekday]bool{}
	for _, iv := range hours {
		if iv.Weekday < time.Sunday || iv.Weekday > time.Saturday || days[iv.Weekday] {
			return false
		}
		days[iv.Weekday] = true
		if iv.Start%30 != 0 || iv.End%30 != 0 || iv.Start < 0 || iv.Start >= iv.End || iv.End > 24*60 {
			return false
		}
	}
	return true
}

// randomBound is mostly a half-hour bound in -01:00..25:00 with 24:00 and 00:00 frequent, and
// sometimes any minute in that range.
func randomBound(rng *rand.Rand) int {
	switch rng.IntN(6) {
	case 0:
		return 24 * 60
	case 1:
		return 0
	case 2:
		return rng.IntN(26*60+1) - 60
	default:
		return (rng.IntN(52) - 2) * 30
	}
}

// randomShape returns a random name and weekly-hours shape near the edges of INV-005.
func randomShape(rng *rand.Rand) (string, []catalog.Interval) {
	name := "Dr. Lee"
	if rng.IntN(10) == 0 {
		name = ""
	}
	if rng.IntN(2) == 0 {
		// Anything goes: weekdays may repeat or be out of range, bounds may be anywhere.
		hours := make([]catalog.Interval, rng.IntN(9))
		for i := range hours {
			day := time.Weekday(rng.IntN(7))
			if rng.IntN(20) == 0 {
				day = time.Weekday(rng.IntN(11) - 2)
			}
			hours[i] = catalog.Interval{Weekday: day, Start: randomBound(rng), End: randomBound(rng)}
		}
		return name, hours
	}
	// Near the valid shapes: distinct weekdays with ordered half-hour bounds inside the day, of
	// which one interval is sometimes broken by a random bound.
	days := rng.Perm(7)[:1+rng.IntN(7)]
	hours := make([]catalog.Interval, len(days))
	for i, d := range days {
		a, b := rng.IntN(49)*30, rng.IntN(49)*30
		hours[i] = catalog.Interval{Weekday: time.Weekday(d), Start: min(a, b), End: max(a, b)}
	}
	if rng.IntN(3) == 0 {
		i := rng.IntN(len(hours))
		if rng.IntN(2) == 0 {
			hours[i].Start = randomBound(rng)
		} else {
			hours[i].End = randomBound(rng)
		}
	}
	return name, hours
}

// INV-005: for any shape, Create accepts exactly when the rule holds; an accepted resource
// round-trips through the real store unchanged (Get and List), and a rejected one stores nothing.
func TestProperty_Resource_AcceptsExactlyTheShapesOfInv005AndRoundTrips(t *testing.T) {
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "booking.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	c := catalog.New(st)
	ctx := context.Background()
	rng := rand.New(rand.NewPCG(6, 2026))

	var accepted, rejected int
	for i := range 3000 {
		name, hours := randomShape(rng)
		want := inv005(name, hours)
		r, err := c.Create(ctx, name, hours)
		if want != (err == nil) {
			t.Fatalf("case %d: name %q hours %+v: Create err = %v, INV-005 holds = %v", i, name, hours, err, want)
		}
		if !want {
			if !errors.Is(err, catalog.ErrInvalidResource) {
				t.Fatalf("case %d: err = %v, want ErrInvalidResource", i, err)
			}
			rejected++
			continue
		}
		accepted++
		if r.Name != name || len(r.WeeklyHours) != len(hours) {
			t.Fatalf("case %d: created %+v from %q %+v", i, r, name, hours)
		}
		for _, iv := range hours {
			if !slices.Contains(r.WeeklyHours, iv) {
				t.Fatalf("case %d: created %+v lacks %+v", i, r, iv)
			}
		}
		got, err := c.Get(ctx, r.ID)
		if err != nil || !reflect.DeepEqual(got, r) {
			t.Fatalf("case %d: Get = %+v, %v; want %+v", i, got, err, r)
		}
	}
	list, err := c.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != accepted {
		t.Fatalf("store holds %d resources, want the %d accepted (%d rejected)", len(list), accepted, rejected)
	}
	if accepted < 300 || rejected < 300 {
		t.Fatalf("generator is lopsided: %d accepted, %d rejected", accepted, rejected)
	}
}
