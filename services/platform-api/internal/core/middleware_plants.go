package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/database/dbgen"
)

// MiddlewarePlants lists the Plants currently assigned to middlewareID --
// the set that drives what buildConfigSnapshot computes for it.
func (s *Service) MiddlewarePlants(ctx context.Context, principal auth.Principal, middlewareID string) ([]Plant, error) {
	id, err := parseUUID(middlewareID)
	if err != nil {
		return nil, ErrMiddlewareNotFound
	}
	allowed, err := s.queries.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "read", ResourceType: "middleware_plant"})
	if err != nil {
		return nil, fmt.Errorf("check middleware plant read permission: %w", err)
	}
	if !allowed {
		return nil, ErrForbidden
	}
	rows, err := s.pool.Query(ctx, `
SELECT p.id, p.organization_id, o.name, p.code, p.name, p.timezone, p.latitude, p.longitude,
       p.installed_dc_kw, p.installed_ac_kw, p.image_url, p.is_active, p.created_at, p.updated_at
FROM middleware_gateway.middleware_plant mp
JOIN plant.plant p ON p.id = mp.plant_id
JOIN organization o ON o.id = p.organization_id
WHERE mp.middleware_client_id=$1
ORDER BY p.code`, id)
	if err != nil {
		return nil, fmt.Errorf("list middleware plants: %w", err)
	}
	defer rows.Close()
	plants := []Plant{}
	for rows.Next() {
		var plantID, organizationID pgtype.UUID
		var organizationName, code, name, timezone string
		var latitude, longitude pgtype.Float8
		var installedDcKW, installedAcKW pgtype.Numeric
		var imageURL *string
		var isActive bool
		var createdAt, updatedAt pgtype.Timestamptz
		if err := rows.Scan(&plantID, &organizationID, &organizationName, &code, &name, &timezone, &latitude, &longitude, &installedDcKW, &installedAcKW, &imageURL, &isActive, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan middleware plant: %w", err)
		}
		plants = append(plants, plantFromFields(plantID, organizationID, organizationName, code, name, timezone, latitude, longitude, installedDcKW, installedAcKW, imageURL, isActive, createdAt, updatedAt))
	}
	return plants, rows.Err()
}

// AssignMiddlewarePlant assigns plantID to middlewareID. Because
// middleware_plant carries a UNIQUE(plant_id) constraint (one Middleware per
// Plant, confirmed with the user), this is an upsert that silently
// reassigns the Plant away from whichever Middleware held it before -- both
// the new and (if different) previously-holding Middleware get their
// config recomputed and pushed afterward.
func (s *Service) AssignMiddlewarePlant(ctx context.Context, principal auth.Principal, middlewareID, plantID string, sourceIP *netip.Addr) error {
	mwUUID, err := parseUUID(middlewareID)
	if err != nil {
		return ErrMiddlewareNotFound
	}
	plantUUID, err := parseUUID(plantID)
	if err != nil {
		return ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin assign middleware plant: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	var mwOrgID pgtype.UUID
	if err = tx.QueryRow(ctx, `SELECT organization_id FROM auth.middleware_client WHERE id=$1`, mwUUID).Scan(&mwOrgID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrMiddlewareNotFound
		}
		return fmt.Errorf("lookup middleware organization: %w", err)
	}
	if err = s.requireOrganizationPermission(ctx, q, principal, "update", "middleware_plant", mwOrgID); err != nil {
		return err
	}
	var plantOrgID pgtype.UUID
	if err = tx.QueryRow(ctx, `SELECT organization_id FROM plant.plant WHERE id=$1`, plantUUID).Scan(&plantOrgID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalid
		}
		return fmt.Errorf("lookup plant organization: %w", err)
	}
	if plantOrgID != mwOrgID {
		return ErrInvalid
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO middleware_gateway.middleware_plant (middleware_client_id, organization_id, plant_id) VALUES ($1,$2,$3)
ON CONFLICT (plant_id) DO UPDATE SET middleware_client_id=EXCLUDED.middleware_client_id, created_at=now()`,
		mwUUID, mwOrgID, plantUUID); err != nil {
		return fmt.Errorf("assign middleware plant: %w", err)
	}
	correlationID, err := newUUID()
	if err != nil {
		return err
	}
	after, _ := json.Marshal(map[string]string{"middlewareId": middlewareID, "plantId": plantID})
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: mwOrgID, ActorUserID: principal.UserID, Action: "middleware_plant.assigned",
		TargetType: "middleware_client", TargetID: mwUUID, AfterData: after, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return fmt.Errorf("audit middleware plant assign: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit assign middleware plant: %w", err)
	}
	return nil
}

func (s *Service) UnassignMiddlewarePlant(ctx context.Context, principal auth.Principal, middlewareID, plantID string, sourceIP *netip.Addr) error {
	mwUUID, err := parseUUID(middlewareID)
	if err != nil {
		return ErrMiddlewareNotFound
	}
	plantUUID, err := parseUUID(plantID)
	if err != nil {
		return ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin unassign middleware plant: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	var mwOrgID pgtype.UUID
	if err = tx.QueryRow(ctx, `SELECT organization_id FROM auth.middleware_client WHERE id=$1`, mwUUID).Scan(&mwOrgID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrMiddlewareNotFound
		}
		return fmt.Errorf("lookup middleware organization: %w", err)
	}
	if err = s.requireOrganizationPermission(ctx, q, principal, "update", "middleware_plant", mwOrgID); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `DELETE FROM middleware_gateway.middleware_plant WHERE middleware_client_id=$1 AND plant_id=$2`, mwUUID, plantUUID)
	if err != nil {
		return fmt.Errorf("unassign middleware plant: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	correlationID, err := newUUID()
	if err != nil {
		return err
	}
	before, _ := json.Marshal(map[string]string{"middlewareId": middlewareID, "plantId": plantID})
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: mwOrgID, ActorUserID: principal.UserID, Action: "middleware_plant.unassigned",
		TargetType: "middleware_client", TargetID: mwUUID, BeforeData: before, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return fmt.Errorf("audit middleware plant unassign: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit unassign middleware plant: %w", err)
	}
	return nil
}
