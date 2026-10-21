# ADR-0011: Resolving a repeated idempotency key

**Status:** Accepted  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

A repeated idempotency key must return the original booking and create nothing new (FR-006, INV-003); the same key with a different payload is rejected (ADR-0001); a key repeated after cancellation returns the cancelled original (ADR-0002). ADR-0008 enforces the key and the active slot with two unique indexes. A genuine repeat of an identical request violates both indexes at once, SQLite does not guarantee which violation it reports first, and a payload mismatch cannot be detected by an index at all. The outcome of a failed insert therefore has to be decided by reading the stored booking, which the ADR set did not yet state. The order between this and the not-in-the-past check is the separate question of DEC-012.

## Decision

We will insert the booking first, and when the insert fails with a unique violation, read the booking stored under the request's idempotency key and decide from it: a stored booking with the same payload returns the original booking, a stored booking with a different payload is the key conflict of ADR-0001, and no stored booking means the slot was taken. This satisfies FR-006 and INV-003 deterministically, because the classification depends on what is stored and not on which index happened to report first.

## Alternatives considered

### insert_then_classify_violation (chosen)
Insert first and classify a violation by reading the key. One path handles replays, conflicts and concurrent races, and it needs no transaction that reads and then writes.

### lookup_key_then_insert
Read the key and compare the payload before inserting. Rejected as the primary mechanism: a concurrent request can insert the same key between the read and the insert, so the violation still has to be classified, which makes the up-front read an extra step on the common path rather than a replacement.

## Consequences

### Positive
- The result of a repeat or a race does not depend on index checking order.
- The 50-request scenario resolves through the same classification as an ordinary replay.

### Negative
- A failed insert is followed by a read on the replay and conflict paths.
- A late replay of a slot that has already started meets the not-in-the-past check first unless DEC-012 decides otherwise.

### Neutral
- A named test is needed for each classified outcome, including the concurrent case.
- The read after a violation is a single autocommit read, in line with ADR-0012.

## References

- DEC-027 (resolved by this ADR)
- ADR-0001, ADR-0002, ADR-0008, ADR-0012
- FR-006, INV-001, INV-003, AC-023
- CTX-001, AGG-002

## History

- 2026-10-21: Created — insert first, classify a unique violation by reading the stored booking.
