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
