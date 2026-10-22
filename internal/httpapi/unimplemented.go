package httpapi

import "context"

// Operations of the contract whose cards have not landed yet. Each card moves its operation out of
// this file into the file of its tag.

// GetBooking belongs to a later card (FR-007/AC-029); no card of the current plan names it yet.
func (s *Server) GetBooking(context.Context, GetBookingRequestObject) (GetBookingResponseObject, error) {
	return nil, errNotImplemented
}

// CancelBooking is CARD-004's.
func (s *Server) CancelBooking(context.Context, CancelBookingRequestObject) (CancelBookingResponseObject, error) {
	return nil, errNotImplemented
}

// GetAvailability is CARD-008's.
func (s *Server) GetAvailability(context.Context, GetAvailabilityRequestObject) (GetAvailabilityResponseObject, error) {
	return nil, errNotImplemented
}
