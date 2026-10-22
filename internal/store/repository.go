package store

import (
	"context"
	"fmt"

	"github.com/adverax/booking-demo/internal/booking"
	"github.com/adverax/booking-demo/internal/catalog"
)

// The store is the adapter of the ports declared by the components that consume it.
var (
	_ booking.Repository = (*Store)(nil)
	_ catalog.Repository = (*Store)(nil)
)

// InsertBooking stores b with one autocommit INSERT: no transaction and no read before it
// (ADR-0012, CON-007). The partial unique index on the active slot and the unique index on the
// idempotency key decide whether it is accepted (ADR-0008); a rejection by either is returned
// wrapping booking.ErrNotUnique. The insert waits at most busyTimeout for the write lock.
func (s *Store) InsertBooking(ctx context.Context, b booking.Booking) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO booking (id, resource_id, slot_start, customer_name, idempotency_key, status)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		b.ID, b.ResourceID, toEpoch(b.SlotStart), b.CustomerName, b.IdempotencyKey, string(b.Status))
	if IsUniqueViolation(err) {
		return fmt.Errorf("store: insert booking: %w", booking.ErrNotUnique)
	}
	if err != nil {
		return fmt.Errorf("store: insert booking: %w", err)
	}
	return nil
}

// InsertResource stores r and its working hours in one transaction, so a resource never exists
// without its hours. The transaction only writes: its first statement is an INSERT, so it takes
// the write lock up front (waiting at most busyTimeout) and never upgrades a read snapshot.
func (s *Store) InsertResource(ctx context.Context, r catalog.Resource) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: insert resource: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.ExecContext(ctx, "INSERT INTO resource (id, name) VALUES (?, ?)", r.ID, r.Name); err != nil {
		return fmt.Errorf("store: insert resource: %w", err)
	}
	for _, iv := range r.WeeklyHours {
		if _, err = tx.ExecContext(ctx,
			"INSERT INTO resource_working_hours (resource_id, weekday, start_minute, end_minute) VALUES (?, ?, ?, ?)",
			r.ID, int(iv.Weekday), iv.Start, iv.End); err != nil {
			return fmt.Errorf("store: insert working hours of %s: %w", iv.Weekday, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("store: insert resource: %w", err)
	}
	return nil
}

// resourceRows selects every resource with its working hours, one row per interval, resources in
// creation order (rowid) and each resource's intervals by weekday. A resource always has at least
// one interval (InsertResource is all or nothing, INV-005), so the inner join loses none.
const resourceRows = `SELECT r.id, r.name, h.weekday, h.start_minute, h.end_minute
	FROM resource r JOIN resource_working_hours h ON h.resource_id = r.id`

// ListResources returns every stored resource with its working hours, in creation order. It is a
// single statement, so it reads one consistent snapshot.
func (s *Store) ListResources(ctx context.Context) ([]catalog.Resource, error) {
	rs, err := s.queryResources(ctx, resourceRows+" ORDER BY r.rowid, h.weekday")
	if err != nil {
		return nil, fmt.Errorf("store: list resources: %w", err)
	}
	return rs, nil
}

// GetResource returns the resource with the given id and its working hours, or an error wrapping
// catalog.ErrNotFound.
func (s *Store) GetResource(ctx context.Context, id string) (catalog.Resource, error) {
	rs, err := s.queryResources(ctx, resourceRows+" WHERE r.id = ? ORDER BY h.weekday", id)
	if err != nil {
		return catalog.Resource{}, fmt.Errorf("store: get resource: %w", err)
	}
	if len(rs) == 0 {
		return catalog.Resource{}, fmt.Errorf("store: get resource: %w", catalog.ErrNotFound)
	}
	return rs[0], nil
}

// queryResources runs a resourceRows query and folds consecutive rows of one resource into one
// catalog.Resource. The result is never nil.
func (s *Store) queryResources(ctx context.Context, query string, args ...any) ([]catalog.Resource, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rs := []catalog.Resource{}
	for rows.Next() {
		var (
			id, name string
			iv       catalog.Interval
		)
		if err := rows.Scan(&id, &name, &iv.Weekday, &iv.Start, &iv.End); err != nil {
			return nil, err
		}
		if n := len(rs); n == 0 || rs[n-1].ID != id {
			rs = append(rs, catalog.Resource{ID: id, Name: name})
		}
		last := &rs[len(rs)-1]
		last.WeeklyHours = append(last.WeeklyHours, iv)
	}
	return rs, rows.Err()
}
