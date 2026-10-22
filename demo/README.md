# Demo

A guided terminal walkthrough of the booking service. It builds the real binary, starts it on a
fresh SQLite file and drives the live HTTP API step by step, with a short comment before each
result. Nothing is mocked and nothing is left behind: the server and its temporary database are
removed on exit.

```
./demo/demo.sh          # interactive: press Enter between steps
./demo/demo.sh --auto   # no prompts, short pauses (for a recording)
./demo/demo.sh --fast   # no prompts, no pauses (quick check)
```

Needs `go`, `curl` and `python3`. Set `DEMO_PORT` (default `18080`) if the port is taken; the
script refuses to run against a port that something else already answers on. `NO_COLOR=1`
turns colors off.

## What it shows

| Step | Scenario | Rule of the brief |
|---|---|---|
| 1 | An operator publishes a resource with weekly working hours | Resources |
| 2 | A customer lists free 30-minute slots | Availability |
| 3 | A customer books a slot; the slot disappears from the list | Booking |
| 4 | The frontend retries with the same key and gets the same booking back; the same key with another customer is rejected | Idempotent booking |
| 5 | A second customer cannot take the same slot | No double booking |
| 6 | A slot outside working hours, in the past, off the half hour, and a range over 14 days are rejected with stable error codes | Bookable means available |
| 7 | Cancelling frees the slot at once; a repeated cancel is harmless; the cancelled booking is never reactivated | Cancellation is final |
| 8 | 50 parallel requests for one slot: exactly one wins, the other 49 get a conflict, none fails | The concurrency criterion |

## Talking points

- The same criteria run as tests: `go test -count=1 -run TestRelease ./cmd/booking/` (API end to
  end on a fresh database, the 50-request race, every rule has a named test).
- No double booking is not an `if` in the code: it is a unique index in the schema, so it holds
  under concurrency (ADR-0008).
- All times are UTC. The service has no authentication and no UI by design (see the brief).
