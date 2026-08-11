CREATE TABLE alarm.event_logbook (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    plant_id uuid NOT NULL,
    device_id uuid,
    event_type text NOT NULL,
    category text NOT NULL DEFAULT '',
    title text NOT NULL,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz,
    note text NOT NULL DEFAULT '',
    source text NOT NULL DEFAULT 'MANUAL',
    created_by uuid NOT NULL REFERENCES auth.app_user(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT event_logbook_plant_fk FOREIGN KEY (organization_id, plant_id)
        REFERENCES plant.plant(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT event_logbook_device_fk FOREIGN KEY (organization_id, plant_id, device_id)
        REFERENCES plant.device(organization_id, plant_id, id) ON DELETE RESTRICT,
    CONSTRAINT event_logbook_type_valid CHECK (event_type IN ('FAULT', 'MAINTENANCE', 'CURTAILMENT', 'NOTE')),
    CONSTRAINT event_logbook_source_valid CHECK (source IN ('MANUAL', 'SYSTEM')),
    CONSTRAINT event_logbook_title_length CHECK (length(btrim(title)) BETWEEN 1 AND 200),
    CONSTRAINT event_logbook_category_length CHECK (length(category) <= 100),
    CONSTRAINT event_logbook_note_length CHECK (length(note) <= 4000),
    CONSTRAINT event_logbook_time_order CHECK (ends_at IS NULL OR ends_at >= starts_at)
);

CREATE INDEX event_logbook_plant_time_idx
ON alarm.event_logbook (organization_id, plant_id, starts_at DESC, id DESC);
