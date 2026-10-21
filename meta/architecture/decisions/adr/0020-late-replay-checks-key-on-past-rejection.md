# ADR-0020: Replay of a key after the slot has started

**Status:** Accepted  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

A repeated booking request with the same idempotency key must return the original booking and create nothing new (FR-006, INV-003), and a booking is accepted only for a slot whose start is not earlier than the current time (FR-005, INV-002, ADR-0016). A customer whose first request succeeded but whose response was lost may repeat it after the slot has started. Both rules then apply to the same request: replaying means returning the original, while the slot is now in the past. The ADR set did not say which wins. ADR-0011 resolves a repeat by inserting first and classifying a unique violation by reading the stored booking, and it keeps the common path free of an extra read. If the late repeat were rejected as a past slot, the customer would conclude that the booking does not exist although it does.

## Decision

We will keep the order of ADR-0011 (validate, insert, classify a violation), and when validation rejects a request because the slot is in the past, we will look up the idempotency key before answering: a stored booking with the same payload is returned as the original booking, a stored booking with a different payload is the key conflict of ADR-0001, and no stored booking means the request is rejected as a past slot. This satisfies FR-006 and INV-003 for the late retry that matters most, and keeps INV-002 for every request that has no stored booking, with the extra read confined to the rare rejection path.

## Alternatives considered

### validate_first_replay_check_on_past_rejection (chosen)
Validation and insert as before, with a key lookup only when the rejection is a past slot. No read on the normal path and ADR-0011 unchanged; a small special case that needs its own acceptance criterion and test. This alternative was added during the decision and was not in the original list.

### lookup_first
Read the key before any validation on every request. Uniform and the usual idempotency pattern, but it adds a read to every new booking and partly reverses the reasoning of ADR-0011. Rejected.

### validate_first
Reject a late repeat as a past slot. The simplest, but the customer would be told that a booking that exists failed. Rejected.

## Consequences

### Positive
- A late retry of a successful booking returns the original booking instead of a misleading error.
- The normal booking path stays free of the extra read.

### Negative
- One more branch on the rejection path, with its own acceptance criterion and test.
- A past-slot rejection now depends on a lookup of the key, so its outcome can differ for a repeated key.

### Neutral
- The key lookup is a single autocommit read, in line with ADR-0012.
- A repeat of a key with a different payload for a past slot reports the key conflict, not the past-slot error.

## References

- DEC-012 (resolved by this ADR)
- FR-005, FR-006, INV-002, INV-003
- ADR-0001, ADR-0011, ADR-0012, ADR-0016
- CTX-001

## History

- 2026-10-21: Created — a past-slot rejection checks the idempotency key and returns the original booking when the payload matches.
