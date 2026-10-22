package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"

	"github.com/adverax/booking-demo/internal/httpapi"
)

// API tests of the resource catalog (FR-001, FR-002, INV-005, ADR-0004) over the real store; every
// response is validated against api/openapi.yaml by apiEnv.do.

// getResource reads one resource and validates the answer against the getResource operation of
// the contract. apiEnv.do looks operations up by literal path, which a templated path
// (/resources/{resourceId}) does not match, so the route is named here.
func (e *apiEnv) getResource(id string) *httptest.ResponseRecorder {
	e.t.Helper()
	const template = "/resources/{resourceId}"
	target := "/resources/" + id
	rec := serve(e.handler, http.MethodGet, target, "")
	item := e.spec.Paths.Value(template)
	if item == nil {
		e.t.Fatalf("no path %s in the spec", template)
	}
	err := openapi3filter.ValidateResponse(context.Background(), &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    httptest.NewRequest(http.MethodGet, target, nil),
			PathParams: map[string]string{"resourceId": id},
			Route:      &routers.Route{Spec: e.spec, Path: template, PathItem: item, Method: http.MethodGet, Operation: item.Get},
		},
		Status:  rec.Code,
		Header:  rec.Header(),
		Body:    io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
		Options: &openapi3filter.Options{IncludeResponseStatus: true},
	})
	if err != nil {
		e.t.Fatalf("GET %s: response %d %s does not conform to the contract: %v", target, rec.Code, rec.Body, err)
	}
	return rec
}

func monday(start, end string) httpapi.WeeklyHours {
	return weekdayHours([]httpapi.Weekday{httpapi.WeekdayMonday}, start, end)
}

func (e *apiEnv) listResources() []httpapi.Resource {
	e.t.Helper()
	rec := e.do(http.MethodGet, "/resources", nil)
	if rec.Code != http.StatusOK {
		e.t.Fatalf("list resources: %d %s", rec.Code, rec.Body)
	}
	var l httpapi.ResourceList
	if err := json.Unmarshal(rec.Body.Bytes(), &l); err != nil {
		e.t.Fatal(err)
	}
	return l.Items
}

// assertRejectedAndNothingStored creates a resource that violates INV-005 and checks the 400
// validation_error answer, that the list stays empty and that no row reached either table.
func assertRejectedAndNothingStored(t *testing.T, name string, hours httpapi.WeeklyHours) {
	t.Helper()
	e := newAPIEnv(t)
	rec := e.do(http.MethodPost, "/resources", httpapi.CreateResourceRequest{Name: name, WeeklyHours: hours})
	assertError(t, rec, http.StatusBadRequest, httpapi.ErrorCodeValidationError)
	if items := e.listResources(); len(items) != 0 {
		t.Fatalf("listed after a rejected create: %+v", items)
	}
	if n := e.count("SELECT (SELECT count(*) FROM resource) + (SELECT count(*) FROM resource_working_hours)"); n != 0 {
		t.Fatalf("%d rows stored after a rejected create", n)
	}
}

// AC-002: "Dr. Lee" without working hours is rejected; nothing is persisted.
func TestCreateResource_RejectsMissingWorkingHours(t *testing.T) {
	t.Run("empty list", func(t *testing.T) { assertRejectedAndNothingStored(t, "Dr. Lee", httpapi.WeeklyHours{}) })
	t.Run("absent field", func(t *testing.T) {
		e := newAPIEnv(t)
		assertError(t, serve(e.handler, http.MethodPost, "/resources", `{"name":"Dr. Lee"}`), http.StatusBadRequest, httpapi.ErrorCodeValidationError)
		if items := e.listResources(); len(items) != 0 {
			t.Fatalf("listed after a rejected create: %+v", items)
		}
	})
}

// AC-003: an empty name with Mon-Fri 09:00-17:00 is rejected; nothing is persisted.
func TestCreateResource_RejectsEmptyName(t *testing.T) {
	assertRejectedAndNothingStored(t, "", weekdayHours(monToFri, "09:00", "17:00"))
}

// AC-034: two Monday intervals 09:00-12:00 and 13:00-17:00 are rejected; nothing is persisted.
func TestCreateResource_RejectsTwoIntervalsOnOneWeekday(t *testing.T) {
	assertRejectedAndNothingStored(t, "Dr. Lee", append(monday("09:00", "12:00"), monday("13:00", "17:00")...))
}

// AC-035: a Monday interval 09:15-17:00 is rejected; nothing is persisted.
func TestCreateResource_RejectsMisalignedIntervalBound(t *testing.T) {
	assertRejectedAndNothingStored(t, "Dr. Lee", monday("09:15", "17:00"))
}

// AC-036: a Monday interval 22:00-02:00, crossing midnight UTC, is rejected; nothing is persisted.
func TestCreateResource_RejectsIntervalCrossingMidnightUtc(t *testing.T) {
	assertRejectedAndNothingStored(t, "Dr. Lee", monday("22:00", "02:00"))
}

// AC-059: a Monday interval 22:00-24:00 is persisted and returned with a generated id.
func TestCreateResource_AcceptsIntervalEndingAtTwentyFourHundred(t *testing.T) {
	e := newAPIEnv(t)
	hours := monday("22:00", "24:00")
	r := e.createResource("Dr. Lee", hours)
	if r.Id == "" || r.Name != "Dr. Lee" || !slices.Equal(r.WeeklyHours, hours) {
		t.Fatalf("returned %+v", r)
	}
	if n := e.count("SELECT count(*) FROM resource_working_hours WHERE resource_id = ? AND weekday = 1 AND start_minute = 1320 AND end_minute = 1440", r.Id); n != 1 {
		t.Fatalf("stored rows Monday 22:00-24:00: %d, want 1", n)
	}
	rec := e.getResource(r.Id)
	var got httpapi.Resource
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || !slices.Equal(got.WeeklyHours, hours) {
		t.Fatalf("read back: %d %s", rec.Code, rec.Body)
	}
}

// AC-060: a Monday interval 24:00-24:30 is rejected; nothing is persisted.
func TestCreateResource_RejectsIntervalStartingAtTwentyFourHundred(t *testing.T) {
	assertRejectedAndNothingStored(t, "Dr. Lee", monday("24:00", "24:30"))
}

// AC-061: a Monday interval 23:00-24:30 is rejected; nothing is persisted.
func TestCreateResource_RejectsIntervalBoundAfterTwentyFourHundred(t *testing.T) {
	assertRejectedAndNothingStored(t, "Dr. Lee", monday("23:00", "24:30"))
}

// Further shapes INV-005 forbids that the schema alone would also refuse, or would not: each is
// answered by the catalog with validation_error and nothing is stored.
func TestCreateResource_RejectsOtherInvalidShapes(t *testing.T) {
	for name, hours := range map[string]httpapi.WeeklyHours{
		"empty interval":         monday("09:00", "09:00"),
		"misaligned end":         monday("09:00", "17:45"),
		"24:00 to 24:00":         monday("24:00", "24:00"),
		"one bad among good":     append(weekdayHours(monToFri[1:], "09:00", "17:00"), monday("09:00", "24:59")...),
		"repeat after other day": append(append(monday("09:00", "10:00"), weekdayHours(monToFri[4:], "09:00", "10:00")...), monday("11:00", "12:00")...),
	} {
		t.Run(name, func(t *testing.T) { assertRejectedAndNothingStored(t, "Dr. Lee", hours) })
	}
}

// AC-004: with "Dr. Lee" and "Room A" persisted, the list holds exactly those two.
func TestListResources_ReturnsAll(t *testing.T) {
	e := newAPIEnv(t)
	if items := e.listResources(); len(items) != 0 {
		t.Fatalf("empty catalog lists %+v", items)
	}
	lee := e.createResource("Dr. Lee", weekdayHours(monToFri, "09:00", "17:00"))
	room := e.createResource("Room A", monday("08:00", "24:00"))

	items := e.listResources()
	if len(items) != 2 {
		t.Fatalf("listed %d resources, want 2: %+v", len(items), items)
	}
	for _, want := range []httpapi.Resource{lee, room} {
		i := slices.IndexFunc(items, func(r httpapi.Resource) bool { return r.Id == want.Id })
		if i < 0 {
			t.Fatalf("%s missing from %+v", want.Name, items)
		}
		if items[i].Name != want.Name || !slices.Equal(items[i].WeeklyHours, want.WeeklyHours) {
			t.Errorf("listed %+v, want %+v", items[i], want)
		}
	}
}

// AC-005: reading "Room A" by its id returns its name and weekly working hours.
func TestGetResource_ReturnsResource(t *testing.T) {
	e := newAPIEnv(t)
	e.createResource("Dr. Lee", monday("09:00", "17:00"))
	hours := weekdayHours(monToFri, "10:00", "18:30")
	room := e.createResource("Room A", hours)

	rec := e.getResource(room.Id)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body)
	}
	var got httpapi.Resource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Id != room.Id || got.Name != "Room A" || !slices.Equal(got.WeeklyHours, hours) {
		t.Fatalf("got %+v, want Room A with %+v", got, hours)
	}
}

// AC-006: reading the unknown id "missing-id" is 404 not_found.
func TestGetResource_UnknownIdNotFound(t *testing.T) {
	e := newAPIEnv(t)
	e.createResource("Room A", monday("09:00", "17:00"))
	assertError(t, e.getResource("missing-id"), http.StatusNotFound, httpapi.ErrorCodeNotFound)
}
