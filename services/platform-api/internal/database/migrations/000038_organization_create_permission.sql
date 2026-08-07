-- Two things this migration needs that its first draft got wrong, both fatal
-- on a fresh database:
--   1. Schema: 000033_schema_namespacing.sql moved permission and
--      role_permission into the auth schema, so unqualified names fail.
--   2. Id: ...171 is already taken by 000029's 'read'/'session' permission.
--      Permission ids live in the ...1xx block (highest in use: ...199), so
--      this takes ...200. Nothing looks this permission up by id -- the code
--      resolves it by (action, resource_type) -- so the value only has to be
--      unique.
INSERT INTO auth.permission (id, action, resource_type, description) VALUES
    ('00000000-0000-4000-8000-000000000200', 'create', 'organization', 'Create new organizations');

INSERT INTO auth.role_permission (organization_id, role_id, permission_id)
VALUES (NULL, '00000000-0000-4000-8000-000000000201'::uuid, '00000000-0000-4000-8000-000000000200'::uuid);
