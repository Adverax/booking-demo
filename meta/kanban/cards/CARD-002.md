# CARD-002: SQLite store and embedded schema migrations

**Status:** done
**Priority:** P1
**Category:** feature
**Estimate:** 1c
**Complexity:** architectural
**Closure:** closed
**Revision pending:** false
**Skill:** golang-pro
**TDD:** —
**Branch:** card/002-sqlite-store-migrations
**Worktree:** —
**Source:** meta/architecture/handoff.md#increment-1/store-and-migrations
**Idea:** — (no match in backlog-scored.yml)
**Wave:** 1
**Depends on:** —
**Touches:** internal/store/*.go, internal/store/migrations/**, go.mod, go.sum
**UI surface:** none
**Readiness:** 7/7
**Review score:** 9.0 (cycle 3/3)
**Started:** 2026-10-21T17:35:05Z
**Closed:** 2026-10-21T23:41:31Z
**Actual:** 0.8d
**Merge commit:** 183ac832fbcae082d5de3eb0799c5f24765cfccd
**Blocked by:** —

## What to implement

The storage half of the Increment 1 walking skeleton (COMP-005 SQLite Store, COMP-006 Schema Migrator).

- Open SQLite through `modernc.org/sqlite` and `database/sql` (ADR-0006, CON-003, no cgo), with WAL journal mode, a busy timeout and foreign keys set in the connection string (ADR-0006, ADR-0012). The database path comes from configuration supplied by the caller (CON-005); reading the environment is the bootstrap's job (CARD-003).
- A small hand-written migration runner: versioned SQL files embedded with `go:embed` under `internal/store/migrations/`, applied at open time inside a transaction, the applied version tracked in `PRAGMA user_version` (ADR-0007). A fresh database needs no extra step (NFR-002).
- The first migration creates the resource table (name, weekly working hours in the ADR-0004 shape) and the booking table (id, resource id, slot start, customer name, idempotency key, status), with every instant stored as integer UTC epoch seconds (ADR-0013, CON-008).
- The two uniqueness guarantees of ADR-0008 live in the schema: a partial unique index on (resource id, slot start) for bookings whose status is active, and a unique index on the idempotency key across all bookings (ADR-0008, ADR-0015).
- Expose the store's open/migrate entry point and a way for callers to tell a unique-violation error apart from other errors; the booking write and read operations are added by the cards that need them.

Owed verification (handoff "Before cards are cut"): the driver's transaction-lock DSN option (`_txlock`) was not verified against the pinned `modernc.org/sqlite` version, and ADR-0006's statement that no such option exists is unreliable. Check it against the version this card pins and record the result in Worktree notes, so ADR-0012's documented fallback is known to be available or not before CARD-003 runs the 50-writer test.

Open decisions touching this card (do not answer them here):
- DEC-029: whether the single-instance and local-disk premise of ADR-0005/ADR-0008 is enforced (an exclusive lock on the database, a lock file) or only documented. It could race the migration runner of ADR-0007. Do not add an instance lock.
- DEC-033: whether the component diagram draws the bootstrap's relation to the store (the owner of the WAL and busy-timeout settings). Diagram only; it does not move the connection settings out of this card.

## Engineering constraints

- EC(ADR-0013/R1): every instant the store persists is written and read back as an integer number of UTC epoch seconds, for any instant, with no second representation of the same instant in an index key
  test: PropertyTest_Store_InstantsRoundTripAsEpochSeconds

## Guardrails

- G-1: Do not edit `api/openapi.yaml`, `internal/httpapi/**.go` — owned by CARD-001 this wave

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

- testable_acceptance_criteria — gap_free_by_non_applicability: not applicable — untraced candidate (`FR: —`, decompose 10b): there is no requirement to carry criteria from (ADR-0038/R1).
- engineering_constraint_coverage — gap_free
- architecture_traceability — gap_free
- blocking_decisions_resolved — gap_free
- dependencies_resolved — gap_free
- design_seed_for_user_facing — gap_free_by_non_applicability: not applicable — the card's own `UI surface:` machine stamp reads `none` (step 6b, FR-061); there is no surface to seed.
- open_subject_answered — gap_free

## Architecture context

- **FR:** —
- **NFR:** —
- **ADR:** ADR-0006, ADR-0007, ADR-0008, ADR-0013
- **Components:** COMP-005, COMP-006
- **Trace:** meta/architecture/trace.yml

## Failure matrix

Config values: `busyTimeout` = 5 s (constant in `internal/store/store.go`, set per connection through the DSN); `N` = number of embedded migrations. No retries anywhere in this card: every failure is returned to the caller once.

| # | Operation / boundary | Failure mode | Declared behavior | Numeric bound |
|---|---|---|---|---|
| F1 | `Open` — config | Poison input: empty path | Rejected before any I/O; no file created | 0 files touched, 0 retries |
| F2 | `Open` — file system | Path cannot be opened/created (missing dir, permissions) | `Open` returns the driver error; the pool is closed | 1 attempt, 0 retries, 0 leaked connections |
| F3 | `Open` — journal mode | Degraded dependency: file system does not honour WAL (`PRAGMA journal_mode` is not `wal`) | `Open` fails instead of silently running in rollback-journal mode | 1 check per `Open` |
| F4 | `Open` — DSN | Poison input: path with URI-special characters (`?`, `#`, `%`, space) | Path is percent-encoded into a `file:` URI; the file opened is exactly the given path | 0 misrouted files |
| F5 | Migration runner | Poison migration (SQL error in file k ≤ N) | All pending migrations run in ONE `BEGIN IMMEDIATE` transaction; it is rolled back; `user_version` and schema stay as before; `Open` fails | 0 partially applied migrations |
| F6 | Migration runner | Crash mid-migration (process killed) | SQLite rolls the uncommitted transaction back on the next open; the next `Open` re-applies every pending migration | 0 partially applied migrations; ≤ N migrations re-run once |
| F7 | Migration runner | Two `Open` calls on the same fresh file at once (duplicate delivery of startup) | The second blocks on `BEGIN IMMEDIATE`, then reads `user_version` inside its own write transaction and applies nothing | Each migration applied exactly once; the waiter blocks ≤ `busyTimeout` (5 s) per busy wait, then fails `SQLITE_BUSY`. No instance lock is added (DEC-029 stays open) |
| F7a | `Open` — journal mode | Two `Open` calls on a brand-new file (not yet WAL) at once | Observed on modernc.org/sqlite v1.61.0: the one-time switch of a new file to WAL can fail FAST with `SQLITE_BUSY` without consulting the busy handler; the losing `Open` returns that error and closes its pool. Not retried (single instance per ADR-0005; DEC-029 open). Once the file is WAL (persistent), F7 applies | 1 failed `Open`, 0 schema damage, 0 retries |
| F8 | Migration runner | Database newer than the binary (`user_version` > N) | Refused with `ErrSchemaTooNew`; transaction rolled back, nothing written | 0 writes |
| F9 | Migration runner | Malformed embedded set (bad name, gap, duplicate version) | Refused before the database is touched | 0 writes |
| F10 | `Open` | Timeout / cancellation of the caller's `ctx` | The driver's busy wait and running statement observe `ctx`; `Open` returns an error and closes the pool; the open transaction dies with its connection | Returns ≤ min(ctx deadline, `busyTimeout`) + one statement step after the lock wait starts |
| F11 | Every statement through the pool (later cards' inserts/updates) | Timeout under writer contention | SQLite single writer: a statement waits on the busy handler | ≤ `busyTimeout` (5 s) per statement, then `SQLITE_BUSY` is returned (never classified as a unique violation) |
| F12 | Unique indexes | Retry / duplicate delivery of an insert | Rejected by the index and reported as `IsUniqueViolation(err) == true` (extended code `SQLITE_CONSTRAINT_UNIQUE` only); primary-key, foreign-key, check and type errors are NOT unique violations | ≤ 1 active booking per (resource id, slot start); ≤ 1 booking per idempotency key, under any number of retries |
| F13 | Schema — `booking.slot_start` | Poison input: non-integer instant (a second representation of an instant) | `STRICT` table rejects text/real values that do not convert losslessly to INTEGER | 0 non-integer index keys |
| F14 | Connection pool | Saturation under many concurrent writers | `database/sql` default (unbounded open connections); a connection blocked on the busy handler is held ≤ `busyTimeout` | Abandoned work: none — no goroutines are started by the store |
| F15 | `Close` | Graceful shutdown with statements in flight | `sql.DB.Close` refuses new statements and waits for in-flight ones; the last connection close checkpoints the WAL | Waits ≤ the longest in-flight statement, itself ≤ `busyTimeout` + its execution time |
| F16 | Store API | Non-cooperative callee | Not applicable: the store invokes no caller-supplied handlers or callbacks | — |

## Worktree notes

- [Env] forge 2026.10.10
Summary: `internal/store` opens SQLite through modernc.org/sqlite v1.61.0 (pinned in go.mod) behind database/sql, with `busy_timeout(5000)`, `journal_mode(WAL)` and `foreign_keys(1)` in the DSN, and migrates the schema inside `Open` from embedded SQL. Migration 0001 creates `resource`, `resource_working_hours` and `booking` (all STRICT) plus the two ADR-0008 unique indexes. `IsUniqueViolation(err)` tells a unique-index rejection apart from every other error. No booking read/write operations and no cancelled-to-active trigger (that is CARD-004's migration 0002, ADR-0010).
Public surface for later cards:
- `store.Open(ctx, path) (*store.Store, error)` — path from the caller's config (CON-005); empty path rejected; migrates before returning; closes the pool on any error.
- `(*Store).Close() error`.
- `store.IsUniqueViolation(err) bool` — true only for `SQLITE_CONSTRAINT_UNIQUE` (2067), i.e. either the active-slot index or the idempotency-key index; deliberately does not say which (ADR-0011 classifies from stored data). Primary-key (1555), foreign-key, check and datatype violations are false.
- `store.ErrSchemaTooNew` — database `user_version` above the newest embedded migration.
- Unexported, for later store methods: `toEpoch(time.Time) int64` and `fromEpoch(int64) time.Time` are the only conversions of instants into and out of SQL (ADR-0013); `s.db` is the `*sql.DB`.
STRUCTURE: one Go package `internal/store` holds both COMP-005 (store.go) and COMP-006's runner (migrate.go); COMP-006's SQL lives in `internal/store/migrations/` as data, not a Go package — the component diagram maps COMP-006 to that directory, and `go:embed` can only embed files at or below the embedding package, so the runner sits in `store`.
STRUCTURE: migration files are `migrations/NNNN_description.sql`, versions 1..N with no gaps; a released migration is never edited, a change is a new file (ADR-0007: rollback is a forward migration). The next free version is 0002 (CARD-004's trigger).
STRUCTURE: all pending migrations run in ONE `BEGIN IMMEDIATE` transaction on a dedicated `sql.Conn`, which also sets `PRAGMA user_version` — atomic schema + version; the write lock is taken before `user_version` is read so a concurrent starter waits and then applies nothing. This is not an instance lock (DEC-029 untouched).
STRUCTURE: `Store` is a struct over `*sql.DB`; later cards add methods (`func (s *Store) InsertBooking(...)` etc.) in new files under `internal/store/`. No repository interfaces here — the consuming components (booking, catalog) own their ports if they need them.
STRUCTURE: every table is `STRICT`, ids are `TEXT NOT NULL PRIMARY KEY` generated by the application (format left to the API contract), instants are `INTEGER` epoch seconds, status is `TEXT` CHECK-ed to `'active'|'cancelled'`, strings use the default BINARY collation (byte-for-byte, ADR-0015).
STRUCTURE: weekly working hours (ADR-0004) are a child table `resource_working_hours(resource_id, weekday, start_minute, end_minute)` with PRIMARY KEY (resource_id, weekday) = at most one interval per weekday; weekday is Go's `time.Weekday` (0 = Sunday); bounds are minutes since 00:00 UTC, multiples of 30, `0 <= start < end <= 1440`. The schema CHECKs mirror ADR-0004 as a backstop; the domain validation with stable error codes stays CARD-006's.
STRUCTURE: the busy timeout is a package constant (5 s), not configuration — no operational reason to vary it yet; CARD-003's 50-writer test is what proves it long enough.
`_txlock` verification (owed by the handoff, for ADR-0012's fallback): AVAILABLE on the pinned modernc.org/sqlite v1.61.0. The driver parses `_txlock` with values `deferred` (default), `immediate`, `exclusive` (case-insensitive; any other value fails the connection with `unknown _txlock`), and `BeginTx` then issues `BEGIN <mode>` for non-read-only transactions (driver source: sqlite.go parse, tx.go `begin`). Verified behaviourally by `TestDriver_TxLockImmediateTakesTheWriteLockAtBegin` (internal/store/txlock_test.go): with one `_txlock=immediate` transaction open, a deferred BEGIN on another connection succeeds, a second `_txlock=immediate` BEGIN waits the busy timeout and fails with SQLITE_BUSY at BEGIN, and `_txlock=sometimes` is rejected. So ADR-0006's "no such option" is wrong for this version, and ADR-0012's fallback is available. The store does not use it.
Engineering constraint EC(ADR-0013/R1): `TestPropertyTest_Store_InstantsRoundTripAsEpochSeconds` (internal/store/epoch_test.go) — Go only runs top-level functions named `Test…`, so the name carries a `Test` prefix; `go test ./... -run 'PropertyTest_Store_InstantsRoundTripAsEpochSeconds'` selects and runs it (verified). Property-based: 10 000 random instants (random second, random sub-second part, random UTC offset −14:00..+14:00) plus edge cases through the conversion, and 200+ through the database checking `typeof(slot_start) = 'integer'`, the read-back UTC instant, and that the same instant in another offset collides in the active-slot index (one key per instant). The seed is logged.
Invariants touched (schema level) and their property test: INV-001 (active slot unique) and INV-003 (key unique across all bookings) — `TestPropertySchema_ActiveSlotAndKeyStayUnique`, a model-based random sequence of inserts and cancellations. INV-004's trigger and INV-005's domain validation belong to CARD-004 / CARD-006.
Commands run: `go build ./...` ok; `go vet ./...` ok; `go test ./...` ok; `go test ./internal/store -race -count=30` ok.
Observation (F7a): on a brand-new file, two concurrent `Open`s can make the one-time switch to WAL fail fast with SQLITE_BUSY (busy handler not consulted). Declared, not retried — the service is single-instance (ADR-0005) and DEC-029 is open; CARD-003's bootstrap opens once. Concurrent opens of an existing WAL file (even unmigrated) are safe and apply each migration once.
go.mod: the `go` directive was normalised by the toolchain from `go 1.26` to `go 1.26.0` when the dependency was added.
- [Scope] go.mod, go.sum, internal/store/epoch.go, internal/store/epoch_test.go, internal/store/migrate.go, internal/store/migrate_test.go, internal/store/migrations/0001_create_resource_and_booking.sql, internal/store/schema_test.go, internal/store/store.go, internal/store/store_test.go, internal/store/txlock_test.go
- [Build gate] PASSED (full — go.mod changed, impact underivable; 1s; 26 tests)
- [Review 1/3] Score: 9.0 — crit: 0, imp: 0
- [Review sync] 1 report(s) → meta/review/
- [Review 1/3] Score: 9.0 ✓ threshold reached + no critical/important (Step 8h: 11/11 card rules addressed — 3 holds, 8 unchecked, 0 violated; minor_triage: 2 sampled of 2, neither should gate)
- [AC/EC check] pass 1 at head 54e0b15292a492c2b5032869d75f252c2ec5e0e6: EC-1 ✓ demonstrated (go test -run 'PropertyTest_Store_InstantsRoundTripAsEpochSeconds' ran TestPropertyTest_Store_InstantsRoundTripAsEpochSeconds, PASS; seeded 10,000 conversions + 200 DB round trips), G-1 ✓ demonstrated (diff/status list no api/openapi.yaml or internal/httpapi/ path). The agent also emitted a spurious NO-CRITERIA line, retracted in the same output (the card does carry EC-1). Checkmark deferred: the docs step commits after this point and done binds the checkmark to HEAD, so the gate is re-run on the post-docs HEAD.
- [Docs] forge:readme on internal/store, internal/store/migrations → 200ed1b docs(store): add READMEs (new files; no Forge-Provenance trailer on that commit)
- [AC/EC check] All criteria/constraints ✓ (head-sha: 200ed1b7e5eba348e8580a01b04639c562072944) (evidence):
  EC-1 ✓ demonstrated — evidence: go test -count=1 -v -run 'PropertyTest_Store_InstantsRoundTripAsEpochSeconds' ./... ran TestPropertyTest_Store_InstantsRoundTripAsEpochSeconds (subtests conversion, through_the_database) --- PASS; ok internal/store; seeded PCG, 10,000 conversions + 200+ DB round trips
  G-1 ✓ demonstrated — evidence: git diff --name-only main...HEAD + git status --porcelain list no api/openapi.yaml or internal/httpapi/** path (bounded check of this branch's changed files)
- [Done] untracked reviewer artefact meta/impact/impact-set-9ed245051f38-54e0b15292a4-worktree.json moved from the worktree to the main repo meta/impact/ (uncommitted) before done
- [Merge gate] dependency 2026-10-21T17:53:18Z REFUSED (deps:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855, mode=interactive, scan=incomplete) — 1 blocking, 0 allowlisted, 0 non-blocking finding(s)
- [Merge gate] dependency-item 2026-10-21T17:53:18Z blocks not-provided make-audit make-audit — audit: not-provided (no Makefile in the worktree)
- [Escalated] 2026-10-21T17:53:42Z — done refused at dependency gate (step 2c): audit: not-provided (no Makefile/make audit; govulncheck not installed; no meta/audit-allowlist.json) — review 9.0 and AC/EC/G gate passed, nothing merged · station: implementation (likely: project harness/ops setup, not a code defect) · route: provide the dependency audit the gate requires — /forge:harness to add a Makefile `audit` target per skills/harness/references/audit.md, install govulncheck, and add meta/audit-allowlist.json entries (id, reason, expires) for any licence outside the allowlist; then /kanban unblock CARD-002 and /kanban done CARD-002. Worktree ../booking-demo-card-002 and branch card/002-sqlite-store-migrations kept (head 200ed1b, checkmark bound to it).
- [Unblocked] (2026-10-22) — dependency gate remedy provided on main 5fdd66b: Makefile `audit` target (govulncheck v1.9.0 + go-licenses v2.0.1), meta/audit-allowlist.json (MIT, BSD-3-Clause; no vulnerability entries)
- [Rebase] 2026-10-22 — card branch rebased onto main 5fdd66b (forge:merge, no conflicts; worktree card notes stashed and restored around it) so the dependency gate finds the harness `make audit` target in the worktree; commits 54e0b15→19c612b, 200ed1b→e866099, patch-identical (`git range-diff` `=`); post-rebase `go test ./...` PASSED (tree a0d527e75baa919eaf3e8e3f39b99cb9b4986b91, 2s, 26 tests). The [AC/EC check] checkmark names 200ed1b, so review cycle 2 re-runs the gate over e866099.
- [Build gate] PASSED (full — go.mod changed, impact underivable; 2s; 26 tests)
- [Scope gate] cycle 2: in_scope (excess 2/14: internal/store/README.md, meta/review/; no comp spread, no poaching, no guardrail hits)
- [Review 2/3] Score: 9.0 — crit: 0, imp: 0 (full re-review of the rebased branch main 5fdd66b...e866099; report parse_review accepted, security: applicable (0 findings))
- [Review sync] 2 report(s) → meta/review/
- [Review 2/3] Score: 9.0 ✓ threshold reached + no critical/important (Step 8h: 11/11 card rules addressed — 3 holds, 8 unchecked, 0 violated)
- [Minor triage] verified: sampled 2/3 Minor finding(s), rationale recorded
- [Done] untracked reviewer artefact meta/impact/impact-set-5fdd66b02758-e86609959341-worktree.json moved from the worktree to the main repo meta/impact/ (uncommitted)
- [8h spot-check] ✗ ADR-0013/R1 not reproduced — cited span 0001:31-36 does not contain slot_start (booking table is 0001:27-34, slot_start INTEGER at line 30); every other element (toEpoch/fromEpoch, EC property test PASS, time.Now grep 0 hits) re-derived, no violation found
- [8h spot-check] ✗ CON-008 not reproduced — cited 0001:31 is `customer_name TEXT`, not the INTEGER instant column (slot_start is 0001:30); epoch.go, time.Now and RFC 3339 scans re-derived, no violation found
- [8h spot-check] ADR-0015/R1 reproduced (COLLATE/NOCASE 0 hits, index ON booking (resource_id, slot_start) WHERE active at 0001:37-39, TestSchema_KeysAndNamesCompareByteForByte PASS)
- [8h spot-check] precondition note: worktree held only bookkeeping (M meta/kanban/cards/CARD-002.md, untracked meta/review/*.yml) — no code delta; skeptics ran sequentially and changed no files
- [Scope gate] cycle 3: in_scope (no code change since cycle 2)
- [Review 3/3] Score: 9.0 — crit: 0, imp: 0 (full re-review at e866099 with the cycle-2 Step 8h re-derivation failures passed back; citations corrected; parse_review accepted, security: applicable (0 findings))
- [Review sync] 3 report(s) → meta/review/
- [Review 3/3] Score: 9.0 ✓ threshold reached + no critical/important (Step 8h: 11/11 card rules addressed — 3 holds, 8 unchecked, 0 violated)
- [Minor triage] verified: sampled 2/3 Minor finding(s), rationale recorded
- [8h spot-check] 3/3 sampled holds reproduced (ADR-0013/R1, ADR-0015/R1, CON-008)
- [AC/EC check] All criteria/constraints ✓ (head-sha: e86609959341fa2339b9e920b23637fbf6a4d060) (evidence):
  EC-1 ✓ demonstrated — evidence: go test ./... -run 'PropertyTest_Store_InstantsRoundTripAsEpochSeconds' -v -count=1 ran TestPropertyTest_Store_InstantsRoundTripAsEpochSeconds (internal/store/epoch_test.go:27) --- PASS (subtests conversion, through_the_database); seeded, 7 edge + 10,000 random conversions, 7 edge + 200 random DB round trips (typeof integer, value, UTC read-back, one index key per instant)
  G-1 ✓ demonstrated — evidence: git diff --name-only main...HEAD (13 files: go.mod, go.sum, internal/store/**) and git status --porcelain --untracked-files=all grepped for ^(api/openapi\.yaml|internal/httpapi/): 0 hits (bounded check of this branch's diff and working tree)
- [Docs] cycle 3: forge:readme step — internal/store/README.md and internal/store/migrations/README.md already current (no code change since 200ed1b/e866099); nothing written
- [Commit] cycle 3: nothing to commit (no fix delta; worktree holds only bookkeeping) — HEAD stays e86609959341fa2339b9e920b23637fbf6a4d060
- [Merge gate] dependency 2026-10-21T23:40:36Z PASS (deps:13d67800455b84b0a04c62d4b5ea61067053970380a5bdfa1c508d4f2c10bf36, mode=interactive, scan=scanned) — 0 blocking, 0 allowlisted, 24 non-blocking finding(s)
- [Done] 2026-10-22 — the implementation agent's `## Failure matrix` section existed only in the worktree card copy (sync-card-body carries Worktree notes lines only); copied verbatim into this main copy before the worktree removal so it is not lost
- [Trace] 7c skipped — Architecture context FR: — and NFR: —, no trace.yml link entry to write back
- [Merge gate] PASS (tree-sha a3590081c870787a6144f77da7dab038a0abf65e, scope=full, 1s, cmd="go test ./...")
