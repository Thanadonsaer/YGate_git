-- AND/OR moves from the rule down onto each condition.
--
-- 000026 gave a rule one condition_logic for all of its conditions, so a rule
-- could say "A and B and C" or "A or B or C" but never "A and B, or C" --
-- which is what an operator actually wants when one alarm covers a fault
-- signal plus a threshold pair. Each condition now carries the connector that
-- joins it to the condition before it; the first condition's is ignored.
--
-- Precedence is ordinary boolean algebra (AND binds tighter than OR), so the
-- stored list reads exactly like the expression it spells out and a rule that
-- used to be all-AND or all-OR keeps its meaning under the backfill below.

ALTER TABLE alarm.alarm_rule_condition
    ADD COLUMN logic text NOT NULL DEFAULT 'AND',
    ADD CONSTRAINT alarm_rule_condition_logic_valid CHECK (logic IN ('AND', 'OR'));

-- Backfill: an existing rule's single logic applies to every one of its
-- conditions, which for a uniform list is the same expression either way.
UPDATE alarm.alarm_rule_condition c
SET logic = r.condition_logic
FROM alarm.alarm_rule r
WHERE r.id = c.alarm_rule_id;

-- alarm_rule.condition_logic is deliberately NOT dropped. Migrations run at
-- service startup, so during a rolling deploy or a rollback an older binary
-- that still SELECTs and INSERTs the column can be talking to this schema at
-- the same time, and every alarm rule read/write would fail against a dropped
-- column. It keeps its NOT NULL DEFAULT 'AND' and simply stops being read.
--
-- ponytail: drop it for real in a later release, once no binary that reads it
-- can still be running -- same expand/contract half-step as 000039's
-- raw_payload.
