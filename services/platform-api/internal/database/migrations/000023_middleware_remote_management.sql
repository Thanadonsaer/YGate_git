ALTER TABLE middleware_client
    ADD COLUMN software_version text;

CREATE TABLE middleware_patch (
    id uuid PRIMARY KEY,
    version text NOT NULL,
    os text NOT NULL,
    arch text NOT NULL,
    binary_filename text NOT NULL,
    sha256 text NOT NULL,
    file_size_bytes bigint NOT NULL,
    storage_path text NOT NULL,
    uploaded_by uuid REFERENCES app_user(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT middleware_patch_sha256_hex CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT middleware_patch_unique_target UNIQUE (version, os, arch)
);

INSERT INTO permission (id, action, resource_type, description) VALUES
    ('00000000-0000-4000-8000-000000000310', 'create', 'middleware_patch', 'Upload a middleware software patch'),
    ('00000000-0000-4000-8000-000000000311', 'read',   'middleware_patch', 'List/download middleware software patches'),
    ('00000000-0000-4000-8000-000000000312', 'delete', 'middleware_patch', 'Delete an uploaded middleware software patch'),
    ('00000000-0000-4000-8000-000000000313', 'update', 'middleware_patch', 'Stage/apply/rollback a middleware software update, or restart the service');

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, '00000000-0000-4000-8000-000000000201'::uuid, p.id
FROM permission p
WHERE p.resource_type = 'middleware_patch';
