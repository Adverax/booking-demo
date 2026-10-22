# booking (command)

## Purpose

The Service Bootstrap (COMP-007): the single binary of the booking service. It reads its configuration from environment variables (CON-005, ADR-0005), opens and migrates the SQLite store (ADR-0007), wires the components and serves the generated HTTP API on `http.ServeMux` (ADR-0009).

## Architecture

- Configuration:

  | Variable | Required | Default | Meaning |
  |----------|----------|---------|---------|
  | `BOOKING_DB_PATH` | yes | — | Path of the SQLite database file; created and migrated if missing |
  | `BOOKING_ADDR` | no | `:8080` | TCP address to listen on |

- Server bounds: read header 5 s, read 10 s, write 30 s, idle 60 s.
- SIGINT or SIGTERM stops accepting and drains in-flight requests for up to 10 s; then remaining connections are closed and the store is closed. Exit status 0 after a clean drain, 1 on a start failure or a drain that timed out, with one line on stderr.
- The service must run as one instance, because SQLite allows one writer process (ADR-0005). Nothing enforces it (DEC-029).
- No logging, metrics or tracing beyond the start-failure line (DEC-007, DEC-010 open).

## Usage

```sh
BOOKING_DB_PATH=/var/lib/booking/booking.db BOOKING_ADDR=127.0.0.1:8080 booking
```

## Tests

- `main_test.go` builds the real binary once (`TestMain`) and runs it on a fresh SQLite file (`startService`).
- `release_test.go` holds the release criteria (`go test -run TestRelease ./...`): 50 concurrent bookings for one slot yield exactly one success (AC-031), the full API scenario on a fresh database (AC-032), and every test name planned in `meta/architecture/requirements.yml` exists as a Go test function (AC-033, ADR-0022).

## See also

- [`deploy`](../../deploy/README.md): sample systemd unit and environment file

- [`internal/httpapi`](../../internal/httpapi/README.md), [`internal/store`](../../internal/store/README.md)
- ADR-0005, ADR-0007, ADR-0009 in `meta/architecture/decisions/adr/`
