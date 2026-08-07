package ingestion

import (
	"context"
	"os"
	"testing"
	"time"

	"ygate/platform-api/internal/testdb"
)

// A fully migrated database must still accept the INSERT an older platform-api
// binary writes, because migrations are applied at service startup: between
// "the new binary migrated the database" and "no old binary is running any
// more" both versions share one schema. Dropping raw_payload broke exactly
// that window in production --
//
//	create raw ingest batch: ERROR: column "raw_payload" of relation
//	"telemetry_ingest_batch" does not exist (SQLSTATE 42703)
//
// -- and every ingest failed until the rollout finished. The column is now
// kept with a '{}' default (000039) and restored where it had already been
// dropped (000041).
func TestMigratedSchemaStillAcceptsPreviousBinaryIngestInsert(t *testing.T) {
	databaseURL := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := testdb.Disposable(t, ctx, databaseURL)

	organizationID, _ := newUUID()
	clientID, _ := newUUID()
	if _, err := pool.Exec(ctx, "INSERT INTO organization(id,code,name) VALUES($1,'COMPAT','Compat Test')", organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO auth.middleware_client(id,organization_id,name,key_prefix,key_hash,auto_onboard)
VALUES($1,$2,'Compat Gateway','ygm_compat01','\x01'::bytea,true)`, clientID, organizationID); err != nil {
		t.Fatal(err)
	}

	batchID, _ := newUUID()
	// Verbatim shape of the pre-000039 CreateOrGetIngestBatch insert.
	if _, err := pool.Exec(ctx, `
INSERT INTO telemetry.telemetry_ingest_batch (
    id, organization_id, middleware_client_id, idempotency_key, payload_hash, raw_payload
) VALUES ($1,$2,$3,'compat-key','\x02'::bytea,'{"schemaVersion":"2.0"}'::jsonb)`,
		batchID, organizationID, clientID); err != nil {
		t.Fatalf("a previous binary's ingest insert must still succeed against the migrated schema: %v", err)
	}

	// And the current insert, which never mentions the column, must still work
	// and simply take the default.
	currentBatchID, _ := newUUID()
	if _, err := pool.Exec(ctx, `
INSERT INTO telemetry.telemetry_ingest_batch (
    id, organization_id, middleware_client_id, idempotency_key, payload_hash
) VALUES ($1,$2,$3,'current-key','\x03'::bytea)`,
		currentBatchID, organizationID, clientID); err != nil {
		t.Fatalf("current ingest insert failed: %v", err)
	}
	var stored string
	if err := pool.QueryRow(ctx,
		`SELECT raw_payload::text FROM telemetry.telemetry_ingest_batch WHERE id=$1`, currentBatchID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "{}" {
		t.Fatalf("raw_payload = %s, want {} -- the current path must not store a second copy of the batch", stored)
	}
}
