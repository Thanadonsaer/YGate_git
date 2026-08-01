package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/auth-service/internal/database/dbgen"
)

var (
	ErrSessionNotFound     = errors.New("session not found")
	ErrSessionConfirmation = errors.New("invalid session clear confirmation")
)

type SessionInfo struct {
	ID            string     `json:"id"`
	CreatedAt     time.Time  `json:"createdAt"`
	LastSeenAt    time.Time  `json:"lastSeenAt"`
	ExpiresAt     time.Time  `json:"expiresAt"`
	IdleExpiresAt time.Time  `json:"idleExpiresAt"`
	RevokedAt     *time.Time `json:"revokedAt,omitempty"`
	ClientIP      string     `json:"clientIp,omitempty"`
	UserAgent     string     `json:"userAgent"`
	Current       bool       `json:"current"`
}

func (s *Service) Sessions(ctx context.Context, principal Principal) ([]SessionInfo, error) {
	rows, err := s.queries.ListUserSessions(ctx, principal.UserID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	result := make([]SessionInfo, 0, len(rows))
	for _, row := range rows {
		var revokedAt *time.Time
		if row.RevokedAt.Valid {
			value := row.RevokedAt.Time
			revokedAt = &value
		}
		clientIP := ""
		if row.ClientIp != nil {
			clientIP = row.ClientIp.String()
		}
		result = append(result, SessionInfo{
			ID: uuidString(row.ID), CreatedAt: row.CreatedAt.Time, LastSeenAt: row.LastSeenAt.Time,
			ExpiresAt: row.ExpiresAt.Time, IdleExpiresAt: row.IdleExpiresAt.Time, RevokedAt: revokedAt,
			ClientIP: clientIP, UserAgent: row.UserAgent, Current: row.ID == principal.SessionID,
		})
	}
	return result, nil
}

func (s *Service) RevokeOwnSession(ctx context.Context, principal Principal, sessionID string, sourceIP *netip.Addr) (bool, error) {
	var id pgtype.UUID
	if err := id.Scan(sessionID); err != nil || !id.Valid {
		return false, ErrSessionNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin session revoke: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	if _, err = q.RevokeOwnedSession(ctx, dbgen.RevokeOwnedSessionParams{SessionID: id, UserID: principal.UserID}); errors.Is(err, pgx.ErrNoRows) {
		return false, ErrSessionNotFound
	} else if err != nil {
		return false, fmt.Errorf("revoke session: %w", err)
	}
	correlationID, err := newUUID()
	if err != nil {
		return false, err
	}
	detail, _ := json.Marshal(map[string]string{"sessionId": uuidString(id)})
	if err = q.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{
		OrganizationID: principal.OrganizationID, ActorUserID: principal.UserID,
		Action: "auth.session.revoked", TargetType: "user_session", TargetID: id,
		AfterData: detail, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return false, fmt.Errorf("audit session revoke: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit session revoke: %w", err)
	}
	return id == principal.SessionID, nil
}

func (s *Service) ClearOwnSessions(ctx context.Context, principal Principal, confirmation string, sourceIP *netip.Addr) error {
	if confirmation != "DELETE" && confirmation != "DELETE ALL SESSIONS" {
		return ErrSessionConfirmation
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin session clear: %w", err)
	}
	defer tx.Rollback(ctx)
	var count int64
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM user_session WHERE user_id=$1`, principal.UserID).Scan(&count); err != nil {
		return fmt.Errorf("count sessions to clear: %w", err)
	}
	correlationID, err := newUUID()
	if err != nil {
		return err
	}
	detail, _ := json.Marshal(map[string]any{"deletedCount": count, "includedCurrent": true})
	q := s.queries.WithTx(tx)
	if err = q.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{
		OrganizationID: principal.OrganizationID, ActorUserID: principal.UserID,
		Action: "auth.sessions.cleared", TargetType: "app_user", TargetID: principal.UserID,
		AfterData: detail, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return fmt.Errorf("audit session clear: %w", err)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM user_session WHERE user_id=$1`, principal.UserID); err != nil {
		return fmt.Errorf("clear sessions: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit session clear: %w", err)
	}
	return nil
}
