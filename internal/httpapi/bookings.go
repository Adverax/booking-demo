package httpapi

import (
	"context"
	"errors"

	"github.com/adverax/booking-demo/internal/booking"
)

// BookSlot books one slot (FR-005): 201 with the new booking, or 409 slot_taken when the store's
// unique index rejected the insert (AC-018, ADR-0008). Instants cross this boundary as RFC 3339
// and are converted to and from the domain's time.Time here only (CON-008).
func (s *Server) BookSlot(ctx context.Context, req BookSlotRequestObject) (BookSlotResponseObject, error) {
	b, err := s.bookings.Book(ctx, booking.Request{
		ResourceID:     req.Body.ResourceId,
		SlotStart:      req.Body.SlotStart,
		CustomerName:   req.Body.CustomerName,
		IdempotencyKey: req.Body.IdempotencyKey,
	})
	if errors.Is(err, booking.ErrSlotTaken) {
		return BookSlot409JSONResponse{Code: ErrorCodeSlotTaken, Message: "the slot already has an active booking"}, nil
	}
	if err != nil {
		return nil, err
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
