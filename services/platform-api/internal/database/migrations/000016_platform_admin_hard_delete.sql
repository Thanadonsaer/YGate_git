-- Hard delete is intentionally separate from ordinary update/disable actions.
-- Only the global Platform Admin role receives these permissions.
INSERT INTO permission (id, action, resource_type, description) VALUES
    ('00000000-0000-4000-8000-000000000194', 'hard_delete', 'user',              'Permanently delete users and owned dependent records'),
    ('00000000-0000-4000-8000-000000000195', 'hard_delete', 'plant',             'Permanently delete Plants and dependent operational records'),
    ('00000000-0000-4000-8000-000000000196', 'hard_delete', 'device',            'Permanently delete Devices and dependent telemetry'),
    ('00000000-0000-4000-8000-000000000197', 'hard_delete', 'device_model',      'Permanently delete unused Device Models and metadata'),
    ('00000000-0000-4000-8000-000000000198', 'hard_delete', 'middleware_client', 'Permanently delete Middleware API key clients and ingestion history');

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, '00000000-0000-4000-8000-000000000201'::uuid, id
FROM permission
WHERE action = 'hard_delete';

-- Audit rows remain immutable and retain the deleted actor UUID as historical
-- identity. A foreign key would block hard delete; ON DELETE SET NULL would
-- attempt to update append-only audit rows and is therefore also invalid.
ALTER TABLE audit_log DROP CONSTRAINT audit_log_actor_fk;

-- Forward-only repair guidance: if hard delete is later disabled, first prove
-- every actor UUID exists, then add a new NOT VALID FK and validate it online.
