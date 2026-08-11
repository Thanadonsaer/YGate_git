package core

import (
	"errors"
	"testing"
	"time"
)

func timePtr(value time.Time) *time.Time { return &value }

func TestValidateEventLogbookInput(t *testing.T) {
	start := time.Date(2026, time.August, 11, 8, 0, 0, 0, time.UTC)
	if _, err := validateEventLogbookInput(CreateEventLogbookInput{EventType: "MAINTENANCE", Title: "Inverter inspection", StartsAt: start, EndsAt: timePtr(start.Add(time.Hour))}); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}
	for _, input := range []CreateEventLogbookInput{
		{EventType: "UNKNOWN", Title: "Event", StartsAt: start},
		{EventType: "FAULT", Title: "", StartsAt: start},
		{EventType: "FAULT", Title: "Event", StartsAt: start, EndsAt: timePtr(start.Add(-time.Minute))},
	} {
		if _, err := validateEventLogbookInput(input); !errors.Is(err, ErrEventLogbookInvalid) {
			t.Fatalf("input %#v error = %v, want ErrEventLogbookInvalid", input, err)
		}
	}
}
