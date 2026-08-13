package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrRegistrationInvalid     = errors.New("invalid registration")
	ErrRegistrationUnavailable = errors.New("registration email is unavailable")
	ErrEmailVerificationToken  = errors.New("invalid or expired email verification token")
	ErrRegistrationConflict    = errors.New("email or username already exists")
)

type VerificationNotifier func(context.Context, string, string) error

type RegisterInput struct {
	Email       string
	Username    string
	DisplayName string
	Password    string
}

func (s *Service) ConfigureEmailVerification(notifier VerificationNotifier) {
	s.verificationNotifier = notifier
}

func (s *Service) Register(ctx context.Context, input RegisterInput, sourceIP *netip.Addr) error {
	if s.verificationNotifier == nil {
		return ErrRegistrationUnavailable
	}
	email := strings.ToLower(strings.TrimSpace(input.Email))
	username := strings.ToLower(strings.TrimSpace(input.Username))
	displayName := strings.TrimSpace(input.DisplayName)
	if !validRegistrationInput(email, username, displayName, input.Password) {
		return ErrRegistrationInvalid
	}
	token, tokenHash, err := newToken()
	if err != nil {
		return err
	}
	userID, err := newUUID()
	if err != nil {
		return err
	}
	tokenID, err := newUUID()
	if err != nil {
		return err
	}
	passwordHash, err := HashPassword(input.Password)
	if err != nil {
		return fmt.Errorf("hash registration password: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin registration: %w", err)
	}
	defer tx.Rollback(ctx)
	var organizationID *string
	if _, err = tx.Exec(ctx, `INSERT INTO auth.app_user(id, organization_id, email, username, display_name, password_hash, status, email_verified_at) VALUES($1,NULL,$2,NULLIF($3,''),$4,$5,'DISABLED',NULL)`, userID, email, username, displayName, passwordHash); err != nil {
		if isUniqueViolation(err) {
			return ErrRegistrationConflict
		}
		return fmt.Errorf("create registration user: %w", err)
	}
	expiresAt := time.Now().UTC().Add(s.resetTTL)
	if _, err = tx.Exec(ctx, `INSERT INTO auth.email_verification_token(id, organization_id, user_id, token_hash, expires_at, requested_ip) VALUES($1,$2,$3,$4,$5,$6)`, tokenID, organizationID, userID, tokenHash, expiresAt, sourceIP); err != nil {
		return fmt.Errorf("create email verification token: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit registration: %w", err)
	}
	if err = s.verificationNotifier(ctx, email, token); err != nil {
		return fmt.Errorf("notify email verification: %w", err)
	}
	return nil
}

func validRegistrationInput(email, username, displayName, password string) bool {
	return validEmail(email) && len(username) <= 100 && len(displayName) > 0 && len(displayName) <= 200 && ValidateNewPassword(password)
}

func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	tokenHash, err := hashPresentedToken(token)
	if err != nil {
		return ErrEmailVerificationToken
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin email verification: %w", err)
	}
	defer tx.Rollback(ctx)
	var tokenID, userID pgtype.UUID
	if err = tx.QueryRow(ctx, `SELECT id, user_id FROM auth.email_verification_token WHERE token_hash=$1 AND used_at IS NULL AND expires_at > now() FOR UPDATE`, tokenHash).Scan(&tokenID, &userID); errors.Is(err, pgx.ErrNoRows) {
		return ErrEmailVerificationToken
	} else if err != nil {
		return fmt.Errorf("load email verification token: %w", err)
	}
	if _, err = tx.Exec(ctx, "UPDATE auth.email_verification_token SET used_at=now() WHERE id=$1", tokenID); err != nil {
		return fmt.Errorf("consume email verification token: %w", err)
	}
	if _, err = tx.Exec(ctx, "UPDATE auth.app_user SET email_verified_at=now(), updated_at=now() WHERE id=$1", userID); err != nil {
		return fmt.Errorf("verify user email: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit email verification: %w", err)
	}
	return nil
}

func (s *Service) ResendVerification(ctx context.Context, email string) error {
	if s.verificationNotifier == nil {
		return ErrRegistrationUnavailable
	}
	email = strings.ToLower(strings.TrimSpace(email))
	var userID pgtype.UUID
	var verified *time.Time
	if err := s.pool.QueryRow(ctx, "SELECT id, email_verified_at FROM auth.app_user WHERE email=$1", email).Scan(&userID, &verified); errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return fmt.Errorf("load verification user: %w", err)
	}
	if verified != nil {
		return nil
	}
	token, tokenHash, err := newToken()
	if err != nil {
		return err
	}
	tokenID, err := newUUID()
	if err != nil {
		return err
	}
	var organizationID pgtype.UUID
	if err = s.pool.QueryRow(ctx, "SELECT organization_id FROM auth.app_user WHERE id=$1", userID).Scan(&organizationID); err != nil {
		return fmt.Errorf("load verification organization: %w", err)
	}
	if _, err = s.pool.Exec(ctx, "UPDATE auth.email_verification_token SET used_at=COALESCE(used_at,now()) WHERE user_id=$1 AND used_at IS NULL", userID); err != nil {
		return fmt.Errorf("invalidate verification tokens: %w", err)
	}
	if _, err = s.pool.Exec(ctx, `INSERT INTO auth.email_verification_token(id, organization_id, user_id, token_hash, expires_at) VALUES($1,$2,$3,$4,$5)`, tokenID, organizationID, userID, tokenHash, time.Now().UTC().Add(s.resetTTL)); err != nil {
		return fmt.Errorf("create verification token: %w", err)
	}
	return s.verificationNotifier(ctx, email, token)
}

func validEmail(value string) bool {
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value && strings.Contains(value, "@") && len(value) <= 320
}

func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "duplicate key value violates unique constraint")
}
