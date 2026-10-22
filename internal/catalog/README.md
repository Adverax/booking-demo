# catalog

## Purpose

The Resource Catalog component (COMP-002): the Resource aggregate (AGG-001) with its weekly working hours (ADR-0004), and its creation (FR-001).

## Architecture

- `Catalog.Create` assigns an opaque id and stores the resource with its working hours through the `Repository` port, all or nothing.
- Working hours are one `Interval` per weekday, bounds in minutes since 00:00 UTC, half-open; an end of 1440 is the end of the UTC day.
- This is the minimal creation path of the walking skeleton. The working-hours rules of INV-005 are not validated here yet (CARD-006); until then the schema's constraints are the only backstop.

## Key entities

| Name | Kind | Description | File |
|------|------|-------------|------|
| `Resource` | type | Name and weekly working hours | [`catalog.go`](./catalog.go) |
| `Interval` | type | Working interval of one weekday, in minutes UTC | [`catalog.go`](./catalog.go) |
| `Catalog` | type | Creates resources (`Create`) | [`catalog.go`](./catalog.go) |
| `Repository` | interface | Port to the store | [`catalog.go`](./catalog.go) |

## Usage

```go
r, err := catalog.New(st).Create(ctx, "Dr. Lee", []catalog.Interval{{Weekday: time.Monday, Start: 540, End: 1020}})
```

## See also

- [`internal/store`](../store/README.md), the adapter of `Repository`
- ADR-0004 in `meta/architecture/decisions/adr/`
