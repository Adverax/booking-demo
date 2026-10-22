# Wave 3 — 2026-10-22   (tag: wave-3)

## Shipped
- CARD-006 (feature): Added full validation of resource creation (a name, weekly working hours with at most one half-hour-aligned interval per weekday inside one UTC day, where 24:00 is allowed only as an interval end), answered with a 400 validation_error before anything is stored, plus listing resources and reading one by id with a 404 not_found for an unknown id.   score 9.0 (1 cycle)   FR-001, FR-002
  merge 4c2cd96

## Requirements closed
- FR-001 ✓ (goal-backward, static read of main): validation in internal/catalog/catalog.go (NewResource), Create validates before the insert, 400 validation_error at the HTTP layer; AC-002, AC-003, AC-034, AC-035, AC-036, AC-059, AC-060, AC-061 demonstrated by their named tests at the AC/EC/G gate.
  ↳ closes the FR-001 gap recorded in wave 2.
- FR-002 ✓ (goal-backward): list and read by id through catalog → store → generated handlers; 404 not_found for an unknown id (AC-004, AC-005, AC-006 demonstrated).
- FR-007, FR-008 (CARD-004) NOT closed this wave: CARD-004 escalated, unmerged.
- No PERF/LOAD NFR targeted this wave.

## Convergence
- ⚠ NOT converged for the wave's declared scope — CARD-004 (FR-007 cancellation, FR-008 cancellation is final) did not merge: escalated by the scope gate (see below). Everything that merged converged: FR-001 and FR-002 ✓.
- UI gaps: none — backend-only HTTP API (UI surface: none), no frontend in scope.
  ↳ FR-008 converged in wave 4 (2026-10-22): CARD-004 merged after unblock (851e24a); FR-007 cancellation holds, its availability half waits for CARD-008.

## Wave checkpoint
- Wave 3 carries no checkpoint entry in meta/kanban/waves.yml (Increment 2 closes at wave 4, Increment 3 at wave 5).

## Known gaps / escalations
- CARD-004 escalated — station: decompose. The scope gate returned RUNAWAY before the first review: its diff covered 2 of 3 Touches globs (internal/httpapi/**.go, internal/store/*.go) of the ready same-wave sibling CARD-006, the very sibling the conflict graph had serialized behind it (conflict.hotspots is empty, so shared adapter globs count as poaching). The implementation (e39372e: cancel and read booking, migration 0002 trigger, 8 AC tests + 2 property tests) is kept unreviewed in worktree ../booking-demo-CARD-004. A dry-run merge onto the new main shows content conflicts in internal/httpapi/unimplemented.go, internal/store/README.md, internal/store/repository.go and internal/store/repository_test.go. Route: a human decision at the decompose station, then `/kanban unblock CARD-004` and `/kanban review CARD-004` (rebase onto main, keep both sides, re-run all gates).
- Procedure gap observed (not a code defect): the scope gate's poaching rule fires on any first card of a predicted-conflict pair whose Touches overlap, so serialization by the conflict graph and the poach rule contradict each other unless conflict.hotspots is seeded; the same shape escalated CARD-001 in wave 1.
- Trace write-back (done step 7c) not performed: meta/architecture/ is read-only for this run.
- Post-wave docs: forge:docs, forge:explain, forge:harness sync and the root README refresh were not run (no user-facing role docs in scope for this run; Makefile and root docs not edited).
- No gate overrides this wave.

## Migrations
- none merged (CARD-006 adds no migration; CARD-004's 0002_cancelled_booking_is_final.sql is on its unmerged branch).

## Smoke test
- 2026-10-22, on main after the CARD-006 merge: `go build ./... && go test ./...` PASS (3s, uncached); `make build test vet` PASS; 96 top-level tests pass, 0 fail; `make audit` govulncheck ok, licenses ok (MIT ×5, BSD-3-Clause ×7, Apache-2.0 ×1; 0 findings).

## Metrics snapshot
- `capture.py --wave 3` exit 2, REFUSED: "the signal definitions registry is missing: meta/metrics/signal-definitions.yml" (ADR-0032). No snapshot written.
