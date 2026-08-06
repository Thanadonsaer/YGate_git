-- Reduce the built-in, unmodifiable ("system") roles down to exactly one:
-- Platform Admin, renamed System Admin, kept as the single immutable
-- superuser role (every permission, is_system stays true).
--
-- The other six baseline roles (Organization Admin, Plant Manager, Engineer,
-- Operator, Viewer, Auditor) become ordinary editable/deletable roles
-- (is_system=false). They keep their existing name, permissions, and any
-- user_role assignments unchanged — only the "cannot edit/delete" protection
-- is lifted. Admins can now rename, reshape, or remove them from the Roles &
-- Permissions page like any custom role.
UPDATE role SET name = 'System Admin', updated_at = now()
WHERE id = '00000000-0000-4000-8000-000000000201'::uuid AND organization_id IS NULL;

UPDATE role SET is_system = false, updated_at = now()
WHERE id IN (
    '00000000-0000-4000-8000-000000000202'::uuid,
    '00000000-0000-4000-8000-000000000203'::uuid,
    '00000000-0000-4000-8000-000000000204'::uuid,
    '00000000-0000-4000-8000-000000000205'::uuid,
    '00000000-0000-4000-8000-000000000206'::uuid,
    '00000000-0000-4000-8000-000000000207'::uuid
) AND organization_id IS NULL;
