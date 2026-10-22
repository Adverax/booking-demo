package store

import (
	"context"
	"errors"
	"testing"

	"github.com/adverax/booking-demo/internal/booking"
)

// GetBookingByIdempotencyKey is the classifying read of ADR-0011: it returns the booking stored
// under the key in its current state, compares the key byte for byte (ADR-0015), and reports an
// unknown key as booking.ErrNotFound.
func TestStore_GetBookingByIdempotencyKey(t *testing.T) {
	s := openTestStore(t)
	insertResource(t, s.db, "r1")
	ctx := context.Background()
	stored := newBooking("b1", "r1", slotTime, "Key-1")
	if err := s.InsertBooking(ctx, stored); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetBookingByIdempotencyKey(ctx, "Key-1"); err != nil || got != stored {
		t.Fatalf("GetBookingByIdempotencyKey = %+v, %v; want %+v", got, err, stored)
	}

	if _, err := s.UpdateBookingStatus(ctx, "b1", booking.StatusCancelled); err != nil {
		t.Fatal(err)
	}
	want := stored
	want.Status = booking.StatusCancelled
	if got, err := s.GetBookingByIdempotencyKey(ctx, "Key-1"); err != nil || got != want {
		t.Fatalf("after cancel: %+v, %v; want %+v", got, err, want)
	}

	for _, key := range []string{"key-1", "KEY-1", "Key-1 ", " Key-1", "Key-", "missing"} {
		if _, err := s.GetBookingByIdempotencyKey(ctx, key); !errors.Is(err, booking.ErrNotFound) {
			t.Errorf("key %q: err = %v, want ErrNotFound", key, err)
		}
	}
}

// A failing read is reported as an error, not as "not found": the classifier must not turn a
// store failure into slot_taken.
func TestStore_GetBookingByIdempotencyKeyReportsStoreFailures(t *testing.T) {
	s := openTestStore(t)
	s.Close()
	_, err := s.GetBookingByIdempotencyKey(context.Background(), "k1")
	if err == nil || errors.Is(err, booking.ErrNotFound) {
		t.Fatalf("err = %v, want a store failure", err)
	}
}
