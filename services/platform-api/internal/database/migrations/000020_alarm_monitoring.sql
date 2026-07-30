INSERT INTO permission (id, action, resource_type, description) VALUES
    ('00000000-0000-4000-8000-000000000220', 'read',   'alarm', 'Read alarm rules and events'),
    ('00000000-0000-4000-8000-000000000221', 'create', 'alarm', 'Create alarm rules'),
    ('00000000-0000-4000-8000-000000000222', 'update', 'alarm', 'Update alarm rules'),
    ('00000000-0000-4000-8000-000000000223', 'delete', 'alarm', 'Delete alarm rules'),
    ('00000000-0000-4000-8000-000000000224', 'ack',    'alarm', 'Acknowledge alarm events');

INSERT INTO role_permission (organization_id, role_id, permission_id)
SELECT NULL, r.id, p.id
FROM role r
JOIN permission p ON p.resource_type = 'alarm'
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
      p.action = 'read'
      OR p.action = 'ack' AND r.id IN (
          '00000000-0000-4000-8000-000000000201'::uuid,
          '00000000-0000-4000-8000-000000000202'::uuid,
          '00000000-0000-4000-8000-000000000203'::uuid,
          '00000000-0000-4000-8000-000000000204'::uuid,
          '00000000-0000-4000-8000-000000000205'::uuid
      )
      OR p.action IN ('create', 'update', 'delete') AND r.id IN (
          '00000000-0000-4000-8000-000000000201'::uuid,
          '00000000-0000-4000-8000-000000000202'::uuid,
          '00000000-0000-4000-8000-000000000203'::uuid
      )
  );

CREATE TABLE alarm_rule (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    plant_id uuid NOT NULL,
    device_id uuid NOT NULL,
    point_key text NOT NULL,
    label text NOT NULL,
    min_value double precision,
    max_value double precision,
    severity text NOT NULL DEFAULT 'warning',
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT alarm_rule_plant_fk FOREIGN KEY (organization_id, plant_id)
        REFERENCES plant(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT alarm_rule_device_fk FOREIGN KEY (organization_id, plant_id, device_id)
        REFERENCES device(organization_id, plant_id, id) ON DELETE CASCADE,
    CONSTRAINT alarm_rule_org_id_unique UNIQUE (organization_id, id),
    CONSTRAINT alarm_rule_label_length CHECK (length(btrim(label)) BETWEEN 1 AND 200),
    CONSTRAINT alarm_rule_point_key_length CHECK (length(btrim(point_key)) BETWEEN 1 AND 200),
    CONSTRAINT alarm_rule_severity_valid CHECK (severity IN ('warning', 'major', 'critical')),
    CONSTRAINT alarm_rule_threshold_present CHECK (min_value IS NOT NULL OR max_value IS NOT NULL)
);

CREATE INDEX alarm_rule_device_point_idx
ON alarm_rule (organization_id, device_id, point_key)
WHERE is_active;

CREATE TABLE alarm_event (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id uuid NOT NULL,
    plant_id uuid NOT NULL,
    device_id uuid NOT NULL,
    alarm_rule_id uuid NOT NULL,
    point_key text NOT NULL,
    severity text NOT NULL,
    value double precision NOT NULL,
    threshold_min double precision,
    threshold_max double precision,
    breached_at timestamptz NOT NULL DEFAULT now(),
    cleared_at timestamptz,
    acknowledged_by uuid REFERENCES app_user(id) ON DELETE SET NULL,
    acknowledged_at timestamptz,
    acknowledged_note text,
    CONSTRAINT alarm_event_plant_fk FOREIGN KEY (organization_id, plant_id)
        REFERENCES plant(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT alarm_event_rule_fk FOREIGN KEY (organization_id, alarm_rule_id)
        REFERENCES alarm_rule(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT alarm_event_ack_consistent CHECK (
        (acknowledged_by IS NULL AND acknowledged_at IS NULL) OR (acknowledged_by IS NOT NULL AND acknowledged_at IS NOT NULL)
    )
);

CREATE INDEX alarm_event_plant_time_idx
ON alarm_event (organization_id, plant_id, breached_at DESC, id DESC);

CREATE UNIQUE INDEX alarm_event_open_rule_unique
ON alarm_event (alarm_rule_id)
WHERE cleared_at IS NULL;
