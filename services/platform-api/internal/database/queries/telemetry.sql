-- name: ListLatestPlantTelemetry :many
SELECT latest.telemetry_reading_id AS id, latest.organization_id, latest.plant_id, latest.device_id,
       d.external_id AS device_external_id, d.name AS device_name,
       latest.gateway_id, latest.observed_at, latest.received_at,
       latest.data_item_map, latest.parameter_count
FROM telemetry.raw_telemetry_latest latest
JOIN plant.device d ON d.organization_id = latest.organization_id
             AND d.plant_id = latest.plant_id
             AND d.id = latest.device_id
WHERE latest.organization_id = sqlc.arg(organization_id)
  AND latest.plant_id = sqlc.arg(plant_id)
ORDER BY d.name, d.external_id, d.id
LIMIT 500;

-- name: GetPlantDevice :one
SELECT id
FROM plant.device
WHERE organization_id = sqlc.arg(organization_id)
  AND plant_id = sqlc.arg(plant_id)
  AND id = sqlc.arg(device_id)
LIMIT 1;

-- Device telemetry history has no query here on purpose: its keyset cursor is
-- a `(observed_at, received_at, id) < (...)` row comparison, and sqlc's
-- analyzer infers the cursor id parameter as timestamptz to match its
-- row-comparison neighbours instead of uuid, which sends the wrong wire format
-- at runtime. It lives as one hand-written query in internal/core/telemetry.go
-- and shares this file's mapping rule through telemetry.mapped_data_items().
