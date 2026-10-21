package store

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"
)

// randomInstant returns an instant between roughly year 1800 and year 5100,
// with a random sub-second part, in a random fixed UTC offset.
func randomInstant(rng *rand.Rand) time.Time {
	sec := rng.Int64N(2e11) - 5e9
	nsec := rng.Int64N(1e9)
	return time.Unix(sec, nsec).In(randomZone(rng))
}

func randomZone(rng *rand.Rand) *time.Location {
	offset := (rng.IntN(28*60+1) - 14*60) * 60 // -14:00 .. +14:00, minute steps
	return time.FixedZone(fmt.Sprintf("Z%+d", offset), offset)
}

// TestPropertyTest_Store_InstantsRoundTripAsEpochSeconds is the property test
// of EC(ADR-0013/R1): for any instant, in any location, the store writes an
// integer number of UTC epoch seconds and reads back that same instant in UTC,
// and one instant has exactly one stored key in the active-slot index.
func TestPropertyTest_Store_InstantsRoundTripAsEpochSeconds(t *testing.T) {
	seed := rand.Uint64()
	t.Logf("seed %d", seed)
	rng := rand.New(rand.NewPCG(seed, ^seed))

	edges := []time.Time{
		time.Unix(0, 0),
		time.Unix(-1, 999_999_999),
		time.Unix(-1, 1),
		time.Unix(1, 0).Add(-time.Nanosecond),
		time.Date(2027, 1, 15, 8, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 15, 9, 0, 0, 0, time.FixedZone("+01:00", 3600)), // same instant as above
		time.Date(9999, 12, 31, 23, 30, 0, 0, time.UTC),
	}

	t.Run("conversion", func(t *testing.T) {
		cases := append([]time.Time{}, edges...)
		for range 10_000 {
			cases = append(cases, randomInstant(rng))
		}
		for _, in := range cases {
			sec := toEpoch(in)
			want := in.Truncate(time.Second) // independent oracle: floor to the second
			if sec != toEpoch(in.UTC()) || sec != toEpoch(in.In(randomZone(rng))) {
				t.Fatalf("%v: epoch depends on the location", in)
			}
			out := fromEpoch(sec)
			if out.Location() != time.UTC {
				t.Fatalf("%v: read back in %v, want UTC", in, out.Location())
			}
			if !out.Equal(want) {
				t.Fatalf("%v: round trip gave %v, want %v", in, out, want)
			}
			if out.Nanosecond() != 0 {
				t.Fatalf("%v: read back with a sub-second part", in)
			}
			if toEpoch(out) != sec {
				t.Fatalf("%v: second round trip changed the stored value", in)
			}
		}
	})

	t.Run("through the database", func(t *testing.T) {
		s := openTestStore(t)
		insertResource(t, s.db, "r1")

		cases := append([]time.Time{}, edges...)
		for range 200 {
			cases = append(cases, randomInstant(rng))
		}
		seen := map[int64]bool{}
		for i, in := range cases {
			sec := toEpoch(in)
			id := fmt.Sprintf("b%d", i)
			err := insertBooking(s.db, id, "r1", sec, "Ann", id, "active")
			if seen[sec] {
				// The same instant already holds an active booking, whatever
				// its location: exactly one key per instant.
				if !IsUniqueViolation(err) {
					t.Fatalf("%v: same instant stored twice under different keys (err %v)", in, err)
				}
				continue
			}
			if err != nil {
				t.Fatalf("%v: insert: %v", in, err)
			}
			seen[sec] = true

			var typ string
			var stored int64
			if err := s.db.QueryRow("SELECT typeof(slot_start), slot_start FROM booking WHERE id = ?", id).
				Scan(&typ, &stored); err != nil {
				t.Fatal(err)
			}
			if typ != "integer" || stored != sec {
				t.Fatalf("%v: stored %s %d, want integer %d", in, typ, stored, sec)
			}
			if out := fromEpoch(stored); !out.Equal(in.Truncate(time.Second)) || out.Location() != time.UTC {
				t.Fatalf("%v: read back %v", in, out)
			}

			// The same instant spelled in another location is the same key.
			again := in.In(randomZone(rng))
			err = insertBooking(s.db, id+"-again", "r1", toEpoch(again), "Ann", id+"-again", "active")
			if !IsUniqueViolation(err) {
				t.Fatalf("%v as %v: a second index key for one instant (err %v)", in, again, err)
			}
		}
		if !seen[toEpoch(edges[4])] || len(seen) == len(cases) {
			t.Fatal("edge cases did not exercise the duplicate-instant path")
		}
	})
}
