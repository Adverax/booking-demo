# ADR-0017: Performance targets for the first release

**Status:** Accepted  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

The architecture covers the non-functional categories explicitly, and performance (latency and throughput targets) was surfaced as an open question because the brief states none. The only load-related requirement in the brief is the release test with 50 simultaneous booking requests for one slot where exactly one succeeds (NFR-001), and that is a statement about correctness under concurrency, not about speed. The service is a small backend behind the studio's gateway (CON-006) for a small business client, and no availability target exists either (DEC-007).

## Decision

We will treat performance targets as not applicable to the first release, with the recorded reason that the brief states no latency or throughput figure and NFR-001 verifies correctness under concurrency only, and we will not invent numbers. This satisfies the explicit-coverage rule for the category without placing an unfounded target on the delivery cards, and the question is reopened when the studio supplies a metric and a threshold.

## Alternatives considered

### not_applicable (chosen)
State that the category does not apply now and why. Honest and cheap, and it keeps the cards free of invented targets. The cost is that no performance regression would be caught until a target exists.

### target_defined
The operator names a metric and a threshold (for example p95 per endpoint at a stated request rate) to be formalized as an NFR. Rejected for now because no such figure exists in the brief and none was supplied.

## Consequences

### Positive
- No invented number reaches a card, a test or a review.
- The decision is explicit and revisitable instead of a silent omission.

### Negative
- Nothing in the first release detects a slowdown unrelated to correctness.

### Neutral
- The performance category is recorded as not applicable with this reason in the NFR coverage.
- A later NFR with a stated metric supersedes this decision.

## References

- DEC-006 (resolved by this ADR)
- NFR-001, CON-006, DEC-007
- CTX-001

## History

- 2026-10-21: Created — performance targets are not applicable to the first release.
