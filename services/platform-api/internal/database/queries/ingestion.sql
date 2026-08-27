-- name: AuthenticateMiddlewareClient :one
SELECT id, organization_id, name, auto_onboard
FROM auth.middleware_client
WHERE key_hash = sqlc.arg(key_hash) AND is_active = true
LIMIT 1;

-- Only payload_hash is stored, never the payload itself: the hash is all the
-- idempotency-conflict check below needs, and keeping the body would duplicate
-- every reading already in raw_register_reading (see migration 000039).
-- name: CreateOrGetIngestBatch :one
INSERT INTO telemetry.telemetry_ingest_batch (
    id, organization_id, middleware_client_id, idempotency_key, payload_hash
) VALUES (
    sqlc.arg(id), sqlc.arg(organization_id), sqlc.arg(middleware_client_id),
    sqlc.arg(idempotency_key), sqlc.arg(payload_hash)
)
ON CONFLICT (middleware_client_id, idempotency_key) DO UPDATE
SET idempotency_key = EXCLUDED.idempotency_key
RETURNING id, payload_hash, status, accepted_count, duplicate_count, rejected_count,
          onboarded_plant_count, onboarded_device_count, errors;

-- name: CompleteIngestBatch :exec
UPDATE telemetry.telemetry_ingest_batch
SET accepted_count = sqlc.arg(accepted_count),
    duplicate_count = sqlc.arg(duplicate_count),
    rejected_count = sqlc.arg(rejected_count),
    onboarded_plant_count = sqlc.arg(onboarded_plant_count),
    onboarded_device_count = sqlc.arg(onboarded_device_count),
    errors = sqlc.arg(errors), status = 'PROCESSED', processed_at = now()
WHERE id = sqlc.arg(id);

-- name: TouchMiddlewareClient :exec
UPDATE auth.middleware_client SET last_seen_at = now(), updated_at = now() WHERE id = sqlc.arg(id);

-- name: GetIngestionPlant :one
SELECT id, organization_id, code, name, is_active
FROM plant.plant
WHERE organization_id = sqlc.arg(organization_id) AND code = sqlc.arg(code)
LIMIT 1;

-- name: GetIngestionDevice :one
SELECT id, organization_id, plant_id, device_model_id, external_id, name, is_active
FROM plant.device
WHERE organization_id = sqlc.arg(organization_id)
  AND plant_id = sqlc.arg(plant_id)
  AND external_id = sqlc.arg(external_id)
LIMIT 1;
-- name: OnboardPlant :one
INSERT INTO plant.plant (id, organization_id, code, name, timezone)
VALUES (sqlc.arg(id), sqlc.arg(organization_id), sqlc.arg(code), sqlc.arg(name), 'Asia/Bangkok')
ON CONFLICT (organization_id, code) DO UPDATE SET code = EXCLUDED.code
RETURNING id, organization_id, code, name, is_active, (xmax = 0) AS created;

-- name: OnboardDeviceModel :one
INSERT INTO plant.device_model (
    id, organization_id, manufacturer, model, device_type, source_type_id
) VALUES (
    sqlc.arg(id), sqlc.arg(organization_id), 'Middleware', sqlc.arg(model),
    sqlc.arg(device_type), sqlc.arg(source_type_id)
)
ON CONFLICT (organization_id, manufacturer, model) DO UPDATE SET model = EXCLUDED.model
RETURNING id, organization_id, manufacturer, model, device_type, source_type_id,
          is_active, (xmax = 0) AS created;

-- name: OnboardDevice :one
INSERT INTO plant.device (
    id, organization_id, plant_id, device_model_id, external_id, name, source_metadata
) VALUES (
    sqlc.arg(id), sqlc.arg(organization_id), sqlc.arg(plant_id), sqlc.arg(device_model_id),
    sqlc.arg(external_id), sqlc.arg(name), sqlc.arg(source_metadata)
)
ON CONFLICT (organization_id, plant_id, external_id) DO UPDATE SET external_id = EXCLUDED.external_id
RETURNING id, organization_id, plant_id, device_model_id, external_id, name,
          is_active, (xmax = 0) AS created;

-- name: CreateInventoryAuditEvent :exec
INSERT INTO audit_log (
    organization_id, action, target_type, target_id, after_data, correlation_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(action), sqlc.arg(target_type),
    sqlc.arg(target_id), sqlc.arg(after_data), sqlc.arg(correlation_id)
);

-- name: MiddlewareClientPullConfig :one
SELECT poll_interval_seconds, command_timeout_seconds, api_polling_enabled
FROM auth.middleware_client
WHERE id = sqlc.arg(id);
