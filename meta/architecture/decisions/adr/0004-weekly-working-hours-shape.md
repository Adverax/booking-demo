# ADR-0004: Shape of weekly working hours

**Status:** Accepted (revised 2026-10-22)  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** 2026-10-22  
**Migration:** grandfather  
**Pattern:** —  
**API-Posture:** —

## Context

An operator creates a resource with weekly working hours, and the free slots of that resource are derived from them (FR-001, FR-003). The design brief gives only the example Mon-Fri 09:00-17:00 and states that anything not written is out of scope; holidays and per-date exceptions are explicitly excluded. Slots are 30 minutes long and aligned to the half hour, and all times are UTC (CON-001). The brief does not say whether one weekday may carry several intervals (for example a lunch break), whether interval bounds must be half-hour aligned, or whether an interval may cross midnight UTC.

## Decision

We will allow exactly one working interval per weekday, with both bounds aligned to the half hour, entirely inside a single UTC day. The end of an interval may be written 24:00, meaning the end of that UTC day (exclusive); no start may be 24:00, and no bound may exceed it. This satisfies FR-001 and FR-003 because it covers the brief's example, keeps slot derivation a single comparison per day, and avoids validation rules the brief does not ask for.

## Alternatives considered

### single_interval_per_day (chosen)
One interval per weekday, half-hour aligned, not crossing midnight UTC. Simple to validate, to derive slots from and to test. It cannot express a lunch break.

### multiple_intervals_per_day
Several intervals per weekday. Rejected for the first release: it needs overlap and ordering validation, more tests, and goes beyond the example in the brief. It can be added later by a separate decision.

## Consequences

### Positive
- Slot derivation per day is one interval check.
- Validation of working hours is small and fully testable.

### Negative
- A resource with a break during the day cannot be modelled; a business that needs one cannot be served by the first release.
- Cross-midnight shifts are not representable; an interval that ends at midnight is written with the end 24:00.

### Neutral
- Extending to several intervals per day later changes the working-hours schema and needs a new decision.
- Validation errors for misaligned or cross-midnight intervals need stable error codes.

## References

- DEC-004 (resolved by this ADR)
- FR-001 (resources with working hours), FR-003 (availability)
- TERM-002 (working hours), CON-001 (UTC)

## History

- 2026-10-22: Revised — an interval end of 24:00 (the end of the UTC day) is allowed, so "open until midnight" can be expressed inside a single UTC day. Raised by the CARD-001 implementation, which had assumed it; no code under the previous wording exists on the main branch, hence Migration: grandfather.

- 2026-10-21: Created — one half-hour-aligned interval per weekday inside a single UTC day.
