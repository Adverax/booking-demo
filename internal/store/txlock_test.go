package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDriver_TxLockImmediateTakesTheWriteLockAtBegin records the owed
// verification of ADR-0012's fallback against the pinned modernc.org/sqlite:
// the `_txlock=immediate` DSN option exists and makes BeginTx take SQLite's
// write lock at BEGIN, so a second writer's BeginTx waits for it (and fails
// with SQLITE_BUSY once its busy timeout expires) before reading anything.
// The store itself does not use the option; this test keeps the fallback's
// availability checked whenever the driver version changes.
func TestDriver_TxLockImmediateTakesTheWriteLockAtBegin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "txlock.db")
	open := func(extra string) *sql.DB {
		db, err := sql.Open("sqlite", "file:"+escapePath(path)+
			"?_pragma=busy_timeout(200)&_pragma=journal_mode(WAL)"+extra)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		return db
	}
	ctx := context.Background()

	immediate := open("&_txlock=immediate")
	if _, err := immediate.Exec("CREATE TABLE t (x INTEGER)"); err != nil {
		t.Fatal(err)
	}
	holder, err := immediate.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback()

	// A deferred BEGIN on another connection does not contend: no lock yet.
	deferred := open("")
	tx, err := deferred.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("deferred BEGIN failed while an immediate transaction is open: %v", err)
	}
	tx.Rollback()

	// An immediate BEGIN on another connection contends at BEGIN itself.
	second := open("&_txlock=immediate")
	start := time.Now()
	_, err = second.BeginTx(ctx, nil)
	if err == nil {
		t.Fatal("second _txlock=immediate BEGIN succeeded while the first holds the write lock")
	}
	if !strings.Contains(err.Error(), "SQLITE_BUSY") && !strings.Contains(err.Error(), "locked") {
		t.Fatalf("second immediate BEGIN failed with %v, want SQLITE_BUSY", err)
	}
	if waited := time.Since(start); waited < 150*time.Millisecond {
		t.Fatalf("second immediate BEGIN failed after %v without waiting on the busy timeout", waited)
	}

	// An unknown value is rejected, so the option is parsed, not ignored.
	bad := open("&_txlock=sometimes")
	if err := bad.Ping(); err == nil {
		t.Fatal("_txlock=sometimes accepted")
	}
}
