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
    want := []string{"000001_foundation.sql", "000002_authentication.sql", "000003_session_security.sql", "000004_password_recovery.sql", "000005_rbac_registry.sql", "000006_middleware_ingestion.sql", "000007_telemetry_latest.sql", "000008_dashboard_layout.sql", "000009_dashboard_publish.sql", "000010_dashboard_widget_config.sql", "000011_dashboard_sharing.sql", "000012_admin_integrations.sql", "000013_device_register_metadata.sql", "000014_device_model_register_metadata.sql", "000015_raw_register_ingestion.sql", "000016_platform_admin_hard_delete.sql", "000017_platform_admin_audit_clear.sql", "000018_scada_screens.sql", "000019_role_permission_admin.sql", "000020_alarm_monitoring.sql", "000021_middleware_config.sql", "000022_device_modbus_config.sql", "000023_middleware_remote_management.sql", "000024_site_branding.sql", "000025_alarm_notification.sql", "000026_alarm_rule_conditions.sql", "000027_single_system_role.sql", "000028_plant_image.sql", "000029_session_permission.sql", "000030_account_registration.sql", "000031_middleware_poll_interval.sql", "000032_middleware_api_polling_enabled.sql", "000033_schema_namespacing.sql", "000034_telemetry_partitioning.sql", "000035_index_audit.sql", "000036_cleanup_legacy_register_metadata.sql", "000037_raw_telemetry_latest_view.sql", "000038_organization_create_permission.sql", "000039_raw_only_telemetry.sql", "000040_telemetry_retention.sql", "000041_restore_raw_payload_column.sql", "000042_plant_lifecycle.sql", "000043_event_logbook.sql", "000044_alarm_condition_logic.sql", "000045_latest_reading_lateral.sql", "000046_middleware_idle_heartbeat.sql", "000047_alarm_delay.sql", "000048_unassigned_registration_users.sql", "000049_pending_access_status.sql", "000050_register_profiles.sql", "000051_telemetry_display_values.sql", "000052_register_alarm_events.sql"}
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
	for _, table := range []string{"organization", "plant", "asset_group", "device_model", "device", "device_register_metadata", "device_model_register_metadata", "audit_log", "app_user", "role", "permission", "user_session", "password_reset_token", "auth_attempt", "password_recovery_attempt", "middleware_client", "telemetry_ingest_batch", "raw_register_reading", "user_dashboard", "scada_screen", "scada_screen_publication", "alarm_rule", "alarm_event", "event_logbook"} {
		needle := "CREATE TABLE " + table
		if table == "event_logbook" {
			needle = "CREATE TABLE alarm.event_logbook"
		}
		if !strings.Contains(text, needle) {
			t.Fatalf("migrations missing table %s", table)
		}
	}
    for _, required := range []string{"token_hash bytea", "csrf_hash bytea", "audit_log_append_only", "organization_id", "Platform Admin", "Organization Admin", "user_role_user_scope_fk", "plant_installed_dc_nonnegative", "raw_register_reading_client_external_unique", "auto_onboard boolean", "'hard_delete', 'user'", "'clear', 'audit'", "'manage_access', 'scada_screen'", "scada_publication_immutable", "DROP CONSTRAINT audit_log_actor_fk", "'delete', 'role'", "PENDING_ACCESS", "register_profile", "register_profile_address", "register_value_mapping", "device_model_register_profile_fk", "display_item_map", "REGISTER", "register_snapshot", "alarm_email_enabled"} {
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
