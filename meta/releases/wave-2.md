# Wave 2 — 2026-10-22   (tag: wave-2)

## Shipped
- CARD-003 (feature): Added the service bootstrap (environment configuration, store open with migrations, HTTP server with timeouts and graceful shutdown), minimal resource creation, and booking of one slot as a single autocommit insert where the slot uniqueness index picks exactly one winner among concurrent requests and the others get a slot_taken conflict.   score 9.0 (1 cycle)   FR-005, FR-001
  merge 4a8badf

## Requirements closed
- FR-005 ✗ partial (goal-backward, live probe of the real binary) — AC-014, AC-018, AC-019, AC-037, EC-001 ✓; AC-051 ✓ only incidentally; AC-015, AC-016, AC-017, AC-020, AC-050, AC-052, AC-057 ✗ — planned for CARD-007.
- FR-001 ✗ partial — AC-001 ✓, AC-059 ✓ (incidental, no validator yet); AC-002, AC-003, AC-034, AC-035, AC-036, AC-060, AC-061 ✗ — planned for CARD-006.
- No PERF/LOAD NFR targeted this wave (NFR-001's release criterion AC-031 is CARD-009's).

## Convergence
- ⚠ NOT converged — unmet: FR-005 (follow-up: CARD-007), FR-001 (follow-up: CARD-006). Every gap is a slice the decomposition already assigned to a later card; none is unassigned. Triage default (a) implementation gap — untriaged by a human (no interactive operator in this run).
- UI gaps: none — backend-only HTTP API (UI surface: none), no frontend in scope.
  ↳ FR-001 converged in wave 3 (2026-10-22)

## Wave checkpoint
- Increment 1 checkpoint PASSED: `go test -run TestBookSlot_50ConcurrentRequestsExactlyOneWins ./...` on a fresh SQLite file, real binary as a subprocess — 1 × 201, 49 × 409 slot_taken, 0 lock/busy errors (also 20/20 with `-count=20`). Collapses ADR-0006, ADR-0008, ADR-0012, ASM-0006, ADR-0013, NFR-001 (uncertainty only). ADR-0012's `_txlock` fallback was not needed.

## Known gaps / escalations
- No escalations. Deferral captured to backlog: GetBooking (FR-007/AC-029) has no owning card.
- Trace write-back (done step 7c) not performed: meta/architecture/ is read-only for this run.
- Post-wave docs: forge:docs, forge:explain, forge:harness sync and the root README refresh were not run (see run report); the harness would gain a run target for the new BOOKING_DB_PATH/BOOKING_ADDR variables, and the Makefile is not editable in this run.

## Migrations
- none (CARD-003 adds no migration; schema 0001 from wave 1)

## Smoke test
- 2026-10-22, on main: `go build ./... && go test ./...` PASS (4s); uncached and `-race` runs PASS; `make build test vet` PASS; `make audit` govulncheck ok, licenses ok (MIT ×5, BSD-3-Clause ×7, Apache-2.0 ×1; 0 findings).

## Metrics snapshot
- `capture.py --wave 2` exit 2, REFUSED: "the signal definitions registry is missing: meta/metrics/signal-definitions.yml" (ADR-0032). No snapshot written.
