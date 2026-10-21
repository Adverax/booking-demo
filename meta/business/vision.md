# Vision — Booking Service

## Problem

A small business (a clinic, a coworking space, a barbershop) needs to publish when
its resources are available and let customers book time slots without ever
double-booking a resource.

## Goal

A small backend service, reached only through its HTTP API, that lets an operator
publish resources with weekly working hours and lets customers book and cancel
30-minute slots safely, also under concurrent requests.

## Actors

- **Operator** — staff of the business. Creates resources and sets their working hours.
- **Customer** — books and cancels slots through the studio's frontend, which calls the
  API on the customer's behalf.

## The world it lives in

A studio builds the booking product for a small business client. The studio owns the
frontend and the client relationship. This service is the backend behind it, runs
behind the studio's own gateway, and is not exposed to end users directly.

## Success metrics

- (none agreed yet)

The first-release criteria in the design brief are technical and live in
`raw-requirements.md`.

## Non-goals

- Authentication and user accounts.
- Payments, deposits, pricing.
- Notifications (email, SMS, reminders).
- Recurring bookings, multi-slot bookings, time zones other than UTC.
- Holidays and per-date exceptions to working hours.
- Any user interface.

## Source

Frozen design brief: `docs/booking-service.md` (2026-10-21). Scope is not widened
beyond it.
