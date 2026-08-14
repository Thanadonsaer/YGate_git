package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type registerAlarmSignal struct {
	MappingSourceID string
	AddressKey      string
	RawValue        float64
	NumericValue    float64
	DisplayValue    string
	Severity        string
}

func registerAlarmTransitions(open, current []registerAlarmSignal) (toClose, toOpen []registerAlarmSignal) {
	openByID := make(map[string]registerAlarmSignal, len(open))
	currentByID := make(map[string]registerAlarmSignal, len(current))
	for _, signal := range open {
		openByID[signal.MappingSourceID] = signal
	}
	for _, signal := range current {
		currentByID[signal.MappingSourceID] = signal
	}
	for id, signal := range openByID {
		if _, present := currentByID[id]; !present {
			toClose = append(toClose, signal)
		}
	}
	for id, signal := range currentByID {
		if _, present := openByID[id]; !present {
			toOpen = append(toOpen, signal)
		}
	}
	return toClose, toOpen
}

type registerAlarmRow struct {
	addressID, mappingID                                 pgtype.UUID
	addressKey, mode, displayValue, alarmState, severity string
	matchValue                                           pgtype.Int8
	bitIndex                                             pgtype.Int4
	scale, offset                                        float64
}

type registerAlarmSnapshot struct {
	MappingID    string  `json:"mappingId"`
	AddressKey   string  `json:"addressKey"`
	RawValue     float64 `json:"rawValue"`
	NumericValue float64 `json:"numericValue"`
	DisplayValue string  `json:"displayValue"`
	AlarmState   string  `json:"alarmState,omitempty"`
	Severity     string  `json:"severity"`
}

func evaluateRegisterAlarms(ctx context.Context, tx pgx.Tx, organizationID, plantID, deviceID, modelID pgtype.UUID, plantCode, plantName, deviceName string, rawAddressMap, dataItemMap []byte, observedAt time.Time) ([]alarmBreach, error) {
	var raw map[string]float64
	if err := json.Unmarshal(rawAddressMap, &raw); err != nil {
		return nil, fmt.Errorf("decode raw register values for alarms: %w", err)
	}
	var numeric map[string]float64
	if err := json.Unmarshal(dataItemMap, &numeric); err != nil {
		return nil, fmt.Errorf("decode mapped register values for alarms: %w", err)
	}
	rows, err := tx.Query(ctx, `
SELECT address.id, address.address_key, address.mapping_mode, mapping.id,
       mapping.match_value, mapping.bit_index, mapping.display_value,
       COALESCE(mapping.alarm_state,''), COALESCE(mapping.severity,''),
       address.scale, address.value_offset
FROM plant.device_model model
JOIN plant.register_profile_address address
  ON address.organization_id=model.organization_id AND address.profile_id=model.register_profile_id
JOIN plant.register_value_mapping mapping
  ON mapping.organization_id=address.organization_id AND mapping.profile_address_id=address.id
WHERE model.organization_id=$1 AND model.id=$2 AND address.is_alarm
ORDER BY address.address_key, mapping.bit_index NULLS FIRST, mapping.id`, organizationID, modelID)
	if err != nil {
		return nil, fmt.Errorf("list register alarm mappings: %w", err)
	}
	defer rows.Close()
	var definitions []registerAlarmRow
	for rows.Next() {
		var row registerAlarmRow
		if err = rows.Scan(&row.addressID, &row.addressKey, &row.mode, &row.mappingID, &row.matchValue, &row.bitIndex, &row.displayValue, &row.alarmState, &row.severity, &row.scale, &row.offset); err != nil {
			return nil, fmt.Errorf("scan register alarm mapping: %w", err)
		}
		definitions = append(definitions, row)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	current := make([]registerAlarmSignal, 0, len(definitions))
	for _, definition := range definitions {
		rawValue, ok := lookupRegisterValue(raw, definition.addressKey)
		if !ok || strings.EqualFold(definition.alarmState, "NORMAL") {
			continue
		}
		active := definition.matchValue.Valid && definition.bitIndex.Valid == false && isIntegerRegisterValue(rawValue) && definition.matchValue.Int64 == int64(rawValue)
		if strings.EqualFold(definition.mode, "BITMASK") && definition.bitIndex.Valid && rawValue >= 0 && isIntegerRegisterValue(rawValue) {
			active = uint64(rawValue)&(uint64(1)<<uint(definition.bitIndex.Int32)) != 0
		}
		if !active {
			continue
		}
		numericValue, _ := lookupRegisterValue(numeric, definition.addressKey)
		current = append(current, registerAlarmSignal{MappingSourceID: uuidString(definition.mappingID), AddressKey: definition.addressKey, RawValue: rawValue, NumericValue: numericValue, DisplayValue: definition.displayValue, Severity: definition.severity})
	}
	openRows, err := tx.Query(ctx, `SELECT register_mapping_source_id FROM alarm.alarm_event WHERE organization_id=$1 AND plant_id=$2 AND device_id=$3 AND source_type='REGISTER' AND cleared_at IS NULL FOR UPDATE`, organizationID, plantID, deviceID)
	if err != nil {
		return nil, fmt.Errorf("list open register alarms: %w", err)
	}
	var open []registerAlarmSignal
	for openRows.Next() {
		var mappingID pgtype.UUID
		if err = openRows.Scan(&mappingID); err != nil {
			openRows.Close()
			return nil, err
		}
		open = append(open, registerAlarmSignal{MappingSourceID: uuidString(mappingID)})
	}
	openRows.Close()
	if err = openRows.Err(); err != nil {
		return nil, err
	}
	toClose, toOpen := registerAlarmTransitions(open, current)
	for _, signal := range toClose {
		if _, err = tx.Exec(ctx, `UPDATE alarm.alarm_event SET cleared_at=$1 WHERE organization_id=$2 AND device_id=$3 AND source_type='REGISTER' AND register_mapping_source_id=$4 AND cleared_at IS NULL`, observedAt, organizationID, deviceID, registerUUID(signal.MappingSourceID)); err != nil {
			return nil, fmt.Errorf("clear register alarm: %w", err)
		}
	}
	var emailEnabled bool
	var notifyRoleID pgtype.UUID
	if err = tx.QueryRow(ctx, `SELECT alarm_email_enabled, alarm_notify_role_id FROM plant.plant WHERE organization_id=$1 AND id=$2`, organizationID, plantID).Scan(&emailEnabled, &notifyRoleID); err != nil {
		return nil, fmt.Errorf("load plant alarm email settings: %w", err)
	}
	breaches := []alarmBreach{}
	for _, signal := range toOpen {
		snapshot, marshalErr := json.Marshal(registerAlarmSnapshot{MappingID: signal.MappingSourceID, AddressKey: signal.AddressKey, RawValue: signal.RawValue, NumericValue: signal.NumericValue, DisplayValue: signal.DisplayValue, Severity: signal.Severity})
		if marshalErr != nil {
			return nil, marshalErr
		}
		var openedID int64
		err = tx.QueryRow(ctx, `
INSERT INTO alarm.alarm_event (organization_id, plant_id, device_id, alarm_rule_id, point_key, severity, value, condition_snapshot, breached_at, source_type, register_mapping_source_id, register_snapshot)
VALUES ($1,$2,$3,NULL,$4,$5,$6,'[]'::jsonb,$7,'REGISTER',$8,$9)
			ON CONFLICT (device_id, register_mapping_source_id) WHERE cleared_at IS NULL DO NOTHING
		RETURNING id`, organizationID, plantID, deviceID, signal.AddressKey, signal.Severity, signal.NumericValue, observedAt, registerUUID(signal.MappingSourceID), snapshot).Scan(&openedID)
		if err != nil && err != pgx.ErrNoRows {
			return nil, fmt.Errorf("open register alarm: %w", err)
		}
		if err == nil {
			breaches = append(breaches, alarmBreach{plantID: plantID, plantCode: plantCode, plantName: plantName, deviceName: deviceName, severity: signal.Severity, notifyRoleID: notifyRoleID, sourceType: "REGISTER", registerAddress: signal.AddressKey, registerDisplay: signal.DisplayValue, registerRaw: signal.RawValue, registerNumeric: signal.NumericValue, plantEmailEnabled: emailEnabled})
		}
	}
	return breaches, nil
}

func registerUUID(value string) pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan(value)
	return id
}

func lookupRegisterValue(values map[string]float64, addressKey string) (float64, bool) {
	if value, ok := values[addressKey]; ok {
		return value, true
	}
	bare := strings.TrimPrefix(strings.TrimPrefix(addressKey, "reg"), "REG")
	if value, ok := values[bare]; ok {
		return value, true
	}
	value, ok := values["reg"+bare]
	return value, ok
}

func isIntegerRegisterValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && math.Trunc(value) == value
}
