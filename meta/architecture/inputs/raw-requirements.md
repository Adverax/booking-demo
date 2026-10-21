# Raw requirements

# processed: 2026-10-21 → FR-003
- Slots are 30 minutes long and aligned to the half hour (FR). Source: brief, 2026-10-21.
# processed: 2026-10-21 → FR-003
- A slot is free when it falls inside working hours and does not overlap an active booking (FR). Source: brief, 2026-10-21.
# processed: 2026-10-21 → FR-004
- Availability is queried for a resource and a date range of at most 14 days (FR). Source: brief, 2026-10-21.
# processed: 2026-10-21 → FR-005
- A booking is accepted only for a slot inside working hours and not in the past (FR). Source: brief, 2026-10-21.
# processed: 2026-10-21 → FR-005 (INV-001)
- Two active bookings of the same resource never overlap, also under concurrent requests (FR, invariant). Source: brief, 2026-10-21.
# processed: 2026-10-21 → FR-006
- Repeating a booking request with the same idempotency key returns the original booking and creates nothing new (FR, invariant). Source: brief, 2026-10-21.
# processed: 2026-10-21 → FR-006
- The idempotency key is globally unique across the service, not scoped per resource (FR). Source: user, 2026-10-21.
# processed: 2026-10-21 → FR-008
- A cancelled booking cannot be reactivated; a new booking is needed (FR, invariant). Source: brief, 2026-10-21.
# processed: 2026-10-21 → FR-007
- A cancelled booking frees its slot immediately (FR). Source: brief, 2026-10-21.
# processed: 2026-10-21 → FR-007
- Anyone who knows a booking id may cancel it; the service does not verify identity (FR). Source: user, 2026-10-21.
# processed: 2026-10-21 → CON-001
- All times are stored and returned in UTC, in RFC 3339 format (CON). Source: brief, 2026-10-21.
# processed: 2026-10-21 → CON-002
- Go 1.26, standard library `net/http` for the HTTP layer (CON). Source: brief, 2026-10-21.
# processed: 2026-10-21 → CON-003
- SQLite is the only storage, through a pure-Go driver, no cgo (CON). Source: brief, 2026-10-21.
# processed: 2026-10-21 → CON-004
- JSON over HTTP; errors are a JSON object with a stable `code` field (CON). Source: brief, 2026-10-21.
# processed: 2026-10-21 → CON-005
- One binary, configured through environment variables (CON). Source: brief, 2026-10-21.
# processed: 2026-10-21 → CON-006
- No authentication: the service runs behind the studio's own gateway (CON). Source: brief, 2026-10-21.
# processed: 2026-10-21 → NFR-002
- The full API works end to end against a fresh database (NFR, release criterion). Source: brief, 2026-10-21.
# processed: 2026-10-21 → NFR-001
- A concurrency test sends 50 simultaneous booking requests for the same slot and exactly one succeeds (NFR, release criterion). Source: brief, 2026-10-21.
# processed: 2026-10-21 → NFR-003
- Every rule above is covered by at least one named test (NFR, release criterion). Source: brief, 2026-10-21.
- A booking request that repeats an existing idempotency key with a different resource, slot or customer name is rejected with a stable error code and creates nothing (FR, invariant). Source: ADR-0001, 2026-10-21.
- A booking request that repeats the idempotency key of a cancelled booking returns that booking with status cancelled and creates no new booking; a new booking needs a new key (FR, invariant). Source: ADR-0002, 2026-10-21.
- Cancelling an already cancelled booking succeeds and returns the cancelled booking (FR). Source: ADR-0003, 2026-10-21.
- Weekly working hours have at most one interval per weekday; interval bounds are aligned to the half hour and lie inside a single UTC day; resources that violate this are rejected with a stable error code (FR). Source: ADR-0004, 2026-10-21.
- A cancelled booking is never set active again; the storage layer rejects the transition, not only the domain code (FR, invariant). Source: ADR-0010, 2026-10-21.
- When a booking insert violates a uniqueness constraint, the outcome is decided by reading the booking stored under the idempotency key: same payload returns the original booking, different payload is the key conflict, no stored booking means the slot was taken (FR). Source: ADR-0011, 2026-10-21.
- The booking write path uses single autocommit statements and no transaction that reads and then writes (CON). Source: ADR-0012, 2026-10-21.
- Instants are stored as integer UTC epoch seconds, converted to RFC 3339 only at the API boundary; the current time is obtained through an injectable clock (CON). Source: ADR-0013, 2026-10-21.
- Sameness: two resources are the same if their ids are equal; two bookings occupy the same slot if resource id and canonical slot start are equal; idempotency keys and customer names are compared byte for byte as received, with no trimming and no case folding; a repeated request has the same payload as the stored booking only if resource id, slot start and customer name are all equal (FR, invariant). Source: ADR-0015, 2026-10-21.
