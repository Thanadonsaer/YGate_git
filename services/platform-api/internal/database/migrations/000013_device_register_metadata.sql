CREATE TABLE device_register_metadata (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    plant_id uuid NOT NULL,
    device_id uuid NOT NULL,
    address_key text NOT NULL,
    display_name text NOT NULL,
    unit text NOT NULL DEFAULT '',
    data_type text NOT NULL DEFAULT 'number',
    scale double precision NOT NULL DEFAULT 1,
    value_offset double precision NOT NULL DEFAULT 0,
    decimals integer NOT NULL DEFAULT 2,
    is_enabled boolean NOT NULL DEFAULT true,
    notes text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT device_register_metadata_device_fk FOREIGN KEY (organization_id, plant_id, device_id)
        REFERENCES device(organization_id, plant_id, id) ON DELETE CASCADE,
    CONSTRAINT device_register_metadata_address_unique UNIQUE (organization_id, plant_id, device_id, address_key),
    CONSTRAINT device_register_metadata_address_length CHECK (length(btrim(address_key)) BETWEEN 1 AND 200),
    CONSTRAINT device_register_metadata_name_length CHECK (length(btrim(display_name)) BETWEEN 1 AND 200),
    CONSTRAINT device_register_metadata_unit_length CHECK (length(unit) <= 40),
    CONSTRAINT device_register_metadata_data_type CHECK (data_type IN ('number', 'boolean', 'text', 'enum')),
    CONSTRAINT device_register_metadata_decimals CHECK (decimals BETWEEN 0 AND 9),
    CONSTRAINT device_register_metadata_notes_length CHECK (length(notes) <= 500)
);

CREATE INDEX device_register_metadata_device_idx
ON device_register_metadata (organization_id, plant_id, device_id, address_key);
