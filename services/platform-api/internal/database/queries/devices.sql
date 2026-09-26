-- name: GetAuthorizedPlantResource :one
SELECT p.id, p.organization_id, p.code, p.name
FROM plant.plant p
WHERE p.id = sqlc.arg(plant_id)
  AND auth.has_permission(sqlc.arg(user_id), sqlc.arg(action), sqlc.arg(resource_type), p.organization_id, p.id)
LIMIT 1;

-- name: ListPlantDevices :many
SELECT d.id, d.organization_id, d.plant_id, d.external_id, d.name,
       d.device_model_id, dm.manufacturer, dm.model, dm.device_type,
       dm.source_type_id, d.is_active, d.created_at, d.updated_at
FROM plant.device d
JOIN plant.device_model dm ON dm.id = d.device_model_id
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.plant_id = sqlc.arg(plant_id)
ORDER BY d.name, d.external_id, d.id
LIMIT 500;

-- name: GetAuthorizedDeviceForUpdate :one
SELECT d.id, d.organization_id, d.plant_id, d.external_id, d.name,
       d.device_model_id, dm.manufacturer, dm.model, dm.device_type,
       dm.source_type_id, d.is_active, d.created_at, d.updated_at
FROM plant.device d
JOIN plant.device_model dm ON dm.id = d.device_model_id
WHERE d.id = sqlc.arg(device_id)
  AND d.plant_id = sqlc.arg(plant_id)
  AND auth.has_permission(sqlc.arg(user_id), 'update', 'device', d.organization_id, d.plant_id)
LIMIT 1
FOR UPDATE OF d;

-- name: UpdateDevice :one
UPDATE plant.device
SET name = sqlc.arg(name), is_active = sqlc.arg(is_active), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, organization_id, plant_id, external_id, name, device_model_id,
          is_active, created_at, updated_at;