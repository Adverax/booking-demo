# CARD-004: Cancel a booking and keep cancellation final

**Status:** blocked
**Priority:** P1
**Category:** feature
**Estimate:** 1c
**Complexity:** architectural
**Closure:** closed
**Revision pending:** false
**Skill:** golang-pro
**TDD:** —
**Branch:** card/004-cancel-booking-final
**Worktree:** /Users/romanmiakotin/go/src/github.com/adverax/booking-demo-CARD-004
**Source:** meta/architecture/handoff.md#increment-2/cancellation
**Idea:** — (no match in backlog-scored.yml)
**Wave:** 3
**Depends on:** CARD-003
**Touches:** internal/booking/**.go, internal/httpapi/**.go, internal/store/*.go, internal/store/migrations/**
**UI surface:** none
**Readiness:** 7/7
**Review score:** —
**Started:** 2026-10-22T07:17:36Z
**Closed:** —
**Actual:** —
**Merge commit:** —
**Blocked by:** escalated — decompose: scope gate RUNAWAY — poaches serialized sibling CARD-006's Touches (0.67)

## What to implement

Increment 2, the cancellation half of "Idempotency and cancellation".

- Cancel a booking by its id, with no customer identity check (FR-007, CON-006). Cancelling sets the status to cancelled with a single autocommit UPDATE (ADR-0012), which frees the slot for a new booking because the active-slot unique index of ADR-0008 covers active bookings only.
- A repeated cancel of an already cancelled booking succeeds and returns the cancelled booking, with the same response shape as the first cancel (ADR-0003, AC-048, EC-005). An unknown id is a not-found error with a stable code (AC-027).
- Read a booking by id (AC-029), so a cancelled booking can be observed as cancelled.
- Cancelled is final (FR-008, INV-004): keep the domain guard, and add a versioned migration with a schema trigger on the booking status that aborts any update from cancelled to active (ADR-0010, AC-049). CARD-002 (done) shipped `internal/store/migrations/0001_create_resource_and_booking.sql`, embedded and applied by `store.Open`; the trigger is the next versioned file after it. The violation maps to a conflict error with a stable code (AC-030). A new booking of the same slot creates a new booking with a new id (AC-028).

Moved to CARD-008: AC-025 (TestCancelBooking_FreesSlotImmediately) asserts the freed slot through the next availability query, and availability (COMP-003) is built in Increment 3; the Increment 3 checkpoint ("a cancelled booking's slot reappears in the next query") is where it is verified.

Open decisions touching this card (current as of this decompose run; supersedes the list in Worktree notes, which the procedure carries over unchanged). Do not answer them here:
- DEC-017: a cancel racing a booking of the same slot is not covered by this card's ACs.
- DEC-014: whether a no-op repeated cancel emits its domain event again (EVT-003).
- DEC-011: a leaked booking id lets anyone cancel; no mitigation is decided and CON-006 stands.
Resolved since the last cut: DEC-012 (ADR-0020, a past-slot rejection checks the idempotency key first) concerns booking replay only and does not touch cancellation.
Revised since the last cut: ADR-0004 (24:00 interval end) and ADR-0006 (`_txlock` exists) do not change cancellation. Property-test names now carry the `Test` prefix Go requires (`TestProperty_...`).

## Non goals

- NG-1: Any change to who may cancel (CON-006) or to the cancel rule for active bookings (FR-007 out of scope)
- NG-2: Any public route that reactivates a booking (none exists, ASM-0001; FR-008 out of scope)

## Acceptance criteria

- AC-024: Given an active booking for Monday 09:00 made by customer "Ann", when the booking is cancelled by its id, then the booking status equals cancelled — test: TestCancelBooking_MarksCancelled
- AC-026: Given an active booking made by customer "Ann", when the cancel request carries no customer identity, only the booking id, then the booking is cancelled — test: TestCancelBooking_AnyoneWithIdCanCancel
- AC-027: Given no booking with id "missing-id" exists, when that booking is cancelled, then the response is a not-found JSON error with a stable code field — test: TestCancelBooking_UnknownIdNotFound
- AC-048: Given a booking B1 that was already cancelled, when B1 is cancelled again by its id, then the response is a success carrying B1 with its original id and status cancelled — test: TestCancelBooking_RepeatedCancelReturnsCancelledBooking
- AC-028: Given a cancelled booking B1 for Monday 09:00, when a customer books Monday 09:00 with a new idempotency key, then a new booking B2 with a different id is created — test: TestBooking_NewBookingAfterCancelCreatesNewId
- AC-029: Given a cancelled booking B1 for Monday 09:00 and B2 now active for that slot, when B1 is read, then B1 status equals cancelled — test: TestBooking_CancelledStaysCancelled
- AC-030: Given a cancelled booking B1, when any request attempts to set B1 active again, then the response is a conflict JSON error with a stable code field and B1 status stays cancelled — test: TestBooking_CannotReactivate
- AC-049: Given a cancelled booking B1 for Monday 09:00 whose slot has no active booking, when a statement updates the status of B1 to active directly in the database, then the update is aborted by the schema and B1 status stays cancelled — test: TestSchema_RejectsCancelledToActiveTransition

## Engineering constraints

- EC-005: Cancelling a booking any number of times leaves it cancelled and returns it as a success every time
  test: TestProperty_Booking_RepeatedCancelIsIdempotent
- EC(ADR-0010/R1): for any cancelled booking and any sequence of status updates attempted on it, through the domain code or directly in the database, its status stays cancelled
  test: TestProperty_Booking_CancelledNeverBecomesActive

## Guardrails

- G-1: Do not edit `internal/catalog/**.go` — owned by CARD-006 this wave

## System contract

- ADR-0010/R1 — A booking whose status is cancelled is never set active again; the schema rejects the transition (check: test: TestBooking_CannotReactivate)
- ADR-0012/R1 — The booking write path uses single autocommit statements and no transaction that reads and then writes (check: review-lens)
- ADR-0013/R1 — Instants are stored as integer UTC epoch seconds and the current time is read only through the injected clock (check: review-lens)
- ADR-0015/R1 — Idempotency keys and customer names are compared byte for byte as received, and two bookings occupy the same slot only if resource id and canonical slot start are equal (check: review-lens)
- CON-007 — The booking write path uses single autocommit statements and no transaction that reads and then writes. (check: review-lens)
- CON-008 — Instants are stored as integer UTC epoch seconds and converted to RFC 3339 only at the API boundary; the current time is obtained through an injectable clock. (check: review-lens)
- INV-001 — Two active bookings of the same resource never overlap, also under concurrent requests. (check: test: TestBookSlot_50ConcurrentRequestsExactlyOneWins, TestBookSlot_RejectsAlreadyBookedSlot, TestBookSlot_SameStartOnAnotherResourceIsNotTheSameSlot, TestBookSlot_SlotTakenStoresNothingUnderNewKey, TestProperty_Booking_NoOverlapUnderConcurrency)
- INV-002 — A booking is accepted only for a slot inside the resource's working hours and not in the past (a slot whose start is earlier than the current time is rejected, a slot whose start equals or follows the current time is accepted); a past-slot request whose idempotency key already has a stored booking is not rejected as a past slot but resolved by INV-003. (check: test: TestBookSlot_AcceptsSlotStartingExactlyNow, TestBookSlot_RejectsSlotInThePast, TestBookSlot_RejectsSlotOutsideWorkingHours, TestBookSlot_RejectsSlotStartingOneSecondBeforeNow, TestBookSlot_UnknownKeyForPastSlotIsRejectedAsPast)
- INV-003 — Repeating a booking request with the same idempotency key and an identical payload (resource id, slot start, customer name) returns the original booking in its current state and creates nothing new; the same key with a different payload is rejected as a key conflict; the key is unique across the whole service; this holds also after the slot has started, where the stored key is looked up before a past-slot rejection. (check: test: TestBookSlot_ConcurrentRepeatsYieldOneBooking, TestBookSlot_CustomerNameComparisonDoesNotTrim, TestBookSlot_CustomerNameComparisonIsCaseSensitive, TestBookSlot_KeysDifferingOnlyByCaseAreDifferentKeys, TestBookSlot_KeysDifferingOnlyByTrailingSpaceAreDifferentKeys, TestBookSlot_PastSlotRepeatWithDifferentPayloadIsKeyConflict, TestBookSlot_RepeatAfterCancelReturnsCancelledOriginal, TestBookSlot_RepeatAfterSlotStartedCreatesNothingNew, TestBookSlot_RepeatAfterSlotStartedReturnsOriginalBooking, TestBookSlot_RepeatCreatesNothingNew, TestBookSlot_RepeatReturnsOriginalBooking, TestBookSlot_RepeatWithDifferentNameIsKeyConflict, TestBookSlot_RepeatWithDifferentResourceIsKeyConflict, TestBookSlot_RepeatWithDifferentSlotIsKeyConflict, TestProperty_Booking_ConcurrentSameKeyYieldsOneBooking, TestProperty_Booking_IdempotentUnderRetry, TestProperty_Booking_KeyReuseWithDifferentPayloadCreatesNothing)
- INV-004 — A cancelled booking is final and cannot be reactivated, and the schema rejects the transition from cancelled to active as well as the domain code; a new booking is needed. (check: test: TestBookSlot_RepeatAfterCancelCreatesNoNewBooking, TestBooking_CancelledStaysCancelled, TestBooking_CannotReactivate, TestBooking_NewBookingAfterCancelCreatesNewId, TestSchema_RejectsCancelledToActiveTransition)
- INV-005 — A resource always has a name and a weekly working-hours schedule with at most one interval per weekday, each interval bound aligned to the half hour and inside a single UTC day, where the end of an interval may be 24:00 (the end of that UTC day, exclusive), the start may not be 24:00 and no bound exceeds 24:00. (check: test: TestCreateResource_AcceptsIntervalEndingAtTwentyFourHundred, TestCreateResource_RejectsEmptyName, TestCreateResource_RejectsIntervalBoundAfterTwentyFourHundred, TestCreateResource_RejectsIntervalCrossingMidnightUtc, TestCreateResource_RejectsIntervalStartingAtTwentyFourHundred, TestCreateResource_RejectsMisalignedIntervalBound, TestCreateResource_RejectsMissingWorkingHours, TestCreateResource_RejectsTwoIntervalsOnOneWeekday)

## Readiness

- testable_acceptance_criteria — gap_free
- engineering_constraint_coverage — gap_free
- architecture_traceability — gap_free
- blocking_decisions_resolved — gap_free
- dependencies_resolved — gap_free
- design_seed_for_user_facing — gap_free_by_non_applicability: not applicable — the card's own `UI surface:` machine stamp reads `none` (step 6b, FR-061); there is no surface to seed.
- open_subject_answered — gap_free

## Architecture context

- **FR:** FR-007, FR-008
- **NFR:** —
- **ADR:** ADR-0003, ADR-0008, ADR-0010, ADR-0012
- **Components:** COMP-001, COMP-004, COMP-005, COMP-006
- **Trace:** meta/architecture/trace.yml

## Worktree notes

Open decisions touching this card (do not answer them here): DEC-017 (a cancel racing a booking of the same slot) is not covered by this card's ACs; DEC-014 (whether a no-op repeated cancel re-emits its event) is open; DEC-011 (leaked booking ids let anyone cancel) is open and CON-006 stands.
- [Env] forge 2026.10.10
- [Drift gate] ⚠ unprocessed external drift: internal/store/*.go, internal/store/migrations/** (meta/drift-pending.yml event 2026-10-22T06:30:37Z, range 5fdd66b..54118a9; drift.gate default warn)
- [Run] 2026-10-22 dispatched by `run` (wave 3) on the still-open run 2026-10-21T17:24:03Z (journal tail stop_declared, not run_completed → reused per cmd-run resume 1a); dispatcher.state top-level `wave:` still reads 1 (no writer advances it), the active entry carries wave 3 and this card's Wave: 3 stands
- [Conflict graph] predicted edge with CARD-006 (overlap 2/3 = 0.67 ≥ 0.3: internal/httpapi/**.go, internal/store/*.go) — CARD-004 (P1) runs first, CARD-006 starts after this card is merged
- STRUCTURE: the booking status has ONE write path in the store, `UpdateBookingStatus(ctx, id, to)` on the `booking.Repository` port: a single autocommit `UPDATE booking SET status = ? WHERE id = ? RETURNING <columns>` (ADR-0012: no read-then-write; the RETURNING row is the answer, so a cancel is one statement). `Service.Cancel` is that call with `StatusCancelled`. The port takes a target status (not a cancel-only method) so the schema guard of ADR-0010 is reachable, mapped and tested through the adapter; no production caller passes `active` (NG-2).
- STRUCTURE: cancelled-is-final lives twice (ADR-0010): `Booking.WithStatus(to)` is the aggregate's transition rule (domain guard, returns `ErrBookingCancelled`), and trigger `booking_cancelled_is_final` (migration `0002_cancelled_booking_is_final.sql`, `BEFORE UPDATE OF status ... WHEN OLD.status = 'cancelled' AND NEW.status <> 'cancelled'`, `RAISE(ABORT, 'booking_cancelled_is_final')`) is the data guard. The store recognises the abort by code SQLITE_CONSTRAINT_TRIGGER AND that message (`isCancelledIsFinal` in store.go, next to `IsUniqueViolation`), so a future trigger is not mistaken for it. Cancel does not call `WithStatus`: its target (cancelled) is allowed from every status, so a read before the UPDATE would only add a read-then-write window.
- STRUCTURE: domain sentinels `booking.ErrNotFound` (store maps `sql.ErrNoRows`) and `booking.ErrBookingCancelled`; handlers map `ErrNotFound` to the generated 404 types. `ErrBookingCancelled` → 409 `booking_cancelled` is mapped once, in `writeResponseError` (server.go), because no operation of the contract declares a 409 for it (no operation requests reactivation, ASM-0001); every other handler error stays 500.
- STRUCTURE: API tests of operations with path parameters validate against the spec through `apiEnv.validateAt(method, template, target, rec)` (cancel_test.go); the shared `conforms` looks up literal paths only and was left untouched. `apiEnv` and the booking property `env` now expose the opened `*store.Store` (field `store`).
- AC-030 is exercised without a reactivation route (NG-2): `TestBooking_CannotReactivate` checks the domain guard on the read-back aggregate, then serves a test-only `reactivatingServer` (the real Server with its cancel operation replaced by `UpdateBookingStatus(active)` on the real store, skipping the domain guard) through the real `NewHandler`: the schema aborts, the store maps it, the plumbing answers 409 `booking_cancelled` (body validated against the `Error` schema), and GET B1 still reads cancelled.
- Mutation check: with migration 0002 removed, TestSchema_RejectsCancelledToActiveTransition, TestStore_UpdateBookingStatusMapsTheSchemaGuard, TestBooking_CannotReactivate and TestProperty_Booking_CancelledNeverBecomesActive fail; restored.
- Invariants of AGG-002 touched and their property tests: INV-004 → TestProperty_Booking_CancelledNeverBecomesActive (random sequences over domain/port/raw SQL/upsert attempts, slot free or re-booked; also asserts INV-001 count at the slot); EC-005 → TestProperty_Booking_RepeatedCancelIsIdempotent (1..10 cancels, sequential or concurrent). INV-001/INV-003 unchanged (existing properties green).
- SCOPE+ internal/booking/README.md, internal/store/README.md, internal/store/migrations/README.md, internal/httpapi/README.md — factual updates for the new operations, migration 0002 and the 409 mapping (not .go files; next to the Touches globs).
- Shared-file edits kept local: unimplemented.go lost ONLY GetBooking and CancelBooking (resource stubs untouched); api_test.go got one struct field, one constructor argument, and the two booking rows removed from TestServer_UnimplementedOperationsAreInternalError; repository.go only appended.
- Summary: cancel by id (200, idempotent, same body on repeat; 404 not_found), read by id (200/404), migration 0002 trigger, store `UpdateBookingStatus`/`GetBooking`, domain `Cancel`/`Get`/`WithStatus`. Gates: `make build test vet` green; `go test -race` green for booking, store, httpapi; `go generate ./...` no drift; go.mod/go.sum and internal/catalog unchanged.
- [Commit] e39372e — feat(booking): CARD-004 cancel a booking and keep cancellation final (staged via forge:commit stage_card_paths.py, exit 0; READMEs per SCOPE+ above).
- [Scope] internal/booking/README.md, internal/booking/booking.go, internal/booking/booking_test.go, internal/booking/cancel_property_test.go, internal/booking/property_test.go, internal/httpapi/README.md, internal/httpapi/api_test.go, internal/httpapi/bookings.go, internal/httpapi/cancel_test.go, internal/httpapi/server.go, internal/httpapi/unimplemented.go, internal/store/README.md, internal/store/migrations/0002_cancelled_booking_is_final.sql, internal/store/migrations/README.md, internal/store/repository.go, internal/store/repository_test.go, internal/store/schema_test.go, internal/store/store.go
- [Build gate] not reached — the scope gate runs at the TOP of the cycle and escalated first (the implementation agent reported `make build test vet` green, go test -race green on booking/store/httpapi, go generate no drift; not re-verified by the orchestrator)
- [Scope poach] diff covers 0.67 of CARD-006's Touches (matched: internal/httpapi/**.go, internal/store/*.go)
- [Scope gate] runaway: poached CARD-006 (internal/httpapi/**.go, internal/store/*.go, ratio 0.67); excess 3/18 = 0.17 (internal/booking/README.md, internal/httpapi/README.md, internal/store/README.md, SCOPE+ noted); comp_spread 0; guardrail_hits none; conflict.hotspots empty
- [Escalated] 2026-10-22T07:26:56Z — scope gate RUNAWAY (cycle 1, before review): diff covers 67% of ready same-wave sibling CARD-006's Touches (internal/httpapi/**.go, internal/store/*.go); CARD-006 is the sibling serialized behind this card by the predicted conflict edge, so any implementation of CARD-004 inside its own Touches trips the poach rule; conflict.hotspots empty · station: decompose · route: decide at the decompose station whether internal/httpapi/**.go and internal/store/*.go are shared ground between CARD-004 and CARD-006 (seed conflict.hotspots — a human config decision) or re-slice Touches ownership, or let CARD-006 finish first so no ready sibling owns those globs; then /kanban unblock CARD-004 (returns to review) and /kanban review CARD-004 (rebase onto main first). Worktree ../booking-demo-CARD-004 and branch card/004-cancel-booking-final kept (head e39372e, unreviewed)
- [Info] 2026-10-22T07:45Z — after CARD-006 merged (4c2cd96), a dry-run `git merge-tree --write-tree main card/004-cancel-booking-final` reports content conflicts in internal/httpapi/unimplemented.go, internal/store/README.md, internal/store/repository.go, internal/store/repository_test.go (api_test.go auto-merges); the rebase on unblock must keep BOTH sides (CARD-006 resource read methods + CARD-004 booking status/read methods; each side removed its own stubs from unimplemented.go) and re-run all gates. No ready sibling owns internal/httpapi/**.go or internal/store/*.go any more (CARD-006 done), so the poach condition behind this escalation is gone; unblocking remains a human decompose-station decision.
