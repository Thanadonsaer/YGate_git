package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/core"
	"ygate/platform-api/internal/notify"
)

// alarmConditionSnapshot is one condition's value at the moment its rule was
// evaluated, recorded onto the alarm_event row so the event log can show why
// a multi-condition rule fired without depending on the rule's current
// (possibly since-edited) conditions.
type alarmConditionSnapshot struct {
	PointKey string   `json:"pointKey"`
	Value    float64  `json:"value"`
	MinValue *float64 `json:"minValue"`
	MaxValue *float64 `json:"maxValue"`
	Breached bool     `json:"breached"`
	Logic    string   `json:"logic"`
}

// alarmBreach describes a newly-opened alarm_event (not a repeat of one
// already open), gathered so the caller can email its notify role once the
// ingestion transaction has actually committed.
type alarmBreach struct {
	ruleLabel, severity              string
	conditions                       []alarmConditionSnapshot
	notifyRoleID, plantID            pgtype.UUID
	plantCode, plantName, deviceName string
}

// evaluateAlarms runs inside the same transaction as the telemetry insert
// (see ingestReading in service.go), immediately after the reading becomes
// the device's latest value. Every active rule for the device has its
// conditions (one or more point/threshold checks, each carrying the AND/OR
// that joins it to the previous one) checked against this reading: a breach
// opens an alarm_event (a no-op if
// one is already open, enforced by the alarm_event_open_rule_unique partial
// index so repeated breaches never duplicate), and no-longer-breaching
// clears any open event for that rule. Returns only the breaches that newly
// opened an event, i.e. the ones worth notifying about.
func evaluateAlarms(ctx context.Context, tx pgx.Tx, organizationID, plantID, deviceID pgtype.UUID, plantCode, plantName, deviceName string, dataItemMap map[string]float64, observedAt time.Time) ([]alarmBreach, error) {
	// lastBreachedAt rides along on this query rather than costing a second
	// round trip per rule -- alarm_event_rule_recent_idx makes it one index
	// descent. It feeds the Alarm Delay check below.
	rows, err := tx.Query(ctx, `
SELECT r.id, r.label, r.severity, r.notify_role_id, r.alarm_delay_seconds,
       (SELECT max(e.breached_at) FROM alarm.alarm_event e WHERE e.alarm_rule_id = r.id)
FROM alarm.alarm_rule r
WHERE r.organization_id=$1 AND r.device_id=$2 AND r.is_active`, organizationID, deviceID)
	if err != nil {
		return nil, fmt.Errorf("list active alarm rules: %w", err)
	}
	type rule struct {
		id              pgtype.UUID
		label, severity string
		notifyRoleID    pgtype.UUID
		delaySeconds    int32
		lastBreachedAt  pgtype.Timestamptz
	}
	var rules []rule
	for rows.Next() {
		var r rule
		if err = rows.Scan(&r.id, &r.label, &r.severity, &r.notifyRoleID, &r.delaySeconds, &r.lastBreachedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan alarm rule: %w", err)
		}
		rules = append(rules, r)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("list active alarm rules: %w", err)
	}

	var breaches []alarmBreach
	for _, r := range rules {
		conditionRows, err := tx.Query(ctx, `SELECT point_key, min_value, max_value, logic FROM alarm.alarm_rule_condition WHERE alarm_rule_id=$1 ORDER BY position`, r.id)
		if err != nil {
			return nil, fmt.Errorf("list alarm rule conditions: %w", err)
		}
		type condition struct {
			pointKey           string
			minValue, maxValue pgtype.Float8
			logic              string
		}
		var conditions []condition
		for conditionRows.Next() {
			var c condition
			if err = conditionRows.Scan(&c.pointKey, &c.minValue, &c.maxValue, &c.logic); err != nil {
				conditionRows.Close()
				return nil, fmt.Errorf("scan alarm rule condition: %w", err)
			}
			conditions = append(conditions, c)
		}
		conditionRows.Close()
		if err = conditionRows.Err(); err != nil {
			return nil, fmt.Errorf("list alarm rule conditions: %w", err)
		}

		// ponytail: a rule only gets evaluated on a reading that carries every
		// one of its condition points; a reading missing some of them is
		// skipped rather than partially evaluated. Upgrade to per-point cached
		// last-known-values if points start arriving across separate readings.
		snapshots := make([]alarmConditionSnapshot, 0, len(conditions))
		breached := make([]bool, 0, len(conditions))
		logic := make([]string, 0, len(conditions))
		allPresent := true
		for _, c := range conditions {
			value, present := dataItemMap[c.pointKey]
			if !present {
				allPresent = false
				break
			}
			isBreached := (c.minValue.Valid && value < c.minValue.Float64) || (c.maxValue.Valid && value > c.maxValue.Float64)
			breached = append(breached, isBreached)
			logic = append(logic, c.logic)
			snapshots = append(snapshots, alarmConditionSnapshot{PointKey: c.pointKey, Value: value, MinValue: floatPointerOf(c.minValue), MaxValue: floatPointerOf(c.maxValue), Breached: isBreached, Logic: c.logic})
		}
		if !allPresent {
			continue
		}
		// Each condition carries the AND/OR joining it to the one before it, so
		// the verdict is a sum of products rather than one all-or-any rule --
		// core owns that fold so evaluation and the editor cannot disagree.
		ruleBreach := core.EvaluateConditions(breached, logic)

		// Alarm Delay: this rule alarmed recently enough that a fresh one is
		// held back entirely -- no event, no email. Checked before the insert
		// rather than folded into it so the rule is one tested Go function
		// (core.AlarmSuppressed) instead of a subtle interval predicate in SQL.
		//
		// `continue`, not a fall-through to the clear below: the rule IS
		// breaching, so clearing whatever is open would be wrong.
		if ruleBreach && core.AlarmSuppressed(r.lastBreachedAt.Time, observedAt, r.delaySeconds) {
			continue
		}

		if ruleBreach {
			snapshotJSON, marshalErr := json.Marshal(snapshots)
			if marshalErr != nil {
				return nil, fmt.Errorf("encode alarm event condition snapshot: %w", marshalErr)
			}
			var openedID int64
			err = tx.QueryRow(ctx, `
INSERT INTO alarm.alarm_event (organization_id, plant_id, device_id, alarm_rule_id, severity, condition_snapshot, breached_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (alarm_rule_id) WHERE cleared_at IS NULL DO NOTHING
RETURNING id`,
				organizationID, plantID, deviceID, r.id, r.severity, snapshotJSON, observedAt).Scan(&openedID)
			if err != nil && err != pgx.ErrNoRows {
				return nil, fmt.Errorf("open alarm event: %w", err)
			}
			if err == nil && r.notifyRoleID.Valid {
				breaches = append(breaches, alarmBreach{
					ruleLabel: r.label, severity: r.severity, conditions: snapshots, notifyRoleID: r.notifyRoleID, plantID: plantID,
					plantCode: plantCode, plantName: plantName, deviceName: deviceName,
				})
			}
			continue
		}
		if _, err = tx.Exec(ctx, `UPDATE alarm.alarm_event SET cleared_at=now() WHERE alarm_rule_id=$1 AND cleared_at IS NULL`, r.id); err != nil {
			return nil, fmt.Errorf("clear alarm event: %w", err)
		}
	}
	return breaches, nil
}

func floatPointerOf(value pgtype.Float8) *float64 {
	if !value.Valid {
		return nil
	}
	copied := value.Float64
	return &copied
}

// notifyAlarmBreaches emails each breach's notify role. Runs detached from the
// ingestion request (see IngestBatch) on its own timeout, so a slow or failing
// SMTP relay can never affect ingestion latency or its HTTP response.
func (s *Service) notifyAlarmBreaches(organizationID pgtype.UUID, breaches []alarmBreach) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, b := range breaches {
		recipients, err := s.alarmNotifyRecipients(ctx, organizationID, b.plantID, b.notifyRoleID)
		if err != nil {
			log.Printf("alarm notify: resolve recipients: %v", err)
			continue
		}
		if len(recipients) == 0 {
			continue
		}
		subject := fmt.Sprintf("[%s] %s - %s", strings.ToUpper(b.severity), b.ruleLabel, b.plantName)
		rows := []notify.EmailRow{
			{Key: "Plant", Value: fmt.Sprintf("%s (%s)", b.plantName, b.plantCode)},
			{Key: "Device", Value: b.deviceName},
			{Key: "Severity", Value: strings.ToUpper(b.severity)},
		}
		for _, c := range b.conditions {
			label := c.PointKey
			if c.Breached {
				label += " ⚠"
			}
			rows = append(rows, notify.EmailRow{Key: label, Value: fmt.Sprintf("%v (threshold %s)", c.Value, thresholdText(c.MinValue, c.MaxValue))})
		}
		html, renderErr := notify.RenderEmail(notify.EmailContent{
			Title: fmt.Sprintf("Alarm rule %q breached", b.ruleLabel),
			Rows:  rows,
		})
		if renderErr != nil {
			log.Printf("alarm notify: render email: %v", renderErr)
			continue
		}
		if err = s.mailer.Send(ctx, recipients, subject, html); err != nil {
			log.Printf("alarm notify: send email to %v: %v", recipients, err)
		}
	}
}

// alarmNotifyRecipients resolves a notify role to the active users who should
// be emailed: users holding that role either unscoped or scoped to this plant.
func (s *Service) alarmNotifyRecipients(ctx context.Context, organizationID, plantID, roleID pgtype.UUID) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
SELECT DISTINCT u.email
FROM auth.user_role ur
JOIN auth.app_user u ON u.id = ur.user_id
WHERE ur.role_id = $1 AND u.organization_id = $2 AND u.status = 'ACTIVE'
  AND (ur.plant_id IS NULL OR ur.plant_id = $3)`, roleID, organizationID, plantID)
	if err != nil {
		return nil, fmt.Errorf("resolve alarm notify recipients: %w", err)
	}
	defer rows.Close()
	var emails []string
	for rows.Next() {
		var email string
		if err = rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("scan alarm notify recipient: %w", err)
		}
		emails = append(emails, email)
	}
	return emails, rows.Err()
}

func thresholdText(minValue, maxValue *float64) string {
	switch {
	case minValue != nil && maxValue != nil:
		return fmt.Sprintf("between %v and %v", *minValue, *maxValue)
	case minValue != nil:
		return fmt.Sprintf("below %v", *minValue)
	case maxValue != nil:
		return fmt.Sprintf("above %v", *maxValue)
	default:
		return "n/a"
	}
}
