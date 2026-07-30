ALTER TABLE device_model
    ADD COLUMN source_type_id integer,
    ADD CONSTRAINT device_model_source_type_nonnegative CHECK (source_type_id IS NULL OR source_type_id >= 0);

ALTER TABLE device
    ADD COLUMN source_metadata jsonb NOT NULL DEFAULT '{}',
    ADD CONSTRAINT device_source_metadata_object CHECK (jsonb_typeof(source_metadata) = 'object');

CREATE TABLE middleware_client (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organization(id) ON DELETE RESTRICT,
    name text NOT NULL,
    key_prefix text NOT NULL,
    key_hash bytea NOT NULL UNIQUE,
    auto_onboard boolean NOT NULL DEFAULT false,
    is_active boolean NOT NULL DEFAULT true,
    last_seen_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT middleware_client_name_length CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    CONSTRAINT middleware_client_prefix_length CHECK (length(key_prefix) BETWEEN 4 AND 32),
    CONSTRAINT middleware_client_org_name_unique UNIQUE (organization_id, name),
    CONSTRAINT middleware_client_org_id_unique UNIQUE (organization_id, id)
);

CREATE TABLE telemetry_ingest_batch (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    middleware_client_id uuid NOT NULL,
    idempotency_key text NOT NULL,
    payload_hash bytea NOT NULL,
    raw_payload jsonb NOT NULL,
    accepted_count integer NOT NULL DEFAULT 0,
    duplicate_count integer NOT NULL DEFAULT 0,
    rejected_count integer NOT NULL DEFAULT 0,
    onboarded_plant_count integer NOT NULL DEFAULT 0,
    onboarded_device_count integer NOT NULL DEFAULT 0,
    errors jsonb NOT NULL DEFAULT '[]',
    status text NOT NULL DEFAULT 'PROCESSING',
    received_at timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    CONSTRAINT telemetry_batch_client_fk FOREIGN KEY (organization_id, middleware_client_id)
        REFERENCES middleware_client(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT telemetry_batch_idempotency_length CHECK (length(idempotency_key) BETWEEN 1 AND 200),
    CONSTRAINT telemetry_batch_raw_object CHECK (jsonb_typeof(raw_payload) = 'object'),
    CONSTRAINT telemetry_batch_errors_array CHECK (jsonb_typeof(errors) = 'array'),
    CONSTRAINT telemetry_batch_status_valid CHECK (status IN ('PROCESSING', 'PROCESSED')),
    CONSTRAINT telemetry_batch_counts_nonnegative CHECK (
        accepted_count >= 0 AND duplicate_count >= 0 AND rejected_count >= 0
        AND onboarded_plant_count >= 0 AND onboarded_device_count >= 0
    ),
    CONSTRAINT telemetry_batch_client_key_unique UNIQUE (middleware_client_id, idempotency_key),
    CONSTRAINT telemetry_batch_org_id_unique UNIQUE (organization_id, id)
);

CREATE TABLE telemetry_reading (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    plant_id uuid NOT NULL,
    device_id uuid NOT NULL,
    middleware_client_id uuid NOT NULL,
    ingest_batch_id uuid NOT NULL,
    gateway_id text NOT NULL,
    external_key text NOT NULL,
    observed_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    data_item_map jsonb NOT NULL,
    parameter_count integer NOT NULL,
    CONSTRAINT telemetry_reading_plant_fk FOREIGN KEY (organization_id, plant_id)
        REFERENCES plant(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT telemetry_reading_device_fk FOREIGN KEY (organization_id, plant_id, device_id)
        REFERENCES device(organization_id, plant_id, id) ON DELETE RESTRICT,
    CONSTRAINT telemetry_reading_client_fk FOREIGN KEY (organization_id, middleware_client_id)
        REFERENCES middleware_client(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT telemetry_reading_batch_fk FOREIGN KEY (organization_id, ingest_batch_id)
        REFERENCES telemetry_ingest_batch(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT telemetry_reading_gateway_length CHECK (length(btrim(gateway_id)) BETWEEN 1 AND 200),
    CONSTRAINT telemetry_reading_external_key_length CHECK (length(external_key) BETWEEN 1 AND 500),
    CONSTRAINT telemetry_reading_map_object CHECK (jsonb_typeof(data_item_map) = 'object'),
    CONSTRAINT telemetry_reading_parameter_count CHECK (parameter_count >= 0),
    CONSTRAINT telemetry_reading_client_external_unique UNIQUE (middleware_client_id, external_key)
);

CREATE INDEX middleware_client_organization_idx ON middleware_client (organization_id, is_active, name);
CREATE INDEX telemetry_ingest_received_idx ON telemetry_ingest_batch (organization_id, received_at DESC);
CREATE INDEX telemetry_reading_plant_time_idx ON telemetry_reading (organization_id, plant_id, observed_at DESC);
CREATE INDEX telemetry_reading_device_time_idx ON telemetry_reading (organization_id, device_id, observed_at DESC);