package availability_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adverax/booking-demo/internal/availability"
	"github.com/adverax/booking-demo/internal/catalog"
)

// Property tests of Availability Search. Generators are seeded and bounded.

// randomResource draws weekly hours satisfying INV-005: each weekday open with probability 2/3,
// with half-hour aligned bounds 0 <= Start < End <= 24:00.
func randomResource(t *testing.T, rng *rand.Rand) catalog.Resource {
	t.Helper()
	var hours []catalog.Interval
	for d := time.Sunday; d <= time.Saturday; d++ {
		if rng.IntN(3) == 0 {
			continue
		}
		start := rng.IntN(48) * 30
		end := start + (1+rng.IntN((catalog.EndOfDay-start)/30))*30
		hours = append(hours, catalog.Interval{Weekday: d, Start: start, End: end})
	}
	if hours == nil {
		hours = []catalog.Interval{{Weekday: time.Monday, Start: 0, End: catalog.EndOfDay}}
	}
	return mustResource(t, "r1", hours...)
}

// inAnyZone returns t in a random fixed zone at a random time of its UTC day; its UTC day is
// unchanged.
func inAnyZone(rng *rand.Rand, utcMidnight time.Time) time.Time {
	t := utcMidnight.Add(time.Duration(rng.Int64N(int64(24 * time.Hour))))
	return t.In(time.FixedZone("z", (rng.IntN(27)-12)*3600+rng.IntN(4)*900))
}

// oracleFree is the reference of FR-003 written with the time package: every UTC half hour of the
// range whose slot lies inside the working interval of its weekday and whose start is not booked.
func oracleFree(r catalog.Resource, first time.Time, days int, booked map[int64]bool) []availability.Slot {
	out := []availability.Slot{}
	for t := first; t.Before(first.AddDate(0, 0, days)); t = t.Add(30 * time.Minute) {
		m := t.Hour()*60 + t.Minute()
		for _, iv := range r.WeeklyHours {
			if iv.Weekday == t.Weekday() && iv.Start <= m && m+30 <= iv.End && !booked[t.Unix()] {
				out = append(out, availability.Slot{Start: t, End: t.Add(30 * time.Minute)})
			}
		}
	}
	return out
}

// FR-003, FR-004, INV-001's read side: for any weekly hours, any range of 1..14 UTC days between
// 1975 and 2095, and any set of active bookings, the result is exactly the half-hour slots inside
// working hours that are not booked, ordered by start, each 30 minutes long and in UTC.
func TestProperty_Availability_ListsExactlyFreeSlotsInsideWorkingHours(t *testing.T) {
	rng := rand.New(rand.NewPCG(8, 8))
	for iter := range 300 {
		r := randomResource(t, rng)
		first := time.Unix((157_766_400+rng.Int64N(3_786_912_000))/86400*86400, 0).UTC()
		days := 1 + rng.IntN(availability.MaxRangeDays)
		booked := map[int64]bool{}
		var bookedStarts []time.Time
		// Bookings around and inside the range, some outside working hours or the range.
		for range rng.IntN(40) {
			s := first.Add(time.Duration(rng.IntN((days+2)*48)-48) * 30 * time.Minute)
			if !booked[s.Unix()] {
				booked[s.Unix()] = true
				bookedStarts = append(bookedStarts, s)
			}
		}
		svc := availability.NewService(fakeResources{r.ID: r}, &fakeBookings{starts: map[string][]time.Time{r.ID: bookedStarts}})
		got, err := svc.FreeSlots(context.Background(), r.ID, inAnyZone(rng, first), inAnyZone(rng, first.AddDate(0, 0, days-1)))
		if err != nil {
			t.Fatalf("iter %d: %v", iter, err)
		}
		if want := oracleFree(r, first, days, booked); !slices.Equal(got, want) {
			t.Fatalf("iter %d: hours %v, %d days from %v, booked %v:\n got  %v\n want %v", iter, r.WeeklyHours, days, first, bookedStarts, got, want)
		}
	}
}

// FR-004, ADR-0018: for any pair of days, the range is accepted exactly when from is not after to
// and it covers at most 14 days counting both ends; a rejected range reads no booking.
func TestProperty_Availability_RangeRule(t *testing.T) {
	rng := rand.New(rand.NewPCG(4, 4))
	r := randomResource(t, rng)
	for iter := range 500 {
		from := time.Unix((157_766_400+rng.Int64N(3_786_912_000))/86400*86400, 0).UTC()
		offset := rng.IntN(41) - 20
		to := from.AddDate(0, 0, offset)
		b := &fakeBookings{}
		svc := availability.NewService(fakeResources{r.ID: r}, b)
		_, err := svc.FreeSlots(context.Background(), r.ID, inAnyZone(rng, from), inAnyZone(rng, to))
		wantOK := offset >= 0 && offset+1 <= availability.MaxRangeDays
		if (err == nil) != wantOK {
			t.Fatalf("iter %d: from %v to %v (offset %d): err %v, want ok %v", iter, from, to, offset, err, wantOK)
		}
		if !wantOK && b.reads.Load() != 0 {
			t.Fatalf("iter %d: a rejected range read bookings", iter)
		}
	}
}

// EC(ADR-0013/R1): for any range and any clock value, availability computes slots from integer UTC
// epoch seconds and reads the current time only from an injected clock. The component takes no
// clock because no rule of FR-003/FR-004 depends on now (past slots are listed, ASM-0002); so the
// property is that its answer is a function of its inputs alone:
//   - every slot bound is a whole UTC epoch second on the half hour, in UTC;
//   - ranges drawn decades before and after the real time, with bounds in any zone, give equal
//     answers on repeated queries, and slots before the real now are listed (a hidden reading
//     of the system clock, such as a past filter, would show);
//   - no production file of the package calls time.Now, time.Since or time.Until.
func TestProperty_Availability_ReadsTimeOnlyThroughInjectedClock(t *testing.T) {
	rng := rand.New(rand.NewPCG(13, 13))
	pastSeen := 0
	for iter := range 300 {
		r := randomResource(t, rng)
		first := time.Unix((157_766_400+rng.Int64N(3_786_912_000))/86400*86400, 0).UTC()
		days := 1 + rng.IntN(availability.MaxRangeDays)
		svc := availability.NewService(fakeResources{r.ID: r}, &fakeBookings{})
		last := first.AddDate(0, 0, days-1)
		a, err := svc.FreeSlots(context.Background(), r.ID, inAnyZone(rng, first), inAnyZone(rng, last))
		if err != nil {
			t.Fatalf("iter %d: %v", iter, err)
		}
		b, err := svc.FreeSlots(context.Background(), r.ID, inAnyZone(rng, first), inAnyZone(rng, last))
		if err != nil || !slices.Equal(a, b) {
			t.Fatalf("iter %d: repeated query differs: %v / %v (err %v)", iter, a, b, err)
		}
		for _, s := range a {
			if s.Start.Location() != time.UTC || s.Start.Nanosecond() != 0 || s.Start.Unix()%1800 != 0 ||
				s.End.Unix()-s.Start.Unix() != 1800 || s.End.Nanosecond() != 0 {
				t.Fatalf("iter %d: slot %+v is not whole UTC epoch seconds on the half hour", iter, s)
			}
		}
		if len(a) > 0 && a[0].Start.Unix() < 1_700_000_000 {
			pastSeen++
		}
	}
	if pastSeen == 0 {
		t.Fatal("generator drew no range in the past")
	}
	assertNoSystemClock(t)
}

func assertNoSystemClock(t *testing.T) {
	t.Helper()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "time" && (sel.Sel.Name == "Now" || sel.Sel.Name == "Since" || sel.Sel.Name == "Until") {
					t.Errorf("%s: availability reads the system clock (time.%s); use an injected clock", fset.Position(sel.Pos()), sel.Sel.Name)
				}
			}
			return true
		})
	}
	if checked == 0 {
		t.Fatal("no production file checked")
	}
}
