package httpapi

import (
	"context"
	"errors"

	"github.com/adverax/booking-demo/internal/booking"
)

// BookSlot books one slot (FR-005, FR-006): 201 with the new booking; 200 with the original
// booking in its current state when the key is replayed with the same payload (ADR-0002, ADR-0011);
// 409 idempotency_key_conflict when the key is stored with another payload (ADR-0001); 409
// slot_taken when the slot has an active booking under another key (AC-018, ADR-0008); 400
// validation_error for an empty key or customer name or a slot start not on the half hour
// (ADR-0018, AC-017); 404 not_found for an unknown resource (AC-020); 422
// slot_outside_working_hours (AC-015); 422 slot_in_past for a past slot with nothing stored under
// its key (ADR-0016, ADR-0020). Nothing is stored on any error. Instants cross this boundary as RFC 3339 and are converted to and from the domain's
// time.Time here only (CON-008).
func (s *Server) BookSlot(ctx context.Context, req BookSlotRequestObject) (BookSlotResponseObject, error) {
	b, replayed, err := s.bookings.Book(ctx, booking.Request{
		ResourceID:     req.Body.ResourceId,
		SlotStart:      req.Body.SlotStart,
		CustomerName:   req.Body.CustomerName,
		IdempotencyKey: req.Body.IdempotencyKey,
	})
	switch {
	case errors.Is(err, booking.ErrInvalidRequest):
		return BookSlot400JSONResponse{ValidationErrorJSONResponse{Code: ErrorCodeValidationError, Message: err.Error()}}, nil
	case errors.Is(err, booking.ErrUnknownResource):
		return BookSlot404JSONResponse{NotFoundJSONResponse{Code: ErrorCodeNotFound, Message: "no resource with this id"}}, nil
	case errors.Is(err, booking.ErrOutsideWorkingHours):
		return BookSlot422JSONResponse{Code: ErrorCodeSlotOutsideWorkingHours, Message: "the slot is not inside the resource's working hours"}, nil
	case errors.Is(err, booking.ErrSlotInPast):
		return BookSlot422JSONResponse{Code: ErrorCodeSlotInPast, Message: "the slot starts before the current time"}, nil
	case errors.Is(err, booking.ErrKeyConflict):
		return BookSlot409JSONResponse{Code: ErrorCodeIdempotencyKeyConflict, Message: "the idempotency key is already used with a different payload"}, nil
	case errors.Is(err, booking.ErrSlotTaken):
		return BookSlot409JSONResponse{Code: ErrorCodeSlotTaken, Message: "the slot already has an active booking"}, nil
	case err != nil:
		return nil, err
	case replayed:
		return BookSlot200JSONResponse(bookingToAPI(b)), nil
	}
	return BookSlot201JSONResponse(bookingToAPI(b)), nil
}

// GetBooking reads one booking in its current state, a cancelled one included (AC-029): 200 with
// the booking, or 404 not_found.
func (s *Server) GetBooking(ctx context.Context, req GetBookingRequestObject) (GetBookingResponseObject, error) {
	b, err := s.bookings.Get(ctx, req.BookingId)
	if errors.Is(err, booking.ErrNotFound) {
		return GetBooking404JSONResponse{NotFoundJSONResponse{Code: ErrorCodeNotFound, Message: "no booking with this id"}}, nil
	}
	if err != nil {
		return nil, err
	}
	return GetBooking200JSONResponse(bookingToAPI(b)), nil
}

// CancelBooking cancels a booking by its id alone, with no identity check (FR-007, CON-006): 200
// with the cancelled booking, also when it was already cancelled, in the same shape (ADR-0003,
// AC-048); 404 not_found for an unknown id (AC-027).
func (s *Server) CancelBooking(ctx context.Context, req CancelBookingRequestObject) (CancelBookingResponseObject, error) {
	b, err := s.bookings.Cancel(ctx, req.BookingId)
	if errors.Is(err, booking.ErrNotFound) {
		return CancelBooking404JSONResponse{NotFoundJSONResponse{Code: ErrorCodeNotFound, Message: "no booking with this id"}}, nil
	}
	if err != nil {
		return nil, err
	}
	return CancelBooking200JSONResponse(bookingToAPI(b)), nil
}

func bookingToAPI(b booking.Booking) Booking {
	return Booking{
		Id:           b.ID,
		ResourceId:   b.ResourceID,
		SlotStart:    b.SlotStart.UTC(),
		SlotEnd:      b.SlotEnd().UTC(),
		CustomerName: b.CustomerName,
		Status:       BookingStatus(b.Status),
	}
}
