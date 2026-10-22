# CARD-008: Availability search over a date range

**Status:** done
**Priority:** P1
**Category:** feature
**Estimate:** 1c
**Complexity:** standard
**Closure:** closed
**Revision pending:** false
**Skill:** golang-pro
**TDD:** —
**Branch:** card/008-availability-search
**Worktree:** —
**Source:** meta/architecture/handoff.md#increment-3/availability
**Idea:** — (no match in backlog-scored.yml)
**Wave:** 5
**Depends on:** CARD-004, CARD-005, CARD-006
**Touches:** internal/availability/**.go, internal/httpapi/**.go, internal/store/*.go
**UI surface:** none
**Readiness:** 7/7
**Review score:** 9.0 (cycle 1/3)
**Started:** 2026-10-22T09:22:22Z
**Closed:** 2026-10-22T09:37:11Z
**Actual:** 0.0d
**Merge commit:** 62cbcf8fbe3ed5d4ac36f00292490a1fb19d79ca
**Blocked by:** —

## What to implement

Increment 3, the availability half of "Catalog and availability" (COMP-003 Availability Search).

- For a resource and a date range of at most 14 days, list the free slots: 30 minutes long, aligned to the half hour, inside the resource's working hours (read through COMP-002, ASM-0007), and not overlapping an active booking (FR-003, FR-004). Cancelled bookings do not occupy their slot.
- A range of exactly 14 days succeeds; 15 days is a 4xx JSON error with a stable code; an unknown resource is not found.
- A range whose start is after its end is a 4xx JSON error with a stable code, rejected before any read of bookings (ADR-0018, DEC-016 resolved; AC-054).
- Slots are returned as RFC 3339 UTC instants (CON-001); stored instants are integer epoch seconds (ADR-0013).

Relocated acceptance criteria. Two ACs the handoff lists under Increment 2 assert their outcome through the availability query, which does not exist before this card; the Increment 3 checkpoint ("a cancelled booking's slot reappears in the next query") covers them, so they are carried here:
- AC-025 of FR-007 (TestCancelBooking_FreesSlotImmediately), relying on cancellation from CARD-004;
- AC-042 of FR-006 (TestBookSlot_RepeatAfterCancelCreatesNoNewBooking), relying on the replay classifier from CARD-005.
Their requirements' engineering constraints (EC-002..EC-005) stay with CARD-004 and CARD-005, which own those behaviours.

Open decisions and assumptions touching this card (current as of this decompose run; supersedes the list in Worktree notes, which the procedure carries over unchanged). Do not answer them here:
- DEC-015: whether the cross-aggregate read of working hours through COMP-002 is recorded on the capabilities. It does not change where this card reads them.
- ASM-0002 (past slots listed as free) is an unvalidated assumption this increment collapses; do not add a past-slot filter beyond what the ACs state.
Resolved since the first cut: DEC-016 (ADR-0018) adds AC-054 to this card. DEC-012 (ADR-0020, a past-slot rejection checks the idempotency key first) concerns booking only; availability keeps the rules its ACs state.
Revised since the last cut: ADR-0004 (2026-10-22) lets an interval end at 24:00, the end of the UTC day, exclusive. Working hours are read through COMP-002 (CARD-006 owns the rule and AC-059..AC-061); this card's ACs are unchanged. Property-test names now carry the `Test` prefix Go requires (`TestProperty_...`).

## Non goals

- NG-1: The exact error-code name for a reversed range (decided in the API specification, FR-004 out of scope)
- NG-2: Any change to the 14-day maximum (unchanged, FR-004 out of scope)
- NG-3: A rule for a one-point range whose start equals its end (not covered by the ADR-0018 delta; it stays as the existing range rules treat it, FR-004)

## Acceptance criteria

- AC-007: Given a resource open Mon 09:00-10:00 UTC with no bookings, when free slots are requested for that Monday, then exactly 2 slots are returned, starting 09:00 and 09:30, each 30 minutes long — test: TestAvailability_ReturnsHalfHourSlotsInsideWorkingHours
- AC-008: Given a resource open Mon 09:00-10:00 UTC with an active booking for 09:00, when free slots are requested for that Monday, then only the 09:30 slot is returned — test: TestAvailability_ExcludesActiveBookedSlot
- AC-009: Given a resource open Mon 09:00-10:00 UTC, when free slots are requested for that Monday, then no returned slot starts before 09:00 or ends after 10:00 — test: TestAvailability_ExcludesSlotsOutsideWorkingHours
- AC-010: Given a resource open Mon-Fri only, when free slots are requested for a Saturday, then the response contains 0 slots — test: TestAvailability_NoSlotsOnNonWorkingDay
- AC-011: Given a persisted resource and a date range of exactly 14 days, when free slots are requested, then the request succeeds with slots for the whole range — test: TestAvailability_AcceptsRangeOf14Days
- AC-012: Given a persisted resource and a date range of 15 days, when free slots are requested, then the response is a 4xx JSON error with a stable code field — test: TestAvailability_RejectsRangeOver14Days
- AC-013: Given no resource with id "missing-id" exists, when free slots are requested for it, then the response is a not-found JSON error with a stable code field — test: TestAvailability_UnknownResourceNotFound
- AC-054: Given a persisted resource and a date range whose start is 2 days after its end, when free slots are requested, then the response is a 4xx JSON error with a stable code field — test: TestAvailability_RejectsRangeStartAfterEnd
- AC-025: Given Monday 09:00 booked and no free slot listed for it, when the booking is cancelled, then the next availability query lists the Monday 09:00 slot as free — test: TestCancelBooking_FreesSlotImmediately
- AC-042: Given a booking B1 for Monday 09:00 created with key K1 and then cancelled, when the identical request is repeated with key K1, then the total number of bookings stays 1 and the Monday 09:00 slot is still listed as free — test: TestBookSlot_RepeatAfterCancelCreatesNoNewBooking

## Engineering constraints

- EC(ADR-0013/R1): for any range and any injected clock value, availability computes slots from integer UTC epoch seconds and reads the current time only from the injected clock
  test: TestProperty_Availability_ReadsTimeOnlyThroughInjectedClock

## Guardrails

- G-1: Do not edit `internal/booking/**.go` — owned by CARD-007 this wave
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

- **FR:** FR-003, FR-004
- **NFR:** —
- **ADR:** ADR-0004, ADR-0008, ADR-0009, ADR-0013, ADR-0018
- **Components:** COMP-001, COMP-002, COMP-003, COMP-005
- **Trace:** meta/architecture/trace.yml

## Worktree notes

Open decisions touching this card (do not answer them here): DEC-016 (range input validation), DEC-015 (cross-aggregate read of working hours). AC-025 and AC-042 are relocated here from Increment 2 (see What to implement).
[Deferred] to next wave (2026-10-22) — wave 5 restricted by user decision to CARD-007; CARD-007 and CARD-008 both touch internal/httpapi/**.go and the scope gate's poached measurement counts a ready same-wave sibling.
[Unblocked] (2026-10-22) — `unblock CARD-008` (cmd-lifecycle, externally-blocked branch: no worktree, not escalated → ready). The automatic deferral lift (cmd-decompose step 12 wave kick-off) is conditional on `waves.duration` in meta/.skills.yml, which is not set, so it cannot fire in this project; `unblock` is the documented manual lift.
- [Env] forge 2026.10.10
- [Drift gate] ⚠ unprocessed external drift: internal/store/*.go, internal/httpapi/**.go (meta/drift-pending.yml, events through 2026-10-22; drift.gate default warn)
- [Run] 2026-10-22 dispatched by `run` on the still-open run 2026-10-21T17:24:03Z as the remaining card of wave 5 (deferral lifted by `unblock`, commit 47f7e15); dispatcher.state top-level `wave:` still reads 1 (no writer advances it; add-active ignores --wave after the bootstrap), this card's Wave: 5 stands
- [Start gates] 0 forge 2026.10.10 >= min_version 2026.10.10; 1 status ready, Revision pending false; 1b no legacy block; 1c drift warn (above); 1d no States: field; 2 Depends on CARD-004, CARD-005, CARD-006 done; 4b make_probe start-gate exit 0 (decision gate, source autodetect: `go build ./... && go test ./...`, require_gate false); 5 fork-point interactive → main 47f7e15; worktree ../booking-demo-CARD-008, branch card/008-availability-search; model: Complexity standard → heavy (balanced profile: opus)
- [Implementation] 2026-10-22 (opus). Skill golang-pro is not available in this environment; implemented without it, following forge:engineering-standards.
- STRUCTURE: new package internal/availability (COMP-003) with its own ports `Resources` (implemented by *catalog.Catalog) and `Bookings` (implemented by *store.Store); it does not import internal/booking, matching the C4 relations (availability -> catalog, availability -> store). Its `SlotLength` is pinned equal to `booking.SlotLength` by TestAvailability_SlotLengthEqualsBookingSlotLength (test-only import).
- STRUCTURE: slots are GENERATED, not tested: each working interval is enumerated in 30-minute steps `for m := Start; m < End; m += 30`, relying on the catalog's half-hour alignment (INV-005). Availability therefore contains no third copy of the slot-containment predicate. The CARD-007 follow-up (move `insideWorkingHours` of internal/booking to a catalog.Resource method) remains a follow-up for a later card touching internal/booking (G-1 forbids it here); availability would not need to call it.
- STRUCTURE: all slot arithmetic is on integer UTC epoch seconds (day = floor(unix/86400)*86400, weekday from the epoch day number); only the UTC calendar day of `from`/`to` counts. Availability takes no clock (no rule of FR-003/FR-004 needs now; no past filter, ASM-0002); the EC test proves clock independence and the absence of time.Now/Since/Until in the package (AST check).
- STRUCTURE: order inside FreeSlots: range check (ErrInvalidRange) -> resource read (ErrUnknownResource) -> one autocommit SELECT of active booking starts in [first day 00:00, last day + 24h) (store.ActiveBookingStarts, ADR-0012). No migration changed (G-2); the partial unique index booking_active_slot_uq serves the query.
- STRUCTURE: internal/httpapi/unimplemented.go deleted (GetAvailability was its last operation) together with errNotImplemented and TestServer_UnimplementedOperationsAreInternalError (it asserted the stub's 500, now obsolete). httpapi.NewServer gains an availability argument: NewServer(catalog, availability, booking).
- SCOPE+ cmd/booking/main.go — additive DI wiring: `availability.NewService(cat, st)` passed to httpapi.NewServer (one import, one argument).
- AC -> test: AC-007 TestAvailability_ReturnsHalfHourSlotsInsideWorkingHours; AC-008 TestAvailability_ExcludesActiveBookedSlot; AC-009 TestAvailability_ExcludesSlotsOutsideWorkingHours; AC-010 TestAvailability_NoSlotsOnNonWorkingDay; AC-011 TestAvailability_AcceptsRangeOf14Days; AC-012 TestAvailability_RejectsRangeOver14Days; AC-013 TestAvailability_UnknownResourceNotFound; AC-054 TestAvailability_RejectsRangeStartAfterEnd (counting Bookings port: 0 reads on the reversed range, 1 on a valid one); AC-025 TestCancelBooking_FreesSlotImmediately; AC-042 TestBookSlot_RepeatAfterCancelCreatesNoNewBooking (all in internal/httpapi/availability_test.go, API-level, contract-validated). EC(ADR-0013/R1) TestProperty_Availability_ReadsTimeOnlyThroughInjectedClock (internal/availability/property_test.go).
- Extra tests: TestProperty_Availability_ListsExactlyFreeSlotsInsideWorkingHours (oracle over random hours/ranges/bookings, 300 cases), TestProperty_Availability_RangeRule (500 cases), unit tests in internal/availability/availability_test.go, store integration tests TestStore_ActiveBookingStarts* (internal/store/availability_test.go), TestAvailability_MalformedDateIsValidationError. Mutation spot-check: off-by-one in the 14-day rule, `<=` in the enumeration and ignoring bookings each fail tests.
- Gates: `make build test vet` -> build ok, all 6 packages ok, vet clean; `go test -race -count=1 ./...` -> all 6 packages ok (TestBookSlot_50ConcurrentRequestsExactlyOneWins passes); gofmt -l clean. Open decisions (DEC-014/015/017/029..033) untouched.
[Commit] untracked outside Touches: internal/availability/README.md
- [Commit] internal/availability/README.md (new per-directory README, requested by the orchestrator) was staged explicitly with the card commit debafba; it is outside the Touches globs (*.go only).
- [Review entry] 2026-10-22 — blocker check: no [BLOCKER line (worktree copy); empty-implementation guard: 1 commit main..HEAD (debafba); review-fatigue: 0 prior rounds; Complexity: standard → full apparatus (max_cycles 3, skeptic cap 6, holds cap 3); review.require_gate false
- [Scope] cmd/booking/main.go, internal/availability/README.md, internal/availability/availability.go, internal/availability/availability_test.go, internal/availability/property_test.go, internal/httpapi/README.md, internal/httpapi/api_test.go, internal/httpapi/availability.go, internal/httpapi/availability_test.go, internal/httpapi/cancel_test.go, internal/httpapi/handlers.go, internal/httpapi/unimplemented.go (deleted), internal/store/README.md, internal/store/availability_test.go, internal/store/repository.go
- [Scope gate] ⚠ grown: +COMP-007 · 4 files outside Touches (cycle 1, review_cycle_ops.py scope-gate exit 1, verdict grown): excess 4/15 = 0.27 (cmd/booking/main.go DI wiring, internal/availability/README.md, internal/httpapi/README.md, internal/store/README.md — the wiring is SCOPE+ noted by the agent, the READMEs are docs of touched packages); comp_spread +COMP-007 (cmd/booking); poached none (no ready sibling in wave 5); guardrail_hits none; conflict.hotspots empty — GROWN handling: SCOPE NOTE handed to the reviewer, no escalation
- [Commit note] internal/availability/README.md was staged with plain `git add` by the implementation agent after forge:commit's staging script refused it (exit 4: outside the Touches globs, which name .go files only); no meta/ path is in the commit (INV-023 holds) — recorded as a procedure deviation, not a gate bypass
- [Build gate] PASSED (impact: 6 pkgs — every package of the module, 1s) — impact set meta/impact/impact-set-47f7e1599b58-debafba411a5-worktree.json (underivable []): cmd/booking, internal/availability, internal/httpapi, internal/store + reverse deps internal/booking, internal/catalog; cycle_test_cmd `go build ./... && go test ./cmd/booking ./internal/availability ./internal/httpapi ./internal/store ./internal/booking ./internal/catalog`
- [System contract] fresh lens (system_rules.py --card CARD-008) matches the card's 11 rules (ADR-0010/R1, ADR-0012/R1, ADR-0013/R1, ADR-0015/R1, CON-007, CON-008, INV-001..INV-005) — no refresh
- [Review 1/3] Score: 9.0 — crit: 0, imp: 0 (FULL review of main 47f7e15...debafba; review agent (opus, fresh context) ran forge:review via the Skill tool; rendered brief (assemble-brief --render-check exit 0, 0 unresolved) handed to the agent by file path; 2 Minor: F-001 availability.go:117 the [Start,End) half-hour enumeration is a third encoding of catalog.Interval semantics (correct, pinned by TestProperty_Availability_ListsExactlyFreeSlotsInsideWorkingHours; centralising needs catalog/booking edits, G-1), F-002 property_test.go:172 the no-system-clock AST check matches only the identifier `time` (an aliased/dot import would pass); 1 out-of-scope: no runtime OpenAPI validation in the dev server (pre-existing); scope note judged: main.go wiring required by the NewServer signature, READMEs keep per-directory docs current — no unnecessary scope)
- [Review sync] 1 report(s) → meta/review/ (20261022T093238Z-CARD-008-cycle1.yml; report-guard: current_report_present)
- [Review 1/3] parse-review verdict ok (score 9.0, crit 0, imp 0, minor 2, out_of_scope 1); parse_review --report accepted, security: applicable (0 findings); carry-forward: no-previous-report (accepted); Step 8h: 11/11 card rules addressed — 11 holds, 0 unchecked, 0 violated
- [Review 1/3] Score: 9.0 ✓ threshold reached + no critical/important
- [Adversarial] none — 0 gating findings this cycle, nothing to refute
- [Minor triage] verified: sampled 2/2 Minor finding(s), rationale recorded (both should_gate: false)
- [8h spot-check] precondition: worktree held only bookkeeping (M meta/kanban/cards/CARD-008.md, untracked meta/review/*.yml, meta/impact/); one skeptic agent (opus) processed the three holds sequentially; porcelain unchanged afterwards
- [8h spot-check] 3/3 sampled holds reproduced (ADR-0010/R1, ADR-0013/R1, INV-001) — TestBooking_CannotReactivate, TestProperty_Availability_ReadsTimeOnlyThroughInjectedClock, the 5 INV-001 tests (+ TestProperty_Booking_NoOverlapUnderConcurrency -race) re-run green; mutations killed: trigger WHEN 0 (500 vs 409), `_ = time.Now()` in availability.go (AST check fails), non-unique booking_active_slot_uq (property + 50-concurrent fail); original bytes restored (shasum identical)
- [AC/EC check] fresh-context agent (sonnet) at debafba411a584bb61770480ada9f083cdceea60: AC-007, AC-008, AC-009, AC-010, AC-011, AC-012, AC-013, AC-054, AC-025, AC-042 ✓ demonstrated (named tests in internal/httpapi/availability_test.go --- PASS, go test -count=1 -v); EC(ADR-0013/R1) ✓ demonstrated (TestProperty_Availability_ReadsTimeOnlyThroughInjectedClock --- PASS, 300 seeded iterations over random resources, 1..14-day ranges and zones, past ranges included, plus the AST no-time.Now/Since/Until scan; caveat recorded: the service takes no clock, so the property holds by the absence of any time source rather than by varying a clock value); G-1 ✓ demonstrated, G-2 ✓ demonstrated (structural: git diff --name-only main...HEAD + git status filtered for internal/booking/** and internal/store/migrations/** — no hit)
- [AC/EC check] All criteria/constraints ✓ (head-sha: debafba411a584bb61770480ada9f083cdceea60) (evidence): the agent's verdicts above — 10 ACs, 1 EC and 2 Gs demonstrated at this HEAD
- [Docs] forge:readme step: changed dirs internal/availability, internal/httpapi, internal/store, cmd/booking — internal/availability/README.md (new), internal/httpapi/README.md and internal/store/README.md already updated in debafba; cmd/booking 2-line wiring needs no README change; no change
- [Commit] no new commit — the card's commit debafba already holds code, tests and READMEs; the review/AC phase produced no change
- [Merge gate] dependency 2026-10-22T09:36:25Z PASS (deps:6c7c2329ab05ae31e1b37ecc0f8dac9bb86982d912d4b80ebd9d45d19344b5b9, mode=interactive, scan=scanned) — 0 blocking, 0 allowlisted, 0 non-blocking finding(s)
- [Merge gate] dependency cross-check: make audit run in the worktree at HEAD debafba (AUDIT_DIR in the session scratchpad, exit 0) → govulncheck.status ok, licenses.status ok; 0 govulncheck finding objects; licences MIT ×5, BSD-3-Clause ×7, Apache-2.0 ×1 (13 modules, all allowlisted); licenses.err only go-licenses warnings (non-Go asm in golang.org/x/sys); go.mod/go.sum unchanged by the card
- [Deferral scan] 0 hits in the added lines (later card/TODO/FIXME/HACK/XXX/deferred/for now/aspirational/not implemented/placeholder/stub); nothing captured to the backlog
- [Done] 2a gates: (i) Review score 9.0 + [AC/EC check] ✓ present; (i-sha) checkmark-guard ok (head_sha_matches debafba); (ii) 1 canonical report meta/review/*CARD-008-cycle1.yml, report-guard ok (current_report_present); (iii) vacuum-gate not_applicable (not_on_vacuum_path); 2b deferral scan 0 hits; 2c dependency gate PASS (scan=scanned); 2d no meta/kanban or meta/review path committed on the card branch; 3a placement target (interactive)
- [Done] 2026-10-22 — merge lock acquired (receipt CARD-008); review_copies exit 0 (nothing removed, 1 kept); rebase step: card branch already based on main tip 47f7e15, `git rebase --autostash main` was a no-op (forge:merge's own uncommitted-changes stop does not fit done step 2's expected meta/ dirt, same handling as CARD-007; no fetch: nothing is ever pushed/pulled); post-rebase gate executed as `go build ./... && go test -count=1 ./...` exit 0 on branch tree 6a4814014126b8d22a484b1eb4e6ad0058234833 (2s; 185 top-level tests passed, 0 failed by a follow-up `go test -count=1 -json ./...` count); merged --no-ff as 62cbcf8 (wave_branch.py merge, placement target)
- [Done] changelog a7ab1e6 (commit-bookkeeping); .merge.lock released
- [Done] 5b: sync-card-body exit 0 (placement=target, changed=false — every worktree note line was already on main); review report and impact-set JSON copied (byte-identical); then the worktree's card copy was restored to HEAD and its verified untracked copies deleted, worktree ../booking-demo-CARD-008 removed without --force, branch deleted with -d
- [Trace] 7c skipped — writing meta/architecture/trace.yml is outside this run's mandate (operator rule: kanban never edits meta/architecture/); FR-003/FR-004 evidence tests are listed in the [AC/EC check] notes
- [Rebase dependents] step 8: CARD-009 depends on CARD-008 but is blocked (readiness — blocking_decisions_resolved) with no worktree — nothing to rebase
- [Architect note] out-of-scope, routed to the architect station (not acted on): INV-004's check ref TestBookSlot_RepeatAfterCancelCreatesNoNewBooking now exists (added by this card, internal/httpapi/availability_test.go) — the gap both CARD-007 review cycles reported is closed in code; trace write-back still pending at the architect station
- [Follow-up] catalog.Interval's [Start,End)/24:00 reading is now encoded in three places (catalog validation, booking.insideWorkingHours, availability's half-hour enumeration — review cycle 1 Minor F-001); one catalog-owned operation (e.g. Interval.Slots) used by booking and availability needs a card allowed to edit internal/booking and internal/catalog (G-1 forbade it here). Review Minor F-002: the no-system-clock AST check matches only the identifier `time` (aliased/dot import would pass)
- [Merge gate] PASS (tree-sha 6a4814014126b8d22a484b1eb4e6ad0058234833, scope=full, 2s, cmd="go build ./... && go test -count=1 ./...")
