package dbgen

import (
	"strings"
	"testing"
)

// raw_register_reading is partitioned by observed_at, so Postgres forces its
// dedup UNIQUE constraint to include the partition key. The insert's conflict
// target has to match that constraint exactly or a retried batch inserts
// duplicate readings instead of being skipped.
func TestInsertRawRegisterReadingUsesPartitionUniqueKey(t *testing.T) {
	want := "ON CONFLICT (middleware_client_id, external_key, observed_at) DO NOTHING"
	if !strings.Contains(insertRawRegisterReading, want) {
		t.Fatalf("InsertRawRegisterReading conflict target does not match raw_register_reading partition constraint: want %q", want)
	}
}

// The ingest batch row stores only the payload hash. Re-adding the payload
// itself would put a second verbatim copy of every reading in the database
// (see migration 000039).
func TestCreateOrGetIngestBatchStoresHashNotPayload(t *testing.T) {
	if strings.Contains(createOrGetIngestBatch, "raw_payload") {
		t.Fatal("CreateOrGetIngestBatch must not store raw_payload")
	}
	if !strings.Contains(createOrGetIngestBatch, "payload_hash") {
		t.Fatal("CreateOrGetIngestBatch must still store payload_hash for idempotency conflict detection")
	}
}
