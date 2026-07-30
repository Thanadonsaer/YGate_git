INSERT INTO permission (id, action, resource_type, description) VALUES
    ('00000000-0000-4000-8000-000000000190', 'read',   'api_contract',      'Read OpenAPI contracts'),
    ('00000000-0000-4000-8000-000000000191', 'read',   'middleware_client', 'Read Middleware API keys'),
    ('00000000-0000-4000-8000-000000000192', 'create', 'middleware_client', 'Create Middleware API keys'),
    ('00000000-0000-4000-8000-000000000193', 'update', 'middleware_client', 'Update Middleware API keys');

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, role.id, permission.id
FROM role
CROSS JOIN permission
WHERE role.id IN (
    '00000000-0000-4000-8000-000000000201'::uuid,
    '00000000-0000-4000-8000-000000000202'::uuid
)
AND permission.id IN (
    '00000000-0000-4000-8000-000000000190'::uuid,
    '00000000-0000-4000-8000-000000000191'::uuid,
    '00000000-0000-4000-8000-000000000192'::uuid,
    '00000000-0000-4000-8000-000000000193'::uuid
);
