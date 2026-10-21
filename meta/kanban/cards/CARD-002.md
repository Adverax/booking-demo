# CARD-002: SQLite store and embedded schema migrations

**Status:** ready
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
**Review score:** —
**Started:** —
**Closed:** —
**Actual:** —
**Merge commit:** —
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

## Worktree notes

—
