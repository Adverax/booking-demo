# CARD-007: Booking validation: working hours, past slots, alignment, unknown resource

**Status:** ready
**Priority:** P1
**Category:** feature
**Estimate:** 1c
**Complexity:** architectural
**Closure:** closed
**States:** enumerable
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
**Readiness:** 7/7
**Review score:** —
**Started:** —
**Closed:** —
**Actual:** —
**Merge commit:** —
**Blocked by:** —

## What to implement

Increment 3, the booking-validation tests of "Catalog and availability" (FR-005, INV-002).

- Reject a booking whose slot lies outside the resource's working hours, read through COMP-002 (ASM-0007, ADR-0004) (AC-015).
- Reject a booking whose slot is in the past, judged against the injected clock (ADR-0013) (AC-016). "In the past" means the slot start is earlier than now; a start equal to now is accepted (ADR-0016, DEC-005 resolved): a start one second before now is rejected (AC-050) and a start exactly at now is booked (AC-051). One comparison of two epoch-second instants.
- Reject a start time not aligned to the half hour (AC-017).
- Reject an empty customer name before any insert (ADR-0018, DEC-016 resolved; AC-052). The empty idempotency key (AC-053) belongs to CARD-005.
- An unknown resource is a not-found error with a stable code (AC-020).
- Every rejection is a 4xx JSON error with a stable code and persists nothing.
- Past-slot rejection checks the idempotency key first (ADR-0020, DEC-012 resolved). Keep the order of ADR-0011: validate, insert, classify a violation. Only when validation rejects a request because its slot is in the past, look the key up before answering: a stored booking with the same payload is returned as the original booking; a stored booking with a different payload is the key conflict of ADR-0001; no stored booking means the request is rejected as a past slot and nothing is stored under the key (AC-057). The lookup is one autocommit read (ADR-0012) on that rejection path only; the normal path gets no extra read. Reuse CARD-005's read of the booking stored under a key and its payload comparison (ADR-0015) instead of writing a second one.
- CARD-005's late-replay tests (AC-055, AC-056, AC-058: TestBookSlot_RepeatAfterSlotStartedReturnsOriginalBooking, TestBookSlot_RepeatAfterSlotStartedCreatesNothingNew, TestBookSlot_PastSlotRepeatWithDifferentPayloadIsKeyConflict) pass before this card only because no past check exists yet. Once this card puts the past check in front of the insert, they show that the key lookup is in place, and they must keep passing.
- EC-001 (no overlap under concurrency) must keep holding with the validation in front of the insert.

The validation outcomes, declared before code (decompose 6d2):

| state | output | channel | test |
|---|---|---|---|
| unknown resource | not-found JSON error with its stable code, nothing stored | HTTP error response | TestBookSlot_UnknownResourceNotFound |
| empty customer name | 4xx JSON error with its stable code, nothing stored | HTTP error response | TestBookSlot_RejectsEmptyCustomerName |
| start not aligned to the half hour | 4xx JSON error with its stable code, nothing stored | HTTP error response | TestBookSlot_RejectsNonAlignedStart |
| slot outside the resource's working hours | 4xx JSON error with its stable code, nothing stored | HTTP error response | TestBookSlot_RejectsSlotOutsideWorkingHours |
| slot start before now, no booking stored under the key | past-slot 4xx JSON error with its stable code, nothing stored under the key | HTTP error response | TestBookSlot_RejectsSlotInThePast, TestBookSlot_RejectsSlotStartingOneSecondBeforeNow, TestBookSlot_UnknownKeyForPastSlotIsRejectedAsPast |
| slot start before now, key stored with the same payload | the original booking with its id, success; nothing new stored | HTTP response of the book endpoint | TestBookSlot_RepeatAfterSlotStartedReturnsOriginalBooking, TestBookSlot_RepeatAfterSlotStartedCreatesNothingNew (CARD-005) |
| slot start before now, key stored with a different payload | key-conflict JSON error with the ADR-0001 code, not the past-slot code | HTTP error response | TestBookSlot_PastSlotRepeatWithDifferentPayloadIsKeyConflict (CARD-005) |
| slot start equal to or after now, valid input | passed on to insert-then-classify (CARD-003, CARD-005) | HTTP response of the book endpoint | TestBookSlot_AcceptsSlotStartingExactlyNow |

Open decisions touching this card (current as of this decompose run; supersedes the list in Worktree notes, which the procedure carries over unchanged and which still names DEC-005, DEC-012 and DEC-016 as open). Do not answer them here:
- DEC-015: whether the cross-aggregate read of working hours through COMP-002 is recorded on the capabilities. It does not change where this card reads them.
- DEC-017: a cancel racing a booking of the same slot is not covered by this card's ACs.
Resolved since the first cut:
- DEC-005, resolved by ADR-0016: start before now is past, start equal to now is accepted (AC-050, AC-051).
- DEC-016, resolved by ADR-0018: an empty customer name is rejected (AC-052).
- DEC-012, resolved by ADR-0020: a past-slot rejection checks the idempotency key first and returns the original booking when the payload matches. It adds AC-057 to this card; the stored-key outcomes are AC-055, AC-056 and AC-058 on CARD-005.

## Non goals

- NG-1: Partial overlaps of bookings (cannot occur with fixed 30-minute slots, ASM-0003; FR-005 out of scope)
- NG-2: The acceptance criteria of a stored key replayed after the slot has started (FR-006 owns that replay, ADR-0020; FR-005 out of scope): AC-055, AC-056 and AC-058 stay on CARD-005
- NG-3: Maximum length of a customer name and the error-code names (left to the API specification, FR-005 out of scope)

## Acceptance criteria

- AC-015: Given a resource open Mon-Fri 09:00-17:00 UTC and a future Monday 08:00 slot, when a customer books that slot, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestBookSlot_RejectsSlotOutsideWorkingHours
- AC-016: Given a resource open Mon-Fri 09:00-17:00 UTC and a slot that started 1 hour ago inside working hours, when a customer books that slot, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestBookSlot_RejectsSlotInThePast
- AC-017: Given a request for a start time of 09:15 UTC, not aligned to the half hour, when a customer books that slot, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestBookSlot_RejectsNonAlignedStart
- AC-020: Given no resource with id "missing-id" exists, when a customer books a slot of it, then the response is a not-found JSON error with a stable code field — test: TestBookSlot_UnknownResourceNotFound
- AC-050: Given a resource open Mon-Fri 09:00-17:00 UTC and an injected clock reading Monday 09:00:01 UTC, when a customer books the Monday 09:00 slot, whose start is 1 second before now, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestBookSlot_RejectsSlotStartingOneSecondBeforeNow
- AC-051: Given a resource open Mon-Fri 09:00-17:00 UTC and an injected clock reading exactly Monday 09:00:00 UTC, when customer "Ann" books the Monday 09:00 slot, whose start equals now, then a booking for Monday 09:00 with the name "Ann" is persisted — test: TestBookSlot_AcceptsSlotStartingExactlyNow
- AC-052: Given a resource open Mon-Fri 09:00-17:00 UTC and a free future Monday 09:00 slot, when a customer books that slot with an empty customer name and a new idempotency key, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestBookSlot_RejectsEmptyCustomerName
- AC-057: Given a resource open Mon-Fri 09:00-17:00 UTC, an injected clock reading Monday 09:00:01 UTC, and no booking stored under key K9, when customer "Ann" books the Monday 09:00 slot, which started 1 second ago, with the unknown key K9, then the response is a past-slot 4xx JSON error with a stable code field and no booking is stored under K9 — test: TestBookSlot_UnknownKeyForPastSlotIsRejectedAsPast

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

- **FR:** FR-005
- **NFR:** —
- **ADR:** ADR-0004, ADR-0009, ADR-0013, ADR-0016, ADR-0018, ADR-0020
- **Components:** COMP-001, COMP-002, COMP-004
- **Trace:** meta/architecture/trace.yml

## Worktree notes

Open decisions touching this card (do not answer them here): DEC-005 (the "in the past" boundary), DEC-012 (order of the late-replay check), DEC-016 (input-validation ACs).
