# ADR-0015: Sameness relations of resources, bookings and idempotency keys

**Status:** Accepted  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

The invariants speak of two bookings overlapping (INV-001), of a repeated request with the same idempotency key (INV-003, EC-002) and of the same booking being returned, but nothing in the model states how "the same" is decided. The validator reports this, because an implementation would invent the relation incrementally and several components depend on it: the unique indexes of ADR-0008, the replay classification of ADR-0011 and the payload comparison of ADR-0001. Most of the relation is already fixed by earlier decisions (the slot rule of ADR-0008, the payload fields of ADR-0001, the canonical instant of ADR-0013). Only the comparison of the two strings the caller controls, the idempotency key and the customer name, is open, and it was decided with the user to be exact.

## Decision

We will define sameness as follows, and state it in the glossary and in the acceptance criteria:
- Two resources are the same if they have the same resource id.
- Two bookings occupy the same slot if they have the same resource id and the same slot start, the start being the canonical UTC instant of ADR-0013; this is the overlap of INV-001, since slots are fixed 30-minute aligned slots.
- Two idempotency keys are the same if their strings are byte-for-byte equal as received, with no trimming and no case folding.
- A repeated booking request has the same payload as the stored booking if it names the same resource id, the same slot start and a customer name that is byte-for-byte equal to the stored one; any difference is the key conflict of ADR-0001.

This satisfies the sameness warnings of the model because each relation is stated once, with the identifying fields and the normalization (none for strings, the canonical instant for times).

## Alternatives considered

### exact_comparison (chosen)
Strings are compared exactly as received. Predictable and easy to test; the studio's frontend must resend the same key and the same name on a retry.

### normalized_comparison
Compare the customer name after trimming and case folding. Rejected: it needs a normalization rule that must be specified and tested, and it can merge names that differ meaningfully or split keys that differ only in form, without any requirement in the brief for it.

## Consequences

### Positive
- Every sameness relation of the model has one stated definition.
- The replay classification, the unique indexes and the payload comparison all use the same relations.

### Negative
- A retry with a trivially different spelling of the name is a key conflict, which the frontend must avoid.
- Keys that differ only by case or whitespace are different keys.

### Neutral
- Handling of an empty or oversized key is not decided here (DEC-016).
- The glossary gains terms for the slot, the idempotency key and the payload comparison.

## References

- ADR-0001, ADR-0008, ADR-0011, ADR-0013
- INV-001, INV-003, EC-002
- DEC-035 (resolved by this ADR)
- CTX-001

## History

- 2026-10-21: Created — sameness of resources, bookings, keys and payloads is defined, with exact string comparison.

## Rules
```yaml
- id: ADR-0015/R1
  statement: Idempotency keys and customer names are compared byte for byte as received, and two bookings occupy the same slot only if resource id and canonical slot start are equal
  scope: {contexts: [CTX-001]}
  check: {kind: review-lens}
  severity: mandatory
```
