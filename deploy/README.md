# deploy

## Purpose

A sample of running the booking binary under an OS supervisor (ADR-0005): a systemd unit and the environment file it reads. A sample, not an installer.

## Files

- `booking.service`: systemd unit. Runs `/usr/local/bin/booking` as one process, reads `/etc/booking/booking.env`, keeps the database under `/var/lib/booking` (`StateDirectory`), restarts on failure and gives the 10 s drain room on stop.
- `booking.env.example`: the two settings the binary reads, `BOOKING_DB_PATH` (required) and `BOOKING_ADDR` (default `:8080`). See [`cmd/booking`](../cmd/booking/README.md).

## Constraints

- One instance on a local disk: SQLite allows one writer process. The unit runs one process; nothing else enforces it (DEC-029 open).
- No logging, metrics or uptime settings: none is decided (DEC-007, DEC-010 open).

## Usage

The install steps are in the header of `booking.service`. Check the unit with `systemd-analyze verify booking.service`.
