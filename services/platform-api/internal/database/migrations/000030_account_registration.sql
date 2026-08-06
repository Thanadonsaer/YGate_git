ALTER TABLE app_user ADD COLUMN email_verified_at timestamptz;

UPDATE app_user SET email_verified_at = created_at WHERE email_verified_at IS NULL;

CREATE TABLE email_verification_token (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    user_id uuid NOT NULL,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    requested_ip inet,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT email_verification_token_user_fk FOREIGN KEY (organization_id, user_id) REFERENCES app_user(organization_id, id) ON DELETE CASCADE
);

CREATE INDEX email_verification_token_user_idx ON email_verification_token(user_id, expires_at);