-- Audit clear is a view boundary, not a mutation of append-only source rows.
-- Only the global Platform Admin role may append the clear marker.
INSERT INTO permission (id, action, resource_type, description) VALUES
    ('00000000-0000-4000-8000-000000000199', 'clear', 'audit', 'Clear the operational audit view while retaining immutable source rows');

INSERT INTO role_permission (organization_id, role_id, permission_id)
VALUES (NULL, '00000000-0000-4000-8000-000000000201'::uuid, '00000000-0000-4000-8000-000000000199'::uuid);
