package catalog

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepo struct {
	err   error
	saved []Resource
}

func (r *fakeRepo) InsertResource(_ context.Context, res Resource) error {
	if r.err != nil {
		return r.err
	}
	r.saved = append(r.saved, res)
	return nil
}

func TestCatalog_CreateAssignsAnIDAndStoresTheResource(t *testing.T) {
	repo := &fakeRepo{}
	hours := []Interval{{Weekday: time.Monday, Start: 540, End: 1020}}
	r, err := New(repo).Create(context.Background(), "Dr. Lee", hours)
	if err != nil {
		t.Fatal(err)
	}
	if r.ID == "" || r.Name != "Dr. Lee" || len(r.WeeklyHours) != 1 || r.WeeklyHours[0] != hours[0] {
		t.Fatalf("created %+v", r)
	}
	if len(repo.saved) != 1 || repo.saved[0].ID != r.ID {
		t.Fatalf("saved %+v", repo.saved)
	}
	r2, _ := New(repo).Create(context.Background(), "Dr. Lee", hours)
	if r2.ID == r.ID {
		t.Error("two resources got the same id")
	}
}

func TestCatalog_CreateReturnsTheStoreError(t *testing.T) {
	boom := errors.New("disk I/O error")
	if _, err := New(&fakeRepo{err: boom}).Create(context.Background(), "Dr. Lee", nil); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}
