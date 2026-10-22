# Wave 5 — 2026-10-22   (tag: wave-5)

## Shipped
- CARD-007 (feature): Added booking validation in a fixed order before any insert: an empty customer name or a slot start not on the UTC half hour is 400 validation_error (the temporary flooring of an unaligned start is removed), an unknown resource is 404 not_found, a slot outside the resource's working hours (read through the catalog) is 422 slot_outside_working_hours, and a slot starting before the injected clock's now (equal is accepted) is 422 slot_in_past unless the idempotency key is stored, in which case one key read returns the original booking (same payload) or 409 idempotency_key_conflict; the normal booking path makes no key read, and the bookSlot contract text states this order.   score 9.5 (2 cycles: 8.5 → 9.5)   FR-005
  merge 1dc0cdc (card commits 62ed082, 5883404), changelog 9b25192
- CARD-008 (feature): Added availability search: for a resource and a UTC date range of at most 14 days (both ends inclusive), the free 30-minute slots on the half hour are listed, derived from the resource's weekly working hours (an interval may end at 24:00) and the active bookings read with one autocommit SELECT, so a cancelled booking's slot is free again on the next query; a reversed range or one over 14 days is 400 validation_error before any booking is read, and an unknown resource is 404 not_found.   score 9.0 (1 cycle)   FR-003, FR-004
  merge 62cbcf8 (card commit debafba), changelog a7ab1e6
  Wave continuation: CARD-008 was deferred before dispatch by user decision (commit 349de59) so CARD-007 could run alone; after CARD-007 merged, the deferral was lifted with `unblock CARD-008` (47f7e15) — the automatic lift at the decompose wave kick-off is conditional on waves.duration, which this project does not set — and the card ran in the same open run as the last card of wave 5.

## Requirements closed
- FR-005 ✓ (goal-backward agent on main 9b25192, runtime probe of the real binary on a fresh SQLite file): AC-014, AC-015, AC-016, AC-017, AC-018, AC-019 (50 parallel distinct-key requests → 1 × 201, 49 × 409), AC-020, AC-037, AC-052, AC-057 ✓ by binary; AC-050, AC-051 ✓ by code reading (the binary uses the real clock; both are demonstrated by their named tests at the AC/EC/G gate).
- FR-003 ✓, FR-004 ✓ (goal-backward agent on main a7ab1e6, runtime probe of the real binary on a fresh SQLite file): AC-007 (2 slots 09:00/09:30), AC-008 (09:00 booked → only 09:30), AC-009, AC-010 (Saturday → 0 slots), AC-011 (14 days → 200, slots on both weeks), AC-012 (15 days → 400 validation_error), AC-013 (missing-id → 404 not_found), AC-054 (from 2 days after to → 400 validation_error) ✓ by binary.
- Relocated criteria: AC-025 (FR-007, cancelled slot free on the next query) ✓ and AC-042 (FR-006, repeat with K1 after cancel returns the cancelled original, 09:00 still free) ✓ by binary; the agent could not count rows directly (no list endpoint), so "no new booking" rests on the replay returning the original id — the named test TestBookSlot_RepeatAfterCancelCreatesNoNewBooking counts exactly 1 booking.
- No PERF/LOAD NFR targeted this wave.

## Convergence
- ✅ converged — every target requirement of this wave's two merged cards demonstrated: FR-005 (CARD-007), FR-003 and FR-004 (CARD-008) (API; backend-only, UI surface: none). Increment 3 is complete.
- Cross-wave closure: FR-006's past-slot clause routed to CARD-007 in wave 4 now holds (AC-057 demonstrated; AC-055/056/058 still green with the past check in front of the insert) — noted in meta/releases/wave-4.md.

## Wave checkpoint
- Increment 3 checkpoint PASSED (meta/kanban/waves.yml wave 5 → status: passed; evaluated after CARD-008 merged — it was left pending at the first close of wave 5): «for a resource with weekly working hours and one active booking, the availability for a seven-day range lists exactly the free half-hour slots, and a cancelled booking's slot reappears in the next query». Demonstration: the real binary on a fresh SQLite file (scratchpad), resource open Mon-Fri 09:00-11:00 and Sunday 22:00-24:00, range 2026-11-02..2026-11-08 (7 days): 24 free slots, the expected set exactly (4 per weekday, none on Saturday, Sunday 22:00..23:30 with the last slot ending at 24:00); after booking Monday 09:30 → 23 slots without it; after cancelling → 24 slots with Monday 09:30 again. Reversed range and 15-day range → 400 validation_error, unknown resource → 404 not_found. Collapses ASM-0002 (past slots listed as free — kept, no filter), ASM-0007, ADR-0004.

## Known gaps / escalations
- No escalations this wave (CARD-007 and CARD-008 both merged).
- Open decisions left open: DEC-015 (cross-aggregate read of working hours through COMP-002 recorded on capabilities), DEC-017 (cancel racing a booking of the same slot).
- Architect-station notes (not acted on — kanban never edits meta/architecture/): INV-004's check ref TestBookSlot_RepeatAfterCancelCreatesNoNewBooking now exists (added by CARD-008), closing the gap CARD-007's reviews reported; trace write-back (done 7c) not performed for CARD-007 or CARD-008.
- Contract text: api/openapi.yaml's bookSlot description was aligned with ADR-0020 in 5883404 (an "Order of checks" paragraph); schemas, enums and codes unchanged. Consequence stated there: a stored key reused with a different payload that also fails shape/resource/working-hours validation gets that validation error, not 409.
- Follow-up (not done): catalog.Interval's [Start,End)/24:00 reading is encoded three times — catalog validation, booking's private insideWorkingHours and availability's half-hour enumeration (CARD-008 review Minor F-001). Availability does not restate the containment predicate (it generates slots from the interval), but a shared catalog-owned operation used by both booking and availability needs a card allowed to edit internal/booking and internal/catalog (CARD-008's G-1 forbade it). CARD-008 review Minor F-002: the no-system-clock AST check matches only the identifier `time`.
- Procedure note: CARD-008's implementation agent staged internal/availability/README.md with plain `git add` after forge:commit's staging script refused it (outside the Touches globs, which name .go files only); no meta/ path entered the card commit.
- Declared in the failure matrix: worst case 3 statements × busyTimeout 5 s = 15 s per booking request exceeds shutdownTimeout 10 s (constant predates the card).
- Post-wave docs: forge:docs, forge:explain, forge:harness sync and the root README refresh were not run (no user-facing role docs in scope; no harness-relevant change: no new dependency, env var or migration).
- No gate overrides this wave.

## Migrations
- none

## Smoke test
- 2026-10-22, second run on main a7ab1e6 after the CARD-008 merge: `go build ./... && go test ./...` PASS (4s; merge-gate reuse not taken: target tree moved with the changelog commit, working tree dirty); `make build test vet` PASS; `go test -count=1 -race ./...` PASS (6 packages); merge gate on the card branch counted 185 top-level tests passed, 0 failed; TestBookSlot_50ConcurrentRequestsExactlyOneWins PASS; `make audit` govulncheck ok (0 findings), licenses ok (MIT ×5, BSD-3-Clause ×7, Apache-2.0 ×1); go.mod/go.sum unchanged.
- Tag: `wave-5` already exists (annotated, on 9b25192, from the first close of wave 5) — not moved or re-created (skip with a note, per the release-object step); CARD-008's merge 62cbcf8 and changelog a7ab1e6 sit after the tag.
- 2026-10-22, on main 9b25192 after the CARD-007 merge: `go build ./... && go test ./...` PASS (2s; merge-gate reuse not taken: target tree moved with the changelog commit, record cmd differs, working tree dirty); `make build test vet` PASS; `go test -count=1 -race ./...` PASS (5 packages); 162 top-level tests pass, 0 fail (77 subtests); TestBookSlot_50ConcurrentRequestsExactlyOneWins PASS; `make audit` govulncheck ok (0 findings), licenses ok (MIT ×5, BSD-3-Clause ×7, Apache-2.0 ×1).

## Metrics snapshot
- `capture.py --wave 5` exit 2, REFUSED: "the signal definitions registry is missing: meta/metrics/signal-definitions.yml" (ADR-0032). No snapshot written.
