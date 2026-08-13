-- Self-registration creates an unassigned account. The administrator assigns
-- its organization and baseline role after email verification.
ALTER TABLE auth.email_verification_token
    DROP CONSTRAINT email_verification_token_user_fk;

ALTER TABLE auth.email_verification_token
    ALTER COLUMN organization_id DROP NOT NULL;

ALTER TABLE auth.email_verification_token
    ADD CONSTRAINT email_verification_token_user_fk
    FOREIGN KEY (organization_id, user_id) REFERENCES auth.app_user(organization_id, id) ON DELETE CASCADE;
