ALTER TABLE plant
    ADD CONSTRAINT plant_installed_dc_nonnegative CHECK (installed_dc_kw IS NULL OR installed_dc_kw >= 0),
    ADD CONSTRAINT plant_installed_ac_nonnegative CHECK (installed_ac_kw IS NULL OR installed_ac_kw >= 0);

ALTER TABLE user_role
    ADD CONSTRAINT user_role_plant_requires_organization CHECK (plant_id IS NULL OR organization_id IS NOT NULL),
    ADD CONSTRAINT user_role_user_scope_fk FOREIGN KEY (organization_id, user_id)
        REFERENCES app_user(organization_id, id) ON DELETE CASCADE,
    ADD CONSTRAINT user_role_plant_scope_fk FOREIGN KEY (organization_id, plant_id)
        REFERENCES plant(organization_id, id) ON DELETE CASCADE;

CREATE INDEX user_role_user_scope_idx ON user_role (user_id, organization_id, plant_id, role_id);
CREATE INDEX role_permission_permission_idx ON role_permission (permission_id, role_id);

INSERT INTO permission (id, action, resource_type, description) VALUES
    ('00000000-0000-4000-8000-000000000101', 'read',           'organization', 'Read organization master data'),
    ('00000000-0000-4000-8000-000000000102', 'update',         'organization', 'Update organization master data'),
    ('00000000-0000-4000-8000-000000000110', 'read',           'plant',        'Read plant master data'),
    ('00000000-0000-4000-8000-000000000111', 'create',         'plant',        'Create plant master data'),
    ('00000000-0000-4000-8000-000000000112', 'update',         'plant',        'Update or disable plant master data'),
    ('00000000-0000-4000-8000-000000000120', 'read',           'asset_group',  'Read asset groups'),
    ('00000000-0000-4000-8000-000000000121', 'create',         'asset_group',  'Create asset groups'),
    ('00000000-0000-4000-8000-000000000122', 'update',         'asset_group',  'Update asset groups'),
    ('00000000-0000-4000-8000-000000000130', 'read',           'device_model', 'Read device models'),
    ('00000000-0000-4000-8000-000000000131', 'create',         'device_model', 'Create device models'),
    ('00000000-0000-4000-8000-000000000132', 'update',         'device_model', 'Update device models'),
    ('00000000-0000-4000-8000-000000000140', 'read',           'device',       'Read devices'),
    ('00000000-0000-4000-8000-000000000141', 'create',         'device',       'Create devices'),
    ('00000000-0000-4000-8000-000000000142', 'update',         'device',       'Update or disable devices'),
    ('00000000-0000-4000-8000-000000000150', 'read',           'user',         'Read users'),
    ('00000000-0000-4000-8000-000000000151', 'create',         'user',         'Create users'),
    ('00000000-0000-4000-8000-000000000152', 'update',         'user',         'Update users'),
    ('00000000-0000-4000-8000-000000000153', 'disable',        'user',         'Disable users'),
    ('00000000-0000-4000-8000-000000000154', 'unlock',         'user',         'Unlock users'),
    ('00000000-0000-4000-8000-000000000155', 'reset_password', 'user',         'Issue an administrative reset'),
    ('00000000-0000-4000-8000-000000000160', 'read',           'role',         'Read roles and grants'),
    ('00000000-0000-4000-8000-000000000161', 'create',         'role',         'Create tenant roles'),
    ('00000000-0000-4000-8000-000000000162', 'update',         'role',         'Update tenant roles'),
    ('00000000-0000-4000-8000-000000000163', 'assign',         'role',         'Assign roles within granted scope'),
    ('00000000-0000-4000-8000-000000000170', 'read',           'audit',        'Read append-only audit events');

INSERT INTO role (id, organization_id, name, description, is_system) VALUES
    ('00000000-0000-4000-8000-000000000201', NULL, 'Platform Admin',     'Platform-wide administration', true),
    ('00000000-0000-4000-8000-000000000202', NULL, 'Organization Admin', 'Organization administration', true),
    ('00000000-0000-4000-8000-000000000203', NULL, 'Plant Manager',      'Plant configuration and operations', true),
    ('00000000-0000-4000-8000-000000000204', NULL, 'Engineer',           'Engineering configuration and analysis', true),
    ('00000000-0000-4000-8000-000000000205', NULL, 'Operator',           'Read-only monitoring and operations', true),
    ('00000000-0000-4000-8000-000000000206', NULL, 'Viewer',             'Read-only plant visibility', true),
    ('00000000-0000-4000-8000-000000000207', NULL, 'Auditor',            'Read-only configuration and audit visibility', true);

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, '00000000-0000-4000-8000-000000000201'::uuid, id FROM permission;

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, '00000000-0000-4000-8000-000000000202'::uuid, id
FROM permission
WHERE NOT (resource_type = 'organization' AND action = 'create');

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, '00000000-0000-4000-8000-000000000203'::uuid, id
FROM permission
WHERE (resource_type = 'plant' AND action IN ('read', 'update'))
   OR resource_type IN ('asset_group', 'device_model', 'device')
   OR (resource_type = 'audit' AND action = 'read');

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, '00000000-0000-4000-8000-000000000204'::uuid, id
FROM permission
WHERE (resource_type IN ('plant', 'asset_group', 'device_model', 'device') AND action IN ('read', 'update'));

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, role_id, permission_id
FROM (
    SELECT r.id AS role_id, p.id AS permission_id
    FROM role r
    CROSS JOIN permission p
    WHERE r.id IN (
        '00000000-0000-4000-8000-000000000205'::uuid,
        '00000000-0000-4000-8000-000000000206'::uuid
    )
      AND p.action = 'read'
      AND p.resource_type IN ('organization', 'plant', 'asset_group', 'device_model', 'device')
) grants;

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, '00000000-0000-4000-8000-000000000207'::uuid, id
FROM permission
WHERE action = 'read' AND resource_type IN ('organization', 'plant', 'audit');