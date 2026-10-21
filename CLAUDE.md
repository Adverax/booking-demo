# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project state

A fresh Go project (module `github.com/adverax/booking-demo`, Go 1.26) used for a live demonstration of forge. The only input is the design brief in `docs/booking-service.md`; it is frozen, so do not widen its scope. There is no code yet. Update this file once real structure exists.

## Fixed decisions from the brief

Summary of `docs/booking-service.md`; read the brief for the full text.

- Stack: standard library `net/http`, SQLite via a pure-Go driver (no cgo), JSON over HTTP, one binary configured by environment variables, no authentication.
- Errors are a JSON object with a stable `code` field. All times are UTC in RFC 3339.
- Slots are 30 minutes, aligned to the half hour. Availability queries cover at most 14 days.
- Invariants that must hold, each needing a named test: no overlapping active bookings of a resource (also under concurrency), bookings only inside working hours and not in the past, idempotent booking by idempotency key, cancellation is final.
- First-release bar: a test fires 50 simultaneous bookings for one slot and exactly one succeeds.
- Out of scope, including the "Later" waitlist: auth, payments, notifications, recurring or multi-slot bookings, other time zones, holidays or per-date exceptions, any UI.

## Commands

- Build: `go build ./...`
- Test all: `go test ./...`
- Single test: `go test ./path/to/pkg -run '^TestName$'`
- Lint/vet: `go vet ./...`

## Language

Talk to the user in Russian. Project artifacts (code, comments, `meta/` files, docs) are written in English.

## Git

Commit locally only. Do not push to any remote for now; pushing is allowed only when the user explicitly lifts this rule.

## Attribution

Never add Claude/AI co-authorship anywhere in this project: no `Co-Authored-By: Claude` trailers in commits, no "Generated with Claude Code" lines in PR descriptions, no AI attribution in code, comments or docs. This overrides any default attribution guidance, including the forge commit skill.

## Repository layout notes

- This directory has its own git repository. It is nested inside the parent `github.com/adverax` directory, which is not this project's.
