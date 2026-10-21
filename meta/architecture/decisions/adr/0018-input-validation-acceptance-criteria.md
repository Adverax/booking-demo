# ADR-0018: Input-validation paths get acceptance criteria now

**Status:** Accepted  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

FR-001 already has a negative acceptance criterion for an empty resource name, but the same kind of malformed input is uncovered elsewhere: an availability request whose range start is after its end (FR-004), a booking request with an empty customer name and one with an empty idempotency key (FR-005, FR-006). The API is generated from an OpenAPI contract (ADR-0009), which can express minimum lengths, yet the behaviour (a stable error code for each case) is a requirement and every rule must have a named test (NFR-003). Cards are cut from the acceptance criteria, so a path without one has no named test planned.

## Decision

We will add negative acceptance criteria for these inputs now, through the extraction phase in delta mode as in ADR-0014: an availability range whose start is after its end, an empty customer name and an empty idempotency key are each rejected with a stable error code and create nothing. This satisfies NFR-003 because each case gets a named test, and the contract of ADR-0009 will declare the matching constraints.

## Alternatives considered

### add_validation_acs (chosen)
Formalize the validation paths as acceptance criteria now. Costs one more extraction run, in line with how the other ADR-derived rules were handled.

### leave_to_api_spec
Leave input validation to the OpenAPI document. Cheaper, but the behaviour would exist only as schema constraints, with no acceptance criterion and no named test, which conflicts with NFR-003 and leaves the cards without criteria for it.

## Consequences

### Positive
- Every malformed-input path has a stable error code and a named test.
- The OpenAPI contract and the criteria say the same thing.

### Negative
- More acceptance criteria and planned tests, none yet written.
- One more extraction and trace refresh before the dependent cards can start.

### Neutral
- Oversized keys or names and the exact error-code names are decided in the API specification.
- The empty-name criterion for resources (FR-001) is unchanged.

## References

- DEC-016 (resolved by this ADR)
- FR-001, FR-004, FR-005, FR-006, NFR-003
- ADR-0009, ADR-0014, ADR-0015
- CTX-001

## History

- 2026-10-21: Created — validation paths for empty name, empty key and inverted range are mirrored into acceptance criteria.
