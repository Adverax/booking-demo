package httpapi

import (
	"context"
	"errors"

	"github.com/adverax/booking-demo/internal/availability"
)

// GetAvailability lists the free slots of a resource for the UTC days from through to, both
// inclusive (FR-003, FR-004): 200 with the slots ordered by start; 400 validation_error for a range
// whose from is after its to or that covers more than 14 days, before any booking is read
// (ADR-0018, AC-012, AC-054); 404 not_found for an unknown resource (AC-013). Slot instants leave
// as RFC 3339 UTC (CON-001, CON-008).
func (s *Server) GetAvailability(ctx context.Context, req GetAvailabilityRequestObject) (GetAvailabilityResponseObject, error) {
	free, err := s.availability.FreeSlots(ctx, req.ResourceId, req.Params.From.Time, req.Params.To.Time)
	switch {
	case errors.Is(err, availability.ErrInvalidRange):
		return GetAvailability400JSONResponse{Code: ErrorCodeValidationError, Message: err.Error()}, nil
	case errors.Is(err, availability.ErrUnknownResource):
		return GetAvailability404JSONResponse{NotFoundJSONResponse{Code: ErrorCodeNotFound, Message: "resource not found"}}, nil
	case err != nil:
		return nil, err
	}
	slots := make([]Slot, 0, len(free))
	for _, f := range free {
		slots = append(slots, Slot{Start: f.Start.UTC(), End: f.End.UTC()})
	}
	return GetAvailability200JSONResponse{ResourceId: req.ResourceId, From: req.Params.From, To: req.Params.To, Slots: slots}, nil
}
