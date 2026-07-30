CREATE TABLE raw_register_reading (
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
    register_address_map jsonb NOT NULL,
    parameter_count integer NOT NULL,
    CONSTRAINT raw_register_reading_plant_fk FOREIGN KEY (organization_id, plant_id)
        REFERENCES plant(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT raw_register_reading_device_fk FOREIGN KEY (organization_id, plant_id, device_id)
        REFERENCES device(organization_id, plant_id, id) ON DELETE RESTRICT,
    CONSTRAINT raw_register_reading_client_fk FOREIGN KEY (organization_id, middleware_client_id)
        REFERENCES middleware_client(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT raw_register_reading_batch_fk FOREIGN KEY (organization_id, ingest_batch_id)
        REFERENCES telemetry_ingest_batch(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT raw_register_reading_gateway_length CHECK (length(btrim(gateway_id)) BETWEEN 1 AND 200),
    CONSTRAINT raw_register_reading_external_key_length CHECK (length(external_key) BETWEEN 1 AND 500),
    CONSTRAINT raw_register_reading_map_object CHECK (jsonb_typeof(register_address_map) = 'object'),
    CONSTRAINT raw_register_reading_parameter_count CHECK (parameter_count BETWEEN 1 AND 5000),
    CONSTRAINT raw_register_reading_client_external_unique UNIQUE (middleware_client_id, external_key)
);

CREATE INDEX raw_register_reading_plant_time_idx
    ON raw_register_reading (organization_id, plant_id, observed_at DESC);
CREATE INDEX raw_register_reading_device_time_idx
    ON raw_register_reading (organization_id, device_id, observed_at DESC);
