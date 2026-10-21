package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
)

// embeddedMigrations holds the versioned schema files (ADR-0007). A file is
// named NNNN_description.sql; versions start at 1 and have no gaps. A
// migration, once released, is never edited: a change is a new file.
//
//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// ErrSchemaTooNew is returned when the database carries a schema version
// higher than the newest migration this binary knows.
var ErrSchemaTooNew = errors.New("store: database schema is newer than this binary")

type migration struct {
	version int
	name    string
	sql     string
}

var migrationName = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`)

// loadMigrations reads every *.sql file under migrations/ in fsys and returns
// them ordered by version, refusing a malformed set before any database work.
func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, "migrations")
	if err != nil {
		return nil, fmt.Errorf("store: read migrations: %w", err)
	}
	var ms []migration
	for _, e := range entries {
		m := migrationName.FindStringSubmatch(e.Name())
		if e.IsDir() || m == nil {
			return nil, fmt.Errorf("store: migration file %q is not named NNNN_description.sql", e.Name())
		}
		version, _ := strconv.Atoi(m[1])
		body, err := fs.ReadFile(fsys, "migrations/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("store: read migration %q: %w", e.Name(), err)
		}
		ms = append(ms, migration{version: version, name: e.Name(), sql: string(body)})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	for i, m := range ms {
		if m.version != i+1 {
			return nil, fmt.Errorf("store: migration %q has version %d, want %d (versions start at 1 with no gaps or duplicates)", m.name, m.version, i+1)
		}
	}
	return ms, nil
}

// migrate applies every migration newer than the database's user_version in
// one transaction and records the new version in the same transaction, so a
// failure leaves both schema and version untouched (ADR-0007).
//
// The transaction is BEGIN IMMEDIATE on a dedicated connection: the write
// lock is taken before user_version is read, so a second process starting
// against the same file waits on the busy timeout and then finds nothing to
// apply, instead of failing on a stale read snapshot. This is not an instance
// lock (DEC-029 is open); it only makes the runner itself safe to repeat.
func migrate(ctx context.Context, db *sql.DB, ms []migration) (err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			// A failed rollback leaves the transaction to die with the
			// connection: Open closes the pool on any migration error.
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
		}
	}()

	var current int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	if current > len(ms) {
		return fmt.Errorf("%w: database version %d, newest known %d", ErrSchemaTooNew, current, len(ms))
	}
	for _, m := range ms[current:] {
		if _, err := conn.ExecContext(ctx, m.sql); err != nil {
			return fmt.Errorf("apply %s: %w", m.name, err)
		}
	}
	if current < len(ms) {
		// PRAGMA takes no bound parameters; the value is an int we computed.
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", len(ms))); err != nil {
			return err
		}
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}
