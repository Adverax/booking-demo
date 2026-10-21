# User stories

## Booking Service (2026-10-21)

US-001: As an operator, I want to create a resource with a name and weekly working
        hours (for example Mon-Fri 09:00-17:00), so that customers can book it
        within those hours.
US-002: As an operator, I want to list resources and read a single one, so that I can
        check what is published.
US-003: As a customer, I want to list resources and read a single one, so that I can
        choose what to book.
US-004: As a customer, I want to see the free slots of a resource for a date range of
        at most 14 days, so that I can pick a time that is actually available.
US-005: As a customer, I want to book one slot of a resource by giving my name and an
        idempotency key, so that I get a booking for that time.
US-006: As a customer, I want a repeated booking request with the same idempotency
        key to return my original booking, so that a retry never creates a second
        booking.
US-007: As a customer, I want to cancel a booking, so that its slot is freed
        immediately for someone else.
