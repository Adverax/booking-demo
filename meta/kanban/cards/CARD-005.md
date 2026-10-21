# CARD-005: Idempotent booking replay with insert-then-classify

**Status:** blocked
**Priority:** P1
**Category:** feature
**Estimate:** 2c
**Complexity:** architectural
**Closure:** closed
**States:** enumerable
**Revision pending:** false
**Skill:** golang-pro
**TDD:** —
**Branch:** card/005-idempotent-replay
**Worktree:** —
**Source:** meta/architecture/handoff.md#increment-2/idempotent-replay
**Idea:** — (no match in backlog-scored.yml)
**Wave:** 4
**Depends on:** CARD-004
**Touches:** internal/booking/**.go, internal/httpapi/**.go, internal/store/*.go
**UI surface:** none
**Readiness:** 5/7 — gaps: blocking_decisions_resolved, dependencies_resolved
**Review score:** —
**Started:** —
**Closed:** —
**Actual:** —
**Merge commit:** —
**Blocked by:** readiness — blocking_decisions_resolved, dependencies_resolved

## What to implement

Increment 2, the idempotency half of "Idempotency and cancellation" (FR-006, INV-003).

- Insert first. When the insert fails with a unique violation, read the booking stored under the request's idempotency key with a separate autocommit read and classify from what is stored (ADR-0011, ADR-0012): same payload returns the original booking in its current state, also when it is cancelled (ADR-0002); a different payload is the key conflict (ADR-0001); no booking under the key means the slot was taken. The classification never depends on which index reported first.
- Payload sameness is resource id, canonical slot start and customer name; keys and names are compared byte for byte as received, with no case folding and no trimming (ADR-0015, AC-043..AC-046). The key is unique across the whole service.
- A taken slot under a new key stores nothing under that key (AC-047).

The classifier's states, declared before code (decompose 6d2):

| state | output | channel | test |
|---|---|---|---|
| inserted (no violation) | the new booking, success | HTTP response of the book endpoint | TestBookSlot_RepeatCreatesNothingNew (first request), TestBookSlot_KeysDifferingOnlyByCaseAreDifferentKeys |
| replay, same payload, booking active | the original booking with its id, success | HTTP response of the book endpoint | TestBookSlot_RepeatReturnsOriginalBooking |
| replay, same payload, booking cancelled | the original booking with status cancelled, success | HTTP response of the book endpoint | TestBookSlot_RepeatAfterCancelReturnsCancelledOriginal |
| key reused with a different payload | key-conflict JSON error with its stable code, nothing stored or changed | HTTP error response | TestBookSlot_RepeatWithDifferentNameIsKeyConflict, TestBookSlot_RepeatWithDifferentSlotIsKeyConflict, TestBookSlot_RepeatWithDifferentResourceIsKeyConflict |
| violation and no booking under the key | slot-taken conflict JSON error with its stable code, nothing stored | HTTP error response | TestBookSlot_SlotTakenStoresNothingUnderNewKey |

Open decision to settle before this card starts (handoff Increment 2: "DEC-012 ... to be settled before the card that implements replay"): DEC-012, whether the key lookup runs before the not-in-the-past validation for a late replay. Do not choose an order here; the past-slot check itself lands in CARD-007.

Moved to CARD-008: AC-042 (TestBookSlot_RepeatAfterCancelCreatesNoNewBooking) asserts that the slot "is still listed as free", which needs the availability query of Increment 3.

## Non goals

- NG-1: The order of the key lookup against the not-in-the-past check (DEC-012, FR-006 out of scope)
- NG-2: Empty or oversized idempotency keys (DEC-016, FR-006 out of scope)
- NG-3: Further concurrency scenarios beyond the stated ones (DEC-017, FR-006 out of scope)

## Acceptance criteria

- AC-021: Given a booking for Monday 09:00 created with idempotency key K1, when the identical request is repeated with key K1, then the response is the original booking with the same booking id — test: TestBookSlot_RepeatReturnsOriginalBooking
- AC-022: Given a booking created with idempotency key K1, when the identical request is repeated with key K1, then the total number of bookings stays 1 — test: TestBookSlot_RepeatCreatesNothingNew
- AC-023: Given 10 simultaneous identical booking requests with the same idempotency key K1, when all are processed, then exactly 1 booking exists and all 10 responses carry the same booking id — test: TestBookSlot_ConcurrentRepeatsYieldOneBooking
- AC-038: Given a booking for Monday 09:00 by customer "Ann" created with idempotency key K1, when a request repeats key K1 for the same slot with customer name "Bob", then the response is a key-conflict JSON error with a stable code field — test: TestBookSlot_RepeatWithDifferentNameIsKeyConflict
- AC-039: Given a booking for Monday 09:00 by "Ann" with key K1 and a free Monday 09:30 slot of the same resource, when a request repeats key K1 for Monday 09:30 with name "Ann", then the response is a key-conflict JSON error and the Monday 09:30 slot stays free — test: TestBookSlot_RepeatWithDifferentSlotIsKeyConflict
- AC-040: Given a booking of resource R1 for Monday 09:00 with key K1 and a free Monday 09:00 slot of resource R2, when a request repeats key K1 for resource R2 at Monday 09:00 with the same name, then the response is a key-conflict JSON error and R2 has 0 bookings — test: TestBookSlot_RepeatWithDifferentResourceIsKeyConflict
- AC-041: Given a booking B1 for Monday 09:00 created with key K1 and then cancelled, when the identical request is repeated with key K1, then the response is B1 with its original id and status cancelled — test: TestBookSlot_RepeatAfterCancelReturnsCancelledOriginal
- AC-043: Given a booking for Monday 09:00 with key "Key-1" and a free Monday 09:30 slot, when customer "Ann" books Monday 09:30 with key "key-1", differing only by case, then a new booking for Monday 09:30 with a new id is persisted — test: TestBookSlot_KeysDifferingOnlyByCaseAreDifferentKeys
- AC-044: Given a booking for Monday 09:00 with key "Key-1" and a free Monday 09:30 slot, when customer "Ann" books Monday 09:30 with key "Key-1 ", differing only by a trailing space, then a new booking for Monday 09:30 with a new id is persisted — test: TestBookSlot_KeysDifferingOnlyByTrailingSpaceAreDifferentKeys
- AC-045: Given a booking for Monday 09:00 by customer "Ann" with key K1, when a request repeats key K1 for the same slot with customer name "ann", then the response is a key-conflict JSON error with a stable code field — test: TestBookSlot_CustomerNameComparisonIsCaseSensitive
- AC-046: Given a booking for Monday 09:00 by customer "Ann" with key K1, when a request repeats key K1 for the same slot with customer name "Ann " with a trailing space, then the response is a key-conflict JSON error with a stable code field — test: TestBookSlot_CustomerNameComparisonDoesNotTrim
- AC-047: Given an active booking for Monday 09:00 stored under key K1, when another request with a new key K2 books the same slot, then the response is a slot-taken conflict JSON error and no booking is stored under key K2 — test: TestBookSlot_SlotTakenStoresNothingUnderNewKey

## Engineering constraints

- EC-002: A booking request repeated with the same idempotency key and an identical payload (same resource id, slot start and customer name), in any number and at any time, never creates a second booking and returns the original
  test: PropertyTest_Booking_IdempotentUnderRetry
- EC-003: For any number of simultaneous requests sharing one idempotency key and an identical payload, exactly one booking is created and every response carries its id
  test: PropertyTest_Booking_ConcurrentSameKeyYieldsOneBooking
- EC-004: A request that repeats an existing idempotency key with any differing resource id, slot start or customer name never creates a booking and never changes the stored one
  test: PropertyTest_Booking_KeyReuseWithDifferentPayloadCreatesNothing
- EC(ADR-0012/R1): for any number of concurrent replays of one key, every request ends as the original booking or a classified conflict and none ends in a lock or busy error, because the insert and the classifying read are separate autocommit statements
  test: PropertyTest_Booking_ConcurrentReplaysNeverSeeLockErrors
- EC(ADR-0015/R1): for any stored booking and any repeat of its key, the repeat is classified as the same payload exactly when resource id, canonical slot start and the byte sequence of the customer name are all equal
  test: PropertyTest_Booking_PayloadComparedByteForByte

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
- blocking_decisions_resolved — gap: blocking decisions: open and blocking: DEC-016 — the candidate waits on an unresolved DEC.
- dependencies_resolved — gap: dependencies: CARD-004 (status 'blocked' does not resolve) — a declared dependency does not resolve.
- design_seed_for_user_facing — gap_free_by_non_applicability: not applicable — the card's own `UI surface:` machine stamp reads `none` (step 6b, FR-061); there is no surface to seed.
- open_subject_answered — gap_free

## Architecture context

- **FR:** FR-006
- **NFR:** —
- **ADR:** ADR-0001, ADR-0002, ADR-0008, ADR-0011, ADR-0012, ADR-0015
- **Components:** COMP-001, COMP-004, COMP-005
- **Trace:** meta/architecture/trace.yml

## Worktree notes

Open decisions touching this card (do not answer them here): DEC-012 (order of the key lookup against the not-in-the-past check for a late replay; the handoff asks for it to be settled before this card), DEC-016 (empty or oversized keys), DEC-017 (concurrent same-key requests with different payloads), DEC-014 (event re-emission on a no-op replay).
