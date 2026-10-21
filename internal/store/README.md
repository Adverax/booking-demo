# store

## Purpose

The SQLite store of the booking service (COMP-005) and the runner of its embedded schema migrations (COMP-006). `Open` connects, checks that WAL mode is active, and applies every pending migration before returning, so a fresh database needs no extra step. Read and write operations are added as `Store` methods by later cards.

## Architecture

- SQLite is reached through `modernc.org/sqlite` behind `database/sql` (ADR-0006). WAL, a 5 s busy timeout and foreign keys are set in the connection string, so every pooled connection carries them (ADR-0012).
- Migrations are SQL files in [`migrations/`](./migrations/README.md), embedded with `go:embed` (ADR-0007). The runner lives here because `go:embed` can only embed files at or below the embedding package.
- All pending migrations run in one `BEGIN IMMEDIATE` transaction on a dedicated connection, together with the `PRAGMA user_version` update: schema and version change atomically, and a concurrent starter waits for the write lock and then applies nothing. This is not an instance lock (DEC-029 stays open).
- A database whose `user_version` is newer than the binary's newest migration is refused with `ErrSchemaTooNew`.
- Every instant is stored as integer UTC epoch seconds and converted only through `toEpoch` / `fromEpoch` (ADR-0013).
- The package defines no repository interfaces; consuming components own their ports.

## Key entities

| Name | Kind | Description | File |
|------|------|-------------|------|
| `Open` | func | Opens (creating if needed) and migrates the database at a path | [`store.go`](./store.go) |
| `Store` | type | Handle over the migrated `*sql.DB`; `Close` shuts it down | [`store.go`](./store.go) |
| `IsUniqueViolation` | func | Reports a unique-index rejection (active slot or idempotency key) | [`store.go`](./store.go) |
| `ErrSchemaTooNew` | var | Database schema is newer than this binary | [`migrate.go`](./migrate.go) |
| `toEpoch` / `fromEpoch` | func | The only conversions of instants into and out of SQL | [`epoch.go`](./epoch.go) |

## Usage

```go
s, err := store.Open(ctx, "booking.db")
if err != nil {
	return err
}
defer s.Close()
```

## See also

- [`migrations/`](./migrations/README.md) - the schema files
- ADR-0006, ADR-0007, ADR-0008, ADR-0012, ADR-0013 in `meta/architecture/decisions/adr/`
