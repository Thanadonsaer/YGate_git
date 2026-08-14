-- Separate a newly registered, unassigned account from an account disabled
-- by an administrator. Verification and access approval remain independent.
ALTER TABLE auth.app_user
    DROP CONSTRAINT app_user_status_valid;

ALTER TABLE auth.app_user
    ADD CONSTRAINT app_user_status_valid
    CHECK (status IN ('ACTIVE', 'PENDING_ACCESS', 'DISABLED'));

UPDATE auth.app_user
SET status = 'PENDING_ACCESS', updated_at = now()
WHERE status = 'DISABLED'
  AND organization_id IS NULL;
