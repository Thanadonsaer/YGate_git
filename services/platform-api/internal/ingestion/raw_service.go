package ingestion

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/database/dbgen"
)

const RawSchemaVersion = "2.0"

// RawReading.RegisterAddressMap is keyed by register address only (e.g.
// "40072"), value is the decoded raw register value. Function code, source
// data type and quality are not carried per reading — see the matching
// comment on the Middleware's domain.Reading for why (fixed FC per profile,
// scaling/typing owned by Register Metadata, quality always GOOD in
// practice). Newly discovered address keys default to a "number" Register
// Metadata row (see ensureModelRegisterMetadata) until an operator refines
// them, same as the v1 dataItemMap auto-onboard path.
type RawReading struct {
	GatewayID          string             `json:"gatewayId"`
	DevDn              string             `json:"devDn"`
	DevName            string             `json:"devName,omitempty"`
	PlantCode          string             `json:"plantCode"`
	PlantName          string             `json:"plantName,omitempty"`
	DevTypeID          int                `json:"devTypeId"`
	Model              string             `json:"model,omitempty"`
	CollectTime        int64              `json:"collectTime"`
	RegisterAddressMap map[string]float64 `json:"registerAddressMap"`
}

type RawBatch struct {
	SchemaVersion string       `json:"schemaVersion"`
	Data          []RawReading `json:"data"`
}

type normalizedRawReading struct {
	RawReading
	ObservedAt time.Time
	DeviceType string
}

func (s *Service) IngestRaw(ctx context.Context, client Client, idempotencyKey string, raw []byte, batch RawBatch, now time.Time) (Result, error) {
	if batch.SchemaVersion != RawSchemaVersion || len(batch.Data) == 0 || len(batch.Data) > 500 || !client.ID.Valid || !client.OrganizationID.Valid {
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
		return Result{}, fmt.Errorf("begin raw ingestion: %w", err)
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
		return Result{}, fmt.Errorf("create raw ingest batch: %w", err)
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
		normalized, validationErr := validateRawReading(reading, client.Name, now)
		if validationErr != nil {
			result.RejectedCount++
			result.Errors = append(result.Errors, RecordError{Index: index, Code: "INVALID_READING", Message: validationErr.Error()})
			continue
		}
		outcome, failure, ingestErr := ingestRawReading(ctx, tx, q, client, batchID, normalized)
		if ingestErr != nil {
			return Result{}, ingestErr
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
		return Result{}, fmt.Errorf("complete raw ingest batch: %w", err)
	}
	if err = q.TouchMiddlewareClient(ctx, client.ID); err != nil {
		return Result{}, fmt.Errorf("touch middleware client: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("commit raw ingestion: %w", err)
	}
	return result, nil
}

func validateRawReading(reading RawReading, clientName string, now time.Time) (normalizedRawReading, error) {
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
		return normalizedRawReading{}, errors.New("invalid plantCode")
	}
	if len(reading.DevDn) == 0 || len(reading.DevDn) > 200 || len(reading.DevName) > 200 || len(reading.PlantName) > 200 {
		return normalizedRawReading{}, errors.New("invalid plant or device identity")
	}
	if reading.DevTypeID <= 0 || len(reading.Model) > 200 || len(reading.GatewayID) == 0 || len(reading.GatewayID) > 200 {
		return normalizedRawReading{}, errors.New("invalid device type, model, or gateway")
	}
	if reading.CollectTime <= 0 || len(reading.RegisterAddressMap) == 0 || len(reading.RegisterAddressMap) > 5000 {
		return normalizedRawReading{}, errors.New("collectTime and registerAddressMap are required")
	}
	for key, rawValue := range reading.RegisterAddressMap {
		address, convErr := strconv.Atoi(key)
		if convErr != nil || address < 0 || address > 65535 {
			return normalizedRawReading{}, fmt.Errorf("register %q has invalid address", key)
		}
		if math.IsNaN(rawValue) || math.IsInf(rawValue, 0) {
			return normalizedRawReading{}, fmt.Errorf("register %q has a non-finite value", key)
		}
	}
	observedAt := time.UnixMilli(reading.CollectTime).UTC()
	if observedAt.After(now.UTC().Add(10 * time.Minute)) {
		return normalizedRawReading{}, errors.New("collectTime is too far in the future")
	}
	return normalizedRawReading{RawReading: reading, ObservedAt: observedAt, DeviceType: deviceType}, nil
}

func ingestRawReading(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, client Client, batchID pgtype.UUID, reading normalizedRawReading) (ingestOutcome, *recordFailure, error) {
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
		metadata, _ := json.Marshal(map[string]any{"source": "MIDDLEWARE_V2", "devTypeId": reading.DevTypeID, "gatewayId": reading.GatewayID})
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
	// The raw payload no longer carries a source data type (see RawReading
	// doc comment), so newly discovered address keys default to "number"
	// here — same as the v1 dataItemMap auto-onboard path in service.go.
	// Operators refine the type (e.g. to boolean) in Register Metadata.
	// Keyed as "reg"+address (not the bare address) to match the curated
	// vendor register catalogs (PVS100/PVS50/SUN2000, imported from Register
	// Metadata's CSV import) so a newly-discovered register that already has
	// a curated row picks up its real display name/scale/unit immediately
	// instead of colliding under a different key and sitting unmatched.
	metadata := make(map[string]string, len(reading.RegisterAddressMap))
	for key := range reading.RegisterAddressMap {
		metadata["reg"+key] = "number"
	}
	if err = ensureModelRegisterMetadata(ctx, tx, q, client, device.DeviceModelID, metadata, "MIDDLEWARE_V2"); err != nil {
		return outcome, nil, err
	}

	addressMap, _ := json.Marshal(reading.RegisterAddressMap)
	readingID, err := newUUID()
	if err != nil {
		return outcome, nil, err
	}
	externalKey := "v2:" + reading.PlantCode + ":" + reading.DevDn + ":" + reading.ObservedAt.Format(time.RFC3339Nano)
	_, err = q.InsertRawRegisterReading(ctx, dbgen.InsertRawRegisterReadingParams{
		ID: readingID, OrganizationID: client.OrganizationID, PlantID: plant.ID, DeviceID: device.ID,
		MiddlewareClientID: client.ID, IngestBatchID: batchID, GatewayID: reading.GatewayID,
		ExternalKey: externalKey, ObservedAt: pgtype.Timestamptz{Time: reading.ObservedAt, Valid: true},
		RegisterAddressMap: addressMap, ParameterCount: int32(len(reading.RegisterAddressMap)),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		outcome.duplicate++
		return outcome, nil, nil
	}
	if err != nil {
		return outcome, nil, fmt.Errorf("insert raw register reading: %w", err)
	}
	outcome.accepted++

	// Raw registers are addresses, not named parameters -- translate through
	// Register Metadata's scale/offset before they're usable as telemetry
	// (dashboard charts, Energy Analysis, alarms). A register with no
	// metadata row yet (shouldn't happen, ensureModelRegisterMetadata just
	// created one for every key above) or explicitly disabled by an operator
	// is left out of the reading entirely rather than stored raw.
	dataItemMap, err := scaledDataItemMap(ctx, tx, client.OrganizationID, device.DeviceModelID, reading.RegisterAddressMap)
	if err != nil {
		return outcome, nil, err
	}
	if len(dataItemMap) == 0 {
		return outcome, nil, nil
	}
	if _, err = recordTelemetryReading(ctx, tx, q, client, batchID, plant, device, reading.GatewayID, externalKey, reading.ObservedAt, dataItemMap, &outcome); err != nil {
		return outcome, nil, err
	}
	return outcome, nil, nil
}

// scaledDataItemMap translates a raw register reading into named-parameter
// values using device_model_register_metadata's scale/value_offset, the same
// table Register Metadata (the admin page) edits. Disabled registers, and
// keys with no metadata row at all, are dropped rather than passed through
// raw -- an operator explicitly turned them off, or hasn't reviewed them yet.
func scaledDataItemMap(ctx context.Context, tx pgx.Tx, organizationID, modelID pgtype.UUID, registers map[string]float64) (map[string]float64, error) {
	rows, err := tx.Query(ctx, `
SELECT address_key, scale, value_offset, is_enabled
FROM plant.device_model_register_metadata
WHERE organization_id=$1 AND device_model_id=$2`, organizationID, modelID)
	if err != nil {
		return nil, fmt.Errorf("load register metadata for scaling: %w", err)
	}
	defer rows.Close()
	dataItemMap := make(map[string]float64, len(registers))
	for rows.Next() {
		var addressKey string
		var scale, offset float64
		var isEnabled bool
		if err := rows.Scan(&addressKey, &scale, &offset, &isEnabled); err != nil {
			return nil, fmt.Errorf("scan register metadata for scaling: %w", err)
		}
		if !isEnabled {
			continue
		}
		// registers is keyed by bare address (the wire format); addressKey is
		// the metadata row's key, "reg"+address for both the curated vendor
		// catalogs and newly-onboarded registers (see ensureModelRegisterMetadata
		// call above) -- strip the prefix to look the raw value up, but keep
		// the DB's addressKey as the dataItemMap key so it still matches what
		// Register Metadata / Device Detail join against.
		if raw, ok := registers[strings.TrimPrefix(addressKey, "reg")]; ok {
			dataItemMap[addressKey] = raw*scale + offset
		}
	}
	return dataItemMap, rows.Err()
}
