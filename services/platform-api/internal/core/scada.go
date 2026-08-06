package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/database/dbgen"
)

const (
	maxScadaNodes  = 300
	maxScadaEdges  = 500
	maxScadaDesign = 512 << 10
)

var (
	ErrScadaNotFound        = errors.New("scada screen not found")
	ErrScadaConflict        = errors.New("scada screen already exists")
	ErrScadaVersionConflict = errors.New("scada screen version conflict")
)

type ScadaPosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type ScadaViewport struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}

type ScadaTelemetryBinding struct {
	DeviceID string `json:"deviceId"`
	PointKey string `json:"pointKey"`
	Unit     string `json:"unit"`
	Decimals int    `json:"decimals"`
}

type ScadaDataItem struct {
	Label    string                `json:"label"`
	Binding  ScadaTelemetryBinding `json:"binding"`
	MinAlarm *float64              `json:"minAlarm,omitempty"`
	MaxAlarm *float64              `json:"maxAlarm,omitempty"`
}

type ScadaNodeData struct {
	Label         string                 `json:"label"`
	EquipmentKind string                 `json:"equipmentKind,omitempty"`
	Binding       *ScadaTelemetryBinding `json:"binding,omitempty"`
	DeviceID      string                 `json:"deviceId,omitempty"`
	DisplayType   string                 `json:"displayType,omitempty"`
	ShapeKind     string                 `json:"shapeKind,omitempty"`
	MinValue      *float64               `json:"minValue,omitempty"`
	MaxValue      *float64               `json:"maxValue,omitempty"`
	OnValue       *float64               `json:"onValue,omitempty"`
	ImageURL      string                 `json:"imageUrl,omitempty"`
	Text          string                 `json:"text,omitempty"`
	Timezone      string                 `json:"timezone,omitempty"`
	Items         []ScadaDataItem        `json:"items,omitempty"`
}

type ScadaNode struct {
	ID       string        `json:"id"`
	Type     string        `json:"type"`
	Position ScadaPosition `json:"position"`
	Data     ScadaNodeData `json:"data"`
	Width    float64       `json:"width,omitempty"`
	Height   float64       `json:"height,omitempty"`
}

type ScadaEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

type ScadaDesign struct {
	Version  int           `json:"version"`
	Nodes    []ScadaNode   `json:"nodes"`
	Edges    []ScadaEdge   `json:"edges"`
	Viewport ScadaViewport `json:"viewport"`
}

type ScadaScreenSummary struct {
	ID               string    `json:"id"`
	OrganizationID   string    `json:"organizationId"`
	PlantID          string    `json:"plantId"`
	PlantCode        string    `json:"plantCode"`
	PlantName        string    `json:"plantName"`
	Name             string    `json:"name"`
	DraftVersion     int32     `json:"draftVersion"`
	PublishedVersion int32     `json:"publishedVersion"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type ScadaScreen struct {
	ScadaScreenSummary
	Design          ScadaDesign `json:"design"`
	CanEdit         bool        `json:"canEdit"`
	CanPublish      bool        `json:"canPublish"`
	CanManageAccess bool        `json:"canManageAccess"`
	CanHardDelete   bool        `json:"canHardDelete"`
}

type PublishedScadaScreen struct {
	ScadaScreenSummary
	SourceDraftVersion int32       `json:"sourceDraftVersion"`
	Design             ScadaDesign `json:"design"`
	PublishedAt        time.Time   `json:"publishedAt"`
}

type ScadaScreenVersion struct {
	Version            int32     `json:"version"`
	SourceDraftVersion int32     `json:"sourceDraftVersion"`
	PublishedBy        string    `json:"publishedBy"`
	PublishedAt        time.Time `json:"publishedAt"`
	Current            bool      `json:"current"`
}

type CreateScadaScreenInput struct {
	PlantID string `json:"plantId"`
	Name    string `json:"name"`
}

type UpdateScadaScreenInput struct {
	DraftVersion int32       `json:"draftVersion"`
	Name         string      `json:"name"`
	Design       ScadaDesign `json:"design"`
}

type PublishScadaScreenInput struct {
	DraftVersion     int32 `json:"draftVersion"`
	PublishedVersion int32 `json:"publishedVersion"`
}

type RollbackScadaScreenInput struct {
	TargetVersion    int32 `json:"targetVersion"`
	PublishedVersion int32 `json:"publishedVersion"`
}

type storedScadaScreen struct {
	Summary ScadaScreenSummary
	Design  ScadaDesign
	Raw     []byte
	OrgID   pgtype.UUID
	PlantID pgtype.UUID
	ID      pgtype.UUID
}

func DefaultScadaDesign() ScadaDesign {
	return ScadaDesign{Version: 1, Nodes: []ScadaNode{}, Edges: []ScadaEdge{}, Viewport: ScadaViewport{Zoom: 1}}
}

func ValidateScadaDesign(design ScadaDesign) error {
	if design.Version != 1 || len(design.Nodes) > maxScadaNodes || len(design.Edges) > maxScadaEdges || !finiteBetween(design.Viewport.X, -10000, 10000) || !finiteBetween(design.Viewport.Y, -10000, 10000) || !finiteBetween(design.Viewport.Zoom, .1, 4) {
		return ErrInvalid
	}
	nodes := make(map[string]bool, len(design.Nodes))
	for _, node := range design.Nodes {
		if !validScadaID(node.ID) || nodes[node.ID] || !finiteBetween(node.Position.X, -10000, 10000) || !finiteBetween(node.Position.Y, -10000, 10000) || strings.TrimSpace(node.Data.Label) != node.Data.Label || node.Data.Label == "" || len(node.Data.Label) > 100 || node.Width != 0 && !finiteBetween(node.Width, 40, 2000) || node.Height != 0 && !finiteBetween(node.Height, 30, 2000) {
			return ErrInvalid
		}
		nodes[node.ID] = true
		switch node.Type {
		case "equipment":
			if !oneOf(node.Data.EquipmentKind, "solar-panel", "inverter", "meter", "grid") {
				return ErrInvalid
			}
		case "metric":
			if node.Data.Binding == nil || !oneOf(node.Data.DisplayType, "", "text", "gauge", "progress", "tank") || !validScadaRange(node.Data.MinValue, node.Data.MaxValue) {
				return ErrInvalid
			}
		case "label":
			if node.Data.Binding != nil {
				return ErrInvalid
			}
		case "shape":
			if !oneOf(node.Data.ShapeKind, "rectangle", "circle", "triangle", "diamond", "hexagon") {
				return ErrInvalid
			}
		case "section":
		case "led":
			if node.Data.Binding == nil || node.Data.OnValue != nil && !finiteBetween(*node.Data.OnValue, -1e15, 1e15) {
				return ErrInvalid
			}
		case "clock":
			if node.Data.Timezone == "" || len(node.Data.Timezone) > 100 {
				return ErrInvalid
			}
			if _, err := time.LoadLocation(node.Data.Timezone); err != nil {
				return ErrInvalid
			}
		case "image":
			if len(node.Data.ImageURL) > 2048 || node.Data.ImageURL != "" && !validScadaImageURL(node.Data.ImageURL) {
				return ErrInvalid
			}
		case "table", "alarms":
			if len(node.Data.Items) < 1 || len(node.Data.Items) > 20 {
				return ErrInvalid
			}
		case "device-summary":
			if _, err := parseUUID(node.Data.DeviceID); err != nil {
				return ErrInvalid
			}
		case "ticker":
			if strings.TrimSpace(node.Data.Text) != node.Data.Text || len(node.Data.Text) > 200 || node.Data.Text == "" && node.Data.Binding == nil {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
		if binding := node.Data.Binding; binding != nil {
			if !validScadaBinding(*binding) {
				return ErrInvalid
			}
		}
		for _, item := range node.Data.Items {
			if strings.TrimSpace(item.Label) != item.Label || item.Label == "" || len(item.Label) > 100 || !validScadaBinding(item.Binding) || !validScadaRange(item.MinAlarm, item.MaxAlarm) {
				return ErrInvalid
			}
		}
	}
	edges := make(map[string]bool, len(design.Edges))
	pairs := make(map[string]bool, len(design.Edges))
	for _, edge := range design.Edges {
		pair := edge.Source + "\x00" + edge.Target
		if !validScadaID(edge.ID) || edges[edge.ID] || !nodes[edge.Source] || !nodes[edge.Target] || edge.Source == edge.Target || pairs[pair] || !oneOf(edge.Type, "default", "smoothstep") {
			return ErrInvalid
		}
		edges[edge.ID], pairs[pair] = true, true
	}
	encoded, err := json.Marshal(design)
	if err != nil || len(encoded) > maxScadaDesign {
		return ErrInvalid
	}
	return nil
}

func validScadaBinding(binding ScadaTelemetryBinding) bool {
	_, err := parseUUID(binding.DeviceID)
	return err == nil && strings.TrimSpace(binding.PointKey) == binding.PointKey && binding.PointKey != "" && len(binding.PointKey) <= 200 && strings.TrimSpace(binding.Unit) == binding.Unit && len(binding.Unit) <= 20 && binding.Decimals >= 0 && binding.Decimals <= 6
}

func validScadaRange(minimum, maximum *float64) bool {
	if minimum != nil && !finiteBetween(*minimum, -1e15, 1e15) || maximum != nil && !finiteBetween(*maximum, -1e15, 1e15) {
		return false
	}
	return minimum == nil || maximum == nil || *minimum < *maximum
}

func validScadaImageURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	return err == nil && (strings.HasPrefix(value, "/") || (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "")
}

func (s *Service) ScadaScreens(ctx context.Context, principal auth.Principal, plantID string) ([]ScadaScreenSummary, error) {
	allowed, err := s.queries.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "view", ResourceType: "scada_screen"})
	if err != nil {
		return nil, fmt.Errorf("check scada view permission: %w", err)
	}
	if !allowed {
		return nil, ErrForbidden
	}
	var filter any
	if strings.TrimSpace(plantID) != "" {
		id, parseErr := parseUUID(plantID)
		if parseErr != nil {
			return nil, ErrInvalid
		}
		filter = id
	}
	rows, err := s.pool.Query(ctx, `
SELECT s.id, s.organization_id, s.plant_id, p.code, p.name, s.name,
       s.draft_version, s.published_version, s.updated_at
FROM scada.scada_screen s
JOIN plant.plant p ON p.organization_id=s.organization_id AND p.id=s.plant_id
WHERE ($2::uuid IS NULL OR s.plant_id=$2)
  AND EXISTS (`+scadaPermissionExistsSQL+`)
ORDER BY p.name, s.name
LIMIT 200`, principal.UserID, filter, "view")
	if err != nil {
		return nil, fmt.Errorf("list scada screens: %w", err)
	}
	defer rows.Close()
	result := []ScadaScreenSummary{}
	for rows.Next() {
		summary, scanErr := scanScadaSummary(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, summary)
	}
	return result, rows.Err()
}

func (s *Service) CreateScadaScreen(ctx context.Context, principal auth.Principal, input CreateScadaScreenInput, sourceIP *netip.Addr) (ScadaScreen, error) {
	plantID, err := parseUUID(input.PlantID)
	input.Name = strings.TrimSpace(input.Name)
	if err != nil || input.Name == "" || len(input.Name) > 100 {
		return ScadaScreen{}, ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ScadaScreen{}, fmt.Errorf("begin scada create: %w", err)
	}
	defer tx.Rollback(ctx)
	organizationID, plantCode, plantName, err := authorizedScadaPlant(ctx, tx, principal, plantID, "edit")
	if err != nil {
		return ScadaScreen{}, err
	}
	id, err := newUUID()
	if err != nil {
		return ScadaScreen{}, err
	}
	design := DefaultScadaDesign()
	designJSON, _ := json.Marshal(design)
	var updatedAt pgtype.Timestamptz
	if err = tx.QueryRow(ctx, `
INSERT INTO scada.scada_screen(id, organization_id, plant_id, name, draft_design, created_by, updated_by)
VALUES($1,$2,$3,$4,$5,$6,$6)
RETURNING updated_at`, id, organizationID, plantID, input.Name, designJSON, principal.UserID).Scan(&updatedAt); err != nil {
		return ScadaScreen{}, mapScadaWriteError(err)
	}
	screen := ScadaScreen{ScadaScreenSummary: ScadaScreenSummary{ID: uuidString(id), OrganizationID: uuidString(organizationID), PlantID: uuidString(plantID), PlantCode: plantCode, PlantName: plantName, Name: input.Name, DraftVersion: 1, UpdatedAt: updatedAt.Time}, Design: design, CanEdit: true}
	if err = s.fillScadaCapabilities(ctx, tx, principal, organizationID, plantID, &screen); err != nil {
		return ScadaScreen{}, err
	}
	if err = auditScada(ctx, s.queries.WithTx(tx), principal, organizationID, id, "scada_screen.created", nil, screen, sourceIP); err != nil {
		return ScadaScreen{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ScadaScreen{}, fmt.Errorf("commit scada create: %w", err)
	}
	return screen, nil
}

func (s *Service) ScadaScreen(ctx context.Context, principal auth.Principal, screenID string) (ScadaScreen, error) {
	id, err := parseUUID(screenID)
	if err != nil {
		return ScadaScreen{}, ErrScadaNotFound
	}
	stored, err := getAuthorizedScadaScreen(ctx, s.pool, principal, id, "view", false)
	if err != nil {
		return ScadaScreen{}, err
	}
	screen := screenFromStored(stored)
	if err = s.fillScadaCapabilities(ctx, s.pool, principal, stored.OrgID, stored.PlantID, &screen); err != nil {
		return ScadaScreen{}, err
	}
	return screen, nil
}

func (s *Service) SaveScadaScreen(ctx context.Context, principal auth.Principal, screenID string, input UpdateScadaScreenInput, sourceIP *netip.Addr) (ScadaScreen, error) {
	id, err := parseUUID(screenID)
	input.Name = strings.TrimSpace(input.Name)
	if err != nil || input.DraftVersion < 1 || input.Name == "" || len(input.Name) > 100 || ValidateScadaDesign(input.Design) != nil {
		return ScadaScreen{}, ErrInvalid
	}
	designJSON, _ := json.Marshal(input.Design)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ScadaScreen{}, fmt.Errorf("begin scada save: %w", err)
	}
	defer tx.Rollback(ctx)
	stored, err := getAuthorizedScadaScreen(ctx, tx, principal, id, "edit", true)
	if err != nil {
		return ScadaScreen{}, err
	}
	if stored.Summary.DraftVersion != input.DraftVersion {
		return ScadaScreen{}, ErrScadaVersionConflict
	}
	if err = validateScadaBindings(ctx, tx, stored.OrgID, stored.PlantID, input.Design); err != nil {
		return ScadaScreen{}, err
	}
	before := screenFromStored(stored)
	var version int32
	var updatedAt pgtype.Timestamptz
	if err = tx.QueryRow(ctx, `
UPDATE scada.scada_screen
SET name=$2, draft_design=$3, draft_version=draft_version+1, updated_by=$4, updated_at=now()
WHERE id=$1
RETURNING draft_version, updated_at`, id, input.Name, designJSON, principal.UserID).Scan(&version, &updatedAt); err != nil {
		return ScadaScreen{}, mapScadaWriteError(err)
	}
	stored.Summary.Name, stored.Summary.DraftVersion, stored.Summary.UpdatedAt = input.Name, version, updatedAt.Time
	stored.Design, stored.Raw = input.Design, designJSON
	after := screenFromStored(stored)
	if err = s.fillScadaCapabilities(ctx, tx, principal, stored.OrgID, stored.PlantID, &after); err != nil {
		return ScadaScreen{}, err
	}
	if err = auditScada(ctx, s.queries.WithTx(tx), principal, stored.OrgID, id, "scada_screen.draft_saved", before, after, sourceIP); err != nil {
		return ScadaScreen{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ScadaScreen{}, fmt.Errorf("commit scada save: %w", err)
	}
	return after, nil
}

func (s *Service) PublishScadaScreen(ctx context.Context, principal auth.Principal, screenID string, input PublishScadaScreenInput, sourceIP *netip.Addr) (PublishedScadaScreen, error) {
	id, err := parseUUID(screenID)
	if err != nil || input.DraftVersion < 1 || input.PublishedVersion < 0 {
		return PublishedScadaScreen{}, ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PublishedScadaScreen{}, fmt.Errorf("begin scada publish: %w", err)
	}
	defer tx.Rollback(ctx)
	stored, err := getAuthorizedScadaScreen(ctx, tx, principal, id, "publish", true)
	if err != nil {
		return PublishedScadaScreen{}, err
	}
	if stored.Summary.DraftVersion != input.DraftVersion || stored.Summary.PublishedVersion != input.PublishedVersion {
		return PublishedScadaScreen{}, ErrScadaVersionConflict
	}
	var version int32
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1 FROM scada.scada_screen_publication WHERE screen_id=$1`, id).Scan(&version); err != nil {
		return PublishedScadaScreen{}, fmt.Errorf("next scada publication version: %w", err)
	}
	publicationID, err := newUUID()
	if err != nil {
		return PublishedScadaScreen{}, err
	}
	var publishedAt pgtype.Timestamptz
	if err = tx.QueryRow(ctx, `
INSERT INTO scada.scada_screen_publication(id, organization_id, plant_id, screen_id, version, source_draft_version, design, published_by)
VALUES($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING published_at`, publicationID, stored.OrgID, stored.PlantID, id, version, stored.Summary.DraftVersion, stored.Raw, principal.UserID).Scan(&publishedAt); err != nil {
		return PublishedScadaScreen{}, fmt.Errorf("insert scada publication: %w", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE scada.scada_screen SET published_version=$2, updated_by=$3, updated_at=now() WHERE id=$1`, id, version, principal.UserID); err != nil {
		return PublishedScadaScreen{}, fmt.Errorf("select scada publication: %w", err)
	}
	stored.Summary.PublishedVersion = version
	result := publishedFromStored(stored, stored.Summary.DraftVersion, stored.Design, publishedAt.Time)
	if err = auditScada(ctx, s.queries.WithTx(tx), principal, stored.OrgID, id, "scada_screen.published", nil, result, sourceIP); err != nil {
		return PublishedScadaScreen{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return PublishedScadaScreen{}, fmt.Errorf("commit scada publish: %w", err)
	}
	return result, nil
}

func (s *Service) PublishedScadaScreen(ctx context.Context, principal auth.Principal, screenID string) (PublishedScadaScreen, error) {
	id, err := parseUUID(screenID)
	if err != nil {
		return PublishedScadaScreen{}, ErrScadaNotFound
	}
	stored, err := getAuthorizedScadaScreen(ctx, s.pool, principal, id, "view", false)
	if err != nil {
		return PublishedScadaScreen{}, err
	}
	if stored.Summary.PublishedVersion < 1 {
		return PublishedScadaScreen{}, ErrScadaNotFound
	}
	return getScadaPublication(ctx, s.pool, stored, stored.Summary.PublishedVersion)
}

func (s *Service) ScadaScreenVersions(ctx context.Context, principal auth.Principal, screenID string) ([]ScadaScreenVersion, error) {
	id, err := parseUUID(screenID)
	if err != nil {
		return nil, ErrScadaNotFound
	}
	stored, err := getAuthorizedScadaScreen(ctx, s.pool, principal, id, "view", false)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT version, source_draft_version, published_by, published_at
FROM scada.scada_screen_publication
WHERE screen_id=$1
ORDER BY version DESC
LIMIT 100`, id)
	if err != nil {
		return nil, fmt.Errorf("list scada versions: %w", err)
	}
	defer rows.Close()
	result := []ScadaScreenVersion{}
	for rows.Next() {
		var version ScadaScreenVersion
		var publishedBy pgtype.UUID
		var publishedAt pgtype.Timestamptz
		if err = rows.Scan(&version.Version, &version.SourceDraftVersion, &publishedBy, &publishedAt); err != nil {
			return nil, fmt.Errorf("scan scada version: %w", err)
		}
		version.PublishedBy, version.PublishedAt, version.Current = uuidString(publishedBy), publishedAt.Time, version.Version == stored.Summary.PublishedVersion
		result = append(result, version)
	}
	return result, rows.Err()
}

func (s *Service) RollbackScadaScreen(ctx context.Context, principal auth.Principal, screenID string, input RollbackScadaScreenInput, sourceIP *netip.Addr) (PublishedScadaScreen, error) {
	id, err := parseUUID(screenID)
	if err != nil || input.TargetVersion < 1 || input.PublishedVersion < 1 {
		return PublishedScadaScreen{}, ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PublishedScadaScreen{}, fmt.Errorf("begin scada rollback: %w", err)
	}
	defer tx.Rollback(ctx)
	stored, err := getAuthorizedScadaScreen(ctx, tx, principal, id, "publish", true)
	if err != nil {
		return PublishedScadaScreen{}, err
	}
	if stored.Summary.PublishedVersion != input.PublishedVersion {
		return PublishedScadaScreen{}, ErrScadaVersionConflict
	}
	result, err := getScadaPublication(ctx, tx, stored, input.TargetVersion)
	if err != nil {
		return PublishedScadaScreen{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE scada.scada_screen SET published_version=$2, updated_by=$3, updated_at=now() WHERE id=$1`, id, input.TargetVersion, principal.UserID); err != nil {
		return PublishedScadaScreen{}, fmt.Errorf("rollback scada publication: %w", err)
	}
	before := map[string]int32{"publishedVersion": stored.Summary.PublishedVersion}
	stored.Summary.PublishedVersion = input.TargetVersion
	result.ScadaScreenSummary.PublishedVersion = input.TargetVersion
	if err = auditScada(ctx, s.queries.WithTx(tx), principal, stored.OrgID, id, "scada_screen.rolled_back", before, result, sourceIP); err != nil {
		return PublishedScadaScreen{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return PublishedScadaScreen{}, fmt.Errorf("commit scada rollback: %w", err)
	}
	return result, nil
}

func (s *Service) HardDeleteScadaScreen(ctx context.Context, principal auth.Principal, screenID, confirmation string, sourceIP *netip.Addr) error {
	id, err := parseUUID(screenID)
	if err != nil {
		return ErrScadaNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin hard delete scada screen: %w", err)
	}
	defer tx.Rollback(ctx)
	allowed, err := hasGlobalPermissionQuery(ctx, tx, principal, "hard_delete", "scada_screen")
	if err != nil {
		return fmt.Errorf("check global scada hard delete permission: %w", err)
	}
	if !allowed {
		return ErrForbidden
	}
	stored, err := getScadaScreenByID(ctx, tx, id, true)
	if err != nil {
		return err
	}
	if !validHardDeleteConfirmation(confirmation, "DELETE SCADA "+stored.Summary.Name) {
		return ErrInvalid
	}
	if err = auditScada(ctx, s.queries.WithTx(tx), principal, stored.OrgID, id, "scada_screen.hard_deleted", screenFromStored(stored), map[string]bool{"hardDeleted": true}, sourceIP); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM scada.scada_screen WHERE id=$1`, id); err != nil {
		return fmt.Errorf("hard delete scada screen: %w", err)
	}
	return commitHardDelete(ctx, tx, "scada screen")
}

const scadaPermissionExistsSQL = `
SELECT 1 FROM auth.user_role ur
JOIN auth.role r ON r.id=ur.role_id
JOIN auth.role_permission rp ON rp.role_id=ur.role_id
JOIN auth.permission pm ON pm.id=rp.permission_id
WHERE ur.user_id=$1
  AND pm.action=$3 AND pm.resource_type='scada_screen'
  AND (r.organization_id IS NULL OR r.organization_id=ur.organization_id)
  AND (rp.organization_id IS NULL OR rp.organization_id=ur.organization_id)
  AND (ur.organization_id IS NULL OR ur.organization_id=s.organization_id)
  AND (ur.plant_id IS NULL OR ur.plant_id=s.plant_id)`

func authorizedScadaPlant(ctx context.Context, querier rowQuerier, principal auth.Principal, plantID pgtype.UUID, action string) (pgtype.UUID, string, string, error) {
	var organizationID pgtype.UUID
	var code, name string
	err := querier.QueryRow(ctx, `
SELECT p.organization_id, p.code, p.name
FROM plant.plant p
WHERE p.id=$2
  AND EXISTS (
      SELECT 1 FROM auth.user_role ur
      JOIN auth.role r ON r.id=ur.role_id
      JOIN auth.role_permission rp ON rp.role_id=ur.role_id
      JOIN auth.permission pm ON pm.id=rp.permission_id
      WHERE ur.user_id=$1 AND pm.action=$3 AND pm.resource_type='scada_screen'
        AND (r.organization_id IS NULL OR r.organization_id=ur.organization_id)
        AND (rp.organization_id IS NULL OR rp.organization_id=ur.organization_id)
        AND (ur.organization_id IS NULL OR ur.organization_id=p.organization_id)
        AND (ur.plant_id IS NULL OR ur.plant_id=p.id)
  )
FOR UPDATE OF p`, principal.UserID, plantID, action).Scan(&organizationID, &code, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, "", "", ErrScadaNotFound
	}
	if err != nil {
		return pgtype.UUID{}, "", "", fmt.Errorf("authorize scada plant: %w", err)
	}
	return organizationID, code, name, nil
}

func getAuthorizedScadaScreen(ctx context.Context, querier rowQuerier, principal auth.Principal, id pgtype.UUID, action string, lock bool) (storedScadaScreen, error) {
	query := `
SELECT s.id, s.organization_id, s.plant_id, p.code, p.name, s.name, s.draft_design,
       s.draft_version, s.published_version, s.updated_at
FROM scada.scada_screen s
JOIN plant.plant p ON p.organization_id=s.organization_id AND p.id=s.plant_id
WHERE s.id=$2 AND EXISTS (` + scadaPermissionExistsSQL + `)`
	if lock {
		query += " FOR UPDATE OF s"
	}
	return scanStoredScada(querier.QueryRow(ctx, query, principal.UserID, id, action))
}

func getScadaScreenByID(ctx context.Context, querier rowQuerier, id pgtype.UUID, lock bool) (storedScadaScreen, error) {
	query := `
SELECT s.id, s.organization_id, s.plant_id, p.code, p.name, s.name, s.draft_design,
       s.draft_version, s.published_version, s.updated_at
FROM scada.scada_screen s
JOIN plant.plant p ON p.organization_id=s.organization_id AND p.id=s.plant_id
WHERE s.id=$1`
	if lock {
		query += " FOR UPDATE OF s"
	}
	return scanStoredScada(querier.QueryRow(ctx, query, id))
}

func scanStoredScada(row interface{ Scan(...any) error }) (storedScadaScreen, error) {
	var stored storedScadaScreen
	var updatedAt pgtype.Timestamptz
	err := row.Scan(&stored.ID, &stored.OrgID, &stored.PlantID, &stored.Summary.PlantCode, &stored.Summary.PlantName, &stored.Summary.Name, &stored.Raw, &stored.Summary.DraftVersion, &stored.Summary.PublishedVersion, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return storedScadaScreen{}, ErrScadaNotFound
	}
	if err != nil {
		return storedScadaScreen{}, fmt.Errorf("scan scada screen: %w", err)
	}
	if err = json.Unmarshal(stored.Raw, &stored.Design); err != nil || ValidateScadaDesign(stored.Design) != nil {
		return storedScadaScreen{}, errors.New("invalid persisted scada design")
	}
	stored.Summary.ID, stored.Summary.OrganizationID, stored.Summary.PlantID = uuidString(stored.ID), uuidString(stored.OrgID), uuidString(stored.PlantID)
	stored.Summary.UpdatedAt = updatedAt.Time
	return stored, nil
}

func scanScadaSummary(row interface{ Scan(...any) error }) (ScadaScreenSummary, error) {
	var summary ScadaScreenSummary
	var id, organizationID, plantID pgtype.UUID
	var updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &organizationID, &plantID, &summary.PlantCode, &summary.PlantName, &summary.Name, &summary.DraftVersion, &summary.PublishedVersion, &updatedAt); err != nil {
		return summary, fmt.Errorf("scan scada summary: %w", err)
	}
	summary.ID, summary.OrganizationID, summary.PlantID, summary.UpdatedAt = uuidString(id), uuidString(organizationID), uuidString(plantID), updatedAt.Time
	return summary, nil
}

func screenFromStored(stored storedScadaScreen) ScadaScreen {
	return ScadaScreen{ScadaScreenSummary: stored.Summary, Design: stored.Design}
}

func (s *Service) fillScadaCapabilities(ctx context.Context, querier rowQuerier, principal auth.Principal, organizationID, plantID pgtype.UUID, screen *ScadaScreen) error {
	var err error
	if screen.CanEdit, err = hasScadaScopePermission(ctx, querier, principal, organizationID, plantID, "edit"); err != nil {
		return err
	}
	if screen.CanPublish, err = hasScadaScopePermission(ctx, querier, principal, organizationID, plantID, "publish"); err != nil {
		return err
	}
	if screen.CanManageAccess, err = hasScadaScopePermission(ctx, querier, principal, organizationID, plantID, "manage_access"); err != nil {
		return err
	}
	screen.CanHardDelete, err = hasGlobalPermissionQuery(ctx, querier, principal, "hard_delete", "scada_screen")
	return err
}

func hasScadaScopePermission(ctx context.Context, querier rowQuerier, principal auth.Principal, organizationID, plantID pgtype.UUID, action string) (bool, error) {
	var allowed bool
	err := querier.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1 FROM auth.user_role ur
    JOIN auth.role r ON r.id=ur.role_id
    JOIN auth.role_permission rp ON rp.role_id=ur.role_id
    JOIN auth.permission pm ON pm.id=rp.permission_id
    WHERE ur.user_id=$1 AND pm.action=$2 AND pm.resource_type='scada_screen'
      AND (r.organization_id IS NULL OR r.organization_id=ur.organization_id)
      AND (rp.organization_id IS NULL OR rp.organization_id=ur.organization_id)
      AND (ur.organization_id IS NULL OR ur.organization_id=$3)
      AND (ur.plant_id IS NULL OR ur.plant_id=$4)
)`, principal.UserID, action, organizationID, plantID).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("check scada %s permission: %w", action, err)
	}
	return allowed, nil
}

func validateScadaBindings(ctx context.Context, querier rowQuerier, organizationID, plantID pgtype.UUID, design ScadaDesign) error {
	seen := map[string]bool{}
	checkDevice := func(rawDeviceID string) error {
		if seen[rawDeviceID] {
			return nil
		}
		seen[rawDeviceID] = true
		deviceID, _ := parseUUID(rawDeviceID)
		var exists bool
		if err := querier.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM plant.device WHERE organization_id=$1 AND plant_id=$2 AND id=$3)`, organizationID, plantID, deviceID).Scan(&exists); err != nil {
			return fmt.Errorf("validate scada binding device: %w", err)
		}
		if !exists {
			return ErrInvalid
		}
		return nil
	}
	for _, node := range design.Nodes {
		deviceIDs := make([]string, 0, len(node.Data.Items)+2)
		if node.Data.Binding != nil {
			deviceIDs = append(deviceIDs, node.Data.Binding.DeviceID)
		}
		if node.Data.DeviceID != "" {
			deviceIDs = append(deviceIDs, node.Data.DeviceID)
		}
		for _, item := range node.Data.Items {
			deviceIDs = append(deviceIDs, item.Binding.DeviceID)
		}
		for _, deviceID := range deviceIDs {
			if err := checkDevice(deviceID); err != nil {
				return err
			}
		}
	}
	return nil
}

func getScadaPublication(ctx context.Context, querier rowQuerier, stored storedScadaScreen, version int32) (PublishedScadaScreen, error) {
	var sourceVersion int32
	var raw []byte
	var publishedAt pgtype.Timestamptz
	err := querier.QueryRow(ctx, `SELECT source_draft_version, design, published_at FROM scada.scada_screen_publication WHERE screen_id=$1 AND version=$2`, stored.ID, version).Scan(&sourceVersion, &raw, &publishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublishedScadaScreen{}, ErrScadaNotFound
	}
	if err != nil {
		return PublishedScadaScreen{}, fmt.Errorf("get scada publication: %w", err)
	}
	var design ScadaDesign
	if err = json.Unmarshal(raw, &design); err != nil || ValidateScadaDesign(design) != nil {
		return PublishedScadaScreen{}, errors.New("invalid persisted scada publication")
	}
	stored.Summary.PublishedVersion = version
	return publishedFromStored(stored, sourceVersion, design, publishedAt.Time), nil
}

func publishedFromStored(stored storedScadaScreen, sourceVersion int32, design ScadaDesign, publishedAt time.Time) PublishedScadaScreen {
	return PublishedScadaScreen{ScadaScreenSummary: stored.Summary, SourceDraftVersion: sourceVersion, Design: design, PublishedAt: publishedAt}
}

func auditScada(ctx context.Context, q *dbgen.Queries, principal auth.Principal, organizationID, targetID pgtype.UUID, action string, before, after any, sourceIP *netip.Addr) error {
	correlationID, err := newUUID()
	if err != nil {
		return err
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{OrganizationID: organizationID, ActorUserID: principal.UserID, Action: action, TargetType: "scada_screen", TargetID: targetID, BeforeData: beforeJSON, AfterData: afterJSON, SourceIp: sourceIP, CorrelationID: correlationID}); err != nil {
		return fmt.Errorf("audit scada change: %w", err)
	}
	return nil
}

func mapScadaWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrScadaConflict
		case "23503", "23514", "22023":
			return ErrInvalid
		}
	}
	return fmt.Errorf("write scada screen: %w", err)
}

func validScadaID(value string) bool {
	return strings.TrimSpace(value) == value && value != "" && len(value) <= 100
}

func finiteBetween(value, minimum, maximum float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= minimum && value <= maximum
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
