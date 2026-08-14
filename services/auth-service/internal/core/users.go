package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/auth-service/internal/auth"
	"ygate/auth-service/internal/database/dbgen"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrUserInvalid  = errors.New("invalid user data")
	ErrUserConflict = errors.New("user already exists")
)

type ManagedUser struct {
	ID               string     `json:"id"`
	OrganizationID   string     `json:"organizationId"`
	OrganizationName string     `json:"organizationName"`
	Email            string     `json:"email"`
	Username         string     `json:"username,omitempty"`
	DisplayName      string     `json:"displayName"`
	Status           string     `json:"status"`
	IsActive         bool       `json:"isActive"`
	FailedLoginCount int32      `json:"failedLoginCount"`
	EmailVerifiedAt  *time.Time `json:"emailVerifiedAt,omitempty"`
	LockedUntil      *time.Time `json:"lockedUntil"`
	Roles            []string   `json:"roles"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type Role struct {
	ID             string  `json:"id"`
	OrganizationID *string `json:"organizationId"`
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	IsSystem       bool    `json:"isSystem"`
}

type CreateUserInput struct {
	OrganizationID string
	Email          string
	Username       string
	DisplayName    string
	Password       string
	RoleID         string
}

type UpdateUserInput struct {
	OrganizationID string
	Email          string
	Username       string
	DisplayName    string
	RoleID         string
	IsActive       bool
}

func (s *Service) Users(ctx context.Context, principal auth.Principal) ([]ManagedUser, error) {
	allowed, err := s.queries.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "read", ResourceType: "user"})
	if err != nil {
		return nil, fmt.Errorf("check user read permission: %w", err)
	}
	if !allowed {
		return nil, ErrForbidden
	}
	rows, err := s.pool.Query(ctx, `
SELECT u.id, u.organization_id, COALESCE(o.name,''), u.email, COALESCE(u.username,''), u.display_name,
	       u.status, u.failed_login_count, u.email_verified_at, u.locked_until, u.created_at, u.updated_at,
       COALESCE(array_agg(r.name ORDER BY r.name) FILTER (WHERE r.id IS NOT NULL), '{}')::text[] AS roles
FROM auth.app_user u
LEFT JOIN organization o ON o.id = u.organization_id
LEFT JOIN auth.user_role ur_user ON ur_user.user_id = u.id
LEFT JOIN auth.role r ON r.id = ur_user.role_id
WHERE EXISTS (
    SELECT 1 FROM auth.user_role ur
    JOIN auth.role rr ON rr.id = ur.role_id
    JOIN auth.role_permission rp ON rp.role_id = ur.role_id
    JOIN auth.permission pm ON pm.id = rp.permission_id
    WHERE ur.user_id = $1
      AND pm.action = 'read' AND pm.resource_type = 'user'
      AND (rr.organization_id IS NULL OR rr.organization_id = ur.organization_id)
      AND (rp.organization_id IS NULL OR rp.organization_id = ur.organization_id)
      AND ur.plant_id IS NULL
      AND (ur.organization_id IS NULL OR ur.organization_id = u.organization_id)
)
GROUP BY u.id, o.name
ORDER BY o.name, u.email
LIMIT 200`, principal.UserID)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	users := []ManagedUser{}
	for rows.Next() {
		var user ManagedUser
		var id, organizationID pgtype.UUID
		var verifiedAt, lockedUntil, createdAt, updatedAt pgtype.Timestamptz
		if err = rows.Scan(&id, &organizationID, &user.OrganizationName, &user.Email, &user.Username, &user.DisplayName, &user.Status, &user.FailedLoginCount, &verifiedAt, &lockedUntil, &createdAt, &updatedAt, &user.Roles); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		user.ID = uuidString(id)
		user.OrganizationID = uuidString(organizationID)
		user.IsActive = user.Status == "ACTIVE"
		user.EmailVerifiedAt = timePointer(verifiedAt)
		user.LockedUntil = timePointer(lockedUntil)
		user.CreatedAt = createdAt.Time
		user.UpdatedAt = updatedAt.Time
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Service) Roles(ctx context.Context, principal auth.Principal) ([]Role, error) {
	allowed, err := s.queries.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "read", ResourceType: "role"})
	if err != nil {
		return nil, fmt.Errorf("check role read permission: %w", err)
	}
	if !allowed {
		return nil, ErrForbidden
	}
	global, err := s.hasGlobalPermission(ctx, principal, "read", "role")
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, organization_id, name, description, is_system
FROM auth.role
WHERE (organization_id IS NULL OR $1 OR organization_id = $2)
  AND ($1::boolean OR name <> $3)
ORDER BY organization_id IS NOT NULL, name <> $3 DESC, name`, global, principal.OrganizationID, systemAdminRoleName)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()
	roles := []Role{}
	for rows.Next() {
		var role Role
		var id, organizationID pgtype.UUID
		if err = rows.Scan(&id, &organizationID, &role.Name, &role.Description, &role.IsSystem); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		role.ID = uuidString(id)
		role.OrganizationID = orgPointer(organizationID)
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (s *Service) CreateUser(ctx context.Context, principal auth.Principal, input CreateUserInput, sourceIP *netip.Addr) (ManagedUser, error) {
	organizationID := principal.OrganizationID
	var err error
	if strings.TrimSpace(input.OrganizationID) != "" {
		organizationID, err = parseUUID(input.OrganizationID)
		if err != nil {
			return ManagedUser{}, ErrUserInvalid
		}
	}
	input.Email, input.Username, input.DisplayName, err = validateUserInput(input.Email, input.Username, input.DisplayName, input.Password)
	if err != nil || !organizationID.Valid {
		return ManagedUser{}, ErrUserInvalid
	}
	roleID, err := parseUUID(input.RoleID)
	if err != nil {
		return ManagedUser{}, ErrUserInvalid
	}
	passwordHash, err := auth.HashPassword(input.Password)
	if err != nil {
		return ManagedUser{}, fmt.Errorf("hash user password: %w", err)
	}
	userID, err := newUUID()
	if err != nil {
		return ManagedUser{}, err
	}
	assignmentID, err := newUUID()
	if err != nil {
		return ManagedUser{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return ManagedUser{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ManagedUser{}, fmt.Errorf("begin create user: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	if err = s.requireOrganizationPermission(ctx, q, principal, "create", "user", organizationID); err != nil {
		return ManagedUser{}, err
	}
	if err = s.requireOrganizationPermission(ctx, q, principal, "assign", "role", organizationID); err != nil {
		return ManagedUser{}, err
	}
	roleName, err := s.authorizedRoleName(ctx, tx, principal, organizationID, roleID)
	if err != nil {
		return ManagedUser{}, err
	}
	var username any
	if input.Username != "" {
		username = input.Username
	}
	if _, err = tx.Exec(ctx, `INSERT INTO auth.app_user(id, organization_id, email, username, display_name, password_hash, email_verified_at) VALUES($1,$2,$3,$4,$5,$6,now())`, userID, organizationID, input.Email, username, input.DisplayName, passwordHash); err != nil {
		return ManagedUser{}, mapUserWriteError(err)
	}
	var assignmentOrganization any = organizationID
	if roleName == systemAdminRoleName {
		assignmentOrganization = nil
	}
	if _, err = tx.Exec(ctx, `INSERT INTO auth.user_role(id, organization_id, user_id, role_id) VALUES($1,$2,$3,$4)`, assignmentID, assignmentOrganization, userID, roleID); err != nil {
		return ManagedUser{}, mapUserWriteError(err)
	}
	user, err := s.getUserInTx(ctx, tx, userID)
	if err != nil {
		return ManagedUser{}, err
	}
	after, _ := json.Marshal(user)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "user.created", TargetType: "app_user", TargetID: userID, AfterData: after, SourceIp: sourceIP, CorrelationID: correlationID}); err != nil {
		return ManagedUser{}, fmt.Errorf("audit user create: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return ManagedUser{}, fmt.Errorf("commit create user: %w", err)
	}
	return user, nil
}

func (s *Service) UpdateUser(ctx context.Context, principal auth.Principal, userID string, input UpdateUserInput, sourceIP *netip.Addr) (ManagedUser, error) {
	id, err := parseUUID(userID)
	if err != nil {
		return ManagedUser{}, ErrUserNotFound
	}
	if uuidString(id) == uuidString(principal.UserID) {
		return ManagedUser{}, ErrUserInvalid
	}
	input.Email, input.Username, input.DisplayName, err = validateUserProfile(input.Email, input.Username, input.DisplayName)
	if err != nil {
		return ManagedUser{}, err
	}
	roleID, err := parseUUID(input.RoleID)
	if err != nil {
		return ManagedUser{}, ErrUserInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ManagedUser{}, fmt.Errorf("begin update user: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	before, err := s.getUserInTx(ctx, tx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ManagedUser{}, ErrUserNotFound
	}
	if err != nil {
		return ManagedUser{}, err
	}
	if input.IsActive {
		var emailVerified bool
		if err = tx.QueryRow(ctx, `SELECT email_verified_at IS NOT NULL FROM auth.app_user WHERE id=$1`, id).Scan(&emailVerified); err != nil {
			return ManagedUser{}, fmt.Errorf("check user email verification: %w", err)
		}
		if !emailVerified {
			return ManagedUser{}, ErrUserInvalid
		}
	}
	organizationID, err := parseUUID(input.OrganizationID)
	if err != nil {
		return ManagedUser{}, ErrUserInvalid
	}
	beforeOrganizationID, beforeOrganizationErr := parseUUID(before.OrganizationID)
	if beforeOrganizationErr == nil && uuidString(beforeOrganizationID) != uuidString(organizationID) {
		global, globalErr := s.hasGlobalPermission(ctx, principal, "update", "user")
		if globalErr != nil || !global {
			return ManagedUser{}, ErrForbidden
		}
	} else if beforeOrganizationErr != nil {
		global, globalErr := s.hasGlobalPermission(ctx, principal, "update", "user")
		if globalErr != nil || !global {
			return ManagedUser{}, ErrForbidden
		}
	} else if err = s.requireOrganizationPermission(ctx, q, principal, "update", "user", organizationID); err != nil {
		return ManagedUser{}, err
	}
	if !input.IsActive {
		if err = s.requireOrganizationPermission(ctx, q, principal, "disable", "user", organizationID); err != nil {
			return ManagedUser{}, err
		}
	}
	if err = s.requireOrganizationPermission(ctx, q, principal, "assign", "role", organizationID); err != nil {
		return ManagedUser{}, err
	}
	roleName, err := s.authorizedRoleName(ctx, tx, principal, organizationID, roleID)
	if err != nil {
		return ManagedUser{}, err
	}
	if hasRole(before.Roles, systemAdminRoleName) && roleName != systemAdminRoleName {
		last, err := isLastActivePlatformAdmin(ctx, tx, id)
		if err != nil {
			return ManagedUser{}, err
		}
		if last {
			return ManagedUser{}, ErrUserInvalid
		}
	}
	status := "DISABLED"
	if input.IsActive {
		status = "ACTIVE"
	}
	var username any
	if input.Username != "" {
		username = input.Username
	}
	if _, err = tx.Exec(ctx, `DELETE FROM auth.user_role WHERE user_id=$1 AND plant_id IS NULL`, id); err != nil {
		return ManagedUser{}, fmt.Errorf("replace user baseline role: %w", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE auth.app_user SET organization_id=$2, email=$3, username=$4, display_name=$5, status=$6, updated_at=now() WHERE id=$1`, id, organizationID, input.Email, username, input.DisplayName, status); err != nil {
		return ManagedUser{}, mapUserWriteError(err)
	}
	assignmentID, err := newUUID()
	if err != nil {
		return ManagedUser{}, err
	}
	var assignmentOrganization any = organizationID
	if roleName == systemAdminRoleName {
		assignmentOrganization = nil
	}
	if _, err = tx.Exec(ctx, `INSERT INTO auth.user_role(id, organization_id, user_id, role_id) VALUES($1,$2,$3,$4)`, assignmentID, assignmentOrganization, id, roleID); err != nil {
		return ManagedUser{}, mapUserWriteError(err)
	}
	if !input.IsActive {
		if _, err = tx.Exec(ctx, `UPDATE auth.user_session SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1 AND revoked_at IS NULL`, id); err != nil {
			return ManagedUser{}, fmt.Errorf("revoke updated user sessions: %w", err)
		}
	}
	after, err := s.getUserInTx(ctx, tx, id)
	if err != nil {
		return ManagedUser{}, err
	}
	if err = auditUserChange(ctx, q, principal, organizationID, id, "user.updated", before, after, sourceIP); err != nil {
		return ManagedUser{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ManagedUser{}, fmt.Errorf("commit update user: %w", err)
	}
	return after, nil
}

func (s *Service) ResetUserPassword(ctx context.Context, principal auth.Principal, userID, newPassword string, sourceIP *netip.Addr) error {
	id, err := parseUUID(userID)
	if err != nil {
		return ErrUserNotFound
	}
	if !auth.ValidateNewPassword(newPassword) {
		return ErrUserInvalid
	}
	passwordHash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash reset password: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin admin password reset: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	before, err := s.getUserInTx(ctx, tx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return err
	}
	organizationID, err := parseUUID(before.OrganizationID)
	if err != nil {
		return ErrUserNotFound
	}
	if err = s.requireOrganizationPermission(ctx, q, principal, "reset_password", "user", organizationID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE auth.app_user SET password_hash=$2, password_changed_at=now(), failed_login_count=0, locked_until=NULL, updated_at=now() WHERE id=$1`, id, passwordHash); err != nil {
		return mapUserWriteError(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE auth.user_session SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1 AND revoked_at IS NULL`, id); err != nil {
		return fmt.Errorf("revoke reset user sessions: %w", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE auth.password_reset_token SET used_at=COALESCE(used_at,now()) WHERE user_id=$1 AND used_at IS NULL`, id); err != nil {
		return fmt.Errorf("invalidate user reset tokens: %w", err)
	}
	after, err := s.getUserInTx(ctx, tx, id)
	if err != nil {
		return err
	}
	if err = auditUserChange(ctx, q, principal, organizationID, id, "user.password_admin_reset", before, after, sourceIP); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) HardDeleteUser(ctx context.Context, principal auth.Principal, userID, confirmation string, sourceIP *netip.Addr) error {
	id, err := parseUUID(userID)
	if err != nil {
		return ErrUserNotFound
	}
	if uuidString(id) == uuidString(principal.UserID) {
		return ErrUserInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin hard delete user: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	before, err := s.getUserInTx(ctx, tx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return err
	}
	allowed, err := hasGlobalPermissionQuery(ctx, tx, principal, "hard_delete", "user")
	if err != nil {
		return fmt.Errorf("check global user hard delete permission: %w", err)
	}
	if !allowed {
		return ErrForbidden
	}
	if !validHardDeleteConfirmation(confirmation, "DELETE "+before.Email) {
		return ErrUserInvalid
	}
	if hasRole(before.Roles, systemAdminRoleName) {
		last, err := isLastActivePlatformAdmin(ctx, tx, id)
		if err != nil {
			return err
		}
		if last {
			return ErrUserInvalid
		}
	}
	organizationID, err := optionalOrganizationIDForAudit(before.OrganizationID)
	if err != nil {
		return ErrUserNotFound
	}
	correlationID, err := newUUID()
	if err != nil {
		return err
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(map[string]any{"hardDeleted": true, "dependentRecords": "cascade"})
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "user.hard_deleted", TargetType: "app_user", TargetID: id, BeforeData: beforeJSON, AfterData: afterJSON, SourceIp: sourceIP, CorrelationID: correlationID}); err != nil {
		return fmt.Errorf("audit user hard delete: %w", err)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM auth.app_user WHERE id=$1`, id); err != nil {
		return fmt.Errorf("hard delete user: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *Service) SetUserActive(ctx context.Context, principal auth.Principal, userID string, active bool, sourceIP *netip.Addr) (ManagedUser, error) {
	id, err := parseUUID(userID)
	if err != nil {
		return ManagedUser{}, ErrUserNotFound
	}
	if !active && uuidString(id) == uuidString(principal.UserID) {
		return ManagedUser{}, ErrUserInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ManagedUser{}, fmt.Errorf("begin user status update: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	before, err := s.getUserInTx(ctx, tx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ManagedUser{}, ErrUserNotFound
	}
	if err != nil {
		return ManagedUser{}, err
	}
	if active {
		var emailVerified bool
		if err = tx.QueryRow(ctx, `SELECT email_verified_at IS NOT NULL FROM auth.app_user WHERE id=$1`, id).Scan(&emailVerified); err != nil {
			return ManagedUser{}, fmt.Errorf("check user email verification: %w", err)
		}
		if !emailVerified {
			return ManagedUser{}, ErrUserInvalid
		}
	}
	if active {
		var hasRole bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auth.user_role WHERE user_id=$1 AND plant_id IS NULL)`, id).Scan(&hasRole); err != nil {
			return ManagedUser{}, fmt.Errorf("check user role assignment: %w", err)
		}
		if !hasRole {
			return ManagedUser{}, ErrUserInvalid
		}
	}
	organizationID, err := parseUUID(before.OrganizationID)
	if err != nil {
		return ManagedUser{}, ErrUserNotFound
	}
	action := "disable"
	if active {
		action = "update"
	}
	if err = s.requireOrganizationPermission(ctx, q, principal, action, "user", organizationID); err != nil {
		return ManagedUser{}, err
	}
	status := "DISABLED"
	if active {
		status = "ACTIVE"
	}
	if _, err = tx.Exec(ctx, `UPDATE auth.app_user SET status=$2, updated_at=now() WHERE id=$1`, id, status); err != nil {
		return ManagedUser{}, mapUserWriteError(err)
	}
	if !active {
		if _, err = tx.Exec(ctx, `UPDATE auth.user_session SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, id); err != nil {
			return ManagedUser{}, fmt.Errorf("revoke disabled user sessions: %w", err)
		}
	}
	after, err := s.getUserInTx(ctx, tx, id)
	if err != nil {
		return ManagedUser{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return ManagedUser{}, err
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "user.status_updated", TargetType: "app_user", TargetID: id, BeforeData: beforeJSON, AfterData: afterJSON, SourceIp: sourceIP, CorrelationID: correlationID}); err != nil {
		return ManagedUser{}, fmt.Errorf("audit user status update: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return ManagedUser{}, fmt.Errorf("commit user status update: %w", err)
	}
	return after, nil
}

func (s *Service) UnlockUser(ctx context.Context, principal auth.Principal, userID string, sourceIP *netip.Addr) (ManagedUser, error) {
	id, err := parseUUID(userID)
	if err != nil {
		return ManagedUser{}, ErrUserNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ManagedUser{}, fmt.Errorf("begin user unlock: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	before, err := s.getUserInTx(ctx, tx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ManagedUser{}, ErrUserNotFound
	}
	if err != nil {
		return ManagedUser{}, err
	}
	organizationID, err := parseUUID(before.OrganizationID)
	if err != nil {
		return ManagedUser{}, ErrUserNotFound
	}
	if err = s.requireOrganizationPermission(ctx, q, principal, "unlock", "user", organizationID); err != nil {
		return ManagedUser{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE auth.app_user SET failed_login_count=0, locked_until=NULL, updated_at=now() WHERE id=$1`, id); err != nil {
		return ManagedUser{}, mapUserWriteError(err)
	}
	after, err := s.getUserInTx(ctx, tx, id)
	if err != nil {
		return ManagedUser{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return ManagedUser{}, err
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "user.unlocked", TargetType: "app_user", TargetID: id, BeforeData: beforeJSON, AfterData: afterJSON, SourceIp: sourceIP, CorrelationID: correlationID}); err != nil {
		return ManagedUser{}, fmt.Errorf("audit user unlock: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return ManagedUser{}, fmt.Errorf("commit user unlock: %w", err)
	}
	return after, nil
}

func validateUserInput(email, username, displayName, password string) (string, string, string, error) {
	email, username, displayName, err := validateUserProfile(email, username, displayName)
	if err != nil || !auth.ValidateNewPassword(password) {
		return email, username, displayName, ErrUserInvalid
	}
	return email, username, displayName, nil
}

func validateUserProfile(email, username, displayName string) (string, string, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 320 {
		return email, username, displayName, ErrUserInvalid
	}
	username = strings.ToLower(strings.TrimSpace(username))
	if strings.ContainsAny(username, " \t\r\n") || len(username) > 100 {
		return email, username, displayName, ErrUserInvalid
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" || len(displayName) > 200 {
		return email, username, displayName, ErrUserInvalid
	}
	return email, username, displayName, nil
}

func (s *Service) hasGlobalPermission(ctx context.Context, principal auth.Principal, action, resource string) (bool, error) {
	return hasGlobalPermissionQuery(ctx, s.pool, principal, action, resource)
}

func (s *Service) authorizedRoleName(ctx context.Context, tx pgx.Tx, principal auth.Principal, organizationID, roleID pgtype.UUID) (string, error) {
	var roleName string
	var roleOrganization pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT name, organization_id FROM auth.role WHERE id=$1 AND (organization_id IS NULL OR organization_id=$2)`, roleID, organizationID).Scan(&roleName, &roleOrganization); errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUserInvalid
	} else if err != nil {
		return "", fmt.Errorf("load user role: %w", err)
	}
	if roleName == systemAdminRoleName {
		global, err := hasGlobalPermissionQuery(ctx, tx, principal, "assign", "role")
		if err != nil {
			return "", err
		}
		if !global {
			return "", ErrForbidden
		}
	}
	return roleName, nil
}

func hasRole(roles []string, name string) bool {
	for _, role := range roles {
		if role == name {
			return true
		}
	}
	return false
}

func isLastActivePlatformAdmin(ctx context.Context, tx pgx.Tx, excludedUserID pgtype.UUID) (bool, error) {
	var remaining int
	err := tx.QueryRow(ctx, `
SELECT count(DISTINCT u.id)
FROM auth.app_user u
JOIN auth.user_role ur ON ur.user_id=u.id AND ur.organization_id IS NULL
JOIN auth.role r ON r.id=ur.role_id AND r.organization_id IS NULL
WHERE u.status='ACTIVE' AND r.name=$2 AND u.id<>$1`, excludedUserID, systemAdminRoleName).Scan(&remaining)
	if err != nil {
		return false, fmt.Errorf("count remaining system admin users: %w", err)
	}
	return remaining == 0, nil
}

func auditUserChange(ctx context.Context, q *dbgen.Queries, principal auth.Principal, organizationID, userID pgtype.UUID, action string, before, after ManagedUser, sourceIP *netip.Addr) error {
	correlationID, err := newUUID()
	if err != nil {
		return err
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{OrganizationID: organizationID, ActorUserID: principal.UserID, Action: action, TargetType: "app_user", TargetID: userID, BeforeData: beforeJSON, AfterData: afterJSON, SourceIp: sourceIP, CorrelationID: correlationID}); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return nil
}

func (s *Service) getUserInTx(ctx context.Context, tx pgx.Tx, id pgtype.UUID) (ManagedUser, error) {
	var user ManagedUser
	var userID, organizationID pgtype.UUID
	var lockedUntil, createdAt, updatedAt pgtype.Timestamptz
	err := tx.QueryRow(ctx, `
SELECT u.id, u.organization_id, COALESCE(o.name,''), u.email, COALESCE(u.username,''), u.display_name,
       u.status, u.failed_login_count, u.locked_until, u.created_at, u.updated_at
FROM auth.app_user u
LEFT JOIN organization o ON o.id = u.organization_id
WHERE u.id=$1
FOR UPDATE OF u`, id).Scan(&userID, &organizationID, &user.OrganizationName, &user.Email, &user.Username, &user.DisplayName, &user.Status, &user.FailedLoginCount, &lockedUntil, &createdAt, &updatedAt)
	if err != nil {
		return ManagedUser{}, err
	}
	roles, err := s.userRolesInTx(ctx, tx, id)
	if err != nil {
		return ManagedUser{}, err
	}
	user.ID = uuidString(userID)
	user.OrganizationID = uuidString(organizationID)
	user.IsActive = user.Status == "ACTIVE"
	user.LockedUntil = timePointer(lockedUntil)
	user.Roles = roles
	user.CreatedAt = createdAt.Time
	user.UpdatedAt = updatedAt.Time
	return user, nil
}

func (s *Service) userRolesInTx(ctx context.Context, tx pgx.Tx, userID pgtype.UUID) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT r.name FROM auth.user_role ur JOIN auth.role r ON r.id = ur.role_id WHERE ur.user_id=$1 ORDER BY r.name`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user roles: %w", err)
	}
	defer rows.Close()
	roles := []string{}
	for rows.Next() {
		var role string
		if err = rows.Scan(&role); err != nil {
			return nil, fmt.Errorf("scan user role: %w", err)
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func mapUserWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrUserConflict
		case "23503", "23514", "22023":
			return ErrUserInvalid
		}
	}
	return fmt.Errorf("write user: %w", err)
}
