# Changelog

## 2026-10-22
- CARD-003 (feature): Added the service bootstrap (environment configuration, store open with migrations, HTTP server with timeouts and graceful shutdown), minimal resource creation, and booking of one slot as a single autocommit insert where the slot uniqueness index picks exactly one winner among concurrent requests and the others get a slot_taken conflict.
- CARD-001 (feature): Added the OpenAPI contract for resources, availability and bookings with one stable set of error codes, and the generated net/http server interface mounted on http.ServeMux with JSON error responses for malformed requests.
- CARD-002 (feature): Added the SQLite store with WAL, busy timeout and foreign keys, and embedded schema migrations that create the resource and booking tables with the slot and idempotency-key uniqueness indexes on startup.
