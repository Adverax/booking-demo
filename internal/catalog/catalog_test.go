package catalog

import (
	"context"
	"errors"
	"reflect"
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

func (r *fakeRepo) ListResources(context.Context) ([]Resource, error) {
	return r.saved, r.err
}

func (r *fakeRepo) GetResource(_ context.Context, id string) (Resource, error) {
	if r.err != nil {
		return Resource{}, r.err
	}
	for _, res := range r.saved {
		if res.ID == id {
			return res, nil
		}
	}
	return Resource{}, ErrNotFound
}

var monToFri = []Interval{
	{Weekday: time.Monday, Start: 540, End: 1020},
	{Weekday: time.Tuesday, Start: 540, End: 1020},
	{Weekday: time.Wednesday, Start: 540, End: 1020},
	{Weekday: time.Thursday, Start: 540, End: 1020},
	{Weekday: time.Friday, Start: 540, End: 1020},
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
	if _, err := New(&fakeRepo{err: boom}).Create(context.Background(), "Dr. Lee", monToFri); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

// Every shape INV-005 forbids is rejected as ErrInvalidResource and never reaches the repository.
func TestCatalog_CreateRejectsInvalidResourcesWithoutStoring(t *testing.T) {
	mon := func(start, end int) []Interval { return []Interval{{Weekday: time.Monday, Start: start, End: end}} }
	cases := map[string]struct {
		name  string
		hours []Interval
	}{
		"empty name":                {"", monToFri},
		"no working hours":          {"Dr. Lee", nil},
		"empty working hours":       {"Dr. Lee", []Interval{}},
		"two intervals on Monday":   {"Dr. Lee", []Interval{{time.Monday, 540, 720}, {time.Monday, 780, 1020}}},
		"misaligned start 09:15":    {"Dr. Lee", mon(555, 1020)},
		"misaligned end 17:01":      {"Dr. Lee", mon(540, 1021)},
		"crossing midnight":         {"Dr. Lee", mon(1320, 120)},
		"empty interval":            {"Dr. Lee", mon(540, 540)},
		"start 24:00":               {"Dr. Lee", mon(1440, 1470)},
		"end 24:30":                 {"Dr. Lee", mon(1380, 1470)},
		"negative start":            {"Dr. Lee", mon(-30, 60)},
		"weekday below Sunday":      {"Dr. Lee", []Interval{{-1, 540, 1020}}},
		"weekday above Saturday":    {"Dr. Lee", []Interval{{7, 540, 1020}}},
		"one bad among good ones":   {"Dr. Lee", append(monToFri[1:], Interval{time.Monday, 540, 1500})},
		"duplicate after other day": {"Dr. Lee", []Interval{{time.Monday, 540, 600}, {time.Friday, 540, 600}, {time.Monday, 600, 660}}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepo{}
			_, err := New(repo).Create(context.Background(), c.name, c.hours)
			if !errors.Is(err, ErrInvalidResource) {
				t.Fatalf("err = %v, want ErrInvalidResource", err)
			}
			if len(repo.saved) != 0 {
				t.Fatalf("stored %+v", repo.saved)
			}
		})
	}
}

// The edges ADR-0004 allows: a whole day 00:00-24:00, an end of 24:00, a last slot 23:30-24:00,
// and all seven weekdays.
func TestNewResource_AcceptsTheAllowedEdges(t *testing.T) {
	week := make([]Interval, 0, 7)
	for d := time.Sunday; d <= time.Saturday; d++ {
		week = append(week, Interval{Weekday: d, Start: 0, End: EndOfDay})
	}
	for name, hours := range map[string][]Interval{
		"whole day":       {{time.Monday, 0, EndOfDay}},
		"ends at 24:00":   {{time.Monday, 1320, EndOfDay}},
		"last slot":       {{time.Monday, 1410, EndOfDay}},
		"first slot":      {{time.Monday, 0, 30}},
		"all seven days":  week,
		"Mon-Fri 9 to 17": monToFri,
	} {
		if _, err := NewResource("r1", "Dr. Lee", hours); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// The hours are kept ordered by weekday and are a copy: changing the caller's slice afterwards
// does not change the resource.
func TestNewResource_OrdersHoursByWeekdayAndCopiesThem(t *testing.T) {
	hours := []Interval{{time.Friday, 540, 600}, {time.Sunday, 0, 60}, {time.Monday, 540, 600}}
	r, err := NewResource("r1", "Dr. Lee", hours)
	if err != nil {
		t.Fatal(err)
	}
	want := []Interval{{time.Sunday, 0, 60}, {time.Monday, 540, 600}, {time.Friday, 540, 600}}
	if !reflect.DeepEqual(r.WeeklyHours, want) {
		t.Fatalf("hours = %+v, want %+v", r.WeeklyHours, want)
	}
	hours[0].Start = 30
	if !reflect.DeepEqual(r.WeeklyHours, want) {
		t.Fatal("the resource shares the caller's slice")
	}
}

func TestCatalog_ListAndGetReadThroughTheRepository(t *testing.T) {
	repo := &fakeRepo{}
	c := New(repo)
	r, err := c.Create(context.Background(), "Room A", monToFri)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Get(context.Background(), r.ID)
	if err != nil || !reflect.DeepEqual(got, r) {
		t.Fatalf("Get = %+v, %v; want %+v", got, err, r)
	}
	if _, err := c.Get(context.Background(), "missing-id"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get unknown: err = %v, want ErrNotFound", err)
	}
	list, err := c.List(context.Background())
	if err != nil || len(list) != 1 || list[0].ID != r.ID {
		t.Fatalf("List = %+v, %v", list, err)
	}
	boom := errors.New("disk I/O error")
	repo.err = boom
	if _, err := c.List(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("List: err = %v, want %v", err, boom)
	}
	if _, err := c.Get(context.Background(), r.ID); !errors.Is(err, boom) {
		t.Fatalf("Get: err = %v, want %v", err, boom)
	}
}
