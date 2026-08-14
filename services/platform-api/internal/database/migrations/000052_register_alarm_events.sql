-- Decoded Register Profile alarms use the existing Alarm Log and realtime
-- channel. Their source snapshot is immutable even if a Profile changes.
ALTER TABLE alarm.alarm_event
    ALTER COLUMN alarm_rule_id DROP NOT NULL,
    ADD COLUMN source_type text NOT NULL DEFAULT 'RULE',
    ADD COLUMN register_mapping_source_id uuid,
    ADD COLUMN register_snapshot jsonb;

ALTER TABLE alarm.alarm_event
    ADD CONSTRAINT alarm_event_source_valid CHECK (source_type IN ('RULE', 'REGISTER')),
    ADD CONSTRAINT alarm_event_source_fields CHECK (
        (source_type = 'RULE' AND alarm_rule_id IS NOT NULL AND register_mapping_source_id IS NULL AND register_snapshot IS NULL)
        OR (source_type = 'REGISTER' AND alarm_rule_id IS NULL AND register_mapping_source_id IS NOT NULL AND register_snapshot IS NOT NULL)
    );

DROP INDEX alarm.alarm_event_open_rule_unique;

CREATE UNIQUE INDEX alarm_event_open_rule_unique
ON alarm.alarm_event (alarm_rule_id)
WHERE cleared_at IS NULL AND source_type = 'RULE';

CREATE UNIQUE INDEX alarm_event_open_register_unique
ON alarm.alarm_event (device_id, register_mapping_source_id)
WHERE cleared_at IS NULL AND source_type = 'REGISTER';

ALTER TABLE plant.plant
    ADD COLUMN alarm_email_enabled boolean NOT NULL DEFAULT false,
    ADD COLUMN alarm_notify_role_id uuid REFERENCES auth.role(id) ON DELETE SET NULL;
