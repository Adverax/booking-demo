package booking_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adverax/booking-demo/internal/booking"
	"github.com/adverax/booking-demo/internal/catalog"
	"github.com/adverax/booking-demo/internal/store"
)

// Property tests of the Booking aggregate's invariants against the real store on a SQLite file:
// the invariants are held by the store's unique indexes (ADR-0008), so a fake would prove nothing.
// Generators use fixed seeds, so every run checks the same cases and a failure reproduces.

type env struct {
	svc     *booking.Service
	catalog *catalog.Catalog
	db      *sql.DB
	store   *store.Store
}

func newEnv(t *testing.T, now func() time.Time) *env {
	t.Helper()
	path := filepath.Join(t.TempDir(), "booking.db")
	st, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cat := catalog.New(st)
	return &env{svc: booking.NewService(st, cat, now), catalog: cat, db: db, store: st}
}

var fixedClock = func() time.Time { return time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC) }

// base is a future canonical slot start relative to fixedClock: Monday 2026-11-02 09:00 UTC.
var base = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)

// newResource creates a resource open around the clock on every day, so working hours reject no
// slot of the generators below; the working-hours rule has its own tests.
func (e *env) newResource(t *testing.T) string {
	t.Helper()
	var hours []catalog.Interval
	for d := time.Sunday; d <= time.Saturday; d++ {
		hours = append(hours, catalog.Interval{Weekday: d, Start: 0, End: catalog.EndOfDay})
	}
	r, err := e.catalog.Create(context.Background(), "Dr. Lee", hours)
	if err != nil {
		t.Fatal(err)
	}
	return r.ID
}

func (e *env) activeAt(t *testing.T, resourceID string, start time.Time) int {
	t.Helper()
	var n int
	if err := e.db.QueryRow("SELECT count(*) FROM booking WHERE resource_id = ? AND slot_start = ? AND status = 'active'",
		resourceID, start.Unix()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// bookConcurrently releases all requests at once and returns each one's error.
func (e *env) bookConcurrently(reqs []booking.Request) []error {
	errs := make([]error, len(reqs))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, req := range reqs {
		wg.Go(func() {
			<-start
			_, _, errs[i] = e.svc.Book(context.Background(), req)
		})
	}
	close(start)
	wg.Wait()
	return errs
}

// inAnyZone is the slot start slot spelled in a random zone, whole and half-hour offsets alike: a
// non-canonical spelling of the same instant. Since CARD-007 a start must lie on the UTC half hour
// (AC-017), so the instant is kept and only its spelling varies.
func inAnyZone(rng *rand.Rand, slot time.Time) time.Time {
	return slot.In(time.FixedZone("z", (rng.IntN(27)-12)*3600+rng.IntN(4)*900))
}

// offHalfHour is an instant inside the slot starting at slot but not its start: at least one
// nanosecond and less than a half hour after it. Booking it is rejected (AC-017).
func offHalfHour(rng *rand.Rand, slot time.Time) time.Time {
	return slot.Add(1 + time.Duration(rng.Int64N(int64(booking.SlotLength)-1)))
}

// EC-001 / INV-001: for any set of concurrent booking requests over some slots of one resource, at
// most one active booking exists per slot afterwards, and every slot that was asked for by a valid
// request has one. Validation runs in front of the insert (CARD-007): requests with a start off the
// half hour are mixed in, are rejected as invalid, and neither win nor block a slot.
func TestProperty_Booking_NoOverlapUnderConcurrency(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 1))
	e := newEnv(t, fixedClock)
	for iter := range 25 {
		resourceID := e.newResource(t)
		slots := make([]time.Time, 1+rng.IntN(4))
		for i := range slots {
			slots[i] = base.Add(time.Duration(i) * booking.SlotLength)
		}
		reqs := make([]booking.Request, 2+rng.IntN(39))
		valid := make([]bool, len(reqs))
		for i := range reqs {
			slot := slots[rng.IntN(len(slots))]
			start := inAnyZone(rng, slot)
			if valid[i] = rng.IntN(5) != 0; !valid[i] {
				start = offHalfHour(rng, slot)
			}
			reqs[i] = booking.Request{
				ResourceID:     resourceID,
				SlotStart:      start,
				CustomerName:   fmt.Sprintf("c%d", i),
				IdempotencyKey: fmt.Sprintf("it%d-k%d", iter, i),
			}
		}
		errs := e.bookConcurrently(reqs)

		asked := map[time.Time]bool{}
		for i, r := range reqs {
			if valid[i] {
				asked[booking.CanonicalSlotStart(r.SlotStart)] = true
			}
		}
		wins := 0
		for i, err := range errs {
			switch {
			case !valid[i]:
				if !errors.Is(err, booking.ErrInvalidRequest) {
					t.Fatalf("iter %d request %d (off the half hour): err %v, want ErrInvalidRequest", iter, i, err)
				}
			case err == nil:
				wins++
			case !errors.Is(err, booking.ErrSlotTaken):
				t.Fatalf("iter %d request %d: %v", iter, i, err)
			}
		}
		if wins != len(asked) {
			t.Fatalf("iter %d: %d successes for %d distinct slots", iter, wins, len(asked))
		}
		for _, s := range slots {
			n := e.activeAt(t, resourceID, s)
			if n > 1 || (asked[s] && n != 1) {
				t.Fatalf("iter %d: slot %v has %d active bookings (asked: %v)", iter, s, n, asked[s])
			}
		}
	}
}

// ADR-0012/R1: for any number of concurrent writers on one slot, every request ends as a success
// or a slot conflict, never as a lock or busy error.
func TestProperty_Booking_ConcurrentWritersNeverSeeLockErrors(t *testing.T) {
	rng := rand.New(rand.NewPCG(2, 2))
	e := newEnv(t, fixedClock)
	sizes := []int{2, 50}
	for range 10 {
		sizes = append(sizes, 2+rng.IntN(49))
	}
	for iter, n := range sizes {
		resourceID := e.newResource(t)
		reqs := make([]booking.Request, n)
		for i := range reqs {
			reqs[i] = booking.Request{ResourceID: resourceID, SlotStart: base, CustomerName: "c", IdempotencyKey: fmt.Sprintf("w%d-%d", iter, i)}
		}
		wins := 0
		for i, err := range e.bookConcurrently(reqs) {
			switch {
			case err == nil:
				wins++
			case errors.Is(err, booking.ErrSlotTaken):
			default:
				t.Fatalf("%d writers, request %d: %v (want success or slot taken)", n, i, err)
			}
		}
		if wins != 1 {
			t.Fatalf("%d writers: %d successes, want 1", n, wins)
		}
	}
}

// ADR-0013/R1: for any accepted slot start, in any zone's spelling, and any injected clock value not
// after it, the stored slot start is the canonical UTC half-hour start as integer epoch seconds; and
// the booking logic has no other source of the current time than the injected clock. (Since
// CARD-007 a start off the half hour or before now is rejected, so the generator draws an aligned
// start and a clock value at or before it.)
func TestProperty_Booking_SlotStartStoredAsCanonicalEpochSeconds(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 3))
	var now time.Time // the injected clock's value, redrawn per case
	e := newEnv(t, func() time.Time { return now })
	for iter := range 200 {
		slot := rng.Int64N(4_102_444_800/1800) * 1800 // 1970 .. 2100, on the half hour
		now = time.Unix(rng.Int64N(slot+1), rng.Int64N(1e9))
		resourceID := e.newResource(t)
		requested := time.Unix(slot, 0).In(time.FixedZone("z", (rng.IntN(27)-12)*3600+rng.IntN(4)*900))

		b, _, err := e.svc.Book(context.Background(), booking.Request{ResourceID: resourceID, SlotStart: requested, CustomerName: "c", IdempotencyKey: fmt.Sprintf("k%d", iter)})
		if err != nil {
			t.Fatalf("iter %d: %v", iter, err)
		}
		var stored int64
		var typ string
		if err := e.db.QueryRow("SELECT slot_start, typeof(slot_start) FROM booking WHERE id = ?", b.ID).Scan(&stored, &typ); err != nil {
			t.Fatal(err)
		}
		sec := requested.Unix()
		want := sec - ((sec%1800)+1800)%1800
		if typ != "integer" || stored != want || stored%1800 != 0 {
			t.Fatalf("iter %d: requested %v, stored %v (%s), want %d", iter, requested, stored, typ, want)
		}
		if b.SlotStart.Unix() != want || b.SlotStart.Location() != time.UTC || b.SlotStart.Nanosecond() != 0 {
			t.Fatalf("iter %d: returned slot start %v, want %v UTC", iter, b.SlotStart, time.Unix(want, 0).UTC())
		}
	}

	assertNoSystemClock(t)
}

// assertNoSystemClock fails when a production file of this package reads the system clock: the only
// clock is the injected one (ADR-0013/R1).
func assertNoSystemClock(t *testing.T) {
	t.Helper()
	for _, f := range productionFiles(t) {
		ast.Inspect(f.file, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "time" && (sel.Sel.Name == "Now" || sel.Sel.Name == "Since" || sel.Sel.Name == "Until") {
					t.Errorf("%s: booking logic reads the system clock (time.%s); use the injected clock", f.fset.Position(sel.Pos()), sel.Sel.Name)
				}
			}
			return true
		})
	}
}

type parsedFile struct {
	fset *token.FileSet
	file *ast.File
}

func productionFiles(t *testing.T) []parsedFile {
	t.Helper()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var files []parsedFile
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, parsedFile{fset, f})
	}
	if len(files) == 0 {
		t.Fatal("no production files found")
	}
	return files
}

// ADR-0015/R1: for any pair of booking requests, they compete for the same slot exactly when their
// resource ids and canonical slot starts are equal.
func TestProperty_Booking_SlotIdentityIsResourceAndCanonicalStart(t *testing.T) {
	rng := rand.New(rand.NewPCG(4, 4))
	e := newEnv(t, fixedClock)
	for iter := range 300 {
		resources := []string{e.newResource(t), e.newResource(t)}
		pick := func() (string, time.Time) {
			slot := base.Add(time.Duration(rng.IntN(3)) * booking.SlotLength)
			return resources[rng.IntN(2)], inAnyZone(rng, slot)
		}
		r1, s1 := pick()
		r2, s2 := pick()
		if _, _, err := e.svc.Book(context.Background(), booking.Request{ResourceID: r1, SlotStart: s1, CustomerName: "a", IdempotencyKey: fmt.Sprintf("p%d-a", iter)}); err != nil {
			t.Fatalf("iter %d first: %v", iter, err)
		}
		_, _, err := e.svc.Book(context.Background(), booking.Request{ResourceID: r2, SlotStart: s2, CustomerName: "b", IdempotencyKey: fmt.Sprintf("p%d-b", iter)})
		same := r1 == r2 && booking.CanonicalSlotStart(s1).Equal(booking.CanonicalSlotStart(s2))
		if competed := errors.Is(err, booking.ErrSlotTaken); competed != same || (!same && err != nil) {
			t.Fatalf("iter %d: (%s, %v) then (%s, %v): err %v, same slot %v", iter, r1, s1, r2, s2, err, same)
		}
	}
}
