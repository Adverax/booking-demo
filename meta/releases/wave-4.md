# Wave 4 — 2026-10-22   (tag: wave-4)

## Shipped
- CARD-005 (feature): Added idempotent booking replay with insert-then-classify: an empty idempotency key is rejected with 400 validation_error before anything is stored, and on a unique-index rejection one separate autocommit read of the booking under the key decides the answer — the same payload (resource, canonical slot start, customer name compared byte for byte) returns the original booking in its current state with 200 (also after cancellation and after the slot has started), a different payload is 409 idempotency_key_conflict, and no booking under the key is 409 slot_taken.   score 9.5 (1 cycle)   FR-006
  merge 5478983, changelog bbdf6cf
- Also on main since the wave-3 tag: CARD-004 (wave 3, merged 851e24a after its unblock) — cancel a booking and keep cancellation final (FR-007, FR-008).

## Requirements closed
- FR-006 — every acceptance criterion this card owns holds (goal-backward agent, runtime probe of the real binary on a fresh SQLite file: AC-021, AC-022, AC-038..AC-041, AC-043..AC-047, AC-053 ✓; AC-023 ✓ static read, its property test green at the AC/EC/G gate); AC-055, AC-056, AC-058 demonstrated by their named tests at the AC/EC/G gate (the repeat of a stored key returns the original after the slot has started).
  FR-level clauses that belong to later cards are NOT yet satisfied: AC-042's "slot still listed as free" needs the availability query (CARD-008, moved there by decompose); the "a key with no stored booking for a past slot is rejected as past" clause needs the past-slot check (CARD-007, AC-057). The goal-backward agent marked AC-055 ✗ on that FR-level clause; the AC's own scenario (identical repeat after the slot started returns B1) holds.
- FR-007 — cancellation works (AC-024, AC-026, AC-027, AC-048, EC-005 ✓, runtime probe); AC-025's "slot listed as free again" half needs CARD-008's availability query (the slot is free in the store: a new key booked it, 201).
- FR-008 ✓ (goal-backward): AC-028, AC-029, AC-030, AC-049 — a direct SQL UPDATE to active on the real database file was aborted by trigger booking_cancelled_is_final.
- No PERF/LOAD NFR targeted this wave.

## Convergence
- ⚠ NOT converged at FR level — every criterion owned by this wave's card holds; the open criteria are routed to already-planned cards, not new follow-ups: AC-042 and AC-025 (availability listing) → CARD-008 (wave 5); FR-006's past-slot clause (AC-057, ordering of the key lookup against the past check per ADR-0020) → CARD-007 (wave 5). Diagnosis (implementation gap vs misunderstood requirement) not asked: no interactive operator in this run; both are decomposition-planned, so recorded as implementation gaps owned by those cards.
- FR-008 converged (CARD-004, merged after the wave-3 tag).
- UI gaps: none — backend-only HTTP API (UI surface: none), no frontend in scope.

## Wave checkpoint
- Increment 2 checkpoint PASSED (meta/kanban/waves.yml wave 4 → status: passed): on fresh SQLite databases (each API test opens a fresh file), the replay scenarios pass — TestBookSlot_RepeatReturnsOriginalBooking, TestBookSlot_RepeatCreatesNothingNew, TestBookSlot_RepeatWithDifferentName/Slot/ResourceIsKeyConflict, TestBookSlot_RepeatAfterCancelReturnsCancelledOriginal, TestBookSlot_RepeatAfterSlotStartedReturnsOriginalBooking, TestBookSlot_PastSlotRepeatWithDifferentPayloadIsKeyConflict, TestCancelBooking_RepeatedCancelReturnsCancelledBooking, TestBooking_CancelledStaysCancelled, TestBooking_CannotReactivate, TestSchema_RejectsCancelledToActiveTransition — all --- PASS (go test -count=1); TestBookSlot_50ConcurrentRequestsExactlyOneWins (real binary) PASS.

## Known gaps / escalations
- No escalations this wave. Before dispatch the stale `escalated` entry of CARD-004 (done) was cleared with dispatcher_state.py unpark and escalations.md regenerated empty.
- Open decisions left open and recorded on the card: DEC-014 (whether a no-op replay re-emits its event — no event exists; a replay emits nothing), DEC-017 (concurrent same-key requests with different payloads — failure-matrix row states current behaviour).
- Follow-ups for CARD-007 (on its card at its start, from CARD-005's notes): keep replay classification ahead of the past-slot rejection and look the key up only on that rejection path (ADR-0020); `classify`'s "no booking under the key → slot_taken" must become the past-slot error on that path; the replay property generators send misaligned slot starts that the alignment rule will reject; consider pinning "no key read on the normal path" in fakeRepo (cycle-1 Minor).
- Trace write-back (done step 7c) not performed: meta/architecture/ is read-only for this run.
- Post-wave docs: forge:docs, forge:explain, forge:harness sync and the root README refresh were not run (no user-facing role docs in scope; no harness-relevant change: no new dependency, env var or migration).
- No gate overrides this wave.

## Migrations
- none merged by CARD-005 (CARD-004's 0002_cancelled_booking_is_final.sql landed on main after the wave-3 tag and is part of this tag's tree).

## Smoke test
- 2026-10-22, on main after the CARD-005 merge: `go build ./... && go test ./...` PASS (3s); `make build test vet` PASS; `go test -count=1 -race ./...` PASS (5 packages); `go generate ./...` no drift; 145 top-level tests pass, 0 fail (205 results incl. subtests); `make audit` govulncheck ok (0 findings), licenses ok (MIT ×5, BSD-3-Clause ×7, Apache-2.0 ×1).

## Metrics snapshot
- `capture.py --wave 4` exit 2, REFUSED: "the signal definitions registry is missing: meta/metrics/signal-definitions.yml" (ADR-0032). No snapshot written.
