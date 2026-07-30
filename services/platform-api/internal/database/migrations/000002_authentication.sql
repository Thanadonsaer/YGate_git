CREATE TABLE app_user (
    id uuid PRIMARY KEY,
    organization_id uuid REFERENCES organization(id) ON DELETE RESTRICT,
    email text NOT NULL UNIQUE,
    username text UNIQUE,
    display_name text NOT NULL,
    password_hash text NOT NULL,
    status text NOT NULL DEFAULT 'ACTIVE',
    failed_login_count integer NOT NULL DEFAULT 0,
    locked_until timestamptz,
    password_changed_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT app_user_email_normalized CHECK (email = lower(email)),
    CONSTRAINT app_user_username_normalized CHECK (username IS NULL OR username = lower(username)),
    CONSTRAINT app_user_status_valid CHECK (status IN ('ACTIVE', 'DISABLED')),
    CONSTRAINT app_user_failed_login_nonnegative CHECK (failed_login_count >= 0),
    CONSTRAINT app_user_org_id_unique UNIQUE (organization_id, id)
);

CREATE TABLE role (
    id uuid PRIMARY KEY,
    organization_id uuid REFERENCES organization(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    is_system boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT role_org_id_unique UNIQUE (organization_id, id)
);

CREATE UNIQUE INDEX role_global_name_unique ON role (lower(name)) WHERE organization_id IS NULL;
CREATE UNIQUE INDEX role_org_name_unique ON role (organization_id, lower(name)) WHERE organization_id IS NOT NULL;

CREATE TABLE permission (
    id uuid PRIMARY KEY,
    action text NOT NULL,
    resource_type text NOT NULL,
    description text NOT NULL DEFAULT '',
    CONSTRAINT permission_action_resource_unique UNIQUE (action, resource_type)
);

CREATE TABLE role_permission (
    organization_id uuid REFERENCES organization(id) ON DELETE CASCADE,
    role_id uuid NOT NULL REFERENCES role(id) ON DELETE CASCADE,
    permission_id uuid NOT NULL REFERENCES permission(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE user_role (
    id uuid PRIMARY KEY,
    organization_id uuid REFERENCES organization(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    role_id uuid NOT NULL REFERENCES role(id) ON DELETE CASCADE,
    plant_id uuid REFERENCES plant(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX user_role_unscoped_unique ON user_role (user_id, role_id) WHERE plant_id IS NULL;
CREATE UNIQUE INDEX user_role_plant_unique ON user_role (user_id, role_id, plant_id) WHERE plant_id IS NOT NULL;

CREATE TABLE user_session (
    id uuid PRIMARY KEY,
    organization_id uuid REFERENCES organization(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    idle_expires_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    client_ip inet,
    user_agent text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT user_session_expiry_order CHECK (idle_expires_at <= expires_at)
);

CREATE TABLE password_reset_token (
    id uuid PRIMARY KEY,
    organization_id uuid REFERENCES organization(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    requested_ip inet,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE auth_attempt (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    identifier text NOT NULL,
    source_ip inet,
    success boolean NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE audit_log
    ADD CONSTRAINT audit_log_actor_fk FOREIGN KEY (actor_user_id) REFERENCES app_user(id) ON DELETE RESTRICT;

CREATE INDEX app_user_organization_idx ON app_user (organization_id, email);
CREATE INDEX user_session_user_active_idx ON user_session (user_id, expires_at) WHERE revoked_at IS NULL;
CREATE INDEX password_reset_user_active_idx ON password_reset_token (user_id, expires_at) WHERE used_at IS NULL;
CREATE INDEX auth_attempt_identifier_time_idx ON auth_attempt (identifier, created_at DESC) WHERE success = false;
CREATE INDEX auth_attempt_ip_time_idx ON auth_attempt (source_ip, created_at DESC) WHERE success = false;
