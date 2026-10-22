# catalog

## Purpose

The Resource Catalog component (COMP-002): the Resource aggregate (AGG-001) with its weekly working hours (ADR-0004), its validated creation (FR-001) and its list and read by id (FR-002). It is the single owner of the working-hours rules (ASM-0007).

## Architecture

- `NewResource` is the aggregate constructor and the one place INV-005 is enforced: a non-empty name, at least one interval, at most one interval per weekday, both bounds on the half hour, `0 <= Start < End <= 1440` (24:00 is allowed only as an end, so an interval never crosses midnight UTC). Every violation wraps `ErrInvalidResource`; accepted hours are copied and ordered by weekday.
- `Catalog.Create` validates through `NewResource` before anything reaches the store, then stores the resource with its working hours through the `Repository` port, all or nothing. `Catalog.List` and `Catalog.Get` read through the same port; `Get` of an unknown id wraps `ErrNotFound`.
- Working hours are one `Interval` per weekday, bounds in minutes since 00:00 UTC, half-open; an end of 1440 is the end of the UTC day.
- The schema's CHECK constraints stay a backstop only; the validation answers first with a stable error.

## Key entities

| Name | Kind | Description | File |
|------|------|-------------|------|
| `Resource` | type | Name and weekly working hours | [`catalog.go`](./catalog.go) |
| `NewResource` | func | Builds a valid Resource or fails with `ErrInvalidResource` (INV-005) | [`catalog.go`](./catalog.go) |
| `Interval` | type | Working interval of one weekday, in minutes UTC | [`catalog.go`](./catalog.go) |
| `Catalog` | type | Creates (`Create`), lists (`List`) and reads (`Get`) resources | [`catalog.go`](./catalog.go) |
| `ErrInvalidResource` / `ErrNotFound` | var | Rejected input; unknown resource id | [`catalog.go`](./catalog.go) |
| `Repository` | interface | Port to the store | [`catalog.go`](./catalog.go) |

## Usage

```go
r, err := catalog.New(st).Create(ctx, "Dr. Lee", []catalog.Interval{{Weekday: time.Monday, Start: 540, End: 1020}})
```

## See also

- [`internal/store`](../store/README.md), the adapter of `Repository`
- ADR-0004 in `meta/architecture/decisions/adr/`
