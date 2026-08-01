package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/auth-service/internal/database/dbgen"
)

var ErrInvalidResetToken = errors.New("invalid or expired reset token")

const (
	maxResetRequests = 5
	maxResetAttempts = 10
)

type ResetNotifier func(context.Context, string, string) error

func (s *Service) ConfigurePasswordRecovery(ttl time.Duration, notifier ResetNotifier) {
	if ttl > 0 {
		s.resetTTL = ttl
	}
	s.resetNotifier = notifier
}

func (s *Service) RequestPasswordReset(ctx context.Context, email string, sourceIP *netip.Addr) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || len(email) > 320 {
		return nil
	}
	attempts, err := s.queries.CountRecentPasswordRecoveryAttempts(ctx, dbgen.CountRecentPasswordRecoveryAttemptsParams{
		Operation: "REQUEST", Identifier: email, SourceIp: sourceIP,
	})
	if err != nil {
		return fmt.Errorf("count password reset requests: %w", err)
	}
	if attempts >= maxResetRequests {
		return nil
	}
	user, err := s.queries.GetPasswordResetUser(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && (user.Status != "ACTIVE" || s.resetNotifier == nil) {
		return s.queries.RecordPasswordRecoveryAttempt(ctx, dbgen.RecordPasswordRecoveryAttemptParams{
			Operation: "REQUEST", Identifier: email, SourceIp: sourceIP, Success: false,
		})
	}
	if err != nil {
		return fmt.Errorf("load password reset user: %w", err)
	}
	token, tokenHash, err := newToken()
	if err != nil {
		return err
	}
	tokenID, err := newUUID()
	if err != nil {
		return err
	}
	expiresAt := time.Now().UTC().Add(s.resetTTL)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin password reset request: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	if err = q.InvalidatePasswordResetTokens(ctx, user.ID); err != nil {
		return fmt.Errorf("invalidate reset tokens: %w", err)
	}
	if err = q.CreatePasswordResetToken(ctx, dbgen.CreatePasswordResetTokenParams{
		ID: tokenID, OrganizationID: user.OrganizationID, UserID: user.ID, TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true}, RequestedIp: sourceIP,
	}); err != nil {
		return fmt.Errorf("create password reset token: %w", err)
	}
	if err = q.RecordPasswordRecoveryAttempt(ctx, dbgen.RecordPasswordRecoveryAttemptParams{
		Operation: "REQUEST", Identifier: email, SourceIp: sourceIP, Success: true,
	}); err != nil {
		return fmt.Errorf("record password reset request: %w", err)
	}
	detail, _ := json.Marshal(map[string]string{"delivery": "notifier"})
	if err = q.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{
		OrganizationID: user.OrganizationID, Action: "auth.password_reset.requested", TargetType: "app_user",
		TargetID: user.ID, AfterData: detail, SourceIp: sourceIP,
	}); err != nil {
		return fmt.Errorf("audit password reset request: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit password reset request: %w", err)
	}
	if err = s.resetNotifier(ctx, user.Email, token); err != nil {
		return fmt.Errorf("notify password reset: %w", err)
	}
	return nil
}

func (s *Service) ResetPassword(ctx context.Context, token, newPassword string, sourceIP *netip.Addr) error {
	if !ValidateNewPassword(newPassword) {
		return ErrWeakPassword
	}
	attempts, err := s.queries.CountRecentPasswordRecoveryAttempts(ctx, dbgen.CountRecentPasswordRecoveryAttemptsParams{
		Operation: "RESET", SourceIp: sourceIP,
	})
	if err != nil {
		return fmt.Errorf("count password reset attempts: %w", err)
	}
	if attempts >= maxResetAttempts {
		return ErrRateLimited
	}
	tokenHash, err := hashPresentedToken(token)
	if err != nil {
		_ = s.recordResetAttempt(ctx, sourceIP, false)
		return ErrInvalidResetToken
	}
	passwordHash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash reset password: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin password reset: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	reset, err := q.GetActivePasswordResetToken(ctx, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		if recordErr := q.RecordPasswordRecoveryAttempt(ctx, dbgen.RecordPasswordRecoveryAttemptParams{Operation: "RESET", SourceIp: sourceIP, Success: false}); recordErr != nil {
			return fmt.Errorf("record invalid reset: %w", recordErr)
		}
		if err = tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit invalid reset: %w", err)
		}
		return ErrInvalidResetToken
	}
	if err != nil {
		return fmt.Errorf("load password reset token: %w", err)
	}
	accountAttempts, err := q.CountRecentPasswordRecoveryAttempts(ctx, dbgen.CountRecentPasswordRecoveryAttemptsParams{
		Operation: "RESET", Identifier: reset.Email, SourceIp: sourceIP,
	})
	if err != nil {
		return fmt.Errorf("count account password reset attempts: %w", err)
	}
	if accountAttempts >= maxResetAttempts {
		return ErrRateLimited
	}
	if err = q.MarkPasswordResetTokenUsed(ctx, reset.TokenID); err != nil {
		return fmt.Errorf("consume password reset token: %w", err)
	}
	if err = q.UpdateUserPassword(ctx, dbgen.UpdateUserPasswordParams{PasswordHash: passwordHash, UserID: reset.UserID}); err != nil {
		return fmt.Errorf("update reset password: %w", err)
	}
	if err = q.RevokeAllUserSessions(ctx, reset.UserID); err != nil {
		return fmt.Errorf("revoke sessions after reset: %w", err)
	}
	if err = q.RecordPasswordRecoveryAttempt(ctx, dbgen.RecordPasswordRecoveryAttemptParams{Operation: "RESET", Identifier: reset.Email, SourceIp: sourceIP, Success: true}); err != nil {
		return fmt.Errorf("record password reset: %w", err)
	}
	if err = q.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{
		OrganizationID: reset.OrganizationID, Action: "auth.password_reset.completed",
		TargetType: "app_user", TargetID: reset.UserID, SourceIp: sourceIP,
	}); err != nil {
		return fmt.Errorf("audit password reset: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit password reset: %w", err)
	}
	return nil
}

func (s *Service) recordResetAttempt(ctx context.Context, sourceIP *netip.Addr, success bool) error {
	return s.queries.RecordPasswordRecoveryAttempt(ctx, dbgen.RecordPasswordRecoveryAttemptParams{
		Operation: "RESET", SourceIp: sourceIP, Success: success,
	})
}
