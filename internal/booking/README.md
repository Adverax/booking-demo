# booking

## Purpose

The Slot Booking component (COMP-004): the Booking aggregate (AGG-002) and the service that books one 30-minute slot of a resource under an idempotency key (FR-005, FR-006).

## Architecture

- `Service.Book` validates, inserts, then classifies (ADR-0011, ADR-0020). Validation stops at the first rejection, in this order, and stores nothing:
  1. input shape, with no statement: an empty idempotency key, an empty customer name, or a slot start not on the UTC half hour is `ErrInvalidRequest` (ADR-0018, AC-017); a start spelled in another zone is kept as its canonical UTC start (`CanonicalSlotStart`, ADR-0013, ADR-0015);
  2. the resource, read through the `Resources` port (the Resource Catalog, COMP-002): an unknown id is `ErrUnknownResource`;
  3. its working hours: a slot not inside the half-open interval of its UTC weekday (an end of 24:00 is the end of the day, ADR-0004) is `ErrOutsideWorkingHours`;
  4. the past check, last: a slot whose start is earlier than the injected clock's now, compared as integer UTC epoch seconds, is in the past (ADR-0016); a start equal to now is accepted.
- Only a past slot looks the key up before answering (ADR-0020): one autocommit read; the same payload returns the stored booking (`replayed = true`), a different payload is `ErrKeyConflict`, no booking under the key is `ErrSlotInPast`. Shape, resource and working-hours rejections never consult the key.
- A valid request is stored through the `Repository` port with one insert. There is no key read before the insert (ADR-0020 rejects lookup first): the store's unique indexes decide the winner (ADR-0008, ADR-0012). A rejection (`ErrNotUnique`) is classified by insert-then-classify (ADR-0011): one separate autocommit read of the booking stored under the request's key (`Repository.GetBookingByIdempotencyKey`) decides, never which index fired. Same payload (`Booking.SamePayload`: resource id, canonical slot start, byte-equal customer name) returns the stored booking in its current state, cancelled included, with `replayed = true` (ADR-0002); a different payload is `ErrKeyConflict` (ADR-0001); no booking under the key is `ErrSlotTaken`. The past path and the classifier share this one comparison (`resolveStoredKey`).
- The current time comes only from the clock injected into `NewService` (ADR-0013). `TestProperty_Booking_SlotStartStoredAsCanonicalEpochSeconds` fails if a production file of this package calls `time.Now`.
- Keys and names are kept byte for byte as received (ADR-0015).
- `Service.Cancel` cancels by id alone, no identity check (FR-007, CON-006), with the store's single `UPDATE` (ADR-0012); a repeated cancel returns the cancelled booking again (ADR-0003); an unknown id is `ErrNotFound`. `Service.Get` reads one booking.
- Cancelled is final (INV-004, ADR-0010) twice over: `Booking.WithStatus` is the domain transition rule, and the schema trigger of migration 0002 aborts any update out of `cancelled`, reported by the store as `ErrBookingCancelled`.

## Key entities

| Name | Kind | Description | File |
|------|------|-------------|------|
| `Booking` | type | One booked slot; `SlotStart` is canonical UTC | [`booking.go`](./booking.go) |
| `CanonicalSlotStart` | func | Start of the UTC half-hour slot containing an instant | [`booking.go`](./booking.go) |
| `Booking.WithStatus` | method | Transition rule: a cancelled booking stays cancelled | [`booking.go`](./booking.go) |
| `Service` | type | Books (`Book`), cancels (`Cancel`) and reads (`Get`) a booking | [`booking.go`](./booking.go) |
| `Repository` | interface | Port to the store: one autocommit statement per operation | [`booking.go`](./booking.go) |
| `Resources` | interface | Port to the Resource Catalog: reads a resource's working hours | [`booking.go`](./booking.go) |
| `ErrUnknownResource`, `ErrOutsideWorkingHours`, `ErrSlotInPast` | var | Validation rejections: no such resource; outside working hours; past slot with no booking under the key | [`booking.go`](./booking.go) |
| `Booking.SamePayload` | method | Payload sameness of a repeated key (ADR-0015) | [`booking.go`](./booking.go) |
| `ErrSlotTaken`, `ErrNotUnique` | var | Slot already booked; insert rejected by a unique index | [`booking.go`](./booking.go) |
| `ErrKeyConflict`, `ErrInvalidRequest` | var | Key stored with another payload; request rejected before any statement | [`booking.go`](./booking.go) |
| `ErrNotFound`, `ErrBookingCancelled` | var | No booking with the id; a change would leave `cancelled` | [`booking.go`](./booking.go) |

## Usage

```go
cat := catalog.New(st)
svc := booking.NewService(st, cat, time.Now) // st implements booking.Repository, cat booking.Resources
b, replayed, err := svc.Book(ctx, booking.Request{ResourceID: id, SlotStart: start, CustomerName: "Ann", IdempotencyKey: key})
```

## See also

- [`internal/store`](../store/README.md), the adapter of `Repository`
- [`internal/catalog`](../catalog/README.md), the implementation of `Resources`
- ADR-0001, ADR-0002, ADR-0004, ADR-0008, ADR-0011, ADR-0012, ADR-0013, ADR-0015, ADR-0016, ADR-0018, ADR-0020 in `meta/architecture/decisions/adr/`
