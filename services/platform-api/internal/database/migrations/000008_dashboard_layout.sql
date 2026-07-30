INSERT INTO permission (id, action, resource_type, description) VALUES
    ('00000000-0000-4000-8000-000000000180', 'read',   'dashboard', 'Read private dashboard layouts'),
    ('00000000-0000-4000-8000-000000000181', 'update', 'dashboard', 'Create or update private dashboard layouts');

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, role_id, permission_id
FROM (
    SELECT r.id AS role_id, p.id AS permission_id
    FROM role r
    CROSS JOIN permission p
    WHERE r.id IN (
        '00000000-0000-4000-8000-000000000201'::uuid,
        '00000000-0000-4000-8000-000000000202'::uuid,
        '00000000-0000-4000-8000-000000000203'::uuid,
        '00000000-0000-4000-8000-000000000204'::uuid
    )
      AND p.resource_type = 'dashboard'
) grants;

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, id, '00000000-0000-4000-8000-000000000180'::uuid
FROM role
WHERE id IN (
    '00000000-0000-4000-8000-000000000205'::uuid,
    '00000000-0000-4000-8000-000000000206'::uuid
);

CREATE TABLE user_dashboard (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    owner_user_id uuid NOT NULL,
    name text NOT NULL DEFAULT 'Overview',
    layouts jsonb NOT NULL,
    version integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT user_dashboard_owner_fk FOREIGN KEY (organization_id, owner_user_id)
        REFERENCES app_user(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT user_dashboard_name_length CHECK (length(btrim(name)) BETWEEN 1 AND 100),
    CONSTRAINT user_dashboard_layouts_object CHECK (jsonb_typeof(layouts) = 'object'),
    CONSTRAINT user_dashboard_version_positive CHECK (version > 0),
    CONSTRAINT user_dashboard_owner_unique UNIQUE (organization_id, owner_user_id)
);
