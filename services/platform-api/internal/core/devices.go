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
	ID                  string    `json:"id"`
	OrganizationID      string    `json:"organizationId"`
	PlantID             string    `json:"plantId"`
	ExternalID          string    `json:"externalId"`
	Name                string    `json:"name"`
	DeviceModelID       string    `json:"deviceModelId"`
	Manufacturer        string    `json:"manufacturer"`
	Model               string    `json:"model"`
	DeviceType          string    `json:"deviceType"`
	SourceTypeID        *int32    `json:"sourceTypeId"`
	ModbusHost          *string   `json:"modbusHost"`
	ModbusPort          *int32    `json:"modbusPort"`
	ModbusUnitID        int32     `json:"modbusUnitId"`
	PollIntervalSeconds int32     `json:"pollIntervalSeconds"`
	IsActive            bool      `json:"isActive"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

// UpdateDeviceInput and CreateDeviceInput are admin-facing (httpapi) inputs
// only -- v1/v2 ingestion auto-onboarding writes device rows through its own
// separate path (internal/ingestion, dbgen OnboardDevice), never through
// these, so dropping Manufacturer/DeviceType/SourceTypeID/SerialNumber here
// does not touch ingestion at all.
type UpdateDeviceInput struct {
	Name                string
	DeviceModelID       string
	ModbusHost          string
	ModbusPort          *int32
	ModbusUnitID        int32
	PollIntervalSeconds int32
	IsActive            bool
}

type CreateDeviceInput struct {
	ExternalID          string
	Name                string
	DeviceModelID       string
	ModbusHost          string
	ModbusPort          *int32
	ModbusUnitID        int32
	PollIntervalSeconds int32
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
	// Modbus register-decode config, used by a middleware's computed
	// config snapshot -- see core/middleware_config.go's buildConfigSnapshot.
	// Nil/empty means this row is display-metadata only, not pollable.
	ModbusFunctionCode *int32    `json:"modbusFunctionCode"`
	ModbusRegister     *int32    `json:"modbusRegister"`
	ModbusWordOrder    *string   `json:"modbusWordOrder"`
	ModbusDataType     *string   `json:"modbusDataType"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
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

// UpdateDeviceModelRegisterMetadataInput embeds the device-level input so
// the shared fields keep going through the same validateRegisterMetadata
// path; the Modbus fields are model-only (no per-device register decode
// override -- a device's registers are determined by its model/firmware).
type UpdateDeviceModelRegisterMetadataInput struct {
	UpdateDeviceRegisterMetadataInput
	ModbusFunctionCode *int32
	ModbusRegister     *int32
	ModbusWordOrder    string
	ModbusDataType     string
}

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
	rows, err := s.pool.Query(ctx, `
SELECT d.id, d.organization_id, d.plant_id, d.external_id, d.name, d.device_model_id,
       dm.manufacturer, dm.model, dm.device_type, dm.source_type_id,
       d.modbus_host, d.modbus_port, d.modbus_unit_id, d.poll_interval_seconds,
       d.is_active, d.created_at, d.updated_at
FROM device d
JOIN device_model dm ON dm.id = d.device_model_id
WHERE d.organization_id=$1 AND d.plant_id=$2
ORDER BY d.name, d.external_id, d.id
LIMIT 500`, plant.OrganizationID, plant.ID)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()
	devices := []Device{}
	for rows.Next() {
		device, err := scanDeviceRow(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
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
	s.recomputeAndPushMiddlewaresForDeviceModel(context.WithoutCancel(ctx), id)
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
       scale, value_offset, decimals, is_enabled, notes,
       modbus_function_code, modbus_register, modbus_word_order, modbus_data_type, created_at, updated_at
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
	if err = validateRegisterMetadata(&input.UpdateDeviceRegisterMetadataInput); err != nil {
		return DeviceModelRegisterMetadata{}, err
	}
	input.ModbusWordOrder = strings.TrimSpace(input.ModbusWordOrder)
	input.ModbusDataType = strings.TrimSpace(input.ModbusDataType)
	if err = validateModbusRegisterFields(input.ModbusFunctionCode, input.ModbusRegister, input.ModbusWordOrder, input.ModbusDataType); err != nil {
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
    scale, value_offset, decimals, is_enabled, notes,
    modbus_function_code, modbus_register, modbus_word_order, modbus_data_type
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
ON CONFLICT (organization_id, device_model_id, address_key) DO UPDATE
SET display_name=EXCLUDED.display_name, unit=EXCLUDED.unit, data_type=EXCLUDED.data_type,
    scale=EXCLUDED.scale, value_offset=EXCLUDED.value_offset, decimals=EXCLUDED.decimals,
    is_enabled=EXCLUDED.is_enabled, notes=EXCLUDED.notes,
    modbus_function_code=EXCLUDED.modbus_function_code, modbus_register=EXCLUDED.modbus_register,
    modbus_word_order=EXCLUDED.modbus_word_order, modbus_data_type=EXCLUDED.modbus_data_type, updated_at=now()
RETURNING id, organization_id, device_model_id, address_key, display_name, unit, data_type,
          scale, value_offset, decimals, is_enabled, notes,
          modbus_function_code, modbus_register, modbus_word_order, modbus_data_type, created_at, updated_at`,
		id, organizationID, modelUUID, input.AddressKey, input.DisplayName, input.Unit, input.DataType,
		input.Scale, input.Offset, input.Decimals, input.IsEnabled, input.Notes,
		nullableInt32(input.ModbusFunctionCode), nullableInt32(input.ModbusRegister),
		nullableText(input.ModbusWordOrder), nullableText(input.ModbusDataType))
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
	s.recomputeAndPushMiddlewaresForDeviceModel(context.WithoutCancel(ctx), modelUUID)
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
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete model register metadata: %w", err)
	}
	s.recomputeAndPushMiddlewaresForDeviceModel(context.WithoutCancel(ctx), modelUUID)
	return nil
}

func (s *Service) CreateDevice(ctx context.Context, principal auth.Principal, plantID string, input CreateDeviceInput, sourceIP *netip.Addr) (Device, error) {
	plantUUID, err := parseUUID(plantID)
	if err != nil {
		return Device{}, ErrNotFound
	}
	deviceModelUUID, err := parseUUID(input.DeviceModelID)
	if err != nil {
		return Device{}, ErrInvalid
	}
	input.ExternalID = strings.TrimSpace(input.ExternalID)
	input.Name = strings.TrimSpace(input.Name)
	input.ModbusHost = strings.TrimSpace(input.ModbusHost)
	if input.ExternalID == "" || input.Name == "" || len(input.ExternalID) > 200 || len(input.Name) > 200 {
		return Device{}, ErrInvalid
	}
	if err = validateDeviceModbusFields(input.ModbusHost, input.ModbusPort, input.ModbusUnitID, input.PollIntervalSeconds); err != nil {
		return Device{}, err
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
	model, err := getDeviceModelRow(ctx, tx, deviceModelUUID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && model.OrganizationID != uuidString(plant.OrganizationID)) {
		return Device{}, ErrInvalid
	}
	if err != nil {
		return Device{}, err
	}
	deviceID, err := newUUID()
	if err != nil {
		return Device{}, err
	}
	metadata, _ := json.Marshal(map[string]any{"source": "MANUAL"})
	row := tx.QueryRow(ctx, `
WITH inserted AS (
    INSERT INTO device (id, organization_id, plant_id, device_model_id, external_id, name, modbus_host, modbus_port, modbus_unit_id, poll_interval_seconds, source_metadata)
    VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
    RETURNING *
)
SELECT inserted.id, inserted.organization_id, inserted.plant_id, inserted.external_id, inserted.name, inserted.device_model_id,
       dm.manufacturer, dm.model, dm.device_type, dm.source_type_id,
       inserted.modbus_host, inserted.modbus_port, inserted.modbus_unit_id, inserted.poll_interval_seconds,
       inserted.is_active, inserted.created_at, inserted.updated_at
FROM inserted JOIN device_model dm ON dm.id = inserted.device_model_id`,
		deviceID, plant.OrganizationID, plant.ID, deviceModelUUID, input.ExternalID, input.Name,
		nullableText(input.ModbusHost), nullableInt32(input.ModbusPort), input.ModbusUnitID, input.PollIntervalSeconds, metadata)
	device, err := scanDeviceRow(row)
	if err != nil {
		return Device{}, mapWriteError(err)
	}
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
	s.recomputeAndPushMiddlewareForPlant(context.WithoutCancel(ctx), plant.OrganizationID, plantUUID)
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
	deviceModelUUID, err := parseUUID(input.DeviceModelID)
	if err != nil {
		return Device{}, ErrInvalid
	}
	input.Name = strings.TrimSpace(input.Name)
	input.ModbusHost = strings.TrimSpace(input.ModbusHost)
	if input.Name == "" || len(input.Name) > 200 {
		return Device{}, ErrInvalid
	}
	if err = validateDeviceModbusFields(input.ModbusHost, input.ModbusPort, input.ModbusUnitID, input.PollIntervalSeconds); err != nil {
		return Device{}, err
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
	current, err := getDeviceForUpdate(ctx, tx, principal.UserID, plantUUID, deviceUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	if err != nil {
		return Device{}, fmt.Errorf("lock device: %w", err)
	}
	model, err := getDeviceModelRow(ctx, tx, deviceModelUUID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && model.OrganizationID != current.OrganizationID) {
		return Device{}, ErrInvalid
	}
	if err != nil {
		return Device{}, err
	}
	row := tx.QueryRow(ctx, `
WITH updated AS (
    UPDATE device
    SET name=$2, device_model_id=$3, modbus_host=$4, modbus_port=$5, modbus_unit_id=$6, poll_interval_seconds=$7, is_active=$8, updated_at=now()
    WHERE id=$1
    RETURNING *
)
SELECT updated.id, updated.organization_id, updated.plant_id, updated.external_id, updated.name, updated.device_model_id,
       dm.manufacturer, dm.model, dm.device_type, dm.source_type_id,
       updated.modbus_host, updated.modbus_port, updated.modbus_unit_id, updated.poll_interval_seconds,
       updated.is_active, updated.created_at, updated.updated_at
FROM updated JOIN device_model dm ON dm.id = updated.device_model_id`,
		deviceUUID, input.Name, deviceModelUUID, nullableText(input.ModbusHost), nullableInt32(input.ModbusPort),
		input.ModbusUnitID, input.PollIntervalSeconds, input.IsActive)
	device, err := scanDeviceRow(row)
	if err != nil {
		return Device{}, mapWriteError(err)
	}
	before, _ := json.Marshal(current)
	after, _ := json.Marshal(device)
	organizationID, _ := parseUUID(current.OrganizationID)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: organizationID, ActorUserID: principal.UserID,
		Action: "device.updated", TargetType: "device", TargetID: deviceUUID,
		BeforeData: before, AfterData: after, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return Device{}, fmt.Errorf("audit device update: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Device{}, fmt.Errorf("commit update device: %w", err)
	}
	s.recomputeAndPushMiddlewareForPlant(context.WithoutCancel(ctx), organizationID, plantUUID)
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

// validateModbusRegisterFields enforces "all four set together, or none" --
// a register-metadata row is either display-only (no Modbus fields) or a
// real pollable register (all four present and valid). See
// core/middleware_config.go's buildConfigSnapshot for where these feed the
// wire config pushed to a middleware gateway.
func validateModbusRegisterFields(functionCode, register *int32, wordOrder, dataType string) error {
	set := functionCode != nil || register != nil || wordOrder != "" || dataType != ""
	if !set {
		return nil
	}
	if functionCode == nil || register == nil || dataType == "" {
		return ErrInvalid
	}
	if *functionCode != 3 && *functionCode != 4 {
		return ErrInvalid
	}
	if *register < 0 || *register > 65535 {
		return ErrInvalid
	}
	if wordOrder != "" && wordOrder != "HIGH_LOW" && wordOrder != "LOW_HIGH" {
		return ErrInvalid
	}
	switch dataType {
	case "U16", "I16", "U32", "I32", "U64", "FLOAT32":
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
       scale, value_offset, decimals, is_enabled, notes,
       modbus_function_code, modbus_register, modbus_word_order, modbus_data_type, created_at, updated_at
FROM device_model_register_metadata
WHERE organization_id=$1 AND device_model_id=$2 AND address_key=$3`, organizationID, modelID, addressKey)
	return scanDeviceModelRegisterMetadata(row)
}

func scanDeviceModelRegisterMetadata(row registerMetadataScanner) (DeviceModelRegisterMetadata, error) {
	var item DeviceModelRegisterMetadata
	var id, organizationID, modelID pgtype.UUID
	var modbusFunctionCode, modbusRegister pgtype.Int4
	var modbusWordOrder, modbusDataType pgtype.Text
	var createdAt, updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &organizationID, &modelID, &item.AddressKey, &item.DisplayName, &item.Unit, &item.DataType, &item.Scale, &item.Offset, &item.Decimals, &item.IsEnabled, &item.Notes,
		&modbusFunctionCode, &modbusRegister, &modbusWordOrder, &modbusDataType, &createdAt, &updatedAt); err != nil {
		return DeviceModelRegisterMetadata{}, fmt.Errorf("scan model register metadata: %w", err)
	}
	item.ID = uuidString(id)
	item.OrganizationID = uuidString(organizationID)
	item.DeviceModelID = uuidString(modelID)
	item.ModbusFunctionCode = int32Pointer(modbusFunctionCode)
	item.ModbusRegister = int32Pointer(modbusRegister)
	item.ModbusWordOrder = textPointer(modbusWordOrder)
	item.ModbusDataType = textPointer(modbusDataType)
	item.CreatedAt = createdAt.Time
	item.UpdatedAt = updatedAt.Time
	return item, nil
}

// scanDeviceRow scans the column order shared by Devices, CreateDevice, and
// UpdateDevice's SQL: device joined with its device_model.
func scanDeviceRow(row registerMetadataScanner) (Device, error) {
	var d Device
	var id, organizationID, plantID, deviceModelID pgtype.UUID
	var sourceTypeID pgtype.Int4
	var modbusHost pgtype.Text
	var modbusPort pgtype.Int4
	var createdAt, updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &organizationID, &plantID, &d.ExternalID, &d.Name, &deviceModelID,
		&d.Manufacturer, &d.Model, &d.DeviceType, &sourceTypeID,
		&modbusHost, &modbusPort, &d.ModbusUnitID, &d.PollIntervalSeconds,
		&d.IsActive, &createdAt, &updatedAt); err != nil {
		return Device{}, fmt.Errorf("scan device: %w", err)
	}
	d.ID = uuidString(id)
	d.OrganizationID = uuidString(organizationID)
	d.PlantID = uuidString(plantID)
	d.DeviceModelID = uuidString(deviceModelID)
	d.SourceTypeID = int32Pointer(sourceTypeID)
	d.ModbusHost = textPointer(modbusHost)
	d.ModbusPort = int32Pointer(modbusPort)
	d.CreatedAt = createdAt.Time
	d.UpdatedAt = updatedAt.Time
	return d, nil
}

// getDeviceForUpdate locks the device row (FOR UPDATE) and authorizes the
// caller for the "update"/"device" permission, mirroring
// authorizedDeviceScopeQuery's scoping rules.
func getDeviceForUpdate(ctx context.Context, tx pgx.Tx, userID, plantID, deviceID pgtype.UUID) (Device, error) {
	row := tx.QueryRow(ctx, `
SELECT d.id, d.organization_id, d.plant_id, d.external_id, d.name, d.device_model_id,
       dm.manufacturer, dm.model, dm.device_type, dm.source_type_id,
       d.modbus_host, d.modbus_port, d.modbus_unit_id, d.poll_interval_seconds,
       d.is_active, d.created_at, d.updated_at
FROM device d
JOIN device_model dm ON dm.id = d.device_model_id
WHERE d.id=$1 AND d.plant_id=$2
  AND EXISTS (
      SELECT 1 FROM user_role ur
      JOIN role r ON r.id = ur.role_id
      JOIN role_permission rp ON rp.role_id = ur.role_id
      JOIN permission pm ON pm.id = rp.permission_id
      WHERE ur.user_id = $3
        AND pm.action = 'update' AND pm.resource_type = 'device'
        AND (r.organization_id IS NULL OR r.organization_id = ur.organization_id)
        AND (rp.organization_id IS NULL OR rp.organization_id = ur.organization_id)
        AND (ur.organization_id IS NULL OR ur.organization_id = d.organization_id)
        AND (ur.plant_id IS NULL OR ur.plant_id = d.plant_id)
  )
LIMIT 1
FOR UPDATE OF d`, deviceID, plantID, userID)
	return scanDeviceRow(row)
}

func validateDeviceModbusFields(host string, port *int32, unitID, pollIntervalSeconds int32) error {
	if len(host) > 255 {
		return ErrInvalid
	}
	if (host == "") != (port == nil) {
		return ErrInvalid
	}
	if port != nil && (*port < 1 || *port > 65535) {
		return ErrInvalid
	}
	if unitID < 0 || unitID > 255 {
		return ErrInvalid
	}
	if pollIntervalSeconds < 1 || pollIntervalSeconds > 3600 {
		return ErrInvalid
	}
	return nil
}

func nullableText(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func nullableInt32(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}

func int32Pointer(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	copy := value.Int32
	return &copy
}
