package dbgen

import (
	"strings"
	"testing"
)

func TestInsertTelemetryReadingUsesPartitionUniqueKey(t *testing.T) {
	want := "ON CONFLICT (middleware_client_id, external_key, observed_at) DO NOTHING"
	if !strings.Contains(insertTelemetryReading, want) {
		t.Fatalf("InsertTelemetryReading conflict target does not match telemetry_reading partition constraint: want %q", want)
	}
}
