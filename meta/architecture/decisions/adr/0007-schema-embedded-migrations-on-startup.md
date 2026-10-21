# ADR-0007: Schema creation and evolution

**Status:** Accepted  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

The full API must work end to end against a fresh database (NFR-002), and the service is one binary configured through environment variables (CON-005) with SQLite as its only storage (CON-003). The brief names no mechanism for creating or evolving the schema. Following ADR-0006, the store is reached through the modernc.org/sqlite driver, so any migration tooling must work with it. A fresh database is the normal case for every test run and for the first start of the service.

## Decision

We will keep versioned SQL files embedded in the binary with `go:embed` and apply them at startup inside a transaction, tracking the applied version in SQLite's `user_version` pragma. This satisfies NFR-002 because a fresh database needs no extra step, and it keeps the deliverable a single binary as CON-005 requires, with no third-party migration tooling.

## Alternatives considered

### embedded_migrations_on_startup (chosen)
Embedded versioned SQL applied at startup in a transaction. Explicit versioning and no extra dependency. Cost: a small hand-written runner that must be written and tested, and a failed migration blocks startup.

### migration_library_on_startup
A migration library such as goose or golang-migrate run in the process at startup with embedded files. Proven up/down handling and version tracking. Rejected because it adds a dependency whose SQLite support must be shown to work with the chosen driver, and it offers more than one small schema needs.

### separate_migrate_step
A separate subcommand or external tool run before the service starts. Keeps startup from mutating the schema, but every runtime and every test must remember the extra step or a fresh database fails (NFR-002). Rejected.

## Consequences

### Positive
- A fresh database works with no extra step, which makes every test and the first start trivial.
- The artifact stays a single binary with no extra tooling.
- The applied schema version is explicit and queryable.

### Negative
- A hand-written runner is code to own and to test.
- A failed migration prevents the service from starting.

### Neutral
- Migration files live in the repository and are reviewed with the code.
- Rolling a schema back is a new forward migration, as there is no automatic down step.

## References

- DEC-023 (resolved by this ADR)
- ADR-0006 (driver choice this decision depends on)
- NFR-002, CON-003, CON-005

## History

- 2026-10-21: Created — embedded migrations applied at startup, version tracked in user_version.
