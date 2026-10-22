# availability

## Purpose

The Availability Search component (COMP-003): a read-only projection that lists the free slots of one resource over a range of UTC days, both ends inclusive, at most 14 days (FR-003, FR-004). A free slot is 30 minutes long, starts on the UTC half hour, lies inside the resource's working hours and does not overlap an active booking; cancelled bookings do not occupy their slot.

## Architecture

- Two ports, owned here: `Resources` reads working hours through the Resource Catalog (COMP-002, ASM-0007; `*catalog.Catalog` implements it) and `Bookings` reads the active booking starts of one resource in a window (`*store.Store` implements it with one autocommit `SELECT`, ADR-0012).
- The order is fixed: the range is checked first (`ErrInvalidRange`: from after to, or more than 14 days), before anything is read (ADR-0018, AC-054); then the resource (`ErrUnknownResource`); then the bookings.
- Slots are generated, not tested: each working interval is enumerated in half-hour steps from `Start` up to `End`. The catalog guarantees half-hour aligned bounds (INV-005), so there is no slot-containment predicate here.
- All arithmetic is on integer UTC epoch seconds (ADR-0013). Only the UTC calendar day of `from` and `to` counts. The component reads no clock: past slots are listed like any other (ASM-0002).
- The package does not depend on Slot Booking; its `SlotLength` is pinned equal to `booking.SlotLength` by a test.

## Key entities

| Name | Kind | Description | File |
|------|------|-------------|------|
| `Service` / `NewService` | type | Answers `FreeSlots(ctx, resourceID, from, to)` | [`availability.go`](./availability.go) |
| `Slot` | type | One free slot `[Start, End)` in UTC | [`availability.go`](./availability.go) |
| `Resources`, `Bookings` | interface | Ports to the catalog and the store | [`availability.go`](./availability.go) |
| `ErrInvalidRange`, `ErrUnknownResource` | var | Rejections, mapped to 400 `validation_error` and 404 `not_found` by `internal/httpapi` | [`availability.go`](./availability.go) |

## Usage

```go
svc := availability.NewService(catalog.New(st), st)
slots, err := svc.FreeSlots(ctx, resourceID, from, to)
```

## See also

- [`internal/httpapi`](../httpapi/README.md) - the `getAvailability` operation
- ADR-0004, ADR-0013, ADR-0018 in `meta/architecture/decisions/adr/`
