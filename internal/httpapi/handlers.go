package httpapi

import (
	"github.com/adverax/booking-demo/internal/availability"
	"github.com/adverax/booking-demo/internal/booking"
	"github.com/adverax/booking-demo/internal/catalog"
)

// Server implements StrictServerInterface over the domain components: it translates the generated
// request types into domain calls, and domain results and errors into the generated response
// types. Each operation lives in the file of its tag (resources.go, availability.go, bookings.go).
type Server struct {
	catalog      *catalog.Catalog
	availability *availability.Service
	bookings     *booking.Service
}

var _ StrictServerInterface = (*Server)(nil)

// NewServer returns the Server backed by the given components.
func NewServer(c *catalog.Catalog, a *availability.Service, b *booking.Service) *Server {
	return &Server{catalog: c, availability: a, bookings: b}
}
