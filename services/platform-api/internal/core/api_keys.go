package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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
	ErrAPIKeyNotFound = errors.New("api key not found")
	ErrAPIKeyInvalid  = errors.New("invalid api key data")
	ErrAPIKeyConflict = errors.New("api key already exists")
)

type APIKeyClient struct {
	ID               string     `json:"id"`
	OrganizationID   string     `json:"organizationId"`
	OrganizationName string     `json:"organizationName"`
	Name             string     `json:"name"`
	KeyPrefix        string     `json:"keyPrefix"`
	AutoOnboard      bool       `json:"autoOnboard"`
	IsActive         bool       `json:"isActive"`
	LastSeenAt       *time.Time `json:"lastSeenAt"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type CreatedAPIKeyClient struct {
	APIKeyClient
	APIKey string `json:"apiKey"`
}

type CreateAPIKeyInput struct {
	OrganizationID string
	Name           string
	AutoOnboard    bool
}

type UpdateAPIKeyInput struct {
	Name        string
	AutoOnboard bool
	IsActive    bool
}

func (s *Service) CanReadAPIContract(ctx context.Context, principal auth.Principal) error {
	allowed, err := s.queries.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "read", ResourceType: "api_contract"})
	if err != nil {
		return fmt.Errorf("check api contract read permission: %w", err)
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func (s *Service) APIKeys(ctx context.Context, principal auth.Principal) ([]APIKeyClient, error) {
	allowed, err := s.queries.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "read", ResourceType: "middleware_client"})
	if err != nil {
		return nil, fmt.Errorf("check api key read permission: %w", err)
	}
	if !allowed {
		return nil, ErrForbidden
	}
	rows, err := s.pool.Query(ctx, `
SELECT mc.id, mc.organization_id, o.name, mc.name, mc.key_prefix, mc.auto_onboard,
       mc.is_active, mc.last_seen_at, mc.created_at, mc.updated_at
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
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()
	clients := []APIKeyClient{}
	for rows.Next() {
		client, err := scanAPIKeyClient(rows)
		if err != nil {
			return nil, err
		}
		clients = append(clients, client)
	}
	return clients, rows.Err()
}

func (s *Service) CreateAPIKey(ctx context.Context, principal auth.Principal, input CreateAPIKeyInput, sourceIP *netip.Addr) (CreatedAPIKeyClient, error) {
	organizationID := principal.OrganizationID
	var err error
	if strings.TrimSpace(input.OrganizationID) != "" {
		organizationID, err = parseUUID(input.OrganizationID)
		if err != nil {
			return CreatedAPIKeyClient{}, ErrAPIKeyInvalid
		}
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 200 || !organizationID.Valid {
		return CreatedAPIKeyClient{}, ErrAPIKeyInvalid
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return CreatedAPIKeyClient{}, fmt.Errorf("generate api key: %w", err)
	}
	apiKey := "ygm_" + base64.RawURLEncoding.EncodeToString(secret)
	keyHash := sha256.Sum256([]byte(apiKey))
	id, err := newUUID()
	if err != nil {
		return CreatedAPIKeyClient{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return CreatedAPIKeyClient{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CreatedAPIKeyClient{}, fmt.Errorf("begin create api key: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	if err = s.requireOrganizationPermission(ctx, q, principal, "create", "middleware_client", organizationID); err != nil {
		return CreatedAPIKeyClient{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO middleware_client(id, organization_id, name, key_prefix, key_hash, auto_onboard) VALUES($1,$2,$3,$4,$5,$6)`, id, organizationID, input.Name, apiKey[:12], keyHash[:], input.AutoOnboard); err != nil {
		return CreatedAPIKeyClient{}, mapAPIKeyWriteError(err)
	}
	client, err := s.getAPIKeyInTx(ctx, tx, id)
	if err != nil {
		return CreatedAPIKeyClient{}, err
	}
	after, _ := json.Marshal(client)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "middleware_client.created", TargetType: "middleware_client", TargetID: id, AfterData: after, SourceIp: sourceIP, CorrelationID: correlationID}); err != nil {
		return CreatedAPIKeyClient{}, fmt.Errorf("audit api key create: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return CreatedAPIKeyClient{}, fmt.Errorf("commit create api key: %w", err)
	}
	return CreatedAPIKeyClient{APIKeyClient: client, APIKey: apiKey}, nil
}

func (s *Service) SetAPIKeyActive(ctx context.Context, principal auth.Principal, keyID string, active bool, sourceIP *netip.Addr) (APIKeyClient, error) {
	id, err := parseUUID(keyID)
	if err != nil {
		return APIKeyClient{}, ErrAPIKeyNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return APIKeyClient{}, fmt.Errorf("begin api key status update: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	before, err := s.getAPIKeyInTx(ctx, tx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return APIKeyClient{}, ErrAPIKeyNotFound
	}
	if err != nil {
		return APIKeyClient{}, err
	}
	organizationID, err := parseUUID(before.OrganizationID)
	if err != nil {
		return APIKeyClient{}, ErrAPIKeyNotFound
	}
	if err = s.requireOrganizationPermission(ctx, q, principal, "update", "middleware_client", organizationID); err != nil {
		return APIKeyClient{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE middleware_client SET is_active=$2, updated_at=now() WHERE id=$1`, id, active); err != nil {
		return APIKeyClient{}, mapAPIKeyWriteError(err)
	}
	after, err := s.getAPIKeyInTx(ctx, tx, id)
	if err != nil {
		return APIKeyClient{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return APIKeyClient{}, err
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "middleware_client.status_updated", TargetType: "middleware_client", TargetID: id, BeforeData: beforeJSON, AfterData: afterJSON, SourceIp: sourceIP, CorrelationID: correlationID}); err != nil {
		return APIKeyClient{}, fmt.Errorf("audit api key status update: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return APIKeyClient{}, fmt.Errorf("commit api key status update: %w", err)
	}
	return after, nil
}

func (s *Service) UpdateAPIKey(ctx context.Context, principal auth.Principal, keyID string, input UpdateAPIKeyInput, sourceIP *netip.Addr) (APIKeyClient, error) {
	id, err := parseUUID(keyID)
	if err != nil {
		return APIKeyClient{}, ErrAPIKeyNotFound
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 200 {
		return APIKeyClient{}, ErrAPIKeyInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return APIKeyClient{}, fmt.Errorf("begin api key update: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	before, err := s.getAPIKeyInTx(ctx, tx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return APIKeyClient{}, ErrAPIKeyNotFound
	}
	if err != nil {
		return APIKeyClient{}, err
	}
	organizationID, err := parseUUID(before.OrganizationID)
	if err != nil {
		return APIKeyClient{}, ErrAPIKeyNotFound
	}
	if err = s.requireOrganizationPermission(ctx, q, principal, "update", "middleware_client", organizationID); err != nil {
		return APIKeyClient{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE middleware_client SET name=$2, auto_onboard=$3, is_active=$4, updated_at=now() WHERE id=$1`, id, input.Name, input.AutoOnboard, input.IsActive); err != nil {
		return APIKeyClient{}, mapAPIKeyWriteError(err)
	}
	after, err := s.getAPIKeyInTx(ctx, tx, id)
	if err != nil {
		return APIKeyClient{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return APIKeyClient{}, err
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "middleware_client.updated", TargetType: "middleware_client", TargetID: id, BeforeData: beforeJSON, AfterData: afterJSON, SourceIp: sourceIP, CorrelationID: correlationID}); err != nil {
		return APIKeyClient{}, fmt.Errorf("audit api key update: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return APIKeyClient{}, fmt.Errorf("commit api key update: %w", err)
	}
	return after, nil
}

func (s *Service) getAPIKeyInTx(ctx context.Context, tx pgx.Tx, id pgtype.UUID) (APIKeyClient, error) {
	row := tx.QueryRow(ctx, `
SELECT mc.id, mc.organization_id, o.name, mc.name, mc.key_prefix, mc.auto_onboard,
       mc.is_active, mc.last_seen_at, mc.created_at, mc.updated_at
FROM middleware_client mc
JOIN organization o ON o.id = mc.organization_id
WHERE mc.id=$1
FOR UPDATE OF mc`, id)
	return scanAPIKeyClient(row)
}

type apiKeyScanner interface{ Scan(...any) error }

func scanAPIKeyClient(row apiKeyScanner) (APIKeyClient, error) {
	var client APIKeyClient
	var id, organizationID pgtype.UUID
	var lastSeenAt, createdAt, updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &organizationID, &client.OrganizationName, &client.Name, &client.KeyPrefix, &client.AutoOnboard, &client.IsActive, &lastSeenAt, &createdAt, &updatedAt); err != nil {
		return APIKeyClient{}, fmt.Errorf("scan api key: %w", err)
	}
	client.ID = uuidString(id)
	client.OrganizationID = uuidString(organizationID)
	client.LastSeenAt = timePointer(lastSeenAt)
	client.CreatedAt = createdAt.Time
	client.UpdatedAt = updatedAt.Time
	return client, nil
}

func mapAPIKeyWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrAPIKeyConflict
		case "23503", "23514", "22023":
			return ErrAPIKeyInvalid
		}
	}
	return fmt.Errorf("write api key: %w", err)
}
