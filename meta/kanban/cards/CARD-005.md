# CARD-005: Idempotent booking replay with insert-then-classify

**Status:** ready
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
**Readiness:** 7/7
**Review score:** —
**Started:** —
**Closed:** —
**Actual:** —
**Merge commit:** —
**Blocked by:** —

## What to implement

Increment 2, the idempotency half of "Idempotency and cancellation" (FR-006, INV-003).

- Insert first. When the insert fails with a unique violation, read the booking stored under the request's idempotency key with a separate autocommit read and classify from what is stored (ADR-0011, ADR-0012): same payload returns the original booking in its current state, also when it is cancelled (ADR-0002); a different payload is the key conflict (ADR-0001); no booking under the key means the slot was taken. The classification never depends on which index reported first. CARD-002 (done) ships `store.IsUniqueViolation(err)`, true only for a unique-index rejection (either the active-slot or the idempotency-key index) and deliberately silent about which one fired, so the classifying read is what decides.
- Payload sameness is resource id, canonical slot start and customer name; keys and names are compared byte for byte as received, with no case folding and no trimming (ADR-0015, AC-043..AC-046). The key is unique across the whole service.
- A taken slot under a new key stores nothing under that key (AC-047).
- An empty idempotency key is rejected before any insert with a 4xx JSON error carrying a stable code, and nothing is persisted (ADR-0018, AC-053). An empty key never reaches the classifier.
- Late replay (ADR-0020, DEC-012 resolved): these rules hold also after the slot has started. An identical repeat of a stored key returns the original booking, not a past-slot error, and creates nothing new (AC-055, AC-058); a repeat of a stored key with a different payload for the same past slot is the key conflict of ADR-0001, not the past-slot error (AC-056). Write these tests with the injected clock moved past the slot start after the first booking (ADR-0013). No past-slot check exists yet in this card (CARD-007 adds it), so here they follow from insert-then-classify; once CARD-007 puts the past check in front of the insert and looks the key up when that check rejects (ADR-0020), these tests are what keeps that order honest. Do not add a key read on the normal booking path: ADR-0020 rejects lookup_first and keeps the extra read on the past-slot rejection path only.

The classifier's states, declared before code (decompose 6d2):

| state | output | channel | test |
|---|---|---|---|
| rejected before insert: empty idempotency key | 4xx JSON error with its stable code, nothing stored | HTTP error response | TestBookSlot_RejectsEmptyIdempotencyKey |
| inserted (no violation) | the new booking, success | HTTP response of the book endpoint | TestBookSlot_RepeatCreatesNothingNew (first request), TestBookSlot_KeysDifferingOnlyByCaseAreDifferentKeys |
| replay, same payload, booking active | the original booking with its id, success | HTTP response of the book endpoint | TestBookSlot_RepeatReturnsOriginalBooking |
| replay, same payload, booking cancelled | the original booking with status cancelled, success | HTTP response of the book endpoint | TestBookSlot_RepeatAfterCancelReturnsCancelledOriginal |
| key reused with a different payload | key-conflict JSON error with its stable code, nothing stored or changed | HTTP error response | TestBookSlot_RepeatWithDifferentNameIsKeyConflict, TestBookSlot_RepeatWithDifferentSlotIsKeyConflict, TestBookSlot_RepeatWithDifferentResourceIsKeyConflict |
| violation and no booking under the key | slot-taken conflict JSON error with its stable code, nothing stored | HTTP error response | TestBookSlot_SlotTakenStoresNothingUnderNewKey |
| replay after the slot has started, same payload | the original booking with its id, success, not a past-slot error; nothing new stored | HTTP response of the book endpoint | TestBookSlot_RepeatAfterSlotStartedReturnsOriginalBooking, TestBookSlot_RepeatAfterSlotStartedCreatesNothingNew |
| key reused with a different payload after the slot has started | key-conflict JSON error with the ADR-0001 code, not the past-slot code; nothing stored or changed | HTTP error response | TestBookSlot_PastSlotRepeatWithDifferentPayloadIsKeyConflict |

Open decisions touching this card (current as of this decompose run; supersedes the list in Worktree notes, which the procedure carries over unchanged and which still names DEC-012 and DEC-016 as open). Do not answer them here:
- DEC-017: concurrent same-key requests with different payloads are not covered by this card's ACs.
- DEC-014: whether a no-op replay emits its domain event again (EVT-002).
Resolved since the first cut:
- DEC-012, resolved by ADR-0020: a past-slot rejection checks the idempotency key first and returns the original booking when the payload matches. It adds AC-055, AC-056 and AC-058 to this card; the past-slot check and the key lookup on its rejection path, with AC-057 (unknown key for a past slot is rejected as past), land in CARD-007.
- DEC-016, resolved by ADR-0018: adds AC-053 to this card.
Revised since the last cut: ADR-0004 (24:00 interval end) and ADR-0006 (`_txlock` exists) do not change replay. Property-test names now carry the `Test` prefix Go requires (`TestProperty_...`).

Moved to CARD-008: AC-042 (TestBookSlot_RepeatAfterCancelCreatesNoNewBooking) asserts that the slot "is still listed as free", which needs the availability query of Increment 3.

## Non goals

- NG-1: A past slot whose idempotency key has no stored booking (FR-005 owns it, AC-057; FR-006 out of scope)
- NG-2: Maximum length of an idempotency key (left to the API specification, FR-006 out of scope)
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
- AC-053: Given a resource open Mon-Fri 09:00-17:00 UTC and a free future Monday 09:00 slot, when customer "Ann" books that slot with an empty idempotency key, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestBookSlot_RejectsEmptyIdempotencyKey
- AC-055: Given a resource open Mon-Fri 09:00-17:00 UTC, a booking B1 for Monday 09:00 by "Ann" created with key K1 while the injected clock read Monday 08:00 UTC, and the clock now reading Monday 09:00:01 UTC, when the identical request is repeated with key K1 after the slot has started, then the response is B1 with its original id and not a past-slot error — test: TestBookSlot_RepeatAfterSlotStartedReturnsOriginalBooking
- AC-058: Given a booking B1 for Monday 09:00 by "Ann" created with key K1 while the injected clock read Monday 08:00 UTC, and the clock now reading Monday 09:00:01 UTC, when the identical request is repeated with key K1 after the slot has started, then the total number of bookings stays 1 — test: TestBookSlot_RepeatAfterSlotStartedCreatesNothingNew
- AC-056: Given a booking B1 for Monday 09:00 by "Ann" created with key K1 while the injected clock read Monday 08:00 UTC, and the clock now reading Monday 09:00:01 UTC, when a request repeats key K1 for the same past slot with customer name "Bob", then the response is a key-conflict JSON error with the code of ADR-0001, not the past-slot code — test: TestBookSlot_PastSlotRepeatWithDifferentPayloadIsKeyConflict

## Engineering constraints

- EC-002: A booking request repeated with the same idempotency key and an identical payload (same resource id, slot start and customer name), in any number and at any time (also after the slot has started), never creates a second booking and returns the original
  test: TestProperty_Booking_IdempotentUnderRetry
- EC-003: For any number of simultaneous requests sharing one idempotency key and an identical payload, exactly one booking is created and every response carries its id
  test: TestProperty_Booking_ConcurrentSameKeyYieldsOneBooking
- EC-004: A request that repeats an existing idempotency key with any differing resource id, slot start or customer name (also for a slot that has already started) never creates a booking and never changes the stored one
  test: TestProperty_Booking_KeyReuseWithDifferentPayloadCreatesNothing
- EC(ADR-0012/R1): for any number of concurrent replays of one key, every request ends as the original booking or a classified conflict and none ends in a lock or busy error, because the insert and the classifying read are separate autocommit statements
  test: TestProperty_Booking_ConcurrentReplaysNeverSeeLockErrors
- EC(ADR-0015/R1): for any stored booking and any repeat of its key, the repeat is classified as the same payload exactly when resource id, canonical slot start and the byte sequence of the customer name are all equal
  test: TestProperty_Booking_PayloadComparedByteForByte

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

- **FR:** FR-006
- **NFR:** —
- **ADR:** ADR-0001, ADR-0002, ADR-0008, ADR-0011, ADR-0012, ADR-0015, ADR-0018, ADR-0020
- **Components:** COMP-001, COMP-004, COMP-005
- **Trace:** meta/architecture/trace.yml

## Worktree notes

Open decisions touching this card (do not answer them here): DEC-012 (order of the key lookup against the not-in-the-past check for a late replay; the handoff asks for it to be settled before this card), DEC-016 (empty or oversized keys), DEC-017 (concurrent same-key requests with different payloads), DEC-014 (event re-emission on a no-op replay).
