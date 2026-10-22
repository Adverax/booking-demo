# ADR-0021: NFR-001 is a correctness criterion, not a scalability target

**Status:** Accepted  
**Date:** 2026-10-22  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

NFR-001 requires that 50 simultaneous booking requests for the same slot yield exactly one success, and it was filed under the scalability category of the non-functional coverage because that was the closest fixed category. The design brief frames the criterion as a correctness test ("exactly one succeeds") and states no throughput, latency or load figure at all. Filing it under scalability marked that category as covered, which hides that no scale requirement exists. ADR-0017 already records that performance targets do not apply to the first release for the same reason.

## Decision

We will treat NFR-001 as a correctness release criterion about concurrent bookings and not as a scalability target, and we will leave the scalability category not applicable to the first release, with the reason that the brief states no load figure. This satisfies the explicit-coverage rule honestly: the 50-request test keeps its place as the proof of the no-double-booking invariant, and no category is reported as covered by a requirement that does not measure it.

## Alternatives considered

### reclassify_as_correctness (chosen)
Record NFR-001 as a correctness criterion and the scalability category as not applicable. Accurate, and consistent with ADR-0017. The cost is a one-time change of the category label in the model.

### keep_scalability_label
Keep NFR-001 under scalability as the closest fixed category. Rejected: it reports scalability as covered while no scale figure exists, which is a coverage claim ahead of evidence.

## Consequences

### Positive
- The coverage report no longer claims scalability coverage that does not exist.
- NFR-001 reads as what it is, the proof of INV-001 under concurrency.

### Negative
- A real scale requirement, if the studio states one later, needs a new NFR and a decision to reopen this category.

### Neutral
- The 50-request test, its number and its place in the release criteria are unchanged.
- The performance and scalability categories are both not applicable, with reasons ADR-0017 and this ADR.

## References

- DEC-018 (resolved by this ADR)
- NFR-001, INV-001, ADR-0017
- CTX-001

## History

- 2026-10-22: Created — NFR-001 is a correctness criterion and the scalability category is not applicable.
