package core

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/database/dbgen"
	"ygate/platform-api/internal/gatewayhub"
)

var (
	ErrForbidden = errors.New("permission denied")
	ErrNotFound  = errors.New("plant not found")
	ErrInvalid   = errors.New("invalid plant data")
	ErrConflict  = errors.New("plant code already exists")
	plantCodeRE  = regexp.MustCompile(`^[A-Z0-9][A-Z0-9 ._()/+=-]*$`)
)

type Service struct {
	pool          *pgxpool.Pool
	queries       *dbgen.Queries
	hub           *gatewayhub.Hub
	patchDir      string
	logoDir       string
	plantImageDir string
	publicBaseURL string
}

func New(pool *pgxpool.Pool, hub *gatewayhub.Hub) *Service {
	return &Service{pool: pool, queries: dbgen.New(pool), hub: hub, patchDir: "./data/middleware-patches", logoDir: "./data/site-logos", plantImageDir: "./data/plant-images"}
}

// WithMiddlewarePatchDir overrides the directory uploaded middleware patch
// zips are stored under (see middleware_patch.go). Optional -- New already
// sets a sane default; production wires this from PLATFORM_MIDDLEWARE_PATCH_DIR.
func (s *Service) WithMiddlewarePatchDir(dir string) *Service {
	if dir != "" {
		s.patchDir = dir
	}
	return s
}

// WithSiteLogoDir overrides the directory uploaded site logo files are
// stored under (see site_settings.go). Optional -- New already sets a sane
// default; production wires this from PLATFORM_SITE_LOGO_DIR.
func (s *Service) WithSiteLogoDir(dir string) *Service {
	if dir != "" {
		s.logoDir = dir
	}
	return s
}

// WithPublicBaseURL sets the externally-reachable base URL (e.g.
// https://ygate.example.com) middleware patch download links are built
// against -- must be the same host a Middleware's realtime WS connection
// already reaches. Empty by default; StageMiddlewareUpdate errors clearly
// if a Stage is attempted without it configured.
func (s *Service) WithPublicBaseURL(url string) *Service {
	s.publicBaseURL = url
	return s
}

type Plant struct {
	ID               string    `json:"id"`
	OrganizationID   string    `json:"organizationId"`
	OrganizationName string    `json:"organizationName"`
	Code             string    `json:"code"`
	Name             string    `json:"name"`
	Timezone         string    `json:"timezone"`
	Latitude         *float64  `json:"latitude"`
	Longitude        *float64  `json:"longitude"`
	InstalledDcKW    *float64  `json:"installedDcKw"`
	InstalledAcKW    *float64  `json:"installedAcKw"`
	LifecycleStatus   string    `json:"lifecycleStatus"`
	ImageURL         *string   `json:"imageUrl,omitempty"`
	IsActive         bool      `json:"isActive"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type CreatePlantInput struct {
	OrganizationID string
	Code           string
	Name           string
	Timezone       string
	Latitude       *float64
	Longitude      *float64
	InstalledDcKW  *float64
	InstalledAcKW  *float64
	LifecycleStatus string
	ImageURL       *string `json:"imageUrl,omitempty"`
}

type UpdatePlantInput struct {
	Code          string
	Name          string
	Timezone      string
	Latitude      *float64
	Longitude     *float64
	InstalledDcKW *float64
	InstalledAcKW *float64
	LifecycleStatus string
	ImageURL      *string `json:"imageUrl,omitempty"`
	IsActive      bool
}

const (
	PlantLifecycleInConstruction = "IN_CONSTRUCTION"
	PlantLifecycleOperational    = "OPERATIONAL"
	PlantLifecycleOffline        = "OFFLINE"
	PlantLifecycleRetired        = "RETIRED"
)

func validatePlantLifecycle(status string) error {
	switch strings.TrimSpace(status) {
	case PlantLifecycleInConstruction, PlantLifecycleOperational, PlantLifecycleOffline, PlantLifecycleRetired:
		return nil
	default:
		return ErrInvalid
	}
}

func (s *Service) Plants(ctx context.Context, principal auth.Principal) ([]Plant, error) {
	allowed, err := s.queries.HasUserPermission(ctx, dbgen.HasUserPermissionParams{
		UserID: principal.UserID, Action: "read", ResourceType: "plant",
	})
	if err != nil {
		return nil, fmt.Errorf("check plant read permission: %w", err)
	}
	if !allowed {
		return nil, ErrForbidden
	}
	rows, err := s.queries.ListAuthorizedPlants(ctx, principal.UserID)
	if err != nil {
		return nil, fmt.Errorf("list plants: %w", err)
	}
	plants := make([]Plant, 0, len(rows))
	for _, row := range rows {
		plants = append(plants, plantFromFields(
			row.ID, row.OrganizationID, row.OrganizationName, row.Code, row.Name, row.Timezone,
			row.Latitude, row.Longitude, row.InstalledDcKw, row.InstalledAcKw, row.LifecycleStatus, textPointer(row.ImageUrl),
			row.IsActive, row.CreatedAt, row.UpdatedAt,
		))
	}
	return plants, nil
}

func (s *Service) Plant(ctx context.Context, principal auth.Principal, plantID string) (Plant, error) {
	id, err := parseUUID(plantID)
	if err != nil {
		return Plant{}, ErrNotFound
	}
	row, err := s.queries.GetAuthorizedPlant(ctx, dbgen.GetAuthorizedPlantParams{
		PlantID: id, UserID: principal.UserID, Action: "read",
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Plant{}, ErrNotFound
	}
	if err != nil {
		return Plant{}, fmt.Errorf("get plant: %w", err)
	}
	return plantFromFields(
		row.ID, row.OrganizationID, row.OrganizationName, row.Code, row.Name, row.Timezone,
		row.Latitude, row.Longitude, row.InstalledDcKw, row.InstalledAcKw, row.LifecycleStatus, textPointer(row.ImageUrl),
		row.IsActive, row.CreatedAt, row.UpdatedAt,
	), nil
}

func (s *Service) CreatePlant(ctx context.Context, principal auth.Principal, input CreatePlantInput, sourceIP *netip.Addr) (Plant, error) {
	organizationID := principal.OrganizationID
	var err error
	if strings.TrimSpace(input.OrganizationID) != "" {
		organizationID, err = parseUUID(input.OrganizationID)
		if err != nil {
			return Plant{}, ErrInvalid
		}
	}
	input.Code, input.Name, input.Timezone, err = validatePlant(input.Code, input.Name, input.Timezone, input.Latitude, input.Longitude, input.InstalledDcKW, input.InstalledAcKW)
	if input.LifecycleStatus == "" {
		input.LifecycleStatus = PlantLifecycleOperational
	}
	if err != nil || validatePlantLifecycle(input.LifecycleStatus) != nil || !organizationID.Valid {
		return Plant{}, ErrInvalid
	}
	id, err := newUUID()
	if err != nil {
		return Plant{}, err
	}
	correlationID, err := newUUID()
	if err != nil {
		return Plant{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Plant{}, fmt.Errorf("begin create plant: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	allowed, err := q.HasOrganizationPermission(ctx, dbgen.HasOrganizationPermissionParams{
		UserID: principal.UserID, Action: "create", ResourceType: "plant", OrganizationID: organizationID,
	})
	if err != nil {
		return Plant{}, fmt.Errorf("check plant create permission: %w", err)
	}
	if !allowed {
		return Plant{}, ErrForbidden
	}
	row, err := q.CreatePlant(ctx, dbgen.CreatePlantParams{
		ID: id, OrganizationID: organizationID, Code: input.Code, Name: input.Name, Timezone: input.Timezone,
		Latitude: nullableFloat(input.Latitude), Longitude: nullableFloat(input.Longitude),
		InstalledDcKw: nullableFloat(input.InstalledDcKW), InstalledAcKw: nullableFloat(input.InstalledAcKW), LifecycleStatus: input.LifecycleStatus,
	})
	if err != nil {
		return Plant{}, mapWriteError(err)
	}
	organizationName, err := q.GetOrganizationName(ctx, organizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Plant{}, ErrInvalid
	}
	if err != nil {
		return Plant{}, fmt.Errorf("load plant organization: %w", err)
	}
	plant := plantFromFields(
		row.ID, row.OrganizationID, organizationName, row.Code, row.Name, row.Timezone,
		row.Latitude, row.Longitude, row.InstalledDcKw, row.InstalledAcKw, row.LifecycleStatus, textPointer(row.ImageUrl),
		row.IsActive, row.CreatedAt, row.UpdatedAt,
	)
	after, _ := json.Marshal(plant)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "plant.created",
		TargetType: "plant", TargetID: id, AfterData: after, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return Plant{}, fmt.Errorf("audit plant create: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Plant{}, fmt.Errorf("commit create plant: %w", err)
	}
	return plant, nil
}

func (s *Service) UpdatePlant(ctx context.Context, principal auth.Principal, plantID string, input UpdatePlantInput, sourceIP *netip.Addr) (Plant, error) {
	id, err := parseUUID(plantID)
	if err != nil {
		return Plant{}, ErrNotFound
	}
	input.Code, input.Name, input.Timezone, err = validatePlant(input.Code, input.Name, input.Timezone, input.Latitude, input.Longitude, input.InstalledDcKW, input.InstalledAcKW)
	if err != nil || validatePlantLifecycle(input.LifecycleStatus) != nil {
		return Plant{}, ErrInvalid
	}
	correlationID, err := newUUID()
	if err != nil {
		return Plant{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Plant{}, fmt.Errorf("begin update plant: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	current, err := q.GetAuthorizedPlantForUpdate(ctx, dbgen.GetAuthorizedPlantForUpdateParams{PlantID: id, UserID: principal.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Plant{}, ErrNotFound
	}
	if err != nil {
		return Plant{}, fmt.Errorf("lock plant: %w", err)
	}
	beforePlant := plantFromFields(
		current.ID, current.OrganizationID, current.OrganizationName, current.Code, current.Name, current.Timezone,
		current.Latitude, current.Longitude, current.InstalledDcKw, current.InstalledAcKw, current.LifecycleStatus, textPointer(current.ImageUrl),
		current.IsActive, current.CreatedAt, current.UpdatedAt,
	)
	row, err := q.UpdatePlant(ctx, dbgen.UpdatePlantParams{
		ID: id, Code: input.Code, Name: input.Name, Timezone: input.Timezone,
		Latitude: nullableFloat(input.Latitude), Longitude: nullableFloat(input.Longitude),
		InstalledDcKw: nullableFloat(input.InstalledDcKW), InstalledAcKw: nullableFloat(input.InstalledAcKW), LifecycleStatus: input.LifecycleStatus, IsActive: input.IsActive,
	})
	if err != nil {
		return Plant{}, mapWriteError(err)
	}
	plant := plantFromFields(
		row.ID, row.OrganizationID, current.OrganizationName, row.Code, row.Name, row.Timezone,
		row.Latitude, row.Longitude, row.InstalledDcKw, row.InstalledAcKw, row.LifecycleStatus, textPointer(row.ImageUrl),
		row.IsActive, row.CreatedAt, row.UpdatedAt,
	)
	before, _ := json.Marshal(beforePlant)
	after, _ := json.Marshal(plant)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: current.OrganizationID, ActorUserID: principal.UserID, Action: "plant.updated",
		TargetType: "plant", TargetID: id, BeforeData: before, AfterData: after,
		SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return Plant{}, fmt.Errorf("audit plant update: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Plant{}, fmt.Errorf("commit update plant: %w", err)
	}
	return plant, nil
}

func validatePlant(code, name, timezone string, latitude, longitude, installedDcKW, installedAcKW *float64) (string, string, string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	name = strings.TrimSpace(name)
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		timezone = "Asia/Bangkok"
	}
	if len(code) == 0 || len(code) > 100 || !plantCodeRE.MatchString(code) || len(name) == 0 || len(name) > 200 || len(timezone) > 100 {
		return code, name, timezone, ErrInvalid
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return code, name, timezone, ErrInvalid
	}
	if invalidNumber(latitude, -90, 90) || invalidNumber(longitude, -180, 180) || invalidNumber(installedDcKW, 0, math.MaxFloat64) || invalidNumber(installedAcKW, 0, math.MaxFloat64) {
		return code, name, timezone, ErrInvalid
	}
	return code, name, timezone, nil
}

func invalidNumber(value *float64, minimum, maximum float64) bool {
	return value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0) || *value < minimum || *value > maximum)
}

func nullableFloat(value *float64) pgtype.Float8 {
	if value == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *value, Valid: true}
}

func floatPointer(value pgtype.Float8) *float64 {
	if !value.Valid {
		return nil
	}
	copy := value.Float64
	return &copy
}

func numericPointer(value pgtype.Numeric) *float64 {
	if !value.Valid {
		return nil
	}
	converted, err := value.Float64Value()
	if err != nil || !converted.Valid {
		return nil
	}
	copy := converted.Float64
	return &copy
}

func plantFromFields(
	id, organizationID pgtype.UUID, organizationName, code, name, timezone string,
	latitude, longitude pgtype.Float8, installedDcKW, installedAcKW pgtype.Numeric, lifecycleStatus string, imageURL *string,
	isActive bool, createdAt, updatedAt pgtype.Timestamptz,
) Plant {
	return Plant{
		ID: uuidString(id), OrganizationID: uuidString(organizationID), OrganizationName: organizationName,
		Code: code, Name: name, Timezone: timezone, Latitude: floatPointer(latitude), Longitude: floatPointer(longitude), LifecycleStatus: lifecycleStatus,
		InstalledDcKW: numericPointer(installedDcKW), InstalledAcKW: numericPointer(installedAcKW), ImageURL: imageURL,
		IsActive: isActive, CreatedAt: createdAt.Time, UpdatedAt: updatedAt.Time,
	}
}

func mapWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrConflict
		case "23503", "23514", "22023":
			return ErrInvalid
		}
	}
	return fmt.Errorf("write plant: %w", err)
}

func parseUUID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(strings.TrimSpace(value)); err != nil || !id.Valid {
		return id, ErrInvalid
	}
	return id, nil
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

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	b := value.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
