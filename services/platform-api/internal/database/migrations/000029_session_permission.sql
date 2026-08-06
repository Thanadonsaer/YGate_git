INSERT INTO permission (id, action, resource_type, description)
VALUES ('00000000-0000-4000-8000-000000000171', 'read', 'session', 'View and manage your own sessions')
ON CONFLICT (id) DO NOTHING;

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, '00000000-0000-4000-8000-000000000201'::uuid, id
FROM permission
WHERE id = '00000000-0000-4000-8000-000000000171'::uuid
  AND NOT EXISTS (
    SELECT 1 FROM role_permission
    WHERE organization_id IS NULL
      AND role_id = '00000000-0000-4000-8000-000000000201'::uuid
      AND permission_id = id
  );