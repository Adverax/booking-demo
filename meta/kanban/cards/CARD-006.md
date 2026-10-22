# CARD-006: Resource catalog: validated creation, list and read

**Status:** done
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
**Review score:** 9.0 (cycle 1/3)
**Started:** 2026-10-22T07:27:26Z
**Closed:** 2026-10-22T07:39:16Z
**Actual:** 0.0d
**Merge commit:** 4c2cd96119103ad3df0882ed0de81f5de1be50f7
**Blocked by:** —

## What to implement

Increment 3, the catalog half of "Catalog and availability" (COMP-002 Resource Catalog).

- Full validation of resource creation on top of the minimal path from CARD-003 (FR-001, INV-005, ADR-0004): a name is required; weekly working hours are required; at most one interval per weekday; interval bounds aligned to the half hour; each interval inside a single UTC day, where the end of an interval may be 24:00 (the end of that UTC day, exclusive), the start may not be 24:00 and no bound may exceed 24:00 (ADR-0004 revised 2026-10-22). Every violation is a 4xx JSON error with a stable code and nothing is persisted (AC-002, AC-003, AC-034..AC-036, AC-060, AC-061); an interval ending at 24:00 is accepted and persisted (AC-059). AC-001 (the happy path) is already carried by CARD-003.
- The schema shipped by CARD-002 (migration 0001, `resource_working_hours`) already stores bounds as minutes since 00:00 UTC and admits `end_minute = 1440` ("until midnight") while its CHECK rejects a start at or after the end and an end beyond 1440. That CHECK is a backstop, not the contract: the validation above answers with the stable code before anything reaches the store, and no migration is needed.
- List all resources and read a single resource by id; an unknown id is a not-found error with a stable code (FR-002).
- COMP-002 is the single owner of the working-hours rules (ASM-0007): availability (CARD-008) and booking validation (CARD-007) read working hours through it.

The Increment 3 rollback states that this increment "only adds read paths and validation over existing storage": no schema change.

Open decisions touching this card (current as of this decompose run; supersedes the list in Worktree notes, which the procedure carries over unchanged). Do not answer them here:
- DEC-015: whether the cross-aggregate read of working hours (availability and booking validation reading through this component) is recorded on the capabilities. It does not change this card's ownership of the rules.
Resolved since the first cut: DEC-016 (ADR-0018) added criteria for booking and availability inputs only; resource creation's validation criteria are unchanged. DEC-012 (ADR-0020, a past-slot rejection checks the idempotency key first) concerns booking only and does not touch this card.
Revised since the last cut: ADR-0004 (2026-10-22) allows an interval end of 24:00 and adds AC-059, AC-060 and AC-061 to this card (FR-001 statement and INV-005 reworded to match).

## Non goals

- NG-1: Holidays, per-date exceptions, several intervals per day, a lunch break (FR-001 out of scope)

## Acceptance criteria

- AC-002: Given a request with name "Dr. Lee" and no working hours, when the operator creates the resource, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestCreateResource_RejectsMissingWorkingHours
- AC-003: Given a request with an empty name and working hours Mon-Fri 09:00-17:00, when the operator creates the resource, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestCreateResource_RejectsEmptyName
- AC-034: Given a request with name "Dr. Lee" and two Monday intervals 09:00-12:00 and 13:00-17:00 UTC, when the operator creates the resource, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestCreateResource_RejectsTwoIntervalsOnOneWeekday
- AC-035: Given a request with name "Dr. Lee" and a Monday interval 09:15-17:00 UTC, the start not aligned to the half hour, when the operator creates the resource, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestCreateResource_RejectsMisalignedIntervalBound
- AC-036: Given a request with name "Dr. Lee" and a Monday interval 22:00-02:00 UTC that crosses midnight, when the operator creates the resource, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestCreateResource_RejectsIntervalCrossingMidnightUtc
- AC-059: Given a request with name "Dr. Lee" and a Monday interval 22:00-24:00 UTC, the end written as the end of the UTC day, when the operator creates the resource, then the resource is persisted and returned with a generated id — test: TestCreateResource_AcceptsIntervalEndingAtTwentyFourHundred
- AC-060: Given a request with name "Dr. Lee" and a Monday interval 24:00-24:30 UTC, the start written as 24:00, when the operator creates the resource, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestCreateResource_RejectsIntervalStartingAtTwentyFourHundred
- AC-061: Given a request with name "Dr. Lee" and a Monday interval 23:00-24:30 UTC, the end past 24:00 and aligned to the half hour, when the operator creates the resource, then the response is a 4xx JSON error with a stable code field and nothing is persisted — test: TestCreateResource_RejectsIntervalBoundAfterTwentyFourHundred
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

- **FR:** FR-001, FR-002
- **NFR:** —
- **ADR:** ADR-0004, ADR-0009
- **Components:** COMP-001, COMP-002, COMP-005
- **Trace:** meta/architecture/trace.yml

## Worktree notes

Open decisions touching this card (do not answer them here): DEC-016 (input-validation ACs) concerns availability and booking inputs, not resource creation, whose validation ACs exist; DEC-015 (recording the cross-aggregate read of working hours) is open.
- [Env] forge 2026.10.10
- [Drift gate] ⚠ unprocessed external drift: internal/store/*.go (meta/drift-pending.yml event 2026-10-22T06:30:37Z, range 5fdd66b..54118a9; drift.gate default warn)
- [Run] 2026-10-22 dispatched by `run` (wave 3, run 2026-10-21T17:24:03Z) through the refill: its predicted-edge partner CARD-004 escalated (not active, not merged), so this card starts from main bdc10d0 WITHOUT CARD-004's changes; dispatcher.state top-level `wave:` still reads 1, this card's Wave: 3 stands
Implementation summary (CARD-006):
- internal/catalog: `NewResource(id, name, hours)` is the aggregate constructor enforcing INV-005 / ADR-0004 (revised 2026-10-22): name non-empty; at least one interval; weekday 0..6 at most once; bounds on the half hour; `0 <= Start < End <= EndOfDay (1440)` — so 24:00 is allowed only as an end, a start of 24:00, any bound past 24:00 and a midnight-crossing interval are rejected. Every rejection wraps `catalog.ErrInvalidResource`. Hours are copied and ordered by weekday (Sunday first), so an accepted resource round-trips through the store unchanged. `Catalog.Create` validates before touching the repository; new `Catalog.List` / `Catalog.Get` (Get wraps `catalog.ErrNotFound`). `Catalog.Get` is the read path for CARD-007/CARD-008 (ASM-0007); DEC-015 left open.
- `catalog.Repository` port gained `ListResources` and `GetResource`; internal/store implements them (appended to repository.go): one SELECT joining resource and resource_working_hours, resources in creation order (rowid), intervals by weekday; an unknown id returns `catalog.ErrNotFound`. No schema change, no migration.
- internal/httpapi: `ListResources` and `GetResource` moved out of unimplemented.go into resources.go (booking stubs untouched); `CreateResource` maps `ErrInvalidResource` → 400 `validation_error`; `GetResource` maps `ErrNotFound` → 404 `not_found`. api_test.go: only the two resource rows were dropped from `TestServer_UnimplementedOperationsAreInternalError`.
- Tests: AC tests (exact names) in internal/httpapi/resources_test.go, each rejection asserting 400 validation_error, an empty `GET /resources` and zero rows in both tables; responses validated against api/openapi.yaml (a local `getResource` helper validates the templated `/resources/{resourceId}` route, since the shared `conforms` only matches literal paths). Domain unit tests in internal/catalog/catalog_test.go; store integration tests in internal/store/repository_test.go; property test `TestProperty_Resource_AcceptsExactlyTheShapesOfInv005AndRoundTrips` (internal/catalog/property_test.go, fixed seed, 3000 shapes, real SQLite store): accept iff an independent restatement of INV-005 holds; accepted resources read back equal via Get, list size equals accepted count, rejected ones store nothing.
- Not changed (outside Touches): internal/store/README.md still lists only InsertBooking/InsertResource in its entity table; a docs pass may add ListResources/GetResource. api/openapi.yaml unchanged; no generate drift; go.mod/go.sum unchanged.
- Gates: `make build test vet` green; `go test -race` green for catalog, store, httpapi; `go generate ./...` no drift.
- [Scope] internal/catalog/catalog.go, internal/catalog/catalog_test.go, internal/catalog/property_test.go, internal/httpapi/api_test.go, internal/httpapi/resources.go, internal/httpapi/resources_test.go, internal/httpapi/unimplemented.go, internal/store/repository.go, internal/store/repository_test.go
- [Build gate] PASSED (full: `make build test vet`, every package of the module, 2s warm cache) — review.test_scope impact narrowed to all 5 packages anyway (httpapi and cmd/booking depend on catalog/store), so the full command was used
- [Scope gate] in_scope (cycle 1): excess 0/9; poached none (CARD-004 blocked, no ready sibling); comp_spread 0; guardrail_hits none
- [System contract] fresh lens (system_rules.py --card CARD-006) matches the card's 11 rules — no refresh
- [Review 1/3] Score: 9.0 — crit: 0, imp: 0
- [Review sync] 1 report(s) → meta/review/
- [Review 1/3] Score: 9.0 ✓ threshold reached + no critical/important (report parse_review accepted, security: applicable (0 findings); Step 8h: 11/11 card rules addressed — 5 holds, 6 unchecked (check_ref_missing ×5: tests owned by CARD-004/005/007; no_eligible_fact ×1: ADR-0015/R1), 0 violated)
- [Adversarial] none — 0 gating findings this cycle, nothing to refute
- [Minor triage] verified: sampled 2/2 Minor finding(s) (F-001 stale catalog/store READMEs, F-002 3000-case property test against SQLite), both should_gate: false, rationale recorded
- [8h spot-check] precondition note: worktree held only bookkeeping (M meta/kanban/cards/CARD-006.md, untracked meta/review/*.yml, meta/impact/) — no code delta; skeptic ran read-only (no mutation), one agent processing the three holds sequentially
- [8h spot-check] 3/3 sampled holds reproduced (INV-005, ADR-0012/R1, ADR-0013/R1) — skeptic notes the INV-005 verdict wording is slightly broad (the AC-059 test asserts persistence, not 400; the absent-field subtest of AC-002 checks the list, not the row count) and that a whitespace-only name is accepted (the spec only sets minLength: 1) — no clause violation
- [AC/EC check] fresh-context agent (sonnet) at 253e691: AC-002, AC-003, AC-034, AC-035, AC-036, AC-059, AC-060, AC-061, AC-004, AC-005, AC-006 ✓ demonstrated (named tests --- PASS, internal/httpapi); G-1, G-2 ✓ demonstrated (diff + porcelain contain no path under internal/booking/** or internal/store/migrations/**); no EC section
- [Docs] forge:readme step run inline by the orchestrator (review F-001): internal/catalog/README.md and internal/store/README.md refreshed for NewResource/List/Get/ErrInvalidResource/ErrNotFound and ListResources/GetResource; internal/httpapi/README.md still current. SCOPE+ internal/catalog/README.md, internal/store/README.md — package READMEs next to the Touches globs (not .go)
- [Commit] 6a95cd4 — docs(catalog): describe resource validation, list and read in package READMEs (forge:commit stage_card_paths.py exit 0; it also staged meta/impact/*.json, which was unstaged before committing — nothing under meta/ committed)
- [Scope gate] re-run at 6a95cd4: in_scope (excess 2/11 = 0.18: the two READMEs; poached none; guardrail_hits none)
- [AC/EC check] All criteria/constraints ✓ (head-sha: 6a95cd4dfad554f135a2358dbfb7cd774b2cd88d) (evidence): the agent's verdicts at 253e691 above; 253e691..6a95cd4 changes only internal/catalog/README.md and internal/store/README.md, and the orchestrator re-ran all 11 named AC tests at 6a95cd4 (go test -count=1, internal/httpapi: 11 × --- PASS, ok 0.462s) and re-checked the guarded globs (no hit)
- [Merge gate] dependency 2026-10-22T07:38:09Z PASS (deps:18a5c1c3dc3eb61b9f849923d5b113f42012585588443199c530241addb049fb, mode=interactive, scan=scanned) — 0 blocking, 0 allowlisted, 0 non-blocking finding(s)
- [Merge gate] dependency cross-check: make audit in the worktree → .audit/govulncheck.status ok, .audit/licenses.status ok; 0 govulncheck findings; licences Apache-2.0 ×1, BSD-3-Clause ×7, MIT ×5 (all allowlisted); go.mod/go.sum unchanged by the card
- [Deferral scan] 0 hits in the added lines (no TODO/FIXME/later/CARD-NNN markers)
- [Done] 2026-10-22 — rebase step: card branch already based on main tip bdc10d0 (merge-base == main), so `git rebase main` was a no-op (git refused only because of the expected uncommitted card copy); the full suite then ran on the branch tree 64dd6ab3402284c49bfec998ab8f607481515a34: `go build ./... && go test ./...` exit 0, 96 top-level tests passed, 3s
- [Done] worktree bookkeeping (card copy, meta/review/20261022T073449Z-CARD-006-cycle1.yml) synced to the main repo; worktree removed without --force, branch deleted with -d
- [Trace] 7c skipped — writing meta/architecture/trace.yml is outside this run's mandate (operator rule: kanban never edits meta/architecture/); FR-001/FR-002 evidence tests listed in the [AC/EC check] notes
- [Merge gate] PASS (tree-sha 15b422e18fcc145e49232d3fe6dbffabbb67469f, scope=full, 3s, cmd="go build ./... && go test ./...")
