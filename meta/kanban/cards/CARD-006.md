# CARD-006: Resource catalog: validated creation, list and read

**Status:** ready
**Priority:** P2
**Category:** feature
**Estimate:** 1c
**Complexity:** standard
**Closure:** closed
**Revision pending:** false
**Skill:** golang-pro
**TDD:** —
**Branch:** card/006-resource-catalog
**Worktree:** —
**Source:** meta/architecture/handoff.md#increment-3/catalog
**Idea:** — (no match in backlog-scored.yml)
**Wave:** 3
**Depends on:** CARD-003
**Touches:** internal/catalog/**.go, internal/httpapi/**.go, internal/store/*.go
**UI surface:** none
**Readiness:** 7/7
**Review score:** —
**Started:** —
**Closed:** —
**Actual:** —
**Merge commit:** —
**Blocked by:** —

## What to implement

Increment 3, the catalog half of "Catalog and availability" (COMP-002 Resource Catalog).

- Full validation of resource creation on top of the minimal path from CARD-003 (FR-001, INV-005, ADR-0004): a name is required; weekly working hours are required; at most one interval per weekday; interval bounds aligned to the half hour; each interval inside a single UTC day. Every violation is a 4xx JSON error with a stable code and nothing is persisted (AC-002, AC-003, AC-034..AC-036). AC-001 (the happy path) is already carried by CARD-003.
- List all resources and read a single resource by id; an unknown id is a not-found error with a stable code (FR-002).
- COMP-002 is the single owner of the working-hours rules (ASM-0007): availability (CARD-008) and booking validation (CARD-007) read working hours through it.

The Increment 3 rollback states that this increment "only adds read paths and validation over existing storage": no schema change.

Open decisions touching this card (current as of this decompose run; supersedes the list in Worktree notes, which the procedure carries over unchanged). Do not answer them here:
- DEC-015: whether the cross-aggregate read of working hours (availability and booking validation reading through this component) is recorded on the capabilities. It does not change this card's ownership of the rules.
Resolved since the first cut: DEC-016 (ADR-0018) added criteria for booking and availability inputs only; resource creation's validation criteria are unchanged.

## Non goals

- NG-1: Holidays, per-date exceptions, several intervals per day, a lunch break (FR-001 out of scope)

## Acceptance criteria

- AC-002: Given a request with name "Dr. Lee" and no working hours, when the operator creates the resource, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestCreateResource_RejectsMissingWorkingHours
- AC-003: Given a request with an empty name and working hours Mon-Fri 09:00-17:00, when the operator creates the resource, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestCreateResource_RejectsEmptyName
- AC-034: Given a request with name "Dr. Lee" and two Monday intervals 09:00-12:00 and 13:00-17:00 UTC, when the operator creates the resource, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestCreateResource_RejectsTwoIntervalsOnOneWeekday
- AC-035: Given a request with name "Dr. Lee" and a Monday interval 09:15-17:00 UTC, the start not aligned to the half hour, when the operator creates the resource, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestCreateResource_RejectsMisalignedIntervalBound
- AC-036: Given a request with name "Dr. Lee" and a Monday interval 22:00-02:00 UTC that crosses midnight, when the operator creates the resource, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestCreateResource_RejectsIntervalCrossingMidnightUtc
- AC-004: Given two persisted resources "Dr. Lee" and "Room A", when the resources are listed, then the response contains exactly those 2 resources — test: TestListResources_ReturnsAll
- AC-005: Given a persisted resource "Room A" with a known id, when that resource is read by id, then the response contains the name and weekly working hours of "Room A" — test: TestGetResource_ReturnsResource
- AC-006: Given no resource with id "missing-id" exists, when that id is read, then the response is a not-found JSON error with a stable code field — test: TestGetResource_UnknownIdNotFound

## Guardrails

- G-1: Do not edit `internal/booking/**.go` — owned by CARD-004 this wave
- G-2: Do not edit `internal/store/migrations/**` — Increment 3 rollback: "revert the increment's merges; it only adds read paths and validation over existing storage."

## System contract

- ADR-0010/R1 — A booking whose status is cancelled is never set active again; the schema rejects the transition (check: test: TestBooking_CannotReactivate)
- ADR-0012/R1 — The booking write path uses single autocommit statements and no transaction that reads and then writes (check: review-lens)
- ADR-0013/R1 — Instants are stored as integer UTC epoch seconds and the current time is read only through the injected clock (check: review-lens)
- ADR-0015/R1 — Idempotency keys and customer names are compared byte for byte as received, and two bookings occupy the same slot only if resource id and canonical slot start are equal (check: review-lens)
- CON-007 — The booking write path uses single autocommit statements and no transaction that reads and then writes. (check: review-lens)
- CON-008 — Instants are stored as integer UTC epoch seconds and converted to RFC 3339 only at the API boundary; the current time is obtained through an injectable clock. (check: review-lens)
- INV-001 — Two active bookings of the same resource never overlap, also under concurrent requests. (check: test: PropertyTest_Booking_NoOverlapUnderConcurrency, TestBookSlot_50ConcurrentRequestsExactlyOneWins, TestBookSlot_RejectsAlreadyBookedSlot, TestBookSlot_SameStartOnAnotherResourceIsNotTheSameSlot, TestBookSlot_SlotTakenStoresNothingUnderNewKey)
- INV-002 — A booking is accepted only for a slot inside the resource's working hours and not in the past (a slot whose start is earlier than the current time is rejected, a slot whose start equals or follows the current time is accepted). (check: test: TestBookSlot_AcceptsSlotStartingExactlyNow, TestBookSlot_RejectsSlotInThePast, TestBookSlot_RejectsSlotOutsideWorkingHours, TestBookSlot_RejectsSlotStartingOneSecondBeforeNow)
- INV-003 — Repeating a booking request with the same idempotency key and an identical payload (resource id, slot start, customer name) returns the original booking in its current state and creates nothing new; the same key with a different payload is rejected as a key conflict; the key is unique across the whole service. (check: test: PropertyTest_Booking_ConcurrentSameKeyYieldsOneBooking, PropertyTest_Booking_IdempotentUnderRetry, PropertyTest_Booking_KeyReuseWithDifferentPayloadCreatesNothing, TestBookSlot_ConcurrentRepeatsYieldOneBooking, TestBookSlot_CustomerNameComparisonDoesNotTrim, TestBookSlot_CustomerNameComparisonIsCaseSensitive, TestBookSlot_KeysDifferingOnlyByCaseAreDifferentKeys, TestBookSlot_KeysDifferingOnlyByTrailingSpaceAreDifferentKeys, TestBookSlot_RepeatAfterCancelReturnsCancelledOriginal, TestBookSlot_RepeatCreatesNothingNew, TestBookSlot_RepeatReturnsOriginalBooking, TestBookSlot_RepeatWithDifferentNameIsKeyConflict, TestBookSlot_RepeatWithDifferentResourceIsKeyConflict, TestBookSlot_RepeatWithDifferentSlotIsKeyConflict)
- INV-004 — A cancelled booking is final and cannot be reactivated, and the schema rejects the transition from cancelled to active as well as the domain code; a new booking is needed. (check: test: TestBookSlot_RepeatAfterCancelCreatesNoNewBooking, TestBooking_CancelledStaysCancelled, TestBooking_CannotReactivate, TestBooking_NewBookingAfterCancelCreatesNewId, TestSchema_RejectsCancelledToActiveTransition)
- INV-005 — A resource always has a name and a weekly working-hours schedule with at most one interval per weekday, each interval bound aligned to the half hour and inside a single UTC day. (check: test: TestCreateResource_RejectsEmptyName, TestCreateResource_RejectsIntervalCrossingMidnightUtc, TestCreateResource_RejectsMisalignedIntervalBound, TestCreateResource_RejectsMissingWorkingHours, TestCreateResource_RejectsTwoIntervalsOnOneWeekday)

## Readiness

- testable_acceptance_criteria — gap_free
- engineering_constraint_coverage — gap_free_by_non_applicability: not applicable — no requirement this candidate names declares `engineering_constraints:` and step 6c5 promoted none, so the card format's "omit the section entirely" applies.
- architecture_traceability — gap_free
- blocking_decisions_resolved — gap_free
- dependencies_resolved — gap_free
- design_seed_for_user_facing — gap_free_by_non_applicability: not applicable — the card's own `UI surface:` machine stamp reads `none` (step 6b, FR-061); there is no surface to seed.
- open_subject_answered — gap_free

## Architecture context

- **FR:** FR-001, FR-002
- **NFR:** —
- **ADR:** ADR-0004, ADR-0009
- **Components:** COMP-001, COMP-002, COMP-005
- **Trace:** meta/architecture/trace.yml

## Worktree notes

Open decisions touching this card (do not answer them here): DEC-016 (input-validation ACs) concerns availability and booking inputs, not resource creation, whose validation ACs exist; DEC-015 (recording the cross-aggregate read of working hours) is open.
