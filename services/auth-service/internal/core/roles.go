package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/auth-service/internal/auth"
	"ygate/auth-service/internal/database/dbgen"
)

var (
	ErrRoleNotFound = errors.New("role not found")
	ErrRoleInvalid  = errors.New("invalid role data")
	ErrRoleConflict = errors.New("role already exists")
	ErrRoleInUse    = errors.New("role is assigned to users")
)

// systemAdminRoleName is the one built-in, unmodifiable role (is_system=true,
// organization_id NULL, every permission) — see
// platform-api/internal/database/migrations/000027_single_system_role.sql.
// Every other former "system" role became an ordinary editable/deletable
// custom role in that migration.
const systemAdminRoleName = "System Admin"

// Permission is a catalog entry from the permission table; the catalog itself
// is fixed by migrations, only role_permission links are editable.
type Permission struct {
	ID           string `json:"id"`
	Action       string `json:"action"`
	ResourceType string `json:"resourceType"`
	Description  string `json:"description"`
}

// RoleDetail extends the baseline Role (defined in users.go) with the fields
// needed to edit a role: its org scope and the permissions it grants.
type RoleDetail struct {
	Role
	PermissionIDs []string `json:"permissionIds"`
}

type CreateRoleInput struct {
	Global         bool
	OrganizationID string
	Name           string
	Description    string
	PermissionIDs  []string
}

type UpdateRoleInput struct {
	Name          string
	Description   string
	PermissionIDs []string
}

type rowsQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func (s *Service) Permissions(ctx context.Context, principal auth.Principal) ([]Permission, error) {
	allowed, err := s.queries.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "read", ResourceType: "role"})
	if err != nil {
		return nil, fmt.Errorf("check permission catalog read permission: %w", err)
	}
	if !allowed {
		return nil, ErrForbidden
	}
	rows, err := s.pool.Query(ctx, `SELECT id, action, resource_type, description FROM auth.permission ORDER BY resource_type, action`)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	defer rows.Close()
	permissions := []Permission{}
	for rows.Next() {
		var permission Permission
		var id pgtype.UUID
		if err = rows.Scan(&id, &permission.Action, &permission.ResourceType, &permission.Description); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		permission.ID = uuidString(id)
		permissions = append(permissions, permission)
	}
	return permissions, rows.Err()
}

func (s *Service) RoleDetail(ctx context.Context, principal auth.Principal, roleID string) (RoleDetail, error) {
	allowed, err := s.queries.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "read", ResourceType: "role"})
	if err != nil {
		return RoleDetail{}, fmt.Errorf("check role read permission: %w", err)
	}
	if !allowed {
		return RoleDetail{}, ErrForbidden
	}
	id, err := parseUUID(roleID)
	if err != nil {
		return RoleDetail{}, ErrRoleNotFound
	}
	var organizationID pgtype.UUID
	var detail RoleDetail
	err = s.pool.QueryRow(ctx, `SELECT organization_id, name, description, is_system FROM auth.role WHERE id=$1`, id).
		Scan(&organizationID, &detail.Name, &detail.Description, &detail.IsSystem)
	if errors.Is(err, pgx.ErrNoRows) {
		return RoleDetail{}, ErrRoleNotFound
	}
	if err != nil {
		return RoleDetail{}, fmt.Errorf("load role: %w", err)
	}
	detail.ID = roleID
	detail.Role.OrganizationID = orgPointer(organizationID)
	if organizationID.Valid {
		orgAllowed, err := s.queries.HasOrganizationPermission(ctx, dbgen.HasOrganizationPermissionParams{UserID: principal.UserID, Action: "read", ResourceType: "role", OrganizationID: organizationID})
		if err != nil {
			return RoleDetail{}, fmt.Errorf("check org role read permission: %w", err)
		}
		if !orgAllowed {
			return RoleDetail{}, ErrForbidden
		}
	} else if detail.Name == systemAdminRoleName {
		global, err := s.hasGlobalPermission(ctx, principal, "read", "role")
		if err != nil {
			return RoleDetail{}, err
		}
		if !global {
			return RoleDetail{}, ErrForbidden
		}
	}
	permissionIDs, err := rolePermissionIDs(ctx, s.pool, id)
	if err != nil {
		return RoleDetail{}, err
	}
	detail.PermissionIDs = permissionIDs
	return detail, nil
}

func (s *Service) CreateRole(ctx context.Context, principal auth.Principal, input CreateRoleInput, sourceIP *netip.Addr) (RoleDetail, error) {
	name, description, permissionIDs, err := validateRoleInput(input.Name, input.Description, input.PermissionIDs)
	if err != nil {
		return RoleDetail{}, err
	}
	var organizationID pgtype.UUID
	if !input.Global {
		organizationID = principal.OrganizationID
		if strings.TrimSpace(input.OrganizationID) != "" {
			organizationID, err = parseUUID(input.OrganizationID)
			if err != nil {
				return RoleDetail{}, ErrRoleInvalid
			}
		}
		if !organizationID.Valid {
			return RoleDetail{}, ErrRoleInvalid
		}
	}
	id, err := newUUID()
	if err != nil {
		return RoleDetail{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return RoleDetail{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RoleDetail{}, fmt.Errorf("begin create role: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	if err = requireRoleWritePermission(ctx, tx, q, principal, "create", organizationID); err != nil {
		return RoleDetail{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO auth.role(id, organization_id, name, description, is_system) VALUES($1,$2,$3,$4,false)`, id, orgOrNil(organizationID), name, description); err != nil {
		return RoleDetail{}, mapRoleWriteError(err)
	}
	if err = replaceRolePermissions(ctx, tx, principal, organizationID, id, permissionIDs); err != nil {
		return RoleDetail{}, err
	}
	detail := RoleDetail{
		Role:          Role{ID: uuidString(id), OrganizationID: orgPointer(organizationID), Name: name, Description: description, IsSystem: false},
		PermissionIDs: stringIDs(permissionIDs),
	}
	after, _ := json.Marshal(detail)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "role.created",
		TargetType: "role", TargetID: id, AfterData: after, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return RoleDetail{}, fmt.Errorf("audit role create: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return RoleDetail{}, fmt.Errorf("commit create role: %w", err)
	}
	return detail, nil
}

func (s *Service) UpdateRole(ctx context.Context, principal auth.Principal, roleID string, input UpdateRoleInput, sourceIP *netip.Addr) (RoleDetail, error) {
	id, err := parseUUID(roleID)
	if err != nil {
		return RoleDetail{}, ErrRoleNotFound
	}
	name, description, permissionIDs, err := validateRoleInput(input.Name, input.Description, input.PermissionIDs)
	if err != nil {
		return RoleDetail{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return RoleDetail{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RoleDetail{}, fmt.Errorf("begin update role: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	var organizationID pgtype.UUID
	var currentName, currentDescription string
	var isSystem bool
	err = tx.QueryRow(ctx, `SELECT organization_id, name, description, is_system FROM auth.role WHERE id=$1 FOR UPDATE`, id).
		Scan(&organizationID, &currentName, &currentDescription, &isSystem)
	if errors.Is(err, pgx.ErrNoRows) {
		return RoleDetail{}, ErrRoleNotFound
	}
	if err != nil {
		return RoleDetail{}, fmt.Errorf("lock role: %w", err)
	}
	if isSystem {
		return RoleDetail{}, ErrForbidden
	}
	if err = requireRoleWritePermission(ctx, tx, q, principal, "update", organizationID); err != nil {
		return RoleDetail{}, err
	}
	beforePermissionIDs, err := rolePermissionIDs(ctx, tx, id)
	if err != nil {
		return RoleDetail{}, err
	}
	before := RoleDetail{
		Role:          Role{ID: roleID, OrganizationID: orgPointer(organizationID), Name: currentName, Description: currentDescription, IsSystem: false},
		PermissionIDs: beforePermissionIDs,
	}
	if _, err = tx.Exec(ctx, `UPDATE auth.role SET name=$2, description=$3, updated_at=now() WHERE id=$1`, id, name, description); err != nil {
		return RoleDetail{}, mapRoleWriteError(err)
	}
	if err = replaceRolePermissions(ctx, tx, principal, organizationID, id, permissionIDs); err != nil {
		return RoleDetail{}, err
	}
	after := RoleDetail{
		Role:          Role{ID: roleID, OrganizationID: orgPointer(organizationID), Name: name, Description: description, IsSystem: false},
		PermissionIDs: stringIDs(permissionIDs),
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "role.updated",
		TargetType: "role", TargetID: id, BeforeData: beforeJSON, AfterData: afterJSON,
		SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return RoleDetail{}, fmt.Errorf("audit role update: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return RoleDetail{}, fmt.Errorf("commit update role: %w", err)
	}
	return after, nil
}

func (s *Service) DeleteRole(ctx context.Context, principal auth.Principal, roleID string, sourceIP *netip.Addr) error {
	id, err := parseUUID(roleID)
	if err != nil {
		return ErrRoleNotFound
	}
	correlationID, err := newUUID()
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete role: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	var organizationID pgtype.UUID
	var name, description string
	var isSystem bool
	err = tx.QueryRow(ctx, `SELECT organization_id, name, description, is_system FROM auth.role WHERE id=$1 FOR UPDATE`, id).
		Scan(&organizationID, &name, &description, &isSystem)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRoleNotFound
	}
	if err != nil {
		return fmt.Errorf("lock role: %w", err)
	}
	if isSystem {
		return ErrForbidden
	}
	if err = requireRoleWritePermission(ctx, tx, q, principal, "delete", organizationID); err != nil {
		return err
	}
	var inUse bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auth.user_role WHERE role_id=$1)`, id).Scan(&inUse); err != nil {
		return fmt.Errorf("check role usage: %w", err)
	}
	if inUse {
		return ErrRoleInUse
	}
	permissionIDs, err := rolePermissionIDs(ctx, tx, id)
	if err != nil {
		return err
	}
	before := RoleDetail{
		Role:          Role{ID: roleID, OrganizationID: orgPointer(organizationID), Name: name, Description: description, IsSystem: false},
		PermissionIDs: permissionIDs,
	}
	beforeJSON, _ := json.Marshal(before)
	if _, err = tx.Exec(ctx, `DELETE FROM auth.role WHERE id=$1`, id); err != nil {
		return mapRoleWriteError(err)
	}
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "role.deleted",
		TargetType: "role", TargetID: id, BeforeData: beforeJSON, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return fmt.Errorf("audit role delete: %w", err)
	}
	return tx.Commit(ctx)
}

// requireRoleWritePermission mirrors the org-scoped vs. global permission
// split already used for plants (org-scoped) and hard-delete (global-only):
// a role with organization_id set requires the org-scoped grant, a global
// role (organization_id NULL) requires the platform-wide grant.
func requireRoleWritePermission(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, principal auth.Principal, action string, organizationID pgtype.UUID) error {
	if organizationID.Valid {
		allowed, err := q.HasOrganizationPermission(ctx, dbgen.HasOrganizationPermissionParams{UserID: principal.UserID, Action: action, ResourceType: "role", OrganizationID: organizationID})
		if err != nil {
			return fmt.Errorf("check role %s permission: %w", action, err)
		}
		if !allowed {
			return ErrForbidden
		}
		return nil
	}
	allowed, err := hasGlobalPermissionQuery(ctx, tx, principal, action, "role")
	if err != nil {
		return fmt.Errorf("check global role %s permission: %w", action, err)
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func rolePermissionIDs(ctx context.Context, querier rowsQuerier, roleID pgtype.UUID) ([]string, error) {
	rows, err := querier.Query(ctx, `SELECT permission_id FROM auth.role_permission WHERE role_id=$1 ORDER BY permission_id`, roleID)
	if err != nil {
		return nil, fmt.Errorf("list role permissions: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id pgtype.UUID
		if err = rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan role permission: %w", err)
		}
		ids = append(ids, uuidString(id))
	}
	return ids, rows.Err()
}

/*
requireGrantablePermissions blocks privilege escalation through role authoring:
without it an organization admin could mint a role carrying rights they do not
hold, assign it to themselves, and step outside their own scope.

The comparison is on resource_type + action rather than permission id, so it
matches exactly what `can()` checks in the web app -- the picker hides what this
rejects, and the two cannot drift apart.

Permissions the role already carries are exempt. An org admin editing a role
that was seeded with rights beyond theirs can still rename it or adjust the
parts they do control, instead of being locked out of the whole record by a
permission they cannot even see.
*/
func requireGrantablePermissions(ctx context.Context, tx pgx.Tx, principal auth.Principal, roleID pgtype.UUID, permissionIDs []pgtype.UUID) error {
	if len(permissionIDs) == 0 {
		return nil
	}
	var ungrantable int
	err := tx.QueryRow(ctx, `
SELECT count(*)
FROM auth.permission wanted
WHERE wanted.id = ANY($2::uuid[])
  AND NOT EXISTS (
      SELECT 1 FROM auth.role_permission existing
      WHERE existing.role_id = $3 AND existing.permission_id = wanted.id
  )
  AND NOT EXISTS (
      SELECT 1
      FROM auth.user_role ur
      JOIN auth.role_permission rp ON rp.role_id = ur.role_id
      JOIN auth.permission mine ON mine.id = rp.permission_id
      WHERE ur.user_id = $1
        AND mine.resource_type = wanted.resource_type
        AND mine.action = wanted.action
  )`, principal.UserID, permissionIDs, roleID).Scan(&ungrantable)
	if err != nil {
		return fmt.Errorf("check grantable permissions: %w", err)
	}
	if ungrantable > 0 {
		return ErrForbidden
	}
	return nil
}

// replaceRolePermissions mirrors UpdateUser's delete-then-insert idiom for
// user_role: the permission set of a role is always replaced wholesale.
func replaceRolePermissions(ctx context.Context, tx pgx.Tx, principal auth.Principal, organizationID, roleID pgtype.UUID, permissionIDs []pgtype.UUID) error {
	if err := requireGrantablePermissions(ctx, tx, principal, roleID, permissionIDs); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM auth.role_permission WHERE role_id=$1`, roleID); err != nil {
		return fmt.Errorf("clear role permissions: %w", err)
	}
	orgParam := orgOrNil(organizationID)
	for _, permissionID := range permissionIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO auth.role_permission(organization_id, role_id, permission_id) VALUES($1,$2,$3)`, orgParam, roleID, permissionID); err != nil {
			return mapRoleWriteError(err)
		}
	}
	return nil
}

func validateRoleInput(name, description string, permissionIDs []string) (string, string, []pgtype.UUID, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if len(name) == 0 || len(name) > 100 || len(description) > 500 {
		return name, description, nil, ErrRoleInvalid
	}
	seen := make(map[string]struct{}, len(permissionIDs))
	parsed := make([]pgtype.UUID, 0, len(permissionIDs))
	for _, raw := range permissionIDs {
		raw = strings.TrimSpace(raw)
		if _, duplicate := seen[raw]; duplicate {
			continue
		}
		seen[raw] = struct{}{}
		id, err := parseUUID(raw)
		if err != nil {
			return name, description, nil, ErrRoleInvalid
		}
		parsed = append(parsed, id)
	}
	return name, description, parsed, nil
}

func orgOrNil(id pgtype.UUID) any {
	if !id.Valid {
		return nil
	}
	return id
}

func stringIDs(ids []pgtype.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = uuidString(id)
	}
	return out
}

func mapRoleWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrRoleConflict
		case "23503", "23514", "22023":
			return ErrRoleInvalid
		}
	}
	return fmt.Errorf("write role: %w", err)
}
