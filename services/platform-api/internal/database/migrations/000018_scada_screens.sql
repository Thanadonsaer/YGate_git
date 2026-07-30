INSERT INTO permission (id, action, resource_type, description) VALUES
    ('00000000-0000-4000-8000-000000000210', 'view',          'scada_screen', 'View published and draft SCADA screens within scope'),
    ('00000000-0000-4000-8000-000000000211', 'edit',          'scada_screen', 'Create and edit SCADA drafts within scope'),
    ('00000000-0000-4000-8000-000000000212', 'publish',       'scada_screen', 'Publish and roll back SCADA screens within scope'),
    ('00000000-0000-4000-8000-000000000213', 'manage_access', 'scada_screen', 'Manage SCADA screen access within scope'),
    ('00000000-0000-4000-8000-000000000214', 'hard_delete',   'scada_screen', 'Permanently delete SCADA screens and publication history');

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, r.id, p.id
FROM role r
JOIN permission p ON p.resource_type='scada_screen'
WHERE r.id IN (
    '00000000-0000-4000-8000-000000000201'::uuid,
    '00000000-0000-4000-8000-000000000202'::uuid,
    '00000000-0000-4000-8000-000000000203'::uuid,
    '00000000-0000-4000-8000-000000000204'::uuid,
    '00000000-0000-4000-8000-000000000205'::uuid,
    '00000000-0000-4000-8000-000000000206'::uuid,
    '00000000-0000-4000-8000-000000000207'::uuid
)
  AND (
      p.action='view'
      OR p.action='edit' AND r.id IN (
          '00000000-0000-4000-8000-000000000201'::uuid,
          '00000000-0000-4000-8000-000000000202'::uuid,
          '00000000-0000-4000-8000-000000000203'::uuid,
          '00000000-0000-4000-8000-000000000204'::uuid
      )
      OR p.action IN ('publish','manage_access') AND r.id IN (
          '00000000-0000-4000-8000-000000000201'::uuid,
          '00000000-0000-4000-8000-000000000202'::uuid,
          '00000000-0000-4000-8000-000000000203'::uuid
      )
      OR p.action='hard_delete' AND r.id='00000000-0000-4000-8000-000000000201'::uuid
  );

CREATE TABLE scada_screen (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    plant_id uuid NOT NULL,
    name text NOT NULL,
    draft_design jsonb NOT NULL DEFAULT '{"version":1,"nodes":[],"edges":[],"viewport":{"x":0,"y":0,"zoom":1}}',
    draft_version integer NOT NULL DEFAULT 1,
    published_version integer NOT NULL DEFAULT 0,
    created_by uuid REFERENCES app_user(id) ON DELETE SET NULL,
    updated_by uuid REFERENCES app_user(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT scada_screen_plant_fk FOREIGN KEY (organization_id, plant_id)
        REFERENCES plant(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT scada_screen_org_plant_id_unique UNIQUE (organization_id, plant_id, id),
    CONSTRAINT scada_screen_name_length CHECK (length(btrim(name)) BETWEEN 1 AND 100),
    CONSTRAINT scada_screen_draft_object CHECK (jsonb_typeof(draft_design)='object'),
    CONSTRAINT scada_screen_draft_version_positive CHECK (draft_version > 0),
    CONSTRAINT scada_screen_published_version_nonnegative CHECK (published_version >= 0)
);

CREATE UNIQUE INDEX scada_screen_plant_name_unique
ON scada_screen (organization_id, plant_id, lower(name));

CREATE INDEX scada_screen_plant_updated_idx
ON scada_screen (organization_id, plant_id, updated_at DESC, id);

CREATE TABLE scada_screen_publication (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    plant_id uuid NOT NULL,
    screen_id uuid NOT NULL,
    version integer NOT NULL,
    source_draft_version integer NOT NULL,
    design jsonb NOT NULL,
    published_by uuid NOT NULL,
    published_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT scada_publication_screen_fk FOREIGN KEY (organization_id, plant_id, screen_id)
        REFERENCES scada_screen(organization_id, plant_id, id) ON DELETE CASCADE,
    CONSTRAINT scada_publication_version_positive CHECK (version > 0 AND source_draft_version > 0),
    CONSTRAINT scada_publication_design_object CHECK (jsonb_typeof(design)='object'),
    CONSTRAINT scada_publication_screen_version_unique UNIQUE (screen_id, version)
);

CREATE FUNCTION reject_scada_publication_update() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'published SCADA versions are immutable';
END;
$$;

CREATE TRIGGER scada_publication_immutable
BEFORE UPDATE ON scada_screen_publication
FOR EACH ROW EXECUTE FUNCTION reject_scada_publication_update();

CREATE INDEX scada_publication_screen_time_idx
ON scada_screen_publication (screen_id, version DESC);
