package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// openRaw opens a database with the store's connection settings but without
// migrating it, so the runner can be driven with test migration sets.
func openRaw(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", dsn(filepath.Join(t.TempDir(), "raw.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func mustLoad(t *testing.T, fsys fstest.MapFS) []migration {
	t.Helper()
	ms, err := loadMigrations(fsys)
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	return ms
}

func file(sql string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(sql)} }

func TestMigrate_AppliesOnlyPendingMigrations(t *testing.T) {
	db := openRaw(t)
	ctx := context.Background()
	v1 := fstest.MapFS{"migrations/0001_a.sql": file("CREATE TABLE a (x INTEGER) STRICT;")}
	if err := migrate(ctx, db, mustLoad(t, v1)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO a VALUES (1)"); err != nil {
		t.Fatal(err)
	}

	// Re-running 0001 would fail ("table a already exists"), so success proves
	// only 0002 ran.
	v2 := fstest.MapFS{
		"migrations/0001_a.sql": file("CREATE TABLE a (x INTEGER) STRICT;"),
		"migrations/0002_b.sql": file("CREATE TABLE b (y INTEGER) STRICT; INSERT INTO b VALUES (2);"),
	}
	if err := migrate(ctx, db, mustLoad(t, v2)); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if got := userVersion(t, db); got != 2 {
		t.Fatalf("user_version = %d, want 2", got)
	}
	var y int
	if err := db.QueryRow("SELECT y FROM b").Scan(&y); err != nil || y != 2 {
		t.Fatalf("multi-statement migration not fully applied: y=%d err=%v", y, err)
	}
	if err := migrate(ctx, db, mustLoad(t, v2)); err != nil {
		t.Fatalf("migrate with nothing pending: %v", err)
	}
}

func TestMigrate_FailedMigrationLeavesSchemaAndVersionUntouched(t *testing.T) {
	db := openRaw(t)
	set := fstest.MapFS{
		"migrations/0001_good.sql": file("CREATE TABLE good (x INTEGER) STRICT;"),
		"migrations/0002_bad.sql":  file("CREATE TABLE half (x INTEGER) STRICT; THIS IS NOT SQL;"),
	}
	if err := migrate(context.Background(), db, mustLoad(t, set)); err == nil {
		t.Fatal("migrate succeeded with a broken migration")
	}
	if got := userVersion(t, db); got != 0 {
		t.Fatalf("user_version = %d after a failed run, want 0", got)
	}
	if tables := tableNames(t, db); len(tables) != 0 {
		t.Fatalf("tables left behind by a failed run: %v", tables)
	}
}

func TestMigrate_RejectsDatabaseNewerThanBinary(t *testing.T) {
	db := openRaw(t)
	if _, err := db.Exec("PRAGMA user_version = 7"); err != nil {
		t.Fatal(err)
	}
	set := fstest.MapFS{"migrations/0001_a.sql": file("CREATE TABLE a (x INTEGER) STRICT;")}
	err := migrate(context.Background(), db, mustLoad(t, set))
	if !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("migrate error = %v, want ErrSchemaTooNew", err)
	}
	if got := userVersion(t, db); got != 7 {
		t.Fatalf("user_version = %d, want 7 (untouched)", got)
	}
	if tables := tableNames(t, db); len(tables) != 0 {
		t.Fatalf("tables created on a too-new database: %v", tables)
	}
}

func TestLoadMigrations_RejectsMalformedSets(t *testing.T) {
	ok := "SELECT 1;"
	cases := map[string]fstest.MapFS{
		"gap": {
			"migrations/0001_a.sql": file(ok),
			"migrations/0003_c.sql": file(ok),
		},
		"duplicate version": {
			"migrations/0001_a.sql": file(ok),
			"migrations/0001_b.sql": file(ok),
		},
		"does not start at 1": {"migrations/0002_a.sql": file(ok)},
		"bad name":            {"migrations/1_a.sql": file(ok)},
		"not sql":             {"migrations/0001_a.txt": file(ok)},
	}
	for name, fsys := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := loadMigrations(fsys); err == nil {
				t.Fatal("loadMigrations accepted a malformed set")
			}
		})
	}
}

func TestLoadMigrations_EmbeddedSetIsWellFormed(t *testing.T) {
	ms, err := loadMigrations(embeddedMigrations)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) == 0 {
		t.Fatal("no embedded migrations")
	}
}
