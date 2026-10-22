// Package catalog is the Resource Catalog component (COMP-002): the Resource aggregate (AGG-001)
// with its weekly working hours (ADR-0004), and its creation.
//
// This is the minimal creation path the walking skeleton needs so that a slot can be booked. The
// full working-hours validation of INV-005 (alignment, one interval per weekday, the 24:00 rules)
// is CARD-006's; until then the store's schema constraints are the only backstop.
package catalog

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"
)

// Interval is the working interval of one weekday in UTC, half-open: [Start, End). Bounds are
// minutes since 00:00 UTC; End = 1440 is the end of the UTC day (ADR-0004).
type Interval struct {
	Weekday time.Weekday
	Start   int
	End     int
}

// Resource is a bookable resource with a name and weekly working hours.
type Resource struct {
	ID          string
	Name        string
	WeeklyHours []Interval
}

// Repository persists resources. It is the port of this component to the store (COMP-005).
type Repository interface {
	// InsertResource stores r and all of its working hours atomically: all or nothing.
	InsertResource(ctx context.Context, r Resource) error
}

// Catalog creates resources.
type Catalog struct {
	repo Repository
}

// New returns a Catalog storing through repo.
func New(repo Repository) *Catalog {
	return &Catalog{repo: repo}
}

// Create persists a resource named name with the given weekly hours and returns it with its
// generated id.
func (c *Catalog) Create(ctx context.Context, name string, hours []Interval) (Resource, error) {
	r := Resource{ID: rand.Text(), Name: name, WeeklyHours: hours}
	if err := c.repo.InsertResource(ctx, r); err != nil {
		return Resource{}, fmt.Errorf("catalog: insert resource: %w", err)
	}
	return r, nil
}
