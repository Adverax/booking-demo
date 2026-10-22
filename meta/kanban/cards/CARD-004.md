# CARD-004: Cancel a booking and keep cancellation final

**Status:** done
**Priority:** P1
**Category:** feature
**Estimate:** 1c
**Complexity:** architectural
**Closure:** closed
**Revision pending:** false
**Skill:** golang-pro
**TDD:** —
**Branch:** card/004-cancel-booking-final
**Worktree:** —
**Source:** meta/architecture/handoff.md#increment-2/cancellation
**Idea:** — (no match in backlog-scored.yml)
**Wave:** 3
**Depends on:** CARD-003
**Touches:** internal/booking/**.go, internal/httpapi/**.go, internal/store/*.go, internal/store/migrations/**
**UI surface:** none
**Readiness:** 7/7
**Review score:** 9.0 (cycle 1/3)
**Started:** 2026-10-22T07:17:36Z
**Closed:** 2026-10-22T08:03:05Z
**Actual:** 0.1d
**Merge commit:** 851e24a3f4353b0ec31a1d95718695dae3476e14
**Blocked by:** —

## What to implement

Increment 2, the cancellation half of "Idempotency and cancellation".

- Cancel a booking by its id, with no customer identity check (FR-007, CON-006). Cancelling sets the status to cancelled with a single autocommit UPDATE (ADR-0012), which frees the slot for a new booking because the active-slot unique index of ADR-0008 covers active bookings only.
- A repeated cancel of an already cancelled booking succeeds and returns the cancelled booking, with the same response shape as the first cancel (ADR-0003, AC-048, EC-005). An unknown id is a not-found error with a stable code (AC-027).
- Read a booking by id (AC-029), so a cancelled booking can be observed as cancelled.
- Cancelled is final (FR-008, INV-004): keep the domain guard, and add a versioned migration with a schema trigger on the booking status that aborts any update from cancelled to active (ADR-0010, AC-049). CARD-002 (done) shipped `internal/store/migrations/0001_create_resource_and_booking.sql`, embedded and applied by `store.Open`; the trigger is the next versioned file after it. The violation maps to a conflict error with a stable code (AC-030). A new booking of the same slot creates a new booking with a new id (AC-028).

Moved to CARD-008: AC-025 (TestCancelBooking_FreesSlotImmediately) asserts the freed slot through the next availability query, and availability (COMP-003) is built in Increment 3; the Increment 3 checkpoint ("a cancelled booking's slot reappears in the next query") is where it is verified.

Open decisions touching this card (current as of this decompose run; supersedes the list in Worktree notes, which the procedure carries over unchanged). Do not answer them here:
- DEC-017: a cancel racing a booking of the same slot is not covered by this card's ACs.
- DEC-014: whether a no-op repeated cancel emits its domain event again (EVT-003).
- DEC-011: a leaked booking id lets anyone cancel; no mitigation is decided and CON-006 stands.
Resolved since the last cut: DEC-012 (ADR-0020, a past-slot rejection checks the idempotency key first) concerns booking replay only and does not touch cancellation.
Revised since the last cut: ADR-0004 (24:00 interval end) and ADR-0006 (`_txlock` exists) do not change cancellation. Property-test names now carry the `Test` prefix Go requires (`TestProperty_...`).

## Non goals

- NG-1: Any change to who may cancel (CON-006) or to the cancel rule for active bookings (FR-007 out of scope)
- NG-2: Any public route that reactivates a booking (none exists, ASM-0001; FR-008 out of scope)

## Acceptance criteria

- AC-024: Given an active booking for Monday 09:00 made by customer "Ann", when the booking is cancelled by its id, then the booking status equals cancelled — test: TestCancelBooking_MarksCancelled
- AC-026: Given an active booking made by customer "Ann", when the cancel request carries no customer identity, only the booking id, then the booking is cancelled — test: TestCancelBooking_AnyoneWithIdCanCancel
- AC-027: Given no booking with id "missing-id" exists, when that booking is cancelled, then the response is a not-found JSON error with a stable code field — test: TestCancelBooking_UnknownIdNotFound
- AC-048: Given a booking B1 that was already cancelled, when B1 is cancelled again by its id, then the response is a success carrying B1 with its original id and status cancelled — test: TestCancelBooking_RepeatedCancelReturnsCancelledBooking
- AC-028: Given a cancelled booking B1 for Monday 09:00, when a customer books Monday 09:00 with a new idempotency key, then a new booking B2 with a different id is created — test: TestBooking_NewBookingAfterCancelCreatesNewId
- AC-029: Given a cancelled booking B1 for Monday 09:00 and B2 now active for that slot, when B1 is read, then B1 status equals cancelled — test: TestBooking_CancelledStaysCancelled
- AC-030: Given a cancelled booking B1, when any request attempts to set B1 active again, then the response is a conflict JSON error with a stable code field and B1 status stays cancelled — test: TestBooking_CannotReactivate
- AC-049: Given a cancelled booking B1 for Monday 09:00 whose slot has no active booking, when a statement updates the status of B1 to active directly in the database, then the update is aborted by the schema and B1 status stays cancelled — test: TestSchema_RejectsCancelledToActiveTransition

## Engineering constraints

- EC-005: Cancelling a booking any number of times leaves it cancelled and returns it as a success every time
  test: TestProperty_Booking_RepeatedCancelIsIdempotent
- EC(ADR-0010/R1): for any cancelled booking and any sequence of status updates attempted on it, through the domain code or directly in the database, its status stays cancelled
  test: TestProperty_Booking_CancelledNeverBecomesActive

## Guardrails

- G-1: Do not edit `internal/catalog/**.go` — owned by CARD-006 this wave

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

- **FR:** FR-007, FR-008
- **NFR:** —
- **ADR:** ADR-0003, ADR-0008, ADR-0010, ADR-0012
- **Components:** COMP-001, COMP-004, COMP-005, COMP-006
- **Trace:** meta/architecture/trace.yml

## Failure matrix

Config values: `busyTimeout` = 5 s (internal/store/store.go), `writeTimeout` = 30 s, `shutdownTimeout` = 10 s (cmd/booking/main.go). Every operation below issues exactly ONE autocommit SQL statement (ADR-0012), so "a statement" is the whole operation.

| Operation / boundary | Failure mode | Declared behavior | Bound |
|---|---|---|---|
| CancelBooking (`UPDATE booking SET status = ? WHERE id = ? RETURNING ...`) | write lock held by another writer (timeout) | the statement waits for SQLite's write lock, then fails SQLITE_BUSY → 500 `internal_error`; nothing is changed (the statement did not run) | wait ≤ `busyTimeout` = 5 s per request; handler total ≤ 5 s + statement time, < `writeTimeout` 30 s |
| CancelBooking | retry / duplicate delivery (same id, any number of times, sequential or concurrent) | every attempt is 200 with the same booking (same id, status `cancelled`); the row is identical after 1 or N cancels (EC-005, ADR-0003) | N attempts → 0 state changes after the first; no duplicate window exists: the operation is a status assignment, not an insert |
| CancelBooking | crash mid-operation (process killed, power loss) | WAL: the UPDATE is committed entirely or not at all; a client that got no answer retries and gets 200 (row above) | 0 partially applied cancels |
| CancelBooking | graceful shutdown (SIGTERM) | in-flight request finishes under `http.Server.Shutdown`; a request still waiting on the lock when the shutdown deadline passes is abandoned with its connection, and its statement either commits or not (atomic) | in-flight drain ≤ `shutdownTimeout` = 10 s; abandoned statement ends ≤ `busyTimeout` = 5 s later, when the pool closes |
| CancelBooking | poison input: unknown id, non-ASCII id, very long id | no row matches → 404 `not_found`, nothing changed (AC-027); id compared byte for byte; an empty segment (`/bookings//cancel`) is non-canonical → 404 by the plumbing | id size ≤ net/http `MaxHeaderBytes` (1 MiB default), else 431 by net/http |
| CancelBooking | request context cancelled (client gone) | the driver interrupts the statement; either it already committed (cancel happened) or it did not; the client's retry is idempotent | interrupt latency: one SQLite VM step |
| CancelBooking racing BookSlot of the same slot (DEC-017, open) | concurrency | SQLite serialises the two writes; whichever commits first wins; INV-001 holds by the partial unique index in both orders (book first → 409 `slot_taken`; cancel first → 201). Which order is not promised (DEC-017 open) | each side waits ≤ `busyTimeout` = 5 s |
| Any write setting a cancelled booking to another status (domain `Booking.WithStatus`, store `UpdateBookingStatus`, raw SQL `UPDATE`/`UPSERT ... DO UPDATE`) | invariant violation attempt (INV-004) | domain: `ErrBookingCancelled`, booking unchanged; schema: trigger `booking_cancelled_is_final` aborts the statement (SQLITE_CONSTRAINT_TRIGGER), store maps it to `booking.ErrBookingCancelled`, HTTP plumbing answers 409 `booking_cancelled`; row stays `cancelled` | 0 rows ever leave `cancelled` through UPDATE; NOT covered: `DELETE` + re-`INSERT` / `INSERT OR REPLACE` (a new row, not a status update; no write path of the service issues them) |
| GetBooking (`SELECT ... WHERE id = ?`) | timeout / lock | WAL readers do not wait on the writer; a read fails only on I/O error → 500 | no lock wait (WAL snapshot read) |
| GetBooking | poison input (unknown id) | 404 `not_found` | as above |
| Migration 0002 (trigger) at `store.Open` | crash / failure mid-migration | applied with every pending migration in one `BEGIN IMMEDIATE` transaction with the new `user_version`; failure → Open fails, schema and version untouched; next start re-applies | one transaction; lock wait ≤ `busyTimeout` = 5 s |
| non-cooperative callee | — | no callee of this card runs user code or ignores cancellation: the only callee is the SQLite driver, which honours ctx by interrupting | n/a |

## Worktree notes

Open decisions touching this card (do not answer them here): DEC-017 (a cancel racing a booking of the same slot) is not covered by this card's ACs; DEC-014 (whether a no-op repeated cancel re-emits its event) is open; DEC-011 (leaked booking ids let anyone cancel) is open and CON-006 stands.
- [Env] forge 2026.10.10
- [Drift gate] ⚠ unprocessed external drift: internal/store/*.go, internal/store/migrations/** (meta/drift-pending.yml event 2026-10-22T06:30:37Z, range 5fdd66b..54118a9; drift.gate default warn)
- [Run] 2026-10-22 dispatched by `run` (wave 3) on the still-open run 2026-10-21T17:24:03Z (journal tail stop_declared, not run_completed → reused per cmd-run resume 1a); dispatcher.state top-level `wave:` still reads 1 (no writer advances it), the active entry carries wave 3 and this card's Wave: 3 stands
- [Conflict graph] predicted edge with CARD-006 (overlap 2/3 = 0.67 ≥ 0.3: internal/httpapi/**.go, internal/store/*.go) — CARD-004 (P1) runs first, CARD-006 starts after this card is merged
- STRUCTURE: the booking status has ONE write path in the store, `UpdateBookingStatus(ctx, id, to)` on the `booking.Repository` port: a single autocommit `UPDATE booking SET status = ? WHERE id = ? RETURNING <columns>` (ADR-0012: no read-then-write; the RETURNING row is the answer, so a cancel is one statement). `Service.Cancel` is that call with `StatusCancelled`. The port takes a target status (not a cancel-only method) so the schema guard of ADR-0010 is reachable, mapped and tested through the adapter; no production caller passes `active` (NG-2).
- STRUCTURE: cancelled-is-final lives twice (ADR-0010): `Booking.WithStatus(to)` is the aggregate's transition rule (domain guard, returns `ErrBookingCancelled`), and trigger `booking_cancelled_is_final` (migration `0002_cancelled_booking_is_final.sql`, `BEFORE UPDATE OF status ... WHEN OLD.status = 'cancelled' AND NEW.status <> 'cancelled'`, `RAISE(ABORT, 'booking_cancelled_is_final')`) is the data guard. The store recognises the abort by code SQLITE_CONSTRAINT_TRIGGER AND that message (`isCancelledIsFinal` in store.go, next to `IsUniqueViolation`), so a future trigger is not mistaken for it. Cancel does not call `WithStatus`: its target (cancelled) is allowed from every status, so a read before the UPDATE would only add a read-then-write window.
- STRUCTURE: domain sentinels `booking.ErrNotFound` (store maps `sql.ErrNoRows`) and `booking.ErrBookingCancelled`; handlers map `ErrNotFound` to the generated 404 types. `ErrBookingCancelled` → 409 `booking_cancelled` is mapped once, in `writeResponseError` (server.go), because no operation of the contract declares a 409 for it (no operation requests reactivation, ASM-0001); every other handler error stays 500.
- STRUCTURE: API tests of operations with path parameters validate against the spec through `apiEnv.validateAt(method, template, target, rec)` (cancel_test.go); the shared `conforms` looks up literal paths only and was left untouched. `apiEnv` and the booking property `env` now expose the opened `*store.Store` (field `store`).
- AC-030 is exercised without a reactivation route (NG-2): `TestBooking_CannotReactivate` checks the domain guard on the read-back aggregate, then serves a test-only `reactivatingServer` (the real Server with its cancel operation replaced by `UpdateBookingStatus(active)` on the real store, skipping the domain guard) through the real `NewHandler`: the schema aborts, the store maps it, the plumbing answers 409 `booking_cancelled` (body validated against the `Error` schema), and GET B1 still reads cancelled.
- Mutation check: with migration 0002 removed, TestSchema_RejectsCancelledToActiveTransition, TestStore_UpdateBookingStatusMapsTheSchemaGuard, TestBooking_CannotReactivate and TestProperty_Booking_CancelledNeverBecomesActive fail; restored.
- Invariants of AGG-002 touched and their property tests: INV-004 → TestProperty_Booking_CancelledNeverBecomesActive (random sequences over domain/port/raw SQL/upsert attempts, slot free or re-booked; also asserts INV-001 count at the slot); EC-005 → TestProperty_Booking_RepeatedCancelIsIdempotent (1..10 cancels, sequential or concurrent). INV-001/INV-003 unchanged (existing properties green).
- SCOPE+ internal/booking/README.md, internal/store/README.md, internal/store/migrations/README.md, internal/httpapi/README.md — factual updates for the new operations, migration 0002 and the 409 mapping (not .go files; next to the Touches globs).
- Shared-file edits kept local: unimplemented.go lost ONLY GetBooking and CancelBooking (resource stubs untouched); api_test.go got one struct field, one constructor argument, and the two booking rows removed from TestServer_UnimplementedOperationsAreInternalError; repository.go only appended.
- Summary: cancel by id (200, idempotent, same body on repeat; 404 not_found), read by id (200/404), migration 0002 trigger, store `UpdateBookingStatus`/`GetBooking`, domain `Cancel`/`Get`/`WithStatus`. Gates: `make build test vet` green; `go test -race` green for booking, store, httpapi; `go generate ./...` no drift; go.mod/go.sum and internal/catalog unchanged.
- [Commit] e39372e — feat(booking): CARD-004 cancel a booking and keep cancellation final (staged via forge:commit stage_card_paths.py, exit 0; READMEs per SCOPE+ above).
- [Scope] internal/booking/README.md, internal/booking/booking.go, internal/booking/booking_test.go, internal/booking/cancel_property_test.go, internal/booking/property_test.go, internal/httpapi/README.md, internal/httpapi/api_test.go, internal/httpapi/bookings.go, internal/httpapi/cancel_test.go, internal/httpapi/server.go, internal/httpapi/unimplemented.go, internal/store/README.md, internal/store/migrations/0002_cancelled_booking_is_final.sql, internal/store/migrations/README.md, internal/store/repository.go, internal/store/repository_test.go, internal/store/schema_test.go, internal/store/store.go
- [Build gate] not reached — the scope gate runs at the TOP of the cycle and escalated first (the implementation agent reported `make build test vet` green, go test -race green on booking/store/httpapi, go generate no drift; not re-verified by the orchestrator)
- [Scope poach] diff covers 0.67 of CARD-006's Touches (matched: internal/httpapi/**.go, internal/store/*.go)
- [Scope gate] runaway: poached CARD-006 (internal/httpapi/**.go, internal/store/*.go, ratio 0.67); excess 3/18 = 0.17 (internal/booking/README.md, internal/httpapi/README.md, internal/store/README.md, SCOPE+ noted); comp_spread 0; guardrail_hits none; conflict.hotspots empty
- [Escalated] 2026-10-22T07:26:56Z — scope gate RUNAWAY (cycle 1, before review): diff covers 67% of ready same-wave sibling CARD-006's Touches (internal/httpapi/**.go, internal/store/*.go); CARD-006 is the sibling serialized behind this card by the predicted conflict edge, so any implementation of CARD-004 inside its own Touches trips the poach rule; conflict.hotspots empty · station: decompose · route: decide at the decompose station whether internal/httpapi/**.go and internal/store/*.go are shared ground between CARD-004 and CARD-006 (seed conflict.hotspots — a human config decision) or re-slice Touches ownership, or let CARD-006 finish first so no ready sibling owns those globs; then /kanban unblock CARD-004 (returns to review) and /kanban review CARD-004 (rebase onto main first). Worktree ../booking-demo-CARD-004 and branch card/004-cancel-booking-final kept (head e39372e, unreviewed)
- [Info] 2026-10-22T07:45Z — after CARD-006 merged (4c2cd96), a dry-run `git merge-tree --write-tree main card/004-cancel-booking-final` reports content conflicts in internal/httpapi/unimplemented.go, internal/store/README.md, internal/store/repository.go, internal/store/repository_test.go (api_test.go auto-merges); the rebase on unblock must keep BOTH sides (CARD-006 resource read methods + CARD-004 booking status/read methods; each side removed its own stubs from unimplemented.go) and re-run all gates. No ready sibling owns internal/httpapi/**.go or internal/store/*.go any more (CARD-006 done), so the poach condition behind this escalation is gone; unblocking remains a human decompose-station decision.
- [Unblocked] (2026-10-22) — decompose-station decision by the user: CARD-006 finished first (done, merged 4c2cd96), so no ready sibling owns internal/httpapi/**.go or internal/store/*.go any more; conflict.hotspots NOT seeded, no gate config changed. heartbeat-gate --action unblock: exit 0 (heartbeat absent). Returns to review; branch to be rebased onto main b8c2b7b.
- [Rebase] 2026-10-22 — /merge main (rebase, --autostash for the uncommitted worktree card copy) in ../booking-demo-CARD-004: e39372e → ec8706f onto main b8c2b7b. Conflicts: internal/httpapi/unimplemented.go and internal/store/README.md (merge_policy classify-conflict exit 0: resolvable / additive), internal/store/repository.go and internal/store/repository_test.go (classify-conflict exit 3, conflicting_logic — both hunks are functions appended at the same end-of-file position); resolved keep-both per the user's decision: unimplemented.go keeps BOTH removals (CARD-006's ListResources/GetResource stubs and CARD-004's GetBooking/CancelBooking stubs; only GetAvailability remains); repository.go keeps CARD-006's resourceRows/ListResources/GetResource/queryResources then CARD-004's bookingColumns/UpdateBookingStatus/GetBooking/scanBooking; repository_test.go keeps CARD-006's three resource tests then CARD-004's four booking-status tests; store README keeps main's extended InsertBooking/ListResources bullet plus CARD-004's UpdateBookingStatus bullet, and both entity rows. api_test.go auto-merged (unimplemented table holds only GetAvailability). The autostash re-apply conflicted on meta/kanban/cards/CARD-004.md (main b8c2b7b committed the card); the worktree copy was restored verbatim from the autostash (agent-owned, uncommitted). gofmt clean. Post-rebase gate (go1.27.3 local; go.mod go 1.26.0 + toolchain go1.26.10 untouched): go build ./... && go vet ./... && go test -count=1 -race ./... PASSED (cmd/booking, booking, catalog, httpapi, store ok; 8s); make build test vet green; 118 top-level tests pass, 0 fail; go generate ./... no drift; make audit: .audit/govulncheck.status ok (0 findings), .audit/licenses.status ok (13 modules: MIT ×5, BSD-3-Clause ×7, Apache-2.0 ×1); go.mod/go.sum unchanged vs main. tree-sha: de7f029650d4342bd2b74776703a0f7de59abecf.
- [Review re-entry] 2026-10-22 — /kanban review CARD-004 after unblock and rebase onto main b8c2b7b (HEAD ec8706f). Blocker check: no [BLOCKER line (main + worktree copies); empty-implementation guard: 1 commit main..HEAD; review-fatigue: total_rounds 0 / ceiling 6 — ok; Complexity: architectural → full apparatus (max_cycles 3, skeptic cap 6, holds cap 3)
- [Scope] internal/booking/README.md, internal/booking/booking.go, internal/booking/booking_test.go, internal/booking/cancel_property_test.go, internal/booking/property_test.go, internal/httpapi/README.md, internal/httpapi/api_test.go, internal/httpapi/bookings.go, internal/httpapi/cancel_test.go, internal/httpapi/server.go, internal/httpapi/unimplemented.go, internal/store/README.md, internal/store/migrations/0002_cancelled_booking_is_final.sql, internal/store/migrations/README.md, internal/store/repository.go, internal/store/repository_test.go, internal/store/schema_test.go, internal/store/store.go
- [Scope gate] in_scope (re-entry after rebase, cycle 1): poached none (CARD-006 done, no ready sibling); excess 3/18 = 0.17 (internal/booking/README.md, internal/httpapi/README.md, internal/store/README.md, SCOPE+ noted); comp_spread 0; guardrail_hits none; conflict.hotspots unchanged (empty)
- [Build gate] PASSED (full, post-rebase: go build ./... && go vet ./... && go test -count=1 -race ./... — all 5 packages ok; make build test vet green) — review.test_scope impact covers booking, store, httpapi and their reverse deps cmd/booking (catalog also run), so the full command was used
- [System contract] fresh lens (system_rules.py --card CARD-004) matches the card's 11 rules — no refresh
- [Review 1/3] Score: 9.0 — crit: 0, imp: 0 (re-entry after the rebase; FULL review of main b8c2b7b...ec8706f incl. the keep-both conflict resolution; review agent spawned with fresh context and ran forge:review via the Skill tool; brief deviation: the rendered brief (assemble-brief --render-check exit 0) was handed to the agent by file path rather than inlined; the Failure matrix was taken from the worktree card copy, where the implementation agent wrote it; 2 Minor: F-001 409 booking_cancelled answered on operations whose spec does not declare it, F-002 Booking.WithStatus domain guard has no production caller; out-of-scope: GetBooking exposes name/key to id holders (CON-006/DEC-011), INV-001..004 check refs partly owned by later cards)
- [Review sync] 1 report(s) → meta/review/ (20261022T075741Z-CARD-004-cycle1.yml; report-guard: current)
- [Review 1/3] parse-review on the YAML report: verdict ok (score 9.0, crit 0, imp 0, minor 2); parse_review --report accepted, security: applicable (0 findings); carry-forward: no-previous-report (accepted); Step 8h: 11/11 card rules addressed — 7 holds, 4 unchecked (check_ref_missing: INV-001..INV-004, refs owned by later cards), 0 violated
- [Review 1/3] Score: 9.0 ✓ threshold reached + no critical/important
- [Adversarial] none — 0 gating findings this cycle, nothing to refute
- [Minor triage] verified: sampled 2/2 Minor finding(s), rationale recorded (both should_gate: false)
- [8h spot-check] precondition note: worktree held only bookkeeping (M meta/kanban/cards/CARD-004.md, untracked meta/review/*.yml, meta/impact/) — no code delta; one skeptic agent processed the three holds sequentially, read-only (porcelain unchanged afterwards)
- [8h spot-check] 3/3 sampled holds reproduced (ADR-0010/R1, INV-005, ADR-0012/R1) — skeptic observation: `INSERT OR REPLACE` on a cancelled row is a delete + insert, so the UPDATE trigger does not fire; no write path in the code issues REPLACE, ADR-0010 scopes R1 to updates, and the card's Failure matrix already declares DELETE + re-INSERT / INSERT OR REPLACE as NOT covered — no clause violation
- [AC/EC check] fresh-context agent (sonnet) at ec8706fb8e4fab3672066788bc64dd3b29202f9a: AC-024, AC-026, AC-027, AC-048, AC-028, AC-029, AC-030 ✓ demonstrated (named tests --- PASS, internal/httpapi); AC-049 ✓ demonstrated (TestSchema_RejectsCancelledToActiveTransition --- PASS, internal/store); EC-005 ✓ demonstrated (TestProperty_Booking_RepeatedCancelIsIdempotent --- PASS, 20 seeded trials of 1..10 cancels, sequential or concurrent); EC(ADR-0010/R1) ✓ demonstrated (TestProperty_Booking_CancelledNeverBecomesActive --- PASS, 30 seeded trials of 1..12 domain/store/raw-SQL attempts); G-1 ✓ demonstrated (git diff --name-only main...HEAD + uncommitted: no path under internal/catalog/)
- [AC/EC check] All criteria/constraints ✓ (head-sha: ec8706fb8e4fab3672066788bc64dd3b29202f9a) (evidence): the agent's verdicts above — 8 ACs, 2 ECs and G-1 demonstrated at this HEAD
- [Docs] forge:readme step: changed dirs internal/booking, internal/httpapi, internal/store, internal/store/migrations — READMEs already current after the rebase (store README carries both CARD-006's and CARD-004's entries; httpapi README names resources.go/bookings.go, unimplemented.go and the 409 mapping); no change
- [Commit] no new commit — the card's only commit ec8706f (rebased e39372e) already holds code, tests and READMEs; the review cycle produced no code or docs change
- [Merge gate] dependency 2026-10-22T08:01:46Z PASS (deps:eb8be44bf0c00016bf9baafff769e6e49464fe5fd5d276b5996d539903026242, mode=interactive, scan=scanned) — 0 blocking, 0 allowlisted, 0 non-blocking finding(s)
- [Merge gate] dependency cross-check: make audit run in the worktree after the rebase at HEAD ec8706f (the gate's verdict above reports scan=scanned, sources govulncheck) → .audit/govulncheck.status ok, .audit/licenses.status ok; 0 govulncheck findings; licences MIT ×5, BSD-3-Clause ×7, Apache-2.0 ×1 (13 modules, all allowlisted); go.mod/go.sum unchanged by the card
- [Deferral scan] 1 hit in the added lines, incidental: `stubServer` test fixture name in internal/httpapi/cancel_test.go (not a deferral); nothing captured to the backlog
- [Done] 2a gates: (i) Review score 9.0 + [AC/EC check] ✓ present; (i-sha) checkmark-guard ok (head_sha_matches ec8706f); (ii) 1 canonical report meta/review/20261022T075741Z-CARD-004-cycle1.yml; (ii-fresh) report-guard ok (current_report_present); (iii) vacuum-gate not_applicable (not_on_vacuum_path); 2d: no meta/kanban or meta/review path committed on the card branch; worktree uncommitted: only the card copy, the review report and untracked meta/impact/ (transient impact-set JSON, not committed)
- [Done] 5b: sync-card-body exit 0 (placement=target, changed=false — every worktree note line was already on main); the `## Failure matrix` section the implementation agent wrote only in the worktree copy (not a notes line, so not carried by sync-card-body) was copied verbatim into the main card before the worktree removal; review report 20261022T075741Z-CARD-004-cycle1.yml copied to meta/review/
- [Done] 2026-10-22 — rebase step: card branch already based on main tip b8c2b7b (merge-base == main), forge:merge `git rebase --autostash main` was a no-op (autostash of the card copy applied cleanly); full suite on the branch tree de7f029650d4342bd2b74776703a0f7de59abecf: `go build ./... && go test -count=1 -json ./...` exit 0, 118 top-level tests passed, 0 failed, 2s; merged --no-ff as 851e24a (wave_branch.py merge, placement target); changelog 3abdd9b (commit-bookkeeping); .merge.lock released
- [Done] worktree ../booking-demo-CARD-004 removed without --force after its bookkeeping was verified preserved on main (card notes: sync-notes appended 0; review report byte-identical copy in meta/review/; impact-set JSON copied to meta/impact/); branch deleted with -d
- [Trace] 7c skipped — writing meta/architecture/trace.yml is outside this run's mandate (operator rule: kanban never edits meta/architecture/); FR-007/FR-008 evidence tests listed in the [AC/EC check] notes
- [Conflict history] not written — the conflicts (internal/httpapi/unimplemented.go, internal/store/README.md, internal/store/repository.go, internal/store/repository_test.go; pair CARD-006 → CARD-004, predicted by the conflict edge, resolution keep-both by the user's decision) arose at the unblock rebase, not at done's step 4 (a no-op); recorded in the [Rebase] note above instead, following the CARD-001 precedent (no meta/kanban/conflict-history.yml exists)
- [Merge gate] PASS (tree-sha 33e13fe2e120896a7add0a2f64761e13c3ced8eb, scope=full, 2s, cmd="go build ./... && go test -count=1 -json ./...")
- [Done] post-merge verification on main 3abdd9b: make build test vet green; go test -count=1 -race ./... green (5 packages); 118 top-level tests pass, 0 fail (171 test results incl. subtests); go generate ./... no drift; make audit: .audit/govulncheck.status ok (0 findings), .audit/licenses.status ok (13 modules: MIT ×5, BSD-3-Clause ×7, Apache-2.0 ×1)
