INSERT INTO permission (id, action, resource_type, description) VALUES
    ('00000000-0000-4000-8000-000000000164', 'delete', 'role', 'Delete tenant roles that are not in use');

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, r.id, p.id
FROM role r
JOIN permission p ON p.resource_type = 'role' AND p.action = 'delete'
WHERE r.id IN (
    '00000000-0000-4000-8000-000000000201'::uuid,
    '00000000-0000-4000-8000-000000000202'::uuid
);
