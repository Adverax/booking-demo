// Package httpapi is the HTTP API of the booking service (COMP-001): the types and the net/http
// server interface generated from api/openapi.yaml (api.gen.go, ADR-0009), and NewHandler, which
// mounts them on an http.ServeMux so that every failure the plumbing itself detects is answered
// with the JSON error shape of the contract (CON-004).
//
// Handlers live with the cards that own each behaviour: they implement StrictServerInterface and
// answer with the generated response types, so a response the contract does not declare does not
// compile.
package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"

	"github.com/adverax/booking-demo/internal/booking"
)

// maxRequestBodyBytes caps how much of a request body is read. The largest request of the
// contract (a resource with seven intervals) is a few hundred bytes; the cap only bounds the
// memory a malformed or hostile body can take.
const maxRequestBodyBytes = 1 << 20

// jsonBodyRoutes are the ServeMux patterns of the operations that take a JSON request body. The
// generated decoder reads only the first JSON value of a body, so NewHandler checks the whole body
// of these routes first. TestNewHandler_BodyWithTrailingDataIsValidationError keeps this set equal
// to the operations of api/openapi.yaml that declare a requestBody.
var jsonBodyRoutes = map[string]bool{
	"POST /resources": true,
	"POST /bookings":  true,
}

// NewHandler mounts the generated routes on a new http.ServeMux, backed by ssi, and returns the
// handler to serve. Failures detected before or around ssi are answered with the contract's error
// shape:
//   - unknown path, or a path that is not in canonical form (an empty segment, "." or ".."), which
//     http.ServeMux would otherwise answer with a redirect: 404 not_found; known path with another
//     method: 405 method_not_allowed;
//   - malformed path or query parameter: 400 validation_error;
//   - on an operation that takes a JSON body, a body that is not exactly one JSON value (empty,
//     malformed, or followed by trailing data): 400 validation_error; a well-formed body of the
//     wrong type: 400 validation_error from the generated decoder;
//   - body over maxRequestBodyBytes: 413 request_too_large;
//   - an error returned by ssi, or a panic before anything was written: 500 internal_error,
//     without the error text.
func NewHandler(ssi StrictServerInterface) http.Handler {
	mux := http.NewServeMux()
	HandlerWithOptions(
		NewStrictHandlerWithOptions(ssi, nil, StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  writeRequestError,
			ResponseErrorHandlerFunc: writeResponseError,
		}),
		StdHTTPServerOptions{BaseRouter: mux, ErrorHandlerFunc: writeRequestError},
	)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isCanonicalPath(r.URL.Path) {
			writeError(w, http.StatusNotFound, ErrorCodeNotFound, "not found")
			return
		}
		h, pattern := mux.Handler(r)
		if pattern == "" {
			writeRouteError(w, r, h)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		if jsonBodyRoutes[pattern] {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				writeRequestError(w, r, err)
				return
			}
			if !json.Valid(body) {
				writeError(w, http.StatusBadRequest, ErrorCodeValidationError, "request body is not a single JSON value")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		tw := &trackingWriter{ResponseWriter: w}
		defer recoverAsInternalError(tw)
		mux.ServeHTTP(tw, r)
	})
}

// writeError writes the contract's error shape. Handlers of later cards use the generated
// response types instead; this is for failures outside any operation's handler.
func writeError(w http.ResponseWriter, status int, code ErrorCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Error{Code: code, Message: message})
}

// writeRequestError answers a request the generated code could not bind or decode. The message
// describes the caller's own input, so it is returned.
func writeRequestError(w http.ResponseWriter, _ *http.Request, err error) {
	if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
		writeError(w, http.StatusRequestEntityTooLarge, ErrorCodeRequestTooLarge, "request body too large")
		return
	}
	writeError(w, http.StatusBadRequest, ErrorCodeValidationError, err.Error())
}

// writeResponseError answers an error returned by a handler, or a response that failed to encode.
// The error text is internal and is not returned. Once a status line was sent nothing more can be
// said on this response, so nothing is written. No logging is added here: no logging requirement
// is decided (DEC-010).
//
// A handler error wrapping booking.ErrBookingCancelled is the one domain error answered here
// instead of by a typed response: no operation of the contract requests the change it reports
// (ADR-0010, ASM-0001), yet if any ever reaches the schema guard the caller gets the stable
// 409 booking_cancelled the contract's ErrorCode declares, not a bare 500 (AC-030).
func writeResponseError(w http.ResponseWriter, _ *http.Request, err error) {
	if tw, ok := w.(*trackingWriter); ok && tw.wrote {
		return
	}
	if errors.Is(err, booking.ErrBookingCancelled) {
		writeError(w, http.StatusConflict, ErrorCodeBookingCancelled, "the booking is cancelled; a cancelled booking is final")
		return
	}
	writeError(w, http.StatusInternalServerError, ErrorCodeInternalError, "internal error")
}

// recoverAsInternalError turns a handler panic into 500 internal_error when nothing was written
// yet. http.ErrAbortHandler, and a panic after the status line was sent, are re-raised so that
// net/http aborts the connection as it is designed to.
func recoverAsInternalError(tw *trackingWriter) {
	v := recover()
	if v == nil {
		return
	}
	if v == http.ErrAbortHandler || tw.wrote {
		panic(v)
	}
	// TODO(DEC-010): the panic value and stack are dropped here, so a recovered panic leaves no
	// trace (net/http would have logged it). Whether and how the service logs is undecided;
	// record the panic here once DEC-010 is resolved.
	writeError(tw, http.StatusInternalServerError, ErrorCodeInternalError, "internal error")
}

// isCanonicalPath reports whether p is already in the form http.ServeMux routes on: rooted, and
// unchanged by path.Clean apart from a kept trailing slash. ServeMux answers any other path with a
// redirect to its cleaned form, which the contract does not declare.
func isCanonicalPath(p string) bool {
	if p == "" || p[0] != '/' {
		return false
	}
	clean := path.Clean(p)
	if p[len(p)-1] == '/' && clean != "/" {
		clean += "/"
	}
	return clean == p
}

// writeRouteError answers a request that matches no route. h is the handler http.ServeMux chose
// for it; it is run against a recorder only to learn whether the mux answers 404 or 405 and which
// methods it allows.
func writeRouteError(w http.ResponseWriter, r *http.Request, h http.Handler) {
	rec := &statusRecorder{header: http.Header{}}
	h.ServeHTTP(rec, r)
	if rec.status == http.StatusMethodNotAllowed {
		w.Header().Set("Allow", rec.header.Get("Allow"))
		writeError(w, http.StatusMethodNotAllowed, ErrorCodeMethodNotAllowed, "method not allowed")
		return
	}
	writeError(w, http.StatusNotFound, ErrorCodeNotFound, "not found")
}

// trackingWriter remembers whether anything was written, so that an error found later does not
// try to send a second status line.
type trackingWriter struct {
	http.ResponseWriter
	wrote bool
}

func (w *trackingWriter) WriteHeader(status int) {
	w.wrote = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *trackingWriter) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *trackingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// statusRecorder captures the status and headers of http.ServeMux's own 404/405 answer; the
// plain-text body is discarded.
type statusRecorder struct {
	header http.Header
	status int
}

func (r *statusRecorder) Header() http.Header { return r.header }

func (r *statusRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return len(b), nil
}
