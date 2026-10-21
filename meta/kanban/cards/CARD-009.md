# CARD-009: Release criteria suite and supervisor deployment sample

**Status:** blocked
**Priority:** P1
**Category:** feature
**Estimate:** 1c
**Complexity:** standard
**Closure:** closed
**Revision pending:** false
**Skill:** golang-pro
**TDD:** —
**Branch:** card/009-release-criteria
**Worktree:** —
**Source:** meta/architecture/handoff.md#increment-4
**Idea:** — (no match in backlog-scored.yml)
**Wave:** 6
**Depends on:** CARD-007, CARD-008
**Touches:** api/openapi.yaml, internal/httpapi/**.go, cmd/booking/**.go
**UI surface:** none
**Readiness:** 6/7 — gaps: blocking_decisions_resolved
**Review score:** —
**Started:** —
**Closed:** —
**Actual:** —
**Merge commit:** —
**Blocked by:** readiness — blocking_decisions_resolved, dependencies_resolved

## What to implement

Increment 4, "Release criteria". The checkpoint is `go test -run TestRelease ./...` passing.

- TestRelease_ConcurrencyExactlyOneWinner: against the real binary and a fresh database, 50 simultaneous booking requests for one free slot yield exactly one success (NFR-001, AC-031).
- TestRelease_FullAPIEndToEndOnFreshDatabase: on a fresh database, create a resource, list availability, book, repeat the booking and cancel, all through the HTTP API (NFR-002, AC-032).
- TestRelease_EveryRuleHasANamedTest: every rule of the design brief (the invariants and the stated FRs) is linked to at least one named test (NFR-003, AC-033).
- Check that the generated HTTP plumbing returns the stable JSON error shape with a `code` field for every failure, including the generator's own request-decoding and routing failures (ASM-0005, CON-004).
- A sample OS supervisor unit and environment file for running the binary (ADR-0005): the database path and other settings as environment variables, a single instance on a local disk. The handoff names no path for these two files; choose one and record it in Worktree notes.

Open decisions touching this card (current as of this decompose run). Do not answer them here:
- DEC-031: how rule-to-test coverage is checked (an ADR or harness element, or left to the cards). It decides the mechanism behind TestRelease_EveryRuleHasANamedTest. Readiness gap.
- DEC-018: NFR-001's category label (scalability, or a correctness release criterion). Readiness gap.
- DEC-029: whether the single-instance premise is enforced or only documented, which shapes the supervisor unit.
- DEC-007, DEC-010: no uptime target and no logging, metrics or tracing requirement is decided; the supervisor unit adds none.
Resolved since the first cut: DEC-034 (ADR-0019, trace citations aligned), DEC-006 (ADR-0017, performance not applicable: the release suite measures correctness under concurrency, not latency or throughput) and DEC-012 (ADR-0020, a past-slot rejection checks the idempotency key first and returns the original booking when the payload matches). The rule list behind AC-033 now includes AC-050..AC-058, and INV-002/INV-003 carry the late-replay tests.

## Acceptance criteria

- AC-031: Given a free future slot and a fresh database, when 50 simultaneous booking requests target that slot, then successful_bookings == 1 — test: TestRelease_ConcurrencyExactlyOneWinner
- AC-032: Given a fresh database, when the end-to-end scenario creates a resource, lists availability, books, repeats the booking and cancels, then end_to_end_api_scenarios_passing == 100% — test: TestRelease_FullAPIEndToEndOnFreshDatabase
- AC-033: Given the list of rules and their linked test names in requirements.yml, when the rule-to-test coverage check runs, then rules_covered_by_a_named_test == 100% — test: TestRelease_EveryRuleHasANamedTest

## Guardrails

- G-1: Do not edit `internal/store/migrations/**` — Increment 4 rollback: "nothing new is shipped by this increment beyond the checks and a sample supervisor unit and environment file; revert the merges."
- G-2: No new product behaviour: the increment ships only the checks and a sample supervisor unit and environment file (Increment 4 rollback); every existing acceptance test of CARD-003..CARD-008 keeps passing

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
- engineering_constraint_coverage — gap_free_by_non_applicability: not applicable — no requirement this candidate names declares `engineering_constraints:` and step 6c5 promoted none, so the card format's "omit the section entirely" applies.
- architecture_traceability — gap_free
- blocking_decisions_resolved — gap: blocking decisions: open and blocking: DEC-018, DEC-031 — the candidate waits on an unresolved DEC.
- dependencies_resolved — gap_free
- design_seed_for_user_facing — gap_free_by_non_applicability: not applicable — the card's own `UI surface:` machine stamp reads `none` (step 6b, FR-061); there is no surface to seed.
- open_subject_answered — gap_free

## Architecture context

- **FR:** —
- **NFR:** NFR-001, NFR-002, NFR-003
- **ADR:** ADR-0005, ADR-0009
- **Components:** COMP-001, COMP-007
- **Trace:** meta/architecture/trace.yml

## Worktree notes

Open decisions touching this card (do not answer them here): DEC-031 (rule-to-test coverage mechanism), DEC-018 (NFR-001 category), DEC-029 (single-instance enforcement).
