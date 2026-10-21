package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "booking.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func userVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	return v
}

func tableNames(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()
	rows, err := db.Query("SELECT name FROM sqlite_schema WHERE type = 'table'")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()
	names := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names[n] = true
	}
	return names
}

func TestOpen_FreshDatabaseIsMigrated(t *testing.T) {
	s := openTestStore(t)

	want, err := loadMigrations(embeddedMigrations)
	if err != nil {
		t.Fatal(err)
	}
	if got := userVersion(t, s.db); got != len(want) || got < 1 {
		t.Fatalf("user_version = %d, want %d", got, len(want))
	}
	tables := tableNames(t, s.db)
	for _, name := range []string{"resource", "resource_working_hours", "booking"} {
		if !tables[name] {
			t.Errorf("table %q missing after Open on a fresh database", name)
		}
	}
}

func TestOpen_EveryConnectionCarriesThePragmas(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	// Hold several connections at once so the pool must create new ones.
	var conns []*sql.Conn
	for range 3 {
		c, err := s.db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	for i, c := range conns {
		var mode string
		var fk, busy int
		if err := c.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
			t.Fatal(err)
		}
		if err := c.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
			t.Fatal(err)
		}
		if err := c.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busy); err != nil {
			t.Fatal(err)
		}
		if mode != "wal" || fk != 1 || busy != int(busyTimeout.Milliseconds()) {
			t.Errorf("conn %d: journal_mode=%q foreign_keys=%d busy_timeout=%d, want wal 1 %d",
				i, mode, fk, busy, busyTimeout.Milliseconds())
		}
		c.Close()
	}
}

func TestOpen_ReopenKeepsDataAndAppliesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "booking.db")
	ctx := context.Background()

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	insertResource(t, s.db, "r1")
	v := userVersion(t, s.db)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = Open(ctx, path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer s.Close()
	if got := userVersion(t, s.db); got != v {
		t.Fatalf("user_version after reopen = %d, want %d", got, v)
	}
	var n int
	if err := s.db.QueryRow("SELECT count(*) FROM resource").Scan(&n); err != nil || n != 1 {
		t.Fatalf("resources after reopen = %d (err %v), want 1", n, err)
	}
}

func TestOpen_RejectsEmptyPath(t *testing.T) {
	if _, err := Open(context.Background(), ""); err == nil {
		t.Fatal("Open(\"\") succeeded, want an error")
	}
}

func TestOpen_FailsWhenDirectoryDoesNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "booking.db")
	if _, err := Open(context.Background(), path); err == nil {
		t.Fatal("Open succeeded on a path whose directory does not exist")
	}
}

func TestOpen_PathWithURISpecialCharacters(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a b?c#d%20e")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "booking.db")
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database not created at the exact given path: %v", err)
	}
}

// TestOpen_ConcurrentOpensOfAnUnmigratedFileApplyMigrationsOnce covers F7.
// The file is already in WAL mode (a persistent setting) but has no schema:
// the very first switch of a brand-new file to WAL can fail fast with
// SQLITE_BUSY under a concurrent open without consulting the busy handler,
// which the failure matrix declares separately (F7a).
func TestOpen_ConcurrentOpensOfAnUnmigratedFileApplyMigrationsOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "booking.db")
	raw, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatal(err)
	}
	if err := checkJournalMode(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	const n = 8
	stores := make([]*Store, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { stores[i], errs[i] = Open(context.Background(), path) })
	}
	wg.Wait()
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("Open %d: %v", i, errs[i])
		}
		defer stores[i].Close()
	}
	want, _ := loadMigrations(embeddedMigrations)
	if got := userVersion(t, stores[0].db); got != len(want) {
		t.Fatalf("user_version = %d, want %d", got, len(want))
	}
}

func TestOpen_HonoursContextWhileWaitingForTheWriteLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "booking.db")
	holder := openStoreAt(t, path)

	// Hold the write lock so the migration runner's BEGIN IMMEDIATE waits.
	conn, err := holder.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	s, err := Open(ctx, path)
	elapsed := time.Since(start)
	if err == nil {
		s.Close()
		t.Fatal("Open succeeded while another connection held the write lock")
	}
	if elapsed > busyTimeout/2 {
		t.Fatalf("Open returned after %v; the context deadline (200ms) was not honoured", elapsed)
	}
}

func openStoreAt(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestIsUniqueViolation(t *testing.T) {
	s := openTestStore(t)
	insertResource(t, s.db, "r1")
	if err := insertBooking(s.db, "b1", "r1", 1_800_000_000, "Ann", "k1", "active"); err != nil {
		t.Fatal(err)
	}

	slotTaken := insertBooking(s.db, "b2", "r1", 1_800_000_000, "Bob", "k2", "active")
	keyUsed := insertBooking(s.db, "b3", "r1", 1_800_001_800, "Bob", "k1", "active")
	duplicateID := insertBooking(s.db, "b1", "r1", 1_800_003_600, "Bob", "k3", "active")
	unknownResource := insertBooking(s.db, "b4", "nope", 1_800_003_600, "Bob", "k4", "active")
	badStatus := insertBooking(s.db, "b5", "r1", 1_800_003_600, "Bob", "k5", "pending")
	for name, err := range map[string]error{
		"slot taken": slotTaken, "key used": keyUsed, "primary key": duplicateID,
		"foreign key": unknownResource, "check": badStatus,
	} {
		if err == nil {
			t.Fatalf("%s insert succeeded, want a constraint error", name)
		}
	}

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("boom"), false},
		{"active slot index", slotTaken, true},
		{"idempotency key index", keyUsed, true},
		{"wrapped unique violation", errors.Join(errors.New("ctx"), slotTaken), true},
		{"primary key", duplicateID, false},
		{"foreign key", unknownResource, false},
		{"check", badStatus, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsUniqueViolation(c.err); got != c.want {
				t.Fatalf("IsUniqueViolation(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}
