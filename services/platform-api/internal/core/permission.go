package core

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/database/dbgen"
)

// requireOrganizationPermission and hasGlobalPermissionQuery used to live in
// users.go, but they're generic permission checks every domain depends on
// (middleware, devices, scada, audit, hard-delete), not just users/roles --
// they stay here so those callers keep compiling once users.go/roles.go move
// to auth-service. auth-service's own core package carries a duplicate copy
// (in internal/core/helpers.go) for its own moved users.go/roles.go.

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

func hasGlobalPermissionQuery(ctx context.Context, querier rowQuerier, principal auth.Principal, action, resource string) (bool, error) {
	var allowed bool
	err := querier.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1 FROM user_role ur
    JOIN role r ON r.id = ur.role_id
    JOIN role_permission rp ON rp.role_id = ur.role_id
    JOIN permission pm ON pm.id = rp.permission_id
    WHERE ur.user_id = $1
      AND ur.organization_id IS NULL
      AND pm.action = $2
      AND pm.resource_type = $3
      AND r.organization_id IS NULL
      AND rp.organization_id IS NULL
)`, principal.UserID, action, resource).Scan(&allowed)
	return allowed, err
}

func (s *Service) requireGlobalPermission(ctx context.Context, principal auth.Principal, action, resource string) error {
	allowed, err := hasGlobalPermissionQuery(ctx, s.pool, principal, action, resource)
	if err != nil {
		return fmt.Errorf("check global %s %s permission: %w", resource, action, err)
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

// orgPointer used to live in roles.go; alarms.go also needs it to render a
// nullable organization id as *string, so it stays here too.
func orgPointer(id pgtype.UUID) *string {
	if !id.Valid {
		return nil
	}
	value := uuidString(id)
	return &value
}
