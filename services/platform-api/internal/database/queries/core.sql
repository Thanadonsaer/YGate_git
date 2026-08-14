-- name: HasOrganizationPermission :one
SELECT EXISTS (
    SELECT 1 FROM auth.user_role ur
    JOIN auth.role r ON r.id = ur.role_id
    JOIN auth.role_permission rp ON rp.role_id = ur.role_id
    JOIN auth.permission pm ON pm.id = rp.permission_id
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
    SELECT 1 FROM auth.user_role ur
    JOIN auth.role r ON r.id = ur.role_id
    JOIN auth.role_permission rp ON rp.role_id = ur.role_id
    JOIN auth.permission pm ON pm.id = rp.permission_id
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
       p.image_url,
       p.lifecycle_status, p.is_active, p.alarm_email_enabled, p.alarm_notify_role_id, p.created_at, p.updated_at
FROM plant.plant p
JOIN organization o ON o.id = p.organization_id
WHERE EXISTS (
    SELECT 1 FROM auth.user_role ur
    JOIN auth.role r ON r.id = ur.role_id
    JOIN auth.role_permission rp ON rp.role_id = ur.role_id
    JOIN auth.permission pm ON pm.id = rp.permission_id
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
       p.image_url,
       p.lifecycle_status, p.is_active, p.alarm_email_enabled, p.alarm_notify_role_id, p.created_at, p.updated_at
FROM plant.plant p
JOIN organization o ON o.id = p.organization_id
WHERE p.id = sqlc.arg(plant_id)
  AND EXISTS (
      SELECT 1 FROM auth.user_role ur
      JOIN auth.role r ON r.id = ur.role_id
      JOIN auth.role_permission rp ON rp.role_id = ur.role_id
      JOIN auth.permission pm ON pm.id = rp.permission_id
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
       p.image_url,
       p.lifecycle_status, p.is_active, p.alarm_email_enabled, p.alarm_notify_role_id, p.created_at, p.updated_at
FROM plant.plant p
JOIN organization o ON o.id = p.organization_id
WHERE p.id = sqlc.arg(plant_id)
  AND EXISTS (
      SELECT 1 FROM auth.user_role ur
      JOIN auth.role r ON r.id = ur.role_id
      JOIN auth.role_permission rp ON rp.role_id = ur.role_id
      JOIN auth.permission pm ON pm.id = rp.permission_id
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
INSERT INTO plant.plant (
    id, organization_id, code, name, timezone, latitude, longitude,
    installed_dc_kw, installed_ac_kw, lifecycle_status, alarm_email_enabled, alarm_notify_role_id
) VALUES (
    sqlc.arg(id), sqlc.arg(organization_id), sqlc.arg(code), sqlc.arg(name), sqlc.arg(timezone),
    sqlc.narg(latitude)::double precision, sqlc.narg(longitude)::double precision,
    sqlc.narg(installed_dc_kw)::double precision, sqlc.narg(installed_ac_kw)::double precision,
    sqlc.arg(lifecycle_status), sqlc.arg(alarm_email_enabled), sqlc.narg(alarm_notify_role_id)
)
RETURNING id, organization_id, code, name, timezone, latitude, longitude,
          installed_dc_kw,
          installed_ac_kw,
          image_url,
          lifecycle_status, is_active, alarm_email_enabled, alarm_notify_role_id, created_at, updated_at;

-- name: UpdatePlant :one
UPDATE plant.plant
SET code = sqlc.arg(code), name = sqlc.arg(name), timezone = sqlc.arg(timezone),
    latitude = sqlc.narg(latitude)::double precision,
    longitude = sqlc.narg(longitude)::double precision,
    installed_dc_kw = sqlc.narg(installed_dc_kw)::double precision,
    installed_ac_kw = sqlc.narg(installed_ac_kw)::double precision,
    lifecycle_status = sqlc.arg(lifecycle_status),
    is_active = sqlc.arg(is_active),
    alarm_email_enabled = sqlc.arg(alarm_email_enabled),
    alarm_notify_role_id = sqlc.narg(alarm_notify_role_id),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, organization_id, code, name, timezone, latitude, longitude,
          installed_dc_kw,
          installed_ac_kw,
          image_url,
          lifecycle_status, is_active, alarm_email_enabled, alarm_notify_role_id, created_at, updated_at;

-- name: ValidateAlarmNotifyRole :one
SELECT EXISTS (
    SELECT 1 FROM auth.role
    WHERE id = sqlc.arg(role_id)
      AND (organization_id IS NULL OR organization_id = sqlc.arg(organization_id))
)::boolean;

-- name: CreateAuditEventFull :exec
INSERT INTO audit_log (
    organization_id, actor_user_id, action, target_type, target_id,
    before_data, after_data, source_ip, correlation_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(actor_user_id), sqlc.arg(action),
    sqlc.arg(target_type), sqlc.arg(target_id), sqlc.arg(before_data),
    sqlc.arg(after_data), sqlc.arg(source_ip), sqlc.arg(correlation_id)
);
