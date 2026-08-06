package ingestion

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
	"ygate/platform-api/internal/database"
	"ygate/platform-api/internal/gatewayhub"
)

func TestIngestionAutoOnboardingAndIdempotencyAgainstPostgreSQL(t *testing.T) {
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
	lockedClientID, _ := newUUID()
	key := "ygm_" + strings.Repeat("a", 43)
	lockedKey := "ygm_" + strings.Repeat("b", 43)
	keyHash, lockedHash := sha256.Sum256([]byte(key)), sha256.Sum256([]byte(lockedKey))
	if _, err := pool.Exec(ctx, "INSERT INTO organization(id,code,name) VALUES($1,'YGATE-INGEST-TEST','Ingestion Test')", organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO auth.middleware_client(id,organization_id,name,key_prefix,key_hash,auto_onboard) VALUES($1,$2,'Auto Gateway','ygm_aaaaaaaa',$3,true),($4,$2,'Locked Gateway','ygm_bbbbbbbb',$5,false)`, clientID, organizationID, keyHash[:], lockedClientID, lockedHash[:]); err != nil {
		t.Fatal(err)
	}

	service := New(pool, nil)
	client, err := service.Authenticate(ctx, key)
	if err != nil || !client.AutoOnboard || client.ID != clientID {
		t.Fatalf("client=%+v err=%v", client, err)
	}
	now := time.Now().UTC()
	batch := Batch{Data: []Reading{{GatewayID: "GW-1", PlantCode: "NE=49712672", PlantName: "Legacy Plant", DevDn: "INV-1", DevName: "Inverter 1", DevTypeID: 1, Model: "SUN2000", CollectTime: now.Add(-24 * time.Hour).UnixMilli(), DataItemMap: map[string]float64{"active_power": 42.5, "temperature": 31}}}}
	raw, _ := json.Marshal(batch)
	first, err := service.Ingest(ctx, client, "", raw, batch, now)
	if err != nil || first.AcceptedCount != 1 || first.OnboardedPlantCount != 1 || first.OnboardedDeviceCount != 1 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	duplicate, err := service.Ingest(ctx, client, "", raw, batch, now)
	if err != nil || duplicate.Status != "duplicate" || duplicate.IngestionID != first.IngestionID {
		t.Fatalf("duplicate=%+v err=%v", duplicate, err)
	}

	batch.Data[0].CollectTime = now.Add(-23 * time.Hour).UnixMilli()
	secondRaw, _ := json.Marshal(batch)
	second, err := service.Ingest(ctx, client, "second-reading", secondRaw, batch, now)
	if err != nil || second.AcceptedCount != 1 || second.OnboardedPlantCount != 0 || second.OnboardedDeviceCount != 0 {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	batch.Data[0].PlantName = "Different payload"
	conflictRaw, _ := json.Marshal(batch)
	if _, err = service.Ingest(ctx, client, "second-reading", conflictRaw, batch, now); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflict error=%v", err)
	}

	lockedClient, err := service.Authenticate(ctx, lockedKey)
	if err != nil {
		t.Fatal(err)
	}
	unknown := Batch{Data: []Reading{{PlantCode: "UNKNOWN", DevDn: "D1", DevTypeID: 1, CollectTime: now.UnixMilli(), DataItemMap: map[string]float64{"x": 1}}}}
	unknownRaw, _ := json.Marshal(unknown)
	rejected, err := service.Ingest(ctx, lockedClient, "", unknownRaw, unknown, now)
	if err != nil || rejected.RejectedCount != 1 || len(rejected.Errors) != 1 || rejected.Errors[0].Code != "UNKNOWN_PLANT" {
		t.Fatalf("rejected=%+v err=%v", rejected, err)
	}

	var plants, devices, readings, audits, unknownPlants int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM plant.plant), (SELECT count(*) FROM plant.device), (SELECT count(*) FROM telemetry.telemetry_reading), (SELECT count(*) FROM audit_log WHERE action IN ('plant.auto_onboarded','device.auto_onboarded')), (SELECT count(*) FROM plant.plant WHERE code='UNKNOWN')`).Scan(&plants, &devices, &readings, &audits, &unknownPlants); err != nil {
		t.Fatal(err)
	}
	if plants != 1 || devices != 1 || readings != 2 || audits != 2 || unknownPlants != 0 {
		t.Fatalf("plants=%d devices=%d readings=%d audits=%d unknown=%d", plants, devices, readings, audits, unknownPlants)
	}
	var data map[string]float64
	var storedRaw []byte
	if err = pool.QueryRow(ctx, `SELECT data_item_map FROM telemetry.telemetry_reading ORDER BY observed_at LIMIT 1`).Scan(&storedRaw); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(storedRaw, &data); err != nil || data["active_power"] != 42.5 {
		t.Fatalf("data=%v err=%v", data, err)
	}

	adminID, _ := newUUID()
	assignmentID, _ := newUUID()
	engineerID, _ := newUUID()
	engineerAssignmentID, _ := newUUID()
	if _, err = pool.Exec(ctx, `INSERT INTO auth.app_user(id,organization_id,email,display_name,password_hash) VALUES($1,$2,'registry@test.invalid','Registry Admin','unused')`, adminID, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO auth.user_role(id,user_id,role_id) VALUES($1,$2,'00000000-0000-4000-8000-000000000201')`, assignmentID, adminID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO auth.app_user(id,organization_id,email,display_name,password_hash) VALUES($1,$2,'engineer@test.invalid','Dashboard Engineer','unused')`, engineerID, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO auth.user_role(id,user_id,role_id) VALUES($1,$2,'00000000-0000-4000-8000-000000000204')`, engineerAssignmentID, engineerID); err != nil {
		t.Fatal(err)
	}
	var plantID pgtype.UUID
	if err = pool.QueryRow(ctx, "SELECT id FROM plant.plant WHERE code='NE=49712672'").Scan(&plantID); err != nil {
		t.Fatal(err)
	}
	registry := core.New(pool, gatewayhub.New())
	principal := auth.Principal{UserID: adminID, OrganizationID: organizationID}
	registered, err := registry.Devices(ctx, principal, uuidString(plantID))
	if err != nil || len(registered) != 1 || registered[0].ExternalID != "INV-1" {
		t.Fatalf("registered=%+v err=%v", registered, err)
	}
	latest, err := registry.LatestTelemetry(ctx, principal, uuidString(plantID))
	if err != nil || len(latest) != 1 || latest[0].DeviceID != registered[0].ID || latest[0].DataItemMap["active_power"] != 42.5 || !latest[0].ObservedAt.Equal(now.Add(-23*time.Hour).Truncate(time.Millisecond)) {
		t.Fatalf("latest=%+v err=%v", latest, err)
	}
	dashboard, err := registry.DashboardOverview(ctx, principal, 5*time.Minute, now)
	if err != nil || dashboard.PlantCount != 1 || dashboard.ActiveDeviceCount != 1 || dashboard.ReportingDeviceCount != 1 || dashboard.StaleDeviceCount != 1 || dashboard.OfflineDeviceCount != 0 || len(dashboard.Plants) != 1 || dashboard.Plants[0].CommunicationStatus != "DEGRADED" {
		t.Fatalf("dashboard=%+v err=%v", dashboard, err)
	}
	layout, err := registry.DashboardLayout(ctx, principal)
	if err != nil || layout.ID != nil || layout.Version != 0 || layout.RegistryVersion != 2 || !layout.CanEdit || !layout.CanPublish {
		t.Fatalf("default dashboard layout=%+v err=%v", layout, err)
	}
	engineerLayout, err := registry.DashboardLayout(ctx, auth.Principal{UserID: engineerID, OrganizationID: organizationID})
	if err != nil || !engineerLayout.CanEdit || engineerLayout.CanPublish {
		t.Fatalf("engineer dashboard capability=%+v err=%v", engineerLayout, err)
	}
	if _, err = registry.PublishDashboardLayout(ctx, auth.Principal{UserID: engineerID, OrganizationID: organizationID}, core.PublishDashboardLayoutInput{Version: 1, PublishedVersion: 0}, nil); !errors.Is(err, core.ErrForbidden) {
		t.Fatalf("engineer publish error=%v", err)
	}
	layout.Layouts["lg"][0], layout.Layouts["lg"][1] = layout.Layouts["lg"][1], layout.Layouts["lg"][0]
	layout.Layouts["lg"][0].X, layout.Layouts["lg"][1].X = 0, 3
	for breakpoint, item := range map[string]core.DashboardLayoutItem{
		"lg": {I: "timeseries-line", X: 0, Y: 8, W: 12, H: 5},
		"md": {I: "timeseries-line", X: 0, Y: 10, W: 8, H: 5},
		"sm": {I: "timeseries-line", X: 0, Y: 16, W: 4, H: 6},
	} {
		layout.Layouts[breakpoint] = append(layout.Layouts[breakpoint], item)
	}
	layout.WidgetConfigs["timeseries-line"] = core.DashboardWidgetConfig{
		Version: 1,
		DataBinding: core.DashboardWidgetDataBinding{PlantID: uuidString(plantID), DeviceID: registered[0].ID, PointKey: "active_power", TimeRangeHours: 24},
		Display: core.DashboardWidgetDisplay{Unit: "kW", Decimals: 1},
	}
	savedLayout, err := registry.SaveDashboardLayout(ctx, principal, core.UpdateDashboardLayoutInput{Version: layout.Version, Layouts: layout.Layouts, WidgetConfigs: layout.WidgetConfigs}, nil)
	if err != nil || savedLayout.ID == nil || savedLayout.Version != 1 || savedLayout.Layouts["lg"][0].I != "active-device-count" || savedLayout.WidgetConfigs["timeseries-line"].DataBinding.PointKey != "active_power" {
		t.Fatalf("saved dashboard layout=%+v err=%v", savedLayout, err)
	}
	publishedLayout, err := registry.PublishedDashboardLayout(ctx, principal)
	if err != nil || publishedLayout.Version != 0 || publishedLayout.SourceVersion != 0 || publishedLayout.Layouts["lg"][0].I != "plant-count" || len(publishedLayout.WidgetConfigs) != 0 {
		t.Fatalf("unpublished dashboard layout=%+v err=%v", publishedLayout, err)
	}
	publishedLayout, err = registry.PublishDashboardLayout(ctx, principal, core.PublishDashboardLayoutInput{Version: savedLayout.Version, PublishedVersion: publishedLayout.Version}, nil)
	if err != nil || publishedLayout.Version != 1 || publishedLayout.SourceVersion != savedLayout.Version || publishedLayout.Layouts["lg"][0].I != "active-device-count" || publishedLayout.WidgetConfigs["timeseries-line"].Display.Unit != "kW" {
		t.Fatalf("published dashboard layout=%+v err=%v", publishedLayout, err)
	}
	if _, err = registry.PublishDashboardLayout(ctx, principal, core.PublishDashboardLayoutInput{Version: savedLayout.Version, PublishedVersion: 0}, nil); !errors.Is(err, core.ErrDashboardVersionConflict) {
		t.Fatalf("stale dashboard publish error=%v", err)
	}
	if _, err = registry.SaveDashboardLayout(ctx, principal, core.UpdateDashboardLayoutInput{Version: 0, Layouts: layout.Layouts, WidgetConfigs: layout.WidgetConfigs}, nil); !errors.Is(err, core.ErrDashboardVersionConflict) {
		t.Fatalf("stale dashboard update error=%v", err)
	}
	var dashboardDraftAudits, dashboardPublishAudits int
	if err = pool.QueryRow(ctx, "SELECT count(*) FILTER (WHERE action='dashboard.created'), count(*) FILTER (WHERE action='dashboard.published') FROM audit_log").Scan(&dashboardDraftAudits, &dashboardPublishAudits); err != nil || dashboardDraftAudits != 1 || dashboardPublishAudits != 1 {
		t.Fatalf("dashboard draft audits=%d publish audits=%d err=%v", dashboardDraftAudits, dashboardPublishAudits, err)
	}
	historyInput := core.TelemetryHistoryInput{From: now.Add(-25 * time.Hour), To: now.Add(-22 * time.Hour), Limit: 1}
	firstPage, err := registry.TelemetryHistory(ctx, principal, uuidString(plantID), registered[0].ID, historyInput)
	if err != nil || len(firstPage.Data) != 1 || firstPage.NextCursor == nil || !firstPage.Data[0].ObservedAt.Equal(now.Add(-23*time.Hour).Truncate(time.Millisecond)) {
		t.Fatalf("first history page=%+v err=%v", firstPage, err)
	}
	historyInput.Cursor = *firstPage.NextCursor
	secondPage, err := registry.TelemetryHistory(ctx, principal, uuidString(plantID), registered[0].ID, historyInput)
	if err != nil || len(secondPage.Data) != 1 || secondPage.NextCursor != nil || !secondPage.Data[0].ObservedAt.Equal(now.Add(-24*time.Hour).Truncate(time.Millisecond)) {
		t.Fatalf("second history page=%+v err=%v", secondPage, err)
	}
	older := Batch{Data: []Reading{{GatewayID: "GW-1", PlantCode: "NE=49712672", PlantName: "Legacy Plant", DevDn: "INV-1", DevName: "Inverter 1", DevTypeID: 1, Model: "SUN2000", CollectTime: now.Add(-24*time.Hour - 30*time.Minute).UnixMilli(), DataItemMap: map[string]float64{"active_power": 5}}}}
	olderRaw, _ := json.Marshal(older)
	olderResult, err := service.Ingest(ctx, client, "older-reading", olderRaw, older, now)
	if err != nil || olderResult.AcceptedCount != 1 {
		t.Fatalf("older result=%+v err=%v", olderResult, err)
	}
	latestAfterOlder, err := registry.LatestTelemetry(ctx, principal, uuidString(plantID))
	if err != nil || len(latestAfterOlder) != 1 || latestAfterOlder[0].DataItemMap["active_power"] != 42.5 || !latestAfterOlder[0].ObservedAt.Equal(now.Add(-23*time.Hour).Truncate(time.Millisecond)) {
		t.Fatalf("latest after older=%+v err=%v", latestAfterOlder, err)
	}
	updated, err := registry.UpdateDevice(ctx, principal, uuidString(plantID), registered[0].ID, core.UpdateDeviceInput{Name: "Main Inverter", IsActive: false}, nil)
	if err != nil || updated.Name != "Main Inverter" || updated.IsActive {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	var updateAudits int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM audit_log WHERE action='device.updated'").Scan(&updateAudits); err != nil || updateAudits != 1 {
		t.Fatalf("update audits=%d err=%v", updateAudits, err)
	}
}

func disposablePool(t *testing.T, ctx context.Context, databaseURL string) *pgxpool.Pool {
	t.Helper()
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Del("schema")
	query.Set("search_path", "public")
	parsed.RawQuery = query.Encode()
	admin, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}

	name := fmt.Sprintf("ingestion_test_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		admin.Close()
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+identifier)
		admin.Close()
	})

	dbURL := *parsed
	dbURL.Path = "/" + name
	pool, err := database.Open(ctx, dbURL.String())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		pool.Close()
	})
	return pool
}
