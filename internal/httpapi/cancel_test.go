package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"

	"github.com/adverax/booking-demo/internal/booking"
	"github.com/adverax/booking-demo/internal/catalog"
	"github.com/adverax/booking-demo/internal/httpapi"
)

// API tests of cancelling and reading a booking (FR-007, FR-008): the real handler over the real
// store, every response validated against api/openapi.yaml.

func decodeBooking(t *testing.T, rec *httptest.ResponseRecorder) httpapi.Booking {
	t.Helper()
	var b httpapi.Booking
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("body is not a booking: %v; body %s", err, rec.Body)
	}
	return b
}

// bookAnn books Monday 09:00 for "Ann" on a fresh resource and returns the booking.
func (e *apiEnv) bookAnn() httpapi.Booking {
	e.t.Helper()
	r := e.createResource("Dr. Lee", weekdayHours(monToFri, "09:00", "17:00"))
	rec := e.book(r.Id, monday9, "Ann", "key-ann")
	if rec.Code != http.StatusCreated {
		e.t.Fatalf("book: %d %s", rec.Code, rec.Body)
	}
	return decodeBooking(e.t, rec)
}

// validateAt checks rec against the operation at the spec's path template (conforms looks up
// literal paths only, and these paths carry a booking id).
func (e *apiEnv) validateAt(method, template, target string, rec *httptest.ResponseRecorder) {
	e.t.Helper()
	item := e.spec.Paths.Find(template)
	if item == nil {
		e.t.Fatalf("no path %s in the spec", template)
	}
	err := openapi3filter.ValidateResponse(context.Background(), &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request: httptest.NewRequest(method, target, nil),
			Route:   &routers.Route{Spec: e.spec, Path: template, PathItem: item, Method: method, Operation: item.GetOperation(method)},
		},
		Status:  rec.Code,
		Header:  rec.Header(),
		Body:    io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
		Options: &openapi3filter.Options{IncludeResponseStatus: true},
	})
	if err != nil {
		e.t.Fatalf("%s %s: response %d %s does not conform to the contract: %v", method, target, rec.Code, rec.Body, err)
	}
}

func (e *apiEnv) cancel(id string) *httptest.ResponseRecorder {
	e.t.Helper()
	rec := serve(e.handler, http.MethodPost, "/bookings/"+id+"/cancel", "")
	e.validateAt(http.MethodPost, "/bookings/{bookingId}/cancel", "/bookings/"+id+"/cancel", rec)
	return rec
}

func (e *apiEnv) get(id string) *httptest.ResponseRecorder {
	e.t.Helper()
	rec := serve(e.handler, http.MethodGet, "/bookings/"+id, "")
	e.validateAt(http.MethodGet, "/bookings/{bookingId}", "/bookings/"+id, rec)
	return rec
}

func (e *apiEnv) storedStatus(id string) string {
	e.t.Helper()
	var status string
	if err := e.db.QueryRow("SELECT status FROM booking WHERE id = ?", id).Scan(&status); err != nil {
		e.t.Fatalf("read status of %s: %v", id, err)
	}
	return status
}

// AC-024: Ann's active Monday 09:00 booking, cancelled by its id, has status cancelled.
func TestCancelBooking_MarksCancelled(t *testing.T) {
	e := newAPIEnv(t)
	b1 := e.bookAnn()

	rec := e.cancel(b1.Id)
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body)
	}
	got := decodeBooking(t, rec)
	want := b1
	want.Status = httpapi.BookingStatusCancelled
	if got != want {
		t.Fatalf("cancelled booking %+v, want %+v", got, want)
	}
	if s := e.storedStatus(b1.Id); s != "cancelled" {
		t.Fatalf("stored status %q, want cancelled", s)
	}
}

// AC-026: a cancel request that carries only the booking id, no customer identity of any kind (no
// body, no header naming a customer), cancels Ann's booking (CON-006).
func TestCancelBooking_AnyoneWithIdCanCancel(t *testing.T) {
	e := newAPIEnv(t)
	b1 := e.bookAnn()

	req := httptest.NewRequest(http.MethodPost, "/bookings/"+b1.Id+"/cancel", http.NoBody)
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	e.validateAt(http.MethodPost, "/bookings/{bookingId}/cancel", "/bookings/"+b1.Id+"/cancel", rec)
	if rec.Code != http.StatusOK || decodeBooking(t, rec).Status != httpapi.BookingStatusCancelled {
		t.Fatalf("cancel without identity: %d %s", rec.Code, rec.Body)
	}
	if s := e.storedStatus(b1.Id); s != "cancelled" {
		t.Fatalf("stored status %q, want cancelled", s)
	}
}

// AC-027: cancelling "missing-id" is a not-found JSON error with a stable code; nothing is stored.
func TestCancelBooking_UnknownIdNotFound(t *testing.T) {
	e := newAPIEnv(t)
	assertError(t, e.cancel("missing-id"), http.StatusNotFound, httpapi.ErrorCodeNotFound)
	if n := e.count("SELECT count(*) FROM booking"); n != 0 {
		t.Fatalf("%d bookings stored", n)
	}
}

// Reading an unknown booking is not found too (GetBooking's declared 404).
func TestGetBooking_UnknownIdNotFound(t *testing.T) {
	e := newAPIEnv(t)
	assertError(t, e.get("missing-id"), http.StatusNotFound, httpapi.ErrorCodeNotFound)
}

// AC-048: cancelling B1 again succeeds and carries B1 with its original id and status cancelled,
// byte for byte the response of the first cancel (ADR-0003).
func TestCancelBooking_RepeatedCancelReturnsCancelledBooking(t *testing.T) {
	e := newAPIEnv(t)
	b1 := e.bookAnn()
	first := e.cancel(b1.Id)
	if first.Code != http.StatusOK {
		t.Fatalf("first cancel: %d %s", first.Code, first.Body)
	}

	again := e.cancel(b1.Id)
	if again.Code != http.StatusOK {
		t.Fatalf("repeated cancel: %d %s", again.Code, again.Body)
	}
	got := decodeBooking(t, again)
	if got.Id != b1.Id || got.Status != httpapi.BookingStatusCancelled {
		t.Fatalf("repeated cancel returned %+v", got)
	}
	if again.Body.String() != first.Body.String() {
		t.Fatalf("repeated cancel body %s differs from the first %s", again.Body, first.Body)
	}
}

// cancelAndRebook cancels Ann's Monday 09:00 booking B1 and books the slot again with a new key.
func (e *apiEnv) cancelAndRebook() (b1, b2 httpapi.Booking) {
	e.t.Helper()
	b1 = e.bookAnn()
	if rec := e.cancel(b1.Id); rec.Code != http.StatusOK {
		e.t.Fatalf("cancel: %d %s", rec.Code, rec.Body)
	}
	rec := e.book(b1.ResourceId, monday9, "Bob", "key-bob")
	if rec.Code != http.StatusCreated {
		e.t.Fatalf("booking the freed slot: %d %s", rec.Code, rec.Body)
	}
	return b1, decodeBooking(e.t, rec)
}

// AC-028: after B1 (Monday 09:00) is cancelled, booking Monday 09:00 with a new key creates a new
// booking B2 with a different id; B1 is not reused.
func TestBooking_NewBookingAfterCancelCreatesNewId(t *testing.T) {
	e := newAPIEnv(t)
	b1, b2 := e.cancelAndRebook()
	if b2.Id == b1.Id || b2.Status != httpapi.BookingStatusActive || !b2.SlotStart.Equal(monday9) {
		t.Fatalf("B1 %+v, B2 %+v; want a new active booking with another id", b1, b2)
	}
	if n := e.count("SELECT count(*) FROM booking"); n != 2 {
		t.Fatalf("%d bookings stored, want 2", n)
	}
}

// AC-029: with B1 cancelled and B2 active for the same slot, reading B1 shows it cancelled.
func TestBooking_CancelledStaysCancelled(t *testing.T) {
	e := newAPIEnv(t)
	b1, b2 := e.cancelAndRebook()

	rec := e.get(b1.Id)
	if rec.Code != http.StatusOK {
		t.Fatalf("get B1: %d %s", rec.Code, rec.Body)
	}
	if got := decodeBooking(t, rec); got.Id != b1.Id || got.Status != httpapi.BookingStatusCancelled {
		t.Fatalf("B1 read as %+v, want cancelled", got)
	}
	if got := decodeBooking(t, e.get(b2.Id)); got.Status != httpapi.BookingStatusActive {
		t.Fatalf("B2 read as %+v, want active", got)
	}
}

// reactivatingServer is the request AC-030 needs and the contract does not have (NG-2, ASM-0001):
// its cancel operation instead tries to set the booking ACTIVE through the store's status port,
// bypassing the domain guard, so only the schema stands in its way. Everything else is the real
// server.
type reactivatingServer struct {
	*httpapi.Server
	repo booking.Repository
}

func (s reactivatingServer) CancelBooking(ctx context.Context, req httpapi.CancelBookingRequestObject) (httpapi.CancelBookingResponseObject, error) {
	if _, err := s.repo.UpdateBookingStatus(ctx, req.BookingId, booking.StatusActive); err != nil {
		return nil, err
	}
	return nil, errors.New("reactivation was not rejected")
}

// AC-030, ADR-0010/R1: an attempt to set cancelled B1 active again is a conflict JSON error with
// the stable code booking_cancelled, and B1 stays cancelled. Both guards are exercised: the domain
// rule on the aggregate, and the schema reached through a request that skips the domain rule.
func TestBooking_CannotReactivate(t *testing.T) {
	e := newAPIEnv(t)
	b1 := e.bookAnn()
	if rec := e.cancel(b1.Id); rec.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body)
	}
	svc := booking.NewService(e.store, func() time.Time { return fixedNow })

	// Domain guard.
	stored, err := svc.Get(context.Background(), b1.Id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stored.WithStatus(booking.StatusActive); !errors.Is(err, booking.ErrBookingCancelled) {
		t.Fatalf("domain guard: err = %v, want ErrBookingCancelled", err)
	}

	// Schema guard, through the HTTP plumbing.
	h := httpapi.NewHandler(reactivatingServer{Server: httpapi.NewServer(catalog.New(e.store), svc), repo: e.store})
	rec := serve(h, http.MethodPost, "/bookings/"+b1.Id+"/cancel", "")
	assertError(t, rec, http.StatusConflict, httpapi.ErrorCodeBookingCancelled)
	var body any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if err := schema(t, e.spec, "Error").VisitJSON(body); err != nil {
		t.Fatalf("conflict body is not the contract's Error: %v", err)
	}

	if got := decodeBooking(t, e.get(b1.Id)); got.Status != httpapi.BookingStatusCancelled {
		t.Fatalf("B1 read as %+v after the attempt, want cancelled", got)
	}
	if s := e.storedStatus(b1.Id); s != "cancelled" {
		t.Fatalf("stored status %q, want cancelled", s)
	}
}

// The conflict mapping is specific: any other handler error is still a bare 500 internal_error.
func TestNewHandler_OnlyCancelledIsFinalMapsToConflict(t *testing.T) {
	for err, want := range map[error]httpapi.ErrorCode{
		booking.ErrBookingCancelled:                       httpapi.ErrorCodeBookingCancelled,
		errors.Join(errors.New("x"), booking.ErrNotFound): httpapi.ErrorCodeInternalError,
		errors.New("disk on fire"):                        httpapi.ErrorCodeInternalError,
	} {
		status := http.StatusInternalServerError
		if want == httpapi.ErrorCodeBookingCancelled {
			status = http.StatusConflict
		}
		assertError(t, serve(httpapi.NewHandler(&stubServer{fail: err}), http.MethodGet, "/bookings/b1", ""), status, want)
	}
}
