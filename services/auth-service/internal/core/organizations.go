package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	ErrOrganizationNotFound = errors.New("organization not found")
	ErrOrganizationInvalid  = errors.New("invalid organization data")
	ErrOrganizationConflict = errors.New("organization code already in use")
)

// Organization is the tenant boundary every other domain (plants, users,
// roles, devices, audit) hangs off of. The table itself lives in the
// public schema (see platform-api's 000033_schema_namespacing.sql -- "no
// single domain owns it"), so this file reads/writes it with unqualified
// table names even though the rest of this package talks to auth.*.
type Organization struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	IsActive  bool      `json:"isActive"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type CreateOrganizationInput struct {
	Code string
	Name string
}

type UpdateOrganizationInput struct {
	Code     string
	Name     string
	IsActive bool
}

// Organizations lists every organization the caller's read grant covers:
// a global organization:read grant (System Admin) sees every organization,
// an org-scoped grant (e.g. Organization Admin) sees only their own -- the
// same "ur.organization_id IS NULL is a wildcard" trick HasOrganizationPermission
// uses, just joined outward against the organization table instead of
// checked against a single target id.
func (s *Service) Organizations(ctx context.Context, principal auth.Principal) ([]Organization, error) {
	rows, err := s.pool.Query(ctx, `
SELECT o.id, o.code, o.name, o.is_active, o.created_at, o.updated_at
FROM organization o
WHERE EXISTS (
    SELECT 1 FROM auth.user_role ur
    JOIN auth.role r ON r.id = ur.role_id
    JOIN auth.role_permission rp ON rp.role_id = ur.role_id
    JOIN auth.permission pm ON pm.id = rp.permission_id
    WHERE ur.user_id = $1
      AND pm.action = 'read'
      AND pm.resource_type = 'organization'
      AND (r.organization_id IS NULL OR r.organization_id = ur.organization_id)
      AND (rp.organization_id IS NULL OR rp.organization_id = ur.organization_id)
      AND ur.plant_id IS NULL
      AND (ur.organization_id IS NULL OR ur.organization_id = o.id)
)
ORDER BY o.name`, principal.UserID)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	defer rows.Close()
	organizations := []Organization{}
	for rows.Next() {
		var id pgtype.UUID
		var createdAt, updatedAt pgtype.Timestamptz
		var organization Organization
		if err = rows.Scan(&id, &organization.Code, &organization.Name, &organization.IsActive, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan organization: %w", err)
		}
		organization.ID = uuidString(id)
		organization.CreatedAt = createdAt.Time
		organization.UpdatedAt = updatedAt.Time
		organizations = append(organizations, organization)
	}
	return organizations, rows.Err()
}

func (s *Service) CreateOrganization(ctx context.Context, principal auth.Principal, input CreateOrganizationInput, sourceIP *netip.Addr) (Organization, error) {
	code, name, err := validateOrganizationInput(input.Code, input.Name)
	if err != nil {
		return Organization{}, err
	}
	id, err := newUUID()
	if err != nil {
		return Organization{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return Organization{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Organization{}, fmt.Errorf("begin create organization: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	allowed, err := hasGlobalPermissionQuery(ctx, tx, principal, "create", "organization")
	if err != nil {
		return Organization{}, fmt.Errorf("check global organization create permission: %w", err)
	}
	if !allowed {
		return Organization{}, ErrForbidden
	}
	var createdAt, updatedAt pgtype.Timestamptz
	err = tx.QueryRow(ctx, `INSERT INTO organization(id, code, name, is_active) VALUES($1,$2,$3,true) RETURNING created_at, updated_at`, id, code, name).
		Scan(&createdAt, &updatedAt)
	if err != nil {
		return Organization{}, mapOrganizationWriteError(err)
	}
	organization := Organization{ID: uuidString(id), Code: code, Name: name, IsActive: true, CreatedAt: createdAt.Time, UpdatedAt: updatedAt.Time}
	after, _ := json.Marshal(organization)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: id, ActorUserID: principal.UserID, Action: "organization.created",
		TargetType: "organization", TargetID: id, AfterData: after, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return Organization{}, fmt.Errorf("audit organization create: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Organization{}, fmt.Errorf("commit create organization: %w", err)
	}
	return organization, nil
}

func (s *Service) UpdateOrganization(ctx context.Context, principal auth.Principal, organizationID string, input UpdateOrganizationInput, sourceIP *netip.Addr) (Organization, error) {
	id, err := parseUUID(organizationID)
	if err != nil {
		return Organization{}, ErrOrganizationNotFound
	}
	code, name, err := validateOrganizationInput(input.Code, input.Name)
	if err != nil {
		return Organization{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return Organization{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Organization{}, fmt.Errorf("begin update organization: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	var before Organization
	var beforeCreatedAt, beforeUpdatedAt pgtype.Timestamptz
	err = tx.QueryRow(ctx, `SELECT code, name, is_active, created_at, updated_at FROM organization WHERE id=$1 FOR UPDATE`, id).
		Scan(&before.Code, &before.Name, &before.IsActive, &beforeCreatedAt, &beforeUpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, ErrOrganizationNotFound
	}
	if err != nil {
		return Organization{}, fmt.Errorf("lock organization: %w", err)
	}
	before.ID = organizationID
	before.CreatedAt = beforeCreatedAt.Time
	before.UpdatedAt = beforeUpdatedAt.Time
	if err = s.requireOrganizationPermission(ctx, q, principal, "update", "organization", id); err != nil {
		return Organization{}, err
	}
	var updatedAt pgtype.Timestamptz
	err = tx.QueryRow(ctx, `UPDATE organization SET code=$2, name=$3, is_active=$4, updated_at=now() WHERE id=$1 RETURNING updated_at`, id, code, name, input.IsActive).
		Scan(&updatedAt)
	if err != nil {
		return Organization{}, mapOrganizationWriteError(err)
	}
	after := Organization{ID: organizationID, Code: code, Name: name, IsActive: input.IsActive, CreatedAt: before.CreatedAt, UpdatedAt: updatedAt.Time}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: id, ActorUserID: principal.UserID, Action: "organization.updated",
		TargetType: "organization", TargetID: id, BeforeData: beforeJSON, AfterData: afterJSON,
		SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return Organization{}, fmt.Errorf("audit organization update: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Organization{}, fmt.Errorf("commit update organization: %w", err)
	}
	return after, nil
}

func validateOrganizationInput(code, name string) (string, string, error) {
	code = strings.TrimSpace(code)
	name = strings.TrimSpace(name)
	if len(code) == 0 || len(code) > 50 || len(name) == 0 || len(name) > 200 {
		return code, name, ErrOrganizationInvalid
	}
	return code, name, nil
}

func mapOrganizationWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrOrganizationConflict
		case "23503", "23514", "22023":
			return ErrOrganizationInvalid
		}
	}
	return fmt.Errorf("write organization: %w", err)
}
