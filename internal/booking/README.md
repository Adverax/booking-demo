# booking

## Purpose

The Slot Booking component (COMP-004): the Booking aggregate (AGG-002) and the service that books one 30-minute slot of a resource under an idempotency key (FR-005).

## Architecture

- `Service.Book` normalises the requested start to its canonical UTC half-hour start (`CanonicalSlotStart`, ADR-0013, ADR-0015) and stores the booking through the `Repository` port with one insert. There is no read before the insert: the store's unique indexes decide the winner (ADR-0008, ADR-0012), and a rejection (`ErrNotUnique`) is answered as `ErrSlotTaken`.
- Telling a replay or a key conflict apart from a taken slot (insert-then-classify, ADR-0011) is not done yet; it is CARD-005's.
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
| `ErrSlotTaken`, `ErrNotUnique` | var | Slot already booked; insert rejected by a unique index | [`booking.go`](./booking.go) |
| `ErrNotFound`, `ErrBookingCancelled` | var | No booking with the id; a change would leave `cancelled` | [`booking.go`](./booking.go) |

## Usage

```go
svc := booking.NewService(st, time.Now) // st implements booking.Repository
b, err := svc.Book(ctx, booking.Request{ResourceID: id, SlotStart: start, CustomerName: "Ann", IdempotencyKey: key})
```

## See also

- [`internal/store`](../store/README.md), the adapter of `Repository`
- ADR-0008, ADR-0011, ADR-0012, ADR-0013, ADR-0015 in `meta/architecture/decisions/adr/`
