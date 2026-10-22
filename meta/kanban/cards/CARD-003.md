# CARD-003: Walking skeleton: bootstrap, minimal resource creation and booking one slot

**Status:** done
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
**Review score:** 9.0 (cycle 1/3)
**Started:** 2026-10-22T06:42:57Z
**Closed:** 2026-10-22T07:02:51Z
**Actual:** 0.0d
**Merge commit:** 4a8badf4869bffb21ef4ec20e236e62fb3cd81d9
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

## Failure matrix

Config values: `busyTimeout` = 5 s (internal/store, per statement); `shutdownTimeout` = 10 s; `readHeaderTimeout` = 5 s, `readTimeout` = 10 s, `writeTimeout` = 30 s, `idleTimeout` = 60 s (cmd/booking).

| Operation / boundary | Failure mode | Declared behavior | Numeric bound |
|---|---|---|---|
| POST /bookings — INSERT | N concurrent writers on one slot, distinct keys | One autocommit INSERT each, no read-then-write transaction (ADR-0012); the partial unique index picks the winner: 1 × 201, N−1 × 409 `slot_taken`; lock contention is absorbed by the busy handler | exactly 1 active row per (resource, canonical start); each INSERT waits ≤ `busyTimeout` = 5 s for the write lock; checkpoint: N = 50 → 0 busy/lock errors (AC-019) |
| POST /bookings — INSERT | write lock not obtained within `busyTimeout` (more than ~5 s of queued writes) | SQLITE_BUSY is not a unique violation → 500 `internal_error`, nothing stored | ≤ 5 s + statement time per request; 0 rows written by the failed request |
| POST /bookings | retry / duplicate delivery with the same key (any payload) | the key index rejects it; until CARD-005 adds insert-then-classify every unique violation is answered 409 `slot_taken`, so a same-payload replay gets 409, not 200 (stated gap, CARD-005 owns ADR-0011) | 0 extra rows per repeated key (unique index on key) |
| POST /bookings | crash mid-operation (process killed during the INSERT) | the statement is atomic; WAL recovery at the next `store.Open` keeps it fully committed or absent; the client sees a reset connection | 0 partial rows; ≤ 1 row per request |
| POST /bookings | client disconnects / request context cancelled | ctx reaches the driver, which interrupts the statement; it commits or rolls back atomically; the 500 goes to a dead connection | statement abandoned within the driver's interrupt latency; 0 partial rows |
| POST /bookings | poison input: unknown `resourceId` | foreign-key rejection → 500 `internal_error`, nothing stored (404 `not_found` is CARD-007's) | 0 rows |
| POST /bookings | poison input: malformed JSON, non-RFC 3339 `slotStart`, body > 1 MiB | rejected by NewHandler / generated decoder before the service: 400 `validation_error` / 413 `request_too_large` (CARD-001 plumbing) | 0 rows; body read ≤ 1 MiB |
| POST /bookings | non-aligned `slotStart` | normalised to the canonical UTC half-hour start (floor) before insert; rejection with 400 is CARD-007's | stored start = ⌊unix/1800⌋·1800 |
| POST /resources | crash mid-operation / statement failure after the resource row | resource row and working-hours rows are written in ONE write-only transaction (first statement is a write, so no snapshot upgrade); rollback on any error | 0 or 1 + k rows (k = intervals); never a resource without its hours |
| POST /resources | poison input: malformed `HH:MM`, unknown weekday | 400 `validation_error`, nothing stored | 0 rows |
| POST /resources | input the schema CHECK/PK rejects (misaligned bound, second interval on a weekday, end ≤ start, > 24:00) | transaction rolls back → 500 `internal_error`, nothing stored; 400 with full INV-005 validation is CARD-006's (NG-1) | 0 rows |
| POST /resources | write lock busy | first INSERT waits ≤ `busyTimeout`; later statements hold the lock | ≤ 5 s + execution |
| Unimplemented operations (list/get resource, availability, get/cancel booking) | called before their cards land | handler returns an error → 500 `internal_error` | constant time, no I/O |
| HTTP server | slow or stalled client | `ReadHeaderTimeout` 5 s, `ReadTimeout` 10 s, `WriteTimeout` 30 s, `IdleTimeout` 60 s | a stalled connection is closed after ≤ 10 s reading / 60 s idle |
| HTTP server | load beyond the 50-writer checkpoint | no admission control (no throughput target decided, DEC-007): every request holds one goroutine and at most one pooled SQLite connection; queued writers fail after `busyTimeout` with 500 | per request ≤ 5 s per statement; in-flight count unbounded (stated, not denied) |
| Startup | `BOOKING_DB_PATH` missing; store open/migration fails; listen address unusable | exit status 1 with one line on stderr, before or instead of serving; store closed if it was opened | migration waits ≤ `busyTimeout` for the write lock; 0 requests served |
| Graceful shutdown (SIGTERM / SIGINT) | in-flight requests | stop accepting, wait for in-flight requests via `http.Server.Shutdown` | ≤ `shutdownTimeout` = 10 s; each in-flight statement finishes within `busyTimeout` = 5 s, so a cooperative drain ends in ≤ 5 s + statement time |
| Graceful shutdown | non-cooperative handler (ignores cancellation, outlives 10 s) | `Shutdown` gives up, `Server.Close` drops the connections; the handler goroutine is ABANDONED, not stopped; `store.Close` (`sql.DB.Close`) then waits for statements already running | process exit ≤ 10 s + 5 s (busy timeout of the last running statement) + execution; exit status 1 when the drain timed out |

## Worktree notes

Open decisions touching this card (do not answer them here): DEC-005 (whether a slot already in progress is "in the past") and DEC-016 (empty customer name or key) are not exercised by this card's ACs; the past-slot and input checks land in CARD-007. DEC-017 (cancel racing a booking, concurrent same-key requests with different payloads) is not covered by this card's ACs.

- [Env] forge 2026.10.10
- [Drift gate] ⚠ unprocessed external drift: internal/store/*.go (meta/drift-pending.yml event 2026-10-22T06:30:37Z, range 5fdd66b..54118a9; drift.gate default warn)
- [Run] 2026-10-22 dispatched by run resume of run 2026-10-21T17:24:03Z; dispatcher.state top-level `wave:` still reads 1 (dispatcher_state.py has no writer that advances it on an existing file) — the entry carries wave 2 and this card's Wave: 2 stands
- STRUCTURE: domain packages `internal/catalog` (COMP-002) and `internal/booking` (COMP-004) each declare their own `Repository` port next to the aggregate; `internal/store` implements both in `repository.go` (store imports the domain, never the reverse) — mirrors the component diagram's catalog→store and booking→store relations and the store README's "consuming components own their ports".
- STRUCTURE: `httpapi.Server` (handlers.go) implements the generated `StrictServerInterface` over concrete `*catalog.Catalog` / `*booking.Service` (no service interfaces: one implementation, no context boundary). One file per OpenAPI tag (`resources.go`, `bookings.go`); operations of cards not landed yet live in `unimplemented.go` and return `errNotImplemented` → 500 `internal_error`. A later card moves its method out of that file into its tag file.
- STRUCTURE: domain → HTTP error mapping happens in the handler with the generated response types (`booking.ErrSlotTaken` → `BookSlot409JSONResponse{Code: ErrorCodeSlotTaken}`); unexpected errors are returned and become 500 via NewHandler.
- STRUCTURE: the store reports any unique-index rejection as `booking.ErrNotUnique` (sentinel, wrapped); `booking.Service.Book` is the single place that classifies it — today always `ErrSlotTaken`, CARD-005 adds the ADR-0011 classifying read there.
- STRUCTURE: canonical slot start = `booking.CanonicalSlotStart` (floor of epoch seconds to 1800, UTC); time conversion RFC 3339 ⇄ time.Time only in httpapi, time.Time ⇄ epoch only via store's `toEpoch`/`fromEpoch`.
- STRUCTURE: clock = `func() time.Time` passed to `booking.NewService`; bootstrap passes `time.Now`. No rule of this card reads it (the past-slot rule is CARD-007's); a static check inside `TestProperty_Booking_SlotStartStoredAsCanonicalEpochSeconds` forbids `time.Now/Since/Until` in production files of `internal/booking`.
- STRUCTURE: ids are `crypto/rand.Text()` (26-char base32, stdlib); nothing about their format is promised (DEC-011 open).
- STRUCTURE: bootstrap config is two env vars, `BOOKING_DB_PATH` (required) and `BOOKING_ADDR` (default `:8080`); timeouts are constants (no operational reason to vary yet).
- STRUCTURE: end-to-end tests build the real binary once in `TestMain` (cmd/booking/main_test.go) and drive it as a subprocess on a free loopback port with a fresh SQLite file; a lost port race is retried (≤ 3 attempts).
- STRUCTURE: API tests (internal/httpapi/api_test.go) run the real handler over the real store and validate every response against api/openapi.yaml with kin-openapi `openapi3filter.ValidateResponse` (IncludeResponseStatus on); `TestContract_ResponseCheckRejectsUndeclaredShapes` proves the check is not vacuous.
- SCOPE+ internal/httpapi/README.md, internal/store/README.md — factual updates (httpapi now imports the domain packages; store now implements the ports); new READMEs internal/booking/README.md, internal/catalog/README.md, cmd/booking/README.md next to the Touches globs. CLAUDE.md "Project state" (says there is no code yet) is outside scope and was NOT updated.
- Known gaps, deliberately left to their cards (declared in the Failure matrix): every unique violation is 409 `slot_taken`, so a same-key replay gets 409 instead of 200 (CARD-005); unknown resource → 500 via FK rejection (CARD-007 → 404); schema CHECK/PK violations on resource creation → 500 (CARD-006 → 400); non-aligned slot starts are floored, not rejected (CARD-007).
- No new dependencies: stdlib + modernc.org/sqlite + oapi-codegen runtime; tests use kin-openapi (already in go.mod). go.mod/go.sum unchanged.
- ADR-0012 fallback (`_txlock=immediate`) NOT needed: the AC-019 checkpoint shows 0 lock/busy errors.
- Summary: walking skeleton lands. `cmd/booking` boots from env, opens+migrates the store, serves the generated API with graceful SIGTERM shutdown. `POST /resources` persists a resource + working hours atomically (minimal path). `POST /bookings` normalises the start to the canonical UTC half hour and stores it with one autocommit INSERT; the partial unique index picks the winner; violation → 409 `slot_taken`. Tests: AC-001/014/018/019/037 + the four `TestProperty_Booking_*` ECs, unit/integration/API-contract tests. `make build test vet` green; `go test -race ./...` green; `TestBookSlot_50ConcurrentRequestsExactlyOneWins` passed 30/30 + 50/50 (-count) and 10/10 under -race: always 1 × 201, 49 × 409 `slot_taken`, 0 lock/busy errors.
[Commit] untracked outside Touches: cmd/booking/README.md, internal/booking/README.md, internal/catalog/README.md
- [Commit] 6aed539 — feat: walking skeleton; README.md files outside Touches staged explicitly (see SCOPE+ above).
- [Scope] cmd/booking/README.md, cmd/booking/main.go, cmd/booking/main_test.go, internal/booking/README.md, internal/booking/booking.go, internal/booking/booking_test.go, internal/booking/property_test.go, internal/catalog/README.md, internal/catalog/catalog.go, internal/catalog/catalog_test.go, internal/httpapi/README.md, internal/httpapi/api_test.go, internal/httpapi/bookings.go, internal/httpapi/handlers.go, internal/httpapi/resources.go, internal/httpapi/unimplemented.go, internal/store/README.md, internal/store/repository.go, internal/store/repository_test.go
- [Build gate] PASSED (impact: 5 pkgs — every package of the module, 5s; 107 tests)
- [Scope gate] ⚠ grown: +0 components · 5 files outside Touches (README.md in cmd/booking, internal/booking, internal/catalog, internal/httpapi, internal/store; excess 5/19 = 26%)
- [System contract] fresh lens (system_rules.py --card CARD-003) matches the card's 11 rules — no refresh
- [Review 1/3] Score: 9.0 — crit: 0, imp: 0
- [Review sync] 1 report(s) → meta/review/
- [Review 1/3] Score: 9.0 ✓ threshold reached + no critical/important (report parse_review accepted, security: applicable (0 findings); Step 8h: 11/11 card rules addressed — 6 holds, 5 unchecked (check_ref_missing: tests owned by CARD-004..CARD-007), 0 violated)
- [Adversarial] none — 0 gating findings this cycle, nothing to refute
- [Minor triage] verified: sampled 2/4 Minor finding(s), rationale recorded
- [8h spot-check] precondition note: worktree held only bookkeeping (M meta/kanban/cards/CARD-003.md, untracked meta/review/*.yml, meta/impact/) — no code delta; skeptic ran read-only (no mutation), sequentially
- [8h spot-check] 3/3 sampled holds reproduced (INV-001, ADR-0012/R1, ADR-0013/R1)
- [AC/EC check] All criteria/constraints ✓ (head-sha: 6aed539039944f54aa7197b7e50e536ac89feed1) (evidence):
  AC-001 ✓ demonstrated — evidence: TestCreateResource_Persists --- PASS (internal/httpapi)
  AC-014 ✓ demonstrated — evidence: TestBookSlot_PersistsBooking --- PASS (internal/httpapi)
  AC-018 ✓ demonstrated — evidence: TestBookSlot_RejectsAlreadyBookedSlot --- PASS (internal/httpapi)
  AC-019 ✓ demonstrated — evidence: TestBookSlot_50ConcurrentRequestsExactlyOneWins --- PASS (2.07s) (cmd/booking, real binary subprocess)
  AC-037 ✓ demonstrated — evidence: TestBookSlot_SameStartOnAnotherResourceIsNotTheSameSlot --- PASS (internal/httpapi)
  EC-001 ✓ demonstrated — evidence: TestProperty_Booking_NoOverlapUnderConcurrency --- PASS (0.34s), 25 seeded iterations
  EC(ADR-0012/R1) ✓ demonstrated — evidence: TestProperty_Booking_ConcurrentWritersNeverSeeLockErrors --- PASS (0.17s), 10 seeded iterations
  EC(ADR-0013/R1) ✓ demonstrated — evidence: TestProperty_Booking_SlotStartStoredAsCanonicalEpochSeconds --- PASS (0.04s), 200 seeded iterations + static time.Now scan
  EC(ADR-0015/R1) ✓ demonstrated — evidence: TestProperty_Booking_SlotIdentityIsResourceAndCanonicalStart --- PASS (0.07s), 300 seeded iterations
- [Docs] forge:readme step — cmd/booking, internal/booking, internal/catalog, internal/httpapi, internal/store READMEs written by the implementation commit 6aed539 and judged necessary/current by review cycle 1; no further change
- [Commit] nothing to commit (no fix delta) — HEAD stays 6aed539039944f54aa7197b7e50e536ac89feed1
- [Merge gate] dependency 2026-10-22T07:01:51Z PASS (deps:6fc9f447bb2b5814ca1415c52dfd55a3dc68593d76fb106a8d94ae63b73fccf3, mode=interactive, scan=scanned) — 0 blocking, 0 allowlisted, 0 non-blocking finding(s)
- [Merge gate] dependency cross-check: make audit in the worktree → .audit/govulncheck.status ok, .audit/licenses.status ok; 0 govulncheck findings; licences Apache-2.0 ×1, BSD-3-Clause ×7, MIT ×5 (all allowlisted); go.mod/go.sum unchanged by the card
- [Done] 2026-10-22 — the implementation agent's `## Failure matrix` section existed only in the worktree card copy (sync-card-body carries Worktree notes lines only); copied verbatim into the main card before the worktree was removed
- [Done] worktree bookkeeping (card copy, meta/review/20261022T065842Z-CARD-003-cycle1.yml, meta/impact/*.json) synced to the main repo, then deleted from the worktree so `git worktree remove` runs without --force
- [Trace] 7c skipped — writing meta/architecture/trace.yml is outside this run's mandate (operator rule: kanban never edits meta/architecture/); FR-005/FR-001 trace entries left untouched, evidence tests listed in the [AC/EC check] note
- [Deferral scan] 3 hits: TODO(CARD-005) tracked (incidental); unimplemented stubs for CARD-004/006/008 tracked by comments; GetBooking (FR-007/AC-029) has no owning card → captured to meta/kanban/backlog.md
- [Merge gate] PASS (tree-sha c850acffa95f5be74490146e2120f46c101806ef, scope=full, 1s, cmd="go build ./... && go test ./...")
