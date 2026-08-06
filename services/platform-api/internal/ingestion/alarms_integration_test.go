package ingestion

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestEvaluateAlarmsBreachAndClearAgainstPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := disposablePool(t, ctx, databaseURL)
	defer pool.Close()

	organizationID, _ := newUUID()
	clientID, _ := newUUID()
	key := "ygm_" + strings.Repeat("c", 43)
	keyHash := sha256.Sum256([]byte(key))
	if _, err := pool.Exec(ctx, "INSERT INTO organization(id,code,name) VALUES($1,'YGATE-ALARM-TEST','Alarm Test')", organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO auth.middleware_client(id,organization_id,name,key_prefix,key_hash,auto_onboard) VALUES($1,$2,'Alarm Gateway','ygm_cccccccc',$3,true)`, clientID, organizationID, keyHash[:]); err != nil {
		t.Fatal(err)
	}

	service := New(pool, nil)
	client, err := service.Authenticate(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()

	send := func(t *testing.T, offset time.Duration, value float64) {
		t.Helper()
		batch := Batch{Data: []Reading{{
			GatewayID: "GW-ALARM", PlantCode: "ALARM-PLANT", DevDn: "INV-1", DevTypeID: 1, Model: "SUN2000",
			CollectTime: now.Add(offset).UnixMilli(), DataItemMap: map[string]float64{"active_power": value},
		}}}
		raw, _ := json.Marshal(batch)
		result, ingestErr := service.Ingest(ctx, client, "", raw, batch, now)
		if ingestErr != nil || result.AcceptedCount != 1 {
			t.Fatalf("ingest value=%v result=%+v err=%v", value, result, ingestErr)
		}
	}

	// In-range reading onboards the plant and device; no rule exists yet.
	send(t, -30*time.Minute, 10)

	var plantID, deviceID pgtype.UUID
	if err = pool.QueryRow(ctx, `SELECT id FROM plant.plant WHERE code='ALARM-PLANT'`).Scan(&plantID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT id FROM plant.device WHERE external_id='INV-1' AND plant_id=$1`, plantID).Scan(&deviceID); err != nil {
		t.Fatal(err)
	}
	ruleID, _ := newUUID()
	if _, err = pool.Exec(ctx, `
INSERT INTO alarm.alarm_rule (id, organization_id, plant_id, device_id, label, severity)
VALUES ($1,$2,$3,$4,'High power','major')`, ruleID, organizationID, plantID, deviceID); err != nil {
		t.Fatal(err)
	}
	conditionID, _ := newUUID()
	if _, err = pool.Exec(ctx, `
INSERT INTO alarm.alarm_rule_condition (id, alarm_rule_id, point_key, max_value, position)
VALUES ($1,$2,'active_power',50,0)`, conditionID, ruleID); err != nil {
		t.Fatal(err)
	}

	openEventCount := func(t *testing.T) int {
		t.Helper()
		var count int
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM alarm.alarm_event WHERE alarm_rule_id=$1 AND cleared_at IS NULL`, ruleID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	totalEventCount := func(t *testing.T) int {
		t.Helper()
		var count int
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM alarm.alarm_event WHERE alarm_rule_id=$1`, ruleID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}

	// Breach opens exactly one event.
	send(t, -20*time.Minute, 100)
	if got := openEventCount(t); got != 1 {
		t.Fatalf("open events after first breach = %d", got)
	}

	// A second breach must not duplicate the open event.
	send(t, -10*time.Minute, 110)
	if got := openEventCount(t); got != 1 {
		t.Fatalf("open events after repeated breach = %d", got)
	}
	if got := totalEventCount(t); got != 1 {
		t.Fatalf("total events after repeated breach = %d", got)
	}

	// Back in range clears the open event.
	send(t, -5*time.Minute, 25)
	if got := openEventCount(t); got != 0 {
		t.Fatalf("open events after clear = %d", got)
	}
	var clearedAt pgtype.Timestamptz
	if err = pool.QueryRow(ctx, `SELECT cleared_at FROM alarm.alarm_event WHERE alarm_rule_id=$1 ORDER BY id DESC LIMIT 1`, ruleID).Scan(&clearedAt); err != nil || !clearedAt.Valid {
		t.Fatalf("cleared_at=%+v err=%v", clearedAt, err)
	}

	// Breaching again after a clear opens a second, distinct event.
	send(t, -1*time.Minute, 120)
	if got := openEventCount(t); got != 1 {
		t.Fatalf("open events after re-breach = %d", got)
	}
	if got := totalEventCount(t); got != 2 {
		t.Fatalf("total events after re-breach = %d", got)
	}
}
