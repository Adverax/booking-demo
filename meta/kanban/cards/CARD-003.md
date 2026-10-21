# CARD-003: Walking skeleton: bootstrap, minimal resource creation and booking one slot

**Status:** ready
**Priority:** P1
**Category:** feature
**Estimate:** 2c
**Complexity:** architectural
**Closure:** closed
**Revision pending:** false
**Skill:** golang-pro
**TDD:** —
**Branch:** card/003-walking-skeleton-book-slot
**Worktree:** —
**Source:** meta/architecture/handoff.md#increment-1/walking-skeleton
**Idea:** — (no match in backlog-scored.yml)
**Wave:** 2
**Depends on:** CARD-001, CARD-002
**Touches:** cmd/booking/**.go, internal/httpapi/**.go, internal/booking/**.go, internal/catalog/**.go, internal/store/*.go
**UI surface:** none
**Readiness:** 6/7 — gaps: dependencies_resolved
**Review score:** —
**Started:** —
**Closed:** —
**Actual:** —
**Merge commit:** —
**Blocked by:** —

## What to implement

Increment 1, "Walking skeleton through the riskiest path": the real binary serves the generated API over a fresh SQLite file and books one slot correctly under 50 concurrent writers.

- COMP-007 Service Bootstrap (`cmd/booking/`): read configuration from environment variables (CON-005, ADR-0005; the database path among them), open the store and apply the embedded migrations at startup (ADR-0007; CARD-002, done, ships `store.Open(ctx, path)`, which applies every pending embedded migration before it returns), wire the components and serve the generated `net/http` server from CARD-001 on `http.ServeMux` (ADR-0009).
- A minimal create-resource path, included only because booking needs a resource (handoff Increment 1): persist a resource with a name and weekly working hours and return its generated id (AC-001). Full working-hours validation is Increment 3 (CARD-006), including the 24:00 interval end of the revised ADR-0004 (AC-059..AC-061).
- COMP-004 Slot Booking: book one slot of a resource with a customer name and an idempotency key. The slot start is normalised to its canonical UTC half-hour start before insert (ASM-0006, ADR-0013, ADR-0015); slot identity is (resource id, canonical slot start). The current time comes from an injectable clock (ADR-0013).
- The write is one autocommit INSERT with no transaction that reads and then writes (ADR-0012, CON-007); the partial unique index of ADR-0008 decides the winner, and its violation is mapped to a conflict response with a stable code (AC-018). CARD-002 ships `store.IsUniqueViolation(err)`, true only for a unique-index rejection and deliberately silent about which index fired. Telling a replay apart from a key conflict (insert-then-classify, ADR-0011) is CARD-005's.
- The concurrency test starts the real binary against a fresh SQLite file and fires 50 simultaneous requests with distinct keys at one slot: exactly one success, 49 conflicts, no lock errors (Increment 1 checkpoint, AC-019, NFR-001's uncertainty). If lock errors appear, ADR-0012's fallback (immediate transactions through the driver's lock option) applies. CARD-002 recorded the owed `_txlock` verification: the option is available on the pinned modernc.org/sqlite v1.61.0 (values deferred, immediate, exclusive; covered by `TestDriver_TxLockImmediateTakesTheWriteLockAtBegin`), and ADR-0006 was revised accordingly on 2026-10-22. The store does not use it; reach for it only if the checkpoint shows lock errors.

Requirement slice: the handoff splits FR-005 and FR-001 across increments by their test lists. This card carries FR-005's AC-014, AC-018, AC-019, AC-037 and EC-001, and FR-001's AC-001. FR-005's AC-015, AC-016, AC-017, AC-020 and the ADR-0016/ADR-0018/ADR-0020 criteria AC-050, AC-051, AC-052, AC-057 (start before now, start equal to now, empty customer name, unknown key for a past slot; all listed under Increment 3) go to CARD-007, and FR-001's remaining ACs to CARD-006 (Increment 3), including AC-059, AC-060 and AC-061 (an interval end of 24:00 accepted, a start of 24:00 and a bound after 24:00 rejected), which the handoff lists under Increment 3. NFR-001's own release criterion AC-031 is carried by CARD-009 (Increment 4).

Open decisions touching this card (current as of this decompose run; the decision list in Worktree notes predates ADR-0016..0020, still names DEC-005 and DEC-016 as open, and is carried over unchanged by the procedure, so this list supersedes it). Do not answer them here:
- DEC-017: a cancel racing a booking of the same slot, and concurrent same-key requests with different payloads, are not covered by this card's ACs.
- DEC-029: the single-instance premise that the 50-writer result relies on (ADR-0005, ADR-0008) is not enforced; do not add an instance lock.
- DEC-033: the bootstrap's open-database wiring is not drawn in the component diagram; diagram only.
- DEC-007, DEC-010: no uptime target and no logging, metrics or tracing requirement is decided for the bootstrap (CON-005); do not add any beyond what the code needs to run.
Resolved since the first cut: DEC-005 (ADR-0016) and DEC-016 (ADR-0018) land in CARD-007; DEC-012, resolved by ADR-0020 (a past-slot rejection checks the idempotency key first and returns the original booking when the payload matches), lands in CARD-005 (AC-055, AC-056, AC-058) and CARD-007 (AC-057). This card's ACs exercise none of them, and it adds no past-slot check.
Revised since the last cut: ADR-0004 (2026-10-22: an interval end may be 24:00, the end of the UTC day, exclusive; a start may not be 24:00; no bound beyond 24:00) adds AC-059..AC-061, which land in CARD-006, not here; ADR-0006 (2026-10-22: `_txlock` exists, the driver decision unchanged) is reflected in the concurrency bullet above. Property-test names now carry the `Test` prefix Go requires (`TestProperty_...`).

## Non goals

- NG-1: Full working-hours validation of resource creation (deferred to Increment 3, FR-001 full validation)
- NG-2: Partial overlaps of bookings (cannot occur with fixed 30-minute slots, ASM-0003)
- NG-3: Holidays, per-date exceptions, several intervals per day, a lunch break (FR-001 out of scope)
- NG-4: The replay of a stored key after the slot has started (FR-006 owns it, ADR-0020; FR-005 out of scope)
- NG-5: Maximum length of a customer name and the error-code names (left to the API specification, FR-005 out of scope)

## Acceptance criteria

- AC-001: Given no resources exist and a request with name "Dr. Lee" and working hours Mon-Fri 09:00-17:00 UTC, when the operator creates the resource, then the resource is persisted and returned with a generated id — test: TestCreateResource_Persists
- AC-014: Given a resource open Mon-Fri 09:00-17:00 UTC with no bookings and a future Monday 09:00 slot, when customer "Ann" books that slot with a new idempotency key, then a booking for Monday 09:00 with the name "Ann" is persisted — test: TestBookSlot_PersistsBooking
- AC-018: Given an active booking for Monday 09:00 of a resource, when another customer books the same slot with a different idempotency key, then the response is a conflict JSON error with a stable code field and still 1 active booking exists for that slot — test: TestBookSlot_RejectsAlreadyBookedSlot
- AC-019: Given a free future slot of a resource, when 50 simultaneous booking requests with 50 distinct idempotency keys target that slot, then exactly 1 request succeeds and 49 are rejected with a conflict error — test: TestBookSlot_50ConcurrentRequestsExactlyOneWins
- AC-037: Given resources R1 and R2 both open Mon 09:00-10:00 UTC and an active booking of R1 for Monday 09:00, when customer "Bob" books Monday 09:00 of R2 with a new idempotency key, then a booking of R2 for Monday 09:00 is persisted — test: TestBookSlot_SameStartOnAnotherResourceIsNotTheSameSlot

## Engineering constraints

- EC-001: For any set of concurrent booking requests for overlapping slots of one resource, at most one active booking exists afterwards
  test: TestProperty_Booking_NoOverlapUnderConcurrency
- EC(ADR-0012/R1): for any number of concurrent booking writers on one slot, every request ends as a success or a slot conflict and none ends in a lock or busy error, because each write is a single autocommit statement
  test: TestProperty_Booking_ConcurrentWritersNeverSeeLockErrors
- EC(ADR-0013/R1): for any requested slot start and any injected clock value, the stored slot start is the canonical UTC half-hour start in epoch seconds and the booking logic reads the time only from the injected clock
  test: TestProperty_Booking_SlotStartStoredAsCanonicalEpochSeconds
- EC(ADR-0015/R1): for any pair of booking requests, they compete for the same slot exactly when their resource ids and canonical slot starts are equal
  test: TestProperty_Booking_SlotIdentityIsResourceAndCanonicalStart

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
- dependencies_resolved — gap: dependencies: CARD-001 (status 'blocked' does not resolve) — a declared dependency does not resolve.
- design_seed_for_user_facing — gap_free_by_non_applicability: not applicable — the card's own `UI surface:` machine stamp reads `none` (step 6b, FR-061); there is no surface to seed.
- open_subject_answered — gap_free

## Architecture context

- **FR:** FR-005, FR-001
- **NFR:** —
- **ADR:** ADR-0005, ADR-0007, ADR-0008, ADR-0009, ADR-0012, ADR-0013, ADR-0015
- **Components:** COMP-001, COMP-002, COMP-004, COMP-005, COMP-007
- **Trace:** meta/architecture/trace.yml

## Worktree notes

Open decisions touching this card (do not answer them here): DEC-005 (whether a slot already in progress is "in the past") and DEC-016 (empty customer name or key) are not exercised by this card's ACs; the past-slot and input checks land in CARD-007. DEC-017 (cancel racing a booking, concurrent same-key requests with different payloads) is not covered by this card's ACs.
