# CARD-007: Booking validation: working hours, past slots, alignment, unknown resource

**Status:** done
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
**Review score:** 9.5 (cycle 2/3)
**Started:** 2026-10-22T08:37:16Z
**Closed:** 2026-10-22T09:02:31Z
**Actual:** 0.1d
**Merge commit:** 1dc0cdcb0cffcc8b3804d12ac8cf0229e859b16d
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
Revised since the last cut: ADR-0004 (2026-10-22) lets an interval end at 24:00, the end of the UTC day, exclusive. Working hours are read through COMP-002 (CARD-006 owns the rule and AC-059..AC-061), so this card adds no rule of its own; its ACs are unchanged. Property-test names now carry the `Test` prefix Go requires (`TestProperty_...`).

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
  test: TestProperty_Booking_NoOverlapUnderConcurrency
- EC(ADR-0013/R1): for any slot and any injected clock value, the past-slot decision reads the current time only from the injected clock and compares integer UTC epoch seconds
  test: TestProperty_Booking_PastCheckUsesInjectedClock

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

- **FR:** FR-005
- **NFR:** —
- **ADR:** ADR-0004, ADR-0009, ADR-0013, ADR-0016, ADR-0018, ADR-0020
- **Components:** COMP-001, COMP-002, COMP-004
- **Trace:** meta/architecture/trace.yml

## Failure matrix

Declared before code. `busyTimeout` = 5 s (internal/store/store.go), `shutdownTimeout` = 10 s and `writeTimeout` = 30 s (cmd/booking/main.go). The booking path after this card is `Service.Book`: input-shape checks (no statement) → ONE autocommit SELECT of the resource through COMP-002 (`catalog.Get`) → working-hours check and past check against the injected clock (no statement) → either (past) ONE autocommit SELECT by key, or (not past) ONE autocommit INSERT and, on a unique violation only, ONE autocommit SELECT by key (ADR-0011, ADR-0012, ADR-0020). No transaction, no retry loop, no goroutine spawned.

| operation / boundary | failure mode | declared behavior | numeric bound |
|---|---|---|---|
| Book: input shape | poison input: empty customer name, empty key, `slotStart` not on the UTC half hour (seconds % 1800 ≠ 0 or a sub-second part) | 400 `validation_error` before any statement; the key is NOT looked up | 0 statements, 0 rows written |
| Book: input shape | poison input: oversized name or key (NG-3) | not rejected by this card; NewHandler's body cap bounds it | ≤ 1 MiB (`maxRequestBodyBytes`) |
| Book: resource read (COMP-002) | unknown resource id | 404 `not_found`; the key is NOT looked up | 1 SELECT, 0 rows written |
| Book: resource read | timeout / I/O error / ctx cancelled | 500 `internal_error`; nothing stored; a retry starts over | wait ≤ busyTimeout = 5 s (WAL read normally takes no lock: ≈ 0); 0 rows written |
| Book: working hours | slot not inside the interval of its UTC weekday (`Start ≤ m` and `m + 30 ≤ End`, End may be 24:00) | 422 `slot_outside_working_hours`; the key is NOT looked up | 1 SELECT, 0 rows written |
| Book: past check | slot start < now as integer UTC epoch seconds (start == now accepted; now's sub-second part is dropped) | key lookup, see the three rows below; never an insert | clock read exactly once per request |
| Book: past + key lookup | no booking under the key | 422 `slot_in_past`; nothing stored under the key | 2 SELECTs, 0 rows written |
| Book: past + key lookup | key stored, same payload (also cancelled) | 200 with the stored booking in its current state, replayed | 2 SELECTs, 0 rows written |
| Book: past + key lookup | key stored, different payload | 409 `idempotency_key_conflict`, not `slot_in_past`; stored booking unchanged | 2 SELECTs, 0 rows written/changed |
| Book: past + key lookup | timeout / I/O error / ctx cancelled on the key read | 500 `internal_error`; nothing stored | ≤ busyTimeout = 5 s; 0 rows written |
| Book: INSERT (not past) | write-lock contention | waits on SQLite's busy timeout; on expiry 500 `internal_error`, nothing stored (atomic single statement) | ≤ busyTimeout = 5 s; rows 0 or 1 |
| Book: whole request | worst-case latency | at most 3 statements, each bounded, no retry loop (resource SELECT + INSERT + classifying SELECT, or resource SELECT + key SELECT) | ≤ 3 × busyTimeout = 15 s < writeTimeout = 30 s |
| Book: N concurrent requests, one free valid slot, distinct keys | race on the insert after validation | validation reads only immutable data (resources are never updated or deleted, no such operation exists) and the clock, so it cannot be invalidated between check and insert; the unique active-slot index picks exactly one winner (EC-001); losers 409 `slot_taken` | 1 active row per slot; each request ≤ 3 × busyTimeout |
| Book: retry / duplicate delivery after the slot started | same key, same payload, any number of repeats | past check → key lookup → original returned (200); insert never attempted | rows under the key stay 1 |
| Book: clock skew between check and insert | clock passes the slot start while the request is in flight | the clock is read once per request; a request judged not-past is inserted even if the slot starts before the INSERT commits | window ≤ request duration ≤ 3 × busyTimeout = 15 s |
| Book: crash mid-operation | process dies before the INSERT commits / after it commits | nothing stored, retry decides afresh / booking persists, retry replays (before slot start via insert-then-classify, after via the past-path lookup) | rows under the key ≤ 1 |
| Book: graceful shutdown (SIGTERM) | request in flight when the drain starts | http.Server.Shutdown waits; the request ends within its statement bound | ≤ 3 × busyTimeout = 15 s; shutdownTimeout = 10 s can expire first, then the process exits and the request falls into a crash row (rows under the key ≤ 1) |
| non-cooperative callee | a store statement ignoring cancellation | only callees are SQLite via database/sql (busyTimeout per statement) and the injected clock (a pure function); nothing is spawned, so nothing is abandoned | 0 abandoned goroutines |

## Worktree notes

Open decisions touching this card (do not answer them here): DEC-005 (the "in the past" boundary), DEC-012 (order of the late-replay check), DEC-016 (input-validation ACs).
- [Env] forge 2026.10.10
- [Drift gate] ⚠ unprocessed external drift: internal/booking/**.go, internal/httpapi/**.go (meta/drift-pending.yml, events through 2026-10-22; drift.gate default warn)
- [Run] 2026-10-22 dispatched by `run` (wave 5) on the still-open run 2026-10-21T17:24:03Z; CARD-008 deferred first by user decision (commit 349de59) so the scope gate's poached-sibling measurement does not count it; dispatcher.state top-level `wave:` still reads 1 (no writer advances it, add-active ignores --wave after the bootstrap), this card's Wave: 5 stands
- [Start gates] 0 forge 2026.10.10 >= min_version 2026.10.10; 1 status ready, Revision pending false; 1b no legacy block; 1c drift warn (above); 1d States: enumerable — 8-row state|output|channel|test table present, no blank column; 2 Depends on CARD-005, CARD-006 done; 4b make_probe start-gate exit 0 (decision gate, source autodetect: `go build ./... && go test ./...`, require_gate false); 5 fork-point interactive → main 349de59; worktree ../booking-demo-CARD-007, branch card/007-booking-validation; model: Complexity architectural → heavy from quality profile (opus)
- STRUCTURE: validation order in `booking.Service.Book` is shape (empty key, empty customer name, slot start not on the UTC half hour → `ErrInvalidRequest`, 400) → resource read through COMP-002 (unknown → `ErrUnknownResource`, 404) → working hours (`ErrOutsideWorkingHours`, 422) → past check last (start < now in integer UTC epoch seconds), which alone triggers the key lookup (`ErrSlotInPast`, 422 / replay 200 / `ErrKeyConflict` 409) — ADR-0020 confines the lookup to "when validation rejects a request because its slot is in the past"; the contract's Validation paragraph puts shape errors first; no ADR (0011/0015/0016/0018/0020) states a contrary order. Pinned by TestService_BookValidatesInOrderAndOnlyThePastSlotReadsTheKey (unit, counts key reads) and TestBookSlot_OnlyThePastSlotRejectionConsultsTheStoredKey (API). Consequence, declared: a stored key reused with a different payload that is also malformed / unknown-resource / outside hours gets that validation error, not 409 (ADR-0020 over the contract's general "a request whose key is stored is resolved from the stored booking" sentence).
- STRUCTURE: booking reads working hours through a consumer-side port `booking.Resources` (`Get(ctx, id) (catalog.Resource, error)`), implemented by `*catalog.Catalog`; `NewService(repo, resources, now)` — mirrors components-booking.puml "COMP-004 → COMP-002 reads working hours to validate the slot" (DEC-015 left open); booking never reads the resource table through its own Repository.
- STRUCTURE: the slot-inside-hours check (`insideWorkingHours`, private in internal/booking) only applies the catalog's documented reading of `catalog.Interval` (half-open minutes of the UTC day, `catalog.EndOfDay` = 24:00 exclusive); catalog has no exported slot-containment function, and adding one would be an edit outside Touches, so none was added — the shape rule (INV-005) stays catalog's alone. If CARD-008 needs the same check, moving it to a `catalog.Resource` method is the natural refactor.
- STRUCTURE: one key-judging function `Service.resolveStoredKey(ctx, want, absent error)` (renamed from CARD-005's `classify`) serves both the rejected-insert path (absent = `ErrSlotTaken`) and the past path (absent = `ErrSlotInPast`) — one lookup, one `SamePayload` comparison (ADR-0015).
- STRUCTURE: alignment = `CanonicalSlotStart(t).Equal(t)` (whole epoch second, multiple of 1800, no sub-second part; zone spelling free). CARD-003's flooring is reversed: a non-aligned start is 400 and nothing stored; another zone's spelling of an aligned instant is still normalised to canonical UTC.
- SCOPE+ cmd/booking/main.go — DI wiring: `catalog.New(st)` hoisted into `cat` and passed to `booking.NewService(st, cat, time.Now)` (3-line change, required by the new constructor).
- SCOPE+ internal/booking/README.md, internal/httpapi/README.md — text only, describe the validation order and the new error mapping.
- Test generators (CARD-005 follow-up): `withinHalfHour` (random sub-slot offset) replaced by `inAnyZone` (same aligned instant, random zone) in property_test.go / replay_property_test.go; `newResource` now opens all 7 days 00:00-24:00 so the generators' slots stay valid; `TestProperty_Booking_SlotStartStoredAsCanonicalEpochSeconds` now draws an aligned start and a clock value at or before it; `TestBookSlot_NormalisesSlotStartToCanonicalUTC` books 11:00+02:00 instead of 11:17:42+02:00. EC-002..004 and AC-055/056/058 stay green and now exercise the past-path lookup (mutation: making the past path return `ErrSlotInPast` without the lookup fails IdempotentUnderRetry, KeyReuseWithDifferentPayload and AC-055).
- Summary (CARD-007): `booking.Service.Book` validates before insert (order above); new errors `ErrUnknownResource`, `ErrOutsideWorkingHours`, `ErrSlotInPast`; `ErrInvalidRequest` now also for empty name / non-aligned start; `httpapi.BookSlot` maps them to 404 not_found, 422 slot_outside_working_hours, 422 slot_in_past (contract api/openapi.yaml unchanged, no regeneration). Tests: 8 AC API tests with the card's exact names + order test (internal/httpapi/validation_test.go); unit tests for order / no key read on the normal path (counting fake) / past-path resolution / error passthrough / `insideWorkingHours` / `inPast` (internal/booking/booking_test.go); property tests TestProperty_Booking_PastCheckUsesInjectedClock (EC ADR-0013/R1, 400 cases, sub-second and ±1 s boundaries, clock decades from real time, plus the AST no-`time.Now` check) and TestProperty_Booking_AcceptedOnlyInsideWorkingHoursAndNotInPast (INV-002, 300 random schedules, independent epoch-second restatement) in internal/booking/validation_property_test.go; EC-001 TestProperty_Booking_NoOverlapUnderConcurrency now mixes ~20% off-half-hour requests into the concurrent batch (must be ErrInvalidRequest and never win or block a slot). Mutations checked: `<` → `<=` and `<` → `time.Before` in the past check, dropping the past-path lookup, widening the hours check — each fails ≥ 2 tests.
- Gates: `make build test vet` green; `go test -count=1 -race ./...` green; `go generate ./...` no drift; TestBookSlot_50ConcurrentRequestsExactlyOneWins PASS.
- Not changed: api/openapi.yaml, internal/catalog/**, internal/store/** (incl. migrations, G-2), internal/availability/** (G-1, does not exist). DEC-015 / DEC-017 untouched.
- Failure-matrix note: worst case per request is now 3 statements (≤ 3 × busyTimeout = 15 s), which exceeds shutdownTimeout = 10 s (cmd/booking/main.go, unchanged) — declared in the matrix; a request still in flight at the drain deadline falls into a crash row (rows under the key ≤ 1).
- [Commit] 62ed082 — feat(booking): validate working hours, past slots, alignment and unknown resource (forge:commit stage_card_paths.py exit 0; nothing under meta/ staged)
- [Review entry] 2026-10-22 — blocker check: no [BLOCKER line (worktree copy); empty-implementation guard: 1 commit main..HEAD (62ed082); sync-notes appended 13; Failure matrix section copied verbatim into the main card; review-fatigue: total_rounds 0 / ceiling 6 — ok; Complexity: architectural → full apparatus (max_cycles 3, skeptic cap 6, holds cap 3); review.require_gate false (gate source/lint/coverage/vacuum blocks inactive)
- [Scope] cmd/booking/main.go, internal/booking/README.md, internal/booking/booking.go, internal/booking/booking_test.go, internal/booking/property_test.go, internal/booking/replay_property_test.go, internal/booking/validation_property_test.go, internal/httpapi/README.md, internal/httpapi/api_test.go, internal/httpapi/bookings.go, internal/httpapi/cancel_test.go, internal/httpapi/validation_test.go
- [Scope gate] ⚠ grown: +COMP-007 · 3 files outside Touches (cycle 1, review_cycle_ops.py scope-gate exit 1, verdict grown): excess 3/12 = 0.25 (cmd/booking/main.go DI wiring, internal/booking/README.md, internal/httpapi/README.md — all SCOPE+ noted by the agent); comp_spread +COMP-007 (cmd/booking); poached none (CARD-008 deferred, no ready sibling in wave 5); guardrail_hits none; conflict.hotspots empty — GROWN handling: SCOPE NOTE handed to the reviewer, no escalation
- [Build gate] PASSED (impact: 4 pkgs, 2s) — impact set meta/impact/impact-set-349de59d4b9a-62ed08242ca3-worktree.json (underivable []): cmd/booking, internal/booking, internal/httpapi + reverse deps internal/store; cycle_test_cmd `go build ./... && go test ./cmd/booking ./internal/booking ./internal/httpapi ./internal/store`
- [System contract] fresh lens (system_rules.py --card CARD-007) matches the card's 11 rules (ADR-0010/R1, ADR-0012/R1, ADR-0013/R1, ADR-0015/R1, CON-007, CON-008, INV-001..INV-005) — no refresh
- [Review 1/3] Score: 8.5 — crit: 0, imp: 1 (FULL review of main 349de59...62ed082; review agent (opus, fresh context) ran forge:review via the Skill tool; brief deviation: the rendered brief (assemble-brief --render-check exit 0, 0 unresolved) was handed to the agent by file path rather than inlined; Important F: api/openapi.yaml bookSlot Idempotency/Slots paragraphs say a stored key is resolved first (409 on a different payload) and reserve 422 slot_outside_working_hours for a key that is not stored, while the code validates resource/hours before any key read (ADR-0020) — contract text drifted; 1 Minor: TestProperty_Booking_PastCheckUsesInjectedClock draws slots with replacement on one resource (latent ErrSlotTaken collision); 3 out-of-scope: insideWorkingHours restates catalog.Interval semantics, shutdownTimeout 10 s < 15 s worst case (declared), dead INV-004 check ref TestBookSlot_RepeatAfterCancelCreatesNoNewBooking (predates the diff; architect station))
- [Review sync] 1 report(s) → meta/review/ (20261022T085027Z-CARD-007-cycle1.yml; report-guard: current_report_present)
- [Review 1/3] parse-review verdict gate_failed (score 8.5, crit 0, imp 1, minor 1; severity_gate); parse_review --report accepted, security: applicable (0 findings); carry-forward: no-previous-report (accepted); Step 8h: 11/11 card rules addressed — 10 holds, 1 unchecked (check_ref_missing: INV-004), 0 violated
- [Adversarial] F (contract text vs validation-before-key order) CONFIRMED — skeptic (opus, read-only, worktree): api/openapi.yaml:178-187 reads as claimed; booking.go Book returns 404/422 before any key read and validation_test.go pins it; ADR-0020 (lookup_first rejected), the card text and INV-002 agree with the code, so the contract prose is the one document out of line; fix owner: contract description text (api/openapi.yaml is a kanban-card artifact of CARD-001, not meta/architecture), no code change
- [Severity gate 1/3] Score >= threshold but 0 critical / 1 important findings — fix mandatory
- STRUCTURE: validation order in `booking.Service.Book` is shape (empty key, empty customer name, slot start not on the UTC half hour → `ErrInvalidRequest`, 400) → resource read through COMP-002 (unknown → `ErrUnknownResource`, 404) → working hours (`ErrOutsideWorkingHours`, 422) → past check last (start < now in integer UTC epoch seconds), which alone triggers the key lookup (`ErrSlotInPast`, 422 / replay 200 / `ErrKeyConflict` 409) — ADR-0020 confines the lookup to "when validation rejects a request because its slot is in the past"; the contract's Validation paragraph puts shape errors first; no ADR (0011/0015/0016/0018/0020) states a contrary order. Pinned by TestService_BookValidatesInOrderAndOnlyThePastSlotReadsTheKey (unit, counts key reads) and TestBookSlot_OnlyThePastSlotRejectionConsultsTheStoredKey (API). Consequence, declared: a stored key reused with a different payload that is also malformed / unknown-resource / outside hours gets that validation error, not 409 (ADR-0020 over the contract's general "a request whose key is stored is resolved from the stored booking" sentence). Review cycle 1 (F-001): the bookSlot description of api/openapi.yaml was amended to state this order, so the contract no longer says otherwise.
- Summary (CARD-007): `booking.Service.Book` validates before insert (order above); new errors `ErrUnknownResource`, `ErrOutsideWorkingHours`, `ErrSlotInPast`; `ErrInvalidRequest` now also for empty name / non-aligned start; `httpapi.BookSlot` maps them to 404 not_found, 422 slot_outside_working_hours, 422 slot_in_past (contract schemas, enums and response codes unchanged; the bookSlot description text was amended in review cycle 1, F-001; regeneration produces no drift). Tests: 8 AC API tests with the card's exact names + order test (internal/httpapi/validation_test.go); unit tests for order / no key read on the normal path (counting fake) / past-path resolution / error passthrough / `insideWorkingHours` / `inPast` (internal/booking/booking_test.go); property tests TestProperty_Booking_PastCheckUsesInjectedClock (EC ADR-0013/R1, 400 cases, sub-second and ±1 s boundaries, clock decades from real time, plus the AST no-`time.Now` check) and TestProperty_Booking_AcceptedOnlyInsideWorkingHoursAndNotInPast (INV-002, 300 random schedules, independent epoch-second restatement) in internal/booking/validation_property_test.go; EC-001 TestProperty_Booking_NoOverlapUnderConcurrency now mixes ~20% off-half-hour requests into the concurrent batch (must be ErrInvalidRequest and never win or block a slot). Mutations checked: `<` → `<=` and `<` → `time.Before` in the past check, dropping the past-path lookup, widening the hours check — each fails ≥ 2 tests.
- Not changed: internal/catalog/**, internal/store/** (incl. migrations, G-2), internal/availability/** (G-1, does not exist). DEC-015 / DEC-017 untouched.
- SCOPE+ api/openapi.yaml — cross-file consequence of this diff (CARD-098 FILE SCOPE): review cycle 1 F-001, bookSlot description text only (new "Order of checks" paragraph per ADR-0011/ADR-0020; Idempotency and Slots paragraphs, 409 idempotency_key_conflict and 422 slot_outside_working_hours descriptions qualified accordingly; Past slots paragraph unchanged). No schema, enum or response code changed; `go generate ./...` leaves internal/httpapi/api.gen.go unchanged (the generator does not embed the spec).
- [Fix cycle 1] F-002: TestProperty_Booking_PastCheckUsesInjectedClock draws each slot at most once on its shared resource (redraw on repeat), same 400 cases and boundaries.
- [Fix 1] forge:fix agent (opus — architectural card → heavy): FIXED F-001 — test: TestContract_BookSlotDeclaresEveryOutcome, TestContract_SpecIsValidOpenAPI, TestContract_GeneratedCodeMatchesSpec, TestBookSlot_OnlyThePastSlotRejectionConsultsTheStoredKey (api/openapi.yaml bookSlot description only: new "Order of checks" paragraph citing ADR-0011/ADR-0020; schemas/enums/codes unchanged; go generate no drift); FIXED F-002 — test: TestProperty_Booking_PastCheckUsesInjectedClock (draws each slot at most once)
- [Fix 1] FIX PRE-GATE: 5/5 named tests re-run by the orchestrator, all PASS; declarations: 0 updated, 0 confirmed, 2 none
- [Build gate] PASSED (impact: 4 pkgs, 2s) — after fix 1
- [Fix 1] fix delta committed 5883404 (api/openapi.yaml, internal/booking/validation_property_test.go; no meta/ path) so cycle 2 reviews a clean tree
- [Scope gate] cycle 2 (review_cycle_ops.py scope-gate exit 1): still grown — excess 4/13 = 0.31 (+ api/openapi.yaml, SCOPE+ from fix 1 F-001), comp_spread +COMP-007, poached none, guardrail_hits none; family-mandate cycle 2: effective on (interactive); confirmation mode not eligible (scope verdict grown) → FULL review
- [Review 2/3] Score: 9.5 — crit: 0, imp: 0 (FULL review of main 349de59...5883404, opus fresh context, forge:review via Skill tool; brief by file path, assemble-brief --render-check exit 0; VERIFY F-001 ✓ resolved, F-002 ✓ resolved; 0 new findings; 3 out-of-scope carried from cycle 1)
- [Review sync] 2 report(s) → meta/review/ (…-cycle1.yml, 20261022T085720Z-CARD-007-cycle2.yml; report-guard: current_report_present)
- [Review 2/3] parse-review verdict ok (score 9.5, crit 0, imp 0, minor 0, out_of_scope 0); parse_review --report accepted, security: applicable (0 findings); carry-forward: judged, accepted (F-001/F-002 resolved in previous_findings); Step 8h: 11/11 card rules addressed — 10 holds, 1 unchecked (check_ref_missing: INV-004), 0 violated
- [Family check 2] none
- [Build gate] correction: the reviewer reports the cycle-2 impact set as underivable (reverse_deps manifest_changed — api/openapi.yaml changed in fix 1), so the post-fix gate should have degraded to the full suite; the orchestrator re-ran it full at 5883404: `go build ./... && go test ./...` — cmd/booking, booking, catalog, httpapi, store ok (1s, cached) — PASSED (full)
- [Review 2/3] Score: 9.5 ✓ threshold reached + no critical/important
- [Adversarial] none — 0 gating findings this cycle, nothing to refute
- [Minor triage] verified: sampled 0/0 Minor finding(s), rationale recorded (0 Minor findings this cycle — EC-070 zero boundary)
- [8h spot-check] precondition: worktree held only bookkeeping (M meta/kanban/cards/CARD-007.md, untracked meta/review/*.yml, meta/impact/) — fix delta committed 5883404; one skeptic agent (opus) processed the three holds sequentially; porcelain unchanged afterwards
- [8h spot-check] 3/3 sampled holds reproduced (ADR-0010/R1, INV-001, INV-002) — TestBooking_CannotReactivate, the 5 INV-001 tests (+ TestProperty_Booking_NoOverlapUnderConcurrency -race) and the 5 INV-002 tests + TestProperty_Booking_AcceptedOnlyInsideWorkingHoursAndNotInPast re-run green; mutation inPast `<` → `<=` killed (TestBookSlot_AcceptsSlotStartingExactlyNow and the property fail), original bytes restored (shasum identical)
- [AC/EC check] fresh-context agent (sonnet) at 5883404416d4dc6724963e3df78f304cf67648e4: AC-015, AC-016, AC-017, AC-020, AC-050, AC-051, AC-052, AC-057 ✓ demonstrated (named tests in internal/httpapi/validation_test.go --- PASS, go test -count=1 -v; each asserts the 4xx/404 JSON code and nothing stored, AC-051 counts exactly 1 persisted booking for Ann at Monday 09:00); EC-001 ✓ demonstrated (TestProperty_Booking_NoOverlapUnderConcurrency --- PASS, 25 seeded iterations of 2-40 concurrent requests over 1-4 slots incl. off-half-hour starts); EC(ADR-0013/R1) ✓ demonstrated (TestProperty_Booking_PastCheckUsesInjectedClock --- PASS, 400 seeded draws incl. -1 s/0/+1 s offsets, both outcomes required, injected clock read every draw); G-1 ✓ demonstrated, G-2 ✓ demonstrated (structural: git diff --name-only main...HEAD + git status --short filtered for internal/availability/**.go and internal/store/migrations/** — no hit)
- [AC/EC check] All criteria/constraints ✓ (head-sha: 5883404416d4dc6724963e3df78f304cf67648e4) (evidence): the agent's verdicts above — 8 ACs, 2 ECs and 2 Gs demonstrated at this HEAD
- [Docs] forge:readme step: changed dirs internal/booking, internal/httpapi, cmd/booking, api — internal/booking/README.md and internal/httpapi/README.md already updated in 62ed082 (validation order, error mapping); cmd/booking 3-line wiring and api/openapi.yaml description text need no README change; no change
- [Commit] no new commit — the card's commits 62ed082 (implementation) and 5883404 (cycle-1 fix delta) already hold code, tests, contract text and READMEs; the review/AC phase produced no further change
- [Merge gate] dependency 2026-10-22T09:01:27Z PASS (deps:271c2f681bb1a2bf7bbed0526d0cdc3d62a8d23dbac17cfac7891ca0aeee1a72, mode=interactive, scan=scanned) — 0 blocking, 0 allowlisted, 0 non-blocking finding(s)
- [Merge gate] PASS (tree-sha 9bf7fefa25cc519e5f258e8775e2ac6e65c3da02, scope=full, 2s, cmd="go build ./... && go test -count=1 ./...")
- [Merge gate] dependency cross-check: make audit run in the worktree at HEAD 5883404 (AUDIT_DIR in the session scratchpad, exit 0) → govulncheck.status ok, licenses.status ok; 0 govulncheck finding objects; licences MIT ×5, BSD-3-Clause ×7, Apache-2.0 ×1 (13 modules, all allowlisted); licenses.err only go-licenses warnings (non-Go asm in golang.org/x/sys, empty main-module version); go.mod/go.sum unchanged by the card
- [Deferral scan] 0 hits in the added lines (later card/TODO/FIXME/HACK/XXX/deferred/for now/aspirational/not implemented/placeholder/stub); nothing captured to the backlog
- [Done] 2a gates: (i) Review score 9.5 + [AC/EC check] ✓ present; (i-sha) checkmark-guard ok (head_sha_matches 5883404); (ii) 2 canonical reports meta/review/*CARD-007-cycle{1,2}.yml, report-guard ok for both (current_report_present); (iii) vacuum-gate not_applicable (not_on_vacuum_path); 2b deferral scan 0 hits; 2c dependency gate PASS (scan=scanned); 2d no meta/kanban or meta/review path committed on the card branch; 3a placement target (interactive)
- [Done] 2026-10-22 — merge lock acquired (receipt CARD-007); review_copies exit 0 (nothing removed, 2 kept); rebase step: card branch already based on main tip 349de59, forge:merge `git rebase --autostash main` was a no-op (no fetch: nothing is ever pushed/pulled in this project); post-rebase gate executed as `go build ./... && go test -count=1 ./...` exit 0 on branch tree 6b6fb5b9fb1101527be0e530593842ece5a7fb7a (2s; 162 top-level tests passed, 0 failed by a follow-up `go test -count=1 -json ./...` count); merged --no-ff as 1dc0cdc (wave_branch.py merge, placement target); changelog 9b25192 (commit-bookkeeping); .merge.lock released
- [Done] 5b: sync-card-body exit 0 (placement=target, changed=false — every worktree note line and the Failure matrix were already on main); review reports and impact-set JSON copied (byte-identical); then the worktree's card copy was restored to HEAD and its verified untracked copies deleted, worktree ../booking-demo-CARD-007 removed without --force, branch deleted with -d
- [Trace] 7c skipped — writing meta/architecture/trace.yml is outside this run's mandate (operator rule: kanban never edits meta/architecture/); FR-005 evidence tests are listed in the [AC/EC check] notes
- [Rebase dependents] step 8: CARD-009 depends on CARD-007 but is blocked (readiness) with no worktree; CARD-008 does not depend on CARD-007 and is deferred with no worktree — nothing to rebase
- [Architect note] out-of-scope, routed to the architect station (not acted on): INV-004's check ref TestBookSlot_RepeatAfterCancelCreatesNoNewBooking names no existing test (reported by both review cycles); the bookSlot contract text was aligned with ADR-0020 in 5883404 — an architect may want trace/handoff to mention the order of checks
- [Follow-up for CARD-008] insideWorkingHours (internal/booking) restates catalog.Interval semantics; if availability needs the same slot-containment rule, move it to a catalog.Resource method rather than writing a third copy
