// Package sessioncheck is platform-api's minimal mirror of auth-service's
// internal/auth session-validation logic. platform-api can no longer import
// auth-service's internal/auth package (they are separate Go modules, and
// Go's internal/ visibility rule forbids the cross-module import), so this
// package duplicates just the "validate this session cookie" path
// platform-api's own authenticated() middleware needs -- see
// docs/superpowers/plans/2026-08-01-backend-microservices-phase1-auth-service.md
// for the full rationale. It also replicates the idle-timeout touch/extend
// write auth.Service.Authenticate used to perform (see auth-service's
// internal/auth/service.go's TouchSession call): without it, sessions would
// hit their idle timeout even while a user is continuously making
// authenticated requests to platform-api's own routes.
package sessioncheck

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrUnauthenticated matches auth.ErrUnauthenticated's message.
var ErrUnauthenticated = errors.New("authentication required")

// Principal mirrors auth.Principal's fields relevant to session validation
// and CSRF checking.
type Principal struct {
	SessionID      pgtype.UUID
	OrganizationID pgtype.UUID
	UserID         pgtype.UUID
	Email          string
	DisplayName    string
	csrfHash       []byte
}

// ValidCSRF mirrors auth.Principal.ValidCSRF's constant-time comparison.
func (p Principal) ValidCSRF(token string) bool {
	actual, err := hashPresentedToken(token)
	return err == nil && len(p.csrfHash) == len(actual) && subtle.ConstantTimeCompare(p.csrfHash, actual) == 1
}

const activeSessionQuery = `
SELECT s.id AS session_id, s.organization_id, s.user_id, s.csrf_hash,
       u.email, u.display_name
FROM auth.user_session s
JOIN auth.app_user u ON u.id = s.user_id
WHERE s.token_hash = $1
  AND s.revoked_at IS NULL
  AND s.expires_at > now()
  AND s.idle_expires_at > now()
  AND u.status = 'ACTIVE'
LIMIT 1`

// touchSessionQuery matches auth.Service.Authenticate's TouchSession query
// (see auth-service's internal/auth/service.go and the generated
// dbgen.TouchSession it wraps) exactly, including the "at most once per
// minute" guard that caps write amplification on hot sessions.
const touchSessionQuery = `
UPDATE auth.user_session
SET last_seen_at = now(),
    idle_expires_at = LEAST(expires_at, now() + $1::bigint * interval '1 second')
WHERE id = $2
  AND last_seen_at < now() - interval '1 minute'`

// Authenticate looks up the session identified by sessionCookieValue,
// verifying it hasn't expired or been revoked and that its owning user is
// still active, then returns the resulting Principal. It also extends the
// session's idle-expiry the same way auth.Service.Authenticate does, using
// idleTimeout as the extension window.
func Authenticate(ctx context.Context, pool *pgxpool.Pool, sessionCookieValue string, idleTimeout time.Duration) (Principal, error) {
	tokenHash, err := hashPresentedToken(sessionCookieValue)
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}
	var p Principal
	err = pool.QueryRow(ctx, activeSessionQuery, tokenHash).Scan(
		&p.SessionID, &p.OrganizationID, &p.UserID, &p.csrfHash, &p.Email, &p.DisplayName,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("load session: %w", err)
	}
	if _, err = pool.Exec(ctx, touchSessionQuery, int64(idleTimeout/time.Second), p.SessionID); err != nil {
		return Principal{}, fmt.Errorf("touch session: %w", err)
	}
	return p, nil
}

func hashPresentedToken(token string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, fmt.Errorf("invalid token")
	}
	sum := sha256.Sum256(raw)
	return sum[:], nil
}
