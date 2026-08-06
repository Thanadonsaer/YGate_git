-- name: GetAuthorizedPlantResource :one
SELECT p.id, p.organization_id, p.code, p.name
FROM plant.plant p
WHERE p.id = sqlc.arg(plant_id)
  AND EXISTS (
      SELECT 1 FROM auth.user_role ur
      JOIN auth.role r ON r.id = ur.role_id
      JOIN auth.role_permission rp ON rp.role_id = ur.role_id
      JOIN auth.permission pm ON pm.id = rp.permission_id
      WHERE ur.user_id = sqlc.arg(user_id)
        AND pm.action = sqlc.arg(action)
        AND pm.resource_type = sqlc.arg(resource_type)
        AND (r.organization_id IS NULL OR r.organization_id = ur.organization_id)
        AND (rp.organization_id IS NULL OR rp.organization_id = ur.organization_id)
        AND (ur.organization_id IS NULL OR ur.organization_id = p.organization_id)
        AND (ur.plant_id IS NULL OR ur.plant_id = p.id)
  )
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
  AND EXISTS (
      SELECT 1 FROM auth.user_role ur
      JOIN auth.role r ON r.id = ur.role_id
      JOIN auth.role_permission rp ON rp.role_id = ur.role_id
      JOIN auth.permission pm ON pm.id = rp.permission_id
      WHERE ur.user_id = sqlc.arg(user_id)
        AND pm.action = 'update'
        AND pm.resource_type = 'device'
        AND (r.organization_id IS NULL OR r.organization_id = ur.organization_id)
        AND (rp.organization_id IS NULL OR rp.organization_id = ur.organization_id)
        AND (ur.organization_id IS NULL OR ur.organization_id = d.organization_id)
        AND (ur.plant_id IS NULL OR ur.plant_id = d.plant_id)
  )
LIMIT 1
FOR UPDATE OF d;

-- name: UpdateDevice :one
UPDATE plant.device
SET name = sqlc.arg(name), is_active = sqlc.arg(is_active), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, organization_id, plant_id, external_id, name, device_model_id,
          is_active, created_at, updated_at;