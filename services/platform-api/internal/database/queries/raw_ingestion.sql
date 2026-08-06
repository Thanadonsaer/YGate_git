-- name: InsertRawRegisterReading :one
INSERT INTO telemetry.raw_register_reading (
    id, organization_id, plant_id, device_id, middleware_client_id, ingest_batch_id,
    gateway_id, external_key, observed_at, register_address_map, parameter_count
) VALUES (
    sqlc.arg(id), sqlc.arg(organization_id), sqlc.arg(plant_id), sqlc.arg(device_id),
    sqlc.arg(middleware_client_id), sqlc.arg(ingest_batch_id), sqlc.arg(gateway_id),
    sqlc.arg(external_key), sqlc.arg(observed_at), sqlc.arg(register_address_map),
    sqlc.arg(parameter_count)
)
ON CONFLICT (middleware_client_id, external_key, observed_at) DO NOTHING
RETURNING id;
