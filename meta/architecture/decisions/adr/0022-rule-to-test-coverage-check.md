# ADR-0022: How rule-to-test coverage is verified

**Status:** Accepted  
**Date:** 2026-10-22  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

The first release requires that every rule has at least one named test (NFR-003, AC-033). The model names a planned test for every acceptance criterion, edge case and invariant check, and the delivery cards implement those tests. Nothing records who verifies that every named test actually exists once the code is written, and the trace matrix maps NFR-003 to production components that do not perform that check. Without a mechanism the requirement can be reported as met by the model alone, and a renamed or forgotten test is found only by hand. Test names must be valid Go test function names (the rename of the earlier property-test prefix).

## Decision

We will verify rule-to-test coverage with a Go test in the release test set, named `TestRelease_EveryRuleHasANamedTest`, that reads the planned test names from the requirements model and fails when a named test does not exist as a Go test function in the repository. This satisfies NFR-003 and AC-033 because the check runs with the ordinary test command and the delivery gates, so a missing or renamed test fails the build instead of passing silently, and the release test of NFR-001 stays beside it in the same set.

## Alternatives considered

### record_verification_mechanism (chosen)
Name the mechanism in an ADR and implement it as a Go release test. The check lives with the code and runs everywhere the tests run. It couples a test to the model files and needs a way to read the model's YAML.

### keep_outside_model
Leave the mechanism to the delivery card without a recorded decision. Rejected: the coupling between code and model files, and the choice of how the model is read, are design decisions that should not be taken implicitly inside one card.

## Consequences

### Positive
- A missing, renamed or invalid test name fails the build.
- NFR-003 is verified by an executable check and not by the model's own claim.

### Negative
- A code test depends on the layout of the model files under `meta/architecture/`, so a change of the model schema can break it.
- Reading YAML in Go may add a direct dependency, which must carry an allowlisted licence.

### Neutral
- The check verifies that a named test exists, not that it is meaningful; review still judges that.
- It runs in the unit tier unless a separate release tier is introduced later.

## References

- DEC-031 (resolved by this ADR)
- NFR-003, AC-033, NFR-001, AC-031
- CTX-001

## History

- 2026-10-22: Created — a Go release test verifies that every named test exists.
