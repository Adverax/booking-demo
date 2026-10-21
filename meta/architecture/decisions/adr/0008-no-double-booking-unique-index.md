# ADR-0008: Enforcing no double booking under concurrency

**Status:** Accepted  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

Two active bookings of the same resource must never overlap, also under concurrent requests (INV-001), and repeating a booking request with the same idempotency key must return the original booking and create nothing new (INV-003, ADR-0001, ADR-0002). The release criterion is a test where 50 simultaneous requests for one slot yield exactly one success, the others being rejected as conflicts rather than failing with lock errors (NFR-001). Storage is SQLite behind the modernc.org/sqlite driver (ADR-0006), which allows a single writer at a time, and the service runs as a single instance (ADR-0005). Every booking is exactly one 30-minute slot aligned to the half hour (FR-003), so overlap between two bookings of a resource means an equal slot start.

## Decision

We will enforce the invariant in the schema: a partial unique index on the resource and slot start for bookings whose status is active, plus a unique index on the idempotency key. The database rejects the losing insert, and the constraint violation is mapped to a conflict response, with WAL mode and a busy timeout absorbing writer contention. This satisfies INV-001 and INV-003 because the invariant holds whatever the application code does, and the insert is itself the check.

## Alternatives considered

### unique_index_on_active_slot (chosen)
The invariant lives in the schema. No application-level locking is needed, and cancelling a booking by changing its status frees the slot for a new booking (FR-007, FR-008). It relies on the fixed-slot rule, and the two kinds of constraint error, slot taken and idempotency key already used, must be told apart to return the right response.

### immediate_transaction_check_then_insert
Each booking runs in a `BEGIN IMMEDIATE` transaction that reads the key and any overlapping active booking and then inserts. The overlap rule is a query independent of slot length. Rejected because correctness depends on every write path using such a transaction, and 50 simultaneous writers queue on the lock and need a long enough busy timeout to avoid busy failures.

### in_process_serialization
A per-resource mutex or a single writer connection inside the process. Simple to reason about, but the invariant would be held by code and not by data: a second process or a bypassing path breaks it, and it relies on the runtime staying single-instance. Rejected.

## Consequences

### Positive
- The invariant holds in the data, independent of application code paths.
- No application-level locking, and the same index structure answers INV-001 and INV-003.
- A cancelled booking frees its slot automatically through its status.

### Negative
- The design depends on fixed 30-minute slots; multi-slot bookings, which are out of scope, would need a range check instead of an index.
- Constraint violations must be classified (slot versus idempotency key) to return the correct response.

### Neutral
- Writer contention is handled with WAL mode and a busy timeout, which must be configured and tested with the 50-request scenario.
- The concurrency test of NFR-001 becomes the proof that the index behaves as intended.

## References

- DEC-024 (resolved by this ADR)
- ADR-0006 (driver), ADR-0005 (single instance), ADR-0001, ADR-0002
- INV-001, INV-003, NFR-001, FR-003, FR-005, FR-006
- CTX-001, AGG-002

## History

- 2026-10-21: Created — no double booking and key uniqueness enforced by unique indexes in the schema.
