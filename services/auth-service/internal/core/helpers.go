package core

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/auth-service/internal/auth"
	"ygate/auth-service/internal/database/dbgen"
)

// ErrForbidden mirrors platform-api's core.ErrForbidden -- a generic
// permission-denied sentinel, not specific to users/roles/api-keys.
var ErrForbidden = errors.New("permission denied")

// errInvalidUUID is parseUUID's internal error; every caller in this package
// replaces it with its own domain-specific error (ErrUserInvalid,
// ErrRoleInvalid, ...), so its identity is never observed by callers.
var errInvalidUUID = errors.New("invalid uuid")

func parseUUID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(strings.TrimSpace(value)); err != nil || !id.Valid {
		return id, errInvalidUUID
	}
	return id, nil
}

func newUUID() (pgtype.UUID, error) {
	var value pgtype.UUID
	if _, err := rand.Read(value.Bytes[:]); err != nil {
		return value, fmt.Errorf("generate UUID: %w", err)
	}
	value.Bytes[6] = value.Bytes[6]&0x0f | 0x40
	value.Bytes[8] = value.Bytes[8]&0x3f | 0x80
	value.Valid = true
	return value, nil
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	b := value.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func timePointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	copy := value.Time
	return &copy
}

func orgPointer(id pgtype.UUID) *string {
	if !id.Valid {
		return nil
	}
	value := uuidString(id)
	return &value
}

// rowQuerier/hasGlobalPermissionQuery/requireOrganizationPermission are
// duplicated from platform-api's core package (plants.go/devices.go/users.go)
// rather than shared, since they're used by files staying in platform-api
// too (scada.go, audit.go, hard_delete.go, middleware_config.go,
// middleware_plants.go, devices.go) -- platform-api keeps its own copy in
// internal/core/permission.go.

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func hasGlobalPermissionQuery(ctx context.Context, querier rowQuerier, principal auth.Principal, action, resource string) (bool, error) {
	var allowed bool
	err := querier.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1 FROM auth.user_role ur
    JOIN auth.role r ON r.id = ur.role_id
    JOIN auth.role_permission rp ON rp.role_id = ur.role_id
    JOIN auth.permission pm ON pm.id = rp.permission_id
    WHERE ur.user_id = $1
      AND ur.organization_id IS NULL
      AND pm.action = $2
      AND pm.resource_type = $3
      AND r.organization_id IS NULL
      AND rp.organization_id IS NULL
)`, principal.UserID, action, resource).Scan(&allowed)
	return allowed, err
}

// validHardDeleteConfirmation is duplicated from platform-api's
// hard_delete.go (which stays there for plant/device/device-model/api-key
// hard deletes); users.go's HardDeleteUser needs the same check here.
func validHardDeleteConfirmation(actual, legacy string) bool {
	return actual == "DELETE" || actual == legacy
}

func (s *Service) requireOrganizationPermission(ctx context.Context, q *dbgen.Queries, principal auth.Principal, action, resource string, organizationID pgtype.UUID) error {
	allowed, err := q.HasOrganizationPermission(ctx, dbgen.HasOrganizationPermissionParams{UserID: principal.UserID, Action: action, ResourceType: resource, OrganizationID: organizationID})
	if err != nil {
		return fmt.Errorf("check %s %s permission: %w", resource, action, err)
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}
