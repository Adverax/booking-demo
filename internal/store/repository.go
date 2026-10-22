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
