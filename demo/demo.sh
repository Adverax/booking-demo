#!/usr/bin/env bash
# Guided terminal demo of the booking service.
#
# It builds the real binary, starts it on a fresh SQLite file and walks through the
# behaviour promised by the design brief, step by step, against the live HTTP API.
#
#   ./demo/demo.sh            interactive: press Enter between steps
#   ./demo/demo.sh --auto     no prompts, short pauses (for recordings)
#   ./demo/demo.sh --fast     no prompts, no pauses (for a quick check)
#
# Needs: go, curl, python3. Environment: DEMO_PORT (default 18080), NO_COLOR.

set -u -o pipefail

MODE=interactive
case "${1:-}" in
  --auto) MODE=auto ;;
  --fast) MODE=fast ;;
  "") ;;
  *) echo "usage: $0 [--auto|--fast]" >&2; exit 2 ;;
esac

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${DEMO_PORT:-18080}"
BASE="http://127.0.0.1:${PORT}"
WORK="$(mktemp -d)"
SERVER_PID=""

if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  B=$'\033[1m'; D=$'\033[2m'; G=$'\033[32m'; Y=$'\033[33m'; R=$'\033[31m'; C=$'\033[36m'; N=$'\033[0m'
else
  B=""; D=""; G=""; Y=""; R=""; C=""; N=""
fi

cleanup() {
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null
  rm -rf "$WORK"
}
trap cleanup EXIT INT TERM

pause() {
  case "$MODE" in
    interactive) printf '%s' "${D}  [Enter to continue]${N}"; read -r _ ; echo ;;
    auto) sleep 2 ;;
    fast) : ;;
  esac
}

step() { # step <number> <title>
  echo
  echo "${B}${C}== $1. $2${N}"
}

say() { echo "${D}   $*${N}"; }

pretty() {
  # Display only: long lists are folded so the output fits on a projector.
  python3 -c '
import sys, json
raw = sys.stdin.read()
try:
    obj = json.loads(raw)
except Exception:
    print(raw)
    sys.exit(0)
if isinstance(obj, dict):
    if isinstance(obj.get("slots"), list):
        obj["slots"] = "%d slots: %s" % (len(obj["slots"]), " ".join(x["start"][11:16] for x in obj["slots"]))
    if isinstance(obj.get("weeklyHours"), list):
        obj["weeklyHours"] = ", ".join("%s %s-%s" % (h["weekday"][:3], h["start"], h["end"]) for h in obj["weeklyHours"])
print(json.dumps(obj, indent=2, sort_keys=True))
' | sed 's/^/   /'
}

# call <label> <expected-status> <method> <path> [json-body]
# Prints the request, the HTTP status (green when it is the expected one) and the body.
LAST_BODY=""
call() {
  local label="$1" want="$2" method="$3" path="$4" body="${5:-}"
  local out status
  echo "${Y}   > ${label}${N}"
  if [ -n "$body" ]; then
    echo "${D}     ${method} ${path}  $(printf '%s' "$body" | tr -s ' \n' ' ')${N}"
    out="$(curl -s -w $'\n%{http_code}' -X "$method" "${BASE}${path}" -H 'Content-Type: application/json' -d "$body")"
  else
    echo "${D}     ${method} ${path}${N}"
    out="$(curl -s -w $'\n%{http_code}' -X "$method" "${BASE}${path}")"
  fi
  status="${out##*$'\n'}"
  LAST_BODY="${out%$'\n'*}"
  if [ "$status" = "$want" ]; then
    echo "   ${G}HTTP ${status}${N}"
  else
    echo "   ${R}HTTP ${status} (expected ${want})${N}"
  fi
  printf '%s' "$LAST_BODY" | pretty
}

field() { printf '%s' "$LAST_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin)$1)"; }

# --- start the service ---------------------------------------------------------------------
echo "${B}Booking service demo${N}"
say "Real binary, fresh SQLite file, real HTTP. Nothing is mocked."

for tool in go curl python3; do
  command -v "$tool" >/dev/null 2>&1 || { echo "${R}missing tool: ${tool}${N}" >&2; exit 1; }
done

echo "   building the service..."
( cd "$ROOT" && go build -o "$WORK/booking" ./cmd/booking ) || { echo "${R}build failed${N}" >&2; exit 1; }

# Refuse to run against somebody else's server: the port must be free before we start.
if curl -s -o /dev/null --max-time 1 "${BASE}/resources"; then
  echo "${R}port ${PORT} is already in use by another process. Set DEMO_PORT to a free port.${N}" >&2
  exit 1
fi

BOOKING_DB_PATH="$WORK/demo.db" BOOKING_ADDR="127.0.0.1:${PORT}" "$WORK/booking" >"$WORK/server.log" 2>&1 &
SERVER_PID=$!
READY=""
for _ in $(seq 1 50); do
  kill -0 "$SERVER_PID" 2>/dev/null || { echo "${R}server did not start:${N}"; cat "$WORK/server.log"; exit 1; }
  curl -s "${BASE}/resources" 2>/dev/null | grep -q '"items"' && { READY=yes; break; }
  sleep 0.1
done
[ -n "$READY" ] || { echo "${R}server did not become ready${N}"; cat "$WORK/server.log"; exit 1; }
echo "   ${G}running on ${BASE}${N}  (database: a new file, schema created by the embedded migrations)"

# The next Monday, so that the demo always books in the future.
MONDAY="$(python3 -c '
import datetime as d
t = d.datetime.now(d.timezone.utc).date()
print(t + d.timedelta(days=(0 - t.weekday()) % 7 or 7))
')"
say "Demo day: ${MONDAY} (next Monday, UTC). All times are UTC, as the brief requires."
pause

# --- 1. publish a resource -----------------------------------------------------------------
step 1 "An operator publishes a resource with weekly working hours"
call "Create 'Dr. Lee', Monday to Friday 09:00-17:00" 201 POST /resources \
  '{"name":"Dr. Lee","weeklyHours":[
     {"weekday":"monday","start":"09:00","end":"17:00"},
     {"weekday":"tuesday","start":"09:00","end":"17:00"},
     {"weekday":"wednesday","start":"09:00","end":"17:00"},
     {"weekday":"thursday","start":"09:00","end":"17:00"},
     {"weekday":"friday","start":"09:00","end":"17:00"}]}'
RID="$(field '["id"]')"
pause

# --- 2. availability -----------------------------------------------------------------------
step 2 "A customer looks for free 30-minute slots"
Q="from=${MONDAY}&to=${MONDAY}"
call "Free slots on ${MONDAY}" 200 GET "/resources/${RID}/availability?${Q}"
FREE_BEFORE="$(field '["slots"].__len__()')"
say "${FREE_BEFORE} free slots: 09:00, 09:30 ... 16:30. Slots are aligned to the half hour."
pause

# --- 3. book -------------------------------------------------------------------------------
step 3 "The customer books 09:30"
BOOK_BODY="{\"resourceId\":\"${RID}\",\"slotStart\":\"${MONDAY}T09:30:00Z\",\"customerName\":\"Ann\",\"idempotencyKey\":\"demo-key-1\"}"
call "Book 09:30 for Ann (idempotency key demo-key-1)" 201 POST /bookings "$BOOK_BODY"
BID="$(field '["id"]')"
call "Free slots again" 200 GET "/resources/${RID}/availability?${Q}"
say "$(field '["slots"].__len__()') free slots now: the booked slot is gone."
pause

# --- 4. idempotency ------------------------------------------------------------------------
step 4 "The response got lost, so the frontend retries with the same key"
call "Repeat the exact same request" 200 POST /bookings "$BOOK_BODY"
say "Same booking id (${BID}), nothing new was created: HTTP 200 instead of 201."
call "Same key, but a different customer name" 409 POST /bookings \
  "{\"resourceId\":\"${RID}\",\"slotStart\":\"${MONDAY}T09:30:00Z\",\"customerName\":\"Bob\",\"idempotencyKey\":\"demo-key-1\"}"
say "A reused key with another payload is rejected with a stable error code."
pause

# --- 5. no double booking ------------------------------------------------------------------
step 5 "Someone else tries to take the same slot"
call "Bob, new key, same 09:30 slot" 409 POST /bookings \
  "{\"resourceId\":\"${RID}\",\"slotStart\":\"${MONDAY}T09:30:00Z\",\"customerName\":\"Bob\",\"idempotencyKey\":\"demo-key-2\"}"
say "No double booking. The rule is enforced by a unique index in the database, not by code hopes."
pause

# --- 6. rules ------------------------------------------------------------------------------
step 6 "The rules of the brief"
call "Outside working hours (18:00)" 422 POST /bookings \
  "{\"resourceId\":\"${RID}\",\"slotStart\":\"${MONDAY}T18:00:00Z\",\"customerName\":\"Ann\",\"idempotencyKey\":\"demo-key-3\"}"
call "In the past (2020)" 422 POST /bookings \
  "{\"resourceId\":\"${RID}\",\"slotStart\":\"2020-01-06T09:00:00Z\",\"customerName\":\"Ann\",\"idempotencyKey\":\"demo-key-4\"}"
call "Not on the half hour (09:15)" 400 POST /bookings \
  "{\"resourceId\":\"${RID}\",\"slotStart\":\"${MONDAY}T09:15:00Z\",\"customerName\":\"Ann\",\"idempotencyKey\":\"demo-key-5\"}"
call "Availability range longer than 14 days" 400 GET \
  "/resources/${RID}/availability?from=${MONDAY}&to=2099-01-01"
pause

# --- 7. cancellation -----------------------------------------------------------------------
step 7 "Cancellation frees the slot, and it is final"
call "Cancel the booking" 200 POST "/bookings/${BID}/cancel"
call "Cancel it again (a retry is harmless)" 200 POST "/bookings/${BID}/cancel"
call "Free slots after the cancellation" 200 GET "/resources/${RID}/availability?${Q}"
say "$(field '["slots"].__len__()') free slots: 09:30 is free again, immediately."
call "Retry the old key after the cancel" 200 POST /bookings "$BOOK_BODY"
say "The old key returns the cancelled booking. A cancelled booking is never reactivated: a new booking needs a new key."
pause

# --- 8. the race ---------------------------------------------------------------------------
step 8 "The hard one: 50 customers hit the same slot at the same moment"
say "50 parallel HTTP requests, 50 different keys, one slot (10:00). Exactly one may win."
SLOT="${MONDAY}T10:00:00Z"
mkdir -p "$WORK/race"
RACE_PIDS=""
for i in $(seq 1 50); do
  (
    curl -s -o /dev/null -w '%{http_code}\n' -X POST "${BASE}/bookings" \
      -H 'Content-Type: application/json' \
      -d "{\"resourceId\":\"${RID}\",\"slotStart\":\"${SLOT}\",\"customerName\":\"Customer-${i}\",\"idempotencyKey\":\"race-${i}\"}" \
      >"$WORK/race/$i"
  ) &
  RACE_PIDS="$RACE_PIDS $!"
done
# Wait only for the 50 requests; a bare `wait` would also wait for the server itself.
for pid in $RACE_PIDS; do wait "$pid"; done
echo "   result of the race:"
cat "$WORK"/race/* | sort | uniq -c | sed 's/^ *\([0-9]*\) \(.*\)/     \1 x HTTP \2/'
WON="$(cat "$WORK"/race/* | grep -c '^201$')"
if [ "$WON" = "1" ]; then
  echo "   ${G}exactly one booking succeeded${N}; the others were rejected as conflicts, none failed with a server error."
else
  echo "   ${R}expected exactly one HTTP 201, got ${WON}${N}"
fi
pause

# --- wrap up -------------------------------------------------------------------------------
step 9 "Behind the demo"
say "This is the release criteria of the brief, running live. The same ones run as tests:"
echo "     go test -count=1 -run TestRelease ./cmd/booking/    ${D}# API end to end, the 50-request race, every rule has a named test${N}"
echo "     make test                                             ${D}# the full suite${N}"
echo
echo "${B}Done.${N} The server and its temporary database are removed on exit."
