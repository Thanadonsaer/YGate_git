-- One definition of "does this user hold this permission for this scope".
-- Every authorization check in platform-api calls this instead of repeating
-- the user_role/role/role_permission/permission join, so scope rules live in
-- exactly one place.
--
-- Scope is expressed by which arguments are NULL:
--   org + plant  -> org-wide roles in that org, global roles, and plant roles for that plant
--   org, no plant -> org-wide roles in that org and global roles (plant roles never count)
--   neither       -> global roles only (role, grant and assignment all organization-less)
CREATE FUNCTION auth.has_permission(
    p_user_id uuid,
    p_action text,
    p_resource_type text,
    p_organization_id uuid,
    p_plant_id uuid
) RETURNS boolean
LANGUAGE sql STABLE
AS $$
    SELECT EXISTS (
        SELECT 1 FROM auth.user_role ur
        JOIN auth.role r ON r.id = ur.role_id
        JOIN auth.role_permission rp ON rp.role_id = ur.role_id
        JOIN auth.permission pm ON pm.id = rp.permission_id
        WHERE ur.user_id = p_user_id
          AND pm.action = p_action
          AND pm.resource_type = p_resource_type
          AND (r.organization_id IS NULL OR r.organization_id = ur.organization_id)
          AND (rp.organization_id IS NULL OR rp.organization_id = ur.organization_id)
          AND (ur.organization_id IS NULL OR ur.organization_id = p_organization_id)
          AND (ur.plant_id IS NULL OR ur.plant_id = p_plant_id)
    )
$$;
