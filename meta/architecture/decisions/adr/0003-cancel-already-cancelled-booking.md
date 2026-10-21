# ADR-0003: Cancelling an already cancelled booking

**Status:** Accepted  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

A booking can be cancelled by anyone who knows its id, and a cancelled booking frees its slot immediately and stays cancelled (FR-007, FR-008). The caller is the studio's frontend, which may retry a request after a network failure without knowing whether the first attempt reached the service. The brief does not say what a second cancel of the same booking should return. Repeating the cancel does not change any state, because the booking is already cancelled.

## Decision

We will treat cancelling an already cancelled booking as a success and return the cancelled booking again. This satisfies FR-007 and keeps FR-008 intact because the state does not change and the booking is never reactivated, while a retried cancel from the frontend behaves the same as the first one.

## Alternatives considered

### idempotent_success (chosen)
A repeated cancel returns the cancelled booking as success. The frontend can retry safely and needs no special handling.

### conflict_error
Reject the repeated cancel with a stable error code. Rejected: it forces the frontend to tell a harmless "already cancelled" apart from a real failure, for a request that changes nothing.

## Consequences

### Positive
- Retrying a cancel is always safe.
- The frontend needs no special case for "already cancelled".

### Negative
- A mistaken second cancel is not flagged; the caller learns nothing beyond the current state.

### Neutral
- A named test is needed for the repeated-cancel case.
- The response for a repeated cancel is identical in shape to the first cancel response.

## References

- DEC-003 (resolved by this ADR)
- FR-007 (cancellation), FR-008 (cancellation is final)
- CMD-003

## History

- 2026-10-21: Created — a repeated cancel is an idempotent success.
