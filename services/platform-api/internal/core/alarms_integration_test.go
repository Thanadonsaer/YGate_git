package core

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/database"
	"ygate/platform-api/internal/gatewayhub"
)

func TestAlarmRuleAndEventLifecycleAgainstPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	orgID := mustUUID(t, "30000000-0000-4000-8000-000000000001")
	plantID := mustUUID(t, "30000000-0000-4000-8000-000000000002")
	deviceID := mustUUID(t, "30000000-0000-4000-8000-000000000003")
	deviceModelID := mustUUID(t, "30000000-0000-4000-8000-000000000004")
	platformAdminID := mustUUID(t, "30000000-0000-4000-8000-000000000011")
	viewerID := mustUUID(t, "30000000-0000-4000-8000-000000000012")

	if _, err = pool.Exec(ctx, "INSERT INTO organization(id,code,name) VALUES($1,$2,$3)", orgID, "TEST-ALARMS-A", "Test Alarms Organization"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO plant(id,organization_id,code,name,timezone) VALUES($1,$2,'ALARM-PLANT','Alarm Plant','UTC')`, plantID, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO device_model(id,organization_id,manufacturer,model,device_type) VALUES($1,$2,'Test','Model-1','INVERTER')`, deviceModelID, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO device(id,organization_id,plant_id,device_model_id,external_id,name) VALUES($1,$2,$3,$4,'INV-1','Inverter 1')`, deviceID, orgID, plantID, deviceModelID); err != nil {
		t.Fatal(err)
	}
	for _, user := range []struct {
		id             pgtype.UUID
		assignmentID   pgtype.UUID
		role           pgtype.UUID
		organizationID any
		email          string
	}{
		{platformAdminID, mustUUID(t, "30000000-0000-4000-8000-000000000021"), mustUUID(t, "00000000-0000-4000-8000-000000000201"), nil, "alarms-admin@test.invalid"},
		{viewerID, mustUUID(t, "30000000-0000-4000-8000-000000000022"), mustUUID(t, "00000000-0000-4000-8000-000000000206"), orgID, "alarms-viewer@test.invalid"},
	} {
		if _, err = pool.Exec(ctx, `INSERT INTO app_user(id,organization_id,email,display_name,password_hash) VALUES($1,$2,$3,$3,'unused')`, user.id, orgID, user.email); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO user_role(id,organization_id,user_id,role_id) VALUES($1,$2,$3,$4)`, user.assignmentID, user.organizationID, user.id, user.role); err != nil {
			t.Fatal(err)
		}
	}

	service := New(pool, gatewayhub.New())
	admin := auth.Principal{UserID: platformAdminID, OrganizationID: orgID}
	viewer := auth.Principal{UserID: viewerID, OrganizationID: orgID}

	minValue, maxValue := 0.0, 50.0
	rule, err := service.CreateAlarmRule(ctx, admin, uuidString(plantID), CreateAlarmRuleInput{
		DeviceID: uuidString(deviceID), PointKey: "active_power", Label: "High power", MinValue: &minValue, MaxValue: &maxValue, Severity: "major",
	}, nil)
	if err != nil || rule.Severity != "major" || !rule.IsActive {
		t.Fatalf("rule=%+v err=%v", rule, err)
	}
	if _, err = service.CreateAlarmRule(ctx, viewer, uuidString(plantID), CreateAlarmRuleInput{
		DeviceID: uuidString(deviceID), PointKey: "x", Label: "Denied", Severity: "warning", MaxValue: &maxValue,
	}, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer create error = %v", err)
	}
	if _, err = service.CreateAlarmRule(ctx, admin, uuidString(plantID), CreateAlarmRuleInput{
		DeviceID: uuidString(mustUUID(t, "30000000-0000-4000-8000-000000000099")), PointKey: "x", Label: "Bad device", Severity: "warning", MaxValue: &maxValue,
	}, nil); !errors.Is(err, ErrAlarmRuleInvalid) {
		t.Fatalf("unknown device error = %v", err)
	}

	rules, err := service.ListAlarmRules(ctx, admin, uuidString(plantID))
	if err != nil || len(rules) != 1 {
		t.Fatalf("rules=%+v err=%v", rules, err)
	}

	updated, err := service.UpdateAlarmRule(ctx, admin, uuidString(plantID), rule.ID, UpdateAlarmRuleInput{
		Label: "High power v2", Severity: "critical", MaxValue: &maxValue, IsActive: false,
	}, nil)
	if err != nil || updated.Label != "High power v2" || updated.Severity != "critical" || updated.IsActive {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}

	// Simulate an alarm breach the way evaluateAlarms (internal/ingestion) would, to exercise
	// the event list/ack surface without duplicating the ingestion package's own test.
	var eventID int64
	if err = pool.QueryRow(ctx, `
INSERT INTO alarm_event (organization_id, plant_id, device_id, alarm_rule_id, point_key, severity, value, threshold_max)
VALUES ($1,$2,$3,$4,'active_power','critical',75,50) RETURNING id`, orgID, plantID, deviceID, mustUUID(t, rule.ID)).Scan(&eventID); err != nil {
		t.Fatal(err)
	}

	events, err := service.ListAlarmEvents(ctx, admin, uuidString(plantID), 0)
	if err != nil || len(events) != 1 || events[0].ID != eventID || events[0].AcknowledgedBy != nil {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	since, err := service.AlarmEventsSince(ctx, admin, uuidString(plantID), 0)
	if err != nil || len(since) != 1 || since[0].ID != eventID {
		t.Fatalf("since=%+v err=%v", since, err)
	}
	latestID, err := service.LatestAlarmEventID(ctx, admin, uuidString(plantID))
	if err != nil || latestID != eventID {
		t.Fatalf("latestID=%d err=%v", latestID, err)
	}

	acked, err := service.AcknowledgeAlarmEvent(ctx, admin, uuidString(plantID), "42", "seen it", nil)
	if err != nil || acked.AcknowledgedBy == nil || *acked.AcknowledgedBy != uuidString(platformAdminID) {
		t.Fatalf("acked=%+v err=%v", acked, err)
	}
	// Idempotent: a second acknowledgement must not change who/when.
	firstAckAt := *acked.AcknowledgedAt
	reacked, err := service.AcknowledgeAlarmEvent(ctx, admin, uuidString(plantID), "42", "different note", nil)
	if err != nil || reacked.AcknowledgedAt == nil || !reacked.AcknowledgedAt.Equal(firstAckAt) {
		t.Fatalf("reacked=%+v err=%v", reacked, err)
	}

	if err = service.DeleteAlarmRule(ctx, admin, uuidString(plantID), rule.ID, nil); err != nil {
		t.Fatalf("delete rule: %v", err)
	}
	rules, err = service.ListAlarmRules(ctx, admin, uuidString(plantID))
	if err != nil || len(rules) != 0 {
		t.Fatalf("rules after delete=%+v err=%v", rules, err)
	}
}
