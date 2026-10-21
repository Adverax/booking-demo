// Package store is the SQLite store of the booking service (COMP-005) and the
// runner of its embedded schema migrations (COMP-006).
//
// SQLite is reached through modernc.org/sqlite behind database/sql (ADR-0006).
// WAL journal mode, a busy timeout and foreign keys are set in the connection
// string, so every pooled connection carries them (ADR-0006, ADR-0012). The
// schema is migrated inside Open (ADR-0007), so a fresh database needs no
// extra step. Every instant is stored as integer UTC epoch seconds (ADR-0013).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// busyTimeout bounds how long one statement waits for SQLite's single write
// lock before failing with SQLITE_BUSY (ADR-0008, ADR-0012).
const busyTimeout = 5 * time.Second

// Store is the opened, migrated booking database. Read and write operations
// are added as methods by the components that need them.
type Store struct {
	db *sql.DB
}

// Open opens the SQLite database at path, creating the file if needed, and
// applies every pending embedded migration before returning. The path is
// supplied by the caller's configuration (CON-005). On any error the
// connection pool is closed and nothing is returned.
func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("store: empty database path")
	}
	migrations, err := loadMigrations(embeddedMigrations)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("store: open %q: %w", path, err)
	}
	if err := checkJournalMode(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: open %q: %w", path, err)
	}
	if err := migrate(ctx, db, migrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: migrate %q: %w", path, err)
	}
	return &Store{db: db}, nil
}

// Close refuses new statements, waits for the ones in flight and closes the
// database.
func (s *Store) Close() error {
	return s.db.Close()
}

// IsUniqueViolation reports whether err is a rejection by a unique index:
// the active-slot index or the idempotency-key index of ADR-0008. Which index
// fired is deliberately not reported (ADR-0011 classifies from stored data).
// Primary-key, foreign-key, check and datatype violations are not unique
// violations.
func IsUniqueViolation(err error) bool {
	var e *sqlite.Error
	return errors.As(err, &e) && e.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

// dsn builds the driver connection string. The path is percent-encoded into
// a file: URI so characters such as '?' and '#' stay part of the file name.
func dsn(path string) string {
	q := url.Values{}
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busyTimeout.Milliseconds()))
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	return "file:" + escapePath(path) + "?" + q.Encode()
}

func escapePath(path string) string {
	return strings.NewReplacer("%", "%25", "?", "%3F", "#", "%23").Replace(path)
}

// checkJournalMode fails when the file system did not accept WAL mode, so the
// store never silently runs with a rollback journal.
func checkJournalMode(ctx context.Context, db *sql.DB) error {
	var mode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		return err
	}
	if !strings.EqualFold(mode, "wal") {
		return fmt.Errorf("journal mode is %q, want wal", mode)
	}
	return nil
}
