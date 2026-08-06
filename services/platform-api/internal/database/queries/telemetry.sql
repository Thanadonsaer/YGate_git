-- name: ListLatestPlantTelemetry :many
SELECT latest.telemetry_reading_id AS id, latest.organization_id, latest.plant_id, latest.device_id,
       d.external_id AS device_external_id, d.name AS device_name,
       latest.gateway_id, latest.observed_at, latest.received_at,
       latest.data_item_map, latest.parameter_count
FROM telemetry.telemetry_latest latest
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

-- NOTE: internal/core/telemetry.go has a hand-written duplicate of this
-- query (listDeviceTelemetryHistorySQL) working around an sqlc cursor-type
-- inference bug — if you change this query's shape, update that too.
-- name: ListDeviceTelemetryHistory :many
SELECT tr.id, tr.organization_id, tr.plant_id, tr.device_id,
       d.external_id AS device_external_id, d.name AS device_name,
       tr.gateway_id, tr.observed_at, tr.received_at,
       tr.data_item_map, tr.parameter_count
FROM telemetry.telemetry_reading tr
JOIN plant.device d ON d.organization_id = tr.organization_id
             AND d.plant_id = tr.plant_id
             AND d.id = tr.device_id
WHERE tr.organization_id = sqlc.arg(organization_id)
  AND tr.plant_id = sqlc.arg(plant_id)
  AND tr.device_id = sqlc.arg(device_id)
  AND tr.observed_at >= sqlc.arg(from_time)
  AND tr.observed_at < sqlc.arg(to_time)
  AND (
      NOT sqlc.arg(cursor_set)::boolean
      OR (tr.observed_at, tr.received_at, tr.id) <
         (sqlc.arg(cursor_observed_at), sqlc.arg(cursor_received_at), sqlc.arg(cursor_id))
  )
ORDER BY tr.observed_at DESC, tr.received_at DESC, tr.id DESC
LIMIT sqlc.arg(page_limit);
