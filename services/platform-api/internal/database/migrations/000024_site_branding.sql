CREATE TABLE site_setting (
    id boolean PRIMARY KEY DEFAULT true,
    site_name text NOT NULL DEFAULT 'YGATE',
    logo_url text,
    accent_color text NOT NULL DEFAULT 'teal',
    updated_by uuid REFERENCES app_user(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT site_setting_singleton CHECK (id),
    CONSTRAINT site_setting_name_length CHECK (length(btrim(site_name)) BETWEEN 1 AND 100),
    CONSTRAINT site_setting_logo_url_length CHECK (logo_url IS NULL OR length(logo_url) <= 2048),
    CONSTRAINT site_setting_accent_valid CHECK (accent_color IN ('teal', 'indigo', 'emerald', 'amber', 'rose'))
);

INSERT INTO site_setting (id) VALUES (true);

INSERT INTO permission (id, action, resource_type, description) VALUES
    ('00000000-0000-4000-8000-000000000314', 'update', 'site_setting', 'Change the site logo, name, and accent color');

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, '00000000-0000-4000-8000-000000000201'::uuid, p.id
FROM permission p
WHERE p.resource_type = 'site_setting';
