// Package sessioncheck is platform-api's minimal, read-only mirror of
// auth-service's internal/auth session-validation logic. platform-api can no
// longer import auth-service's internal/auth package (they are separate Go
// modules, and Go's internal/ visibility rule forbids the cross-module
// import), so this package duplicates just the "validate this session
// cookie" read path platform-api's own authenticated() middleware needs --
// see docs/superpowers/plans/2026-08-01-backend-microservices-phase1-auth-service.md
// for the full rationale. It performs no writes (no TouchSession/idle-expiry
// extension): that's auth-service's job now.
package sessioncheck

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"

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
FROM user_session s
JOIN app_user u ON u.id = s.user_id
WHERE s.token_hash = $1
  AND s.revoked_at IS NULL
  AND s.expires_at > now()
  AND s.idle_expires_at > now()
  AND u.status = 'ACTIVE'
LIMIT 1`

// Authenticate looks up the session identified by sessionCookieValue,
// verifying it hasn't expired or been revoked and that its owning user is
// still active, then returns the resulting Principal. This is the same
// lookup auth.Service.Authenticate performs (see auth-service's
// internal/auth/service.go), minus the idle-timeout TouchSession write,
// which is out of scope for a read-only check.
func Authenticate(ctx context.Context, pool *pgxpool.Pool, sessionCookieValue string) (Principal, error) {
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
