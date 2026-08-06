-- Alarm rules move from a single point/threshold to N conditions combined
-- with AND/OR, so a rule can watch multiple points on its device at once
-- (e.g. "SOC < 20 AND Charging = 0").
ALTER TABLE alarm_rule
    ADD COLUMN condition_logic text NOT NULL DEFAULT 'AND',
    ADD CONSTRAINT alarm_rule_condition_logic_valid CHECK (condition_logic IN ('AND', 'OR'));

CREATE TABLE alarm_rule_condition (
    id uuid PRIMARY KEY,
    alarm_rule_id uuid NOT NULL REFERENCES alarm_rule(id) ON DELETE CASCADE,
    point_key text NOT NULL,
    min_value double precision,
    max_value double precision,
    position integer NOT NULL DEFAULT 0,
    CONSTRAINT alarm_rule_condition_point_key_length CHECK (length(btrim(point_key)) BETWEEN 1 AND 200),
    CONSTRAINT alarm_rule_condition_threshold_present CHECK (min_value IS NOT NULL OR max_value IS NOT NULL)
);

CREATE INDEX alarm_rule_condition_rule_idx ON alarm_rule_condition (alarm_rule_id, position);

-- Backfill: every existing rule's single point/threshold becomes its first
-- (and, until edited, only) condition. No extension-provided UUID function is
-- used elsewhere in this codebase (ids are always generated in Go), so this
-- one-off backfill mints ids the same portable way.
INSERT INTO alarm_rule_condition (id, alarm_rule_id, point_key, min_value, max_value, position)
SELECT (md5(random()::text || clock_timestamp()::text))::uuid, id, point_key, min_value, max_value, 0
FROM alarm_rule;

DROP INDEX IF EXISTS alarm_rule_device_point_idx;
CREATE INDEX alarm_rule_device_idx ON alarm_rule (organization_id, device_id) WHERE is_active;

ALTER TABLE alarm_rule
    DROP COLUMN point_key,
    DROP COLUMN min_value,
    DROP COLUMN max_value;

-- alarm_event gains a snapshot of every condition's value at breach time;
-- the old single point_key/value columns can't represent a multi-condition
-- breach so they become optional (still populated for single-condition rules).
ALTER TABLE alarm_event
    ALTER COLUMN point_key DROP NOT NULL,
    ALTER COLUMN value DROP NOT NULL,
    ADD COLUMN condition_snapshot jsonb NOT NULL DEFAULT '[]';
