# ADR-0016: What counts as a slot in the past

**Status:** Accepted  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

A booking is accepted only for a slot that is inside working hours and not in the past (FR-005, INV-002). The brief does not say what "in the past" means for a slot that has already started but not yet ended. The current time is read through an injectable clock and instants are stored as integer UTC epoch seconds (ADR-0013), so the rule must be stated precisely enough to test at the boundary. The service has no concept of a walk-in or of a partially used slot, and slots are fixed 30-minute slots aligned to the half hour (FR-003).

## Decision

We will reject a booking when the start of its slot is earlier than the current time, and accept it when the start is equal to or later than the current time. This satisfies FR-005 and INV-002 with a single comparison of two instants, and it makes the boundary testable with a controlled clock.

## Alternatives considered

### start_before_now (chosen)
Reject when the slot start is earlier than now. Simple and unambiguous, and it matches the plain reading of "not in the past". A slot that has just started can no longer be booked.

### end_before_now
Reject only when the slot has fully ended, so a slot in progress is still bookable. More permissive for booking on the spot, but a 30-minute slot could be booked a minute before it ends, which the brief does not ask for.

## Consequences

### Positive
- One comparison decides the rule, and the boundary (start equal to now) is stated.
- A late replay of an old slot is rejected by the same rule that rejects a new booking (see DEC-012).

### Negative
- A customer cannot book the slot that is running right now.

### Neutral
- A named test covers the boundary with the injected clock (ADR-0013).
- Availability may still list past slots (ASM-0002).

## References

- DEC-005 (resolved by this ADR)
- FR-005, INV-002, FR-003
- ADR-0013, ASM-0002, DEC-012
- CTX-001

## History

- 2026-10-21: Created — a slot is in the past when its start is earlier than the current time.
