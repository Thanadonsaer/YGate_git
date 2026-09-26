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

// hasPermission is the one permission check: the scope is set by which ids
// are valid -- org+plant, org only (plant roles never count), or neither
// (global roles only). The rule itself lives in SQL, auth.has_permission
// (migration 000055), which list queries also call per row.
func hasPermission(ctx context.Context, querier rowQuerier, principal auth.Principal, action, resource string, organizationID, plantID pgtype.UUID) (bool, error) {
	var allowed bool
	err := querier.QueryRow(ctx, `SELECT auth.has_permission($1, $2, $3, $4, $5)`, principal.UserID, action, resource, organizationID, plantID).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("check %s %s permission: %w", resource, action, err)
	}
	return allowed, nil
}

// authorize is hasPermission that fails with ErrForbidden.
func authorize(ctx context.Context, querier rowQuerier, principal auth.Principal, action, resource string, organizationID, plantID pgtype.UUID) error {
	allowed, err := hasPermission(ctx, querier, principal, action, resource, organizationID, plantID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func hasGlobalPermissionQuery(ctx context.Context, querier rowQuerier, principal auth.Principal, action, resource string) (bool, error) {
	return hasPermission(ctx, querier, principal, action, resource, pgtype.UUID{}, pgtype.UUID{})
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
