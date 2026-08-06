package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/database/dbgen"
)

var (
	ErrAlarmRuleNotFound  = errors.New("alarm rule not found")
	ErrAlarmRuleInvalid   = errors.New("invalid alarm rule data")
	ErrAlarmEventNotFound = errors.New("alarm event not found")
	alarmSeverities       = map[string]bool{"warning": true, "major": true, "critical": true}
)

// AlarmRuleCondition is one point/threshold check within a rule. A rule
// fires when its conditions combine (AND/OR, see AlarmRule.ConditionLogic)
// to true.
type AlarmRuleCondition struct {
	PointKey string   `json:"pointKey"`
	MinValue *float64 `json:"minValue"`
	MaxValue *float64 `json:"maxValue"`
}

type AlarmRule struct {
	ID             string               `json:"id"`
	OrganizationID string               `json:"organizationId"`
	PlantID        string               `json:"plantId"`
	DeviceID       string               `json:"deviceId"`
	Label          string               `json:"label"`
	ConditionLogic string               `json:"conditionLogic"`
	Conditions     []AlarmRuleCondition `json:"conditions"`
	Severity       string               `json:"severity"`
	IsActive       bool                 `json:"isActive"`
	NotifyRoleID   *string              `json:"notifyRoleId"`
	CreatedAt      time.Time            `json:"createdAt"`
	UpdatedAt      time.Time            `json:"updatedAt"`
}

// NotifyRoleOption is a role a caller can pick as an alarm rule's notify
// target, scoped to what's visible from the rule's organization (global
// system roles plus the organization's own custom roles).
type NotifyRoleOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AlarmEventCondition is one condition's value at the moment its rule
// breached (or, for a cleared event, would no longer breach) — a snapshot,
// since the rule's own conditions can change after the event is recorded.
type AlarmEventCondition struct {
	PointKey string   `json:"pointKey"`
	Value    float64  `json:"value"`
	MinValue *float64 `json:"minValue"`
	MaxValue *float64 `json:"maxValue"`
	Breached bool     `json:"breached"`
}

type AlarmEvent struct {
	ID                int64                 `json:"id"`
	OrganizationID    string                `json:"organizationId"`
	PlantID           string                `json:"plantId"`
	DeviceID          string                `json:"deviceId"`
	AlarmRuleID       string                `json:"alarmRuleId"`
	Severity          string                `json:"severity"`
	ConditionSnapshot []AlarmEventCondition `json:"conditionSnapshot"`
	BreachedAt        time.Time             `json:"breachedAt"`
	ClearedAt         *time.Time            `json:"clearedAt"`
	AcknowledgedBy    *string               `json:"acknowledgedBy"`
	AcknowledgedAt    *time.Time            `json:"acknowledgedAt"`
	AcknowledgedNote  *string               `json:"acknowledgedNote"`
}

type ConditionInput struct {
	PointKey string
	MinValue *float64
	MaxValue *float64
}

type CreateAlarmRuleInput struct {
	DeviceID       string
	Label          string
	ConditionLogic string
	Conditions     []ConditionInput
	Severity       string
	NotifyRoleID   *string
}

type UpdateAlarmRuleInput struct {
	Label          string
	ConditionLogic string
	Conditions     []ConditionInput
	Severity       string
	IsActive       bool
	NotifyRoleID   *string
}

func (s *Service) ListAlarmRules(ctx context.Context, principal auth.Principal, plantID string) ([]AlarmRule, error) {
	plant, err := s.authorizedAlarmPlant(ctx, s.queries, principal, plantID, "read")
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, organization_id, plant_id, device_id, label, condition_logic, severity, is_active, notify_role_id, created_at, updated_at
FROM alarm.alarm_rule
WHERE organization_id=$1 AND plant_id=$2
ORDER BY label, id`, plant.OrganizationID, plant.ID)
	if err != nil {
		return nil, fmt.Errorf("list alarm rules: %w", err)
	}
	defer rows.Close()
	rules := []AlarmRule{}
	for rows.Next() {
		rule, scanErr := scanAlarmRule(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		rules = append(rules, rule)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("list alarm rules: %w", err)
	}
	for i := range rules {
		ruleID, parseErr := parseUUID(rules[i].ID)
		if parseErr != nil {
			continue
		}
		if rules[i].Conditions, err = s.loadAlarmRuleConditions(ctx, s.pool, ruleID); err != nil {
			return nil, err
		}
	}
	return rules, nil
}

func (s *Service) CreateAlarmRule(ctx context.Context, principal auth.Principal, plantID string, input CreateAlarmRuleInput, sourceIP *netip.Addr) (AlarmRule, error) {
	deviceID, err := parseUUID(input.DeviceID)
	if err != nil {
		return AlarmRule{}, ErrAlarmRuleInvalid
	}
	label, severity, conditionLogic, conditions, err := validateAlarmRule(input.Label, input.Severity, input.ConditionLogic, input.Conditions)
	if err != nil {
		return AlarmRule{}, err
	}
	id, err := newUUID()
	if err != nil {
		return AlarmRule{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return AlarmRule{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AlarmRule{}, fmt.Errorf("begin create alarm rule: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	plant, err := s.authorizedAlarmPlant(ctx, q, principal, plantID, "create")
	if err != nil {
		return AlarmRule{}, err
	}
	if _, err = q.GetPlantDevice(ctx, dbgen.GetPlantDeviceParams{OrganizationID: plant.OrganizationID, PlantID: plant.ID, DeviceID: deviceID}); errors.Is(err, pgx.ErrNoRows) {
		return AlarmRule{}, ErrAlarmRuleInvalid
	} else if err != nil {
		return AlarmRule{}, fmt.Errorf("verify alarm rule device: %w", err)
	}
	notifyRoleID, err := s.validateNotifyRole(ctx, tx, plant.OrganizationID, input.NotifyRoleID)
	if err != nil {
		return AlarmRule{}, err
	}
	row := tx.QueryRow(ctx, `
INSERT INTO alarm.alarm_rule (id, organization_id, plant_id, device_id, label, condition_logic, severity, notify_role_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING id, organization_id, plant_id, device_id, label, condition_logic, severity, is_active, notify_role_id, created_at, updated_at`,
		id, plant.OrganizationID, plant.ID, deviceID, label, conditionLogic, severity, notifyRoleID)
	rule, err := scanAlarmRule(row)
	if err != nil {
		return AlarmRule{}, mapAlarmWriteError(err)
	}
	if rule.Conditions, err = s.replaceAlarmRuleConditions(ctx, tx, id, conditions); err != nil {
		return AlarmRule{}, err
	}
	after, _ := json.Marshal(rule)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: plant.OrganizationID, ActorUserID: principal.UserID, Action: "alarm_rule.created",
		TargetType: "alarm_rule", TargetID: id, AfterData: after, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return AlarmRule{}, fmt.Errorf("audit alarm rule create: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return AlarmRule{}, fmt.Errorf("commit create alarm rule: %w", err)
	}
	return rule, nil
}

func (s *Service) UpdateAlarmRule(ctx context.Context, principal auth.Principal, plantID, ruleID string, input UpdateAlarmRuleInput, sourceIP *netip.Addr) (AlarmRule, error) {
	id, err := parseUUID(ruleID)
	if err != nil {
		return AlarmRule{}, ErrAlarmRuleNotFound
	}
	plantUUID, err := parseUUID(plantID)
	if err != nil {
		return AlarmRule{}, ErrAlarmRuleNotFound
	}
	label, severity, conditionLogic, conditions, err := validateAlarmRule(input.Label, input.Severity, input.ConditionLogic, input.Conditions)
	if err != nil {
		return AlarmRule{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return AlarmRule{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AlarmRule{}, fmt.Errorf("begin update alarm rule: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	before, err := lockAlarmRule(ctx, tx, plantUUID, id)
	if err != nil {
		return AlarmRule{}, err
	}
	if _, err = s.authorizedAlarmPlant(ctx, q, principal, plantID, "update"); err != nil {
		return AlarmRule{}, err
	}
	notifyRoleID, err := s.validateNotifyRole(ctx, tx, before.organizationID, input.NotifyRoleID)
	if err != nil {
		return AlarmRule{}, err
	}
	row := tx.QueryRow(ctx, `
UPDATE alarm.alarm_rule SET label=$2, condition_logic=$3, severity=$4, is_active=$5, notify_role_id=$6, updated_at=now()
WHERE id=$1
RETURNING id, organization_id, plant_id, device_id, label, condition_logic, severity, is_active, notify_role_id, created_at, updated_at`,
		id, label, conditionLogic, severity, input.IsActive, notifyRoleID)
	rule, err := scanAlarmRule(row)
	if err != nil {
		return AlarmRule{}, mapAlarmWriteError(err)
	}
	if rule.Conditions, err = s.replaceAlarmRuleConditions(ctx, tx, id, conditions); err != nil {
		return AlarmRule{}, err
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(rule)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: before.organizationID, ActorUserID: principal.UserID, Action: "alarm_rule.updated",
		TargetType: "alarm_rule", TargetID: id, BeforeData: beforeJSON, AfterData: afterJSON,
		SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return AlarmRule{}, fmt.Errorf("audit alarm rule update: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return AlarmRule{}, fmt.Errorf("commit update alarm rule: %w", err)
	}
	return rule, nil
}

func (s *Service) DeleteAlarmRule(ctx context.Context, principal auth.Principal, plantID, ruleID string, sourceIP *netip.Addr) error {
	id, err := parseUUID(ruleID)
	if err != nil {
		return ErrAlarmRuleNotFound
	}
	plantUUID, err := parseUUID(plantID)
	if err != nil {
		return ErrAlarmRuleNotFound
	}
	correlationID, err := newUUID()
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete alarm rule: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	before, err := lockAlarmRule(ctx, tx, plantUUID, id)
	if err != nil {
		return err
	}
	if _, err = s.authorizedAlarmPlant(ctx, q, principal, plantID, "delete"); err != nil {
		return err
	}
	beforeJSON, _ := json.Marshal(before)
	if _, err = tx.Exec(ctx, `DELETE FROM alarm.alarm_rule WHERE id=$1`, id); err != nil {
		return mapAlarmWriteError(err)
	}
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: before.organizationID, ActorUserID: principal.UserID, Action: "alarm_rule.deleted",
		TargetType: "alarm_rule", TargetID: id, BeforeData: beforeJSON, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return fmt.Errorf("audit alarm rule delete: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *Service) ListAlarmEvents(ctx context.Context, principal auth.Principal, plantID string, limit int) ([]AlarmEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	plant, err := s.authorizedAlarmPlant(ctx, s.queries, principal, plantID, "read")
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, organization_id, plant_id, device_id, alarm_rule_id, severity, condition_snapshot,
       breached_at, cleared_at, acknowledged_by, acknowledged_at, acknowledged_note
FROM alarm.alarm_event
WHERE organization_id=$1 AND plant_id=$2
ORDER BY breached_at DESC, id DESC
LIMIT $3`, plant.OrganizationID, plant.ID, limit)
	if err != nil {
		return nil, fmt.Errorf("list alarm events: %w", err)
	}
	defer rows.Close()
	events := []AlarmEvent{}
	for rows.Next() {
		event, scanErr := scanAlarmEvent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// AlarmEventsSince returns events newer than sinceEventID in ascending order,
// for realtime tail polling (see httpapi.realtimeHandler).
func (s *Service) AlarmEventsSince(ctx context.Context, principal auth.Principal, plantID string, sinceEventID int64) ([]AlarmEvent, error) {
	plant, err := s.authorizedAlarmPlant(ctx, s.queries, principal, plantID, "read")
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, organization_id, plant_id, device_id, alarm_rule_id, severity, condition_snapshot,
       breached_at, cleared_at, acknowledged_by, acknowledged_at, acknowledged_note
FROM alarm.alarm_event
WHERE organization_id=$1 AND plant_id=$2 AND id > $3
ORDER BY id ASC
LIMIT 200`, plant.OrganizationID, plant.ID, sinceEventID)
	if err != nil {
		return nil, fmt.Errorf("list alarm events since: %w", err)
	}
	defer rows.Close()
	events := []AlarmEvent{}
	for rows.Next() {
		event, scanErr := scanAlarmEvent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// LatestAlarmEventID returns the current maximum event id for a plant, used
// as the realtime baseline so a fresh connection does not replay history.
func (s *Service) LatestAlarmEventID(ctx context.Context, principal auth.Principal, plantID string) (int64, error) {
	plant, err := s.authorizedAlarmPlant(ctx, s.queries, principal, plantID, "read")
	if err != nil {
		return 0, err
	}
	var latest int64
	if err = s.pool.QueryRow(ctx, `SELECT COALESCE(max(id), 0) FROM alarm.alarm_event WHERE organization_id=$1 AND plant_id=$2`, plant.OrganizationID, plant.ID).Scan(&latest); err != nil {
		return 0, fmt.Errorf("latest alarm event id: %w", err)
	}
	return latest, nil
}

func (s *Service) AcknowledgeAlarmEvent(ctx context.Context, principal auth.Principal, plantID, eventID string, note string, sourceIP *netip.Addr) (AlarmEvent, error) {
	id, err := parseInt64(eventID)
	if err != nil {
		return AlarmEvent{}, ErrAlarmEventNotFound
	}
	plantUUID, err := parseUUID(plantID)
	if err != nil {
		return AlarmEvent{}, ErrAlarmEventNotFound
	}
	note = strings.TrimSpace(note)
	if len(note) > 500 {
		return AlarmEvent{}, ErrAlarmRuleInvalid
	}
	correlationID, err := newUUID()
	if err != nil {
		return AlarmEvent{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AlarmEvent{}, fmt.Errorf("begin acknowledge alarm event: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	var organizationID pgtype.UUID
	current, err := scanAlarmEvent(tx.QueryRow(ctx, `
SELECT id, organization_id, plant_id, device_id, alarm_rule_id, severity, condition_snapshot,
       breached_at, cleared_at, acknowledged_by, acknowledged_at, acknowledged_note
FROM alarm.alarm_event WHERE id=$1 AND plant_id=$2 FOR UPDATE`, id, plantUUID))
	if errors.Is(err, pgx.ErrNoRows) {
		return AlarmEvent{}, ErrAlarmEventNotFound
	}
	if err != nil {
		return AlarmEvent{}, err
	}
	organizationID, err = parseUUID(current.OrganizationID)
	if err != nil {
		return AlarmEvent{}, fmt.Errorf("parse alarm event organization: %w", err)
	}
	if _, err = s.authorizedAlarmPlant(ctx, q, principal, plantID, "ack"); err != nil {
		return AlarmEvent{}, err
	}
	if current.AcknowledgedBy != nil {
		return current, tx.Commit(ctx)
	}
	event, err := scanAlarmEvent(tx.QueryRow(ctx, `
UPDATE alarm.alarm_event SET acknowledged_by=$2, acknowledged_at=now(), acknowledged_note=$3
WHERE id=$1
RETURNING id, organization_id, plant_id, device_id, alarm_rule_id, severity, condition_snapshot,
          breached_at, cleared_at, acknowledged_by, acknowledged_at, acknowledged_note`,
		id, principal.UserID, note))
	if err != nil {
		return AlarmEvent{}, fmt.Errorf("acknowledge alarm event: %w", err)
	}
	after, _ := json.Marshal(event)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "alarm_event.acknowledged",
		TargetType: "alarm_event", AfterData: after, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return AlarmEvent{}, fmt.Errorf("audit alarm event acknowledge: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return AlarmEvent{}, fmt.Errorf("commit acknowledge alarm event: %w", err)
	}
	return event, nil
}

func (s *Service) authorizedAlarmPlant(ctx context.Context, q *dbgen.Queries, principal auth.Principal, plantID, action string) (dbgen.GetAuthorizedPlantResourceRow, error) {
	id, err := parseUUID(plantID)
	if err != nil {
		return dbgen.GetAuthorizedPlantResourceRow{}, ErrNotFound
	}
	plant, err := q.GetAuthorizedPlantResource(ctx, dbgen.GetAuthorizedPlantResourceParams{
		PlantID: id, UserID: principal.UserID, Action: action, ResourceType: "alarm",
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.GetAuthorizedPlantResourceRow{}, ErrNotFound
	}
	if err != nil {
		return dbgen.GetAuthorizedPlantResourceRow{}, fmt.Errorf("authorize alarm plant: %w", err)
	}
	return plant, nil
}

// ListAlarmNotifyRoles returns the roles a caller may pick as an alarm
// rule's notify target: global system roles plus the rule's own organization's
// custom roles. Gated the same as reading the plant's alarm rules.
func (s *Service) ListAlarmNotifyRoles(ctx context.Context, principal auth.Principal, plantID string) ([]NotifyRoleOption, error) {
	plant, err := s.authorizedAlarmPlant(ctx, s.queries, principal, plantID, "read")
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, name FROM auth.role WHERE organization_id IS NULL OR organization_id=$1 ORDER BY name`, plant.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("list alarm notify roles: %w", err)
	}
	defer rows.Close()
	options := []NotifyRoleOption{}
	for rows.Next() {
		var id pgtype.UUID
		var name string
		if err = rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan alarm notify role: %w", err)
		}
		options = append(options, NotifyRoleOption{ID: uuidString(id), Name: name})
	}
	return options, rows.Err()
}

// validateNotifyRole resolves an optional notify-role id, confirming it names
// a role visible to the rule's organization (global or org-owned) so a rule
// can't be wired to notify a role from another organization.
func (s *Service) validateNotifyRole(ctx context.Context, tx pgx.Tx, organizationID pgtype.UUID, roleID *string) (pgtype.UUID, error) {
	if roleID == nil || strings.TrimSpace(*roleID) == "" {
		return pgtype.UUID{}, nil
	}
	id, err := parseUUID(*roleID)
	if err != nil {
		return pgtype.UUID{}, ErrAlarmRuleInvalid
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM auth.role WHERE id=$1 AND (organization_id IS NULL OR organization_id=$2))`, id, organizationID).Scan(&exists); err != nil {
		return pgtype.UUID{}, fmt.Errorf("validate notify role: %w", err)
	}
	if !exists {
		return pgtype.UUID{}, ErrAlarmRuleInvalid
	}
	return id, nil
}

type storedAlarmRule struct {
	organizationID pgtype.UUID
}

// lockAlarmRule loads and row-locks an alarm rule ahead of an update or
// delete, mirroring how UpdateUser/UpdatePlant load the "before" state
// before authorizing the write. Scoping by plantID (matching the URL's
// {plantId} segment, like UpdateDevice/DeleteDevice) ensures a rule id can't
// be mutated through a mismatched plant path.
func lockAlarmRule(ctx context.Context, tx pgx.Tx, plantID, id pgtype.UUID) (storedAlarmRule, error) {
	var organizationID pgtype.UUID
	err := tx.QueryRow(ctx, `SELECT organization_id FROM alarm.alarm_rule WHERE id=$1 AND plant_id=$2 FOR UPDATE`, id, plantID).Scan(&organizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return storedAlarmRule{}, ErrAlarmRuleNotFound
	}
	if err != nil {
		return storedAlarmRule{}, fmt.Errorf("lock alarm rule: %w", err)
	}
	return storedAlarmRule{organizationID: organizationID}, nil
}

// validateAlarmRule validates the fields shared by create and update: label,
// severity, AND/OR condition logic, and every condition's point key and
// min/max threshold pair.
func validateAlarmRule(label, severity, conditionLogic string, conditions []ConditionInput) (string, string, string, []ConditionInput, error) {
	label = strings.TrimSpace(label)
	severity = strings.ToLower(strings.TrimSpace(severity))
	conditionLogic = strings.ToUpper(strings.TrimSpace(conditionLogic))
	if conditionLogic == "" {
		conditionLogic = "AND"
	}
	if len(label) == 0 || len(label) > 200 || !alarmSeverities[severity] || (conditionLogic != "AND" && conditionLogic != "OR") {
		return label, severity, conditionLogic, conditions, ErrAlarmRuleInvalid
	}
	if len(conditions) == 0 || len(conditions) > 20 {
		return label, severity, conditionLogic, conditions, ErrAlarmRuleInvalid
	}
	normalized := make([]ConditionInput, len(conditions))
	for i, condition := range conditions {
		pointKey := strings.TrimSpace(condition.PointKey)
		if len(pointKey) == 0 || len(pointKey) > 200 {
			return label, severity, conditionLogic, conditions, ErrAlarmRuleInvalid
		}
		if condition.MinValue == nil && condition.MaxValue == nil {
			return label, severity, conditionLogic, conditions, ErrAlarmRuleInvalid
		}
		for _, value := range []*float64{condition.MinValue, condition.MaxValue} {
			if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
				return label, severity, conditionLogic, conditions, ErrAlarmRuleInvalid
			}
		}
		if condition.MinValue != nil && condition.MaxValue != nil && *condition.MinValue >= *condition.MaxValue {
			return label, severity, conditionLogic, conditions, ErrAlarmRuleInvalid
		}
		normalized[i] = ConditionInput{PointKey: pointKey, MinValue: condition.MinValue, MaxValue: condition.MaxValue}
	}
	return label, severity, conditionLogic, normalized, nil
}

// pgxQuerier is satisfied by both *pgxpool.Pool and pgx.Tx, so
// loadAlarmRuleConditions can run either as a plain read or inside a
// caller's transaction.
type pgxQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (s *Service) loadAlarmRuleConditions(ctx context.Context, q pgxQuerier, ruleID pgtype.UUID) ([]AlarmRuleCondition, error) {
	rows, err := q.Query(ctx, `SELECT point_key, min_value, max_value FROM alarm.alarm_rule_condition WHERE alarm_rule_id=$1 ORDER BY position`, ruleID)
	if err != nil {
		return nil, fmt.Errorf("load alarm rule conditions: %w", err)
	}
	defer rows.Close()
	conditions := []AlarmRuleCondition{}
	for rows.Next() {
		var condition AlarmRuleCondition
		var minValue, maxValue pgtype.Float8
		if err = rows.Scan(&condition.PointKey, &minValue, &maxValue); err != nil {
			return nil, fmt.Errorf("scan alarm rule condition: %w", err)
		}
		condition.MinValue = floatPointer(minValue)
		condition.MaxValue = floatPointer(maxValue)
		conditions = append(conditions, condition)
	}
	return conditions, rows.Err()
}

// replaceAlarmRuleConditions swaps a rule's conditions wholesale (delete then
// reinsert) rather than diffing, since a rule rarely has more than a handful
// and the editor always submits the full list.
func (s *Service) replaceAlarmRuleConditions(ctx context.Context, tx pgx.Tx, ruleID pgtype.UUID, conditions []ConditionInput) ([]AlarmRuleCondition, error) {
	if _, err := tx.Exec(ctx, `DELETE FROM alarm.alarm_rule_condition WHERE alarm_rule_id=$1`, ruleID); err != nil {
		return nil, fmt.Errorf("clear alarm rule conditions: %w", err)
	}
	result := make([]AlarmRuleCondition, 0, len(conditions))
	for position, condition := range conditions {
		conditionID, err := newUUID()
		if err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `
INSERT INTO alarm.alarm_rule_condition (id, alarm_rule_id, point_key, min_value, max_value, position)
VALUES ($1,$2,$3,$4,$5,$6)`,
			conditionID, ruleID, condition.PointKey, nullableFloat(condition.MinValue), nullableFloat(condition.MaxValue), position); err != nil {
			return nil, mapAlarmWriteError(err)
		}
		result = append(result, AlarmRuleCondition{PointKey: condition.PointKey, MinValue: condition.MinValue, MaxValue: condition.MaxValue})
	}
	return result, nil
}

func scanAlarmRule(row auditScanner) (AlarmRule, error) {
	var rule AlarmRule
	var id, organizationID, plantID, deviceID, notifyRoleID pgtype.UUID
	var createdAt, updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &organizationID, &plantID, &deviceID, &rule.Label, &rule.ConditionLogic, &rule.Severity, &rule.IsActive, &notifyRoleID, &createdAt, &updatedAt); err != nil {
		return AlarmRule{}, fmt.Errorf("scan alarm rule: %w", err)
	}
	rule.ID = uuidString(id)
	rule.OrganizationID = uuidString(organizationID)
	rule.PlantID = uuidString(plantID)
	rule.NotifyRoleID = orgPointer(notifyRoleID)
	rule.DeviceID = uuidString(deviceID)
	rule.CreatedAt = createdAt.Time
	rule.UpdatedAt = updatedAt.Time
	return rule, nil
}

func scanAlarmEvent(row auditScanner) (AlarmEvent, error) {
	var event AlarmEvent
	var organizationID, plantID, deviceID, alarmRuleID, acknowledgedBy pgtype.UUID
	var breachedAt pgtype.Timestamptz
	var clearedAt, acknowledgedAt pgtype.Timestamptz
	var acknowledgedNote pgtype.Text
	var conditionSnapshot []byte
	if err := row.Scan(&event.ID, &organizationID, &plantID, &deviceID, &alarmRuleID, &event.Severity, &conditionSnapshot,
		&breachedAt, &clearedAt, &acknowledgedBy, &acknowledgedAt, &acknowledgedNote); err != nil {
		return AlarmEvent{}, fmt.Errorf("scan alarm event: %w", err)
	}
	event.OrganizationID = uuidString(organizationID)
	event.PlantID = uuidString(plantID)
	event.DeviceID = uuidString(deviceID)
	event.AlarmRuleID = uuidString(alarmRuleID)
	event.ConditionSnapshot = []AlarmEventCondition{}
	if len(conditionSnapshot) > 0 {
		if err := json.Unmarshal(conditionSnapshot, &event.ConditionSnapshot); err != nil {
			return AlarmEvent{}, fmt.Errorf("decode alarm event condition snapshot: %w", err)
		}
	}
	event.BreachedAt = breachedAt.Time
	event.ClearedAt = timePointer(clearedAt)
	event.AcknowledgedBy = orgPointer(acknowledgedBy)
	event.AcknowledgedAt = timePointer(acknowledgedAt)
	if acknowledgedNote.Valid {
		event.AcknowledgedNote = &acknowledgedNote.String
	}
	return event, nil
}

func mapAlarmWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505", "23503", "23514", "22023":
			return ErrAlarmRuleInvalid
		}
	}
	return fmt.Errorf("write alarm rule: %w", err)
}

func parseInt64(value string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || id <= 0 {
		return 0, ErrAlarmEventNotFound
	}
	return id, nil
}
