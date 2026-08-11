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
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/database/dbgen"
)

var ErrEventLogbookInvalid = errors.New("invalid event logbook data")

type EventLogbookEntry struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organizationId"`
	PlantID        string     `json:"plantId"`
	DeviceID       *string    `json:"deviceId,omitempty"`
	EventType      string     `json:"eventType"`
	Category       string     `json:"category"`
	Title          string     `json:"title"`
	StartsAt       time.Time  `json:"startsAt"`
	EndsAt         *time.Time `json:"endsAt,omitempty"`
	Note           string     `json:"note"`
	Source         string     `json:"source"`
	CreatedBy      string     `json:"createdBy"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type CreateEventLogbookInput struct {
	DeviceID  string
	EventType string
	Category  string
	Title     string
	StartsAt  time.Time
	EndsAt    *time.Time
	Note      string
	Source    string
}

func validateEventLogbookInput(input CreateEventLogbookInput) (CreateEventLogbookInput, error) {
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.EventType = strings.ToUpper(strings.TrimSpace(input.EventType))
	input.Category = strings.TrimSpace(input.Category)
	input.Title = strings.TrimSpace(input.Title)
	input.Note = strings.TrimSpace(input.Note)
	input.Source = strings.ToUpper(strings.TrimSpace(input.Source))
	if input.Source == "" {
		input.Source = "MANUAL"
	}
	if input.StartsAt.IsZero() || input.Title == "" || len(input.Title) > 200 || len(input.Category) > 100 || len(input.Note) > 4000 {
		return CreateEventLogbookInput{}, ErrEventLogbookInvalid
	}
	switch input.EventType {
	case "FAULT", "MAINTENANCE", "CURTAILMENT", "NOTE":
	default:
		return CreateEventLogbookInput{}, ErrEventLogbookInvalid
	}
	if input.Source != "MANUAL" && input.Source != "SYSTEM" {
		return CreateEventLogbookInput{}, ErrEventLogbookInvalid
	}
	if input.EndsAt != nil && input.EndsAt.Before(input.StartsAt) {
		return CreateEventLogbookInput{}, ErrEventLogbookInvalid
	}
	return input, nil
}

func (s *Service) ListEventLogbook(ctx context.Context, principal auth.Principal, plantID string, limit int) ([]EventLogbookEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	plant, err := s.authorizedAlarmPlant(ctx, s.queries, principal, plantID, "read")
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, organization_id, plant_id, device_id, event_type, category, title,
       starts_at, ends_at, note, source, created_by, created_at, updated_at
FROM alarm.event_logbook
WHERE organization_id=$1 AND plant_id=$2
ORDER BY starts_at DESC, id DESC LIMIT $3`, plant.OrganizationID, plant.ID, limit)
	if err != nil {
		return nil, fmt.Errorf("list event logbook: %w", err)
	}
	defer rows.Close()
	entries := make([]EventLogbookEntry, 0)
	for rows.Next() {
		entry, scanErr := scanEventLogbookEntry(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Service) CreateEventLogbook(ctx context.Context, principal auth.Principal, plantID string, input CreateEventLogbookInput, sourceIP *netip.Addr) (EventLogbookEntry, error) {
	input, err := validateEventLogbookInput(input)
	if err != nil {
		return EventLogbookEntry{}, err
	}
	id, err := newUUID()
	if err != nil {
		return EventLogbookEntry{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return EventLogbookEntry{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return EventLogbookEntry{}, fmt.Errorf("begin create event logbook: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	plant, err := s.authorizedAlarmPlant(ctx, q, principal, plantID, "create")
	if err != nil {
		return EventLogbookEntry{}, err
	}
	var deviceID pgtype.UUID
	if input.DeviceID != "" {
		deviceID, err = parseUUID(input.DeviceID)
		if err != nil {
			return EventLogbookEntry{}, ErrEventLogbookInvalid
		}
		if _, err = q.GetPlantDevice(ctx, dbgen.GetPlantDeviceParams{OrganizationID: plant.OrganizationID, PlantID: plant.ID, DeviceID: deviceID}); errors.Is(err, pgx.ErrNoRows) {
			return EventLogbookEntry{}, ErrEventLogbookInvalid
		} else if err != nil {
			return EventLogbookEntry{}, fmt.Errorf("verify event logbook device: %w", err)
		}
	}
	row := tx.QueryRow(ctx, `
INSERT INTO alarm.event_logbook (id, organization_id, plant_id, device_id, event_type, category, title, starts_at, ends_at, note, source, created_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
RETURNING id, organization_id, plant_id, device_id, event_type, category, title, starts_at, ends_at, note, source, created_by, created_at, updated_at`,
		id, plant.OrganizationID, plant.ID, deviceID, input.EventType, input.Category, input.Title, input.StartsAt, input.EndsAt, input.Note, input.Source, principal.UserID)
	entry, err := scanEventLogbookEntry(row)
	if err != nil {
		return EventLogbookEntry{}, err
	}
	after, _ := json.Marshal(entry)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: plant.OrganizationID, ActorUserID: principal.UserID, Action: "event_logbook.created",
		TargetType: "event_logbook", TargetID: id, AfterData: after, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return EventLogbookEntry{}, fmt.Errorf("audit event logbook create: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return EventLogbookEntry{}, fmt.Errorf("commit event logbook create: %w", err)
	}
	return entry, nil
}

type rowScanner interface{ Scan(...any) error }

func scanEventLogbookEntry(row rowScanner) (EventLogbookEntry, error) {
	var id, organizationID, plantID, deviceID, createdBy pgtype.UUID
	var eventType, category, title, note, source string
	var startsAt, endsAt, createdAt, updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &organizationID, &plantID, &deviceID, &eventType, &category, &title, &startsAt, &endsAt, &note, &source, &createdBy, &createdAt, &updatedAt); err != nil {
		return EventLogbookEntry{}, fmt.Errorf("scan event logbook: %w", err)
	}
	var deviceIDValue *string
	if deviceID.Valid {
		value := uuidString(deviceID)
		deviceIDValue = &value
	}
	var endsAtValue *time.Time
	if endsAt.Valid {
		value := endsAt.Time
		endsAtValue = &value
	}
	return EventLogbookEntry{
		ID: uuidString(id), OrganizationID: uuidString(organizationID), PlantID: uuidString(plantID), DeviceID: deviceIDValue,
		EventType: eventType, Category: category, Title: title, StartsAt: startsAt.Time, EndsAt: endsAtValue, Note: note, Source: source,
		CreatedBy: uuidString(createdBy), CreatedAt: createdAt.Time, UpdatedAt: updatedAt.Time,
	}, nil
}
