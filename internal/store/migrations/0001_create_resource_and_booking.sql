-- 0001: resources with weekly working hours, and bookings.
--
-- Every table is STRICT, so a column holds exactly its declared type and an
-- instant can never be stored in a second representation (ADR-0013).

CREATE TABLE resource (
    id   TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL
) STRICT;

-- Weekly working hours in the shape of ADR-0004: at most one interval per
-- weekday (the primary key), both bounds aligned to the half hour, inside a
-- single UTC day. weekday follows Go's time.Weekday (0 = Sunday). Bounds are
-- minutes since 00:00 UTC; end_minute = 1440 means "until midnight".
CREATE TABLE resource_working_hours (
    resource_id  TEXT    NOT NULL REFERENCES resource (id),
    weekday      INTEGER NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    start_minute INTEGER NOT NULL CHECK (start_minute % 30 = 0),
    end_minute   INTEGER NOT NULL CHECK (end_minute % 30 = 0),
    PRIMARY KEY (resource_id, weekday),
    CHECK (0 <= start_minute AND start_minute < end_minute AND end_minute <= 1440)
) STRICT, WITHOUT ROWID;

-- slot_start is the canonical slot start as integer UTC epoch seconds
-- (ADR-0013, CON-008). customer_name and idempotency_key use the default
-- BINARY collation: compared byte for byte as received (ADR-0015).
CREATE TABLE booking (
    id              TEXT    NOT NULL PRIMARY KEY,
    resource_id     TEXT    NOT NULL REFERENCES resource (id),
    slot_start      INTEGER NOT NULL,
    customer_name   TEXT    NOT NULL,
    idempotency_key TEXT    NOT NULL,
    status          TEXT    NOT NULL CHECK (status IN ('active', 'cancelled'))
) STRICT;

-- ADR-0008: no two active bookings of one resource share a slot start (INV-001).
CREATE UNIQUE INDEX booking_active_slot_uq
    ON booking (resource_id, slot_start)
    WHERE status = 'active';

-- ADR-0008, ADR-0015: an idempotency key is unique across all bookings,
-- whatever their status (INV-003).
CREATE UNIQUE INDEX booking_idempotency_key_uq
    ON booking (idempotency_key);
