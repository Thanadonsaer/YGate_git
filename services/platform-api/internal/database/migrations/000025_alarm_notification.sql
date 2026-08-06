ALTER TABLE alarm_rule
    ADD COLUMN notify_role_id uuid REFERENCES role(id) ON DELETE SET NULL;

-- NULL means no notification is sent for the rule (unchanged default behavior).
