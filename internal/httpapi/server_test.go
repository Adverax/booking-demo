package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/adverax/booking-demo/internal/httpapi"
)

// stubServer stands in for the handlers of later cards. Each operation records its name and
// answers with a success the contract declares, unless fail or panicValue says otherwise.
type stubServer struct {
	called     string
	fail       error
	panicValue any
}

func (s *stubServer) enter(op string) error {
	s.called = op
	if s.panicValue != nil {
		panic(s.panicValue)
	}
	return s.fail
}

func (s *stubServer) BookSlot(_ context.Context, req httpapi.BookSlotRequestObject) (httpapi.BookSlotResponseObject, error) {
	if err := s.enter("bookSlot"); err != nil {
		return nil, err
	}
	if req.Body.IdempotencyKey == "taken" {
		return httpapi.BookSlot409JSONResponse{Code: httpapi.ErrorCodeSlotTaken, Message: "slot taken"}, nil
	}
	return httpapi.BookSlot201JSONResponse{Id: "b1", ResourceId: req.Body.ResourceId, Status: httpapi.BookingStatusActive}, nil
}

func (s *stubServer) GetBooking(_ context.Context, req httpapi.GetBookingRequestObject) (httpapi.GetBookingResponseObject, error) {
	if err := s.enter("getBooking"); err != nil {
		return nil, err
	}
	return httpapi.GetBooking200JSONResponse{Id: req.BookingId}, nil
}

func (s *stubServer) CancelBooking(_ context.Context, req httpapi.CancelBookingRequestObject) (httpapi.CancelBookingResponseObject, error) {
	if err := s.enter("cancelBooking"); err != nil {
		return nil, err
	}
	return httpapi.CancelBooking200JSONResponse{Id: req.BookingId, Status: httpapi.BookingStatusCancelled}, nil
}

func (s *stubServer) ListResources(context.Context, httpapi.ListResourcesRequestObject) (httpapi.ListResourcesResponseObject, error) {
	if err := s.enter("listResources"); err != nil {
		return nil, err
	}
	return httpapi.ListResources200JSONResponse{Items: []httpapi.Resource{}}, nil
}

func (s *stubServer) CreateResource(_ context.Context, req httpapi.CreateResourceRequestObject) (httpapi.CreateResourceResponseObject, error) {
	if err := s.enter("createResource"); err != nil {
		return nil, err
	}
	return httpapi.CreateResource201JSONResponse{Id: "r1", Name: req.Body.Name, WeeklyHours: req.Body.WeeklyHours}, nil
}

func (s *stubServer) GetResource(_ context.Context, req httpapi.GetResourceRequestObject) (httpapi.GetResourceResponseObject, error) {
	if err := s.enter("getResource"); err != nil {
		return nil, err
	}
	return httpapi.GetResource200JSONResponse{Id: req.ResourceId}, nil
}

func (s *stubServer) GetAvailability(_ context.Context, req httpapi.GetAvailabilityRequestObject) (httpapi.GetAvailabilityResponseObject, error) {
	if err := s.enter("getAvailability"); err != nil {
		return nil, err
	}
	return httpapi.GetAvailability200JSONResponse{ResourceId: req.ResourceId, From: req.Params.From, To: req.Params.To, Slots: []httpapi.Slot{}}, nil
}

var _ httpapi.StrictServerInterface = (*stubServer)(nil)

const (
	validResource = `{"name":"Dr. Lee","weeklyHours":[{"weekday":"monday","start":"09:00","end":"17:00"}]}`
	validBooking  = `{"resourceId":"r1","slotStart":"2026-11-02T09:00:00Z","customerName":"Ann","idempotencyKey":"k1"}`
)

func serve(h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// assertError checks the contract's error shape (CON-004).
func assertError(t *testing.T, rec *httptest.ResponseRecorder, status int, code httpapi.ErrorCode) httpapi.Error {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body %s", rec.Code, status, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var e httpapi.Error
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("body is not a JSON error: %v; body %s", err, rec.Body)
	}
	if e.Code != code {
		t.Fatalf("code = %q, want %q", e.Code, code)
	}
	return e
}

// Every operation of the specification is mounted on the http.ServeMux and reaches its handler.
func TestNewHandler_MountsEveryOperationOfTheSpec(t *testing.T) {
	doc := loadSpec(t)
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			t.Run(op.OperationID, func(t *testing.T) {
				target := strings.NewReplacer("{resourceId}", "r1", "{bookingId}", "b1").Replace(path)
				body := ""
				switch op.OperationID {
				case "getAvailability":
					target += "?from=2026-11-02&to=2026-11-15"
				case "createResource":
					body = validResource
				case "bookSlot":
					body = validBooking
				}
				stub := &stubServer{}
				rec := serve(httpapi.NewHandler(stub), method, target, body)
				if stub.called != op.OperationID {
					t.Fatalf("%s %s reached %q, want %q (status %d, body %s)", method, target, stub.called, op.OperationID, rec.Code, rec.Body)
				}
				if rec.Code < 200 || rec.Code > 299 {
					t.Fatalf("status = %d, want a success", rec.Code)
				}
			})
		}
	}
}

func TestNewHandler_TypedErrorResponsePassesThrough(t *testing.T) {
	body := strings.Replace(validBooking, `"k1"`, `"taken"`, 1)
	assertError(t, serve(httpapi.NewHandler(&stubServer{}), http.MethodPost, "/bookings", body), http.StatusConflict, httpapi.ErrorCodeSlotTaken)
}

func TestNewHandler_UnknownPathIsNotFound(t *testing.T) {
	assertError(t, serve(httpapi.NewHandler(&stubServer{}), http.MethodGet, "/nope", ""), http.StatusNotFound, httpapi.ErrorCodeNotFound)
}

func TestNewHandler_WrongMethodIsMethodNotAllowed(t *testing.T) {
	stub := &stubServer{}
	rec := serve(httpapi.NewHandler(stub), http.MethodDelete, "/bookings/b1", "")
	assertError(t, rec, http.StatusMethodNotAllowed, httpapi.ErrorCodeMethodNotAllowed)
	if allow := rec.Header().Get("Allow"); !strings.Contains(allow, "GET") {
		t.Fatalf("Allow = %q, want it to list GET", allow)
	}
	if stub.called != "" {
		t.Fatalf("handler %q was called", stub.called)
	}
}

func TestNewHandler_MalformedBodyIsValidationError(t *testing.T) {
	for _, body := range []string{"", "{", "[]", `{"customerName": 7}`} {
		stub := &stubServer{}
		assertError(t, serve(httpapi.NewHandler(stub), http.MethodPost, "/bookings", body), http.StatusBadRequest, httpapi.ErrorCodeValidationError)
		if stub.called != "" {
			t.Fatalf("body %q reached the handler", body)
		}
	}
}

func TestNewHandler_MalformedQueryParameterIsValidationError(t *testing.T) {
	for _, query := range []string{"", "?from=2026-11-02", "?from=tomorrow&to=2026-11-15", "?from=2026-11-02&to=2026-13-01"} {
		stub := &stubServer{}
		assertError(t, serve(httpapi.NewHandler(stub), http.MethodGet, "/resources/r1/availability"+query, ""), http.StatusBadRequest, httpapi.ErrorCodeValidationError)
		if stub.called != "" {
			t.Fatalf("query %q reached the handler", query)
		}
	}
}

func TestNewHandler_OversizedBodyIsRequestTooLarge(t *testing.T) {
	body := `{"name":"` + strings.Repeat("a", 1<<20) + `"}`
	stub := &stubServer{}
	assertError(t, serve(httpapi.NewHandler(stub), http.MethodPost, "/resources", body), http.StatusRequestEntityTooLarge, httpapi.ErrorCodeRequestTooLarge)
	if stub.called != "" {
		t.Fatal("an oversized body reached the handler")
	}
}

func TestNewHandler_HandlerErrorIsInternalErrorWithoutDetail(t *testing.T) {
	h := httpapi.NewHandler(&stubServer{fail: errors.New("disk on fire at /var/db")})
	e := assertError(t, serve(h, http.MethodGet, "/resources", ""), http.StatusInternalServerError, httpapi.ErrorCodeInternalError)
	if strings.Contains(e.Message, "disk") {
		t.Fatalf("internal error detail leaked: %q", e.Message)
	}
}

func TestNewHandler_HandlerPanicIsInternalError(t *testing.T) {
	h := httpapi.NewHandler(&stubServer{panicValue: "boom"})
	assertError(t, serve(h, http.MethodGet, "/resources", ""), http.StatusInternalServerError, httpapi.ErrorCodeInternalError)
}

func TestNewHandler_AbortHandlerPanicIsReraised(t *testing.T) {
	h := httpapi.NewHandler(&stubServer{panicValue: http.ErrAbortHandler})
	defer func() {
		if v := recover(); v != http.ErrAbortHandler {
			t.Fatalf("recovered %v, want http.ErrAbortHandler re-raised", v)
		}
	}()
	serve(h, http.MethodGet, "/resources", "")
}

// Property (CON-004, ASM-0005): whatever method, path and body arrive, every failure response is a
// JSON Error whose code is one the specification declares.
func TestProperty_EveryFailureIsJSONErrorWithDeclaredCode(t *testing.T) {
	doc := loadSpec(t)
	declared := map[httpapi.ErrorCode]bool{}
	for _, v := range schema(t, doc, "ErrorCode").Enum {
		declared[httpapi.ErrorCode(v.(string))] = true
	}
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	paths := []string{"/", "/resources", "/resources/%s", "/resources/%s/availability", "/resources/%s/availability?from=%s&to=%s", "/bookings", "/bookings/%s", "/bookings/%s/cancel", "/%s"}
	bodies := []string{"", "{", "null", "[]", validResource, validBooking, `{"slotStart":"yesterday"}`}
	h := httpapi.NewHandler(&stubServer{})

	property := func(m, p, b uint8, seg1, seg2, seg3 string, raw []byte) bool {
		segs := []any{escape(seg1), escape(seg2), escape(seg3)}
		path := paths[int(p)%len(paths)]
		target := path
		if n := strings.Count(path, "%s"); n > 0 {
			target = fmt.Sprintf(path, segs[:n]...)
		}
		body := bodies[int(b)%len(bodies)]
		if b%3 == 0 {
			body = string(raw)
		}
		rec := serve(h, methods[int(m)%len(methods)], target, body)
		if rec.Code < 400 {
			return true
		}
		var e httpapi.Error
		return rec.Header().Get("Content-Type") == "application/json" &&
			json.Unmarshal(rec.Body.Bytes(), &e) == nil &&
			declared[e.Code]
	}
	seed := time.Now().UnixNano()
	t.Logf("seed %d", seed)
	cfg := &quick.Config{MaxCount: 2000, Rand: rand.New(rand.NewSource(seed))}
	if err := quick.Check(property, cfg); err != nil {
		t.Fatal(err)
	}
}

// escape keeps generated segments valid in a request target; empty segments stay empty so that
// unknown and partial paths are generated too.
func escape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
