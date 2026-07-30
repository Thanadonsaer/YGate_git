-- name: ListDashboardPlantStatus :many
SELECT p.id AS plant_id, p.code, p.name, p.timezone, p.is_active,
       count(d.id)::bigint AS device_count,
       count(d.id) FILTER (WHERE d.is_active)::bigint AS active_device_count,
       count(tl.device_id) FILTER (WHERE d.is_active)::bigint AS reporting_device_count,
       count(tl.device_id) FILTER (WHERE d.is_active AND tl.observed_at < sqlc.arg(stale_before))::bigint AS stale_device_count,
       count(d.id) FILTER (WHERE d.is_active AND tl.device_id IS NULL)::bigint AS offline_device_count,
       max(tl.observed_at) FILTER (WHERE d.is_active) AS last_observed_at
FROM plant p
LEFT JOIN device d ON d.organization_id = p.organization_id AND d.plant_id = p.id
LEFT JOIN telemetry_latest tl ON tl.organization_id = d.organization_id AND tl.device_id = d.id
WHERE EXISTS (
    SELECT 1 FROM user_role ur
    JOIN role r ON r.id = ur.role_id
    JOIN role_permission rp ON rp.role_id = ur.role_id
    JOIN permission pm ON pm.id = rp.permission_id
    WHERE ur.user_id = sqlc.arg(user_id)
      AND pm.action = 'read' AND pm.resource_type = 'device'
      AND (r.organization_id IS NULL OR r.organization_id = ur.organization_id)
      AND (rp.organization_id IS NULL OR rp.organization_id = ur.organization_id)
      AND (ur.organization_id IS NULL OR ur.organization_id = p.organization_id)
      AND (ur.plant_id IS NULL OR ur.plant_id = p.id)
)
GROUP BY p.id, p.code, p.name, p.timezone, p.is_active
ORDER BY p.name, p.code, p.id
LIMIT 200;

-- name: GetUserDashboard :one
SELECT id, organization_id, owner_user_id, name, layouts, version,
       widget_configs, published_layouts, published_widget_configs,
       published_version, published_from_version, published_at, visibility, access_version,
       created_at, updated_at
FROM user_dashboard
WHERE organization_id = sqlc.arg(organization_id)
  AND owner_user_id = sqlc.arg(owner_user_id)
LIMIT 1;

-- name: GetUserDashboardForUpdate :one
SELECT id, organization_id, owner_user_id, name, layouts, version,
       widget_configs, published_layouts, published_widget_configs,
       published_version, published_from_version, published_at, visibility, access_version,
       created_at, updated_at
FROM user_dashboard
WHERE organization_id = sqlc.arg(organization_id)
  AND owner_user_id = sqlc.arg(owner_user_id)
LIMIT 1
FOR UPDATE;

-- name: CreateUserDashboard :one
INSERT INTO user_dashboard (id, organization_id, owner_user_id, layouts, widget_configs)
VALUES (sqlc.arg(id), sqlc.arg(organization_id), sqlc.arg(owner_user_id), sqlc.arg(layouts), sqlc.arg(widget_configs))
RETURNING id, organization_id, owner_user_id, name, layouts, version,
          widget_configs, published_layouts, published_widget_configs,
          published_version, published_from_version, published_at, visibility, access_version,
          created_at, updated_at;

-- name: UpdateUserDashboard :one
UPDATE user_dashboard
SET layouts = sqlc.arg(layouts), widget_configs = sqlc.arg(widget_configs),
    version = version + 1, updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, organization_id, owner_user_id, name, layouts, version,
          widget_configs, published_layouts, published_widget_configs,
          published_version, published_from_version, published_at, visibility, access_version,
          created_at, updated_at;

-- name: PublishUserDashboard :one
UPDATE user_dashboard
SET published_layouts = layouts,
    published_widget_configs = widget_configs,
    published_version = published_version + 1,
    published_from_version = version,
    published_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, organization_id, owner_user_id, name, layouts, version,
          widget_configs, published_layouts, published_widget_configs,
          published_version, published_from_version, published_at, visibility, access_version,
          created_at, updated_at;

-- name: ListOrganizationSharedDashboards :many
SELECT ud.id, ud.name, u.display_name AS owner_display_name,
       ud.published_version, ud.published_at
FROM user_dashboard ud
JOIN app_user u ON u.organization_id = ud.organization_id AND u.id = ud.owner_user_id
WHERE ud.organization_id = sqlc.arg(organization_id)
  AND ud.owner_user_id <> sqlc.arg(viewer_user_id)
  AND ud.visibility = 'ORGANIZATION'
  AND ud.published_layouts IS NOT NULL
ORDER BY ud.updated_at DESC, ud.id
LIMIT 100;

-- name: GetOrganizationSharedDashboard :one
SELECT id, organization_id, owner_user_id, name, layouts, version,
       widget_configs, published_layouts, published_widget_configs,
       published_version, published_from_version, published_at, visibility, access_version,
       created_at, updated_at
FROM user_dashboard
WHERE id = sqlc.arg(id)
  AND organization_id = sqlc.arg(organization_id)
  AND owner_user_id <> sqlc.arg(viewer_user_id)
  AND visibility = 'ORGANIZATION'
  AND published_layouts IS NOT NULL
LIMIT 1;

-- name: UpdateUserDashboardSharing :one
UPDATE user_dashboard
SET visibility = sqlc.arg(visibility), access_version = access_version + 1, updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, organization_id, owner_user_id, name, layouts, version,
          widget_configs, published_layouts, published_widget_configs,
          published_version, published_from_version, published_at, visibility, access_version,
          created_at, updated_at;
