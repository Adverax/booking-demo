package store

import "time"

// toEpoch is the only way an instant enters the database: integer seconds
// since the Unix epoch, UTC (ADR-0013, CON-008). The location of t is
// irrelevant; a sub-second part is dropped (floor), as stored instants are
// whole seconds. Slot starts are half-hour aligned before they get here.
func toEpoch(t time.Time) int64 {
	return t.Unix()
}

// fromEpoch is the only way an instant leaves the database: the stored epoch
// seconds as a UTC time.
func fromEpoch(sec int64) time.Time {
	return time.Unix(sec, 0).UTC()
}
