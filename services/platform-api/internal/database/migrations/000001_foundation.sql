CREATE TABLE organization (
    id uuid PRIMARY KEY,
    code text NOT NULL UNIQUE,
    name text NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE plant (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organization(id) ON DELETE RESTRICT,
    code text NOT NULL,
    name text NOT NULL,
    timezone text NOT NULL,
    latitude double precision,
    longitude double precision,
    installed_dc_kw numeric(18,3),
    installed_ac_kw numeric(18,3),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT plant_latitude_range CHECK (latitude IS NULL OR latitude BETWEEN -90 AND 90),
    CONSTRAINT plant_longitude_range CHECK (longitude IS NULL OR longitude BETWEEN -180 AND 180),
    CONSTRAINT plant_org_code_unique UNIQUE (organization_id, code),
    CONSTRAINT plant_org_id_unique UNIQUE (organization_id, id)
);

CREATE TABLE asset_group (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    plant_id uuid NOT NULL,
    parent_id uuid,
    group_type text NOT NULL,
    code text NOT NULL,
    name text NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT asset_group_plant_fk FOREIGN KEY (organization_id, plant_id)
        REFERENCES plant(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT asset_group_parent_fk FOREIGN KEY (organization_id, plant_id, parent_id)
        REFERENCES asset_group(organization_id, plant_id, id) ON DELETE RESTRICT,
    CONSTRAINT asset_group_org_plant_code_unique UNIQUE (organization_id, plant_id, code),
    CONSTRAINT asset_group_org_plant_id_unique UNIQUE (organization_id, plant_id, id)
);

CREATE TABLE device_model (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organization(id) ON DELETE RESTRICT,
    manufacturer text NOT NULL,
    model text NOT NULL,
    device_type text NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT device_model_org_model_unique UNIQUE (organization_id, manufacturer, model),
    CONSTRAINT device_model_org_id_unique UNIQUE (organization_id, id)
);

CREATE TABLE device (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    plant_id uuid NOT NULL,
    asset_group_id uuid,
    device_model_id uuid NOT NULL,
    external_id text NOT NULL,
    name text NOT NULL,
    serial_number text,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT device_plant_fk FOREIGN KEY (organization_id, plant_id)
        REFERENCES plant(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT device_asset_group_fk FOREIGN KEY (organization_id, plant_id, asset_group_id)
        REFERENCES asset_group(organization_id, plant_id, id) ON DELETE RESTRICT,
    CONSTRAINT device_model_fk FOREIGN KEY (organization_id, device_model_id)
        REFERENCES device_model(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT device_org_plant_external_unique UNIQUE (organization_id, plant_id, external_id),
    CONSTRAINT device_org_plant_id_unique UNIQUE (organization_id, plant_id, id)
);

CREATE TABLE audit_log (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id uuid REFERENCES organization(id) ON DELETE RESTRICT,
    actor_user_id uuid,
    action text NOT NULL,
    target_type text NOT NULL,
    target_id uuid,
    before_data jsonb,
    after_data jsonb,
    source_ip inet,
    correlation_id uuid,
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE FUNCTION reject_audit_log_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_log rows are append-only';
END;
$$;

CREATE TRIGGER audit_log_append_only
BEFORE UPDATE OR DELETE ON audit_log
FOR EACH ROW EXECUTE FUNCTION reject_audit_log_mutation();

CREATE INDEX plant_organization_idx ON plant (organization_id, name);
CREATE INDEX asset_group_plant_idx ON asset_group (organization_id, plant_id, parent_id);
CREATE INDEX device_plant_idx ON device (organization_id, plant_id, name);
CREATE INDEX audit_log_org_time_idx ON audit_log (organization_id, occurred_at DESC);
