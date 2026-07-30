package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/database/dbgen"
)

type AuditEvent struct {
	ID             int64           `json:"id"`
	OrganizationID string          `json:"organizationId,omitempty"`
	ActorUserID    string          `json:"actorUserId,omitempty"`
	ActorEmail     string          `json:"actorEmail,omitempty"`
	Action         string          `json:"action"`
	TargetType     string          `json:"targetType"`
	TargetID       string          `json:"targetId,omitempty"`
	BeforeData     json.RawMessage `json:"beforeData,omitempty"`
	AfterData      json.RawMessage `json:"afterData,omitempty"`
	SourceIP       string          `json:"sourceIp,omitempty"`
	CorrelationID  string          `json:"correlationId,omitempty"`
	OccurredAt     time.Time       `json:"occurredAt"`
}

type ClearResult struct {
	ClearedCount int64 `json:"clearedCount"`
}

func (s *Service) AuditEvents(ctx context.Context, principal auth.Principal, limit int) ([]AuditEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	allowed, err := s.queries.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "read", ResourceType: "audit"})
	if err != nil {
		return nil, fmt.Errorf("check audit read permission: %w", err)
	}
	if !allowed {
		return nil, ErrForbidden
	}
	rows, err := s.pool.Query(ctx, `
SELECT al.id, al.organization_id, al.actor_user_id, u.email, al.action, al.target_type,
       al.target_id, al.before_data, al.after_data, al.source_ip::text, al.correlation_id, al.occurred_at
FROM audit_log al
LEFT JOIN app_user u ON u.id = al.actor_user_id
WHERE al.occurred_at > COALESCE((
    SELECT max(marker.occurred_at)
    FROM audit_log marker
    WHERE marker.action='audit.cleared' AND marker.organization_id IS NULL
), '-infinity'::timestamptz)
AND EXISTS (
    SELECT 1 FROM user_role ur
    JOIN role r ON r.id = ur.role_id
    JOIN role_permission rp ON rp.role_id = ur.role_id
    JOIN permission pm ON pm.id = rp.permission_id
    WHERE ur.user_id = $1
      AND pm.action = 'read' AND pm.resource_type = 'audit'
      AND (r.organization_id IS NULL OR r.organization_id = ur.organization_id)
      AND (rp.organization_id IS NULL OR rp.organization_id = ur.organization_id)
      AND ur.plant_id IS NULL
      AND (ur.organization_id IS NULL OR ur.organization_id = al.organization_id)
)
ORDER BY al.occurred_at DESC, al.id DESC
LIMIT $2`, principal.UserID, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()
	events := []AuditEvent{}
	for rows.Next() {
		event, err := scanAuditEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Service) ClearAuditEvents(ctx context.Context, principal auth.Principal, confirmation string, sourceIP *netip.Addr) (ClearResult, error) {
	if !validHardDeleteConfirmation(confirmation, "DELETE ALL AUDIT LOGS") {
		return ClearResult{}, ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ClearResult{}, fmt.Errorf("begin audit clear: %w", err)
	}
	defer tx.Rollback(ctx)
	allowed, err := hasGlobalPermissionQuery(ctx, tx, principal, "clear", "audit")
	if err != nil {
		return ClearResult{}, fmt.Errorf("check global audit clear permission: %w", err)
	}
	if !allowed {
		return ClearResult{}, ErrForbidden
	}
	var count int64
	err = tx.QueryRow(ctx, `
SELECT count(*)
FROM audit_log al
WHERE al.occurred_at > COALESCE((
    SELECT max(marker.occurred_at)
    FROM audit_log marker
    WHERE marker.action='audit.cleared' AND marker.organization_id IS NULL
), '-infinity'::timestamptz)`).Scan(&count)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ClearResult{}, fmt.Errorf("count visible audit events: %w", err)
	}
	correlationID, err := newUUID()
	if err != nil {
		return ClearResult{}, err
	}
	after, _ := json.Marshal(map[string]any{"clearedCount": count, "sourceRowsRetained": true})
	q := s.queries.WithTx(tx)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: pgtype.UUID{}, ActorUserID: principal.UserID,
		Action: "audit.cleared", TargetType: "audit_log", TargetID: principal.UserID,
		AfterData: after, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return ClearResult{}, fmt.Errorf("append audit clear marker: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return ClearResult{}, fmt.Errorf("commit audit clear: %w", err)
	}
	return ClearResult{ClearedCount: count}, nil
}

type auditScanner interface{ Scan(...any) error }

func scanAuditEvent(row auditScanner) (AuditEvent, error) {
	var event AuditEvent
	var organizationID, actorUserID, targetID, correlationID pgtype.UUID
	var actorEmail, sourceIP pgtype.Text
	var beforeData, afterData []byte
	var occurredAt pgtype.Timestamptz
	if err := row.Scan(&event.ID, &organizationID, &actorUserID, &actorEmail, &event.Action, &event.TargetType, &targetID, &beforeData, &afterData, &sourceIP, &correlationID, &occurredAt); err != nil {
		return AuditEvent{}, fmt.Errorf("scan audit event: %w", err)
	}
	event.OrganizationID = uuidString(organizationID)
	event.ActorUserID = uuidString(actorUserID)
	event.TargetID = uuidString(targetID)
	event.CorrelationID = uuidString(correlationID)
	event.OccurredAt = occurredAt.Time
	if actorEmail.Valid {
		event.ActorEmail = actorEmail.String
	}
	if sourceIP.Valid {
		event.SourceIP = sourceIP.String
	}
	if len(beforeData) > 0 {
		event.BeforeData = append(json.RawMessage(nil), beforeData...)
	}
	if len(afterData) > 0 {
		event.AfterData = append(json.RawMessage(nil), afterData...)
	}
	return event, nil
}
