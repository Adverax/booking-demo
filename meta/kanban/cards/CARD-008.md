# CARD-008: Availability search over a date range

**Status:** ready
**Priority:** P1
**Category:** feature
**Estimate:** 1c
**Complexity:** standard
**Closure:** closed
**Revision pending:** false
**Skill:** golang-pro
**TDD:** —
**Branch:** card/008-availability-search
**Worktree:** —
**Source:** meta/architecture/handoff.md#increment-3/availability
**Idea:** — (no match in backlog-scored.yml)
**Wave:** 5
**Depends on:** CARD-004, CARD-005, CARD-006
**Touches:** internal/availability/**.go, internal/httpapi/**.go, internal/store/*.go
**UI surface:** none
**Readiness:** 7/7
**Review score:** —
**Started:** —
**Closed:** —
**Actual:** —
**Merge commit:** —
**Blocked by:** —

## What to implement

Increment 3, the availability half of "Catalog and availability" (COMP-003 Availability Search).

- For a resource and a date range of at most 14 days, list the free slots: 30 minutes long, aligned to the half hour, inside the resource's working hours (read through COMP-002, ASM-0007), and not overlapping an active booking (FR-003, FR-004). Cancelled bookings do not occupy their slot.
- A range of exactly 14 days succeeds; 15 days is a 4xx JSON error with a stable code; an unknown resource is not found.
- A range whose start is after its end is a 4xx JSON error with a stable code, rejected before any read of bookings (ADR-0018, DEC-016 resolved; AC-054).
- Slots are returned as RFC 3339 UTC instants (CON-001); stored instants are integer epoch seconds (ADR-0013).

Relocated acceptance criteria. Two ACs the handoff lists under Increment 2 assert their outcome through the availability query, which does not exist before this card; the Increment 3 checkpoint ("a cancelled booking's slot reappears in the next query") covers them, so they are carried here:
- AC-025 of FR-007 (TestCancelBooking_FreesSlotImmediately), relying on cancellation from CARD-004;
- AC-042 of FR-006 (TestBookSlot_RepeatAfterCancelCreatesNoNewBooking), relying on the replay classifier from CARD-005.
Their requirements' engineering constraints (EC-002..EC-005) stay with CARD-004 and CARD-005, which own those behaviours.

Open decisions and assumptions touching this card (current as of this decompose run; supersedes the list in Worktree notes, which the procedure carries over unchanged). Do not answer them here:
- DEC-015: whether the cross-aggregate read of working hours through COMP-002 is recorded on the capabilities. It does not change where this card reads them.
- ASM-0002 (past slots listed as free) is an unvalidated assumption this increment collapses; do not add a past-slot filter beyond what the ACs state.
Resolved since the first cut: DEC-016 (ADR-0018) adds AC-054 to this card. DEC-012 (ADR-0020, a past-slot rejection checks the idempotency key first) concerns booking only; availability keeps the rules its ACs state.

## Non goals

- NG-1: The exact error-code name for a reversed range (decided in the API specification, FR-004 out of scope)
- NG-2: Any change to the 14-day maximum (unchanged, FR-004 out of scope)
- NG-3: A rule for a one-point range whose start equals its end (not covered by the ADR-0018 delta; it stays as the existing range rules treat it, FR-004)

## Acceptance criteria

- AC-007: Given a resource open Mon 09:00-10:00 UTC with no bookings, when free slots are requested for that Monday, then exactly 2 slots are returned, starting 09:00 and 09:30, each 30 minutes long — test: TestAvailability_ReturnsHalfHourSlotsInsideWorkingHours
- AC-008: Given a resource open Mon 09:00-10:00 UTC with an active booking for 09:00, when free slots are requested for that Monday, then only the 09:30 slot is returned — test: TestAvailability_ExcludesActiveBookedSlot
- AC-009: Given a resource open Mon 09:00-10:00 UTC, when free slots are requested for that Monday, then no returned slot starts before 09:00 or ends after 10:00 — test: TestAvailability_ExcludesSlotsOutsideWorkingHours
- AC-010: Given a resource open Mon-Fri only, when free slots are requested for a Saturday, then the response contains 0 slots — test: TestAvailability_NoSlotsOnNonWorkingDay
- AC-011: Given a persisted resource and a date range of exactly 14 days, when free slots are requested, then the request succeeds with slots for the whole range — test: TestAvailability_AcceptsRangeOf14Days
- AC-012: Given a persisted resource and a date range of 15 days, when free slots are requested, then the response is a 4xx JSON error with a stable code field — test: TestAvailability_RejectsRangeOver14Days
- AC-013: Given no resource with id "missing-id" exists, when free slots are requested for it, then the response is a not-found JSON error with a stable code field — test: TestAvailability_UnknownResourceNotFound
- AC-054: Given a persisted resource and a date range whose start is 2 days after its end, when free slots are requested, then the response is a 4xx JSON error with a stable code field — test: TestAvailability_RejectsRangeStartAfterEnd
- AC-025: Given Monday 09:00 booked and no free slot listed for it, when the booking is cancelled, then the next availability query lists the Monday 09:00 slot as free — test: TestCancelBooking_FreesSlotImmediately
- AC-042: Given a booking B1 for Monday 09:00 created with key K1 and then cancelled, when the identical request is repeated with key K1, then the total number of bookings stays 1 and the Monday 09:00 slot is still listed as free — test: TestBookSlot_RepeatAfterCancelCreatesNoNewBooking

## Engineering constraints

- EC(ADR-0013/R1): for any range and any injected clock value, availability computes slots from integer UTC epoch seconds and reads the current time only from the injected clock
  test: PropertyTest_Availability_ReadsTimeOnlyThroughInjectedClock

## Guardrails

- G-1: Do not edit `internal/booking/**.go` — owned by CARD-007 this wave
- G-2: Do not edit `internal/store/migrations/**` — Increment 3 rollback: "revert the increment's merges; it only adds read paths and validation over existing storage."

## System contract

- ADR-0010/R1 — A booking whose status is cancelled is never set active again; the schema rejects the transition (check: test: TestBooking_CannotReactivate)
- ADR-0012/R1 — The booking write path uses single autocommit statements and no transaction that reads and then writes (check: review-lens)
- ADR-0013/R1 — Instants are stored as integer UTC epoch seconds and the current time is read only through the injected clock (check: review-lens)
- ADR-0015/R1 — Idempotency keys and customer names are compared byte for byte as received, and two bookings occupy the same slot only if resource id and canonical slot start are equal (check: review-lens)
- CON-007 — The booking write path uses single autocommit statements and no transaction that reads and then writes. (check: review-lens)
- CON-008 — Instants are stored as integer UTC epoch seconds and converted to RFC 3339 only at the API boundary; the current time is obtained through an injectable clock. (check: review-lens)
- INV-001 — Two active bookings of the same resource never overlap, also under concurrent requests. (check: test: PropertyTest_Booking_NoOverlapUnderConcurrency, TestBookSlot_50ConcurrentRequestsExactlyOneWins, TestBookSlot_RejectsAlreadyBookedSlot, TestBookSlot_SameStartOnAnotherResourceIsNotTheSameSlot, TestBookSlot_SlotTakenStoresNothingUnderNewKey)
- INV-002 — A booking is accepted only for a slot inside the resource's working hours and not in the past (a slot whose start is earlier than the current time is rejected, a slot whose start equals or follows the current time is accepted); a past-slot request whose idempotency key already has a stored booking is not rejected as a past slot but resolved by INV-003. (check: test: TestBookSlot_AcceptsSlotStartingExactlyNow, TestBookSlot_RejectsSlotInThePast, TestBookSlot_RejectsSlotOutsideWorkingHours, TestBookSlot_RejectsSlotStartingOneSecondBeforeNow, TestBookSlot_UnknownKeyForPastSlotIsRejectedAsPast)
- INV-003 — Repeating a booking request with the same idempotency key and an identical payload (resource id, slot start, customer name) returns the original booking in its current state and creates nothing new; the same key with a different payload is rejected as a key conflict; the key is unique across the whole service; this holds also after the slot has started, where the stored key is looked up before a past-slot rejection. (check: test: PropertyTest_Booking_ConcurrentSameKeyYieldsOneBooking, PropertyTest_Booking_IdempotentUnderRetry, PropertyTest_Booking_KeyReuseWithDifferentPayloadCreatesNothing, TestBookSlot_ConcurrentRepeatsYieldOneBooking, TestBookSlot_CustomerNameComparisonDoesNotTrim, TestBookSlot_CustomerNameComparisonIsCaseSensitive, TestBookSlot_KeysDifferingOnlyByCaseAreDifferentKeys, TestBookSlot_KeysDifferingOnlyByTrailingSpaceAreDifferentKeys, TestBookSlot_PastSlotRepeatWithDifferentPayloadIsKeyConflict, TestBookSlot_RepeatAfterCancelReturnsCancelledOriginal, TestBookSlot_RepeatAfterSlotStartedCreatesNothingNew, TestBookSlot_RepeatAfterSlotStartedReturnsOriginalBooking, TestBookSlot_RepeatCreatesNothingNew, TestBookSlot_RepeatReturnsOriginalBooking, TestBookSlot_RepeatWithDifferentNameIsKeyConflict, TestBookSlot_RepeatWithDifferentResourceIsKeyConflict, TestBookSlot_RepeatWithDifferentSlotIsKeyConflict)
- INV-004 — A cancelled booking is final and cannot be reactivated, and the schema rejects the transition from cancelled to active as well as the domain code; a new booking is needed. (check: test: TestBookSlot_RepeatAfterCancelCreatesNoNewBooking, TestBooking_CancelledStaysCancelled, TestBooking_CannotReactivate, TestBooking_NewBookingAfterCancelCreatesNewId, TestSchema_RejectsCancelledToActiveTransition)
- INV-005 — A resource always has a name and a weekly working-hours schedule with at most one interval per weekday, each interval bound aligned to the half hour and inside a single UTC day. (check: test: TestCreateResource_RejectsEmptyName, TestCreateResource_RejectsIntervalCrossingMidnightUtc, TestCreateResource_RejectsMisalignedIntervalBound, TestCreateResource_RejectsMissingWorkingHours, TestCreateResource_RejectsTwoIntervalsOnOneWeekday)

## Readiness

- testable_acceptance_criteria — gap_free
- engineering_constraint_coverage — gap_free
- architecture_traceability — gap_free
- blocking_decisions_resolved — gap_free
- dependencies_resolved — gap_free
- design_seed_for_user_facing — gap_free_by_non_applicability: not applicable — the card's own `UI surface:` machine stamp reads `none` (step 6b, FR-061); there is no surface to seed.
- open_subject_answered — gap_free

## Architecture context

- **FR:** FR-003, FR-004
- **NFR:** —
- **ADR:** ADR-0004, ADR-0008, ADR-0009, ADR-0013, ADR-0018
- **Components:** COMP-001, COMP-002, COMP-003, COMP-005
- **Trace:** meta/architecture/trace.yml

## Worktree notes

Open decisions touching this card (do not answer them here): DEC-016 (range input validation), DEC-015 (cross-aggregate read of working hours). AC-025 and AC-042 are relocated here from Increment 2 (see What to implement).
