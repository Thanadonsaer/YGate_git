package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/database/dbgen"
)

type LatestTelemetry struct {
	ID               string             `json:"id"`
	OrganizationID   string             `json:"organizationId"`
	PlantID          string             `json:"plantId"`
	DeviceID         string             `json:"deviceId"`
	DeviceExternalID string             `json:"deviceExternalId"`
	DeviceName       string             `json:"deviceName"`
	GatewayID        string             `json:"gatewayId"`
	ObservedAt       time.Time          `json:"observedAt"`
	ReceivedAt       time.Time          `json:"receivedAt"`
	DataItemMap      map[string]float64 `json:"dataItemMap"`
	DisplayItemMap   map[string]string  `json:"displayItemMap"`
	ParameterCount   int32              `json:"parameterCount"`
}

type TelemetryHistoryInput struct {
	From, To time.Time
	Limit    int
	Cursor   string
}

type TelemetryHistoryPage struct {
	Data       []LatestTelemetry `json:"data"`
	NextCursor *string           `json:"nextCursor"`
}

type telemetryHistoryCursor struct {
	ObservedAt time.Time `json:"observedAt"`
	ReceivedAt time.Time `json:"receivedAt"`
	ID         string    `json:"id"`
}

const maxTelemetryHistoryRange = 31 * 24 * time.Hour

// telemetryRowFields is the shape telemetryFromRow decodes: the columns are
// the same whether they come from the latest read model or from history, so
// both callers adapt into this rather than each growing their own decoder.
type telemetryRowFields struct {
	ID               pgtype.UUID
	OrganizationID   pgtype.UUID
	PlantID          pgtype.UUID
	DeviceID         pgtype.UUID
	DeviceExternalID string
	DeviceName       string
	GatewayID        string
	ObservedAt       pgtype.Timestamptz
	ReceivedAt       pgtype.Timestamptz
	DataItemMap      []byte
	ParameterCount   int32
	DisplayItemMap   []byte
}

func (s *Service) LatestTelemetry(ctx context.Context, principal auth.Principal, plantID string) ([]LatestTelemetry, error) {
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
		return nil, fmt.Errorf("authorize telemetry: %w", err)
	}
	// raw_telemetry_latest applies current Register Metadata at read time, so
	// this never serves a stale scale/offset and never has to be reconciled
	// with a second stored copy of the same reading.
	rows, err := s.queries.ListLatestPlantTelemetry(ctx, dbgen.ListLatestPlantTelemetryParams{
		OrganizationID: plant.OrganizationID, PlantID: plant.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("list latest plant telemetry: %w", err)
	}
	readings := make([]LatestTelemetry, 0, len(rows))
	for _, row := range rows {
		reading, decodeErr := telemetryFromRow(telemetryRowFields(row))
		if decodeErr != nil {
			return nil, decodeErr
		}
		readings = append(readings, reading)
	}
	return readings, nil
}

// mappedRawTelemetryHistory is the one hand-written telemetry query left (see
// the note in queries/telemetry.sql: sqlc mistypes the keyset cursor's UUID).
// The register mapping itself is not hand-written -- it is the same
// telemetry.mapped_data_items() the latest read model and alarm evaluation
// call, so history can't drift from what the device page shows.
//
// display_data_items() is deliberately skipped here (unlike
// raw_telemetry_latest): it runs a correlated subquery against
// register_value_mapping per register key, and every history caller
// (energy-analysis, the dashboard history widget) only reads dataItemMap.
// Paying that cost per row, per page, per device selected was the dominant
// cost behind the analysis page hanging when several devices were picked at
// once. displayItemMap stays in the response shape as an empty object so
// TelemetryHistoryPage's JSON contract is unchanged.
func (s *Service) mappedRawTelemetryHistory(ctx context.Context, organizationID, plantID, deviceID pgtype.UUID, input TelemetryHistoryInput, cursor telemetryHistoryCursor, cursorID pgtype.UUID, cursorSet bool, limit int32) ([]telemetryRowFields, error) {
	rows, err := s.pool.Query(ctx, `
SELECT raw.id, raw.organization_id, raw.plant_id, raw.device_id,
       device.external_id, device.name, raw.gateway_id, raw.observed_at, raw.received_at,
       mapped.data_item_map,
       (SELECT count(*)::integer FROM jsonb_object_keys(mapped.data_item_map)),
       '{}'::jsonb
FROM telemetry.raw_register_reading raw
JOIN plant.device device ON device.organization_id=raw.organization_id AND device.plant_id=raw.plant_id AND device.id=raw.device_id
CROSS JOIN LATERAL (
    SELECT telemetry.mapped_data_items(raw.organization_id, raw.plant_id, raw.device_id, device.device_model_id, raw.register_address_map) AS data_item_map
) mapped
WHERE raw.organization_id=$1 AND raw.plant_id=$2 AND raw.device_id=$3 AND raw.observed_at >= $4 AND raw.observed_at < $5
  AND (NOT $6::boolean OR (raw.observed_at, raw.received_at, raw.id) < ($7, $8, $9))
ORDER BY raw.observed_at DESC, raw.received_at DESC, raw.id DESC LIMIT $10`, organizationID, plantID, deviceID,
		pgtype.Timestamptz{Time: input.From.UTC(), Valid: true}, pgtype.Timestamptz{Time: input.To.UTC(), Valid: true}, cursorSet, cursor.ObservedAt, cursor.ReceivedAt, cursorID, limit)
	if err != nil {
		return nil, fmt.Errorf("list mapped raw telemetry history: %w", err)
	}
	defer rows.Close()
	items := make([]telemetryRowFields, 0, limit)
	for rows.Next() {
		var row telemetryRowFields
		if err := rows.Scan(&row.ID, &row.OrganizationID, &row.PlantID, &row.DeviceID, &row.DeviceExternalID, &row.DeviceName, &row.GatewayID, &row.ObservedAt, &row.ReceivedAt, &row.DataItemMap, &row.ParameterCount, &row.DisplayItemMap); err != nil {
			return nil, fmt.Errorf("scan mapped raw telemetry history: %w", err)
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read mapped raw telemetry history: %w", err)
	}
	return items, nil
}

func (s *Service) TelemetryHistory(ctx context.Context, principal auth.Principal, plantID, deviceID string, input TelemetryHistoryInput) (TelemetryHistoryPage, error) {
	plantUUID, err := parseUUID(plantID)
	if err != nil {
		return TelemetryHistoryPage{}, ErrNotFound
	}
	deviceUUID, err := parseUUID(deviceID)
	if err != nil {
		return TelemetryHistoryPage{}, ErrNotFound
	}
	if input.Limit < 1 || input.Limit > 500 || input.From.IsZero() || input.To.IsZero() || !input.From.Before(input.To) || input.To.Sub(input.From) > maxTelemetryHistoryRange {
		return TelemetryHistoryPage{}, ErrInvalid
	}
	plant, err := s.queries.GetAuthorizedPlantResource(ctx, dbgen.GetAuthorizedPlantResourceParams{
		PlantID: plantUUID, UserID: principal.UserID, Action: "read", ResourceType: "device",
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return TelemetryHistoryPage{}, ErrNotFound
	}
	if err != nil {
		return TelemetryHistoryPage{}, fmt.Errorf("authorize telemetry history: %w", err)
	}
	if _, err = s.queries.GetPlantDevice(ctx, dbgen.GetPlantDeviceParams{
		OrganizationID: plant.OrganizationID, PlantID: plant.ID, DeviceID: deviceUUID,
	}); errors.Is(err, pgx.ErrNoRows) {
		return TelemetryHistoryPage{}, ErrNotFound
	} else if err != nil {
		return TelemetryHistoryPage{}, fmt.Errorf("get telemetry device: %w", err)
	}

	cursor, err := decodeTelemetryHistoryCursor(input.Cursor)
	if err != nil || (input.Cursor != "" && (cursor.ObservedAt.Before(input.From) || !cursor.ObservedAt.Before(input.To))) {
		return TelemetryHistoryPage{}, ErrInvalid
	}
	cursorID, err := parseUUID(cursor.ID)
	if input.Cursor == "" {
		cursorID = pgtype.UUID{}
	} else if err != nil {
		return TelemetryHistoryPage{}, ErrInvalid
	}
	// raw_register_reading is the only telemetry store, so there is one
	// deterministic history path and metadata is always applied at read time.
	rows, err := s.mappedRawTelemetryHistory(ctx, plant.OrganizationID, plant.ID, deviceUUID, input, cursor, cursorID, input.Cursor != "", int32(input.Limit+1))
	if err != nil {
		return TelemetryHistoryPage{}, fmt.Errorf("list telemetry history: %w", err)
	}
	hasMore := len(rows) > input.Limit
	if hasMore {
		rows = rows[:input.Limit]
	}
	page := TelemetryHistoryPage{Data: make([]LatestTelemetry, 0, len(rows))}
	for _, row := range rows {
		reading, decodeErr := telemetryFromRow(row)
		if decodeErr != nil {
			return TelemetryHistoryPage{}, decodeErr
		}
		page.Data = append(page.Data, reading)
	}
	if hasMore {
		cursor := encodeTelemetryHistoryCursor(page.Data[len(page.Data)-1])
		page.NextCursor = &cursor
	}
	return page, nil
}

func telemetryFromRow(row telemetryRowFields) (LatestTelemetry, error) {
	values := make(map[string]float64)
	if err := json.Unmarshal(row.DataItemMap, &values); err != nil {
		return LatestTelemetry{}, fmt.Errorf("decode telemetry: %w", err)
	}
	displayValues := make(map[string]string)
	if len(row.DisplayItemMap) > 0 && string(row.DisplayItemMap) != "null" {
		if err := json.Unmarshal(row.DisplayItemMap, &displayValues); err != nil {
			return LatestTelemetry{}, fmt.Errorf("decode telemetry display values: %w", err)
		}
	}
	return LatestTelemetry{
		ID: uuidString(row.ID), OrganizationID: uuidString(row.OrganizationID),
		PlantID: uuidString(row.PlantID), DeviceID: uuidString(row.DeviceID),
		DeviceExternalID: row.DeviceExternalID, DeviceName: row.DeviceName,
		GatewayID: row.GatewayID, ObservedAt: row.ObservedAt.Time,
		ReceivedAt: row.ReceivedAt.Time, DataItemMap: values, DisplayItemMap: displayValues,
		ParameterCount: row.ParameterCount,
	}, nil
}

func encodeTelemetryHistoryCursor(reading LatestTelemetry) string {
	data, _ := json.Marshal(telemetryHistoryCursor{ObservedAt: reading.ObservedAt, ReceivedAt: reading.ReceivedAt, ID: reading.ID})
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeTelemetryHistoryCursor(value string) (telemetryHistoryCursor, error) {
	if value == "" {
		return telemetryHistoryCursor{}, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return telemetryHistoryCursor{}, err
	}
	var cursor telemetryHistoryCursor
	if err = json.Unmarshal(data, &cursor); err != nil || cursor.ObservedAt.IsZero() || cursor.ReceivedAt.IsZero() || cursor.ID == "" {
		return telemetryHistoryCursor{}, ErrInvalid
	}
	return cursor, nil
}
