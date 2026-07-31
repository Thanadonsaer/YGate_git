-- Protocol-level settings shared by every register on a model (matches
-- modbus-api-middleware's device_sets table: address_mode/byte_order/
-- word_order/max_block_size). Defaults match every device model in the
-- real Huawei/ABB catalog sampled from docs/middleware_db/middleware.db
-- (VENDOR_RAW/BIG_ENDIAN/HIGH_LOW/30 on all 11 rows) -- not yet exposed in
-- the admin UI, so a device model needing a different combination requires
-- a direct update for now.
ALTER TABLE device_model
    ADD COLUMN modbus_address_mode text NOT NULL DEFAULT 'VENDOR_RAW',
    ADD COLUMN modbus_byte_order text NOT NULL DEFAULT 'BIG_ENDIAN',
    ADD COLUMN modbus_word_order text NOT NULL DEFAULT 'HIGH_LOW',
    ADD COLUMN modbus_max_block_size integer NOT NULL DEFAULT 30,
    ADD CONSTRAINT device_model_modbus_byte_order_valid CHECK (modbus_byte_order IN ('BIG_ENDIAN', 'LITTLE_ENDIAN')),
    ADD CONSTRAINT device_model_modbus_word_order_valid CHECK (modbus_word_order IN ('HIGH_LOW', 'LOW_HIGH')),
    ADD CONSTRAINT device_model_modbus_max_block_size_range CHECK (modbus_max_block_size BETWEEN 1 AND 125);

ALTER TABLE device_model_register_metadata
    ADD COLUMN modbus_function_code integer,
    ADD COLUMN modbus_register integer,
    ADD COLUMN modbus_word_order text,
    ADD COLUMN modbus_data_type text,
    ADD CONSTRAINT device_model_register_metadata_modbus_fc_valid
        CHECK (modbus_function_code IS NULL OR modbus_function_code IN (3, 4)),
    ADD CONSTRAINT device_model_register_metadata_modbus_register_range
        CHECK (modbus_register IS NULL OR modbus_register BETWEEN 0 AND 65535),
    ADD CONSTRAINT device_model_register_metadata_modbus_word_order_valid
        CHECK (modbus_word_order IS NULL OR modbus_word_order IN ('HIGH_LOW', 'LOW_HIGH')),
    ADD CONSTRAINT device_model_register_metadata_modbus_data_type_valid
        CHECK (modbus_data_type IS NULL OR modbus_data_type IN ('U16', 'I16', 'U32', 'I32', 'U64', 'FLOAT32')),
    ADD CONSTRAINT device_model_register_metadata_modbus_all_or_none
        CHECK (
            (modbus_function_code IS NULL AND modbus_register IS NULL AND modbus_data_type IS NULL)
            OR (modbus_function_code IS NOT NULL AND modbus_register IS NOT NULL AND modbus_data_type IS NOT NULL)
        );

ALTER TABLE device
    ADD COLUMN modbus_host text,
    ADD COLUMN modbus_port integer,
    ADD COLUMN modbus_unit_id integer NOT NULL DEFAULT 1,
    ADD COLUMN poll_interval_seconds integer NOT NULL DEFAULT 10,
    ADD CONSTRAINT device_modbus_port_range
        CHECK (modbus_port IS NULL OR modbus_port BETWEEN 1 AND 65535),
    ADD CONSTRAINT device_modbus_unit_id_range
        CHECK (modbus_unit_id BETWEEN 0 AND 255),
    ADD CONSTRAINT device_poll_interval_range
        CHECK (poll_interval_seconds BETWEEN 1 AND 3600),
    ADD CONSTRAINT device_modbus_host_port_together
        CHECK ((modbus_host IS NULL) = (modbus_port IS NULL));

CREATE TABLE middleware_plant (
    middleware_client_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    plant_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (middleware_client_id, plant_id),
    CONSTRAINT middleware_plant_client_fk FOREIGN KEY (organization_id, middleware_client_id)
        REFERENCES middleware_client(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT middleware_plant_plant_fk FOREIGN KEY (organization_id, plant_id)
        REFERENCES plant(organization_id, id) ON DELETE CASCADE,
    -- Enforces "one Middleware per Plant": a plant_id can only appear once
    -- across all middleware_plant rows, regardless of which middleware.
    CONSTRAINT middleware_plant_one_middleware_per_plant UNIQUE (plant_id)
);

CREATE INDEX middleware_plant_client_idx ON middleware_plant (organization_id, middleware_client_id);

INSERT INTO permission (id, action, resource_type, description) VALUES
    ('00000000-0000-4000-8000-000000000302', 'read',   'middleware_plant', 'Read which Plants a middleware gateway serves'),
    ('00000000-0000-4000-8000-000000000303', 'update', 'middleware_plant', 'Assign or unassign Plants to a middleware gateway');

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, r.id, p.id
FROM role r
JOIN permission p ON p.resource_type = 'middleware_plant'
WHERE r.id IN (
    '00000000-0000-4000-8000-000000000201'::uuid,
    '00000000-0000-4000-8000-000000000202'::uuid
);
