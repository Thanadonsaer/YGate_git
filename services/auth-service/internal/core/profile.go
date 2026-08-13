package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/auth-service/internal/auth"
	"ygate/auth-service/internal/database/dbgen"
)

type SelfProfile struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId,omitempty"`
	Email          string    `json:"email"`
	Username       string    `json:"username"`
	DisplayName    string    `json:"displayName"`
	Roles          []string  `json:"roles"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type UpdateSelfProfileInput struct {
	Email       string
	Username    string
	DisplayName string
}

func (s *Service) OwnProfile(ctx context.Context, principal auth.Principal) (SelfProfile, error) {
	profile, err := getSelfProfile(ctx, s.pool, principal.UserID, false)
	if errors.Is(err, pgx.ErrNoRows) {
		return SelfProfile{}, ErrUserNotFound
	}
	return profile, err
}

func (s *Service) UpdateOwnProfile(ctx context.Context, principal auth.Principal, input UpdateSelfProfileInput, sourceIP *netip.Addr) (SelfProfile, error) {
	var err error
	input.Email, input.Username, input.DisplayName, err = validateUserProfile(input.Email, input.Username, input.DisplayName)
	if err != nil {
		return SelfProfile{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return SelfProfile{}, fmt.Errorf("begin self profile update: %w", err)
	}
	defer tx.Rollback(ctx)
	before, err := getSelfProfile(ctx, tx, principal.UserID, true)
	if errors.Is(err, pgx.ErrNoRows) {
		return SelfProfile{}, ErrUserNotFound
	}
	if err != nil {
		return SelfProfile{}, err
	}
	var username any
	if input.Username != "" {
		username = input.Username
	}
	if _, err = tx.Exec(ctx, `UPDATE auth.app_user SET email=$2, username=$3, display_name=$4, updated_at=now() WHERE id=$1`, principal.UserID, input.Email, username, input.DisplayName); err != nil {
		return SelfProfile{}, mapUserWriteError(err)
	}
	after, err := getSelfProfile(ctx, tx, principal.UserID, false)
	if err != nil {
		return SelfProfile{}, err
	}
	organizationID, err := parseUUID(before.OrganizationID)
	if err != nil {
		return SelfProfile{}, ErrUserNotFound
	}
	correlationID, err := newUUID()
	if err != nil {
		return SelfProfile{}, err
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	q := s.queries.WithTx(tx)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: organizationID, ActorUserID: principal.UserID,
		Action: "user.profile_updated", TargetType: "app_user", TargetID: principal.UserID,
		BeforeData: beforeJSON, AfterData: afterJSON, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return SelfProfile{}, fmt.Errorf("audit self profile update: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return SelfProfile{}, fmt.Errorf("commit self profile update: %w", err)
	}
	return after, nil
}

func getSelfProfile(ctx context.Context, querier rowQuerier, userID pgtype.UUID, lock bool) (SelfProfile, error) {
	query := `SELECT id, organization_id, email, COALESCE(username,''), display_name,
COALESCE((SELECT array_agg(r.name ORDER BY r.name) FROM auth.user_role ur JOIN auth.role r ON r.id=ur.role_id WHERE ur.user_id=auth.app_user.id AND ur.plant_id IS NULL), '{}')::text[], updated_at
FROM auth.app_user WHERE id=$1`
	if lock {
		query += " FOR UPDATE"
	}
	var profile SelfProfile
	var id, organizationID pgtype.UUID
	var updatedAt pgtype.Timestamptz
	if err := querier.QueryRow(ctx, query, userID).Scan(&id, &organizationID, &profile.Email, &profile.Username, &profile.DisplayName, &profile.Roles, &updatedAt); err != nil {
		return SelfProfile{}, fmt.Errorf("get self profile: %w", err)
	}
	profile.ID = uuidString(id)
	profile.OrganizationID = uuidString(organizationID)
	profile.UpdatedAt = updatedAt.Time
	return profile, nil
}
