# CARD-001: OpenAPI contract and generated server interface

**Status:** ready
**Priority:** P1
**Category:** feature
**Estimate:** 1c
**Complexity:** architectural
**Closure:** closed
**Revision pending:** false
**Skill:** api-designer
**TDD:** false <!-- auto-stamp 6d3: no test command resolves (Skill api-designer, review.test_command empty) -->
**Branch:** card/001-openapi-contract
**Worktree:** —
**Source:** meta/architecture/handoff.md#increment-1/openapi-contract
**Idea:** — (no match in backlog-scored.yml)
**Wave:** 1
**Depends on:** —
**Touches:** api/openapi.yaml, internal/httpapi/**.go, go.mod, go.sum
**UI surface:** none
**Readiness:** 7/7
**Review score:** —
**Started:** —
**Closed:** —
**Actual:** —
**Merge commit:** —
**Blocked by:** —

## What to implement

Increment 1 opens with the contract: ADR-0009 requires the OpenAPI document before any handler, and the handoff names `api/openapi.yaml` as the first card of Increment 1.

- Write `api/openapi.yaml` describing the whole HTTP surface the requirements name: create, list and read a resource (FR-001, FR-002); free slots of a resource for a date range of at most 14 days (FR-003, FR-004); book one slot with a customer name and an idempotency key (FR-005, FR-006); read a booking (AC-029); cancel a booking by id (FR-007).
- Instants cross the boundary as RFC 3339 UTC strings (CON-001, CON-008). Weekly working hours follow ADR-0004: at most one interval per weekday, half-hour aligned bounds, inside one UTC day.
- Every error is a JSON object with a stable `code` field (CON-004). The codes are declared once, in the specification (ADR-0009): at least not found, validation error, slot taken (AC-018, AC-047) and idempotency-key conflict (ADR-0001, AC-038..AC-040), kept distinct.
- Generate the types and the `net/http` server interface from the document (ADR-0009 names oapi-codegen as the example generator), mounted on `http.ServeMux`, under `internal/httpapi/`, with the `go:generate` directive that reproduces it. No handler logic: handlers are implemented by the cards that own each behaviour.

Decided since the first cut, and declared by the contract:
- ADR-0018 (DEC-016 resolved): the contract declares the matching input constraints. An empty customer name (AC-052) and an empty idempotency key (AC-053) are rejected with a stable code, and so is an availability range whose start is after its end (AC-054). The range ordering cannot be expressed as a schema keyword: document it on the operation and its error response. Maximum lengths of names and keys are left to the API specification by FR-005 and FR-006 and no acceptance criterion tests them, so do not invent a figure.
- ADR-0016 (DEC-005 resolved): a slot is in the past when its start is earlier than the current time; a start equal to now is accepted. Describe the past-slot rejection that way.
- ADR-0020 (DEC-012 resolved): a past-slot rejection checks the idempotency key first. Describe the book operation's outcomes for a slot that has already started accordingly: a repeat of a stored key with the same payload returns the original booking as a success (AC-055), a repeat of a stored key with a different payload gets the idempotency-key conflict error, not the past-slot error (AC-056), and only a key with no stored booking gets the past-slot error (AC-057).

Open decisions this card must not answer (record them as TODO comments in the spec, do not invent constraints):
- DEC-011: the format of booking ids (guessable or not) is not decided; declare them as opaque strings.
- DEC-030: whether ADR-0009's rejection reason is restated on the drift argument. The decision itself is not reopened; contract-first stands.

## Guardrails

- G-1: Do not edit `internal/store/*.go`, `internal/store/migrations/**` — owned by CARD-002 this wave

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

- testable_acceptance_criteria — gap_free_by_non_applicability: not applicable — untraced candidate (`FR: —`, decompose 10b): there is no requirement to carry criteria from (ADR-0038/R1).
- engineering_constraint_coverage — gap_free_by_non_applicability: not applicable — no requirement this candidate names declares `engineering_constraints:` and step 6c5 promoted none, so the card format's "omit the section entirely" applies.
- architecture_traceability — gap_free
- blocking_decisions_resolved — gap_free
- dependencies_resolved — gap_free
- design_seed_for_user_facing — gap_free_by_non_applicability: not applicable — the card's own `UI surface:` machine stamp reads `none` (step 6b, FR-061); there is no surface to seed.
- open_subject_answered — gap_free

## Architecture context

- **FR:** —
- **NFR:** —
- **ADR:** ADR-0009, ADR-0001, ADR-0004, ADR-0016, ADR-0018, ADR-0020
- **Components:** COMP-001
- **Trace:** meta/architecture/trace.yml

## Worktree notes

—
