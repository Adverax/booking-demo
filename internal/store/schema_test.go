package store

import (
	"database/sql"
	"fmt"
	"math/rand/v2"
	"testing"
)

func insertResource(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	if _, err := db.Exec("INSERT INTO resource (id, name) VALUES (?, ?)", id, "Resource "+id); err != nil {
		t.Fatalf("insert resource %s: %v", id, err)
	}
}

func insertBooking(db *sql.DB, id, resourceID string, slotStart int64, name, key, status string) error {
	_, err := db.Exec(
		`INSERT INTO booking (id, resource_id, slot_start, customer_name, idempotency_key, status)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, resourceID, slotStart, name, key, status)
	return err
}

func insertHours(db *sql.DB, resourceID string, weekday, start, end int) error {
	_, err := db.Exec(
		"INSERT INTO resource_working_hours (resource_id, weekday, start_minute, end_minute) VALUES (?, ?, ?, ?)",
		resourceID, weekday, start, end)
	return err
}

const slot = int64(1_800_000_000) // a half-hour aligned instant

func TestSchema_RejectsSecondActiveBookingOfTheSameSlot(t *testing.T) {
	s := openTestStore(t)
	insertResource(t, s.db, "r1")
	if err := insertBooking(s.db, "b1", "r1", slot, "Ann", "k1", "active"); err != nil {
		t.Fatal(err)
	}
	err := insertBooking(s.db, "b2", "r1", slot, "Bob", "k2", "active")
	if !IsUniqueViolation(err) {
		t.Fatalf("second active booking of one slot: err = %v, want a unique violation", err)
	}
}

func TestSchema_SameStartOnAnotherResourceIsAnotherSlot(t *testing.T) {
	s := openTestStore(t)
	insertResource(t, s.db, "r1")
	insertResource(t, s.db, "r2")
	if err := insertBooking(s.db, "b1", "r1", slot, "Ann", "k1", "active"); err != nil {
		t.Fatal(err)
	}
	if err := insertBooking(s.db, "b2", "r2", slot, "Ann", "k2", "active"); err != nil {
		t.Fatalf("same start on another resource rejected: %v", err)
	}
}

func TestSchema_CancelledBookingFreesItsSlot(t *testing.T) {
	s := openTestStore(t)
	insertResource(t, s.db, "r1")
	if err := insertBooking(s.db, "b1", "r1", slot, "Ann", "k1", "active"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE booking SET status = 'cancelled' WHERE id = 'b1'"); err != nil {
		t.Fatal(err)
	}
	if err := insertBooking(s.db, "b2", "r1", slot, "Bob", "k2", "active"); err != nil {
		t.Fatalf("slot of a cancelled booking not free: %v", err)
	}
}

func TestSchema_IdempotencyKeyIsUniqueAcrossAllBookings(t *testing.T) {
	s := openTestStore(t)
	insertResource(t, s.db, "r1")
	insertResource(t, s.db, "r2")
	if err := insertBooking(s.db, "b1", "r1", slot, "Ann", "k1", "cancelled"); err != nil {
		t.Fatal(err)
	}
	err := insertBooking(s.db, "b2", "r2", slot+3600, "Bob", "k1", "active")
	if !IsUniqueViolation(err) {
		t.Fatalf("key of a cancelled booking on another resource reused: err = %v, want a unique violation", err)
	}
}

func TestSchema_KeysAndNamesCompareByteForByte(t *testing.T) {
	s := openTestStore(t)
	insertResource(t, s.db, "r1")
	keys := []string{"key", "KEY", "key ", " key", "kéy", "kéy"}
	for i, k := range keys {
		if err := insertBooking(s.db, "b"+k, "r1", slot+int64(i)*1800, "Ann", k, "active"); err != nil {
			t.Fatalf("key %q treated as equal to an earlier key: %v", k, err)
		}
	}
	var n int
	if err := s.db.QueryRow("SELECT count(*) FROM booking WHERE customer_name = 'ann'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("customer names compare case-insensitively: %d matches (err %v)", n, err)
	}
}

func TestSchema_RejectsBookingOfUnknownResource(t *testing.T) {
	s := openTestStore(t)
	if err := insertBooking(s.db, "b1", "missing", slot, "Ann", "k1", "active"); err == nil {
		t.Fatal("booking of an unknown resource accepted; foreign keys are not enforced")
	}
}

func TestSchema_RejectsUnknownStatus(t *testing.T) {
	s := openTestStore(t)
	insertResource(t, s.db, "r1")
	if err := insertBooking(s.db, "b1", "r1", slot, "Ann", "k1", "pending"); err == nil {
		t.Fatal("status 'pending' accepted")
	}
}

func TestSchema_RejectsNonIntegerSlotStart(t *testing.T) {
	s := openTestStore(t)
	insertResource(t, s.db, "r1")
	for _, v := range []any{"2027-01-15T08:00:00Z", "2027-01-15 08:00:00", 1.5e9 + 0.5} {
		if _, err := s.db.Exec(
			`INSERT INTO booking (id, resource_id, slot_start, customer_name, idempotency_key, status)
			 VALUES ('b', 'r1', ?, 'Ann', 'k', 'active')`, v); err == nil {
			t.Errorf("slot_start %#v accepted; only integer epoch seconds may be stored", v)
		}
	}
}

func TestSchema_WorkingHoursHaveTheADR0004Shape(t *testing.T) {
	s := openTestStore(t)
	insertResource(t, s.db, "r1")
	for wd := 1; wd <= 5; wd++ { // Mon-Fri 09:00-17:00
		if err := insertHours(s.db, "r1", wd, 9*60, 17*60); err != nil {
			t.Fatalf("weekday %d: %v", wd, err)
		}
	}
	if err := insertHours(s.db, "r1", 0, 0, 1440); err != nil {
		t.Fatalf("whole Sunday rejected: %v", err)
	}
	rejected := map[string][3]int{
		"second interval on one weekday": {1, 18 * 60, 19 * 60},
		"misaligned start":               {6, 9*60 + 15, 17 * 60},
		"misaligned end":                 {6, 9 * 60, 17*60 + 1},
		"crosses midnight":               {6, 22 * 60, 2 * 60},
		"ends after midnight":            {6, 22 * 60, 1470},
		"empty interval":                 {6, 9 * 60, 9 * 60},
		"weekday out of range":           {7, 9 * 60, 17 * 60},
	}
	for name, h := range rejected {
		if err := insertHours(s.db, "r1", h[0], h[1], h[2]); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := insertHours(s.db, "missing", 6, 9*60, 17*60); err == nil {
		t.Error("working hours of an unknown resource accepted")
	}
}

// TestPropertySchema_ActiveSlotAndKeyStayUnique drives random sequences of
// inserts and cancellations against a reference model of the two uniqueness
// guarantees of ADR-0008: an insert succeeds exactly when no active booking
// holds its (resource, slot start) and no booking at all holds its key, and
// every rejection is reported as a unique violation (INV-001, INV-003).
func TestPropertySchema_ActiveSlotAndKeyStayUnique(t *testing.T) {
	seed := rand.Uint64()
	t.Logf("seed %d", seed)
	rng := rand.New(rand.NewPCG(seed, seed))

	for run := range 20 {
		s := openTestStore(t)
		resources := []string{"r1", "r2"}
		for _, r := range resources {
			insertResource(t, s.db, r)
		}
		type slotKey struct {
			resource string
			start    int64
		}
		activeSlot := map[slotKey]string{} // slot -> booking id
		usedKeys := map[string]bool{}
		var active []string
		bookingSlot := map[string]slotKey{}

		for step := range 40 {
			if len(active) > 0 && rng.IntN(4) == 0 {
				i := rng.IntN(len(active))
				id := active[i]
				if _, err := s.db.Exec("UPDATE booking SET status = 'cancelled' WHERE id = ?", id); err != nil {
					t.Fatalf("run %d step %d: cancel %s: %v", run, step, id, err)
				}
				delete(activeSlot, bookingSlot[id])
				active = append(active[:i], active[i+1:]...)
				continue
			}
			sk := slotKey{resources[rng.IntN(2)], slot + int64(rng.IntN(3))*1800}
			key := string(rune('a' + rng.IntN(6)))
			id := fmt.Sprintf("b%d", step)
			_, slotTaken := activeSlot[sk]
			wantOK := !slotTaken && !usedKeys[key]

			err := insertBooking(s.db, id, sk.resource, sk.start, "Ann", key, "active")
			if wantOK != (err == nil) {
				t.Fatalf("run %d step %d: insert %v key %q: err = %v, model expected success = %v", run, step, sk, key, err, wantOK)
			}
			if err != nil {
				if !IsUniqueViolation(err) {
					t.Fatalf("run %d step %d: rejection is not a unique violation: %v", run, step, err)
				}
				continue
			}
			activeSlot[sk] = id
			usedKeys[key] = true
			active = append(active, id)
			bookingSlot[id] = sk
		}

		var dupSlots, dupKeys int
		if err := s.db.QueryRow(`SELECT count(*) FROM (SELECT 1 FROM booking WHERE status = 'active'
			GROUP BY resource_id, slot_start HAVING count(*) > 1)`).Scan(&dupSlots); err != nil {
			t.Fatal(err)
		}
		if err := s.db.QueryRow(`SELECT count(*) FROM (SELECT 1 FROM booking
			GROUP BY idempotency_key HAVING count(*) > 1)`).Scan(&dupKeys); err != nil {
			t.Fatal(err)
		}
		if dupSlots != 0 || dupKeys != 0 {
			t.Fatalf("run %d: %d doubly booked slots, %d reused keys", run, dupSlots, dupKeys)
		}
	}
}
