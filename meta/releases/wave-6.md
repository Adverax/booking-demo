# Wave 6 — 2026-10-22   (tag: wave-6)

## Shipped
- CARD-009 (feature): Added the release criteria suite, run against the real binary on a fresh database: exactly one of 50 simultaneous bookings of one free slot wins (TestRelease_ConcurrencyExactlyOneWinner), the whole API works end to end — create a resource, list availability, book, repeat the booking and cancel (TestRelease_FullAPIEndToEndOnFreshDatabase) — and every test name planned in the requirements model exists as a Go test function (TestRelease_EveryRuleHasANamedTest); plus a check that a malformed path parameter is a 400 validation_error, and a sample systemd unit and environment file (deploy/) for running the single binary as one instance on a local disk.   score 9.0 (1 cycle, est 1c)   NFR-001, NFR-002, NFR-003
  merge 6f87458 (card commit 041faee), changelog 24184e0
  Files: cmd/booking/release_test.go (new), cmd/booking/main_test.go (the 50-request body moved unchanged into a shared helper), internal/httpapi/server_test.go (one ASM-0005 case), cmd/booking/README.md, deploy/booking.service, deploy/booking.env.example, deploy/README.md (new). No production code changed; go.mod/go.sum unchanged (gopkg.in/yaml.v2 reused).

## Requirements closed
- NFR-001 ✓ (goal-backward agent on main 24184e0, runtime probe of the real binary on a fresh SQLite file): 50 simultaneous POST /bookings with distinct keys for one free future slot → 1 × 201, 49 × 409 slot_taken, no 5xx. A correctness criterion (ADR-0021), not a scale target; no PERF/LOAD threshold exists, so there is no ≈ verdict.
- NFR-002 ✓ (same agent, real binary, fresh SQLite file): create resource 201 → availability 200 (slot listed) → book 201 active → repeat 200 identical body → availability without the slot → cancel 200 cancelled → get 200 cancelled → availability with the slot again.
- NFR-003 ✓ (same agent, independent of the repository's test): 66 planned `test:` names in meta/architecture/requirements.yml, 66 found as `func Name(t *testing.T)`, 0 missing, 0 invalid names. TestRelease_EveryRuleHasANamedTest reports the same "66 planned test names, 190 Go test functions, 0 missing".

## Convergence
- ✅ converged — every target requirement of this wave's merged card demonstrated: NFR-001, NFR-002, NFR-003 (API; backend-only, UI surface: none). Increment 4 is complete; every card of the board is done.

## Wave checkpoint
- Increment 4 checkpoint PASSED (meta/kanban/waves.yml wave 6 → status: passed): «`go test -run TestRelease ./...` passes: the full API works end to end against a fresh database, exactly one of 50 concurrent bookings wins, and every rule has at least one named test». Demonstration on main 24184e0: `go test -count=1 -run TestRelease -v ./...` → the three TestRelease_* PASS (cmd/booking ok; the other packages have no matching test). Collapses ASM-0005, DEC-031, NFR-002, NFR-003. Per the rollback line, the first release is the point of no return and is crossed only after this checkpoint passes — nothing was pushed or deployed by this run.

## Known gaps / escalations
- No escalations this wave.
- Scope gate GROWN (cycle 1): 4 of 7 changed files outside Touches (deploy/booking.service, deploy/booking.env.example, deploy/README.md, cmd/booking/README.md); the reviewer judged the excess necessary (the card requires the deploy sample and leaves its path open). Decompose note: Touches did not predict deploy/**. Thresholds unchanged.
- Review Minors not fixed (non-gating): F-001 test discovery ignores //go:build constraints (no such file today); F-002 the coverage denominator is planned names, not rules (66 AC/EC = 66 names today; a rule without a `test:` key would be invisible); F-003 the sample env file binds all interfaces while DEC-011 is open.
- Architect-station notes (not acted on — kanban never edits meta/architecture/): deploy/ is mapped to no component in trace.yml; trace write-back (done 7c) not performed for CARD-009.
- Open decisions left open: DEC-029 (single instance documented in the unit, not enforced), DEC-007/DEC-010 (no uptime, logging or metrics settings in the unit), DEC-017 (only identical concurrent requests are proven), DEC-011.
- The systemd unit was not checked with `systemd-analyze verify` (macOS host, no systemd).
- Procedure notes: forge:commit's staging script refused the deploy/ files (outside the Touches globs) and the implementation agent staged them explicitly; no meta/ path entered the card commit. CARD-009's first `[Merge gate]` record named the card branch's tree; a corrected record with main^{tree} after the changelog commit follows it on the card.
- Post-wave docs: forge:docs, forge:explain, forge:harness sync and the root README refresh were not run (no user-facing FR in this wave; no new dependency, env var or migration; the deploy sample is not harness territory).
- No gate overrides this wave.

## Migrations
- none

## Smoke test
- 2026-10-22, on main 24184e0 after the CARD-009 merge: `go build ./... && go test ./...` PASS (2s; merge-gate reuse not taken: working tree dirty); `make build test vet` PASS; `go test -count=1 -race ./...` PASS (6 packages); merge gate on the card branch counted 190 top-level tests passed, 0 failed; `make audit` govulncheck ok (0 findings), licenses ok (MIT ×5, BSD-3-Clause ×7, Apache-2.0 ×1); go.mod/go.sum unchanged.
