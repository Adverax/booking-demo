# ADR-0019: Aligning evidence citations across trace rows

**Status:** Accepted  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

The trace matrix links each requirement to capabilities, components, ADRs and tests, and the delivery cards inherit their architecture context from it. Two rows disagreed with their siblings and with their own acceptance criteria: the FR-005 row did not cite ADR-0004 although a booking relies on the working-hours shape that FR-003 already cites it for, and the NFR-001 row omitted the HTTP API component although its release test sends 50 HTTP requests. Earlier rounds of the same kind of inconsistency (FR-004 and FR-008) were treated as gaps and fixed.

## Decision

We will add ADR-0004 to the FR-005 row and the HTTP API component to the NFR-001 row, through the synthesis phase in trace-only mode. This keeps the rows consistent with each other and with their acceptance criteria, so a card cut from either row receives the same architecture context its siblings do.

## Alternatives considered

### align_row_citations (chosen)
Add the missing citations. A small, mechanical change that removes a recurring inconsistency.

### leave_rows_as_is
Leave the rows, since the evidence is reachable through other rows. Rejected: a card inherits its context from its own row, so a transitive reference does not reach it.

## Consequences

### Positive
- Rows no longer contradict their siblings or their acceptance criteria.
- Cards inherit the complete architecture context for the requirement.

### Negative
- A further trace-only synthesis pass is needed, which touches only two rows and stale notes.

### Neutral
- Notes in the trace that still name now-resolved decisions as open are refreshed in the same pass.

## References

- DEC-034 (resolved by this ADR)
- FR-005, NFR-001, FR-003
- ADR-0004
- CTX-001

## History

- 2026-10-21: Created — missing ADR and component citations are added to the FR-005 and NFR-001 trace rows.
