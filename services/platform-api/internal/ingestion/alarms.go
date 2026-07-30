package ingestion

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// evaluateAlarms runs inside the same transaction as the telemetry insert
// (see ingestReading in service.go), immediately after the reading becomes
// the device's latest value. Every active rule for the device is checked
// against the point it watches: a breach opens an alarm_event (a no-op if
// one is already open, enforced by the alarm_event_open_rule_unique partial
// index so repeated breaches never duplicate), and a value back in range
// clears any open event for that rule.
func evaluateAlarms(ctx context.Context, tx pgx.Tx, organizationID, plantID, deviceID pgtype.UUID, dataItemMap map[string]float64, observedAt time.Time) error {
	rows, err := tx.Query(ctx, `
SELECT id, point_key, min_value, max_value, severity
FROM alarm_rule
WHERE organization_id=$1 AND device_id=$2 AND is_active`, organizationID, deviceID)
	if err != nil {
		return fmt.Errorf("list active alarm rules: %w", err)
	}
	type rule struct {
		id                 pgtype.UUID
		pointKey, severity string
		minValue, maxValue pgtype.Float8
	}
	var rules []rule
	for rows.Next() {
		var r rule
		if err = rows.Scan(&r.id, &r.pointKey, &r.minValue, &r.maxValue, &r.severity); err != nil {
			rows.Close()
			return fmt.Errorf("scan alarm rule: %w", err)
		}
		rules = append(rules, r)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return fmt.Errorf("list active alarm rules: %w", err)
	}

	for _, r := range rules {
		value, present := dataItemMap[r.pointKey]
		if !present {
			continue
		}
		breach := (r.minValue.Valid && value < r.minValue.Float64) || (r.maxValue.Valid && value > r.maxValue.Float64)
		if breach {
			if _, err = tx.Exec(ctx, `
INSERT INTO alarm_event (organization_id, plant_id, device_id, alarm_rule_id, point_key, severity, value, threshold_min, threshold_max, breached_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (alarm_rule_id) WHERE cleared_at IS NULL DO NOTHING`,
				organizationID, plantID, deviceID, r.id, r.pointKey, r.severity, value, r.minValue, r.maxValue, observedAt); err != nil {
				return fmt.Errorf("open alarm event: %w", err)
			}
			continue
		}
		if _, err = tx.Exec(ctx, `UPDATE alarm_event SET cleared_at=now() WHERE alarm_rule_id=$1 AND cleared_at IS NULL`, r.id); err != nil {
			return fmt.Errorf("clear alarm event: %w", err)
		}
	}
	return nil
}
