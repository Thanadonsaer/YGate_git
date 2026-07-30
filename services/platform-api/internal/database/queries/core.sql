-- name: HasOrganizationPermission :one
SELECT EXISTS (
    SELECT 1 FROM user_role ur
    JOIN role r ON r.id = ur.role_id
    JOIN role_permission rp ON rp.role_id = ur.role_id
    JOIN permission pm ON pm.id = rp.permission_id
    WHERE ur.user_id = sqlc.arg(user_id)
      AND pm.action = sqlc.arg(action)
      AND pm.resource_type = sqlc.arg(resource_type)
      AND (r.organization_id IS NULL OR r.organization_id = ur.organization_id)
      AND (rp.organization_id IS NULL OR rp.organization_id = ur.organization_id)
      AND ur.plant_id IS NULL
      AND (ur.organization_id IS NULL OR ur.organization_id = sqlc.arg(organization_id))
)::boolean;

-- name: HasUserPermission :one
SELECT EXISTS (
    SELECT 1 FROM user_role ur
    JOIN role r ON r.id = ur.role_id
    JOIN role_permission rp ON rp.role_id = ur.role_id
    JOIN permission pm ON pm.id = rp.permission_id
    WHERE ur.user_id = sqlc.arg(user_id)
      AND pm.action = sqlc.arg(action)
      AND pm.resource_type = sqlc.arg(resource_type)
      AND (r.organization_id IS NULL OR r.organization_id = ur.organization_id)
      AND (rp.organization_id IS NULL OR rp.organization_id = ur.organization_id)
)::boolean;
-- name: GetOrganizationName :one
SELECT name FROM organization WHERE id = sqlc.arg(organization_id) AND is_active = true;
-- name: ListAuthorizedPlants :many
SELECT p.id, p.organization_id, o.name AS organization_name,
       p.code, p.name, p.timezone, p.latitude, p.longitude,
       p.installed_dc_kw,
       p.installed_ac_kw,
       p.is_active, p.created_at, p.updated_at
FROM plant p
JOIN organization o ON o.id = p.organization_id
WHERE EXISTS (
    SELECT 1 FROM user_role ur
    JOIN role r ON r.id = ur.role_id
    JOIN role_permission rp ON rp.role_id = ur.role_id
    JOIN permission pm ON pm.id = rp.permission_id
    WHERE ur.user_id = sqlc.arg(user_id)
      AND pm.action = 'read' AND pm.resource_type = 'plant'
      AND (r.organization_id IS NULL OR r.organization_id = ur.organization_id)
      AND (rp.organization_id IS NULL OR rp.organization_id = ur.organization_id)
      AND (ur.organization_id IS NULL OR ur.organization_id = p.organization_id)
      AND (ur.plant_id IS NULL OR ur.plant_id = p.id)
)
ORDER BY o.name, p.name, p.id
LIMIT 200;

-- name: GetAuthorizedPlant :one
SELECT p.id, p.organization_id, o.name AS organization_name,
       p.code, p.name, p.timezone, p.latitude, p.longitude,
       p.installed_dc_kw,
       p.installed_ac_kw,
       p.is_active, p.created_at, p.updated_at
FROM plant p
JOIN organization o ON o.id = p.organization_id
WHERE p.id = sqlc.arg(plant_id)
  AND EXISTS (
      SELECT 1 FROM user_role ur
      JOIN role r ON r.id = ur.role_id
      JOIN role_permission rp ON rp.role_id = ur.role_id
      JOIN permission pm ON pm.id = rp.permission_id
      WHERE ur.user_id = sqlc.arg(user_id)
        AND pm.action = sqlc.arg(action) AND pm.resource_type = 'plant'
        AND (r.organization_id IS NULL OR r.organization_id = ur.organization_id)
        AND (rp.organization_id IS NULL OR rp.organization_id = ur.organization_id)
        AND (ur.organization_id IS NULL OR ur.organization_id = p.organization_id)
        AND (ur.plant_id IS NULL OR ur.plant_id = p.id)
  )
LIMIT 1;

-- name: GetAuthorizedPlantForUpdate :one
SELECT p.id, p.organization_id, o.name AS organization_name,
       p.code, p.name, p.timezone, p.latitude, p.longitude,
       p.installed_dc_kw,
       p.installed_ac_kw,
       p.is_active, p.created_at, p.updated_at
FROM plant p
JOIN organization o ON o.id = p.organization_id
WHERE p.id = sqlc.arg(plant_id)
  AND EXISTS (
      SELECT 1 FROM user_role ur
      JOIN role r ON r.id = ur.role_id
      JOIN role_permission rp ON rp.role_id = ur.role_id
      JOIN permission pm ON pm.id = rp.permission_id
      WHERE ur.user_id = sqlc.arg(user_id)
        AND pm.action = 'update' AND pm.resource_type = 'plant'
        AND (r.organization_id IS NULL OR r.organization_id = ur.organization_id)
        AND (rp.organization_id IS NULL OR rp.organization_id = ur.organization_id)
        AND (ur.organization_id IS NULL OR ur.organization_id = p.organization_id)
        AND (ur.plant_id IS NULL OR ur.plant_id = p.id)
  )
LIMIT 1
FOR UPDATE OF p;

-- name: CreatePlant :one
INSERT INTO plant (
    id, organization_id, code, name, timezone, latitude, longitude,
    installed_dc_kw, installed_ac_kw
) VALUES (
    sqlc.arg(id), sqlc.arg(organization_id), sqlc.arg(code), sqlc.arg(name), sqlc.arg(timezone),
    sqlc.narg(latitude)::double precision, sqlc.narg(longitude)::double precision,
    sqlc.narg(installed_dc_kw)::double precision, sqlc.narg(installed_ac_kw)::double precision
)
RETURNING id, organization_id, code, name, timezone, latitude, longitude,
          installed_dc_kw,
          installed_ac_kw,
          is_active, created_at, updated_at;

-- name: UpdatePlant :one
UPDATE plant
SET code = sqlc.arg(code), name = sqlc.arg(name), timezone = sqlc.arg(timezone),
    latitude = sqlc.narg(latitude)::double precision,
    longitude = sqlc.narg(longitude)::double precision,
    installed_dc_kw = sqlc.narg(installed_dc_kw)::double precision,
    installed_ac_kw = sqlc.narg(installed_ac_kw)::double precision,
    is_active = sqlc.arg(is_active), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, organization_id, code, name, timezone, latitude, longitude,
          installed_dc_kw,
          installed_ac_kw,
          is_active, created_at, updated_at;

-- name: CreateAuditEventFull :exec
INSERT INTO audit_log (
    organization_id, actor_user_id, action, target_type, target_id,
    before_data, after_data, source_ip, correlation_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(actor_user_id), sqlc.arg(action),
    sqlc.arg(target_type), sqlc.arg(target_id), sqlc.arg(before_data),
    sqlc.arg(after_data), sqlc.arg(source_ip), sqlc.arg(correlation_id)
);