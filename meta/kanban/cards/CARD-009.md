# CARD-009: Release criteria suite and supervisor deployment sample

**Status:** done
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
**Review score:** 9.0 (cycle 1/3)
**Started:** 2026-10-22T09:51:20Z
**Closed:** 2026-10-22T10:05:05Z
**Actual:** 0.0d
**Merge commit:** 6f87458a0daecee60375992636a65ec811540145
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
- [Env] forge 2026.10.10
- [Drift gate] ⚠ unprocessed external drift: api/openapi.yaml, cmd/booking/main.go, internal/httpapi/**.go (meta/drift-pending.yml; drift.gate default warn)
- [Run] 2026-10-22 dispatched by `run` on the still-open run 2026-10-21T17:24:03Z as the sole card of wave 6 (the last card of the board); dispatcher.state top-level `wave:` still reads 1 (no writer advances it; add-active ignores --wave after the bootstrap), this card's Wave: 6 stands
- [Start gates] 0 forge 2026.10.10 >= min_version 2026.10.10; 1 status ready, Revision pending false; 1b no legacy block; 1c drift warn (above); 1d no States: field; 2 Depends on CARD-007, CARD-008 done; 4b make_probe start-gate exit 0 (decision gate, source autodetect: `go build ./... && go test ./...`, require_gate false); 5 fork-point interactive → main b1a6a52; worktree ../booking-demo-CARD-009, branch card/009-release-criteria; model: Complexity standard → heavy (balanced profile: opus)
Implementation (2026-10-22):
- Skill golang-pro is not available in this environment; implemented without it. forge:engineering-standards applied (§6 tests, §9 minimalism, §12 parser discipline for the model reader).
- `cmd/booking/release_test.go` (new): TestRelease_ConcurrencyExactlyOneWinner (AC-031), TestRelease_FullAPIEndToEndOnFreshDatabase (AC-032), TestRelease_EveryRuleHasANamedTest (AC-033), plus TestEveryRuleHasANamedTest_DetectsMissingAndInvalidNames (unit test of the checker). Reuses the real-binary harness of main_test.go (TestMain/startService).
- `cmd/booking/main_test.go`: the body of TestBookSlot_50ConcurrentRequestsExactlyOneWins moved unchanged into helper `assertExactlyOneOf50Wins`, called by that test and by TestRelease_ConcurrencyExactlyOneWinner (no duplicated 50-request code; the existing test's assertions are unchanged).
- AC-032 scenario uses the types generated from api/openapi.yaml (httpapi.CreateResourceRequest, Resource, Availability, BookSlotRequest, Booking): create resource (201) -> availability lists 16 slots from 09:00 -> book (201, active) -> slot gone from availability -> repeat (200, same id) -> cancel (200, cancelled) -> GET booking cancelled -> availability equals the original list; stderr empty.
- AC-033 reader: parses meta/architecture/requirements.yml (read-only) with gopkg.in/yaml.v2 and walks the whole tree collecting every `test:` value. In the current model the only test-name field is `acceptance[].test` under `functional[]` and `non_functional[]`: 66 names read, 0 missing. Refuses (fails) rather than skips: zero names, a `test:` that is not a single non-empty string, and any other key containing "test" (e.g. `tests:`), so a schema change of the model cannot silently shrink the check. Go test functions are found with go/parser + go/ast over every *_test.go below the go.mod root (skipping ".", "_" and testdata dirs): top-level `func TestXxx(t *<testing import name>.T)`, no receiver, no type params, no results, name valid per the go tool rule ("Test" then not a lower-case letter); 190 found. A name that is not a valid test name counts as missing.
- Proof the check fails on a missing name: (1) the unit test feeds a synthetic model and Go tree (absent name, PropertyTest_ prefix, Testlower, method, *testing.B param, testdata-only) and asserts all six are reported missing, and that empty/zero-name/list/empty-value/unknown-field/malformed models error; (2) a temporary local experiment renamed TestRelease_FullAPIEndToEndOnFreshDatabase -> the check failed with "non_functional[1].acceptance[0].test names "TestRelease_FullAPIEndToEndOnFreshDatabase", which is not a Go test function in the repository ... 1 missing"; reverted. requirements.yml was never edited.
- ASM-0005 finding: server_test.go reached unknown/non-canonical path, wrong method, malformed/trailing/oversized body, malformed query parameter, handler error and panic, but not the generated binder's path-parameter failure (InvalidParamFormatError via ErrorHandlerFunc). Added TestNewHandler_MalformedPathParameterIsValidationError (`%25zz` segments on all four path-param routes -> 400 validation_error JSON, handler not reached). Passed without production change.
- Deploy sample path: `deploy/` — deploy/booking.service (systemd unit: one process, EnvironmentFile=/etc/booking/booking.env, StateDirectory=booking -> /var/lib/booking on local disk, DynamicUser, TimeoutStopSec=20 for the 10 s drain, Restart=on-failure), deploy/booking.env.example (BOOKING_DB_PATH=/var/lib/booking/booking.db, BOOKING_ADDR=:8080, nothing else), deploy/README.md. Single instance documented only (DEC-029 not decided); no logging/metrics/uptime settings (DEC-007, DEC-010). systemd-analyze is not available on this machine (macOS), so the unit was not machine-verified.
- SCOPE+ deploy/booking.service, deploy/booking.env.example, deploy/README.md — new files outside Touches, required by the card (ADR-0005 sample). cmd/booking/README.md gained a short Tests section and a link to deploy/.
- Untouched: go.mod, go.sum, Makefile, meta/audit-allowlist.json, internal/store/migrations/**, meta/architecture/**, api/openapi.yaml, production code.
- Verification: `make build test vet` ok (all 6 packages pass); `go test -count=1 -race ./...` ok (all 6 packages); `go test -count=1 -run TestRelease -v ./...` -> the three TestRelease_* PASS (log: 66 planned test names, 190 Go test functions, 0 missing); `gofmt -l .` empty.
[Commit] untracked outside Touches: deploy/README.md, deploy/booking.env.example, deploy/booking.service
- Commit deviation: forge:commit's stage_card_paths.py exited 4 (deploy/* untracked outside Touches); the three deploy/ files were then staged explicitly with `git add` as the orchestrator instructed. meta/kanban/** was not staged. Commit 041faee on card/009-release-criteria.
- - [Review entry] 2026-10-22 — blocker check: no [BLOCKER line (worktree copy); empty-implementation guard: 1 commit main..HEAD (041faee); review-fatigue: 0 prior rounds; Complexity: standard → full apparatus (max_cycles 3, skeptic cap 6, holds cap 3); review.require_gate false
- [Scope] cmd/booking/README.md, cmd/booking/main_test.go, cmd/booking/release_test.go, deploy/README.md, deploy/booking.env.example, deploy/booking.service, internal/httpapi/server_test.go
- [Scope gate] ⚠ grown: 4 files outside Touches (cycle 1, review_cycle_ops.py scope-gate exit 1, verdict grown): excess 4/7 = 0.57 (deploy/booking.service, deploy/booking.env.example, deploy/README.md — the supervisor sample the card's What to implement requires, path chosen by the card; cmd/booking/README.md — docs of a touched package); comp_spread none; poached none (no ready sibling in wave 6); guardrail_hits none; conflict.hotspots empty — GROWN handling: SCOPE NOTE handed to the reviewer, no escalation, thresholds untouched
- [Build gate] PASSED (full: impact set underivable — deploy/ has no_recipe marker, meta/impact/impact-set-b1a6a5297dfc-041faeeb747e-worktree.json; DEGRADE arm) `go build ./... && go test ./...` exit 0, 6 packages ok
- [System contract] fresh lens (system_rules.py --card CARD-009) matches the card's 11 rules (ADR-0010/R1, ADR-0012/R1, ADR-0013/R1, ADR-0015/R1, CON-007, CON-008, INV-001..INV-005) — no refresh
- [Review 1/3] Score: 9.0 — crit: 0, imp: 0 (FULL review of main b1a6a52...041faee; review agent (opus, fresh context) ran forge:review via the Skill tool; rendered brief (assemble-brief --render-check exit 0, 0 unresolved) handed to the agent by file path; 3 Minor: F-001 release_test.go:264 test discovery ignores //go:build constraints (no such file today), F-002 release_test.go:209 the coverage denominator is planned names, not rules (66 AC/EC ids = 66 names today), F-003 deploy/booking.env.example:7 the sample binds all interfaces while DEC-011 is open (suggests a comment, no decision); 3 out-of-scope: Touches under-predicted deploy/** (decompose note), deploy/ not mapped to a component in trace.yml (architect note), real-binary tests in the unit tier (pre-existing pattern); scope note judged: the deploy/ files are required by the card, the READMEs keep docs current — excess necessary, no finding)
- [Review sync] 1 report(s) → meta/review/ (20261022T100042Z-CARD-009-cycle1.yml; report-guard: current_report_present)
- [Review 1/3] parse-review verdict ok (score 9.0, crit 0, imp 0, minor 3); parse_review --report accepted, security: applicable (1 finding); carry-forward: no-previous-report; Step 8h: 11/11 card rules addressed — 4 holds, 7 unchecked (no_eligible_fact: no production code in the diff), 0 violated
- [Review 1/3] Score: 9.0 ✓ threshold reached + no critical/important
- [Adversarial] none — 0 gating findings this cycle, nothing to refute
- [Minor triage] verified: sampled 2/3 Minor finding(s), rationale recorded (both should_gate: false)
- [8h spot-check] precondition: worktree held only bookkeeping (M meta/kanban/cards/CARD-009.md, untracked meta/review/*.yml, meta/impact/); one skeptic agent (opus) processed the three holds sequentially; porcelain unchanged afterwards
- [8h spot-check] 3/3 sampled holds reproduced (ADR-0010/R1, INV-001, INV-003) — diff name-only re-listed (no production/schema file); TestBooking_CannotReactivate, the 5 INV-001 tests and the 17 INV-003 tests re-run green with -count=1; release_test.go read-back/replay assertions confirmed at the cited lines; no mutation performed (none needed — no production code in the diff)
- [AC/EC check] fresh-context agent (sonnet) at 041faeeb747eb8b9055d68fb97537108c27d27b1: AC-031 ✓ demonstrated (TestRelease_ConcurrencyExactlyOneWinner --- PASS, go test -count=1 -v); AC-032 ✓ demonstrated (TestRelease_FullAPIEndToEndOnFreshDatabase --- PASS); AC-033 ✓ demonstrated (TestRelease_EveryRuleHasANamedTest --- PASS, "66 planned test names, 190 Go test functions, 0 missing"); no Engineering constraints section; G-1 ✓ demonstrated (structural: git diff --name-only main...HEAD + uncommitted — no path under internal/store/migrations/**); G-2 ✓ demonstrated (go test -count=1 ./... all 6 packages ok; git diff main...HEAD -- '*_test.go' removes no line — no existing test weakened, retargeted or deleted; no product code in the diff)
- [AC/EC check] All criteria/constraints ✓ (head-sha: 041faeeb747eb8b9055d68fb97537108c27d27b1) (evidence): the agent's verdicts above — 3 ACs and 2 Gs demonstrated at this HEAD
- [Docs] forge:readme step: changed dirs cmd/booking, internal/httpapi, deploy — cmd/booking/README.md (Tests section + deploy link) and deploy/README.md (new) already in 041faee; internal/httpapi change is one added test, no README change; no change
- [Commit] no new commit — the card's commit 041faee already holds tests, deploy sample and READMEs; the review/AC phase produced no change; review Minors F-001..F-003 left unfixed (non-gating, recorded as follow-ups)
- [Merge gate] dependency 2026-10-22T10:04:15Z PASS (deps:2cda4d7d74346f0f071496c3dd6c2e3275bb8a409a327e9c6ac4f4374504e558, mode=interactive, scan=scanned) — 0 blocking, 0 allowlisted, 0 non-blocking finding(s)
- [Merge gate] dependency cross-check: make audit run in the worktree at HEAD 041faee (AUDIT_DIR in the session scratchpad, exit 0) → govulncheck.status ok, licenses.status ok; 0 govulncheck finding objects; licences MIT ×5, BSD-3-Clause ×7, Apache-2.0 ×1 (13 modules, all allowlisted); go.mod/go.sum unchanged by the card (git diff --quiet main...HEAD)
- [Deferral scan] 2 raw hits in the added lines, 0 real deferrals: `TestXxx` in a doc comment (matches XXX case-insensitively) and `stub`/`stubServer`, the existing test double of internal/httpapi; nothing captured to the backlog
- [Done] 2a gates: (i) Review score 9.0 + [AC/EC check] ✓ present; (i-sha) checkmark-guard ok (head_sha_matches 041faee); (ii) 1 canonical report meta/review/*CARD-009-cycle1.yml, report-guard ok (current_report_present); (iii) vacuum-gate not_applicable (not_on_vacuum_path); 2b deferral scan 0 real hits; 2c dependency gate PASS (scan=scanned); 2d no meta/kanban or meta/review path committed on the card branch; 3a placement target (interactive)
- [Done] 2026-10-22 — merge lock acquired (receipt CARD-009); review_copies exit 0 (nothing removed, 1 kept); forge:merge (unattended): card branch already based on main tip b1a6a52, `git rebase --autostash main` was a no-op (forge:merge's own uncommitted-changes stop does not fit done step 2's expected meta/ dirt, same handling as CARD-007/CARD-008; no fetch/push — the repository has a pre-existing `origin` remote, never contacted); post-rebase gate `go build ./... && go test -count=1 ./...` exit 0 on branch tree aa502067aecb2e348e40ada321210801c2146c03 (3s; 190 top-level tests passed, 0 failed by a follow-up `go test -count=1 -json ./...` count); merged --no-ff as 6f87458 (wave_branch.py merge, placement target)
- [Done] changelog 24184e0 (commit-bookkeeping); .merge.lock released
- [Done] 5b: sync-card-body exit 0 (placement=target, changed=false — every worktree note line was already on main); review report and impact-set JSON copied (byte-identical); then the worktree's card copy was restored to HEAD and its verified untracked copies deleted, worktree ../booking-demo-CARD-009 removed without --force, branch deleted with -d
- [Trace] 7c skipped — writing meta/architecture/trace.yml is outside this run's mandate (operator rule: kanban never edits meta/architecture/); NFR-001/NFR-002/NFR-003 evidence tests are listed in the [AC/EC check] notes
- [Rebase dependents] step 8: no card depends on CARD-009 — nothing to rebase
- [Architect note] out-of-scope, routed to the architect station (not acted on): deploy/ is mapped to no component in trace.yml (review cycle 1 out-of-scope; candidate: COMP-007 code entry); NFR-003's per-rule presence of a `test:` key is not enforced by TestRelease_EveryRuleHasANamedTest (review Minor F-002 — a model-validator job or a later tightening)
- [Decompose note] Touches under-predicted the card: deploy/** was required by What to implement but absent from Touches (scope gate GROWN 4/7, accepted as necessary by the reviewer)
- [Follow-up] review Minors not fixed (non-gating): F-001 test discovery ignores //go:build constraints; F-003 deploy/booking.env.example binds all interfaces while DEC-011 (security) is open — a comment, not a decision, would make that visible
- [Merge gate] PASS (tree-sha aa502067aecb2e348e40ada321210801c2146c03, scope=full, 3s, cmd="go build ./... && go test -count=1 ./...")
- [Merge gate note] the record above named the card branch's tested tree (aa50206…); cmd-done 7d FIELDS requires main^{tree} after the changelog commit — corrected record follows (last record wins)
- [Merge gate] PASS (tree-sha c0cd218a8be6e02b3a5cd2db709e87b9d5518480, scope=full, 3s, cmd="go build ./... && go test -count=1 ./...")
