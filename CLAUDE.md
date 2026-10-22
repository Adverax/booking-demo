# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project state

A Go project (module `github.com/adverax/booking-demo`, `go 1.26.0` with `toolchain go1.26.10`) used for a live demonstration of forge. The only input is the design brief in `docs/booking-service.md`; it is frozen, so do not widen its scope. Architecture lives in `meta/architecture/` (ADRs in `decisions/adr/`, platform baseline in `platform.yml`, `handoff.md` is generated and untracked); delivery cards and the board live in `meta/kanban/`. All nine delivery cards are merged: the API (resources, availability, booking, idempotent replay, cancellation) is complete against the brief, and the three release criteria are executable tests in `cmd/booking/release_test.go` (`TestRelease_*`). A systemd unit and environment sample for the single binary live in `deploy/`. Open design questions remain as DEC entries in `meta/architecture/decisions/open.yml`; they do not block the delivered scope.

## Fixed decisions from the brief

Summary of `docs/booking-service.md`; read the brief for the full text.

- Stack: standard library `net/http`, SQLite via a pure-Go driver (no cgo), JSON over HTTP, one binary configured by environment variables, no authentication.
- Errors are a JSON object with a stable `code` field. All times are UTC in RFC 3339.
- Slots are 30 minutes, aligned to the half hour. Availability queries cover at most 14 days.
- Invariants that must hold, each needing a named test: no overlapping active bookings of a resource (also under concurrency), bookings only inside working hours and not in the past, idempotent booking by idempotency key, cancellation is final.
- First-release bar: a test fires 50 simultaneous bookings for one slot and exactly one succeeds.
- Out of scope, including the "Later" waitlist: auth, payments, notifications, recurring or multi-slot bookings, other time zones, holidays or per-date exceptions, any UI.

## Commands

- Build, test, vet: `make build`, `make test`, `make vet` (or `go build ./...`, `go test ./...`, `go vet ./...`)
- Single test: `go test ./path/to/pkg -run '^TestName$'`
- Dependency audit: `make tools` once, then `make audit`. The target always exits 0, so read `.audit/govulncheck.status` and `.audit/licenses.status` (both must be `ok`) instead of the exit code. Licences must be allowlisted in `meta/audit-allowlist.json`.
- Regenerate the API server after editing `api/openapi.yaml`: `go generate ./...` (no drift is allowed).
- Run: `BOOKING_DB_PATH=<file> [BOOKING_ADDR=:8080] go run ./cmd/booking`
- Release criteria only: `go test -run TestRelease ./cmd/booking/`

## Architecture in brief

One Go binary (`cmd/booking`) wiring `internal/httpapi` (server generated from `api/openapi.yaml`, ADR-0009), `internal/booking` and `internal/catalog` (domain and ports), and `internal/store` (SQLite through `modernc.org/sqlite`, embedded migrations applied at startup). Two properties span several files: the no-double-booking and idempotency invariants live in the schema as unique indexes (ADR-0008) and the booking write path is a single autocommit insert whose unique violation is classified by reading the stored booking (ADR-0011, ADR-0012); all instants are stored as integer UTC epoch seconds and the current time comes only from an injected clock (ADR-0013).

## Language

Talk to the user in Russian. Project artifacts (code, comments, `meta/` files, docs) are written in English.

## Git

Commit locally only. Do not push to any remote for now; pushing is allowed only when the user explicitly lifts this rule.

## Attribution

Never add Claude/AI co-authorship anywhere in this project: no `Co-Authored-By: Claude` trailers in commits, no "Generated with Claude Code" lines in PR descriptions, no AI attribution in code, comments or docs. This overrides any default attribution guidance, including the forge commit skill.

## Repository layout notes

- This directory has its own git repository. It is nested inside the parent `github.com/adverax` directory, which is not this project's.
