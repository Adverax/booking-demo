# Kanban Board

_Updated: 2026-10-22 09:31_

## Wave plan
| Wave | Cards | Status |
|------|-------|--------|
| 1 | CARD-001 P1, CARD-002 P1 | ✓ done |
| 2 | CARD-003 P1 | ▶ active |
| 3 | CARD-004 P1, CARD-006 P2 | ⏳ blocked (→ CARD-003) |
| 4 | CARD-005 P1 | ⏳ blocked (→ CARD-004) |
| 5 | CARD-007 P1, CARD-008 P1 | ⏳ blocked (→ CARD-004, CARD-005, CARD-006) |
| 6 | CARD-009 P1 | ⏳ blocked (→ CARD-007, CARD-008) |

_Gantt: [meta/kanban/gantt.md](gantt.md)_

## Ready
- **CARD-003** P1 · Walking skeleton: bootstrap, minimal resource creation and booking one slot  _(wave 2)_
- **CARD-004** P1 · Cancel a booking and keep cancellation final  _(wave 3)_
- **CARD-005** P1 · Idempotent booking replay with insert-then-classify  _(wave 4)_
- **CARD-006** P2 · Resource catalog: validated creation, list and read  _(wave 3)_
- **CARD-007** P1 · Booking validation: working hours, past slots, alignment, unknown resource  _(wave 5)_
- **CARD-008** P1 · Availability search over a date range  _(wave 5)_

## In Progress
_(none)_

## Review
_(none)_

## Done
- **CARD-001** OpenAPI contract and generated server interface
- **CARD-002** SQLite store and embedded schema migrations

## Blocked
- **CARD-009** 🔒 Release criteria suite and supervisor deployment sample  ← readiness — blocking_decisions_resolved, dependencies_resolved
