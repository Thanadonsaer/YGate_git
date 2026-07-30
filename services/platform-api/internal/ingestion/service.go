package ingestion

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"ygate/platform-api/internal/database/dbgen"
)

var (
	ErrUnauthenticated     = errors.New("invalid middleware API key")
	ErrInvalidBatch        = errors.New("invalid telemetry batch")
	ErrIdempotencyConflict = errors.New("idempotency key already used for another payload")
	plantCodeRE            = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._=-]*$`)
)

type Client struct {
	ID             pgtype.UUID
	OrganizationID pgtype.UUID
	Name           string
	AutoOnboard    bool
}

type Reading struct {
	GatewayID   string             `json:"gatewayId,omitempty"`
	DevDn       string             `json:"devDn"`
	DevName     string             `json:"devName,omitempty"`
	PlantCode   string             `json:"plantCode"`
	PlantName   string             `json:"plantName,omitempty"`
	DevTypeID   int                `json:"devTypeId"`
	Model       string             `json:"model,omitempty"`
	CollectTime int64              `json:"collectTime"`
	DataItemMap map[string]float64 `json:"dataItemMap"`
}

type Batch struct {
	Data []Reading `json:"data"`
}

type RecordError struct {
	Index   int    `json:"index"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Result struct {
	IngestionID          string        `json:"ingestionId"`
	Status               string        `json:"status"`
	AcceptedCount        int32         `json:"acceptedCount"`
	DuplicateCount       int32         `json:"duplicateCount"`
	RejectedCount        int32         `json:"rejectedCount"`
	OnboardedPlantCount  int32         `json:"onboardedPlantCount"`
	OnboardedDeviceCount int32         `json:"onboardedDeviceCount"`
	Errors               []RecordError `json:"errors"`
}

type Service struct {
	pool    *pgxpool.Pool
	queries *dbgen.Queries
}

func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, queries: dbgen.New(pool)}
}

func (s *Service) Authenticate(ctx context.Context, presentedKey string) (Client, error) {
	presentedKey = strings.TrimSpace(presentedKey)
	if len(presentedKey) < 24 || len(presentedKey) > 200 {
		return Client{}, ErrUnauthenticated
	}
	hash := sha256.Sum256([]byte(presentedKey))
	row, err := s.queries.AuthenticateMiddlewareClient(ctx, hash[:])
	if errors.Is(err, pgx.ErrNoRows) {
		return Client{}, ErrUnauthenticated
	}
	if err != nil {
		return Client{}, fmt.Errorf("authenticate middleware: %w", err)
	}
	return Client{ID: row.ID, OrganizationID: row.OrganizationID, Name: row.Name, AutoOnboard: row.AutoOnboard}, nil
}

func (s *Service) Ingest(ctx context.Context, client Client, idempotencyKey string, raw []byte, batch Batch, now time.Time) (Result, error) {
	if len(batch.Data) == 0 || len(batch.Data) > 500 || !client.ID.Valid || !client.OrganizationID.Valid {
		return Result{}, ErrInvalidBatch
	}
	payloadHash := sha256.Sum256(raw)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = "sha256:" + hex.EncodeToString(payloadHash[:])
	}
	if len(idempotencyKey) > 200 {
		return Result{}, ErrInvalidBatch
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("begin ingestion: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	batchID, err := newUUID()
	if err != nil {
		return Result{}, err
	}
	stored, err := q.CreateOrGetIngestBatch(ctx, dbgen.CreateOrGetIngestBatchParams{
		ID: batchID, OrganizationID: client.OrganizationID, MiddlewareClientID: client.ID,
		IdempotencyKey: idempotencyKey, PayloadHash: payloadHash[:], RawPayload: raw,
	})
	if err != nil {
		return Result{}, fmt.Errorf("create ingest batch: %w", err)
	}
	if !uuidEqual(stored.ID, batchID) {
		if !bytes.Equal(stored.PayloadHash, payloadHash[:]) {
			return Result{}, ErrIdempotencyConflict
		}
		result := resultFromStored(stored)
		result.Status = "duplicate"
		return result, nil
	}

	result := Result{IngestionID: uuidString(batchID), Status: "accepted", Errors: []RecordError{}}
	for index, reading := range batch.Data {
		normalized, validationErr := validateReading(reading, client.Name, now)
		if validationErr != nil {
			result.RejectedCount++
			result.Errors = append(result.Errors, RecordError{Index: index, Code: "INVALID_READING", Message: validationErr.Error()})
			continue
		}
		outcome, failure, err := ingestReading(ctx, tx, q, client, batchID, normalized)
		if err != nil {
			return Result{}, err
		}
		if failure != nil {
			result.RejectedCount++
			result.Errors = append(result.Errors, RecordError{Index: index, Code: failure.Code, Message: failure.Message})
			continue
		}
		result.AcceptedCount += outcome.accepted
		result.DuplicateCount += outcome.duplicate
		result.OnboardedPlantCount += outcome.plants
		result.OnboardedDeviceCount += outcome.devices
	}
	errorsJSON, _ := json.Marshal(result.Errors)
	if err = q.CompleteIngestBatch(ctx, dbgen.CompleteIngestBatchParams{
		AcceptedCount: result.AcceptedCount, DuplicateCount: result.DuplicateCount,
		RejectedCount: result.RejectedCount, OnboardedPlantCount: result.OnboardedPlantCount,
		OnboardedDeviceCount: result.OnboardedDeviceCount, Errors: errorsJSON, ID: batchID,
	}); err != nil {
		return Result{}, fmt.Errorf("complete ingest batch: %w", err)
	}
	if err = q.TouchMiddlewareClient(ctx, client.ID); err != nil {
		return Result{}, fmt.Errorf("touch middleware client: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("commit ingestion: %w", err)
	}
	return result, nil
}

type normalizedReading struct {
	Reading
	ObservedAt time.Time
	DeviceType string
}

func validateReading(reading Reading, clientName string, now time.Time) (normalizedReading, error) {
	reading.PlantCode = strings.ToUpper(strings.TrimSpace(reading.PlantCode))
	reading.PlantName = strings.TrimSpace(reading.PlantName)
	reading.DevDn = strings.TrimSpace(reading.DevDn)
	reading.DevName = strings.TrimSpace(reading.DevName)
	reading.GatewayID = strings.TrimSpace(reading.GatewayID)
	reading.Model = strings.TrimSpace(reading.Model)
	if reading.PlantName == "" {
		reading.PlantName = reading.PlantCode
	}
	if reading.DevName == "" {
		reading.DevName = reading.DevDn
	}
	if reading.GatewayID == "" {
		reading.GatewayID = clientName
	}
	deviceType := fmt.Sprintf("DEV_TYPE_%d", reading.DevTypeID)
	if reading.Model == "" {
		reading.Model = deviceType
	}
	if len(reading.PlantCode) == 0 || len(reading.PlantCode) > 100 || !plantCodeRE.MatchString(reading.PlantCode) {
		return normalizedReading{}, errors.New("invalid plantCode")
	}
	if len(reading.PlantName) > 200 || len(reading.DevDn) == 0 || len(reading.DevDn) > 200 || len(reading.DevName) > 200 {
		return normalizedReading{}, errors.New("invalid plant or device identity")
	}
	if reading.DevTypeID <= 0 || len(reading.Model) > 200 || len(reading.GatewayID) == 0 || len(reading.GatewayID) > 200 {
		return normalizedReading{}, errors.New("invalid device type, model, or gateway")
	}
	if reading.CollectTime <= 0 || len(reading.DataItemMap) == 0 || len(reading.DataItemMap) > 5000 {
		return normalizedReading{}, errors.New("collectTime and dataItemMap are required")
	}
	observedAt := time.UnixMilli(reading.CollectTime).UTC()
	if observedAt.After(now.UTC().Add(10 * time.Minute)) {
		return normalizedReading{}, errors.New("collectTime is too far in the future")
	}
	return normalizedReading{Reading: reading, ObservedAt: observedAt, DeviceType: deviceType}, nil
}

type ingestOutcome struct{ accepted, duplicate, plants, devices int32 }
type recordFailure struct{ Code, Message string }

func ingestReading(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, client Client, batchID pgtype.UUID, reading normalizedReading) (ingestOutcome, *recordFailure, error) {
	var outcome ingestOutcome
	plant, err := q.GetIngestionPlant(ctx, dbgen.GetIngestionPlantParams{OrganizationID: client.OrganizationID, Code: reading.PlantCode})
	if errors.Is(err, pgx.ErrNoRows) {
		if !client.AutoOnboard {
			return outcome, &recordFailure{Code: "UNKNOWN_PLANT", Message: "plant is not registered"}, nil
		}
		id, uuidErr := newUUID()
		if uuidErr != nil {
			return outcome, nil, uuidErr
		}
		created, createErr := q.OnboardPlant(ctx, dbgen.OnboardPlantParams{ID: id, OrganizationID: client.OrganizationID, Code: reading.PlantCode, Name: reading.PlantName})
		if createErr != nil {
			return outcome, nil, fmt.Errorf("onboard plant: %w", createErr)
		}
		plant = dbgen.GetIngestionPlantRow{ID: created.ID, OrganizationID: created.OrganizationID, Code: created.Code, Name: created.Name, IsActive: created.IsActive}
		if created.Created {
			outcome.plants++
			if auditErr := inventoryAudit(ctx, q, client, "plant.auto_onboarded", "plant", created.ID, map[string]any{"code": created.Code, "name": created.Name, "gatewayId": reading.GatewayID}); auditErr != nil {
				return outcome, nil, auditErr
			}
		}
	} else if err != nil {
		return outcome, nil, fmt.Errorf("get ingestion plant: %w", err)
	}
	if !plant.IsActive {
		return outcome, &recordFailure{Code: "PLANT_DISABLED", Message: "plant is disabled"}, nil
	}

	device, err := q.GetIngestionDevice(ctx, dbgen.GetIngestionDeviceParams{OrganizationID: client.OrganizationID, PlantID: plant.ID, ExternalID: reading.DevDn})
	if errors.Is(err, pgx.ErrNoRows) {
		if !client.AutoOnboard {
			return outcome, &recordFailure{Code: "UNKNOWN_DEVICE", Message: "device is not registered"}, nil
		}
		modelID, uuidErr := newUUID()
		if uuidErr != nil {
			return outcome, nil, uuidErr
		}
		model, modelErr := q.OnboardDeviceModel(ctx, dbgen.OnboardDeviceModelParams{
			ID: modelID, OrganizationID: client.OrganizationID, Model: reading.Model,
			DeviceType: reading.DeviceType, SourceTypeID: pgtype.Int4{Int32: int32(reading.DevTypeID), Valid: true},
		})
		if modelErr != nil {
			return outcome, nil, fmt.Errorf("onboard device model: %w", modelErr)
		}
		deviceID, uuidErr := newUUID()
		if uuidErr != nil {
			return outcome, nil, uuidErr
		}
		metadata, _ := json.Marshal(map[string]any{"source": "MIDDLEWARE", "devTypeId": reading.DevTypeID, "gatewayId": reading.GatewayID})
		created, createErr := q.OnboardDevice(ctx, dbgen.OnboardDeviceParams{
			ID: deviceID, OrganizationID: client.OrganizationID, PlantID: plant.ID,
			DeviceModelID: model.ID, ExternalID: reading.DevDn, Name: reading.DevName, SourceMetadata: metadata,
		})
		if createErr != nil {
			return outcome, nil, fmt.Errorf("onboard device: %w", createErr)
		}
		device = dbgen.GetIngestionDeviceRow{ID: created.ID, OrganizationID: created.OrganizationID, PlantID: created.PlantID, DeviceModelID: created.DeviceModelID, ExternalID: created.ExternalID, Name: created.Name, IsActive: created.IsActive}
		if created.Created {
			outcome.devices++
			if auditErr := inventoryAudit(ctx, q, client, "device.auto_onboarded", "device", created.ID, map[string]any{"plantId": uuidString(plant.ID), "externalId": created.ExternalID, "name": created.Name, "gatewayId": reading.GatewayID, "devTypeId": reading.DevTypeID}); auditErr != nil {
				return outcome, nil, auditErr
			}
		}
	} else if err != nil {
		return outcome, nil, fmt.Errorf("get ingestion device: %w", err)
	}
	if !device.IsActive {
		return outcome, &recordFailure{Code: "DEVICE_DISABLED", Message: "device is disabled"}, nil
	}
	metadata := make(map[string]string, len(reading.DataItemMap))
	for key := range reading.DataItemMap {
		metadata[key] = "number"
	}
	if err = ensureModelRegisterMetadata(ctx, tx, q, client, device.DeviceModelID, metadata, "MIDDLEWARE_V1"); err != nil {
		return outcome, nil, err
	}

	dataItemMap, _ := json.Marshal(reading.DataItemMap)
	readingID, err := newUUID()
	if err != nil {
		return outcome, nil, err
	}
	externalKey := reading.PlantCode + ":" + reading.DevDn + ":" + reading.ObservedAt.Format(time.RFC3339Nano)
	insertedID, err := q.InsertTelemetryReading(ctx, dbgen.InsertTelemetryReadingParams{
		ID: readingID, OrganizationID: client.OrganizationID, PlantID: plant.ID, DeviceID: device.ID,
		MiddlewareClientID: client.ID, IngestBatchID: batchID, GatewayID: reading.GatewayID,
		ExternalKey: externalKey, ObservedAt: pgtype.Timestamptz{Time: reading.ObservedAt, Valid: true},
		DataItemMap: dataItemMap, ParameterCount: int32(len(reading.DataItemMap)),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		outcome.duplicate++
		return outcome, nil, nil
	}
	if err != nil {
		return outcome, nil, fmt.Errorf("insert telemetry reading: %w", err)
	}
	if err = q.UpsertTelemetryLatest(ctx, insertedID); err != nil {
		return outcome, nil, fmt.Errorf("upsert latest telemetry: %w", err)
	}
	if err = evaluateAlarms(ctx, tx, client.OrganizationID, plant.ID, device.ID, reading.DataItemMap, reading.ObservedAt); err != nil {
		return outcome, nil, fmt.Errorf("evaluate alarms: %w", err)
	}
	outcome.accepted++
	return outcome, nil, nil
}

func ensureModelRegisterMetadata(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, client Client, modelID pgtype.UUID, items map[string]string, source string) error {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	added := make([]string, 0, len(keys))
	for _, key := range keys {
		id, err := newUUID()
		if err != nil {
			return err
		}
		dataType := metadataDataType(items[key])
		result, err := tx.Exec(ctx, `
INSERT INTO device_model_register_metadata (
    id, organization_id, device_model_id, address_key, display_name, data_type, notes
) VALUES ($1,$2,$3,$4,$4,$5,$6)
ON CONFLICT (organization_id, device_model_id, address_key) DO NOTHING`,
			id, client.OrganizationID, modelID, key, dataType, "Auto-created from "+source)
		if err != nil {
			return fmt.Errorf("auto-create register metadata %q: %w", key, err)
		}
		if result.RowsAffected() == 1 {
			added = append(added, key)
		}
	}
	if len(added) > 0 {
		if err := inventoryAudit(ctx, q, client, "device_model_register_metadata.auto_created", "device_model", modelID, map[string]any{"source": source, "addressKeys": added}); err != nil {
			return err
		}
	}
	return nil
}

func metadataDataType(sourceType string) string {
	switch strings.ToUpper(strings.TrimSpace(sourceType)) {
	case "BOOL", "BOOLEAN", "BIT", "COIL":
		return "boolean"
	default:
		return "number"
	}
}

func inventoryAudit(ctx context.Context, q *dbgen.Queries, client Client, action, targetType string, targetID pgtype.UUID, after any) error {
	data, _ := json.Marshal(after)
	correlationID, err := newUUID()
	if err != nil {
		return err
	}
	if err = q.CreateInventoryAuditEvent(ctx, dbgen.CreateInventoryAuditEventParams{
		OrganizationID: client.OrganizationID, Action: action, TargetType: targetType,
		TargetID: targetID, AfterData: data, CorrelationID: correlationID,
	}); err != nil {
		return fmt.Errorf("audit %s: %w", targetType, err)
	}
	return nil
}

func resultFromStored(row dbgen.CreateOrGetIngestBatchRow) Result {
	result := Result{IngestionID: uuidString(row.ID), Status: strings.ToLower(row.Status), AcceptedCount: row.AcceptedCount, DuplicateCount: row.DuplicateCount, RejectedCount: row.RejectedCount, OnboardedPlantCount: row.OnboardedPlantCount, OnboardedDeviceCount: row.OnboardedDeviceCount, Errors: []RecordError{}}
	_ = json.Unmarshal(row.Errors, &result.Errors)
	return result
}

func newUUID() (pgtype.UUID, error) {
	var value pgtype.UUID
	if _, err := rand.Read(value.Bytes[:]); err != nil {
		return value, fmt.Errorf("generate UUID: %w", err)
	}
	value.Bytes[6] = value.Bytes[6]&0x0f | 0x40
	value.Bytes[8] = value.Bytes[8]&0x3f | 0x80
	value.Valid = true
	return value, nil
}

func uuidEqual(left, right pgtype.UUID) bool {
	return left.Valid == right.Valid && left.Bytes == right.Bytes
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	b := value.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
