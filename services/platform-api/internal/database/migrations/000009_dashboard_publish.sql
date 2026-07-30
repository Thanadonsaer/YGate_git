INSERT INTO permission (id, action, resource_type, description) VALUES
    ('00000000-0000-4000-8000-000000000182', 'publish', 'dashboard', 'Publish private dashboard layouts');

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, id, '00000000-0000-4000-8000-000000000182'::uuid
FROM role
WHERE id IN (
    '00000000-0000-4000-8000-000000000201'::uuid,
    '00000000-0000-4000-8000-000000000202'::uuid,
    '00000000-0000-4000-8000-000000000203'::uuid
);

ALTER TABLE user_dashboard
    ADD COLUMN published_layouts jsonb,
    ADD COLUMN published_version integer NOT NULL DEFAULT 0,
    ADD COLUMN published_from_version integer NOT NULL DEFAULT 0,
    ADD COLUMN published_at timestamptz;

UPDATE user_dashboard
SET published_layouts = layouts,
    published_version = 1,
    published_from_version = version,
    published_at = updated_at;

ALTER TABLE user_dashboard
    ADD CONSTRAINT user_dashboard_published_state CHECK (
        (published_layouts IS NULL AND published_version = 0 AND published_from_version = 0 AND published_at IS NULL)
        OR
        (jsonb_typeof(published_layouts) = 'object' AND published_version > 0 AND published_from_version > 0 AND published_at IS NOT NULL)
    );
