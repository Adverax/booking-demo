# ADR-0001: Idempotency key repeated with a different payload

**Status:** Accepted  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

A booking request carries an idempotency key, and the design brief requires that repeating a request with the same key returns the original booking and creates nothing new (FR-006). The key is globally unique across the service. The brief defines only the case where the repeated request is identical. It does not say what happens when the key is repeated but the resource, the slot or the customer name differs. The caller is the studio's frontend acting on behalf of the customer, and a genuine retry from that frontend always resends the same payload. The service has no authentication (CON-006), so it cannot rely on caller identity to explain a mismatch.

## Decision

We will reject a booking request whose idempotency key already exists but whose resource, slot or customer name differs from the original booking, answering with a stable error `code`. This satisfies FR-006 because an identical repeat still returns the original booking unchanged, and a mismatching repeat is almost certainly a client bug that should surface immediately instead of being hidden behind a successful-looking response.

## Alternatives considered

### return_original
Always return the original booking and ignore the new payload. This keeps the contract simple, but it hides a client defect: the caller would believe the slot it just asked for was booked while the service holds a different booking. Rejected because the failure would be silent.

### reject_conflict (chosen)
Compare the request fields with the stored booking and answer with a stable error when they differ. Costs one more error code and one more test, adds no component, and makes a mismatch visible to the studio's frontend at once.

## Consequences

### Positive
- A client bug that reuses a key is detected at the first request instead of corrupting the customer's expectation.
- An identical retry keeps returning the original booking, so retry-safety is unchanged.

### Negative
- The service must compare the incoming payload with the stored booking on every repeated key.
- The API gains one more stable error code that the frontend has to handle.

### Neutral
- A named test is needed for this case, in line with the rule that every rule is covered by a named test.
- The exact error code name is chosen when the API is specified.

## References

- DEC-001 (resolved by this ADR)
- FR-006 (idempotent booking)
- TERM-008 (idempotency key)
- CON-006 (no authentication)

## History

- 2026-10-21: Created — reject a repeated idempotency key with a different payload so client errors surface immediately.
