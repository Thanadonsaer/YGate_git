package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/database/dbgen"
)

type Device struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	PlantID        string    `json:"plantId"`
	ExternalID     string    `json:"externalId"`
	Name           string    `json:"name"`
	DeviceModelID  string    `json:"deviceModelId"`
	Manufacturer   string    `json:"manufacturer"`
	Model          string    `json:"model"`
	DeviceType     string    `json:"deviceType"`
	SourceTypeID   *int32    `json:"sourceTypeId"`
	IsActive       bool      `json:"isActive"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type UpdateDeviceInput struct {
	Name     string
	IsActive bool
}

type CreateDeviceInput struct {
	ExternalID   string
	Name         string
	Manufacturer string
	Model        string
	DeviceType   string
	SourceTypeID *int32
	SerialNumber string
}

type DeviceRegisterMetadata struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	PlantID        string    `json:"plantId"`
	DeviceID       string    `json:"deviceId"`
	AddressKey     string    `json:"addressKey"`
	DisplayName    string    `json:"displayName"`
	Unit           string    `json:"unit"`
	DataType       string    `json:"dataType"`
	Scale          float64   `json:"scale"`
	Offset         float64   `json:"offset"`
	Decimals       int32     `json:"decimals"`
	IsEnabled      bool      `json:"isEnabled"`
	Notes          string    `json:"notes"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type DeviceModelOption struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	Manufacturer   string    `json:"manufacturer"`
	Model          string    `json:"model"`
	DeviceType     string    `json:"deviceType"`
	SourceTypeID   *int32    `json:"sourceTypeId"`
	IsActive       bool      `json:"isActive"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type DeviceModelInput struct {
	OrganizationID string
	Manufacturer   string
	Model          string
	DeviceType     string
	SourceTypeID   *int32
	IsActive       bool
}

type DeviceModelRegisterMetadata struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	DeviceModelID  string    `json:"deviceModelId"`
	AddressKey     string    `json:"addressKey"`
	DisplayName    string    `json:"displayName"`
	Unit           string    `json:"unit"`
	DataType       string    `json:"dataType"`
	Scale          float64   `json:"scale"`
	Offset         float64   `json:"offset"`
	Decimals       int32     `json:"decimals"`
	IsEnabled      bool      `json:"isEnabled"`
	Notes          string    `json:"notes"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type UpdateDeviceRegisterMetadataInput struct {
	AddressKey  string
	DisplayName string
	Unit        string
	DataType    string
	Scale       float64
	Offset      float64
	Decimals    int32
	IsEnabled   bool
	Notes       string
}

type UpdateDeviceModelRegisterMetadataInput = UpdateDeviceRegisterMetadataInput

func (s *Service) Devices(ctx context.Context, principal auth.Principal, plantID string) ([]Device, error) {
	id, err := parseUUID(plantID)
	if err != nil {
		return nil, ErrNotFound
	}
	plant, err := s.queries.GetAuthorizedPlantResource(ctx, dbgen.GetAuthorizedPlantResourceParams{
		PlantID: id, UserID: principal.UserID, Action: "read", ResourceType: "device",
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("authorize device list: %w", err)
	}
	rows, err := s.queries.ListPlantDevices(ctx, dbgen.ListPlantDevicesParams{
		OrganizationID: plant.OrganizationID, PlantID: plant.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	devices := make([]Device, 0, len(rows))
	for _, row := range rows {
		devices = append(devices, deviceFromFields(
			row.ID, row.OrganizationID, row.PlantID, row.ExternalID, row.Name,
			row.DeviceModelID, row.Manufacturer, row.Model, row.DeviceType,
			row.SourceTypeID, row.IsActive, row.CreatedAt, row.UpdatedAt,
		))
	}
	return devices, nil
}

func (s *Service) DeviceModels(ctx context.Context, principal auth.Principal) ([]DeviceModelOption, error) {
	allowed, err := s.queries.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "read", ResourceType: "device_model"})
	if err != nil {
		return nil, fmt.Errorf("check device model read permission: %w", err)
	}
	if !allowed {
		return nil, ErrForbidden
	}
	rows, err := s.pool.Query(ctx, `
SELECT dm.id, dm.organization_id, dm.manufacturer, dm.model, dm.device_type, dm.source_type_id,
       dm.is_active, dm.created_at, dm.updated_at
FROM device_model dm
WHERE EXISTS (
    SELECT 1 FROM user_role ur
    JOIN role r ON r.id = ur.role_id
    JOIN role_permission rp ON rp.role_id = ur.role_id
    JOIN permission pm ON pm.id = rp.permission_id
    WHERE ur.user_id=$1
      AND pm.action='read' AND pm.resource_type='device_model'
      AND (r.organization_id IS NULL OR r.organization_id = ur.organization_id)
      AND (rp.organization_id IS NULL OR rp.organization_id = ur.organization_id)
      AND ur.plant_id IS NULL
      AND (ur.organization_id IS NULL OR ur.organization_id = dm.organization_id)
)
ORDER BY dm.manufacturer, dm.device_type, dm.model
LIMIT 500`, principal.UserID)
	if err != nil {
		return nil, fmt.Errorf("list device models: %w", err)
	}
	defer rows.Close()
	models := []DeviceModelOption{}
	for rows.Next() {
		var model DeviceModelOption
		var id, organizationID pgtype.UUID
		var sourceTypeID pgtype.Int4
		var createdAt, updatedAt pgtype.Timestamptz
		if err := rows.Scan(&id, &organizationID, &model.Manufacturer, &model.Model, &model.DeviceType, &sourceTypeID, &model.IsActive, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan device model: %w", err)
		}
		model.ID = uuidString(id)
		model.OrganizationID = uuidString(organizationID)
		model.SourceTypeID = int32Pointer(sourceTypeID)
		model.CreatedAt = createdAt.Time
		model.UpdatedAt = updatedAt.Time
		models = append(models, model)
	}
	return models, rows.Err()
}

func (s *Service) CreateDeviceModel(ctx context.Context, principal auth.Principal, input DeviceModelInput, sourceIP *netip.Addr) (DeviceModelOption, error) {
	organizationID, err := s.deviceModelInputOrganization(input, principal)
	if err != nil {
		return DeviceModelOption{}, err
	}
	if err = validateDeviceModelInput(&input); err != nil {
		return DeviceModelOption{}, err
	}
	id, err := newUUID()
	if err != nil {
		return DeviceModelOption{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DeviceModelOption{}, fmt.Errorf("begin create device model: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	if err = s.requireOrganizationPermission(ctx, q, principal, "create", "device_model", organizationID); err != nil {
		return DeviceModelOption{}, err
	}
	model, err := upsertDeviceModelRow(ctx, tx, id, organizationID, input, true)
	if err != nil {
		return DeviceModelOption{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return DeviceModelOption{}, err
	}
	after, _ := json.Marshal(model)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "device_model.created", TargetType: "device_model", TargetID: id, AfterData: after, SourceIp: sourceIP, CorrelationID: correlationID}); err != nil {
		return DeviceModelOption{}, fmt.Errorf("audit device model create: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return DeviceModelOption{}, fmt.Errorf("commit create device model: %w", err)
	}
	return model, nil
}

func (s *Service) UpdateDeviceModel(ctx context.Context, principal auth.Principal, modelID string, input DeviceModelInput, sourceIP *netip.Addr) (DeviceModelOption, error) {
	id, err := parseUUID(modelID)
	if err != nil {
		return DeviceModelOption{}, ErrNotFound
	}
	if err = validateDeviceModelInput(&input); err != nil {
		return DeviceModelOption{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DeviceModelOption{}, fmt.Errorf("begin update device model: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	before, err := getDeviceModelRow(ctx, tx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeviceModelOption{}, ErrNotFound
	}
	if err != nil {
		return DeviceModelOption{}, err
	}
	organizationID, err := parseUUID(before.OrganizationID)
	if err != nil {
		return DeviceModelOption{}, ErrNotFound
	}
	if err = s.requireOrganizationPermission(ctx, q, principal, "update", "device_model", organizationID); err != nil {
		return DeviceModelOption{}, err
	}
	model, err := updateDeviceModelRow(ctx, tx, id, input)
	if err != nil {
		return DeviceModelOption{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return DeviceModelOption{}, err
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(model)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "device_model.updated", TargetType: "device_model", TargetID: id, BeforeData: beforeJSON, AfterData: afterJSON, SourceIp: sourceIP, CorrelationID: correlationID}); err != nil {
		return DeviceModelOption{}, fmt.Errorf("audit device model update: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return DeviceModelOption{}, fmt.Errorf("commit update device model: %w", err)
	}
	return model, nil
}

func (s *Service) DeviceModelRegisterMetadata(ctx context.Context, principal auth.Principal, modelID string) ([]DeviceModelRegisterMetadata, error) {
	modelUUID, err := parseUUID(modelID)
	if err != nil {
		return nil, ErrNotFound
	}
	organizationID, err := s.authorizedDeviceModelScope(ctx, principal, modelUUID, "read")
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, organization_id, device_model_id, address_key, display_name, unit, data_type,
       scale, value_offset, decimals, is_enabled, notes, created_at, updated_at
FROM device_model_register_metadata
WHERE organization_id=$1 AND device_model_id=$2
ORDER BY address_key
LIMIT 1000`, organizationID, modelUUID)
	if err != nil {
		return nil, fmt.Errorf("list model register metadata: %w", err)
	}
	defer rows.Close()
	items := []DeviceModelRegisterMetadata{}
	for rows.Next() {
		item, err := scanDeviceModelRegisterMetadata(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) SetDeviceModelRegisterMetadata(ctx context.Context, principal auth.Principal, modelID string, input UpdateDeviceModelRegisterMetadataInput, sourceIP *netip.Addr) (DeviceModelRegisterMetadata, error) {
	modelUUID, err := parseUUID(modelID)
	if err != nil {
		return DeviceModelRegisterMetadata{}, ErrNotFound
	}
	if err = validateRegisterMetadata((*UpdateDeviceRegisterMetadataInput)(&input)); err != nil {
		return DeviceModelRegisterMetadata{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DeviceModelRegisterMetadata{}, fmt.Errorf("begin model register metadata update: %w", err)
	}
	defer tx.Rollback(ctx)
	organizationID, err := authorizedDeviceModelScopeQuery(ctx, tx, principal, modelUUID, "update")
	if err != nil {
		return DeviceModelRegisterMetadata{}, err
	}
	before, err := getDeviceModelRegisterMetadataInTx(ctx, tx, organizationID, modelUUID, input.AddressKey)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return DeviceModelRegisterMetadata{}, err
	}
	id := pgtype.UUID{}
	if errors.Is(err, pgx.ErrNoRows) {
		id, err = newUUID()
		if err != nil {
			return DeviceModelRegisterMetadata{}, err
		}
	} else {
		id, _ = parseUUID(before.ID)
	}
	row := tx.QueryRow(ctx, `
INSERT INTO device_model_register_metadata (
    id, organization_id, device_model_id, address_key, display_name, unit, data_type,
    scale, value_offset, decimals, is_enabled, notes
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (organization_id, device_model_id, address_key) DO UPDATE
SET display_name=EXCLUDED.display_name, unit=EXCLUDED.unit, data_type=EXCLUDED.data_type,
    scale=EXCLUDED.scale, value_offset=EXCLUDED.value_offset, decimals=EXCLUDED.decimals,
    is_enabled=EXCLUDED.is_enabled, notes=EXCLUDED.notes, updated_at=now()
RETURNING id, organization_id, device_model_id, address_key, display_name, unit, data_type,
          scale, value_offset, decimals, is_enabled, notes, created_at, updated_at`,
		id, organizationID, modelUUID, input.AddressKey, input.DisplayName, input.Unit, input.DataType,
		input.Scale, input.Offset, input.Decimals, input.IsEnabled, input.Notes)
	after, err := scanDeviceModelRegisterMetadata(row)
	if err != nil {
		return DeviceModelRegisterMetadata{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return DeviceModelRegisterMetadata{}, err
	}
	var beforeJSON []byte
	if before.ID != "" {
		beforeJSON, _ = json.Marshal(before)
	}
	afterJSON, _ := json.Marshal(after)
	q := s.queries.WithTx(tx)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "device_model_register_metadata.updated", TargetType: "device_model", TargetID: modelUUID, BeforeData: beforeJSON, AfterData: afterJSON, SourceIp: sourceIP, CorrelationID: correlationID}); err != nil {
		return DeviceModelRegisterMetadata{}, fmt.Errorf("audit model register metadata update: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return DeviceModelRegisterMetadata{}, fmt.Errorf("commit model register metadata update: %w", err)
	}
	return after, nil
}

func (s *Service) DeleteDeviceModelRegisterMetadata(ctx context.Context, principal auth.Principal, modelID, addressKey string, sourceIP *netip.Addr) error {
	modelUUID, err := parseUUID(modelID)
	if err != nil {
		return ErrNotFound
	}
	addressKey = strings.TrimSpace(addressKey)
	if addressKey == "" || len(addressKey) > 200 {
		return ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete model register metadata: %w", err)
	}
	defer tx.Rollback(ctx)
	organizationID, err := authorizedDeviceModelScopeQuery(ctx, tx, principal, modelUUID, "update")
	if err != nil {
		return err
	}
	before, err := getDeviceModelRegisterMetadataInTx(ctx, tx, organizationID, modelUUID, addressKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM device_model_register_metadata WHERE organization_id=$1 AND device_model_id=$2 AND address_key=$3`, organizationID, modelUUID, addressKey); err != nil {
		return fmt.Errorf("delete model register metadata: %w", err)
	}
	correlationID, err := newUUID()
	if err != nil {
		return err
	}
	beforeJSON, _ := json.Marshal(before)
	q := s.queries.WithTx(tx)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "device_model_register_metadata.deleted", TargetType: "device_model", TargetID: modelUUID, BeforeData: beforeJSON, SourceIp: sourceIP, CorrelationID: correlationID}); err != nil {
		return fmt.Errorf("audit model register metadata delete: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *Service) CreateDevice(ctx context.Context, principal auth.Principal, plantID string, input CreateDeviceInput, sourceIP *netip.Addr) (Device, error) {
	plantUUID, err := parseUUID(plantID)
	if err != nil {
		return Device{}, ErrNotFound
	}
	input.ExternalID = strings.TrimSpace(input.ExternalID)
	input.Name = strings.TrimSpace(input.Name)
	input.Manufacturer = strings.TrimSpace(input.Manufacturer)
	input.Model = strings.TrimSpace(input.Model)
	input.DeviceType = strings.TrimSpace(input.DeviceType)
	input.SerialNumber = strings.TrimSpace(input.SerialNumber)
	if input.ExternalID == "" || input.Name == "" || input.Manufacturer == "" || input.Model == "" || input.DeviceType == "" || len(input.ExternalID) > 200 || len(input.Name) > 200 || len(input.Manufacturer) > 200 || len(input.Model) > 200 || len(input.DeviceType) > 100 || len(input.SerialNumber) > 200 {
		return Device{}, ErrInvalid
	}
	if input.SourceTypeID != nil && *input.SourceTypeID < 0 {
		return Device{}, ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Device{}, fmt.Errorf("begin create device: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	plant, err := q.GetAuthorizedPlantResource(ctx, dbgen.GetAuthorizedPlantResourceParams{PlantID: plantUUID, UserID: principal.UserID, Action: "create", ResourceType: "device"})
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	if err != nil {
		return Device{}, fmt.Errorf("authorize device create: %w", err)
	}
	modelID, err := newUUID()
	if err != nil {
		return Device{}, err
	}
	var sourceType pgtype.Int4
	if input.SourceTypeID != nil {
		sourceType = pgtype.Int4{Int32: *input.SourceTypeID, Valid: true}
	}
	var storedModelID pgtype.UUID
	if err = tx.QueryRow(ctx, `
INSERT INTO device_model (id, organization_id, manufacturer, model, device_type, source_type_id)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (organization_id, manufacturer, model) DO UPDATE
SET device_type=EXCLUDED.device_type, source_type_id=EXCLUDED.source_type_id, updated_at=now()
RETURNING id`, modelID, plant.OrganizationID, input.Manufacturer, input.Model, input.DeviceType, sourceType).Scan(&storedModelID); err != nil {
		return Device{}, fmt.Errorf("upsert device model: %w", err)
	}
	deviceID, err := newUUID()
	if err != nil {
		return Device{}, err
	}
	var serial any
	if input.SerialNumber != "" {
		serial = input.SerialNumber
	}
	metadata, _ := json.Marshal(map[string]any{"source": "MANUAL"})
	row := tx.QueryRow(ctx, `
INSERT INTO device (id, organization_id, plant_id, device_model_id, external_id, name, serial_number, source_metadata)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING id, organization_id, plant_id, external_id, name, device_model_id, is_active, created_at, updated_at`, deviceID, plant.OrganizationID, plant.ID, storedModelID, input.ExternalID, input.Name, serial, metadata)
	var id, organizationID, returnedPlantID, returnedModelID pgtype.UUID
	var createdAt, updatedAt pgtype.Timestamptz
	var device Device
	if err = row.Scan(&id, &organizationID, &returnedPlantID, &device.ExternalID, &device.Name, &returnedModelID, &device.IsActive, &createdAt, &updatedAt); err != nil {
		return Device{}, mapWriteError(err)
	}
	device = deviceFromFields(id, organizationID, returnedPlantID, device.ExternalID, device.Name, returnedModelID, input.Manufacturer, input.Model, input.DeviceType, sourceType, device.IsActive, createdAt, updatedAt)
	correlationID, err := newUUID()
	if err != nil {
		return Device{}, err
	}
	after, _ := json.Marshal(device)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{OrganizationID: plant.OrganizationID, ActorUserID: principal.UserID, Action: "device.created", TargetType: "device", TargetID: deviceID, AfterData: after, SourceIp: sourceIP, CorrelationID: correlationID}); err != nil {
		return Device{}, fmt.Errorf("audit device create: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Device{}, fmt.Errorf("commit create device: %w", err)
	}
	return device, nil
}

func (s *Service) UpdateDevice(ctx context.Context, principal auth.Principal, plantID, deviceID string, input UpdateDeviceInput, sourceIP *netip.Addr) (Device, error) {
	plantUUID, err := parseUUID(plantID)
	if err != nil {
		return Device{}, ErrNotFound
	}
	deviceUUID, err := parseUUID(deviceID)
	if err != nil {
		return Device{}, ErrNotFound
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 200 {
		return Device{}, ErrInvalid
	}
	correlationID, err := newUUID()
	if err != nil {
		return Device{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Device{}, fmt.Errorf("begin update device: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	current, err := q.GetAuthorizedDeviceForUpdate(ctx, dbgen.GetAuthorizedDeviceForUpdateParams{
		DeviceID: deviceUUID, PlantID: plantUUID, UserID: principal.UserID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	if err != nil {
		return Device{}, fmt.Errorf("lock device: %w", err)
	}
	beforeDevice := deviceFromFields(
		current.ID, current.OrganizationID, current.PlantID, current.ExternalID, current.Name,
		current.DeviceModelID, current.Manufacturer, current.Model, current.DeviceType,
		current.SourceTypeID, current.IsActive, current.CreatedAt, current.UpdatedAt,
	)
	row, err := q.UpdateDevice(ctx, dbgen.UpdateDeviceParams{
		ID: deviceUUID, Name: input.Name, IsActive: input.IsActive,
	})
	if err != nil {
		return Device{}, fmt.Errorf("update device: %w", err)
	}
	device := deviceFromFields(
		row.ID, row.OrganizationID, row.PlantID, row.ExternalID, row.Name,
		row.DeviceModelID, current.Manufacturer, current.Model, current.DeviceType,
		current.SourceTypeID, row.IsActive, row.CreatedAt, row.UpdatedAt,
	)
	before, _ := json.Marshal(beforeDevice)
	after, _ := json.Marshal(device)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: current.OrganizationID, ActorUserID: principal.UserID,
		Action: "device.updated", TargetType: "device", TargetID: deviceUUID,
		BeforeData: before, AfterData: after, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return Device{}, fmt.Errorf("audit device update: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Device{}, fmt.Errorf("commit update device: %w", err)
	}
	return device, nil
}

func (s *Service) DeviceRegisterMetadata(ctx context.Context, principal auth.Principal, plantID, deviceID string) ([]DeviceRegisterMetadata, error) {
	plantUUID, err := parseUUID(plantID)
	if err != nil {
		return nil, ErrNotFound
	}
	deviceUUID, err := parseUUID(deviceID)
	if err != nil {
		return nil, ErrNotFound
	}
	organizationID, _, err := s.authorizedDeviceScope(ctx, principal, plantUUID, deviceUUID, "read", false)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, organization_id, plant_id, device_id, address_key, display_name, unit, data_type,
       scale, value_offset, decimals, is_enabled, notes, created_at, updated_at
FROM device_register_metadata
WHERE organization_id=$1 AND plant_id=$2 AND device_id=$3
ORDER BY address_key
LIMIT 1000`, organizationID, plantUUID, deviceUUID)
	if err != nil {
		return nil, fmt.Errorf("list register metadata: %w", err)
	}
	defer rows.Close()
	items := []DeviceRegisterMetadata{}
	for rows.Next() {
		item, err := scanDeviceRegisterMetadata(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) SetDeviceRegisterMetadata(ctx context.Context, principal auth.Principal, plantID, deviceID string, input UpdateDeviceRegisterMetadataInput, sourceIP *netip.Addr) (DeviceRegisterMetadata, error) {
	plantUUID, err := parseUUID(plantID)
	if err != nil {
		return DeviceRegisterMetadata{}, ErrNotFound
	}
	deviceUUID, err := parseUUID(deviceID)
	if err != nil {
		return DeviceRegisterMetadata{}, ErrNotFound
	}
	if err = validateRegisterMetadata(&input); err != nil {
		return DeviceRegisterMetadata{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DeviceRegisterMetadata{}, fmt.Errorf("begin register metadata update: %w", err)
	}
	defer tx.Rollback(ctx)
	organizationID, _, err := authorizedDeviceScopeQuery(ctx, tx, principal, plantUUID, deviceUUID, "update", true)
	if err != nil {
		return DeviceRegisterMetadata{}, err
	}
	var before DeviceRegisterMetadata
	before, err = s.getRegisterMetadataInTx(ctx, tx, organizationID, plantUUID, deviceUUID, input.AddressKey)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return DeviceRegisterMetadata{}, err
	}
	id := pgtype.UUID{}
	if errors.Is(err, pgx.ErrNoRows) {
		id, err = newUUID()
		if err != nil {
			return DeviceRegisterMetadata{}, err
		}
	} else {
		id, _ = parseUUID(before.ID)
	}
	row := tx.QueryRow(ctx, `
INSERT INTO device_register_metadata (
    id, organization_id, plant_id, device_id, address_key, display_name, unit, data_type,
    scale, value_offset, decimals, is_enabled, notes
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT (organization_id, plant_id, device_id, address_key) DO UPDATE
SET display_name=EXCLUDED.display_name, unit=EXCLUDED.unit, data_type=EXCLUDED.data_type,
    scale=EXCLUDED.scale, value_offset=EXCLUDED.value_offset, decimals=EXCLUDED.decimals,
    is_enabled=EXCLUDED.is_enabled, notes=EXCLUDED.notes, updated_at=now()
RETURNING id, organization_id, plant_id, device_id, address_key, display_name, unit, data_type,
          scale, value_offset, decimals, is_enabled, notes, created_at, updated_at`,
		id, organizationID, plantUUID, deviceUUID, input.AddressKey, input.DisplayName, input.Unit, input.DataType,
		input.Scale, input.Offset, input.Decimals, input.IsEnabled, input.Notes)
	after, err := scanDeviceRegisterMetadata(row)
	if err != nil {
		return DeviceRegisterMetadata{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return DeviceRegisterMetadata{}, err
	}
	var beforeJSON []byte
	if before.ID != "" {
		beforeJSON, _ = json.Marshal(before)
	}
	afterJSON, _ := json.Marshal(after)
	q := s.queries.WithTx(tx)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: organizationID, ActorUserID: principal.UserID,
		Action: "device_register_metadata.updated", TargetType: "device", TargetID: deviceUUID,
		BeforeData: beforeJSON, AfterData: afterJSON, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return DeviceRegisterMetadata{}, fmt.Errorf("audit register metadata update: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return DeviceRegisterMetadata{}, fmt.Errorf("commit register metadata update: %w", err)
	}
	return after, nil
}

func (s *Service) authorizedDeviceScope(ctx context.Context, principal auth.Principal, plantID, deviceID pgtype.UUID, action string, lock bool) (pgtype.UUID, pgtype.UUID, error) {
	return authorizedDeviceScopeQuery(ctx, s.pool, principal, plantID, deviceID, action, lock)
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func authorizedDeviceScopeQuery(ctx context.Context, querier rowQuerier, principal auth.Principal, plantID, deviceID pgtype.UUID, action string, lock bool) (pgtype.UUID, pgtype.UUID, error) {
	query := `
SELECT d.organization_id, d.plant_id
FROM device d
WHERE d.id=$1 AND d.plant_id=$2
  AND EXISTS (
      SELECT 1 FROM user_role ur
      JOIN role r ON r.id = ur.role_id
      JOIN role_permission rp ON rp.role_id = ur.role_id
      JOIN permission pm ON pm.id = rp.permission_id
      WHERE ur.user_id = $3
        AND pm.action = $4 AND pm.resource_type = 'device'
        AND (r.organization_id IS NULL OR r.organization_id = ur.organization_id)
        AND (rp.organization_id IS NULL OR rp.organization_id = ur.organization_id)
        AND (ur.organization_id IS NULL OR ur.organization_id = d.organization_id)
        AND (ur.plant_id IS NULL OR ur.plant_id = d.plant_id)
  )
LIMIT 1`
	if lock {
		query += " FOR UPDATE OF d"
	}
	var organizationID, authorizedPlantID pgtype.UUID
	err := querier.QueryRow(ctx, query, deviceID, plantID, principal.UserID, action).Scan(&organizationID, &authorizedPlantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, pgtype.UUID{}, ErrNotFound
	}
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("authorize register metadata %s: %w", action, err)
	}
	return organizationID, authorizedPlantID, nil
}

func (s *Service) authorizedDeviceModelScope(ctx context.Context, principal auth.Principal, modelID pgtype.UUID, action string) (pgtype.UUID, error) {
	return authorizedDeviceModelScopeQuery(ctx, s.pool, principal, modelID, action)
}

func authorizedDeviceModelScopeQuery(ctx context.Context, querier rowQuerier, principal auth.Principal, modelID pgtype.UUID, action string) (pgtype.UUID, error) {
	var organizationID pgtype.UUID
	err := querier.QueryRow(ctx, `
SELECT dm.organization_id
FROM device_model dm
WHERE dm.id=$1
  AND EXISTS (
      SELECT 1 FROM user_role ur
      JOIN role r ON r.id = ur.role_id
      JOIN role_permission rp ON rp.role_id = ur.role_id
      JOIN permission pm ON pm.id = rp.permission_id
      WHERE ur.user_id=$2
        AND pm.action=$3 AND pm.resource_type='device_model'
        AND (r.organization_id IS NULL OR r.organization_id = ur.organization_id)
        AND (rp.organization_id IS NULL OR rp.organization_id = ur.organization_id)
        AND (ur.organization_id IS NULL OR ur.organization_id = dm.organization_id)
        AND ur.plant_id IS NULL
  )
LIMIT 1`, modelID, principal.UserID, action).Scan(&organizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, ErrNotFound
	}
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("authorize device model %s: %w", action, err)
	}
	return organizationID, nil
}

func validateRegisterMetadata(input *UpdateDeviceRegisterMetadataInput) error {
	input.AddressKey = strings.TrimSpace(input.AddressKey)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Unit = strings.TrimSpace(input.Unit)
	input.DataType = strings.TrimSpace(input.DataType)
	input.Notes = strings.TrimSpace(input.Notes)
	if input.DisplayName == "" {
		input.DisplayName = input.AddressKey
	}
	if input.DataType == "" {
		input.DataType = "number"
	}
	if input.Scale == 0 {
		input.Scale = 1
	}
	if input.AddressKey == "" || len(input.AddressKey) > 200 || input.DisplayName == "" || len(input.DisplayName) > 200 || len(input.Unit) > 40 || len(input.Notes) > 500 || input.Decimals < 0 || input.Decimals > 9 || math.IsNaN(input.Scale) || math.IsInf(input.Scale, 0) || math.IsNaN(input.Offset) || math.IsInf(input.Offset, 0) {
		return ErrInvalid
	}
	switch input.DataType {
	case "number", "boolean", "text", "enum":
		return nil
	default:
		return ErrInvalid
	}
}

func (s *Service) deviceModelInputOrganization(input DeviceModelInput, principal auth.Principal) (pgtype.UUID, error) {
	if strings.TrimSpace(input.OrganizationID) != "" {
		return parseUUID(input.OrganizationID)
	}
	if !principal.OrganizationID.Valid {
		return pgtype.UUID{}, ErrInvalid
	}
	return principal.OrganizationID, nil
}

func validateDeviceModelInput(input *DeviceModelInput) error {
	input.Manufacturer = strings.TrimSpace(input.Manufacturer)
	input.Model = strings.TrimSpace(input.Model)
	input.DeviceType = strings.TrimSpace(input.DeviceType)
	if input.Manufacturer == "" || input.Model == "" || input.DeviceType == "" || len(input.Manufacturer) > 200 || len(input.Model) > 200 || len(input.DeviceType) > 100 {
		return ErrInvalid
	}
	if input.SourceTypeID != nil && *input.SourceTypeID < 0 {
		return ErrInvalid
	}
	return nil
}

func upsertDeviceModelRow(ctx context.Context, tx pgx.Tx, id, organizationID pgtype.UUID, input DeviceModelInput, failOnConflict bool) (DeviceModelOption, error) {
	var sourceType pgtype.Int4
	if input.SourceTypeID != nil {
		sourceType = pgtype.Int4{Int32: *input.SourceTypeID, Valid: true}
	}
	row := tx.QueryRow(ctx, `
INSERT INTO device_model (id, organization_id, manufacturer, model, device_type, source_type_id, is_active)
VALUES ($1,$2,$3,$4,$5,$6,$7)
RETURNING id, organization_id, manufacturer, model, device_type, source_type_id, is_active, created_at, updated_at`, id, organizationID, input.Manufacturer, input.Model, input.DeviceType, sourceType, input.IsActive)
	model, err := scanDeviceModelRow(row)
	if err != nil && failOnConflict {
		return DeviceModelOption{}, mapWriteError(err)
	}
	return model, err
}

func updateDeviceModelRow(ctx context.Context, tx pgx.Tx, id pgtype.UUID, input DeviceModelInput) (DeviceModelOption, error) {
	var sourceType pgtype.Int4
	if input.SourceTypeID != nil {
		sourceType = pgtype.Int4{Int32: *input.SourceTypeID, Valid: true}
	}
	row := tx.QueryRow(ctx, `
UPDATE device_model
SET manufacturer=$2, model=$3, device_type=$4, source_type_id=$5, is_active=$6, updated_at=now()
WHERE id=$1
RETURNING id, organization_id, manufacturer, model, device_type, source_type_id, is_active, created_at, updated_at`, id, input.Manufacturer, input.Model, input.DeviceType, sourceType, input.IsActive)
	model, err := scanDeviceModelRow(row)
	if err != nil {
		return DeviceModelOption{}, mapWriteError(err)
	}
	return model, nil
}

func getDeviceModelRow(ctx context.Context, tx pgx.Tx, id pgtype.UUID) (DeviceModelOption, error) {
	return scanDeviceModelRow(tx.QueryRow(ctx, `
SELECT id, organization_id, manufacturer, model, device_type, source_type_id, is_active, created_at, updated_at
FROM device_model WHERE id=$1 FOR UPDATE`, id))
}

func scanDeviceModelRow(row registerMetadataScanner) (DeviceModelOption, error) {
	var model DeviceModelOption
	var id, organizationID pgtype.UUID
	var sourceTypeID pgtype.Int4
	var createdAt, updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &organizationID, &model.Manufacturer, &model.Model, &model.DeviceType, &sourceTypeID, &model.IsActive, &createdAt, &updatedAt); err != nil {
		return DeviceModelOption{}, fmt.Errorf("scan device model: %w", err)
	}
	model.ID = uuidString(id)
	model.OrganizationID = uuidString(organizationID)
	model.SourceTypeID = int32Pointer(sourceTypeID)
	model.CreatedAt = createdAt.Time
	model.UpdatedAt = updatedAt.Time
	return model, nil
}

func (s *Service) getRegisterMetadataInTx(ctx context.Context, tx pgx.Tx, organizationID, plantID, deviceID pgtype.UUID, addressKey string) (DeviceRegisterMetadata, error) {
	row := tx.QueryRow(ctx, `
SELECT id, organization_id, plant_id, device_id, address_key, display_name, unit, data_type,
       scale, value_offset, decimals, is_enabled, notes, created_at, updated_at
FROM device_register_metadata
WHERE organization_id=$1 AND plant_id=$2 AND device_id=$3 AND address_key=$4`, organizationID, plantID, deviceID, addressKey)
	return scanDeviceRegisterMetadata(row)
}

type registerMetadataScanner interface{ Scan(...any) error }

func scanDeviceRegisterMetadata(row registerMetadataScanner) (DeviceRegisterMetadata, error) {
	var item DeviceRegisterMetadata
	var id, organizationID, plantID, deviceID pgtype.UUID
	var createdAt, updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &organizationID, &plantID, &deviceID, &item.AddressKey, &item.DisplayName, &item.Unit, &item.DataType, &item.Scale, &item.Offset, &item.Decimals, &item.IsEnabled, &item.Notes, &createdAt, &updatedAt); err != nil {
		return DeviceRegisterMetadata{}, fmt.Errorf("scan register metadata: %w", err)
	}
	item.ID = uuidString(id)
	item.OrganizationID = uuidString(organizationID)
	item.PlantID = uuidString(plantID)
	item.DeviceID = uuidString(deviceID)
	item.CreatedAt = createdAt.Time
	item.UpdatedAt = updatedAt.Time
	return item, nil
}

func getDeviceModelRegisterMetadataInTx(ctx context.Context, tx pgx.Tx, organizationID, modelID pgtype.UUID, addressKey string) (DeviceModelRegisterMetadata, error) {
	row := tx.QueryRow(ctx, `
SELECT id, organization_id, device_model_id, address_key, display_name, unit, data_type,
       scale, value_offset, decimals, is_enabled, notes, created_at, updated_at
FROM device_model_register_metadata
WHERE organization_id=$1 AND device_model_id=$2 AND address_key=$3`, organizationID, modelID, addressKey)
	return scanDeviceModelRegisterMetadata(row)
}

func scanDeviceModelRegisterMetadata(row registerMetadataScanner) (DeviceModelRegisterMetadata, error) {
	var item DeviceModelRegisterMetadata
	var id, organizationID, modelID pgtype.UUID
	var createdAt, updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &organizationID, &modelID, &item.AddressKey, &item.DisplayName, &item.Unit, &item.DataType, &item.Scale, &item.Offset, &item.Decimals, &item.IsEnabled, &item.Notes, &createdAt, &updatedAt); err != nil {
		return DeviceModelRegisterMetadata{}, fmt.Errorf("scan model register metadata: %w", err)
	}
	item.ID = uuidString(id)
	item.OrganizationID = uuidString(organizationID)
	item.DeviceModelID = uuidString(modelID)
	item.CreatedAt = createdAt.Time
	item.UpdatedAt = updatedAt.Time
	return item, nil
}

func deviceFromFields(
	id, organizationID, plantID pgtype.UUID, externalID, name string,
	deviceModelID pgtype.UUID, manufacturer, model, deviceType string,
	sourceTypeID pgtype.Int4, isActive bool, createdAt, updatedAt pgtype.Timestamptz,
) Device {
	return Device{
		ID: uuidString(id), OrganizationID: uuidString(organizationID), PlantID: uuidString(plantID),
		ExternalID: externalID, Name: name, DeviceModelID: uuidString(deviceModelID),
		Manufacturer: manufacturer, Model: model, DeviceType: deviceType,
		SourceTypeID: int32Pointer(sourceTypeID), IsActive: isActive,
		CreatedAt: createdAt.Time, UpdatedAt: updatedAt.Time,
	}
}

func int32Pointer(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	copy := value.Int32
	return &copy
}
