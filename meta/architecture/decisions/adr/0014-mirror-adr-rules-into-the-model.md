# ADR-0014: Mirroring the ADR-derived rules into invariants and acceptance criteria

**Status:** Accepted  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

The model was extracted from the design brief before ADR-0001 to ADR-0013 existed. Those ADRs created behaviour the invariants and acceptance criteria do not yet describe: a repeated key with a different payload is rejected (ADR-0001), a repeated key after cancellation returns the cancelled original (ADR-0002), a repeated cancel is a success (ADR-0003), working hours have a fixed shape (ADR-0004), a cancelled booking cannot be reactivated in the schema (ADR-0010), and a violated insert is classified by reading the stored booking (ADR-0011). The critics found that INV-003 and EC-003 still say a repeated key returns the original without the identical-payload qualification, that INV-005 is weaker than ADR-0004, and that no acceptance criterion covers these paths, although every rule must have a named test (NFR-003). Cards are cut from the acceptance criteria, so any gap there is inherited by the cards.

## Decision

We will carry these rules into the model now, before cards are cut: the rules are written as raw requirements, and the domain extraction phase formalizes them in delta mode, rewording INV-003, INV-005 and EC-003 where they understate the accepted ADRs, adding acceptance criteria for each ADR-derived path and the glossary terms of ADR-0015. This satisfies NFR-003 because every ADR-derived rule gets a named test case, and it keeps the intake path (raw requirements, then extraction) instead of editing the requirements directly.

## Alternatives considered

### mirror_now (chosen)
Formalize the ADR-derived rules through the extraction phase in delta mode before decomposition. Costs one more extraction run and a review of its output.

### defer_to_later
Keep the requirements as extracted and add acceptance criteria when cards are cut or tests are named. Rejected: cards would be cut from incomplete criteria, and the missing paths would only surface when a test cannot be named.

## Consequences

### Positive
- Cards inherit complete acceptance criteria, with a named test for every ADR-derived path.
- The invariants no longer understate or contradict the accepted ADRs.

### Negative
- One more extraction run, and its output needs review.
- More acceptance criteria means more planned test names that do not exist yet.

### Neutral
- Open decisions about input validation (DEC-016), concurrency scenarios (DEC-017) and the order of checks for a late replay (DEC-012) are not part of this change.
- The delta must keep existing identifiers stable and must not reuse retired ones.

## References

- DEC-013 (resolved by this ADR)
- ADR-0001, ADR-0002, ADR-0003, ADR-0004, ADR-0010, ADR-0011, ADR-0015
- INV-003, INV-005, EC-003, NFR-003
- CTX-001

## History

- 2026-10-21: Created — ADR-derived rules are mirrored into invariants and acceptance criteria through a delta extraction before decomposition.
