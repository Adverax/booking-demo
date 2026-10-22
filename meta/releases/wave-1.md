# Wave 1 — 2026-10-22   (tag: wave-1)

## Shipped
- CARD-001 (feature): Added the OpenAPI contract for resources, availability and bookings with one stable set of error codes, and the generated net/http server interface mounted on http.ServeMux with JSON error responses for malformed requests.   score 9.5   FR —
- CARD-002 (feature): Added the SQLite store with WAL, busy timeout and foreign keys, and embedded schema migrations that create the resource and booking tables with the slot and idempotency-key uniqueness indexes on startup.   score 9.0   FR —

## Requirements closed
- none targeted: neither card traces to an FR/NFR (enablers of Increment 1, whose requirements close with CARD-003 in wave 2); goal-backward check skipped with an empty target set.

## Convergence
- ✅ converged — no target FR/NFR this wave (nothing to demonstrate; Increment 1's checkpoint is attached to wave 2).

## Known gaps / escalations
- The run escalated both cards on 2026-10-21 (CARD-001: scope gate runaway over the shared go.mod/go.sum; CARD-002: dependency gate `audit: not-provided`). Both were resolved by hand (harness `make audit`, audit allowlist, toolchain pin) and merged through `unblock`/`done`; the stale `dispatcher.state → escalated` entries were cleared at `run resume` (2026-10-22) via `dispatcher_state.py unpark`.
- Post-wave docs: forge:docs not applicable (no user-facing FR); forge:explain and forge:harness sync / root README were not run in this close-out (see the wave-2 report).

## Migrations
- internal/store/migrations/0001_create_resource_and_booking.sql (applied at open by the embedded runner, ADR-0007)

## Smoke test
- 2026-10-22, on main 51ee8b2: `go build ./... && go test ./...` PASS (2s); `make build test vet` PASS.
