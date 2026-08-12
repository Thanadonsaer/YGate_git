-- Alarm Delay: a per-rule cooldown between consecutive alarms.
--
-- alarm_event_open_rule_unique already stops a *sustained* breach from opening
-- a second event -- while one is open, repeated breaches do nothing. What it
-- does not stop is flapping: a value oscillating either side of its threshold
-- clears the event and immediately opens a new one on the next reading, and
-- every one of those emails its notify role. A sensor sitting right on its
-- limit can produce a hundred alarms and a hundred emails in an afternoon.
--
-- With a delay set, a rule that has alarmed cannot alarm again until that long
-- has passed since the last alarm's breached_at -- no new alarm_event row and
-- no notification. 0 keeps today's behaviour exactly, so existing rules are
-- unaffected until an operator sets one.
--
-- This deliberately measures from the previous *alarm*, not from when it
-- cleared, which is how the requirement is worded: "หลังจากเกิด Alarm ล่าสุด
-- ถ้าเกิดขึ้นอีกภายในช่วงเวลา Delay ที่ตั้งไว้จะยังไม่เตือนซ้ำ".

ALTER TABLE alarm.alarm_rule
    ADD COLUMN alarm_delay_seconds integer NOT NULL DEFAULT 0,
    ADD CONSTRAINT alarm_rule_alarm_delay_range
        CHECK (alarm_delay_seconds BETWEEN 0 AND 86400);

-- Evaluation now asks "when did this rule last alarm?" for every active rule on
-- every reading. Without this that is a scan of the rule's whole event history;
-- with it, one index descent -- DESC matches the ordered limit the planner
-- rewrites max() into.
CREATE INDEX alarm_event_rule_recent_idx
    ON alarm.alarm_event (alarm_rule_id, breached_at DESC);
