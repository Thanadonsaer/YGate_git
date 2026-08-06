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

func (s *Service) HardDeletePlant(ctx context.Context, principal auth.Principal, plantID, confirmation string, sourceIP *netip.Addr) error {
	id, err := parseUUID(plantID)
	if err != nil {
		return ErrNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin hard delete plant: %w", err)
	}
	defer tx.Rollback(ctx)
	if err = requireGlobalHardDelete(ctx, tx, principal, "plant"); err != nil {
		return err
	}
	var organizationID pgtype.UUID
	var code, name string
	if err = tx.QueryRow(ctx, `SELECT organization_id, code, name FROM plant.plant WHERE id=$1 FOR UPDATE`, id).Scan(&organizationID, &code, &name); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("lock plant for hard delete: %w", err)
	}
	if !validHardDeleteConfirmation(confirmation, "DELETE PLANT "+code) {
		return ErrInvalid
	}
	if err = appendHardDeleteAudit(ctx, s.queries.WithTx(tx), principal, organizationID, id, "plant", map[string]any{"code": code, "name": name}, sourceIP); err != nil {
		return err
	}
	for _, statement := range []string{
		`DELETE FROM telemetry.telemetry_latest WHERE organization_id=$1 AND plant_id=$2`,
		`DELETE FROM telemetry.raw_register_reading WHERE organization_id=$1 AND plant_id=$2`,
		`DELETE FROM telemetry.telemetry_reading WHERE organization_id=$1 AND plant_id=$2`,
		`DELETE FROM plant.device WHERE organization_id=$1 AND plant_id=$2`,
		`DELETE FROM plant.plant WHERE organization_id=$1 AND id=$2`,
	} {
		if _, err = tx.Exec(ctx, statement, organizationID, id); err != nil {
			return fmt.Errorf("hard delete plant records: %w", err)
		}
	}
	return commitHardDelete(ctx, tx, "plant")
}

func (s *Service) HardDeleteDevice(ctx context.Context, principal auth.Principal, plantID, deviceID, confirmation string, sourceIP *netip.Addr) error {
	plantUUID, err := parseUUID(plantID)
	if err != nil {
		return ErrNotFound
	}
	deviceUUID, err := parseUUID(deviceID)
	if err != nil {
		return ErrNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin hard delete device: %w", err)
	}
	defer tx.Rollback(ctx)
	if err = requireGlobalHardDelete(ctx, tx, principal, "device"); err != nil {
		return err
	}
	var organizationID pgtype.UUID
	var externalID, name string
	if err = tx.QueryRow(ctx, `SELECT organization_id, external_id, name FROM plant.device WHERE id=$1 AND plant_id=$2 FOR UPDATE`, deviceUUID, plantUUID).Scan(&organizationID, &externalID, &name); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("lock device for hard delete: %w", err)
	}
	if !validHardDeleteConfirmation(confirmation, "DELETE DEVICE "+externalID) {
		return ErrInvalid
	}
	if err = appendHardDeleteAudit(ctx, s.queries.WithTx(tx), principal, organizationID, deviceUUID, "device", map[string]any{"externalId": externalID, "name": name, "plantId": plantID}, sourceIP); err != nil {
		return err
	}
	for _, statement := range []string{
		`DELETE FROM telemetry.telemetry_latest WHERE organization_id=$1 AND plant_id=$2 AND device_id=$3`,
		`DELETE FROM telemetry.raw_register_reading WHERE organization_id=$1 AND plant_id=$2 AND device_id=$3`,
		`DELETE FROM telemetry.telemetry_reading WHERE organization_id=$1 AND plant_id=$2 AND device_id=$3`,
		`DELETE FROM plant.device WHERE organization_id=$1 AND plant_id=$2 AND id=$3`,
	} {
		if _, err = tx.Exec(ctx, statement, organizationID, plantUUID, deviceUUID); err != nil {
			return fmt.Errorf("hard delete device records: %w", err)
		}
	}
	if err = commitHardDelete(ctx, tx, "device"); err != nil {
		return err
	}
	return nil
}

func (s *Service) HardDeleteDeviceModel(ctx context.Context, principal auth.Principal, modelID, confirmation string, sourceIP *netip.Addr) error {
	id, err := parseUUID(modelID)
	if err != nil {
		return ErrNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin hard delete device model: %w", err)
	}
	defer tx.Rollback(ctx)
	if err = requireGlobalHardDelete(ctx, tx, principal, "device_model"); err != nil {
		return err
	}
	var organizationID pgtype.UUID
	var manufacturer, model, deviceType string
	if err = tx.QueryRow(ctx, `SELECT organization_id, manufacturer, model, device_type FROM plant.device_model WHERE id=$1 FOR UPDATE`, id).Scan(&organizationID, &manufacturer, &model, &deviceType); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("lock device model for hard delete: %w", err)
	}
	if !validHardDeleteConfirmation(confirmation, "DELETE MODEL "+modelID) {
		return ErrInvalid
	}
	if err = appendHardDeleteAudit(ctx, s.queries.WithTx(tx), principal, organizationID, id, "device_model", map[string]any{"manufacturer": manufacturer, "model": model, "deviceType": deviceType}, sourceIP); err != nil {
		return err
	}
	for _, statement := range []string{
		`DELETE FROM telemetry.telemetry_latest WHERE organization_id=$1 AND device_id IN (SELECT id FROM plant.device WHERE organization_id=$1 AND device_model_id=$2)`,
		`DELETE FROM telemetry.raw_register_reading WHERE organization_id=$1 AND device_id IN (SELECT id FROM plant.device WHERE organization_id=$1 AND device_model_id=$2)`,
		`DELETE FROM telemetry.telemetry_reading WHERE organization_id=$1 AND device_id IN (SELECT id FROM plant.device WHERE organization_id=$1 AND device_model_id=$2)`,
		`DELETE FROM plant.device WHERE organization_id=$1 AND device_model_id=$2`,
		`DELETE FROM plant.device_model WHERE organization_id=$1 AND id=$2`,
	} {
		if _, err = tx.Exec(ctx, statement, organizationID, id); err != nil {
			return fmt.Errorf("hard delete device model records: %w", err)
		}
	}
	return commitHardDelete(ctx, tx, "device model")
}

func (s *Service) HardDeleteAPIKey(ctx context.Context, principal auth.Principal, keyID, confirmation string, sourceIP *netip.Addr) error {
	id, err := parseUUID(keyID)
	if err != nil {
		return ErrNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin hard delete api key: %w", err)
	}
	defer tx.Rollback(ctx)
	if err = requireGlobalHardDelete(ctx, tx, principal, "middleware_client"); err != nil {
		return err
	}
	// api_keys.go (getAPIKeyInTx/APIKeyClient) moved to auth-service's core
	// package; the middleware_client row lock + lookup needed here for the
	// confirmation phrase and audit trail is inlined directly instead of
	// calling code that no longer lives in this module.
	var organizationID pgtype.UUID
	var name, keyPrefix string
	var autoOnboard, isActive bool
	var createdAt pgtype.Timestamptz
	if err = tx.QueryRow(ctx, `SELECT organization_id, name, key_prefix, auto_onboard, is_active, created_at FROM auth.middleware_client WHERE id=$1 FOR UPDATE`, id).
		Scan(&organizationID, &name, &keyPrefix, &autoOnboard, &isActive, &createdAt); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("lock api key for hard delete: %w", err)
	}
	if !validHardDeleteConfirmation(confirmation, "DELETE API KEY "+name) {
		return ErrInvalid
	}
	if err = appendHardDeleteAudit(ctx, s.queries.WithTx(tx), principal, organizationID, id, "middleware_client", map[string]any{
		"organizationId": uuidString(organizationID),
		"name":           name,
		"keyPrefix":      keyPrefix,
		"autoOnboard":    autoOnboard,
		"isActive":       isActive,
		"createdAt":      createdAt.Time,
	}, sourceIP); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM telemetry.telemetry_latest WHERE organization_id=$1 AND telemetry_reading_id IN (SELECT id FROM telemetry.telemetry_reading WHERE middleware_client_id=$2)`, organizationID, id); err != nil {
		return fmt.Errorf("delete api key latest telemetry: %w", err)
	}
	for _, statement := range []string{
		`DELETE FROM telemetry.raw_register_reading WHERE organization_id=$1 AND middleware_client_id=$2`,
		`DELETE FROM telemetry.telemetry_reading WHERE organization_id=$1 AND middleware_client_id=$2`,
		`DELETE FROM telemetry.telemetry_ingest_batch WHERE organization_id=$1 AND middleware_client_id=$2`,
		`DELETE FROM auth.middleware_client WHERE organization_id=$1 AND id=$2`,
	} {
		if _, err = tx.Exec(ctx, statement, organizationID, id); err != nil {
			return fmt.Errorf("hard delete api key records: %w", err)
		}
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO telemetry.telemetry_latest (organization_id, plant_id, device_id, telemetry_reading_id, gateway_id, observed_at, received_at, data_item_map, parameter_count)
SELECT DISTINCT ON (tr.organization_id, tr.device_id)
       tr.organization_id, tr.plant_id, tr.device_id, tr.id, tr.gateway_id, tr.observed_at, tr.received_at, tr.data_item_map, tr.parameter_count
FROM telemetry.telemetry_reading tr
LEFT JOIN telemetry.telemetry_latest latest ON latest.organization_id=tr.organization_id AND latest.device_id=tr.device_id
WHERE tr.organization_id=$1 AND latest.device_id IS NULL
ORDER BY tr.organization_id, tr.device_id, tr.observed_at DESC, tr.received_at DESC, tr.id DESC
ON CONFLICT (organization_id, device_id) DO NOTHING`, organizationID); err != nil {
		return fmt.Errorf("restore latest telemetry after api key delete: %w", err)
	}
	return commitHardDelete(ctx, tx, "api key")
}

func requireGlobalHardDelete(ctx context.Context, tx pgx.Tx, principal auth.Principal, resource string) error {
	allowed, err := hasGlobalPermissionQuery(ctx, tx, principal, "hard_delete", resource)
	if err != nil {
		return fmt.Errorf("check global %s hard delete permission: %w", resource, err)
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func validHardDeleteConfirmation(actual, legacy string) bool {
	return actual == "DELETE" || actual == legacy
}

func appendHardDeleteAudit(ctx context.Context, q *dbgen.Queries, principal auth.Principal, organizationID, targetID pgtype.UUID, targetType string, before any, sourceIP *netip.Addr) error {
	correlationID, err := newUUID()
	if err != nil {
		return err
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(map[string]any{"hardDeleted": true, "rawIngestionEnvelopeRetained": targetType != "middleware_client"})
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: organizationID, ActorUserID: principal.UserID,
		Action: targetType + ".hard_deleted", TargetType: targetType, TargetID: targetID,
		BeforeData: beforeJSON, AfterData: afterJSON, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return fmt.Errorf("audit %s hard delete: %w", targetType, err)
	}
	return nil
}

func commitHardDelete(ctx context.Context, tx pgx.Tx, resource string) error {
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit hard delete %s: %w", resource, err)
	}
	return nil
}
