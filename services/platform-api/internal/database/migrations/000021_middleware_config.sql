ALTER TABLE middleware_client
    ADD COLUMN site_name text NOT NULL DEFAULT '',
    ADD COLUMN config_version bigint NOT NULL DEFAULT 0,
    ADD COLUMN config_applied_version bigint NOT NULL DEFAULT 0,
    ADD COLUMN config_snapshot jsonb NOT NULL DEFAULT '{}',
    ADD CONSTRAINT middleware_client_config_snapshot_object CHECK (jsonb_typeof(config_snapshot) = 'object');

CREATE TABLE middleware_config_history (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    middleware_client_id uuid NOT NULL,
    version bigint NOT NULL,
    actor_user_id uuid,
    snapshot jsonb NOT NULL,
    push_status text NOT NULL DEFAULT 'PENDING',
    push_reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    acked_at timestamptz,
    CONSTRAINT middleware_config_history_client_fk FOREIGN KEY (organization_id, middleware_client_id)
        REFERENCES middleware_client(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT middleware_config_history_actor_fk FOREIGN KEY (organization_id, actor_user_id)
        REFERENCES app_user(organization_id, id) ON DELETE SET NULL,
    CONSTRAINT middleware_config_history_snapshot_object CHECK (jsonb_typeof(snapshot) = 'object'),
    CONSTRAINT middleware_config_history_status_valid CHECK (push_status IN ('PENDING', 'SENT', 'APPLIED', 'FAILED')),
    CONSTRAINT middleware_config_history_version_positive CHECK (version > 0),
    CONSTRAINT middleware_config_history_unique UNIQUE (middleware_client_id, version)
);

CREATE INDEX middleware_config_history_client_idx
    ON middleware_config_history (organization_id, middleware_client_id, version DESC);

INSERT INTO permission (id, action, resource_type, description) VALUES
    ('00000000-0000-4000-8000-000000000300', 'read',   'middleware_config', 'Read middleware gateway configuration and run live device tests'),
    ('00000000-0000-4000-8000-000000000301', 'update', 'middleware_config', 'Replace middleware gateway configuration');

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, r.id, p.id
FROM role r
JOIN permission p ON p.resource_type = 'middleware_config'
WHERE r.id IN (
    '00000000-0000-4000-8000-000000000201'::uuid,
    '00000000-0000-4000-8000-000000000202'::uuid
);
