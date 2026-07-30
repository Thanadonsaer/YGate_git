package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
	rows, err := s.queries.ListLatestPlantTelemetry(ctx, dbgen.ListLatestPlantTelemetryParams{
		OrganizationID: plant.OrganizationID, PlantID: plant.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("list latest telemetry: %w", err)
	}
	readings := make([]LatestTelemetry, 0, len(rows))
	for _, row := range rows {
		reading, decodeErr := telemetryFromRow(row)
		if decodeErr != nil {
			return nil, decodeErr
		}
		readings = append(readings, reading)
	}
	rawReadings, err := s.latestMappedRawTelemetry(ctx, plant.OrganizationID, plant.ID)
	if err != nil {
		return nil, err
	}
	byDevice := make(map[string]int, len(readings))
	for index, reading := range readings {
		byDevice[reading.DeviceID] = index
	}
	for _, reading := range rawReadings {
		if index, ok := byDevice[reading.DeviceID]; ok {
			readings[index] = reading
		} else {
			readings = append(readings, reading)
		}
	}
	sort.Slice(readings, func(i, j int) bool {
		if readings[i].DeviceName != readings[j].DeviceName {
			return readings[i].DeviceName < readings[j].DeviceName
		}
		return readings[i].DeviceExternalID < readings[j].DeviceExternalID
	})
	return readings, nil
}

func (s *Service) latestMappedRawTelemetry(ctx context.Context, organizationID, plantID pgtype.UUID) ([]LatestTelemetry, error) {
	rows, err := s.pool.Query(ctx, `
WITH latest_raw AS (
    SELECT DISTINCT ON (device_id) *
    FROM raw_register_reading
    WHERE organization_id=$1 AND plant_id=$2
    ORDER BY device_id, observed_at DESC, received_at DESC, id DESC
)
SELECT raw.id, raw.organization_id, raw.plant_id, raw.device_id,
       device.external_id, device.name, raw.gateway_id, raw.observed_at, raw.received_at,
       jsonb_object_agg(item.key, item.value::double precision * metadata.scale + metadata.value_offset)
           FILTER (WHERE metadata.is_enabled),
       count(*) FILTER (WHERE metadata.is_enabled)::integer
FROM latest_raw raw
JOIN device ON device.organization_id=raw.organization_id AND device.plant_id=raw.plant_id AND device.id=raw.device_id
LEFT JOIN telemetry_latest latest ON latest.organization_id=raw.organization_id AND latest.device_id=raw.device_id
CROSS JOIN LATERAL jsonb_each_text(raw.register_address_map) item
LEFT JOIN LATERAL (
    SELECT scale, value_offset, is_enabled
    FROM (
        SELECT scale, value_offset, is_enabled, 2 AS priority
        FROM device_register_metadata
        WHERE organization_id=raw.organization_id AND plant_id=raw.plant_id
          AND device_id=raw.device_id AND address_key=item.key
        UNION ALL
        SELECT scale, value_offset, is_enabled, 1 AS priority
        FROM device_model_register_metadata
        WHERE organization_id=raw.organization_id AND device_model_id=device.device_model_id
          AND address_key=item.key
    ) configured
    ORDER BY priority DESC
    LIMIT 1
) metadata ON true
WHERE latest.device_id IS NULL
   OR (raw.observed_at, raw.received_at, raw.id) > (latest.observed_at, latest.received_at, latest.telemetry_reading_id)
GROUP BY raw.id, raw.organization_id, raw.plant_id, raw.device_id, device.external_id, device.name,
         raw.gateway_id, raw.observed_at, raw.received_at
HAVING count(*) FILTER (WHERE metadata.is_enabled) > 0
ORDER BY device.name, device.external_id, raw.device_id
LIMIT 500`, organizationID, plantID)
	if err != nil {
		return nil, fmt.Errorf("list latest raw telemetry: %w", err)
	}
	defer rows.Close()
	readings := []LatestTelemetry{}
	for rows.Next() {
		var row dbgen.ListLatestPlantTelemetryRow
		if err = rows.Scan(
			&row.ID, &row.OrganizationID, &row.PlantID, &row.DeviceID,
			&row.DeviceExternalID, &row.DeviceName, &row.GatewayID,
			&row.ObservedAt, &row.ReceivedAt, &row.DataItemMap, &row.ParameterCount,
		); err != nil {
			return nil, fmt.Errorf("scan latest raw telemetry: %w", err)
		}
		reading, decodeErr := telemetryFromRow(row)
		if decodeErr != nil {
			return nil, decodeErr
		}
		readings = append(readings, reading)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("list latest raw telemetry: %w", err)
	}
	return readings, nil
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
	rows, err := s.queries.ListDeviceTelemetryHistory(ctx, dbgen.ListDeviceTelemetryHistoryParams{
		OrganizationID: plant.OrganizationID, PlantID: plant.ID, DeviceID: deviceUUID,
		FromTime: pgtype.Timestamptz{Time: input.From.UTC(), Valid: true},
		ToTime:   pgtype.Timestamptz{Time: input.To.UTC(), Valid: true}, CursorSet: input.Cursor != "",
		CursorObservedAt: pgtype.Timestamptz{Time: cursor.ObservedAt.UTC(), Valid: input.Cursor != ""},
		CursorReceivedAt: pgtype.Timestamptz{Time: cursor.ReceivedAt.UTC(), Valid: input.Cursor != ""},
		CursorID:         cursorID, PageLimit: int32(input.Limit + 1),
	})
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

func telemetryFromRow(row dbgen.ListLatestPlantTelemetryRow) (LatestTelemetry, error) {
	values := make(map[string]float64)
	if err := json.Unmarshal(row.DataItemMap, &values); err != nil {
		return LatestTelemetry{}, fmt.Errorf("decode telemetry: %w", err)
	}
	return LatestTelemetry{
		ID: uuidString(row.ID), OrganizationID: uuidString(row.OrganizationID),
		PlantID: uuidString(row.PlantID), DeviceID: uuidString(row.DeviceID),
		DeviceExternalID: row.DeviceExternalID, DeviceName: row.DeviceName,
		GatewayID: row.GatewayID, ObservedAt: row.ObservedAt.Time,
		ReceivedAt: row.ReceivedAt.Time, DataItemMap: values,
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
