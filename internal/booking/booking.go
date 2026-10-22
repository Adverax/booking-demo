// Package booking is the Slot Booking component (COMP-004): the Booking aggregate (AGG-002) and
// the service that books one 30-minute slot of a resource under an idempotency key, cancels a
// booking and reads one back.
//
// The double-booking invariant (INV-001) is not checked here by reading: the Repository's single
// insert is the check, and the store's unique indexes decide the winner (ADR-0008, ADR-0012). A
// rejected insert is classified afterwards from what is stored under the request's idempotency
// key (insert-then-classify, ADR-0011): replay, key conflict or taken slot.
package booking

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"
)

// SlotLength is the fixed length of every slot; slots start on the UTC half hour (FR-003).
const SlotLength = 30 * time.Minute

// slotSeconds is SlotLength in whole seconds, the unit instants are stored in (ADR-0013).
const slotSeconds = int64(SlotLength / time.Second)

// Status is the lifecycle state of a booking. Cancelled is final (INV-004).
type Status string

const (
	StatusActive    Status = "active"
	StatusCancelled Status = "cancelled"
)

// Booking is one booked slot. SlotStart is always a canonical slot start: a UTC instant on the
// half hour with no sub-second part (ADR-0013, ADR-0015).
type Booking struct {
	ID             string
	ResourceID     string
	SlotStart      time.Time
	CustomerName   string
	IdempotencyKey string
	Status         Status
}

// SlotEnd is the exclusive end of the booked slot.
func (b Booking) SlotEnd() time.Time { return b.SlotStart.Add(SlotLength) }

// WithStatus returns b in status to. This is the aggregate's transition rule (INV-004, FR-008):
// a cancelled booking can only stay cancelled, so any other target is ErrBookingCancelled and b is
// returned unchanged. The schema holds the same rule (ADR-0010), so a write path that skips this
// guard is still stopped by the store.
func (b Booking) WithStatus(to Status) (Booking, error) {
	if b.Status == StatusCancelled && to != StatusCancelled {
		return b, ErrBookingCancelled
	}
	b.Status = to
	return b, nil
}

// CanonicalSlotStart returns the start of the UTC half-hour slot that contains t: whole epoch
// seconds rounded down to a multiple of 30 minutes, in UTC. Two requests name the same slot exactly
// when their resource ids and canonical starts are equal (ADR-0015, ASM-0006).
func CanonicalSlotStart(t time.Time) time.Time {
	sec := t.Unix()
	rem := sec % slotSeconds
	if rem < 0 { // floor, also before 1970
		rem += slotSeconds
	}
	return time.Unix(sec-rem, 0).UTC()
}

// SamePayload reports whether o asks for exactly what b holds: the same resource id, the same
// slot start instant (both canonical, see Booking) and a byte-for-byte equal customer name
// (ADR-0015). Nothing is trimmed or case-folded. This is the sameness a repeated idempotency key
// is judged by (ADR-0001, ADR-0011).
func (b Booking) SamePayload(o Booking) bool {
	return b.ResourceID == o.ResourceID && b.SlotStart.Equal(o.SlotStart) && b.CustomerName == o.CustomerName
}

// Request is the caller's booking payload. Strings are kept byte for byte as received (ADR-0015).
type Request struct {
	ResourceID     string
	SlotStart      time.Time
	CustomerName   string
	IdempotencyKey string
}

// ErrSlotTaken means the slot already has an active booking; nothing was stored for the request.
var ErrSlotTaken = errors.New("booking: slot taken")

// ErrKeyConflict means the idempotency key is already stored with a different payload (ADR-0001);
// nothing was stored or changed.
var ErrKeyConflict = errors.New("booking: idempotency key already used with a different payload")

// ErrInvalidRequest is wrapped by every rejection of a request before anything is stored, with the
// reason after it (ADR-0018).
var ErrInvalidRequest = errors.New("booking: invalid request")

// ErrNotUnique is what a Repository returns when the insert was rejected by a uniqueness rule of
// the store: the active-slot rule or the idempotency-key rule (ADR-0008). Which one is not said.
var ErrNotUnique = errors.New("booking: rejected by a uniqueness rule")

// ErrNotFound means no booking has the given id.
var ErrNotFound = errors.New("booking: not found")

// ErrBookingCancelled means a change would set a cancelled booking to another status; cancelled is
// final (INV-004, ADR-0010). Both the domain guard (Booking.WithStatus) and the store's schema
// trigger report it.
var ErrBookingCancelled = errors.New("booking: a cancelled booking is final")

// Repository persists bookings. It is the port of this component to the store (COMP-005).
type Repository interface {
	// InsertBooking stores b with ONE autocommit statement, no read before it (ADR-0012). A
	// rejection by a unique index is reported as an error wrapping ErrNotUnique.
	InsertBooking(ctx context.Context, b Booking) error
	// UpdateBookingStatus sets the status of booking id to `to` with ONE autocommit statement, no
	// read before it (ADR-0012), and returns the booking as stored after the update. An unknown id
	// is ErrNotFound; an update the schema rejects because the booking is cancelled (ADR-0010) is
	// an error wrapping ErrBookingCancelled, and nothing is changed.
	UpdateBookingStatus(ctx context.Context, id string, to Status) (Booking, error)
	// GetBooking returns booking id in its current state, or ErrNotFound.
	GetBooking(ctx context.Context, id string) (Booking, error)
	// GetBookingByIdempotencyKey returns the booking stored under key, byte-for-byte equal, in its
	// current state, or ErrNotFound. It is ONE autocommit read, never part of a transaction with a
	// write (ADR-0011, ADR-0012).
	GetBookingByIdempotencyKey(ctx context.Context, key string) (Booking, error)
}

// Service books slots.
type Service struct {
	repo Repository
	now  func() time.Time
}

// NewService returns a Service storing through repo. now is the only source of the current time
// for booking logic (ADR-0013); the bootstrap passes time.Now, tests pass a fixed clock. This card
// adds no rule that consults it: the past-slot rule that does is CARD-007's.
func NewService(repo Repository, now func() time.Time) *Service {
	return &Service{repo: repo, now: now}
}

// Book books the slot containing req.SlotStart, normalised to its canonical start, for
// req.CustomerName under req.IdempotencyKey (FR-005, FR-006, INV-003).
//
// An empty idempotency key is rejected before any statement, wrapping ErrInvalidRequest
// (ADR-0018). Otherwise the booking is inserted first, with no read before it (ADR-0011,
// ADR-0020 rejects a lookup on this path). Only when the store rejects the insert as not unique is
// the booking stored under the key read, in a separate autocommit statement (ADR-0012), and the
// outcome decided from it, never from which index fired:
//   - a booking with the same payload (Booking.SamePayload): that booking in its current state,
//     cancelled included (ADR-0002), with replayed = true; nothing new is stored;
//   - a booking with a different payload: ErrKeyConflict (ADR-0001);
//   - no booking under the key: ErrSlotTaken; nothing is stored under the key.
//
// A new booking is returned with replayed = false.
func (s *Service) Book(ctx context.Context, req Request) (b Booking, replayed bool, err error) {
	if req.IdempotencyKey == "" {
		return Booking{}, false, fmt.Errorf("%w: idempotency key is required", ErrInvalidRequest)
	}
	b = Booking{
		ID:             rand.Text(),
		ResourceID:     req.ResourceID,
		SlotStart:      CanonicalSlotStart(req.SlotStart),
		CustomerName:   req.CustomerName,
		IdempotencyKey: req.IdempotencyKey,
		Status:         StatusActive,
	}
	err = s.repo.InsertBooking(ctx, b)
	if err == nil {
		return b, false, nil
	}
	if !errors.Is(err, ErrNotUnique) {
		return Booking{}, false, fmt.Errorf("booking: insert: %w", err)
	}
	return s.classify(ctx, b)
}

// classify decides the outcome of a request whose insert was rejected as not unique, from the
// booking stored under its key (ADR-0011).
func (s *Service) classify(ctx context.Context, want Booking) (Booking, bool, error) {
	stored, err := s.repo.GetBookingByIdempotencyKey(ctx, want.IdempotencyKey)
	switch {
	case errors.Is(err, ErrNotFound):
		return Booking{}, false, ErrSlotTaken
	case err != nil:
		return Booking{}, false, fmt.Errorf("booking: classify a rejected insert: %w", err)
	case !stored.SamePayload(want):
		return Booking{}, false, ErrKeyConflict
	}
	return stored, true, nil
}

// Cancel cancels booking id and returns it (FR-007). Anyone who knows the id may cancel: no
// identity is checked (CON-006). The slot is free at once, because the active-slot index covers
// active bookings only (ADR-0008). Cancelling an already cancelled booking changes nothing and
// returns it again as a success (ADR-0003). An unknown id is ErrNotFound.
//
// The store's single UPDATE is the whole operation (ADR-0012): the target status is cancelled,
// which the transition rule (Booking.WithStatus) allows from every status, so no read precedes it.
func (s *Service) Cancel(ctx context.Context, id string) (Booking, error) {
	b, err := s.repo.UpdateBookingStatus(ctx, id, StatusCancelled)
	if err != nil {
		return Booking{}, fmt.Errorf("booking: cancel %q: %w", id, err)
	}
	return b, nil
}

// Get returns booking id in its current state, including a cancelled one, or ErrNotFound.
func (s *Service) Get(ctx context.Context, id string) (Booking, error) {
	b, err := s.repo.GetBooking(ctx, id)
	if err != nil {
		return Booking{}, fmt.Errorf("booking: get %q: %w", id, err)
	}
	return b, nil
}
