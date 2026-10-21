# ADR-0006: SQLite driver

**Status:** Accepted  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

SQLite is the only storage and must be reached through a pure-Go driver with no cgo (CON-003), but the brief names no driver. The available pure-Go drivers differ in how they implement SQLite (machine-translated C versus a WebAssembly build), in how they expose transaction locking modes and busy handling, and in maturity. The driver choice bears on the concurrency release criterion, where 50 simultaneous requests for one slot must yield exactly one success (NFR-001), and on the decisions for schema creation and concurrency enforcement that depend on it.

## Decision

We will use modernc.org/sqlite through the standard `database/sql` interface. This satisfies CON-003 because it is pure Go, and it is the most widely used pure-Go driver with the longest track record; the pragmas the design needs (busy timeout, journal mode, foreign keys) are set through the connection string. The main advantage of the alternative, a driver option for immediate transactions, is not needed once the concurrency decision relies on a unique index.

## Alternatives considered

### modernc_sqlite (chosen)
SQLite's C source machine-translated to Go. Mature and widely used. It produces a large generated code base, slower builds and a larger binary, and it has no connection-string option for immediate transactions, so those would need an explicit statement on a dedicated connection.

### ncruces_go_sqlite3
SQLite compiled to WebAssembly and run by the wazero runtime, with a `database/sql` driver that supports a transaction lock mode option. Rejected because it is a younger project with a smaller user base and a WebAssembly runtime that adds startup cost and an unusual execution model, for an advantage this design does not need.

## Consequences

### Positive
- A mature, widely used driver behind the standard `database/sql` interface.
- Pragmas are configured through the DSN, so no custom connection code is needed.

### Negative
- A larger binary and slower builds because of the generated code base.
- Immediate transactions, if ever needed, require an explicit statement on a dedicated connection.

### Neutral
- The migration approach and the concurrency enforcement must work with this driver (ADR-0007, ADR-0008).
- The driver version needs to be pinned and tracked as a dependency.

## References

- DEC-022 (resolved by this ADR)
- CON-003, NFR-001
- CTX-001

## History

- 2026-10-21: Created — modernc.org/sqlite chosen as the pure-Go SQLite driver.
