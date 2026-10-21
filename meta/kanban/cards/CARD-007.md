# CARD-007: Booking validation: working hours, past slots, alignment, unknown resource

**Status:** blocked
**Priority:** P1
**Category:** feature
**Estimate:** 1c
**Complexity:** standard
**Closure:** closed
**Revision pending:** false
**Skill:** golang-pro
**TDD:** —
**Branch:** card/007-booking-validation
**Worktree:** —
**Source:** meta/architecture/handoff.md#increment-3/booking-validation
**Idea:** — (no match in backlog-scored.yml)
**Wave:** 5
**Depends on:** CARD-005, CARD-006
**Touches:** internal/booking/**.go, internal/httpapi/**.go
**UI surface:** none
**Readiness:** 5/7 — gaps: blocking_decisions_resolved, dependencies_resolved
**Review score:** —
**Started:** —
**Closed:** —
**Actual:** —
**Merge commit:** —
**Blocked by:** readiness — blocking_decisions_resolved, dependencies_resolved

## What to implement

Increment 3, the booking-validation tests of "Catalog and availability" (FR-005, INV-002).

- Reject a booking whose slot lies outside the resource's working hours, read through COMP-002 (ASM-0007, ADR-0004) (AC-015).
- Reject a booking whose slot is in the past, judged against the injected clock (ADR-0013) (AC-016).
- Reject a start time not aligned to the half hour (AC-017).
- An unknown resource is a not-found error with a stable code (AC-020).
- Every rejection is a 4xx JSON error with a stable code and persists nothing.
- EC-001 (no overlap under concurrency) must keep holding with the validation in front of the insert.

Open decisions this card must not answer (they shape exactly this card's behaviour):
- DEC-005: whether "in the past" means the slot start is before now or the slot has fully ended. AC-016 (a slot that started 1 hour ago) is rejected under both readings; do not encode the boundary for a slot that is in progress beyond what AC-016 states.
- DEC-012: whether the not-in-the-past check runs before or after the idempotency-key lookup for a late replay (interplay with CARD-005's classifier). Do not pick an order.
- DEC-016: empty customer name and empty idempotency key validation have no ACs.

## Non goals

- NG-1: Partial overlaps of bookings (cannot occur with fixed 30-minute slots, ASM-0003; FR-005 out of scope)

## Acceptance criteria

- AC-015: Given a resource open Mon-Fri 09:00-17:00 UTC and a future Monday 08:00 slot, when a customer books that slot, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestBookSlot_RejectsSlotOutsideWorkingHours
- AC-016: Given a resource open Mon-Fri 09:00-17:00 UTC and a slot that started 1 hour ago inside working hours, when a customer books that slot, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestBookSlot_RejectsSlotInThePast
- AC-017: Given a request for a start time of 09:15 UTC, not aligned to the half hour, when a customer books that slot, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestBookSlot_RejectsNonAlignedStart
- AC-020: Given no resource with id "missing-id" exists, when a customer books a slot of it, then the response is a not-found JSON error with a stable code field — test: TestBookSlot_UnknownResourceNotFound

## Engineering constraints

- EC-001: For any set of concurrent booking requests for overlapping slots of one resource, at most one active booking exists afterwards
  test: PropertyTest_Booking_NoOverlapUnderConcurrency
- EC(ADR-0013/R1): for any slot and any injected clock value, the past-slot decision reads the current time only from the injected clock and compares integer UTC epoch seconds
  test: PropertyTest_Booking_PastCheckUsesInjectedClock

## Guardrails

- G-1: Do not edit `internal/availability/**.go` — owned by CARD-008 this wave
- G-2: Do not edit `internal/store/migrations/**` — Increment 3 rollback: "revert the increment's merges; it only adds read paths and validation over existing storage."

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
- blocking_decisions_resolved — gap: blocking decisions: open and blocking: DEC-005, DEC-006, DEC-016, DEC-034 — the candidate waits on an unresolved DEC.
- dependencies_resolved — gap: dependencies: CARD-005 (status 'blocked' does not resolve), CARD-006 (status 'blocked' does not resolve) — a declared dependency does not resolve.
- design_seed_for_user_facing — gap_free_by_non_applicability: not applicable — the card's own `UI surface:` machine stamp reads `none` (step 6b, FR-061); there is no surface to seed.
- open_subject_answered — gap_free

## Architecture context

- **FR:** FR-005
- **NFR:** —
- **ADR:** ADR-0004, ADR-0009, ADR-0013
- **Components:** COMP-001, COMP-002, COMP-004
- **Trace:** meta/architecture/trace.yml

## Worktree notes

Open decisions touching this card (do not answer them here): DEC-005 (the "in the past" boundary), DEC-012 (order of the late-replay check), DEC-016 (input-validation ACs).
