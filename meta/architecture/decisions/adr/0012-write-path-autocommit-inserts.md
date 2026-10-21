# ADR-0012: Write path under concurrent bookings

**Status:** Accepted  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

The release criterion is that 50 simultaneous requests for one slot yield exactly one success, the others being conflicts and not lock errors (NFR-001). ADR-0006 selects the modernc.org/sqlite driver and ADR-0008 relies on a busy timeout and WAL to absorb contention among writers behind unique indexes. SQLite allows a single writer, and a deferred transaction that has already read and then tries to write can fail on a stale snapshot without waiting for the busy timeout, which would surface as lock errors in the NFR-001 test. ADR-0006 also states that the driver has no connection-string option for immediate transactions; a check of the driver's documentation indicates that it documents a transaction locking DSN parameter (`_txlock`, with values deferred, immediate and exclusive), but its exact behaviour was not verified against the pinned version, so that statement of ADR-0006 should be treated as unreliable.

## Decision

We will perform each booking write as a single autocommit statement, with WAL mode and a busy timeout configured in the connection string, and we will not use a transaction that reads and then writes on the booking path. The read that classifies a unique violation (ADR-0011) is a separate autocommit read. This satisfies NFR-001 because a lone insert waits on the busy timeout instead of upgrading a stale read snapshot. If the concurrency test shows lock errors, the fallback is to enable immediate transactions through the driver's transaction locking option after verifying it on the pinned version.

## Alternatives considered

### autocommit_insert_only (chosen)
Single autocommit statements, WAL and a busy timeout, no read-then-write transaction. The fewest moving parts; it needs the discipline that no such transaction exists on the write path.

### immediate_transactions
Run booking writes in immediate transactions so the write lock is taken at the start and no snapshot upgrade occurs. Rejected for the first release: writers queue on the lock, and it depends on the driver's `_txlock` behaviour, which is unverified. It remains the documented fallback.

## Consequences

### Positive
- No snapshot-upgrade failures by construction, since the write path has no read-then-write transaction.
- The classification and the write paths stay simple autocommit statements.

### Negative
- The absence of read-then-write transactions on the booking path is a convention that must be kept in code review and tests.
- Under 50 writers some requests wait on the busy timeout, which must be long enough and is proven only by the NFR-001 test.

### Neutral
- The driver's transaction locking option must be checked on the pinned version before it is relied on as the fallback.
- ADR-0006's statement about the driver's DSN options is superseded by this note on that point.

## References

- DEC-028 (resolved by this ADR)
- ADR-0006, ADR-0008, ADR-0011
- NFR-001, AC-019, AC-031
- CTX-001

## History

- 2026-10-21: Created — autocommit inserts with WAL and a busy timeout; immediate transactions kept as a verified fallback.

## Rules
```yaml
- id: ADR-0012/R1
  statement: The booking write path uses single autocommit statements and no transaction that reads and then writes
  scope: {contexts: [CTX-001]}
  check: {kind: review-lens}
  severity: mandatory
```
