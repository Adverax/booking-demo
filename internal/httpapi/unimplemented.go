package httpapi

import "context"

// Operations of the contract whose cards have not landed yet. Each card moves its operation out of
// this file into the file of its tag.

// GetAvailability is CARD-008's.
func (s *Server) GetAvailability(context.Context, GetAvailabilityRequestObject) (GetAvailabilityResponseObject, error) {
	return nil, errNotImplemented
}
