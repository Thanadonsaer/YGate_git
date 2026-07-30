CREATE TABLE telemetry_latest (
    organization_id uuid NOT NULL,
    plant_id uuid NOT NULL,
    device_id uuid NOT NULL,
    telemetry_reading_id uuid NOT NULL REFERENCES telemetry_reading(id) ON DELETE RESTRICT,
    gateway_id text NOT NULL,
    observed_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL,
    data_item_map jsonb NOT NULL,
    parameter_count integer NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, device_id),
    CONSTRAINT telemetry_latest_device_fk FOREIGN KEY (organization_id, plant_id, device_id)
        REFERENCES device(organization_id, plant_id, id) ON DELETE RESTRICT,
    CONSTRAINT telemetry_latest_gateway_length CHECK (length(btrim(gateway_id)) BETWEEN 1 AND 200),
    CONSTRAINT telemetry_latest_map_object CHECK (jsonb_typeof(data_item_map) = 'object'),
    CONSTRAINT telemetry_latest_parameter_count CHECK (parameter_count >= 0)
);

INSERT INTO telemetry_latest (
    organization_id, plant_id, device_id, telemetry_reading_id, gateway_id,
    observed_at, received_at, data_item_map, parameter_count
)
SELECT DISTINCT ON (organization_id, device_id)
       organization_id, plant_id, device_id, id, gateway_id,
       observed_at, received_at, data_item_map, parameter_count
FROM telemetry_reading
ORDER BY organization_id, device_id, observed_at DESC, received_at DESC, id DESC;

CREATE INDEX telemetry_latest_plant_time_idx
ON telemetry_latest (organization_id, plant_id, observed_at DESC);
