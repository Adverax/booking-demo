package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"

	"github.com/adverax/booking-demo/internal/booking"
	"github.com/adverax/booking-demo/internal/catalog"
	"github.com/adverax/booking-demo/internal/httpapi"
	"github.com/adverax/booking-demo/internal/store"
)

// API tests: the real handler over the real store on a fresh SQLite file, with a fixed clock.
// Every response is also validated against api/openapi.yaml (contract test).

// fixedNow is the injected clock of these tests; every slot below is after it (ADR-0013).
var fixedNow = time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)

// monday9 is Monday 2026-11-02 09:00 UTC, a future slot inside Mon-Fri 09:00-17:00.
var monday9 = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)

type apiEnv struct {
	t       *testing.T
	handler http.Handler
	db      *sql.DB
	spec    *openapi3.T
}

func newAPIEnv(t *testing.T) *apiEnv {
	t.Helper()
	path := filepath.Join(t.TempDir(), "booking.db")
	st, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	srv := httpapi.NewServer(catalog.New(st), booking.NewService(st, func() time.Time { return fixedNow }))
	return &apiEnv{t: t, handler: httpapi.NewHandler(srv), db: db, spec: loadSpec(t)}
}

// do sends one request and fails the test when the response does not conform to the contract.
func (e *apiEnv) do(method, target string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		e.t.Fatal(err)
	}
	rec := serve(e.handler, method, target, string(raw))
	e.validate(method, target, rec)
	return rec
}

func (e *apiEnv) validate(method, target string, rec *httptest.ResponseRecorder) {
	e.t.Helper()
	if err := conforms(e.spec, method, target, rec); err != nil {
		e.t.Fatalf("%s %s: response %d %s does not conform to the contract: %v", method, target, rec.Code, rec.Body, err)
	}
}

// conforms validates a recorded response against the operation of the spec it answers.
func conforms(spec *openapi3.T, method, target string, rec *httptest.ResponseRecorder) error {
	item := spec.Paths.Find(target)
	if item == nil {
		return fmt.Errorf("no path %s in the spec", target)
	}
	return openapi3filter.ValidateResponse(context.Background(), &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request: httptest.NewRequest(method, target, nil),
			Route:   &routers.Route{Spec: spec, Path: target, PathItem: item, Method: method, Operation: item.GetOperation(method)},
		},
		Status:  rec.Code,
		Header:  rec.Header(),
		Body:    io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
		Options: &openapi3filter.Options{IncludeResponseStatus: true},
	})
}

// The contract check of these tests is not vacuous: a response the spec does not declare fails.
func TestContract_ResponseCheckRejectsUndeclaredShapes(t *testing.T) {
	spec := loadSpec(t)
	for name, c := range map[string]struct {
		status int
		body   string
	}{
		"201 without required fields": {http.StatusCreated, `{"id":"b1"}`},
		"409 with an unknown code":    {http.StatusConflict, `{"code":"busy","message":"x"}`},
		"undeclared status":           {http.StatusTeapot, `{"code":"internal_error","message":"x"}`},
	} {
		rec := httptest.NewRecorder()
		rec.Header().Set("Content-Type", "application/json")
		rec.WriteHeader(c.status)
		rec.WriteString(c.body)
		if conforms(spec, http.MethodPost, "/bookings", rec) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func weekdayHours(days []httpapi.Weekday, start, end string) httpapi.WeeklyHours {
	var h httpapi.WeeklyHours
	for _, d := range days {
		h = append(h, httpapi.WorkingInterval{Weekday: d, Start: start, End: end})
	}
	return h
}

var monToFri = []httpapi.Weekday{httpapi.WeekdayMonday, httpapi.WeekdayTuesday, httpapi.WeekdayWednesday, httpapi.WeekdayThursday, httpapi.WeekdayFriday}

func (e *apiEnv) createResource(name string, hours httpapi.WeeklyHours) httpapi.Resource {
	e.t.Helper()
	rec := e.do(http.MethodPost, "/resources", httpapi.CreateResourceRequest{Name: name, WeeklyHours: hours})
	if rec.Code != http.StatusCreated {
		e.t.Fatalf("create resource: %d %s", rec.Code, rec.Body)
	}
	var r httpapi.Resource
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		e.t.Fatal(err)
	}
	return r
}

func (e *apiEnv) book(resourceID string, start time.Time, name, key string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(http.MethodPost, "/bookings", httpapi.BookSlotRequest{
		ResourceId: resourceID, SlotStart: start, CustomerName: name, IdempotencyKey: key,
	})
}

func (e *apiEnv) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.db.QueryRow(query, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *apiEnv) activeBookings(resourceID string, start time.Time) int {
	e.t.Helper()
	return e.count("SELECT count(*) FROM booking WHERE resource_id = ? AND slot_start = ? AND status = 'active'", resourceID, start.Unix())
}

// AC-001: a resource "Dr. Lee", Mon-Fri 09:00-17:00 UTC, is persisted and returned with an id.
func TestCreateResource_Persists(t *testing.T) {
	e := newAPIEnv(t)
	hours := weekdayHours(monToFri, "09:00", "17:00")
	r := e.createResource("Dr. Lee", hours)

	if r.Id == "" || r.Name != "Dr. Lee" || len(r.WeeklyHours) != 5 {
		t.Fatalf("returned resource %+v", r)
	}
	for i, wi := range r.WeeklyHours {
		if wi != hours[i] {
			t.Errorf("returned interval %d = %+v, want %+v", i, wi, hours[i])
		}
	}
	var name string
	if err := e.db.QueryRow("SELECT name FROM resource WHERE id = ?", r.Id).Scan(&name); err != nil || name != "Dr. Lee" {
		t.Fatalf("stored resource: name %q, err %v", name, err)
	}
	rows := e.count(`SELECT count(*) FROM resource_working_hours
		WHERE resource_id = ? AND weekday BETWEEN 1 AND 5 AND start_minute = 540 AND end_minute = 1020`, r.Id)
	if rows != 5 {
		t.Fatalf("stored working hours Mon-Fri 09:00-17:00: %d rows, want 5", rows)
	}
}

// An end of 24:00 crosses the boundary as "24:00" both ways (stored as 1440, ADR-0004).
func TestCreateResource_EndOfDayRoundTrips(t *testing.T) {
	e := newAPIEnv(t)
	hours := weekdayHours([]httpapi.Weekday{httpapi.WeekdaySunday}, "22:00", "24:00")
	r := e.createResource("Night desk", hours)
	if r.WeeklyHours[0] != hours[0] {
		t.Fatalf("returned %+v, want %+v", r.WeeklyHours[0], hours[0])
	}
	if n := e.count("SELECT count(*) FROM resource_working_hours WHERE resource_id = ? AND weekday = 0 AND end_minute = 1440", r.Id); n != 1 {
		t.Fatalf("stored rows with end 1440: %d", n)
	}
}

// A bound that cannot be read as HH:MM, or an unknown weekday, is a validation error and nothing
// is stored.
func TestCreateResource_UnreadableIntervalIsValidationError(t *testing.T) {
	cases := map[string]httpapi.WorkingInterval{
		"letters":         {Weekday: httpapi.WeekdayMonday, Start: "ab:cd", End: "17:00"},
		"no colon":        {Weekday: httpapi.WeekdayMonday, Start: "0900", End: "17:00"},
		"sign":            {Weekday: httpapi.WeekdayMonday, Start: "+9:00", End: "17:00"},
		"hour 25":         {Weekday: httpapi.WeekdayMonday, Start: "09:00", End: "25:00"},
		"minute 60":       {Weekday: httpapi.WeekdayMonday, Start: "09:60", End: "17:00"},
		"unknown day":     {Weekday: "funday", Start: "09:00", End: "17:00"},
		"capitalised day": {Weekday: "Monday", Start: "09:00", End: "17:00"},
	}
	for name, wi := range cases {
		t.Run(name, func(t *testing.T) {
			e := newAPIEnv(t)
			rec := e.do(http.MethodPost, "/resources", httpapi.CreateResourceRequest{Name: "Dr. Lee", WeeklyHours: httpapi.WeeklyHours{wi}})
			assertError(t, rec, http.StatusBadRequest, httpapi.ErrorCodeValidationError)
			if n := e.count("SELECT count(*) FROM resource"); n != 0 {
				t.Fatalf("%d resources stored", n)
			}
		})
	}
}

// AC-014: "Ann" books a free future Monday 09:00 slot with a new key; the booking is persisted.
func TestBookSlot_PersistsBooking(t *testing.T) {
	e := newAPIEnv(t)
	r := e.createResource("Dr. Lee", weekdayHours(monToFri, "09:00", "17:00"))

	rec := e.book(r.Id, monday9, "Ann", "key-ann")
	if rec.Code != http.StatusCreated {
		t.Fatalf("book: %d %s", rec.Code, rec.Body)
	}
	var b httpapi.Booking
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if b.Id == "" || b.ResourceId != r.Id || !b.SlotStart.Equal(monday9) || !b.SlotEnd.Equal(monday9.Add(30*time.Minute)) ||
		b.CustomerName != "Ann" || b.Status != httpapi.BookingStatusActive {
		t.Fatalf("returned booking %+v", b)
	}
	if !strings.Contains(rec.Body.String(), `"slotStart":"2026-11-02T09:00:00Z"`) {
		t.Errorf("slotStart is not RFC 3339 UTC with Z: %s", rec.Body)
	}
	var (
		resourceID, name, key, status string
		start                         int64
	)
	if err := e.db.QueryRow("SELECT resource_id, slot_start, customer_name, idempotency_key, status FROM booking WHERE id = ?", b.Id).
		Scan(&resourceID, &start, &name, &key, &status); err != nil {
		t.Fatalf("stored booking: %v", err)
	}
	if resourceID != r.Id || start != monday9.Unix() || name != "Ann" || key != "key-ann" || status != "active" {
		t.Fatalf("stored booking: %s %d %q %q %s", resourceID, start, name, key, status)
	}
}

// AC-018: a second customer with another key gets a conflict with a stable code; still exactly one
// active booking for the slot.
func TestBookSlot_RejectsAlreadyBookedSlot(t *testing.T) {
	e := newAPIEnv(t)
	r := e.createResource("Dr. Lee", weekdayHours(monToFri, "09:00", "17:00"))
	if rec := e.book(r.Id, monday9, "Ann", "key-ann"); rec.Code != http.StatusCreated {
		t.Fatalf("first booking: %d %s", rec.Code, rec.Body)
	}

	rec := e.book(r.Id, monday9, "Bob", "key-bob")
	assertError(t, rec, http.StatusConflict, httpapi.ErrorCodeSlotTaken)
	if n := e.activeBookings(r.Id, monday9); n != 1 {
		t.Fatalf("active bookings for the slot = %d, want 1", n)
	}
	if n := e.count("SELECT count(*) FROM booking WHERE idempotency_key = 'key-bob'"); n != 0 {
		t.Fatalf("%d rows stored under the losing key", n)
	}
}

// AC-037: the same start on another resource is another slot.
func TestBookSlot_SameStartOnAnotherResourceIsNotTheSameSlot(t *testing.T) {
	e := newAPIEnv(t)
	monday := []httpapi.Weekday{httpapi.WeekdayMonday}
	r1 := e.createResource("R1", weekdayHours(monday, "09:00", "10:00"))
	r2 := e.createResource("R2", weekdayHours(monday, "09:00", "10:00"))
	if rec := e.book(r1.Id, monday9, "Ann", "key-ann"); rec.Code != http.StatusCreated {
		t.Fatalf("booking of R1: %d %s", rec.Code, rec.Body)
	}

	rec := e.book(r2.Id, monday9, "Bob", "key-bob")
	if rec.Code != http.StatusCreated {
		t.Fatalf("booking of R2: %d %s", rec.Code, rec.Body)
	}
	if e.activeBookings(r1.Id, monday9) != 1 || e.activeBookings(r2.Id, monday9) != 1 {
		t.Fatal("want one active booking per resource at Monday 09:00")
	}
}

// A slot start with another offset and inside the half hour is stored and returned as the
// canonical UTC half-hour start (ADR-0013, ADR-0015), and competes with that slot.
func TestBookSlot_NormalisesSlotStartToCanonicalUTC(t *testing.T) {
	e := newAPIEnv(t)
	r := e.createResource("Dr. Lee", weekdayHours(monToFri, "09:00", "17:00"))
	plus2 := time.FixedZone("+02:00", 2*3600)
	rec := e.book(r.Id, time.Date(2026, 11, 2, 11, 17, 42, 0, plus2), "Ann", "key-ann") // 09:17:42Z
	if rec.Code != http.StatusCreated {
		t.Fatalf("book: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"slotStart":"2026-11-02T09:00:00Z"`) {
		t.Fatalf("slotStart not canonical UTC: %s", rec.Body)
	}
	assertError(t, e.book(r.Id, monday9, "Bob", "key-bob"), http.StatusConflict, httpapi.ErrorCodeSlotTaken)
}

// Operations whose cards have not landed answer the contract's 500 internal_error.
func TestServer_UnimplementedOperationsAreInternalError(t *testing.T) {
	e := newAPIEnv(t)
	for _, c := range []struct{ method, target string }{
		{http.MethodGet, "/resources/r1/availability?from=2026-11-02&to=2026-11-03"},
		{http.MethodGet, "/bookings/b1"},
		{http.MethodPost, "/bookings/b1/cancel"},
	} {
		rec := serve(e.handler, c.method, c.target, "")
		assertError(t, rec, http.StatusInternalServerError, httpapi.ErrorCodeInternalError)
	}
}
