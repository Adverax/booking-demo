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
**Worktree:** —
**Source:** meta/architecture/handoff.md#increment-2/cancellation
**Idea:** — (no match in backlog-scored.yml)
**Wave:** 3
**Depends on:** CARD-003
**Touches:** internal/booking/**.go, internal/httpapi/**.go, internal/store/*.go, internal/store/migrations/**
**UI surface:** none
**Readiness:** 6/7 — gaps: dependencies_resolved
**Review score:** —
**Started:** —
**Closed:** —
**Actual:** —
**Merge commit:** —
**Blocked by:** readiness — dependencies_resolved

## What to implement

Increment 2, the cancellation half of "Idempotency and cancellation".

- Cancel a booking by its id, with no customer identity check (FR-007, CON-006). Cancelling sets the status to cancelled with a single autocommit UPDATE (ADR-0012), which frees the slot for a new booking because the active-slot unique index of ADR-0008 covers active bookings only.
- A repeated cancel of an already cancelled booking succeeds and returns the cancelled booking, with the same response shape as the first cancel (ADR-0003, AC-048, EC-005). An unknown id is a not-found error with a stable code (AC-027).
- Read a booking by id (AC-029), so a cancelled booking can be observed as cancelled.
- Cancelled is final (FR-008, INV-004): keep the domain guard, and add a versioned migration with a schema trigger on the booking status that aborts any update from cancelled to active (ADR-0010, AC-049). The violation maps to a conflict error with a stable code (AC-030). A new booking of the same slot creates a new booking with a new id (AC-028).

Moved to CARD-008: AC-025 (TestCancelBooking_FreesSlotImmediately) asserts the freed slot through the next availability query, and availability (COMP-003) is built in Increment 3; the Increment 3 checkpoint ("a cancelled booking's slot reappears in the next query") is where it is verified.

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
  test: PropertyTest_Booking_RepeatedCancelIsIdempotent
- EC(ADR-0010/R1): for any cancelled booking and any sequence of status updates attempted on it, through the domain code or directly in the database, its status stays cancelled
  test: PropertyTest_Booking_CancelledNeverBecomesActive

## Guardrails

- G-1: Do not edit `internal/catalog/**.go` — owned by CARD-006 this wave

## System contract

- ADR-0010/R1 — A booking whose status is cancelled is never set active again; the schema rejects the transition (check: test: TestBooking_CannotReactivate)
- ADR-0012/R1 — The booking write path uses single autocommit statements and no transaction that reads and then writes (check: review-lens)
- ADR-0013/R1 — Instants are stored as integer UTC epoch seconds and the current time is read only through the injected clock (check: review-lens)
- ADR-0015/R1 — Idempotency keys and customer names are compared byte for byte as received, and two bookings occupy the same slot only if resource id and canonical slot start are equal (check: review-lens)
- CON-007 — The booking write path uses single autocommit statements and no transaction that reads and then writes. (check: review-lens)
- CON-008 — Instants are stored as integer UTC epoch seconds and converted to RFC 3339 only at the API boundary; the current time is obtained through an injectable clock. (check: review-lens)
- INV-001 — Two active bookings of the same resource never overlap, also under concurrent requests. (check: test: PropertyTest_Booking_NoOverlapUnderConcurrency, TestBookSlot_50ConcurrentRequestsExactlyOneWins, TestBookSlot_RejectsAlreadyBookedSlot, TestBookSlot_SameStartOnAnotherResourceIsNotTheSameSlot, TestBookSlot_SlotTakenStoresNothingUnderNewKey)
- INV-002 — A booking is accepted only for a slot inside the resource's working hours and not in the past. (check: test: TestBookSlot_RejectsSlotInThePast, TestBookSlot_RejectsSlotOutsideWorkingHours)
- INV-003 — Repeating a booking request with the same idempotency key and an identical payload (resource id, slot start, customer name) returns the original booking in its current state and creates nothing new; the same key with a different payload is rejected as a key conflict; the key is unique across the whole service. (check: test: PropertyTest_Booking_ConcurrentSameKeyYieldsOneBooking, PropertyTest_Booking_IdempotentUnderRetry, PropertyTest_Booking_KeyReuseWithDifferentPayloadCreatesNothing, TestBookSlot_ConcurrentRepeatsYieldOneBooking, TestBookSlot_CustomerNameComparisonDoesNotTrim, TestBookSlot_CustomerNameComparisonIsCaseSensitive, TestBookSlot_KeysDifferingOnlyByCaseAreDifferentKeys, TestBookSlot_KeysDifferingOnlyByTrailingSpaceAreDifferentKeys, TestBookSlot_RepeatAfterCancelReturnsCancelledOriginal, TestBookSlot_RepeatCreatesNothingNew, TestBookSlot_RepeatReturnsOriginalBooking, TestBookSlot_RepeatWithDifferentNameIsKeyConflict, TestBookSlot_RepeatWithDifferentResourceIsKeyConflict, TestBookSlot_RepeatWithDifferentSlotIsKeyConflict)
- INV-004 — A cancelled booking is final and cannot be reactivated, and the schema rejects the transition from cancelled to active as well as the domain code; a new booking is needed. (check: test: TestBookSlot_RepeatAfterCancelCreatesNoNewBooking, TestBooking_CancelledStaysCancelled, TestBooking_CannotReactivate, TestBooking_NewBookingAfterCancelCreatesNewId, TestSchema_RejectsCancelledToActiveTransition)
- INV-005 — A resource always has a name and a weekly working-hours schedule with at most one interval per weekday, each interval bound aligned to the half hour and inside a single UTC day. (check: test: TestCreateResource_RejectsEmptyName, TestCreateResource_RejectsIntervalCrossingMidnightUtc, TestCreateResource_RejectsMisalignedIntervalBound, TestCreateResource_RejectsMissingWorkingHours, TestCreateResource_RejectsTwoIntervalsOnOneWeekday)

## Readiness

- testable_acceptance_criteria — gap_free
- engineering_constraint_coverage — gap_free
- architecture_traceability — gap_free
- blocking_decisions_resolved — gap_free
- dependencies_resolved — gap: dependencies: CARD-003 (status 'blocked' does not resolve) — a declared dependency does not resolve.
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
