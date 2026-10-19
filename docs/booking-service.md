# Booking Service — design brief

A small backend service that lets a business publish when its resources are
available and lets customers book time slots without double bookings.

This brief is frozen for the live demo. Anything not written here is out of
scope.

## Context

A studio builds a booking product for a small business client (a clinic, a
coworking space, a barbershop). The studio owns the frontend and the client
relationship. This service is the backend behind it, reached only through its
HTTP API.

## Users

- **Operator** — staff of the business. Creates resources and sets their
  working hours.
- **Customer** — books and cancels slots through the studio's frontend. The
  frontend calls the API on the customer's behalf.

## Capabilities

1. **Resources.** An operator creates a resource (a specialist, a room, a desk)
   with a name and weekly working hours, for example Mon–Fri 09:00–17:00.
   Resources can be listed and read.
2. **Availability.** For a resource and a date range of at most 14 days, the
   service returns the free slots. Slots are 30 minutes long and aligned to the
   half hour. A slot is free when it falls inside working hours and does not
   overlap an active booking.
3. **Booking.** A customer books one slot of a resource, giving a customer
   name and an idempotency key. The service returns the booking.
4. **Cancellation.** A booking can be cancelled. A cancelled booking frees its
   slot immediately.

## Rules that must always hold

- **No double booking.** Two active bookings of the same resource never
  overlap, also under concurrent requests.
- **Bookable means available.** A booking is accepted only for a slot that is
  inside working hours and not in the past.
- **Idempotent booking.** Repeating a booking request with the same idempotency
  key returns the original booking and creates nothing new.
- **Cancellation is final.** A cancelled booking cannot be reactivated; a new
  booking is needed.

## Decisions already made

- Go 1.26, standard library `net/http` for the HTTP layer.
- SQLite as the only storage, through a pure-Go driver (no cgo).
- All times are stored and returned in UTC, in RFC 3339 format.
- JSON over HTTP; errors as a JSON object with a stable `code` field.
- One binary, configured through environment variables.
- No authentication: the service runs behind the studio's own gateway.

## Out of scope

- Authentication and user accounts.
- Payments, deposits, pricing.
- Notifications (email, SMS, reminders).
- Recurring bookings, multi-slot bookings, time zones other than UTC.
- Holidays and per-date exceptions to working hours.
- Any user interface.

## Later, not now

- **Waitlist.** When a slot is taken, a customer can join its waitlist and is
  booked automatically when the slot is cancelled.

## Success criteria for the first release

- The full API works end to end against a fresh database.
- A concurrency test sends 50 simultaneous booking requests for the same slot
  and exactly one succeeds.
- Every rule above is covered by at least one named test.
