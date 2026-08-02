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
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"ygate/auth-service/internal/database/dbgen"
)

var (
	ErrInvalidCredentials     = errors.New("invalid credentials")
	ErrRateLimited            = errors.New("too many login attempts")
	ErrUnauthenticated        = errors.New("authentication required")
	ErrInvalidCurrentPassword = errors.New("current password is invalid")
	ErrWeakPassword           = errors.New("new password does not meet policy")
	dummyPasswordHash, _      = bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)
)

const maxRecentFailures = 10

type Service struct {
	pool            *pgxpool.Pool
	queries         *dbgen.Queries
	idleTimeout     time.Duration
	absoluteTimeout time.Duration
	resetTTL        time.Duration
	resetNotifier   ResetNotifier
}

type LoginInput struct {
	Identifier, Password, UserAgent string
	SourceIP                        *netip.Addr
}

type LoginResult struct {
	Token     string    `json:"-"`
	CSRFToken string    `json:"-"`
	ExpiresAt time.Time `json:"expiresAt"`
	User      LoginUser `json:"user"`
}

type LoginUser struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId,omitempty"`
	Email          string `json:"email"`
	DisplayName    string `json:"displayName"`
}

type Principal struct {
	SessionID      pgtype.UUID
	OrganizationID pgtype.UUID
	UserID         pgtype.UUID
	Email          string
	DisplayName    string
	passwordHash   string
	csrfHash       []byte
}

func (p Principal) User() LoginUser {
	return LoginUser{ID: uuidString(p.UserID), OrganizationID: uuidString(p.OrganizationID), Email: p.Email, DisplayName: p.DisplayName}
}

func (p Principal) ValidCSRF(token string) bool {
	return hashesEqual(p.csrfHash, token)
}

func New(pool *pgxpool.Pool, idleTimeout, absoluteTimeout time.Duration) *Service {
	return &Service{pool: pool, queries: dbgen.New(pool), idleTimeout: idleTimeout, absoluteTimeout: absoluteTimeout, resetTTL: 30 * time.Minute}
}

func (s *Service) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	identifier := strings.ToLower(strings.TrimSpace(input.Identifier))
	if identifier == "" || input.Password == "" {
		return LoginResult{}, ErrInvalidCredentials
	}

	failures, err := s.queries.CountRecentFailedAuthAttempts(ctx, dbgen.CountRecentFailedAuthAttemptsParams{Identifier: identifier, SourceIp: input.SourceIP})
	if err != nil {
		return LoginResult{}, fmt.Errorf("count login attempts: %w", err)
	}
	if failures >= maxRecentFailures {
		return LoginResult{}, ErrRateLimited
	}

	user, err := s.queries.GetLoginUser(ctx, identifier)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(input.Password))
		return LoginResult{}, s.recordFailure(ctx, nil, identifier, input, false)
	}
	if err != nil {
		return LoginResult{}, fmt.Errorf("load login user: %w", err)
	}
	if user.Status != "ACTIVE" || user.LockedUntil.Valid && user.LockedUntil.Time.After(time.Now()) {
		return LoginResult{}, s.recordFailure(ctx, &user, identifier, input, false)
	}
	if !VerifyPassword(user.PasswordHash, input.Password) {
		return LoginResult{}, s.recordFailure(ctx, &user, identifier, input, true)
	}

	return s.recordSuccess(ctx, user, input)
}

func (s *Service) recordFailure(ctx context.Context, user *dbgen.GetLoginUserRow, identifier string, input LoginInput, increment bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin failed login: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	if err = q.RecordAuthAttempt(ctx, dbgen.RecordAuthAttemptParams{Identifier: identifier, SourceIp: input.SourceIP, Success: false}); err != nil {
		return fmt.Errorf("record failed login: %w", err)
	}
	var organizationID, targetID pgtype.UUID
	if user != nil {
		organizationID, targetID = user.OrganizationID, user.ID
		if increment {
			if _, err = q.RecordLoginFailure(ctx, user.ID); err != nil {
				return fmt.Errorf("update failed login: %w", err)
			}
		}
	}
	detail, _ := json.Marshal(map[string]string{"identifier": identifier})
	if err = q.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{
		OrganizationID: organizationID,
		Action:         "auth.login.failed",
		TargetType:     "app_user",
		TargetID:       targetID,
		AfterData:      detail,
		SourceIp:       input.SourceIP,
	}); err != nil {
		return fmt.Errorf("audit failed login: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit failed login: %w", err)
	}
	return ErrInvalidCredentials
}

func (s *Service) recordSuccess(ctx context.Context, user dbgen.GetLoginUserRow, input LoginInput) (LoginResult, error) {
	token, tokenHash, err := newToken()
	if err != nil {
		return LoginResult{}, err
	}
	csrfToken, csrfHash, err := newToken()
	if err != nil {
		return LoginResult{}, err
	}
	sessionID, err := newUUID()
	if err != nil {
		return LoginResult{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return LoginResult{}, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(s.absoluteTimeout)
	idleExpiresAt := now.Add(s.idleTimeout)
	if idleExpiresAt.After(expiresAt) {
		idleExpiresAt = expiresAt
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return LoginResult{}, fmt.Errorf("begin login: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	if err = q.RecordLoginSuccess(ctx, user.ID); err != nil {
		return LoginResult{}, fmt.Errorf("reset login failures: %w", err)
	}
	if err = q.RecordAuthAttempt(ctx, dbgen.RecordAuthAttemptParams{Identifier: user.Email, SourceIp: input.SourceIP, Success: true}); err != nil {
		return LoginResult{}, fmt.Errorf("record successful login: %w", err)
	}
	if err = q.CreateUserSession(ctx, dbgen.CreateUserSessionParams{
		ID: sessionID, OrganizationID: user.OrganizationID, UserID: user.ID, TokenHash: tokenHash, CsrfHash: csrfHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true}, IdleExpiresAt: pgtype.Timestamptz{Time: idleExpiresAt, Valid: true},
		ClientIp: input.SourceIP, UserAgent: input.UserAgent,
	}); err != nil {
		return LoginResult{}, fmt.Errorf("create session: %w", err)
	}
	detail, _ := json.Marshal(map[string]string{"sessionId": uuidString(sessionID)})
	if err = q.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, Action: "auth.login.succeeded",
		TargetType: "app_user", TargetID: user.ID, AfterData: detail, SourceIp: input.SourceIP, CorrelationID: correlationID,
	}); err != nil {
		return LoginResult{}, fmt.Errorf("audit successful login: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return LoginResult{}, fmt.Errorf("commit login: %w", err)
	}
	return LoginResult{
		Token: token, CSRFToken: csrfToken, ExpiresAt: expiresAt,
		User: LoginUser{ID: uuidString(user.ID), OrganizationID: uuidString(user.OrganizationID), Email: user.Email, DisplayName: user.DisplayName},
	}, nil
}
func (s *Service) Permissions(ctx context.Context, userID pgtype.UUID) ([]string, error) {
	rows, err := s.queries.ListUserPermissions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user permissions: %w", err)
	}
	permissions := make([]string, len(rows))
	for i, row := range rows {
		permissions[i] = row.ResourceType + ":" + row.Action
	}
	return permissions, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	tokenHash, err := hashPresentedToken(token)
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}
	session, err := s.queries.GetActiveSession(ctx, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("load session: %w", err)
	}
	if err = s.queries.TouchSession(ctx, dbgen.TouchSessionParams{IdleSeconds: int64(s.idleTimeout / time.Second), SessionID: session.SessionID}); err != nil {
		return Principal{}, fmt.Errorf("touch session: %w", err)
	}
	return Principal{
		SessionID: session.SessionID, OrganizationID: session.OrganizationID, UserID: session.UserID,
		Email: session.Email, DisplayName: session.DisplayName, passwordHash: session.PasswordHash, csrfHash: session.CsrfHash,
	}, nil
}

func (s *Service) Logout(ctx context.Context, principal Principal, sourceIP *netip.Addr) error {
	return s.sessionAction(ctx, principal, sourceIP, "auth.logout", func(q *dbgen.Queries) error {
		return q.RevokeSession(ctx, principal.SessionID)
	})
}

func (s *Service) LogoutAll(ctx context.Context, principal Principal, sourceIP *netip.Addr) error {
	return s.sessionAction(ctx, principal, sourceIP, "auth.logout_all", func(q *dbgen.Queries) error {
		return q.RevokeAllUserSessions(ctx, principal.UserID)
	})
}

func (s *Service) ChangePassword(ctx context.Context, principal Principal, currentPassword, newPassword string, sourceIP *netip.Addr) error {
	if !VerifyPassword(principal.passwordHash, currentPassword) {
		return ErrInvalidCurrentPassword
	}
	if !ValidateNewPassword(newPassword) {
		return ErrWeakPassword
	}
	passwordHash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin password change: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	if err = q.UpdatePasswordAndRevokeOtherSessions(ctx, dbgen.UpdatePasswordAndRevokeOtherSessionsParams{
		UserID: principal.UserID, CurrentSessionID: principal.SessionID, PasswordHash: passwordHash,
	}); err != nil {
		return fmt.Errorf("change password: %w", err)
	}
	if err = createPrincipalAudit(ctx, q, principal, sourceIP, "auth.password.changed"); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit password change: %w", err)
	}
	return nil
}

func (s *Service) sessionAction(ctx context.Context, principal Principal, sourceIP *netip.Addr, action string, apply func(*dbgen.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin %s: %w", action, err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	if err = apply(q); err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	if err = createPrincipalAudit(ctx, q, principal, sourceIP, action); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit %s: %w", action, err)
	}
	return nil
}

func createPrincipalAudit(ctx context.Context, q *dbgen.Queries, principal Principal, sourceIP *netip.Addr, action string) error {
	correlationID, err := newUUID()
	if err != nil {
		return err
	}
	detail, _ := json.Marshal(map[string]string{"sessionId": uuidString(principal.SessionID)})
	if err = q.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{
		OrganizationID: principal.OrganizationID, ActorUserID: principal.UserID, Action: action,
		TargetType: "app_user", TargetID: principal.UserID, AfterData: detail, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return nil
}
