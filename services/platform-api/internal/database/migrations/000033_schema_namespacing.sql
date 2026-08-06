-- Group existing tables into domain schemas ahead of future
-- service-extraction phases (see docs/superpowers/specs/
-- 2026-08-01-backend-microservices-split-design.md and
-- 2026-08-05-database-schema-namespacing-design.md).
-- organization, audit_log, and site_setting stay in public: no single
-- domain owns them.

CREATE SCHEMA IF NOT EXISTS auth;
CREATE SCHEMA IF NOT EXISTS plant;
CREATE SCHEMA IF NOT EXISTS telemetry;
CREATE SCHEMA IF NOT EXISTS middleware_gateway;
CREATE SCHEMA IF NOT EXISTS scada;
CREATE SCHEMA IF NOT EXISTS alarm;
CREATE SCHEMA IF NOT EXISTS dashboard;

ALTER TABLE app_user SET SCHEMA auth;
ALTER TABLE role SET SCHEMA auth;
ALTER TABLE permission SET SCHEMA auth;
ALTER TABLE role_permission SET SCHEMA auth;
ALTER TABLE user_role SET SCHEMA auth;
ALTER TABLE user_session SET SCHEMA auth;
ALTER TABLE password_reset_token SET SCHEMA auth;
ALTER TABLE auth_attempt SET SCHEMA auth;
ALTER TABLE password_recovery_attempt SET SCHEMA auth;
ALTER TABLE email_verification_token SET SCHEMA auth;
ALTER TABLE middleware_client SET SCHEMA auth;

ALTER TABLE plant SET SCHEMA plant;
ALTER TABLE asset_group SET SCHEMA plant;
ALTER TABLE device_model SET SCHEMA plant;
ALTER TABLE device SET SCHEMA plant;
ALTER TABLE device_register_metadata SET SCHEMA plant;
ALTER TABLE device_model_register_metadata SET SCHEMA plant;

ALTER TABLE telemetry_ingest_batch SET SCHEMA telemetry;
ALTER TABLE telemetry_reading SET SCHEMA telemetry;
ALTER TABLE telemetry_latest SET SCHEMA telemetry;
ALTER TABLE raw_register_reading SET SCHEMA telemetry;

ALTER TABLE middleware_config_history SET SCHEMA middleware_gateway;
ALTER TABLE middleware_plant SET SCHEMA middleware_gateway;
ALTER TABLE middleware_patch SET SCHEMA middleware_gateway;

ALTER TABLE scada_screen SET SCHEMA scada;
ALTER TABLE scada_screen_publication SET SCHEMA scada;

ALTER TABLE alarm_rule SET SCHEMA alarm;
ALTER TABLE alarm_event SET SCHEMA alarm;
ALTER TABLE alarm_rule_condition SET SCHEMA alarm;

ALTER TABLE user_dashboard SET SCHEMA dashboard;
