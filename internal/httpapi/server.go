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
	"encoding/json"
	"errors"
	"net/http"
)

// maxRequestBodyBytes caps how much of a request body is read. The largest request of the
// contract (a resource with seven intervals) is a few hundred bytes; the cap only bounds the
// memory a malformed or hostile body can take.
const maxRequestBodyBytes = 1 << 20

// NewHandler mounts the generated routes on a new http.ServeMux, backed by ssi, and returns the
// handler to serve. Failures detected before or around ssi are answered with the contract's error
// shape:
//   - unknown path: 404 not_found; known path with another method: 405 method_not_allowed;
//   - malformed path or query parameter, malformed JSON body: 400 validation_error;
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
		if h, pattern := mux.Handler(r); pattern == "" {
			writeRouteError(w, r, h)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
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
func writeResponseError(w http.ResponseWriter, _ *http.Request, _ error) {
	if tw, ok := w.(*trackingWriter); ok && tw.wrote {
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
	writeError(tw, http.StatusInternalServerError, ErrorCodeInternalError, "internal error")
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
