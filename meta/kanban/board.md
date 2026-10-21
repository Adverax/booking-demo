# Kanban Board

_Updated: 2026-10-21 19:47_

## Wave plan
| Wave | Cards | Status |
|------|-------|--------|
| 1 | CARD-001 P1, CARD-002 P1 | ▶ active |
| 2 | CARD-003 P1 | ⏳ blocked (→ CARD-001, CARD-002) |
| 3 | CARD-004 P1, CARD-006 P2 | ⏳ blocked (→ CARD-003) |
| 4 | CARD-005 P1 | ⏳ blocked (→ CARD-004) |
| 5 | CARD-007 P1, CARD-008 P1 | ⏳ blocked (→ CARD-004, CARD-005, CARD-006) |
| 6 | CARD-009 P1 | ⏳ blocked (→ CARD-007, CARD-008) |

_Gantt: [meta/kanban/gantt.md](gantt.md)_

## Ready
- **CARD-001** P1 · OpenAPI contract and generated server interface  _(wave 1)_
- **CARD-002** P1 · SQLite store and embedded schema migrations  _(wave 1)_

## In Progress
_(none)_

## Review
_(none)_

## Done
_(none)_

## Blocked
- **CARD-003** 🔒 Walking skeleton: bootstrap, minimal resource creation and booking one slot  ← readiness — blocking_decisions_resolved
- **CARD-004** 🔒 Cancel a booking and keep cancellation final  ← readiness — dependencies_resolved
- **CARD-005** 🔒 Idempotent booking replay with insert-then-classify  ← readiness — blocking_decisions_resolved, dependencies_resolved
- **CARD-006** 🔒 Resource catalog: validated creation, list and read  ← readiness — dependencies_resolved
- **CARD-007** 🔒 Booking validation: working hours, past slots, alignment, unknown resource  ← readiness — blocking_decisions_resolved, dependencies_resolved
- **CARD-008** 🔒 Availability search over a date range  ← readiness — blocking_decisions_resolved, dependencies_resolved
- **CARD-009** 🔒 Release criteria suite and supervisor deployment sample  ← readiness — blocking_decisions_resolved, dependencies_resolved
