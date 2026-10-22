# CARD-005: Idempotent booking replay with insert-then-classify

**Status:** done
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
**Review score:** 9.5 (cycle 1/3)
**Started:** 2026-10-22T08:06:01Z
**Closed:** 2026-10-22T08:23:41Z
**Actual:** 0.0d
**Merge commit:** 5478983eebf8b55e88eb79d9817c6643a5504e50
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

## Failure matrix

Declared before code. `busyTimeout` = 5 s (internal/store/store.go), `shutdownTimeout` = 10 s and `writeTimeout` = 30 s (cmd/booking/main.go). The booking path is `Service.Book`: empty-key check → ONE autocommit INSERT → on a unique violation only, ONE autocommit SELECT by key (ADR-0011, ADR-0012).

| operation / boundary | failure mode | declared behavior | numeric bound |
|---|---|---|---|
| Book: key validation | poison input: empty idempotency key | 400 `validation_error` before any statement; the classifier is never reached | 0 statements, 0 rows written |
| Book: key validation | poison input: oversized key (NG-2) | not rejected by this card; the body cap of NewHandler bounds it | key ≤ 1 MiB (`maxRequestBodyBytes`) |
| Book: INSERT | write-lock contention (timeout) | the statement waits on SQLite's busy timeout; if it expires the request is 500 `internal_error` and nothing is stored (single statement is atomic) | wait ≤ busyTimeout = 5 s; rows written 0 or 1 |
| Book: classifying SELECT | timeout / I/O error / ctx cancelled | 500 `internal_error`; nothing was stored for this request (its insert was already rejected); a retry is classified again | wait ≤ busyTimeout = 5 s (in WAL a read normally takes no lock: ≈ 0); rows written 0 |
| Book: whole request | worst-case latency | INSERT then SELECT, never more statements, no retry loop | ≤ 2 × busyTimeout = 10 s < writeTimeout = 30 s |
| Book: retry / duplicate delivery, same key, same payload | any number of repeats, any time, also after the slot started and after cancellation | the insert is rejected by the key index, the read finds the stored booking, the original is returned (200) in its current state | rows under the key stay exactly 1; extra statements per repeat = 2 |
| Book: retry, same key, different payload | resource id, canonical slot start or customer name (bytes) differ | 409 `idempotency_key_conflict`; the stored booking is not changed | rows written 0, rows changed 0 |
| Book: N concurrent requests, same key, same payload | race on the insert | exactly one INSERT commits (unique key index); the N-1 losers' INSERT runs after it under SQLite's single writer, so their following SELECT sees the winner and returns its id; no lock/busy error because no transaction reads then writes | 1 row; N-1 classifying reads; each request ≤ 2 × busyTimeout |
| Book: violation, nothing under the key | slot taken under another key | 409 `slot_taken`; nothing stored under the new key | rows written 0 |
| Book: concurrent same key, different payloads | DEC-017 (open, NG-3) | not designed for here: whichever INSERT commits first owns the key, the others classify against it (conflict or replay); a request that lost on the slot index and reads after a different-payload twin committed the key reports `idempotency_key_conflict` instead of `slot_taken` | rows under the key ≤ 1 (key index) |
| Book: crash mid-operation | process dies before the INSERT commits | nothing stored; a retry inserts | rows 0 |
| Book: crash mid-operation | process dies after the INSERT commits, before the response | the booking persists; the client's retry with the same key replays it (200, same id) | rows under the key = 1 |
| Book: client disconnect / ctx cancelled | handler ctx cancelled during a statement | the statement is interrupted or completes atomically; the response is lost; a retry resolves as one of the two crash rows above | rows under the key ≤ 1 |
| Book: graceful shutdown (SIGTERM) | request in flight when the drain starts | http.Server.Shutdown waits for it; it finishes within its two-statement bound | remaining ≤ 2 × busyTimeout = 10 s ≤ shutdownTimeout = 10 s; past that the process exits and the request falls into a crash row |
| non-cooperative callee | a store statement that ignores cancellation | not applicable beyond the bounds above: the only callee is SQLite through database/sql, bounded by busyTimeout per statement; no goroutine is spawned by this path, so nothing is abandoned | 0 abandoned goroutines |
| event emission on replay | DEC-014 (open) | no domain event exists in the code; a replay emits nothing | — |

## Worktree notes

Open decisions touching this card (do not answer them here): DEC-012 (order of the key lookup against the not-in-the-past check for a late replay; the handoff asks for it to be settled before this card), DEC-016 (empty or oversized keys), DEC-017 (concurrent same-key requests with different payloads), DEC-014 (event re-emission on a no-op replay).
- [Env] forge 2026.10.10
- [Drift gate] ⚠ unprocessed external drift: internal/store/*.go, internal/httpapi/**.go (meta/drift-pending.yml events 2026-10-22T06:30:37Z, 07:52:02Z, 08:02:08Z; drift.gate default warn)
- [Run] 2026-10-22 dispatched by `run` (wave 4) on the still-open run 2026-10-21T17:24:03Z; before dispatch the stale `escalated` entry of CARD-004 (done, merged 851e24a) was cleared with `dispatcher_state.py unpark` and escalations.md regenerated (0 parked); dispatcher.state top-level `wave:` still reads 1 (no writer advances it), the active entry carries wave 4 and this card's Wave: 4 stands
- [Start gates] 1 status ready, Revision pending false; 1b no legacy block; 1d States: enumerable — 8-row state|output|channel|test table present, no blank column; 2 Depends on CARD-004 done; 4b make_probe start-gate exit 0 (decision gate, source autodetect: `go build ./... && go test ./...`, require_gate false); 5 fork-point interactive → main 8454b48; worktree ../booking-demo-CARD-005, branch card/005-idempotent-replay
- STRUCTURE: the replay outcome travels as a third return value, `Service.Book(ctx, req) (Booking, replayed bool, err error)` — the HTTP layer needs 201 vs 200 and "replayed" is a fact of the call, not an attribute of the aggregate; a bool is the smallest carrier (no result type, no sentinel "success" error).
- STRUCTURE: the classifier is a private `Service.classify` called only on `ErrNotUnique`; the only new port method is `Repository.GetBookingByIdempotencyKey` (one autocommit SELECT, ErrNotFound when absent) — insert-then-classify of ADR-0011, no read on the normal path (ADR-0020 rejects lookup_first). CARD-007's past-slot rejection path can reuse `classify` for its key lookup (ADR-0020).
- STRUCTURE: payload sameness lives on the aggregate as `Booking.SamePayload(o Booking)` (resource id ==, SlotStart.Equal on canonical starts, customer name == i.e. byte-for-byte) — the single definition of ADR-0015's payload relation.
- STRUCTURE: request rejections before any statement wrap `booking.ErrInvalidRequest` with the reason (mirrors `catalog.ErrInvalidResource`); httpapi maps it to 400 `validation_error`. Later validation rules (empty customer name, CARD-007) should wrap the same sentinel.
- STRUCTURE: new domain errors `ErrKeyConflict` → 409 `idempotency_key_conflict`. The contract (api/openapi.yaml) already declared 200 replay, 409 `idempotency_key_conflict` and 400 `validation_error`; no contract change, `go generate ./...` shows no drift.
- SCOPE+ internal/booking/README.md, internal/store/README.md, internal/httpapi/README.md — doc lines updated for the classifier, the new port method and the error mapping (outside Touches `**.go`; text edits only).
- Changed existing tests (in scope): booking_test.go fake gains `insertErr`/`inserts` and the new port method; property_test.go and cancel_property_test.go call sites take the third return value; api_test.go `newAPIEnv` now delegates to `newAPIEnvWithClock` (movable clock for AC-055/056/058).
- Not built (open decisions): DEC-014 — no domain event exists, a replay emits nothing; DEC-017 — concurrent same-key different-payload inserts are not designed for (see Failure matrix row). Not-in-the-past check is CARD-007's; AC-055/056/058 pass via insert-then-classify with the injected clock moved past the slot start.
- Summary: insert-then-classify (ADR-0011/0012) implemented in booking.Service.Book; empty key rejected before insert (ADR-0018); store adapter `GetBookingByIdempotencyKey`; BookSlot handler answers 200/201/400/409 slot_taken/409 idempotency_key_conflict. Tests: 17 AC API tests (internal/httpapi/replay_test.go) + 1 replay-body test, 5 EC property tests on real SQLite (internal/booking/replay_property_test.go), classifier unit tests, store integration tests. Mutation check: replacing byte-equality of names with trim+EqualFold fails 6 tests.
- Gates: `make build test vet` ok; `go test -count=1 -race ./...` ok; `go generate ./...` no drift; TestBookSlot_50ConcurrentRequestsExactlyOneWins PASS.
- [Review entry] 2026-10-22 — blocker check: no [BLOCKER line (worktree copy); empty-implementation guard: 1 commit main..HEAD (18b7e94); sync-transition in_progress→review exit 0; sync-notes appended 10; review-fatigue: total_rounds 0 / ceiling 6 — ok; Complexity: architectural → full apparatus (max_cycles 3, skeptic cap 6, holds cap 3); review.require_gate false (gate source/lint/coverage/vacuum blocks inactive)
- [Scope] internal/booking/README.md, internal/booking/booking.go, internal/booking/booking_test.go, internal/booking/cancel_property_test.go, internal/booking/property_test.go, internal/booking/replay_property_test.go, internal/httpapi/README.md, internal/httpapi/api_test.go, internal/httpapi/bookings.go, internal/httpapi/replay_test.go, internal/store/README.md, internal/store/replay_test.go, internal/store/repository.go
- [Scope gate] in_scope (cycle 1, review_cycle_ops.py scope-gate exit 0): poached none (no ready sibling in wave 4); excess 3/13 = 0.23 (internal/booking/README.md, internal/httpapi/README.md, internal/store/README.md — SCOPE+ noted by the agent); comp_spread 0; guardrail_hits none; conflict.hotspots empty
- [Build gate] PASSED (full, 11s) — impact set (meta/impact/impact-set-8454b48ab954-18b7e94ebe98-worktree.json, underivable []) covers booking, httpapi, store and reverse deps cmd/booking, catalog = every package, so the full command was run: go build ./... && go vet ./... && go test -count=1 -race ./... — cmd/booking, booking, catalog, httpapi, store ok; make build test vet green
- [System contract] fresh lens (system_rules.py --card CARD-005) matches the card's 11 rules (ADR-0010/R1, ADR-0012/R1, ADR-0013/R1, ADR-0015/R1, CON-007, CON-008, INV-001..INV-005) — no refresh
- [Review 1/3] Score: 9.5 — crit: 0, imp: 0 (FULL review of main 8454b48...18b7e94; review agent (opus, fresh context) ran forge:review via the Skill tool; brief deviation: the rendered brief (assemble-brief --render-check exit 0) was handed to the agent by file path rather than inlined; 1 Minor: no test pins ADR-0020's no-key-read-on-the-normal-path (fakeRepo does not count GetBookingByIdempotencyKey); 1 out-of-scope: replay property generators send misaligned slot starts that CARD-007's alignment rule will reject — CARD-007 must order alignment vs classification and update the generators)
- [Review sync] 1 report(s) → meta/review/ (20261022T081847Z-CARD-005-cycle1.yml; report-guard: current_report_present)
- [Review 1/3] parse-review (on the reviewer's severity sections + score lines, transcribed to a scratch file) verdict ok (score 9.5, crit 0, imp 0, minor 1, out_of_scope 1); parse_review --report accepted, security: applicable (0 findings); carry-forward: no-previous-report (accepted); Step 8h: 11/11 card rules addressed — 8 holds, 3 unchecked (check_ref_missing: INV-002, INV-004; no_eligible_fact: INV-005), 0 violated
- [Review 1/3] Score: 9.5 ✓ threshold reached + no critical/important
- [Adversarial] none — 0 gating findings this cycle, nothing to refute
- [Minor triage] verified: sampled 1/1 Minor finding(s), rationale recorded (should_gate: false)
- [8h spot-check] precondition note: worktree held only bookkeeping (M meta/kanban/cards/CARD-005.md, untracked meta/review/*.yml, meta/impact/) — no code delta; one skeptic agent (opus) processed the three holds sequentially; porcelain unchanged afterwards
- [8h spot-check] 3/3 sampled holds reproduced (ADR-0010/R1, ADR-0012/R1, ADR-0013/R1) — TestBooking_CannotReactivate, TestProperty_Booking_ConcurrentReplaysNeverSeeLockErrors (-race) and TestProperty_Booking_SlotStartStoredAsCanonicalEpochSeconds re-run green; no BeginTx/Begin on the booking path, no new time.Now; the booking_idempotency_key_uq index is not partial, so a cancelled key cannot be re-inserted active
- [AC/EC check] fresh-context agent (sonnet) at 18b7e94ebe98a06fd4e553c061b43831e5118c55: AC-021, AC-022, AC-023, AC-038, AC-039, AC-040, AC-041, AC-043, AC-044, AC-045, AC-046, AC-047, AC-053, AC-055, AC-058, AC-056 ✓ demonstrated (named tests --- PASS, go test -count=1 -v); EC-002 ✓ demonstrated (TestProperty_Booking_IdempotentUnderRetry --- PASS, 40 seeded iterations of 1..12 repeats incl. clock past slot start and cancellation); EC-003 ✓ demonstrated (TestProperty_Booking_ConcurrentSameKeyYieldsOneBooking --- PASS, seeded random sizes of concurrent releases); EC-004 ✓ demonstrated (TestProperty_Booking_KeyReuseWithDifferentPayloadCreatesNothing --- PASS, 150 seeded iterations); EC(ADR-0012/R1) ✓ demonstrated (TestProperty_Booking_ConcurrentReplaysNeverSeeLockErrors --- PASS, seeded concurrent sizes); EC(ADR-0015/R1) ✓ demonstrated (TestProperty_Booking_PayloadComparedByteForByte --- PASS, 300 iterations over 10 name variants, oracle-checked); no ## Guardrails section → no G items. Orchestrator confirmed the multi-case loops of EC-003/EC-004/EC(ADR-0012/R1) (replay_property_test.go:118-129, 174-179, 219-227) where the agent had only sampled them
- [AC/EC check] All criteria/constraints ✓ (head-sha: 18b7e94ebe98a06fd4e553c061b43831e5118c55) (evidence): the agent's verdicts above — 16 ACs and 5 ECs demonstrated at this HEAD
- [Docs] forge:readme step: changed dirs internal/booking, internal/httpapi, internal/store — READMEs already updated in the card commit 18b7e94 (replay classification, GetBookingByIdempotencyKey, 200/409 mapping); no change
- [Commit] no new commit — the card's only commit 18b7e94 already holds code, tests and READMEs; the review cycle produced no code or docs change
- [Merge gate] dependency 2026-10-22T08:22:37Z PASS (deps:22f5be5f7156b766cc3a50c61ffd49b8f6082d68805fdf56aeca78803e35540a, mode=interactive, scan=scanned) — 0 blocking, 0 allowlisted, 0 non-blocking finding(s)
- [Merge gate] dependency cross-check: make audit run in the worktree at HEAD 18b7e94 (AUDIT_DIR in the session scratchpad, exit 0) → govulncheck.status ok, licenses.status ok; 0 govulncheck finding objects; licences MIT ×5, BSD-3-Clause ×7, Apache-2.0 ×1 (13 modules, all allowlisted); go.mod/go.sum unchanged by the card
- [Deferral scan] 0 hits in the added lines (later card/TODO/FIXME/HACK/XXX/deferred/for now/aspirational/not implemented/placeholder/stub); nothing captured to the backlog
- [Merge gate] PASS (tree-sha 5386bc9ce6f5820270a2e12ff42be2f6362fbeef, scope=full, 1s, cmd="go build ./... && go test ./...")
- [Done] 2a gates: (i) Review score 9.5 + [AC/EC check] ✓ present; (i-sha) checkmark-guard ok (head_sha_matches 18b7e94); (ii) 1 canonical report meta/review/20261022T081847Z-CARD-005-cycle1.yml, report-guard ok (current_report_present); (iii) vacuum-gate not_applicable (not_on_vacuum_path); 2b deferral scan 0 hits; 2c dependency gate PASS (scan=scanned); 2d no meta/kanban or meta/review path committed on the card branch; 3a placement target (interactive)
- [Done] 2026-10-22 — merge lock acquired (receipt CARD-005); review_copies exit 0 (nothing removed); rebase step: card branch already based on main tip 8454b48, forge:merge `git rebase --autostash main` was a no-op; post-rebase gate `go build ./... && go test ./...` exit 0 on branch tree bca3bcef0db8773f122e8b3f59876fe59d38ec9a (package results served from the go test cache for this same tree, 1s; 145 top-level tests passed, 0 failed by a follow-up `go test -json ./...` count); merged --no-ff as 5478983 (wave_branch.py merge, placement target); changelog bbdf6cf (commit-bookkeeping); .merge.lock released
- [Done] 5b: sync-card-body exit 0 (placement=target, changed=false — every worktree note line was already on main); the `## Failure matrix` section the implementation agent wrote only in the worktree copy was copied verbatim into the main card; review report copied (byte-identical); impact-set JSON copied to meta/impact/; then the worktree's card copy was restored to HEAD and its verified untracked copies deleted, worktree ../booking-demo-CARD-005 removed without --force, branch deleted with -d
- [Trace] 7c skipped — writing meta/architecture/trace.yml is outside this run's mandate (operator rule: kanban never edits meta/architecture/); FR-006 evidence tests are listed in the [AC/EC check] notes
- [Rebase dependents] step 8: CARD-007 and CARD-008 depend on CARD-005 but are `ready` with no worktree — nothing to rebase
- [Follow-up for CARD-007] (from review cycle 1 + implementation notes) order the alignment/past-slot checks against replay classification (ADR-0020: the key lookup only on the past-slot rejection path); `classify` maps "no booking under the key" to ErrSlotTaken, which on CARD-007's path must become the past-slot error; replay property generators send misaligned slot starts that the alignment rule will reject; consider pinning "no key read on the normal path" by counting GetBookingByIdempotencyKey calls in fakeRepo (Minor F of cycle 1)
