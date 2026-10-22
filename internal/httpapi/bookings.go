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
