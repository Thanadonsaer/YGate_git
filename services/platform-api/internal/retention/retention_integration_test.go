package retention

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ygate/platform-api/internal/testdb"
)

// Exercises telemetry.apply_retention (migration 000040) against real
// PostgreSQL: the partition-drop path, the delete path for the partition the
// cutoff falls inside, and the ingest-batch rule that must not trip the
// readings' foreign key.
func TestApplyRetentionAgainstPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool := testdb.Disposable(t, ctx, databaseURL)

	now := time.Now().UTC()
	fixture(t, ctx, pool, now)

	// Batch "old-empty" only ever held expired readings, so it must go.
	// Batch "old-backlog" was received long ago but still holds a reading
	// inside the window -- a Middleware working off a backlog produces exactly
	// this -- so deleting it would violate raw_register_reading's FK.
	var droppedPartitions int32
	var deletedReadings, deletedBatches int64
	if err := pool.QueryRow(ctx,
		`SELECT dropped_partitions, deleted_readings, deleted_batches FROM telemetry.apply_retention($1)`,
		7*24*time.Hour,
	).Scan(&droppedPartitions, &deletedReadings, &deletedBatches); err != nil {
		t.Fatalf("apply_retention: %v", err)
	}

	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM telemetry.raw_register_reading`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 2 {
		t.Fatalf("readings remaining = %d, want 2 (the two inside the window)", remaining)
	}
	var oldestRemaining time.Time
	if err := pool.QueryRow(ctx, `SELECT min(observed_at) FROM telemetry.raw_register_reading`).Scan(&oldestRemaining); err != nil {
		t.Fatal(err)
	}
	if oldestRemaining.Before(now.Add(-7 * 24 * time.Hour)) {
		t.Fatalf("oldest surviving reading %s is older than the 7d window", oldestRemaining)
	}

	var batches []string
	rows, err := pool.Query(ctx, `SELECT idempotency_key FROM telemetry.telemetry_ingest_batch ORDER BY idempotency_key`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		batches = append(batches, key)
	}
	rows.Close()
	if len(batches) != 2 || batches[0] != "old-backlog" || batches[1] != "recent" {
		t.Fatalf("batches remaining = %v, want [old-backlog recent]: old-backlog still has a reading in range, so deleting it would break the FK", batches)
	}
	if deletedBatches != 1 {
		t.Fatalf("deleted_batches = %d, want 1 (only the batch with no surviving readings)", deletedBatches)
	}
	if dropped := int(droppedPartitions); dropped < 0 {
		t.Fatalf("dropped_partitions = %d", dropped)
	}
	// Whether an expired reading left via DROP or DELETE depends on where the
	// cutoff falls in the month, so assert the total, not the split.
	if int64(droppedPartitions)+deletedReadings == 0 {
		t.Fatal("retention reclaimed nothing, but three readings were outside the window")
	}

	// Re-running must be a no-op, not an error or a second round of deletes.
	if err = pool.QueryRow(ctx,
		`SELECT dropped_partitions, deleted_readings, deleted_batches FROM telemetry.apply_retention($1)`,
		7*24*time.Hour,
	).Scan(&droppedPartitions, &deletedReadings, &deletedBatches); err != nil {
		t.Fatalf("second apply_retention: %v", err)
	}
	if deletedReadings != 0 || deletedBatches != 0 {
		t.Fatalf("second sweep deleted readings=%d batches=%d, want 0/0", deletedReadings, deletedBatches)
	}

	// A non-positive window would delete everything; it must be refused.
	if _, err = pool.Exec(ctx, `SELECT telemetry.apply_retention($1)`, time.Duration(0)); err == nil {
		t.Fatal("apply_retention accepted a zero retention window")
	}
}

func fixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, now time.Time) {
	t.Helper()
	const setup = `
INSERT INTO organization(id,code,name)
VALUES ('40000000-0000-4000-8000-000000000001','RETENTION','Retention Test');

INSERT INTO auth.middleware_client(id,organization_id,name,key_prefix,key_hash,auto_onboard)
VALUES ('40000000-0000-4000-8000-000000000002','40000000-0000-4000-8000-000000000001',
        'Retention Gateway','ygm_rrrrrrrr','\x00'::bytea,true);

INSERT INTO plant.plant(id,organization_id,code,name,timezone)
VALUES ('40000000-0000-4000-8000-000000000003','40000000-0000-4000-8000-000000000001',
        'RET-PLANT','Retention Plant','Asia/Bangkok');

INSERT INTO plant.device_model(id,organization_id,manufacturer,model,device_type)
VALUES ('40000000-0000-4000-8000-000000000004','40000000-0000-4000-8000-000000000001',
        'Test','RET-1','Inverter');

INSERT INTO plant.device(id,organization_id,plant_id,device_model_id,external_id,name)
VALUES ('40000000-0000-4000-8000-000000000005','40000000-0000-4000-8000-000000000001',
        '40000000-0000-4000-8000-000000000003','40000000-0000-4000-8000-000000000004','RET-INV-1','Retention Inverter');
`
	if _, err := pool.Exec(ctx, setup); err != nil {
		t.Fatal(err)
	}

	// (idempotency key, batch received_at, reading observed_at)
	batches := []struct {
		key        string
		id         string
		receivedAt time.Time
		observed   []time.Time
	}{
		{"old-empty", "40000000-0000-4000-8000-000000000011", now.Add(-30 * 24 * time.Hour),
			[]time.Time{now.Add(-30 * 24 * time.Hour), now.Add(-20 * 24 * time.Hour)}},
		{"old-backlog", "40000000-0000-4000-8000-000000000012", now.Add(-30 * 24 * time.Hour),
			[]time.Time{now.Add(-9 * 24 * time.Hour), now.Add(-1 * time.Hour)}},
		{"recent", "40000000-0000-4000-8000-000000000013", now.Add(-1 * time.Hour),
			[]time.Time{now.Add(-2 * time.Hour)}},
	}
	reading := 0
	for _, batch := range batches {
		if _, err := pool.Exec(ctx, `
INSERT INTO telemetry.telemetry_ingest_batch(id,organization_id,middleware_client_id,idempotency_key,payload_hash,received_at)
VALUES ($1,'40000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000002',$2,'\x00'::bytea,$3)`,
			batch.id, batch.key, batch.receivedAt); err != nil {
			t.Fatal(err)
		}
		for _, observedAt := range batch.observed {
			reading++
			if _, err := pool.Exec(ctx, `
INSERT INTO telemetry.raw_register_reading(
    id,organization_id,plant_id,device_id,middleware_client_id,ingest_batch_id,
    gateway_id,external_key,observed_at,register_address_map,parameter_count)
VALUES (gen_random_uuid(),'40000000-0000-4000-8000-000000000001',
        '40000000-0000-4000-8000-000000000003','40000000-0000-4000-8000-000000000005',
        '40000000-0000-4000-8000-000000000002',$1,'GW-RET',$2,$3,'{"40001": 1}'::jsonb,1)`,
				batch.id, batch.key+"-"+time.Duration(reading).String(), observedAt); err != nil {
				t.Fatal(err)
			}
		}
	}
}
