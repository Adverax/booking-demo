// Package availability is the Availability Search component (COMP-003): a read-only projection
// that lists the free slots of one resource over a range of UTC days (FR-003, FR-004).
//
// A free slot is 30 minutes long, starts on the UTC half hour, lies inside the resource's working
// hours and does not overlap an active booking; cancelled bookings do not occupy their slot.
// Working hours are read through the Resource Catalog (COMP-002, ASM-0007, DEC-015 open); active
// bookings through the Bookings port, implemented by the store (COMP-005).
//
// Slots are generated, not tested: each working interval is enumerated in half-hour steps from its
// Start up to its End. The catalog guarantees both bounds lie on the half hour (INV-005), so the
// enumeration yields exactly the slots inside the interval and no containment predicate is needed.
// All arithmetic is on integer UTC epoch seconds (ADR-0013); the package never reads the current
// time, so past slots are listed like any other (ASM-0002).
package availability

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/adverax/booking-demo/internal/catalog"
)

// SlotLength is the length of every slot (FR-003). It equals booking.SlotLength; a test pins that,
// since this component does not depend on Slot Booking.
const SlotLength = 30 * time.Minute

// MaxRangeDays is the longest range, in UTC days counting both ends (FR-004).
const MaxRangeDays = 14

const (
	daySeconds  = int64(24 * time.Hour / time.Second)
	slotSeconds = int64(SlotLength / time.Second)
	slotMinutes = int(SlotLength / time.Minute)
)

var (
	// ErrInvalidRange is wrapped when the range starts after it ends or covers more than
	// MaxRangeDays days (FR-004, ADR-0018). It is returned before anything is read.
	ErrInvalidRange = errors.New("availability: invalid range")
	// ErrUnknownResource is wrapped when no resource has the requested id. No booking is read.
	ErrUnknownResource = errors.New("availability: unknown resource")
)

// Slot is one free slot, [Start, End), both UTC.
type Slot struct {
	Start time.Time
	End   time.Time
}

// Resources is the port of this component to the Resource Catalog (COMP-002): the only way it
// reads working hours (ASM-0007). *catalog.Catalog implements it.
type Resources interface {
	// Get returns the resource with the given id, or an error wrapping catalog.ErrNotFound.
	Get(ctx context.Context, id string) (catalog.Resource, error)
}

// Bookings is the port of this component to the store (COMP-005).
type Bookings interface {
	// ActiveBookingStarts returns the slot starts of the active bookings of resourceID whose start
	// lies in [from, to), with ONE autocommit read (ADR-0012). Cancelled bookings are not returned.
	ActiveBookingStarts(ctx context.Context, resourceID string, from, to time.Time) ([]time.Time, error)
}

// Service answers availability queries.
type Service struct {
	resources Resources
	bookings  Bookings
}

// NewService returns a Service reading working hours through resources and active bookings
// through bookings.
func NewService(resources Resources, bookings Bookings) *Service {
	return &Service{resources: resources, bookings: bookings}
}

// FreeSlots returns the free slots of resourceID for the UTC days of from through to, both
// inclusive, ordered by start. Only the UTC calendar day of from and of to counts; their time of
// day and location do not.
//
// The order is: the range, then the resource, then its bookings. A range whose from is after its
// to, or that covers more than MaxRangeDays days, wraps ErrInvalidRange before anything is read
// (ADR-0018, AC-054); an unknown resource wraps ErrUnknownResource before any booking is read.
func (s *Service) FreeSlots(ctx context.Context, resourceID string, from, to time.Time) ([]Slot, error) {
	first, last := utcDay(from), utcDay(to)
	if first > last {
		return nil, fmt.Errorf("%w: from %s is after to %s", ErrInvalidRange, dayString(first), dayString(last))
	}
	if days := (last-first)/daySeconds + 1; days > MaxRangeDays {
		return nil, fmt.Errorf("%w: %d days from %s to %s, at most %d allowed", ErrInvalidRange, days, dayString(first), dayString(last), MaxRangeDays)
	}
	r, err := s.resources.Get(ctx, resourceID)
	switch {
	case errors.Is(err, catalog.ErrNotFound):
		return nil, fmt.Errorf("%w: %w", ErrUnknownResource, err)
	case err != nil:
		return nil, fmt.Errorf("availability: read resource: %w", err)
	}
	end := last + daySeconds
	starts, err := s.bookings.ActiveBookingStarts(ctx, resourceID, time.Unix(first, 0).UTC(), time.Unix(end, 0).UTC())
	if err != nil {
		return nil, fmt.Errorf("availability: read active bookings: %w", err)
	}
	booked := make(map[int64]bool, len(starts))
	for _, t := range starts {
		booked[t.Unix()] = true
	}
	var hours [7]*catalog.Interval
	for i := range r.WeeklyHours {
		iv := &r.WeeklyHours[i]
		hours[iv.Weekday] = iv
	}
	slots := []Slot{}
	for day := first; day < end; day += daySeconds {
		iv := hours[weekday(day)]
		if iv == nil {
			continue
		}
		// INV-005: Start and End lie on the half hour, so every step is a whole slot inside
		// [Start, End).
		for m := iv.Start; m < iv.End; m += slotMinutes {
			start := day + int64(m)*60
			if booked[start] {
				continue
			}
			slots = append(slots, Slot{Start: time.Unix(start, 0).UTC(), End: time.Unix(start+slotSeconds, 0).UTC()})
		}
	}
	return slots, nil
}

// utcDay returns the epoch second of 00:00 UTC of the UTC day containing t (floor, also before
// 1970).
func utcDay(t time.Time) int64 {
	sec := t.Unix()
	rem := sec % daySeconds
	if rem < 0 {
		rem += daySeconds
	}
	return sec - rem
}

// weekday returns the UTC weekday of the day starting at epoch second day: 1970-01-01 was a
// Thursday.
func weekday(day int64) time.Weekday {
	w := (day/daySeconds + int64(time.Thursday)) % 7
	if w < 0 {
		w += 7
	}
	return time.Weekday(w)
}

func dayString(day int64) string {
	return time.Unix(day, 0).UTC().Format(time.DateOnly)
}
