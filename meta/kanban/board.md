# Kanban Board

_Updated: 2026-10-22 12:03_

## Wave plan
| Wave | Cards | Status |
|------|-------|--------|
| 1 | CARD-001 P1, CARD-002 P1 | ✓ done |
| 2 | CARD-003 P1 | ✓ done |
| 3 | CARD-004 P1, CARD-006 P2 | ✓ done |
| 4 | CARD-005 P1 | ✓ done |
| 5 | CARD-007 P1, CARD-008 P1 | ▶ active ⚠ (CARD-008 blocked) |
| 6 | CARD-009 P1 | ⏳ blocked (→ CARD-008) |

_Gantt: [meta/kanban/gantt.md](gantt.md)_

## Ready
_(none)_

## In Progress
_(none)_

## Review
_(none)_

## Done
- **CARD-001** OpenAPI contract and generated server interface
- **CARD-002** SQLite store and embedded schema migrations
- **CARD-003** Walking skeleton: bootstrap, minimal resource creation and booking one slot
- **CARD-004** Cancel a booking and keep cancellation final
- **CARD-005** Idempotent booking replay with insert-then-classify
- **CARD-006** Resource catalog: validated creation, list and read
- **CARD-007** Booking validation: working hours, past slots, alignment, unknown resource

## Blocked
- **CARD-008** 🔒 Availability search over a date range  ← deferred to next wave
- **CARD-009** 🔒 Release criteria suite and supervisor deployment sample  ← readiness — blocking_decisions_resolved, dependencies_resolved
