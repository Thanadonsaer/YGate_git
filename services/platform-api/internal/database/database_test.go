package database

import (
	"strings"
	"testing"
)

func TestMigrationsAreEmbeddedInOrder(t *testing.T) {
	names, err := migrationNames()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"000001_foundation.sql", "000002_authentication.sql", "000003_session_security.sql", "000004_password_recovery.sql", "000005_rbac_registry.sql", "000006_middleware_ingestion.sql", "000007_telemetry_latest.sql", "000008_dashboard_layout.sql", "000009_dashboard_publish.sql", "000010_dashboard_widget_config.sql", "000011_dashboard_sharing.sql", "000012_admin_integrations.sql", "000013_device_register_metadata.sql", "000014_device_model_register_metadata.sql", "000015_raw_register_ingestion.sql", "000016_platform_admin_hard_delete.sql", "000017_platform_admin_audit_clear.sql", "000018_scada_screens.sql", "000019_role_permission_admin.sql", "000020_alarm_monitoring.sql"}
	if len(names) != len(want) {
		t.Fatalf("migrations=%v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("migrations=%v", names)
		}
	}

	var combined strings.Builder
	for _, name := range names {
		sql, readErr := migrationFS.ReadFile("migrations/" + name)
		if readErr != nil {
			t.Fatal(readErr)
		}
		combined.Write(sql)
	}
	text := combined.String()
	for _, table := range []string{"organization", "plant", "asset_group", "device_model", "device", "device_register_metadata", "device_model_register_metadata", "audit_log", "app_user", "role", "permission", "user_session", "password_reset_token", "auth_attempt", "password_recovery_attempt", "middleware_client", "telemetry_ingest_batch", "telemetry_reading", "raw_register_reading", "telemetry_latest", "user_dashboard", "scada_screen", "scada_screen_publication", "alarm_rule", "alarm_event"} {
		if !strings.Contains(text, "CREATE TABLE "+table) {
			t.Fatalf("migrations missing table %s", table)
		}
	}
	for _, required := range []string{"token_hash bytea", "csrf_hash bytea", "audit_log_append_only", "organization_id", "Platform Admin", "Organization Admin", "user_role_user_scope_fk", "plant_installed_dc_nonnegative", "telemetry_reading_client_external_unique", "auto_onboard boolean", "'hard_delete', 'user'", "'clear', 'audit'", "'manage_access', 'scada_screen'", "scada_publication_immutable", "DROP CONSTRAINT audit_log_actor_fk", "'delete', 'role'"} {
		if !strings.Contains(text, required) {
			t.Fatalf("migrations missing %s", required)
		}
	}
	for _, forbidden := range []string{"postgis", "timescaledb", "redis", "nats", "prisma"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("migrations contain forbidden dependency %s", forbidden)
		}
	}
}
