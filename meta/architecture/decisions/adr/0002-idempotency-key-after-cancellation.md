# ADR-0002: Idempotency key repeated after the booking was cancelled

**Status:** Accepted  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

A booking can be cancelled, and cancellation is final: a cancelled booking is never reactivated and a new booking is needed (FR-008, INV-004). The idempotency key of a booking is globally unique across the service, and repeating a request with the same key returns the original booking (FR-006, INV-003). The brief does not say what a repeat of that key should do once the original booking has been cancelled. A delayed network retry of the original request can reach the service after the customer has already cancelled, so the choice decides whether such a retry silently creates a new booking.

## Decision

We will answer a repeated idempotency key with the original booking in its current state, including status `cancelled`, and we will not create a new booking from that key; a new booking requires a new key. This satisfies FR-006 and FR-008 because a key keeps identifying exactly one booking for its whole life, and a late retry can never resurrect a booking the customer cancelled.

## Alternatives considered

### return_cancelled_original (chosen)
The repeated key returns the original booking with status `cancelled`. The key stays bound to one booking forever, which keeps the rule simple to state and to test.

### allow_new_booking
Treat the key as free once its booking is cancelled and create a new booking. Rejected: it breaks the one-key-one-booking rule and lets a delayed retry of the original request recreate a booking right after the customer cancelled it.

## Consequences

### Positive
- A key identifies exactly one booking for its whole life.
- A delayed retry after cancellation cannot recreate the cancelled booking.

### Negative
- A customer who wants to book the same slot again after cancelling must use a new key, so the frontend must generate one per booking intent.
- Keys of cancelled bookings must be kept and cannot be reclaimed.

### Neutral
- A named test is needed for the repeat-after-cancel case.
- Storage keeps a uniqueness constraint on the key across all bookings, active or cancelled.

## References

- DEC-002 (resolved by this ADR)
- FR-006 (idempotent booking), FR-008 (cancellation is final)
- INV-003, INV-004

## History

- 2026-10-21: Created — a repeated key returns the cancelled original and never creates a new booking.
