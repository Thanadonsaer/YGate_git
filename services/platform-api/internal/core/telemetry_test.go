package core

import (
	"testing"
	"time"
)

func TestTelemetryHistoryCursorRoundTrip(t *testing.T) {
	reading := LatestTelemetry{
		ID:         "018f8318-4c72-7ad4-9f57-2894d89cb123",
		ObservedAt: time.Date(2026, 7, 27, 3, 0, 0, 123000000, time.UTC),
		ReceivedAt: time.Date(2026, 7, 27, 3, 0, 1, 0, time.UTC),
	}
	cursor, err := decodeTelemetryHistoryCursor(encodeTelemetryHistoryCursor(reading))
	if err != nil || cursor.ID != reading.ID || !cursor.ObservedAt.Equal(reading.ObservedAt) || !cursor.ReceivedAt.Equal(reading.ReceivedAt) {
		t.Fatalf("cursor=%+v err=%v", cursor, err)
	}
	if _, err = decodeTelemetryHistoryCursor("not-base64!"); err == nil {
		t.Fatal("invalid cursor must fail")
	}
}
