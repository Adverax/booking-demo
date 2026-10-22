package httpapi

import (
	"errors"

	"github.com/adverax/booking-demo/internal/booking"
	"github.com/adverax/booking-demo/internal/catalog"
)

// Server implements StrictServerInterface over the domain components: it translates the generated
// request types into domain calls, and domain results and errors into the generated response
// types. Each operation lives in the file of its tag (resources.go, bookings.go); operations whose
// card has not landed yet are in unimplemented.go.
type Server struct {
	catalog  *catalog.Catalog
	bookings *booking.Service
}

var _ StrictServerInterface = (*Server)(nil)

// NewServer returns the Server backed by the given components.
func NewServer(c *catalog.Catalog, b *booking.Service) *Server {
	return &Server{catalog: c, bookings: b}
}

// errNotImplemented is returned by operations whose card has not landed; NewHandler answers it
// with 500 internal_error, as any handler error.
var errNotImplemented = errors.New("httpapi: operation not implemented yet")
