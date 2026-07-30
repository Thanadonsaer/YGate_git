INSERT INTO permission (id, action, resource_type, description) VALUES
    ('00000000-0000-4000-8000-000000000183', 'manage_access', 'dashboard', 'Manage dashboard sharing');

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, id, '00000000-0000-4000-8000-000000000183'::uuid
FROM role
WHERE id IN (
    '00000000-0000-4000-8000-000000000201'::uuid,
    '00000000-0000-4000-8000-000000000202'::uuid,
    '00000000-0000-4000-8000-000000000203'::uuid
);

ALTER TABLE user_dashboard
    ADD COLUMN visibility text NOT NULL DEFAULT 'PRIVATE',
    ADD COLUMN access_version integer NOT NULL DEFAULT 1,
    ADD CONSTRAINT user_dashboard_visibility CHECK (visibility IN ('PRIVATE', 'ORGANIZATION')),
    ADD CONSTRAINT user_dashboard_access_version_positive CHECK (access_version > 0);

CREATE INDEX user_dashboard_organization_shared_idx
    ON user_dashboard (organization_id, updated_at DESC, id)
    WHERE visibility = 'ORGANIZATION' AND published_layouts IS NOT NULL;
