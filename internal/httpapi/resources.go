package httpapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/adverax/booking-demo/internal/catalog"
)

// CreateResource persists a resource and answers 201 with its generated id (FR-001, AC-001).
//
// Only what is needed to translate the wire format is checked here: a weekday from the enum and
// bounds of the form HH:MM. The working-hours rules of INV-005 are CARD-006's.
func (s *Server) CreateResource(ctx context.Context, req CreateResourceRequestObject) (CreateResourceResponseObject, error) {
	hours := make([]catalog.Interval, 0, len(req.Body.WeeklyHours))
	for _, wi := range req.Body.WeeklyHours {
		iv, err := intervalFromAPI(wi)
		if err != nil {
			return CreateResource400JSONResponse{ValidationErrorJSONResponse{Code: ErrorCodeValidationError, Message: err.Error()}}, nil
		}
		hours = append(hours, iv)
	}
	r, err := s.catalog.Create(ctx, req.Body.Name, hours)
	if err != nil {
		return nil, err
	}
	return CreateResource201JSONResponse(resourceToAPI(r)), nil
}

var weekdays = map[Weekday]time.Weekday{
	WeekdaySunday:    time.Sunday,
	WeekdayMonday:    time.Monday,
	WeekdayTuesday:   time.Tuesday,
	WeekdayWednesday: time.Wednesday,
	WeekdayThursday:  time.Thursday,
	WeekdayFriday:    time.Friday,
	WeekdaySaturday:  time.Saturday,
}

func intervalFromAPI(wi WorkingInterval) (catalog.Interval, error) {
	day, ok := weekdays[wi.Weekday]
	if !ok {
		return catalog.Interval{}, fmt.Errorf("unknown weekday %q", wi.Weekday)
	}
	start, err := minutesFromAPI(wi.Start)
	if err != nil {
		return catalog.Interval{}, err
	}
	end, err := minutesFromAPI(wi.End)
	if err != nil {
		return catalog.Interval{}, err
	}
	return catalog.Interval{Weekday: day, Start: start, End: end}, nil
}

// minutesFromAPI reads a time of day "HH:MM" as minutes since 00:00 UTC. "24:00" reads as 1440.
func minutesFromAPI(s string) (int, error) {
	digit := func(i int) (int, bool) { return int(s[i] - '0'), s[i] >= '0' && s[i] <= '9' }
	if len(s) != 5 || s[2] != ':' {
		return 0, fmt.Errorf("time of day %q is not HH:MM", s)
	}
	h1, ok1 := digit(0)
	h2, ok2 := digit(1)
	m1, ok3 := digit(3)
	m2, ok4 := digit(4)
	h, m := h1*10+h2, m1*10+m2
	if !ok1 || !ok2 || !ok3 || !ok4 || h > 24 || m > 59 {
		return 0, fmt.Errorf("time of day %q is not HH:MM", s)
	}
	return h*60 + m, nil
}

func resourceToAPI(r catalog.Resource) Resource {
	hours := make(WeeklyHours, 0, len(r.WeeklyHours))
	for _, iv := range r.WeeklyHours {
		hours = append(hours, WorkingInterval{
			Weekday: Weekday(strings.ToLower(iv.Weekday.String())),
			Start:   minutesToAPI(iv.Start),
			End:     minutesToAPI(iv.End),
		})
	}
	return Resource{Id: r.ID, Name: r.Name, WeeklyHours: hours}
}

func minutesToAPI(min int) string {
	return fmt.Sprintf("%02d:%02d", min/60, min%60)
}
