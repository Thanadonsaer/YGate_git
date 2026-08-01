package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/database/dbgen"
)

var (
	ErrMiddlewareNotFound   = errors.New("middleware gateway not found")
	ErrMiddlewareInvalid    = errors.New("invalid middleware gateway data")
	ErrMiddlewareConflict   = errors.New("middleware gateway already exists")
	ErrMiddlewareOffline    = errors.New("middleware gateway is offline")
	ErrMiddlewareCommandNAK = errors.New("middleware gateway rejected the command")
)

// MiddlewareGateway is the admin-facing view of a middleware_client row
// extended with the gateway-identity fields this feature adds.
type MiddlewareGateway struct {
	ID                   string     `json:"id"`
	OrganizationID       string     `json:"organizationId"`
	OrganizationName     string     `json:"organizationName"`
	Name                 string     `json:"name"`
	SiteName             string     `json:"siteName"`
	KeyPrefix            string     `json:"keyPrefix"`
	IsActive             bool       `json:"isActive"`
	IsOnline             bool       `json:"isOnline"`
	ConfigVersion        int64      `json:"configVersion"`
	ConfigAppliedVersion int64      `json:"configAppliedVersion"`
	LastSeenAt           *time.Time `json:"lastSeenAt"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
}

type CreatedMiddlewareGateway struct {
	MiddlewareGateway
	APIKey string `json:"apiKey"`
}

type CreateMiddlewareInput struct {
	OrganizationID string
	Name           string
	SiteName       string
}

type UpdateMiddlewareInput struct {
	Name     string
	SiteName string
	IsActive bool
}

// The config snapshot shape is intentionally identical, field for field, to
// modbus-api-middleware/internal/domain/configuration.go's Brand/DeviceSet/
// Address/ConnectionConfig/Plant structs. It is stored as-is in
// middleware_client.config_snapshot and pushed to the gateway as-is over
// the realtime WebSocket -- no server-side translation step, so the
// middleware's existing Save*/domain decode logic can consume it directly.

type MiddlewareAddress struct {
	AddressID    int64   `json:"addressId"`
	DeviceSetID  int64   `json:"deviceSetId"`
	FunctionCode int     `json:"functionCode"`
	Register     int     `json:"register"`
	Description  string  `json:"description"`
	CanonicalKey string  `json:"canonicalKey,omitempty"`
	SourceTag    string  `json:"sourceTag,omitempty"`
	Factor       float64 `json:"factor"`
	Offset       float64 `json:"offset,omitempty"`
	DataType     string  `json:"dataType"`
	Length       int     `json:"length,omitempty"`
	WordOrder    string  `json:"wordOrder,omitempty"`
	SourceUnit   string  `json:"sourceUnit,omitempty"`
	Remark       string  `json:"remark,omitempty"`
	Enabled      bool    `json:"enabled"`
}

type MiddlewareDeviceSet struct {
	DeviceSetID  int64               `json:"deviceSetId"`
	BrandID      int64               `json:"brandId,omitempty"`
	BrandName    string              `json:"brandName,omitempty"`
	DevTypeID    int                 `json:"devTypeId,omitempty"`
	DevType      string              `json:"devType"`
	DevModel     string              `json:"devModel"`
	AddressMode  string              `json:"addressMode,omitempty"`
	ByteOrder    string              `json:"byteOrder,omitempty"`
	WordOrder    string              `json:"wordOrder,omitempty"`
	MaxBlockSize int                 `json:"maxBlockSize,omitempty"`
	Addresses    []MiddlewareAddress `json:"addresses"`
}

type MiddlewareBrand struct {
	BrandID          int64  `json:"brandId"`
	BrandName        string `json:"brandName"`
	BrandDescription string `json:"brandDescription,omitempty"`
}

type MiddlewareConnection struct {
	ConnectionID   int64  `json:"connectionId"`
	ConnectionName string `json:"connectionName"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	UnitID         int    `json:"unitId,omitempty"`
	DeviceSetID    int64  `json:"deviceSetId"`
	DevDn          string `json:"devDn,omitempty"`
	DeviceName     string `json:"deviceName,omitempty"`
	PlantCode      string `json:"plantCode,omitempty"`
	PlantName      string `json:"plantName,omitempty"`
	Enabled        bool   `json:"enabled"`
}

type MiddlewarePlant struct {
	PlantCode string `json:"plantCode"`
	PlantName string `json:"plantName"`
}

type MiddlewareConfigSnapshot struct {
	Version     int64                  `json:"version"`
	Brands      []MiddlewareBrand      `json:"brands"`
	DeviceSets  []MiddlewareDeviceSet  `json:"deviceSets"`
	Connections []MiddlewareConnection `json:"connections"`
	Plants      []MiddlewarePlant      `json:"plants"`
}

type ImportMiddlewareConfigResult struct {
	DeviceModelsCreated      int                         `json:"deviceModelsCreated"`
	DeviceModelsReused       int                         `json:"deviceModelsReused"`
	RegisterMetadataUpserted int                         `json:"registerMetadataUpserted"`
	ConnectionsFound         []ImportedConnectionSummary `json:"connectionsFound"`
}

type ImportedConnectionSummary struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	UnitID      int    `json:"unitId"`
	DeviceModel string `json:"deviceModel"`
}

// normalizeModbusDataType mirrors modbus-api-middleware/internal/decoder.NormalizeDataType --
// duplicated here (not imported: different Go module) so a legacy
// Middleware's free-text data types (SHORT/USHORT/FLOAT/...) map onto the
// canonical MIDDLEWARE_DATA_TYPES the rest of this system already uses.
func normalizeModbusDataType(dataType string) string {
	switch strings.ToUpper(strings.TrimSpace(dataType)) {
	case "SHORT", "INT16":
		return "I16"
	case "USHORT", "UINT16":
		return "U16"
	case "LONG", "SLONG", "INT32", "S32", "SW_INT", "SMA_INT32":
		return "I32"
	case "ULONG", "DWORD", "UINT32", "SW_UINT", "SMA_UINT32":
		return "U32"
	case "UINT64", "SMA_UINT64":
		return "U64"
	case "FLOAT", "SW_FLOAT":
		return "FLOAT32"
	default:
		return strings.ToUpper(strings.TrimSpace(dataType))
	}
}

// importFromSnapshot upserts DeviceModel + DeviceModelRegisterMetadata rows
// from an already-decoded MiddlewareConfigSnapshot pulled live from a
// Middleware's local config (see ImportMiddlewareConfig). Idempotent: an
// existing DeviceModel (matched by manufacturer/deviceType/model) is
// reused, never duplicated; register metadata is upserted by addressKey
// the same way the Register Metadata page's own PUT already works.
// Connections are only summarized, never turned into Device rows -- Plant
// assignment stays a human decision (see the design spec).
func (s *Service) importFromSnapshot(ctx context.Context, principal auth.Principal, organizationID string, snapshot MiddlewareConfigSnapshot, sourceIP *netip.Addr) (ImportMiddlewareConfigResult, error) {
	var result ImportMiddlewareConfigResult

	existing, err := s.DeviceModels(ctx, principal)
	if err != nil {
		return result, fmt.Errorf("list existing device models: %w", err)
	}
	type modelKey struct{ manufacturer, deviceType, model string }
	byKey := make(map[modelKey]string, len(existing))
	for _, m := range existing {
		byKey[modelKey{m.Manufacturer, m.DeviceType, m.Model}] = m.ID
	}

	brandNames := make(map[int64]string, len(snapshot.Brands))
	for _, b := range snapshot.Brands {
		brandNames[b.BrandID] = b.BrandName
	}

	resolvedModelName := make(map[int64]string, len(snapshot.DeviceSets))
	for _, ds := range snapshot.DeviceSets {
		manufacturer := brandNames[ds.BrandID]
		if manufacturer == "" {
			manufacturer = ds.BrandName
		}
		key := modelKey{manufacturer, ds.DevType, ds.DevModel}
		modelID, ok := byKey[key]
		if ok {
			result.DeviceModelsReused++
		} else {
			created, err := s.CreateDeviceModel(ctx, principal, DeviceModelInput{
				OrganizationID: organizationID, Manufacturer: manufacturer, DeviceType: ds.DevType, Model: ds.DevModel, IsActive: true,
			}, sourceIP)
			if err != nil {
				return result, fmt.Errorf("create device model %s/%s/%s: %w", manufacturer, ds.DevType, ds.DevModel, err)
			}
			modelID = created.ID
			byKey[key] = modelID
			result.DeviceModelsCreated++
		}
		resolvedModelName[ds.DeviceSetID] = fmt.Sprintf("%s / %s / %s", manufacturer, ds.DevType, ds.DevModel)

		for _, addr := range ds.Addresses {
			addressKey := strings.TrimSpace(addr.CanonicalKey)
			if addressKey == "" {
				addressKey = fmt.Sprintf("%d:%d", addr.FunctionCode, addr.Register)
			}
			functionCode := int32(addr.FunctionCode)
			register := int32(addr.Register)
			_, err := s.SetDeviceModelRegisterMetadata(ctx, principal, modelID, UpdateDeviceModelRegisterMetadataInput{
				UpdateDeviceRegisterMetadataInput: UpdateDeviceRegisterMetadataInput{
					AddressKey: addressKey, DisplayName: addr.Description, DataType: "number", Scale: addr.Factor, Offset: addr.Offset, Decimals: 2, IsEnabled: true,
				},
				ModbusFunctionCode: &functionCode,
				ModbusRegister:     &register,
				ModbusWordOrder:    addr.WordOrder,
				ModbusDataType:     normalizeModbusDataType(addr.DataType),
			}, sourceIP)
			if err != nil {
				return result, fmt.Errorf("upsert register metadata %s: %w", addressKey, err)
			}
			result.RegisterMetadataUpserted++
		}
	}

	for _, conn := range snapshot.Connections {
		result.ConnectionsFound = append(result.ConnectionsFound, ImportedConnectionSummary{
			Host: conn.Host, Port: conn.Port, UnitID: conn.UnitID, DeviceModel: resolvedModelName[conn.DeviceSetID],
		})
	}
	return result, nil
}

type ConfigAckInput struct {
	Version int64
	Status  string // APPLIED | FAILED
	Reason  string
}

func (s *Service) Middlewares(ctx context.Context, principal auth.Principal) ([]MiddlewareGateway, error) {
	allowed, err := s.queries.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "read", ResourceType: "middleware_client"})
	if err != nil {
		return nil, fmt.Errorf("check middleware read permission: %w", err)
	}
	if !allowed {
		return nil, ErrForbidden
	}
	rows, err := s.pool.Query(ctx, `
SELECT mc.id, mc.organization_id, o.name, mc.name, mc.site_name, mc.key_prefix,
       mc.is_active, mc.config_version, mc.config_applied_version, mc.last_seen_at, mc.created_at, mc.updated_at
FROM middleware_client mc
JOIN organization o ON o.id = mc.organization_id
WHERE EXISTS (
    SELECT 1 FROM user_role ur
    JOIN role r ON r.id = ur.role_id
    JOIN role_permission rp ON rp.role_id = ur.role_id
    JOIN permission pm ON pm.id = rp.permission_id
    WHERE ur.user_id = $1
      AND pm.action = 'read' AND pm.resource_type = 'middleware_client'
      AND (r.organization_id IS NULL OR r.organization_id = ur.organization_id)
      AND (rp.organization_id IS NULL OR rp.organization_id = ur.organization_id)
      AND ur.plant_id IS NULL
      AND (ur.organization_id IS NULL OR ur.organization_id = mc.organization_id)
)
ORDER BY o.name, mc.name
LIMIT 200`, principal.UserID)
	if err != nil {
		return nil, fmt.Errorf("list middlewares: %w", err)
	}
	defer rows.Close()
	gateways := []MiddlewareGateway{}
	for rows.Next() {
		gateway, err := scanMiddlewareGateway(rows)
		if err != nil {
			return nil, err
		}
		gateway.IsOnline = s.hub.IsOnline(gateway.ID)
		gateways = append(gateways, gateway)
	}
	return gateways, rows.Err()
}

func (s *Service) CreateMiddleware(ctx context.Context, principal auth.Principal, input CreateMiddlewareInput, sourceIP *netip.Addr) (CreatedMiddlewareGateway, error) {
	organizationID := principal.OrganizationID
	var err error
	if strings.TrimSpace(input.OrganizationID) != "" {
		organizationID, err = parseUUID(input.OrganizationID)
		if err != nil {
			return CreatedMiddlewareGateway{}, ErrMiddlewareInvalid
		}
	}
	input.Name = strings.TrimSpace(input.Name)
	input.SiteName = strings.TrimSpace(input.SiteName)
	if input.Name == "" || len(input.Name) > 200 || len(input.SiteName) > 200 || !organizationID.Valid {
		return CreatedMiddlewareGateway{}, ErrMiddlewareInvalid
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return CreatedMiddlewareGateway{}, fmt.Errorf("generate middleware api key: %w", err)
	}
	apiKey := "ygm_" + base64.RawURLEncoding.EncodeToString(secret)
	keyHash := sha256.Sum256([]byte(apiKey))
	id, err := newUUID()
	if err != nil {
		return CreatedMiddlewareGateway{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return CreatedMiddlewareGateway{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CreatedMiddlewareGateway{}, fmt.Errorf("begin create middleware: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	if err = s.requireOrganizationPermission(ctx, q, principal, "create", "middleware_client", organizationID); err != nil {
		return CreatedMiddlewareGateway{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO middleware_client(id, organization_id, name, site_name, key_prefix, key_hash) VALUES($1,$2,$3,$4,$5,$6)`,
		id, organizationID, input.Name, input.SiteName, apiKey[:12], keyHash[:]); err != nil {
		return CreatedMiddlewareGateway{}, mapMiddlewareWriteError(err)
	}
	gateway, err := s.getMiddlewareInTx(ctx, tx, id)
	if err != nil {
		return CreatedMiddlewareGateway{}, err
	}
	after, _ := json.Marshal(gateway)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "middleware_client.created",
		TargetType: "middleware_client", TargetID: id, AfterData: after, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return CreatedMiddlewareGateway{}, fmt.Errorf("audit middleware create: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return CreatedMiddlewareGateway{}, fmt.Errorf("commit create middleware: %w", err)
	}
	return CreatedMiddlewareGateway{MiddlewareGateway: gateway, APIKey: apiKey}, nil
}

func (s *Service) UpdateMiddleware(ctx context.Context, principal auth.Principal, middlewareID string, input UpdateMiddlewareInput, sourceIP *netip.Addr) (MiddlewareGateway, error) {
	id, err := parseUUID(middlewareID)
	if err != nil {
		return MiddlewareGateway{}, ErrMiddlewareNotFound
	}
	input.Name = strings.TrimSpace(input.Name)
	input.SiteName = strings.TrimSpace(input.SiteName)
	if input.Name == "" || len(input.Name) > 200 || len(input.SiteName) > 200 {
		return MiddlewareGateway{}, ErrMiddlewareInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MiddlewareGateway{}, fmt.Errorf("begin update middleware: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	before, err := s.getMiddlewareInTx(ctx, tx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return MiddlewareGateway{}, ErrMiddlewareNotFound
	}
	if err != nil {
		return MiddlewareGateway{}, err
	}
	organizationID, err := parseUUID(before.OrganizationID)
	if err != nil {
		return MiddlewareGateway{}, ErrMiddlewareNotFound
	}
	if err = s.requireOrganizationPermission(ctx, q, principal, "update", "middleware_client", organizationID); err != nil {
		return MiddlewareGateway{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE middleware_client SET name=$2, site_name=$3, is_active=$4, updated_at=now() WHERE id=$1`,
		id, input.Name, input.SiteName, input.IsActive); err != nil {
		return MiddlewareGateway{}, mapMiddlewareWriteError(err)
	}
	after, err := s.getMiddlewareInTx(ctx, tx, id)
	if err != nil {
		return MiddlewareGateway{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return MiddlewareGateway{}, err
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "middleware_client.updated",
		TargetType: "middleware_client", TargetID: id, BeforeData: beforeJSON, AfterData: afterJSON,
		SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return MiddlewareGateway{}, fmt.Errorf("audit middleware update: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return MiddlewareGateway{}, fmt.Errorf("commit update middleware: %w", err)
	}
	return after, nil
}

// MiddlewareConfig returns the last computed-and-pushed config snapshot for
// middlewareID (read-only -- see buildConfigSnapshot for how it's produced).
func (s *Service) MiddlewareConfig(ctx context.Context, principal auth.Principal, middlewareID string) (MiddlewareConfigSnapshot, error) {
	id, err := parseUUID(middlewareID)
	if err != nil {
		return MiddlewareConfigSnapshot{}, ErrMiddlewareNotFound
	}
	allowed, err := s.queries.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "read", ResourceType: "middleware_config"})
	if err != nil {
		return MiddlewareConfigSnapshot{}, fmt.Errorf("check middleware config read permission: %w", err)
	}
	if !allowed {
		return MiddlewareConfigSnapshot{}, ErrForbidden
	}
	var raw []byte
	var version int64
	err = s.pool.QueryRow(ctx, `SELECT config_snapshot, config_version FROM middleware_client WHERE id=$1`, id).Scan(&raw, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return MiddlewareConfigSnapshot{}, ErrMiddlewareNotFound
	}
	if err != nil {
		return MiddlewareConfigSnapshot{}, fmt.Errorf("get middleware config: %w", err)
	}
	snapshot := MiddlewareConfigSnapshot{Version: version, Brands: []MiddlewareBrand{}, DeviceSets: []MiddlewareDeviceSet{}, Connections: []MiddlewareConnection{}, Plants: []MiddlewarePlant{}}
	if len(raw) > 0 {
		if err = json.Unmarshal(raw, &snapshot); err != nil {
			return MiddlewareConfigSnapshot{}, fmt.Errorf("decode stored middleware config: %w", err)
		}
	}
	snapshot.Version = version
	return snapshot, nil
}

// RunMiddlewareCommand relays a connect-test/read-now command for deviceID
// to whichever Middleware currently serves its Plant, and waits (up to 15s)
// for the result.
func (s *Service) RunMiddlewareCommand(ctx context.Context, principal auth.Principal, deviceID, kind string) (json.RawMessage, error) {
	devUUID, err := parseUUID(deviceID)
	if err != nil {
		return nil, ErrMiddlewareNotFound
	}
	allowed, err := s.queries.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "read", ResourceType: "middleware_config"})
	if err != nil {
		return nil, fmt.Errorf("check middleware command permission: %w", err)
	}
	if !allowed {
		return nil, ErrForbidden
	}
	var middlewareID pgtype.UUID
	err = s.pool.QueryRow(ctx, `
SELECT mp.middleware_client_id
FROM device d
JOIN middleware_plant mp ON mp.plant_id = d.plant_id
WHERE d.id=$1`, devUUID).Scan(&middlewareID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrMiddlewareOffline
	}
	if err != nil {
		return nil, fmt.Errorf("resolve device middleware: %w", err)
	}
	commandID, err := newUUID()
	if err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(map[string]any{
		"type": "command.request", "commandId": uuidString(commandID), "kind": kind, "connectionId": wireID(devUUID),
	})
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result, err := s.hub.RunCommand(runCtx, uuidString(middlewareID), uuidString(commandID), payload)
	if errors.Is(err, context.DeadlineExceeded) {
		return nil, fmt.Errorf("middleware command timed out: %w", ErrMiddlewareCommandNAK)
	}
	if err != nil {
		return nil, ErrMiddlewareOffline
	}
	return result, nil
}

// buildConfigSnapshot computes what middlewareID's config.snapshot should
// currently be: Plants assigned to it (middleware_plant), the Devices in
// those Plants that have modbus_host/modbus_port set and are active, the
// Device Models those Devices reference, and the Modbus-flavored register
// metadata on those models. A Device Set or Brand with nothing to poll is
// simply omitted -- the middleware only needs to know what it must poll.
func buildConfigSnapshot(ctx context.Context, tx pgx.Tx, middlewareID pgtype.UUID) (MiddlewareConfigSnapshot, error) {
	snapshot := MiddlewareConfigSnapshot{Brands: []MiddlewareBrand{}, DeviceSets: []MiddlewareDeviceSet{}, Connections: []MiddlewareConnection{}, Plants: []MiddlewarePlant{}}

	plantRows, err := tx.Query(ctx, `SELECT p.id, p.code, p.name FROM middleware_plant mp JOIN plant p ON p.id = mp.plant_id WHERE mp.middleware_client_id=$1`, middlewareID)
	if err != nil {
		return snapshot, fmt.Errorf("load assigned plants: %w", err)
	}
	type plantInfo struct {
		id         pgtype.UUID
		code, name string
	}
	var plants []plantInfo
	for plantRows.Next() {
		var p plantInfo
		if err = plantRows.Scan(&p.id, &p.code, &p.name); err != nil {
			plantRows.Close()
			return snapshot, fmt.Errorf("scan assigned plant: %w", err)
		}
		plants = append(plants, p)
		snapshot.Plants = append(snapshot.Plants, MiddlewarePlant{PlantCode: p.code, PlantName: p.name})
	}
	plantRows.Close()
	if err = plantRows.Err(); err != nil {
		return snapshot, err
	}
	if len(plants) == 0 {
		return snapshot, nil
	}
	plantIDs := make([]pgtype.UUID, len(plants))
	plantByID := make(map[string]plantInfo, len(plants))
	for i, p := range plants {
		plantIDs[i] = p.id
		plantByID[uuidString(p.id)] = p
	}

	deviceRows, err := tx.Query(ctx, `
SELECT id, plant_id, device_model_id, external_id, name, modbus_host, modbus_port, modbus_unit_id
FROM device
WHERE plant_id = ANY($1) AND modbus_host IS NOT NULL AND modbus_port IS NOT NULL AND is_active`, plantIDs)
	if err != nil {
		return snapshot, fmt.Errorf("load middleware-managed devices: %w", err)
	}
	type deviceInfo struct {
		id, plantID, deviceModelID pgtype.UUID
		externalID, name, host     string
		port, unitID               int32
	}
	var devices []deviceInfo
	modelIDSet := make(map[string]pgtype.UUID)
	for deviceRows.Next() {
		var d deviceInfo
		var host pgtype.Text
		var port pgtype.Int4
		if err = deviceRows.Scan(&d.id, &d.plantID, &d.deviceModelID, &d.externalID, &d.name, &host, &port, &d.unitID); err != nil {
			deviceRows.Close()
			return snapshot, fmt.Errorf("scan device: %w", err)
		}
		d.host, d.port = host.String, port.Int32
		devices = append(devices, d)
		modelIDSet[uuidString(d.deviceModelID)] = d.deviceModelID
	}
	deviceRows.Close()
	if err = deviceRows.Err(); err != nil {
		return snapshot, err
	}
	if len(devices) == 0 {
		return snapshot, nil
	}

	modelIDs := make([]pgtype.UUID, 0, len(modelIDSet))
	for _, id := range modelIDSet {
		modelIDs = append(modelIDs, id)
	}
	modelRows, err := tx.Query(ctx, `
SELECT id, manufacturer, model, device_type, modbus_address_mode, modbus_byte_order, modbus_word_order, modbus_max_block_size
FROM device_model WHERE id = ANY($1)`, modelIDs)
	if err != nil {
		return snapshot, fmt.Errorf("load device models: %w", err)
	}
	type modelInfo struct {
		manufacturer, model, deviceType, addressMode, byteOrder, wordOrder string
		maxBlockSize                                                      int32
	}
	models := make(map[string]modelInfo, len(modelIDs))
	brandSeen := make(map[string]bool)
	for modelRows.Next() {
		var id pgtype.UUID
		var m modelInfo
		if err = modelRows.Scan(&id, &m.manufacturer, &m.model, &m.deviceType, &m.addressMode, &m.byteOrder, &m.wordOrder, &m.maxBlockSize); err != nil {
			modelRows.Close()
			return snapshot, fmt.Errorf("scan device model: %w", err)
		}
		models[uuidString(id)] = m
		if !brandSeen[m.manufacturer] {
			brandSeen[m.manufacturer] = true
			snapshot.Brands = append(snapshot.Brands, MiddlewareBrand{BrandID: brandWireID(m.manufacturer), BrandName: m.manufacturer})
		}
	}
	modelRows.Close()
	if err = modelRows.Err(); err != nil {
		return snapshot, err
	}

	registerRows, err := tx.Query(ctx, `
SELECT id, device_model_id, address_key, display_name, modbus_function_code, modbus_register, modbus_word_order, modbus_data_type, scale, value_offset
FROM device_model_register_metadata
WHERE device_model_id = ANY($1) AND modbus_function_code IS NOT NULL`, modelIDs)
	if err != nil {
		return snapshot, fmt.Errorf("load model registers: %w", err)
	}
	addressesByModel := make(map[string][]MiddlewareAddress)
	for registerRows.Next() {
		var id, modelID pgtype.UUID
		var addressKey, displayName, dataType string
		var wordOrder pgtype.Text
		var functionCode, register int32
		var scale, offset float64
		if err = registerRows.Scan(&id, &modelID, &addressKey, &displayName, &functionCode, &register, &wordOrder, &dataType, &scale, &offset); err != nil {
			registerRows.Close()
			return snapshot, fmt.Errorf("scan model register: %w", err)
		}
		key := uuidString(modelID)
		addressesByModel[key] = append(addressesByModel[key], MiddlewareAddress{
			AddressID: wireID(id), DeviceSetID: wireID(modelID), FunctionCode: int(functionCode), Register: int(register),
			Description: displayName, CanonicalKey: addressKey, Factor: scale, Offset: offset,
			DataType: dataType, WordOrder: wordOrder.String, Enabled: true,
		})
	}
	registerRows.Close()
	if err = registerRows.Err(); err != nil {
		return snapshot, err
	}

	deviceSetSeen := make(map[string]bool)
	for _, d := range devices {
		modelKey := uuidString(d.deviceModelID)
		model, ok := models[modelKey]
		addresses := addressesByModel[modelKey]
		if !ok || len(addresses) == 0 {
			continue // model unknown or has no pollable registers -- nothing to send
		}
		if !deviceSetSeen[modelKey] {
			deviceSetSeen[modelKey] = true
			snapshot.DeviceSets = append(snapshot.DeviceSets, MiddlewareDeviceSet{
				DeviceSetID: wireID(d.deviceModelID), BrandID: brandWireID(model.manufacturer), BrandName: model.manufacturer,
				DevType: model.deviceType, DevModel: model.model, AddressMode: model.addressMode,
				ByteOrder: model.byteOrder, WordOrder: model.wordOrder, MaxBlockSize: int(model.maxBlockSize), Addresses: addresses,
			})
		}
		plant := plantByID[uuidString(d.plantID)]
		snapshot.Connections = append(snapshot.Connections, MiddlewareConnection{
			ConnectionID: wireID(d.id), ConnectionName: d.name, Host: d.host, Port: int(d.port), UnitID: int(d.unitID),
			DeviceSetID: wireID(d.deviceModelID), DevDn: d.externalID, DeviceName: d.name,
			PlantCode: plant.code, PlantName: plant.name, Enabled: true,
		})
	}
	return snapshot, nil
}

// recomputeAndPushMiddleware rebuilds middlewareID's config, and only if
// the content actually changed (by hash) bumps config_version, records
// history, and pushes it over the hub if the gateway is currently
// connected. A no-op when nothing relevant changed avoids spurious pushes
// from unrelated edits elsewhere in the same organization.
func (s *Service) recomputeAndPushMiddleware(ctx context.Context, middlewareID pgtype.UUID) {
	if err := s.doRecomputeAndPushMiddleware(ctx, middlewareID); err != nil {
		log.Printf("recompute middleware config %s: %v", uuidString(middlewareID), err)
	}
}

func (s *Service) doRecomputeAndPushMiddleware(ctx context.Context, middlewareID pgtype.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin recompute middleware config: %w", err)
	}
	defer tx.Rollback(ctx)
	var organizationID pgtype.UUID
	var previousBody []byte
	if err = tx.QueryRow(ctx, `SELECT organization_id, config_snapshot FROM middleware_client WHERE id=$1 FOR UPDATE`, middlewareID).Scan(&organizationID, &previousBody); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("lock middleware: %w", err)
	}
	snapshot, err := buildConfigSnapshot(ctx, tx, middlewareID)
	if err != nil {
		return fmt.Errorf("build config snapshot: %w", err)
	}
	// snapshot.Version isn't set yet at this point (it's assigned below from
	// the UPDATE ... RETURNING), so compare against the previous body with
	// its own version stripped -- easiest is to compare after re-marshaling
	// the previous body's non-version fields the same way.
	var previous MiddlewareConfigSnapshot
	_ = json.Unmarshal(previousBody, &previous)
	previous.Version = 0
	body := mustMarshal(snapshot)
	if string(mustMarshal(previous)) == string(body) {
		return tx.Commit(ctx) // content identical -- no version bump, no push
	}
	var version int64
	if err = tx.QueryRow(ctx, `UPDATE middleware_client SET config_version = config_version + 1, config_snapshot=$2, updated_at=now() WHERE id=$1 RETURNING config_version`,
		middlewareID, body).Scan(&version); err != nil {
		return fmt.Errorf("save computed config: %w", err)
	}
	snapshot.Version = version
	historyID, err := newUUID()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO middleware_config_history(id, organization_id, middleware_client_id, version, snapshot, push_status) VALUES ($1,$2,$3,$4,$5,'PENDING')`,
		historyID, organizationID, middlewareID, version, mustMarshal(snapshot)); err != nil {
		return fmt.Errorf("record config history: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit computed config: %w", err)
	}
	if s.hub.PushConfig(uuidString(middlewareID), pushEnvelope("config.snapshot", snapshot)) {
		_, _ = s.pool.Exec(ctx, `UPDATE middleware_config_history SET push_status='SENT' WHERE middleware_client_id=$1 AND version=$2`, middlewareID, version)
	}
	return nil
}

// recomputeAndPushMiddlewareForPlant is the call site used by Device write
// paths: a Device edit only affects the (at most one) Middleware currently
// serving its Plant.
func (s *Service) recomputeAndPushMiddlewareForPlant(ctx context.Context, organizationID, plantID pgtype.UUID) {
	var middlewareID pgtype.UUID
	err := s.pool.QueryRow(ctx, `SELECT middleware_client_id FROM middleware_plant WHERE plant_id=$1`, plantID).Scan(&middlewareID)
	if errors.Is(err, pgx.ErrNoRows) {
		return
	}
	if err != nil {
		log.Printf("resolve middleware for plant %s: %v", uuidString(plantID), err)
		return
	}
	s.recomputeAndPushMiddleware(ctx, middlewareID)
}

// recomputeAndPushMiddlewaresForDeviceModel is the call site used by
// DeviceModel/RegisterMetadata write paths: a model's register list
// changing can affect every Middleware serving a Plant with a Device of
// that model.
func (s *Service) recomputeAndPushMiddlewaresForDeviceModel(ctx context.Context, deviceModelID pgtype.UUID) {
	rows, err := s.pool.Query(ctx, `
SELECT DISTINCT mp.middleware_client_id
FROM device d
JOIN middleware_plant mp ON mp.plant_id = d.plant_id
WHERE d.device_model_id=$1`, deviceModelID)
	if err != nil {
		log.Printf("resolve middlewares for device model %s: %v", uuidString(deviceModelID), err)
		return
	}
	var middlewareIDs []pgtype.UUID
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			log.Printf("scan middleware for device model %s: %v", uuidString(deviceModelID), err)
			return
		}
		middlewareIDs = append(middlewareIDs, id)
	}
	rows.Close()
	for _, id := range middlewareIDs {
		s.recomputeAndPushMiddleware(ctx, id)
	}
}

// wireID derives a stable int64 wire ID from a Postgres UUID for the
// modbus-api-middleware wire protocol, which only ever treats IDs as
// within-snapshot correlation keys (see docs/adr/0005). Deterministic, so
// the same Device/DeviceModel maps to the same wire ID across pushes --
// needed so a command.request{connectionId} sent for a live Test Connection
// click matches what the middleware currently has loaded.
func wireID(id pgtype.UUID) int64 {
	sum := sha256.Sum256(id.Bytes[:])
	return int64(binary.BigEndian.Uint64(sum[:8]) & 0x7fffffffffffffff)
}

// brandWireID derives a wire ID for a Brand from its manufacturer name
// (device_model has no separate brand table -- manufacturer string is the
// brand identity), so the same manufacturer always maps to the same ID
// across every computed snapshot.
func brandWireID(manufacturer string) int64 {
	sum := sha256.Sum256([]byte("brand:" + manufacturer))
	return int64(binary.BigEndian.Uint64(sum[:8]) & 0x7fffffffffffffff)
}

// HandleGatewayHello is called by the realtime WS handler when a gateway
// first connects. It returns the config push payload if the server holds a
// newer version than the gateway has applied.
func (s *Service) HandleGatewayHello(ctx context.Context, clientID pgtype.UUID, appliedVersion int64) (payload []byte, shouldPush bool, err error) {
	_ = s.queries.TouchMiddlewareClient(ctx, clientID)
	var raw []byte
	var version int64
	err = s.pool.QueryRow(ctx, `SELECT config_snapshot, config_version FROM middleware_client WHERE id=$1`, clientID).Scan(&raw, &version)
	if err != nil {
		return nil, false, fmt.Errorf("load gateway config for hello: %w", err)
	}
	if version <= appliedVersion || len(raw) == 0 {
		return nil, false, nil
	}
	var snapshot MiddlewareConfigSnapshot
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		return nil, false, fmt.Errorf("decode stored config on hello: %w", err)
	}
	snapshot.Version = version
	return pushEnvelope("config.snapshot", snapshot), true, nil
}

// RecordConfigAck persists a gateway's config.ack and, on APPLIED, advances
// config_applied_version so future hellos stop re-pushing this version.
func (s *Service) RecordConfigAck(ctx context.Context, clientID pgtype.UUID, ack ConfigAckInput) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin config ack: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE middleware_config_history SET push_status=$3, push_reason=$4, acked_at=now() WHERE middleware_client_id=$1 AND version=$2`,
		clientID, ack.Version, ack.Status, ack.Reason); err != nil {
		return fmt.Errorf("record config ack: %w", err)
	}
	if ack.Status == "APPLIED" {
		if _, err = tx.Exec(ctx, `UPDATE middleware_client SET config_applied_version=$2, last_seen_at=now() WHERE id=$1 AND config_applied_version < $2`, clientID, ack.Version); err != nil {
			return fmt.Errorf("advance applied config version: %w", err)
		}
	} else {
		if _, err = tx.Exec(ctx, `UPDATE middleware_client SET last_seen_at=now() WHERE id=$1`, clientID); err != nil {
			return fmt.Errorf("touch gateway presence: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func mustMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func pushEnvelope(msgType string, snapshot MiddlewareConfigSnapshot) []byte {
	envelope := struct {
		Type string `json:"type"`
		MiddlewareConfigSnapshot
	}{Type: msgType, MiddlewareConfigSnapshot: snapshot}
	b, _ := json.Marshal(envelope)
	return b
}

func (s *Service) getMiddlewareInTx(ctx context.Context, tx pgx.Tx, id pgtype.UUID) (MiddlewareGateway, error) {
	row := tx.QueryRow(ctx, `
SELECT mc.id, mc.organization_id, o.name, mc.name, mc.site_name, mc.key_prefix,
       mc.is_active, mc.config_version, mc.config_applied_version, mc.last_seen_at, mc.created_at, mc.updated_at
FROM middleware_client mc
JOIN organization o ON o.id = mc.organization_id
WHERE mc.id=$1
FOR UPDATE OF mc`, id)
	gateway, err := scanMiddlewareGateway(row)
	if err != nil {
		return MiddlewareGateway{}, err
	}
	gateway.IsOnline = s.hub.IsOnline(gateway.ID)
	return gateway, nil
}

type middlewareScanner interface{ Scan(...any) error }

func scanMiddlewareGateway(row middlewareScanner) (MiddlewareGateway, error) {
	var gateway MiddlewareGateway
	var id, organizationID pgtype.UUID
	var lastSeenAt, createdAt, updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &organizationID, &gateway.OrganizationName, &gateway.Name, &gateway.SiteName, &gateway.KeyPrefix,
		&gateway.IsActive, &gateway.ConfigVersion, &gateway.ConfigAppliedVersion, &lastSeenAt, &createdAt, &updatedAt); err != nil {
		return MiddlewareGateway{}, fmt.Errorf("scan middleware gateway: %w", err)
	}
	gateway.ID = uuidString(id)
	gateway.OrganizationID = uuidString(organizationID)
	gateway.LastSeenAt = timePointer(lastSeenAt)
	gateway.CreatedAt = createdAt.Time
	gateway.UpdatedAt = updatedAt.Time
	return gateway, nil
}

func mapMiddlewareWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrMiddlewareConflict
		case "23503", "23514", "22023":
			return ErrMiddlewareInvalid
		}
	}
	return fmt.Errorf("write middleware: %w", err)
}
