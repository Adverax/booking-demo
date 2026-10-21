# ADR-0013: Time representation in storage and the source of the current time

**Status:** Accepted  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

All times are stored and returned in UTC, in RFC 3339 format (CON-001). ADR-0009 settles the wire format only. The storage representation of times in SQLite is not decided, and SQLite compares text bytewise, so two spellings of one instant, such as a trailing Z and a zero offset or a fractional second, would be different keys in the unique index on the active slot of ADR-0008 (ASM-0006). The rule that a booking is accepted only for a slot that is not in the past (INV-002, FR-005) needs a source of the current time, and the model provides none, which makes the boundary of that rule hard to test.

## Decision

We will store every instant in the database as an integer number of seconds since the Unix epoch in UTC, convert to and from RFC 3339 only at the API boundary, and obtain the current time through an injectable clock used by the booking and availability logic. This satisfies CON-001 because the stored value is unambiguously UTC and the returned value is RFC 3339 UTC, and it removes the possibility of two index keys for one instant while making the not-in-the-past rule testable without sleeping.

## Alternatives considered

### decide_time_storage_and_clock (chosen)
A single stored representation (integer epoch seconds, UTC) and an injected clock, recorded in this ADR. Costs a one-time convention and means stored values are unreadable without conversion.

### leave_as_implementation_detail
Leave the representation and the clock to the cards, covered by the UTC acceptance criteria. Rejected: it allows different parts of the code to store the same instant in different forms, which is exactly the index-key hazard of ASM-0006, and leaves the past-slot check dependent on the system clock.

## Consequences

### Positive
- One instant has exactly one stored key, so the unique index of ADR-0008 compares equal starts as equal.
- The not-in-the-past rule and slot boundaries are testable with a controlled clock.

### Negative
- Stored times are not human-readable in the database without a conversion.
- Every read and write path must convert at the boundary, so a conversion helper becomes part of the code.

### Neutral
- Slot starts are half-hour aligned, which is validated at the boundary before storage (AC-017).
- The decision of DEC-005 (what exactly counts as in the past) uses this clock.

## References

- DEC-032 (resolved by this ADR)
- CON-001, INV-002, FR-005, AC-017
- ADR-0008, ADR-0009, ASM-0006, DEC-005
- CTX-001

## History

- 2026-10-21: Created — integer epoch seconds in UTC in storage, RFC 3339 at the API, injectable clock.

## Rules
```yaml
- id: ADR-0013/R1
  statement: Instants are stored as integer UTC epoch seconds and the current time is read only through the injected clock
  scope: {contexts: [CTX-001]}
  check: {kind: review-lens}
  severity: mandatory
```
