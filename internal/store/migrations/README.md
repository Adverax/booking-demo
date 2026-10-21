# migrations

## Purpose

Versioned SQL schema files for the booking database (COMP-006). They are data, not a Go package: the runner is `migrate.go` in the parent [`store`](../README.md) package, which embeds `*.sql` from this directory.

## Rules

- File names are `NNNN_description.sql` (lowercase, digits, underscores); versions start at 1 and have no gaps. A malformed set is rejected before any database work.
- A released migration is never edited; a change is a new file (ADR-0007: rollback is a forward migration). The next free version is 0002.
- The database version is stored in `PRAGMA user_version`.

## Contents

| File | Creates |
|------|---------|
| [`0001_create_resource_and_booking.sql`](./0001_create_resource_and_booking.sql) | `resource`, `resource_working_hours` (ADR-0004 shape), `booking`, unique index `booking_active_slot_uq` (one active booking per resource and slot, ADR-0008) and `booking_idempotency_key_uq` (key unique across all bookings, ADR-0015) |

Every table is `STRICT`; ids are application-generated `TEXT`; instants are `INTEGER` UTC epoch seconds (ADR-0013); strings use the default BINARY collation.
