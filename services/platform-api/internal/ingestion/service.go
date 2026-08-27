// Package ingestion accepts register readings from the Middleware. There is
// one ingest path -- raw register addresses, schemaVersion 2.0, see
// raw_service.go. This file holds what that path shares with the rest of the
// service: authentication, auto-created Register Metadata, audit, and the
// small pgtype helpers.
package ingestion

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"ygate/platform-api/internal/database/dbgen"
	"ygate/platform-api/internal/notify"
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
	mailer  *notify.Mailer
}

func New(pool *pgxpool.Pool, mailer *notify.Mailer) *Service {
	return &Service{pool: pool, queries: dbgen.New(pool), mailer: mailer}
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

func (s *Service) MiddlewareClientPullConfig(ctx context.Context, clientID pgtype.UUID) (pollIntervalSeconds, commandTimeoutSeconds int32, apiPollingEnabled bool, err error) {
	row, err := s.queries.MiddlewareClientPullConfig(ctx, clientID)
	if err != nil {
		return 0, 0, false, fmt.Errorf("load middleware client pull config: %w", err)
	}
	return row.PollIntervalSeconds, row.CommandTimeoutSeconds, row.ApiPollingEnabled, nil
}

// RecordMiddlewarePullEvent appends a system audit event for backend-driven Middleware pulls.
func (s *Service) RecordMiddlewarePullEvent(ctx context.Context, client Client, action string, details map[string]any) error {
	var softwareVersion pgtype.Text
	if err := s.pool.QueryRow(ctx, "SELECT software_version FROM auth.middleware_client WHERE id=$1", client.ID).Scan(&softwareVersion); err == nil && softwareVersion.Valid {
		details["middlewareVersion"] = softwareVersion.String
	}
	data, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("marshal middleware pull audit: %w", err)
	}
	correlationID, err := newUUID()
	if err != nil {
		return err
	}
	if err = s.queries.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: client.OrganizationID, Action: action, TargetType: "middleware_client", TargetID: client.ID,
		AfterData: data, CorrelationID: correlationID,
	}); err != nil {
		return fmt.Errorf("record middleware pull audit: %w", err)
	}
	return nil
}

// ingestOutcome accumulates one reading's effect on the batch result;
// recordFailure is a per-reading rejection that doesn't fail the batch.
type ingestOutcome struct {
	accepted, duplicate, plants, devices int32
	breaches                             []alarmBreach
}
type recordFailure struct{ Code, Message string }

func canonicalRegisterKey(key string) string {
	key = strings.TrimSpace(key)
	if strings.HasPrefix(strings.ToLower(key), "reg") {
		return key
	}
	if _, err := strconv.Atoi(key); err == nil {
		return "reg" + key
	}
	return key
}

func ensureModelRegisterMetadata(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, client Client, modelID pgtype.UUID, items map[string]string, source string) error {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	added := make([]string, 0, len(keys))
	for _, rawKey := range keys {
		key := canonicalRegisterKey(rawKey)
		id, err := newUUID()
		if err != nil {
			return err
		}
		dataType := metadataDataType(items[rawKey])
		result, err := tx.Exec(ctx, `
INSERT INTO plant.device_model_register_metadata (
    id, organization_id, device_model_id, address_key, display_name, data_type, notes
) VALUES ($1,$2,$3,$4,$4,$5,$6)
ON CONFLICT (organization_id, device_model_id, address_key) DO NOTHING`,
			id, client.OrganizationID, modelID, key, dataType, "Auto-created from "+source)
		if err != nil {
			return fmt.Errorf("auto-create register metadata %q: %w", key, err)
		}
		if result.RowsAffected() == 1 {
			profileAddressID, profileErr := newUUID()
			if profileErr != nil {
				return profileErr
			}
			if _, profileErr = tx.Exec(ctx, `
INSERT INTO plant.register_profile_address (
    id, organization_id, profile_id, address_key, display_name, data_type, notes
) VALUES ($1,$2,$3,$4,$4,$5,$6)
ON CONFLICT (organization_id, profile_id, address_key) DO NOTHING`,
				profileAddressID, client.OrganizationID, modelID, key, dataType, "Auto-created from "+source); profileErr != nil {
				return fmt.Errorf("auto-create register profile address %q: %w", key, profileErr)
			}
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
