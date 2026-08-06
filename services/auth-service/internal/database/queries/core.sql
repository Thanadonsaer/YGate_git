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

-- name: ListUserPermissions :many
SELECT DISTINCT pm.resource_type, pm.action
FROM auth.user_role ur
JOIN auth.role r ON r.id = ur.role_id
JOIN auth.role_permission rp ON rp.role_id = ur.role_id
    AND (rp.organization_id IS NULL OR rp.organization_id = ur.organization_id)
JOIN auth.permission pm ON pm.id = rp.permission_id
WHERE ur.user_id = sqlc.arg(user_id)
  AND (r.organization_id IS NULL OR r.organization_id = ur.organization_id);

-- name: CreateAuditEventFull :exec
INSERT INTO audit_log (
    organization_id, actor_user_id, action, target_type, target_id,
    before_data, after_data, source_ip, correlation_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(actor_user_id), sqlc.arg(action),
    sqlc.arg(target_type), sqlc.arg(target_id), sqlc.arg(before_data),
    sqlc.arg(after_data), sqlc.arg(source_ip), sqlc.arg(correlation_id)
);
