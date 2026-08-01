package auth

import "github.com/jackc/pgx/v5/pgtype"

// Principal identifies the authenticated caller of a platform-api request.
// It is populated by internal/httpapi's authenticated() middleware from the
// session lookup in internal/sessioncheck (login/session issuance itself
// moved to auth-service -- see
// docs/superpowers/plans/2026-08-01-backend-microservices-phase1-auth-service.md).
type Principal struct {
	SessionID      pgtype.UUID
	OrganizationID pgtype.UUID
	UserID         pgtype.UUID
	Email          string
	DisplayName    string
}

// LoginUser is the user-facing shape of a Principal, as returned by
// User().
type LoginUser struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId,omitempty"`
	Email          string `json:"email"`
	DisplayName    string `json:"displayName"`
}

func (p Principal) User() LoginUser {
	return LoginUser{ID: uuidString(p.UserID), OrganizationID: uuidString(p.OrganizationID), Email: p.Email, DisplayName: p.DisplayName}
}
