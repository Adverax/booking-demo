# ADR-0010: Where the cancellation-is-final rule is enforced

**Status:** Accepted  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

A cancelled booking cannot be reactivated and a new booking is needed (FR-008, INV-004, ADR-0002). ADR-0008 enforces INV-001 and INV-003 with unique indexes in the schema, but the partial index on active slots only constrains rows whose status is active, so an update of a cancelled booking back to active succeeds whenever its slot is free. Nothing in the accepted ADRs says where INV-004 is enforced. The cancellation itself is a status change, which is therefore also the only path by which a reactivation could happen, and the model defines no endpoint that reactivates a booking (AC-030, ASM-0001).

## Decision

We will forbid the transition from cancelled back to active in the schema, with a trigger on the booking status that aborts such an update, and keep the matching guard in the domain code as well. This satisfies FR-008 and INV-004 in the same way ADR-0008 satisfies INV-001 and INV-003: the rule holds in the data whatever the application code does, and AC-030 becomes testable at the storage level without an endpoint that reactivates.

## Alternatives considered

### schema_guard_on_status (chosen)
A trigger (or check) in the schema rejects the cancelled-to-active transition, in addition to the domain guard. Costs one small migration and a mapping of the violation to an error.

### domain_guard_only
Enforce the transition only in the domain code that is the sole writer of the status, and narrow ADR-0008's claim to INV-001 and INV-003. Cheaper, but the protection would rest on there being no other write path, which is the kind of discipline the rest of the design avoids.

## Consequences

### Positive
- INV-004 holds in the data, consistent with ADR-0008.
- AC-030 can be tested directly at the storage level, which also settles most of ASM-0001.

### Negative
- A migration with a trigger must be written, versioned (ADR-0007) and tested.
- A violation must be mapped to a stable error, even though no public route can trigger it.

### Neutral
- The domain guard stays, so a violation is normally caught before reaching the database.
- Any future change to the allowed status transitions needs a schema migration.

## References

- DEC-026 (resolved by this ADR)
- FR-008, INV-004, AC-030
- ADR-0002, ADR-0007, ADR-0008
- CTX-001, AGG-002

## History

- 2026-10-21: Created — a trigger forbids cancelled-to-active, with a domain guard kept as well.

## Rules
```yaml
- id: ADR-0010/R1
  statement: A booking whose status is cancelled is never set active again; the schema rejects the transition
  scope: {contexts: [CTX-001]}
  check: {kind: test, ref: TestBooking_CannotReactivate}
  severity: mandatory
```
