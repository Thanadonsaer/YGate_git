-- Reusable register definitions. The initial migration gives each existing
-- Device Model its own Profile, preserving all current metadata behavior.
CREATE TABLE plant.register_profile (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    name text NOT NULL,
    manufacturer text NOT NULL DEFAULT '',
    description text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT register_profile_org_id_unique UNIQUE (organization_id, id),
    CONSTRAINT register_profile_name_length CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    CONSTRAINT register_profile_description_length CHECK (length(description) <= 500),
    CONSTRAINT register_profile_name_unique UNIQUE (organization_id, name)
);

CREATE TABLE plant.register_profile_address (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    profile_id uuid NOT NULL,
    address_key text NOT NULL,
    display_name text NOT NULL,
    unit text NOT NULL DEFAULT '',
    data_type text NOT NULL DEFAULT 'number',
    scale double precision NOT NULL DEFAULT 1,
    value_offset double precision NOT NULL DEFAULT 0,
    decimals integer NOT NULL DEFAULT 2,
    is_enabled boolean NOT NULL DEFAULT true,
    notes text NOT NULL DEFAULT '',
    modbus_function_code integer,
    modbus_register integer,
    modbus_word_order text,
    modbus_data_type text,
    is_alarm boolean NOT NULL DEFAULT false,
    mapping_mode text NOT NULL DEFAULT 'EXACT',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT register_profile_address_profile_fk FOREIGN KEY (organization_id, profile_id)
        REFERENCES plant.register_profile(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT register_profile_address_org_id_unique UNIQUE (organization_id, id),
    CONSTRAINT register_profile_address_unique UNIQUE (organization_id, profile_id, address_key),
    CONSTRAINT register_profile_address_data_type CHECK (data_type IN ('number', 'boolean', 'text', 'enum')),
    CONSTRAINT register_profile_address_decimals CHECK (decimals BETWEEN 0 AND 9),
    CONSTRAINT register_profile_address_mapping_mode CHECK (mapping_mode IN ('EXACT', 'BITMASK')),
    CONSTRAINT register_profile_address_alarm_mapping CHECK (is_alarm OR mapping_mode = 'EXACT'),
    CONSTRAINT register_profile_address_name_length CHECK (length(btrim(display_name)) BETWEEN 1 AND 200),
    CONSTRAINT register_profile_address_key_length CHECK (length(btrim(address_key)) BETWEEN 1 AND 200)
);

CREATE TABLE plant.register_value_mapping (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    profile_address_id uuid NOT NULL,
    match_value bigint,
    bit_index integer,
    display_value text NOT NULL,
    alarm_state text,
    severity text,
    sort_order integer NOT NULL DEFAULT 0,
    CONSTRAINT register_value_mapping_address_fk FOREIGN KEY (organization_id, profile_address_id)
        REFERENCES plant.register_profile_address(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT register_value_mapping_one_match CHECK ((match_value IS NOT NULL) <> (bit_index IS NOT NULL)),
    CONSTRAINT register_value_mapping_bit_range CHECK (bit_index IS NULL OR bit_index BETWEEN 0 AND 63),
    CONSTRAINT register_value_mapping_display_length CHECK (length(btrim(display_value)) BETWEEN 1 AND 200),
    CONSTRAINT register_value_mapping_alarm_fields CHECK (alarm_state IS NULL OR severity IS NOT NULL)
);

CREATE UNIQUE INDEX register_value_mapping_exact_unique
ON plant.register_value_mapping (organization_id, profile_address_id, match_value)
WHERE match_value IS NOT NULL;

CREATE UNIQUE INDEX register_value_mapping_bit_unique
ON plant.register_value_mapping (organization_id, profile_address_id, bit_index)
WHERE bit_index IS NOT NULL;

ALTER TABLE plant.device_model
    ADD COLUMN register_profile_id uuid;

ALTER TABLE plant.device_model
    ADD CONSTRAINT device_model_register_profile_fk FOREIGN KEY (organization_id, register_profile_id)
        REFERENCES plant.register_profile(organization_id, id) ON DELETE RESTRICT;

INSERT INTO plant.register_profile (id, organization_id, name, manufacturer)
SELECT id, organization_id, left(manufacturer || ' ' || model, 200), manufacturer
FROM plant.device_model;

INSERT INTO plant.register_profile_address (
    id, organization_id, profile_id, address_key, display_name, unit, data_type,
    scale, value_offset, decimals, is_enabled, notes, modbus_function_code,
    modbus_register, modbus_word_order, modbus_data_type
)
SELECT id, organization_id, device_model_id, address_key, display_name, unit, data_type,
       scale, value_offset, decimals, is_enabled, notes, modbus_function_code,
       modbus_register, modbus_word_order, modbus_data_type
FROM plant.device_model_register_metadata;

UPDATE plant.device_model
SET register_profile_id = id;

ALTER TABLE plant.device_model
    ALTER COLUMN register_profile_id SET NOT NULL;

CREATE INDEX register_profile_address_lookup_idx
ON plant.register_profile_address (organization_id, profile_id, address_key);
