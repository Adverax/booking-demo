# CARD-009: Release criteria suite and supervisor deployment sample

**Status:** ready
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
**Readiness:** 7/7
**Review score:** —
**Started:** —
**Closed:** —
**Actual:** —
**Merge commit:** —
**Blocked by:** —

## What to implement

Increment 4, "Release criteria". The checkpoint is `go test -run TestRelease ./...` passing.

- TestRelease_ConcurrencyExactlyOneWinner: against the real binary and a fresh database, 50 simultaneous booking requests for one free slot yield exactly one success (NFR-001, AC-031). NFR-001 is a correctness release criterion that proves INV-001 under concurrency, not a scale target (ADR-0021).
- TestRelease_FullAPIEndToEndOnFreshDatabase: on a fresh database, create a resource, list availability, book, repeat the booking and cancel, all through the HTTP API of the real binary (NFR-002, AC-032).
- TestRelease_EveryRuleHasANamedTest: the mechanism is decided by ADR-0022. A Go test in the release set reads every planned test name from the requirements model (`meta/architecture/requirements.yml`) and fails when a named test does not exist as a Go test function in the repository; a name that is not a valid Go test function name counts as missing (NFR-003, AC-033). It checks existence, not meaningfulness.
- Check that the generated HTTP plumbing returns the stable JSON error shape with a `code` field for every failure, including the generator's own request-decoding and routing failures (ASM-0005, CON-004).
- A sample OS supervisor unit and environment file for running the binary (ADR-0005): the database path and other settings as environment variables, a single instance on a local disk. The handoff names no path for these two files; choose one and record it in Worktree notes.

Existing code this card builds on (main as of this decompose run; CARD-001..CARD-008 are done):
- `cmd/booking/main_test.go` already builds the real binary once in `TestMain` and runs it against a fresh SQLite file (`startService`); `TestBookSlot_50ConcurrentRequestsExactlyOneWins` there is the Increment 1 form of the 50-request check. The release tests can reuse that harness.
- The binary reads `BOOKING_DB_PATH` (required) and `BOOKING_ADDR` (default `:8080`) from the environment (`cmd/booking/README.md`); the sample environment file sets these two and no others.
- ASM-0005's three cases are already covered in `internal/httpapi/server_test.go`: `TestNewHandler_MalformedBodyIsValidationError`, `TestNewHandler_UnknownPathIsNotFound`, `TestNewHandler_WrongMethodIsMethodNotAllowed`, plus `TestProperty_EveryFailureIsJSONErrorWithDeclaredCode`. Keep them green; add a case only for a failure path they do not reach.
- Every planned test name in the model except the three `TestRelease_*` names already exists as a Go test function (measured at this decompose run: 66 planned names, 3 missing), so AC-033 passes once this card's three tests exist; no test outside this card's footprint needs to be written for it.
- ADR-0022 notes that reading YAML in Go may add a dependency with an allowlisted licence. `gopkg.in/yaml.v2` v2.4.0 is already a direct requirement in `go.mod` (used by `internal/httpapi/contract_test.go`); its licence is Apache-2.0 with MIT for the libyaml port, both in `meta/audit-allowlist.json`. Reusing it adds no dependency; a different YAML library would need the dependency gate and a licence check.
- `make test` runs the whole suite including the release tests; the Makefile has `build`, `test`, `vet`, `tools` and `audit` targets.

Open decisions touching this card (current as of this decompose run). Do not answer them here:
- DEC-029: whether the single-instance premise is enforced or only documented, which shapes the supervisor unit.
- DEC-007, DEC-010: no uptime target and no logging, metrics or tracing requirement is decided; the supervisor unit adds none.
- DEC-017: concurrency is proven only for identical concurrent requests; TestRelease_ConcurrencyExactlyOneWinner covers that shape and adds no other race.
None of them blocks this card. The other open decisions (DEC-008, DEC-009, DEC-011, DEC-014, DEC-015, DEC-019, DEC-020, DEC-030, DEC-033) do not touch this card.
Resolved since the first cut: DEC-018 (ADR-0021, NFR-001 is a correctness criterion, category operability, scalability not applicable), DEC-031 (ADR-0022, rule-to-test coverage is verified by TestRelease_EveryRuleHasANamedTest), DEC-034 (ADR-0019, trace citations aligned), DEC-006 (ADR-0017, performance not applicable: the release suite measures correctness under concurrency, not latency or throughput) and DEC-012 (ADR-0020, a past-slot rejection checks the idempotency key first and returns the original booking when the payload matches). The rule list behind AC-033 includes AC-050..AC-058, and INV-002/INV-003 carry the late-replay tests.

## Non goals

- NG-1: No scale, throughput or latency target; the 50-request test measures correctness under concurrency (NFR-001 clarification: the brief states none, ADR-0017, ADR-0021)
- NG-2: TestRelease_EveryRuleHasANamedTest does not judge whether a named test is meaningful (NFR-003 clarification: review does that, ADR-0022)

## Acceptance criteria

- AC-031: Given a free future slot and a fresh database, when 50 simultaneous booking requests target that slot, then successful_bookings == 1 — test: TestRelease_ConcurrencyExactlyOneWinner
- AC-032: Given a fresh database, when the end-to-end scenario creates a resource, lists availability, books, repeats the booking and cancels, then end_to_end_api_scenarios_passing == 100% — test: TestRelease_FullAPIEndToEndOnFreshDatabase
- AC-033: Given the list of rules and their linked test names in requirements.yml, when the Go release test TestRelease_EveryRuleHasANamedTest reads every planned test name from requirements.yml, then rules_covered_by_a_named_test == 100%, and the test fails when a named test is not a Go test function in the repository — test: TestRelease_EveryRuleHasANamedTest

## Guardrails

- G-1: Do not edit `internal/store/migrations/**` — Increment 4 rollback: "nothing new is shipped by this increment beyond the checks and a sample supervisor unit and environment file; revert the merges."
- G-2: No new product behaviour: the increment ships only the checks and a sample supervisor unit and environment file (Increment 4 rollback); every existing test of CARD-001..CARD-008 keeps passing (test: `go test ./...`)

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
- engineering_constraint_coverage — gap_free_by_non_applicability: not applicable — no requirement this candidate names declares `engineering_constraints:` and step 6c5 promoted none, so the card format's "omit the section entirely" applies.
- architecture_traceability — gap_free
- blocking_decisions_resolved — gap_free
- dependencies_resolved — gap_free
- design_seed_for_user_facing — gap_free_by_non_applicability: not applicable — the card's own `UI surface:` machine stamp reads `none` (step 6b, FR-061); there is no surface to seed.
- open_subject_answered — gap_free

## Architecture context

- **FR:** —
- **NFR:** NFR-001, NFR-002, NFR-003
- **ADR:** ADR-0005, ADR-0009, ADR-0021, ADR-0022
- **Components:** COMP-001, COMP-007
- **Trace:** meta/architecture/trace.yml

## Worktree notes

Open decisions touching this card (do not answer them here): DEC-031 (rule-to-test coverage mechanism), DEC-018 (NFR-001 category), DEC-029 (single-instance enforcement).
