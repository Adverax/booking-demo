// Package catalog is the Resource Catalog component (COMP-002): the Resource aggregate (AGG-001)
// with its weekly working hours (ADR-0004), its creation, and reading resources back.
//
// The catalog is the single owner of the working-hours rules (INV-005, ASM-0007): NewResource is
// the only way a new resource comes into being, and it refuses any shape ADR-0004 does not allow.
// Other components read working hours through Get. The store's schema constraints are a backstop
// behind these rules, not the contract.
package catalog

import (
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"time"
)

// EndOfDay is the end of a UTC day in minutes since 00:00 UTC: an interval end of 24:00 (ADR-0004).
const EndOfDay = 24 * 60

// alignment is the half hour every interval bound lies on, in minutes (ADR-0004).
const alignment = 30

var (
	// ErrInvalidResource is wrapped by every rejection of NewResource: the resource would violate
	// INV-005. The wrapping error says which rule.
	ErrInvalidResource = errors.New("invalid resource")
	// ErrNotFound is returned (wrapped) when no resource has the requested id.
	ErrNotFound = errors.New("resource not found")
)

// Interval is the working interval of one weekday in UTC, half-open: [Start, End). Bounds are
// minutes since 00:00 UTC; End = EndOfDay is the end of the UTC day (ADR-0004).
type Interval struct {
	Weekday time.Weekday
	Start   int
	End     int
}

// Resource is a bookable resource with a name and weekly working hours. A Resource built by
// NewResource, or read back from the Repository, satisfies INV-005 and keeps WeeklyHours ordered
// by weekday (Sunday first).
type Resource struct {
	ID          string
	Name        string
	WeeklyHours []Interval
}

// NewResource returns the resource id named name with the given weekly hours, or an error wrapping
// ErrInvalidResource when it would violate INV-005 (ADR-0004):
//   - the name is not empty, and there is at least one interval;
//   - each weekday is a valid time.Weekday and has at most one interval;
//   - both bounds lie on the half hour;
//   - each interval lies inside one UTC day: 0 <= Start < End <= EndOfDay, so a start of 24:00,
//     a bound past 24:00 and an interval crossing midnight are all rejected.
//
// The hours are copied and ordered by weekday; the caller's slice is not kept.
func NewResource(id, name string, hours []Interval) (Resource, error) {
	if name == "" {
		return Resource{}, fmt.Errorf("%w: name is required", ErrInvalidResource)
	}
	if len(hours) == 0 {
		return Resource{}, fmt.Errorf("%w: weekly working hours are required", ErrInvalidResource)
	}
	var seen [7]bool
	for _, iv := range hours {
		if err := iv.validate(); err != nil {
			return Resource{}, fmt.Errorf("%w: %w", ErrInvalidResource, err)
		}
		if seen[iv.Weekday] {
			return Resource{}, fmt.Errorf("%w: more than one interval on %s", ErrInvalidResource, iv.Weekday)
		}
		seen[iv.Weekday] = true
	}
	sorted := slices.Clone(hours)
	slices.SortFunc(sorted, func(a, b Interval) int { return cmp.Compare(a.Weekday, b.Weekday) })
	return Resource{ID: id, Name: name, WeeklyHours: sorted}, nil
}

// validate checks the rules of ADR-0004 that concern one interval alone.
func (iv Interval) validate() error {
	if iv.Weekday < time.Sunday || iv.Weekday > time.Saturday {
		return fmt.Errorf("unknown weekday %d", int(iv.Weekday))
	}
	if iv.Start%alignment != 0 || iv.End%alignment != 0 {
		return fmt.Errorf("%s: bounds %s-%s are not on the half hour", iv.Weekday, clock(iv.Start), clock(iv.End))
	}
	if iv.Start < 0 || iv.Start >= EndOfDay {
		return fmt.Errorf("%s: start %s is not between 00:00 and 23:30", iv.Weekday, clock(iv.Start))
	}
	if iv.End > EndOfDay {
		return fmt.Errorf("%s: end %s is past 24:00", iv.Weekday, clock(iv.End))
	}
	if iv.End <= iv.Start {
		return fmt.Errorf("%s: end %s is not after start %s; an interval may not cross midnight UTC",
			iv.Weekday, clock(iv.End), clock(iv.Start))
	}
	return nil
}

// clock renders minutes since 00:00 as HH:MM for error messages.
func clock(min int) string {
	sign := ""
	if min < 0 {
		sign, min = "-", -min
	}
	return fmt.Sprintf("%s%02d:%02d", sign, min/60, min%60)
}

// Repository persists resources. It is the port of this component to the store (COMP-005).
type Repository interface {
	// InsertResource stores r and all of its working hours atomically: all or nothing.
	InsertResource(ctx context.Context, r Resource) error
	// ListResources returns every stored resource, in creation order.
	ListResources(ctx context.Context) ([]Resource, error)
	// GetResource returns the resource with the given id, or an error wrapping ErrNotFound.
	GetResource(ctx context.Context, id string) (Resource, error)
}

// Catalog creates and reads resources.
type Catalog struct {
	repo Repository
}

// New returns a Catalog storing through repo.
func New(repo Repository) *Catalog {
	return &Catalog{repo: repo}
}

// Create validates and persists a resource named name with the given weekly hours and returns it
// with its generated id. A violation of INV-005 is returned wrapping ErrInvalidResource, and then
// nothing is stored.
func (c *Catalog) Create(ctx context.Context, name string, hours []Interval) (Resource, error) {
	r, err := NewResource(rand.Text(), name, hours)
	if err != nil {
		return Resource{}, err
	}
	if err := c.repo.InsertResource(ctx, r); err != nil {
		return Resource{}, fmt.Errorf("catalog: insert resource: %w", err)
	}
	return r, nil
}

// List returns every resource, in creation order.
func (c *Catalog) List(ctx context.Context) ([]Resource, error) {
	rs, err := c.repo.ListResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog: list resources: %w", err)
	}
	return rs, nil
}

// Get returns the resource with the given id, or an error wrapping ErrNotFound.
func (c *Catalog) Get(ctx context.Context, id string) (Resource, error) {
	r, err := c.repo.GetResource(ctx, id)
	if err != nil {
		return Resource{}, fmt.Errorf("catalog: get resource %q: %w", id, err)
	}
	return r, nil
}
