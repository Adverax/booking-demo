# Wave 5 — 2026-10-22   (tag: wave-5)

## Shipped
- CARD-007 (feature): Added booking validation in a fixed order before any insert: an empty customer name or a slot start not on the UTC half hour is 400 validation_error (the temporary flooring of an unaligned start is removed), an unknown resource is 404 not_found, a slot outside the resource's working hours (read through the catalog) is 422 slot_outside_working_hours, and a slot starting before the injected clock's now (equal is accepted) is 422 slot_in_past unless the idempotency key is stored, in which case one key read returns the original booking (same payload) or 409 idempotency_key_conflict; the normal booking path makes no key read, and the bookSlot contract text states this order.   score 9.5 (2 cycles: 8.5 → 9.5)   FR-005
  merge 1dc0cdc (card commits 62ed082, 5883404), changelog 9b25192
- Deferred, not shipped: CARD-008 (availability search) — deferred before dispatch by user decision (Status: blocked, Blocked by: deferred to next wave; commit 349de59) because it shares internal/httpapi/**.go with CARD-007 and the scope gate counts a ready same-wave sibling as poached.

## Requirements closed
- FR-005 ✓ (goal-backward agent on main 9b25192, runtime probe of the real binary on a fresh SQLite file): AC-014, AC-015, AC-016, AC-017, AC-018, AC-019 (50 parallel distinct-key requests → 1 × 201, 49 × 409), AC-020, AC-037, AC-052, AC-057 ✓ by binary; AC-050, AC-051 ✓ by code reading (the binary uses the real clock; both are demonstrated by their named tests at the AC/EC/G gate).
- No PERF/LOAD NFR targeted this wave.

## Convergence
- ✅ converged for FR-005 — every target requirement of this wave's merged card demonstrated (API; backend-only, UI surface: none).
- The increment (Increment 3) is NOT complete: its availability half (CARD-008) is deferred, so the wave checkpoint stays pending (below).
- Cross-wave closure: FR-006's past-slot clause routed to CARD-007 in wave 4 now holds (AC-057 demonstrated; AC-055/056/058 still green with the past check in front of the insert) — noted in meta/releases/wave-4.md.

## Wave checkpoint
- Increment 3 checkpoint NOT evaluated — stays `status: pending` in meta/kanban/waves.yml: «for a resource with weekly working hours and one active booking, the availability for a seven-day range lists exactly the free half-hour slots, and a cancelled booking's slot reappears in the next query» needs the availability query of CARD-008, which is deferred. Not waived: waiving is a human risk decision and no operator was present; it is re-evaluated when CARD-008 merges.

## Known gaps / escalations
- No escalations this wave.
- Open decisions left open: DEC-015 (cross-aggregate read of working hours through COMP-002 recorded on capabilities), DEC-017 (cancel racing a booking of the same slot).
- Architect-station notes (not acted on — kanban never edits meta/architecture/): INV-004's check ref TestBookSlot_RepeatAfterCancelCreatesNoNewBooking names no existing test (both review cycles); trace write-back (done 7c) not performed.
- Contract text: api/openapi.yaml's bookSlot description was aligned with ADR-0020 in 5883404 (an "Order of checks" paragraph); schemas, enums and codes unchanged. Consequence stated there: a stored key reused with a different payload that also fails shape/resource/working-hours validation gets that validation error, not 409.
- Follow-up for CARD-008: booking's private insideWorkingHours restates catalog.Interval semantics; move it to a catalog.Resource method if availability needs the same rule.
- Declared in the failure matrix: worst case 3 statements × busyTimeout 5 s = 15 s per booking request exceeds shutdownTimeout 10 s (constant predates the card).
- Post-wave docs: forge:docs, forge:explain, forge:harness sync and the root README refresh were not run (no user-facing role docs in scope; no harness-relevant change: no new dependency, env var or migration).
- No gate overrides this wave.

## Migrations
- none

## Smoke test
- 2026-10-22, on main 9b25192 after the CARD-007 merge: `go build ./... && go test ./...` PASS (2s; merge-gate reuse not taken: target tree moved with the changelog commit, record cmd differs, working tree dirty); `make build test vet` PASS; `go test -count=1 -race ./...` PASS (5 packages); 162 top-level tests pass, 0 fail (77 subtests); TestBookSlot_50ConcurrentRequestsExactlyOneWins PASS; `make audit` govulncheck ok (0 findings), licenses ok (MIT ×5, BSD-3-Clause ×7, Apache-2.0 ×1).

## Metrics snapshot
- `capture.py --wave 5` exit 2, REFUSED: "the signal definitions registry is missing: meta/metrics/signal-definitions.yml" (ADR-0032). No snapshot written.
