# Assumptions

## ASM-0001 · AC-030 is reachable through an internal state-machine guard
- status: unvalidated
- owner: roman.miakotin
- source: architect critic, capability-mapping and domain-extraction, 2026-10-21
- date: 2026-10-21
- check: "by synthesis, AC-030 names the entry point that attempts to reactivate a cancelled booking, or is reworded as a guard-level property; settled by 2026-11-30 when the API routes are specified"

The refusal to set a cancelled booking active again is enforced by an internal guard of the Booking aggregate, with no public reactivation route.

## ASM-0002 · Listing past slots as free in availability is acceptable
- status: unvalidated
- owner: roman.miakotin
- source: architect critic, domain-extraction, 2026-10-21
- date: 2026-10-21
- check: "an acceptance criterion states what availability returns for a range that includes past times, settled by 2026-11-30 when FR-003 acceptance criteria are reviewed"

A free slot is defined without the not-in-the-past condition, so the studio's frontend accepts that availability may list a past slot that a booking then rejects.

## ASM-0003 · Overlap equals the same slot, so INV-001 needs no partial-overlap test
- status: unvalidated
- owner: roman.miakotin
- source: architect critic, domain-extraction, 2026-10-21
- date: 2026-10-21
- check: "slots stay fixed 30-minute half-hour-aligned (FR-003); revisit by 2026-11-30 and whenever slot length or alignment becomes configurable"

Because slots are fixed at 30 minutes and aligned to the half hour, two bookings overlap only when they are for the same slot.

## ASM-0004 · The StudioGateway edge is adequately recorded as one open-host-service relation
- status: unvalidated
- owner: roman.miakotin
- source: architect critic, context-mapping, 2026-10-21
- date: 2026-10-21
- check: "by 2026-11-30, when the API is specified, confirm that Customer (TERM-010) means the same thing on the service side and the gateway side, or record the difference as a language seam"

The StudioGateway is the sole caller and the open-host-service relation captures the integration; the Customer term does not change meaning across that edge.

## ASM-0005 · Generated HTTP plumbing honours the stable error contract for every failure
- status: unvalidated
- owner: roman.miakotin
- source: architect critic, adr-writer, 2026-10-21
- date: 2026-10-21
- check: "by 2026-11-30, when the OpenAPI document and the generated server are set up, show that a malformed body, an unknown route and a wrong method all return a JSON object with a stable code (CON-004)"

The generated net/http server, with a custom error handler where needed, returns the stable JSON error shape for malformed requests, unknown routes and wrong methods, not only for domain errors.

## ASM-0006 · Every writer normalises the slot start before insert
- status: unvalidated
- owner: roman.miakotin
- source: architect critic, adr-writer, 2026-10-21
- date: 2026-10-21
- check: "by 2026-11-30, when the storage layer is specified, the slot start is stored in one canonical UTC form (or as integer epoch seconds) so the unique index on the active slot compares equal starts as equal"

The only path that inserts a booking parses the slot start into a single canonical UTC representation, so two spellings of the same instant cannot produce two index keys.

## ASM-0007 · The slot rules have a single owning component
- status: unvalidated
- owner: roman.miakotin
- source: architect critic, synthesis, 2026-10-21
- date: 2026-10-21
- check: "by 2026-11-30, when the component interfaces are specified, half-hour alignment and the not-in-the-past rule are implemented in one place and shared by availability and booking, not twice"

Half-hour alignment of a slot start and the not-in-the-past check are implemented once and used by both availability search and slot booking.
