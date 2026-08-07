package ingestion

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
	"ygate/platform-api/internal/gatewayhub"
	"ygate/platform-api/internal/testdb"
)

func TestRawIngestionPersistsRegisterMapAgainstPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := testdb.Disposable(t, ctx, databaseURL)
	defer pool.Close()

	organizationID, _ := newUUID()
	clientID, _ := newUUID()
	if _, err := pool.Exec(ctx, "INSERT INTO organization(id,code,name) VALUES($1,'YGATE-RAW-TEST','Raw Ingestion Test')", organizationID); err != nil {
		t.Fatal(err)
	}
	key := "ygate_raw_integration_key_123456"
	hash := sha256.Sum256([]byte(key))
	if _, err := pool.Exec(ctx, `INSERT INTO auth.middleware_client(id,organization_id,name,key_prefix,key_hash,auto_onboard)
		VALUES($1,$2,'Raw Gateway','ygate_raw',$3,true)`, clientID, organizationID, hash[:]); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	batch := RawBatch{SchemaVersion: RawSchemaVersion, Data: []RawReading{{
		GatewayID: "G1", PlantCode: "RAW-PLANT", PlantName: "Raw Plant",
		DevDn: "RAW-INV-1", DevName: "Raw Inverter", DevTypeID: 1, Model: "PVS100",
		CollectTime:        now.UnixMilli(),
		RegisterAddressMap: map[string]float64{"40084": 321},
	}}}
	raw, _ := json.Marshal(batch)
	client := Client{ID: clientID, OrganizationID: organizationID, Name: "Raw Gateway", AutoOnboard: true}
	result, err := New(pool, nil).IngestRaw(ctx, client, "", raw, batch, now)
	if err != nil || result.AcceptedCount != 1 || result.OnboardedPlantCount != 1 || result.OnboardedDeviceCount != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	second := batch
	second.Data[0].CollectTime = now.Add(time.Minute).UnixMilli()
	secondRaw, _ := json.Marshal(second)
	secondResult, err := New(pool, nil).IngestRaw(ctx, client, "", secondRaw, second, now.Add(time.Minute))
	if err != nil || secondResult.AcceptedCount != 1 || secondResult.RejectedCount != 0 {
		t.Fatalf("second result=%+v err=%v", secondResult, err)
	}

	var stored []byte
	var count int
	if err = pool.QueryRow(ctx, "SELECT register_address_map, parameter_count FROM telemetry.raw_register_reading").Scan(&stored, &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 || !json.Valid(stored) || bytes.Contains(stored, []byte("dataItemMap")) || bytes.Contains(stored, []byte("canonicalKey")) {
		t.Fatalf("stored=%s count=%d", stored, count)
	}
	var plantID, deviceID, modelID pgtype.UUID
	if err = pool.QueryRow(ctx, `SELECT p.id, d.id, d.device_model_id FROM plant.plant p JOIN plant.device d ON d.plant_id=p.id WHERE p.code='RAW-PLANT'`).Scan(&plantID, &deviceID, &modelID); err != nil {
		t.Fatal(err)
	}
	// Auto-onboard writes Register Metadata under the "reg"-prefixed key (see
	// ingestRawReading); the raw payload carries the bare address. Targeting
	// the bare key here matched zero rows, so this asserted scaling that was
	// never actually applied.
	tag, err := pool.Exec(ctx, `UPDATE plant.device_model_register_metadata SET scale=2, value_offset=1 WHERE organization_id=$1 AND device_model_id=$2 AND address_key='reg40084'`, organizationID, modelID)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("expected one auto-created register metadata row to scale, updated %d", tag.RowsAffected())
	}
	adminID, _ := newUUID()
	assignmentID, _ := newUUID()
	if _, err = pool.Exec(ctx, `INSERT INTO auth.app_user(id,organization_id,email,display_name,password_hash) VALUES($1,$2,'raw@test.invalid','Raw Viewer','unused')`, adminID, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO auth.user_role(id,user_id,role_id) VALUES($1,$2,'00000000-0000-4000-8000-000000000201')`, assignmentID, adminID); err != nil {
		t.Fatal(err)
	}
	latest, err := core.New(pool, gatewayhub.New()).LatestTelemetry(ctx, auth.Principal{UserID: adminID, OrganizationID: organizationID}, uuidString(plantID))
	if err != nil || len(latest) != 1 || latest[0].DeviceID != uuidString(deviceID) || latest[0].DataItemMap["40084"] != 643 {
		t.Fatalf("latest=%+v err=%v", latest, err)
	}
}
