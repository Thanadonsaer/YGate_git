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
	"github.com/jackc/pgx/v5/pgconn"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/database/dbgen"
)

const dashboardRegistryVersion = 2

const timeseriesWidgetID = "timeseries-line"

var ErrDashboardVersionConflict = errors.New("dashboard layout version conflict")

type DashboardWidget struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
}

type DashboardLayoutItem struct {
	I string `json:"i"`
	X int    `json:"x"`
	Y int    `json:"y"`
	W int    `json:"w"`
	H int    `json:"h"`
}

type DashboardResponsiveLayouts map[string][]DashboardLayoutItem

type DashboardWidgetConfig struct {
	Version     int                        `json:"version"`
	DataBinding DashboardWidgetDataBinding `json:"dataBinding"`
	Display     DashboardWidgetDisplay     `json:"display"`
}

type DashboardWidgetDataBinding struct {
	PlantID        string `json:"plantId"`
	DeviceID       string `json:"deviceId"`
	PointKey       string `json:"pointKey"`
	TimeRangeHours int    `json:"timeRangeHours"`
}

type DashboardWidgetDisplay struct {
	Unit     string `json:"unit"`
	Decimals int    `json:"decimals"`
}

type DashboardWidgetConfigs map[string]DashboardWidgetConfig

type DashboardLayout struct {
	ID              *string                    `json:"id"`
	Version         int32                      `json:"version"`
	RegistryVersion int                        `json:"registryVersion"`
	Widgets         []DashboardWidget          `json:"widgets"`
	Layouts         DashboardResponsiveLayouts `json:"layouts"`
	WidgetConfigs   DashboardWidgetConfigs     `json:"widgetConfigs"`
	CanEdit         bool                       `json:"canEdit"`
	CanPublish      bool                       `json:"canPublish"`
	UpdatedAt       *time.Time                 `json:"updatedAt"`
}

type UpdateDashboardLayoutInput struct {
	Version       int32                      `json:"version"`
	Layouts       DashboardResponsiveLayouts `json:"layouts"`
	WidgetConfigs DashboardWidgetConfigs     `json:"widgetConfigs"`
}

type PublishedDashboardLayout struct {
	ID              *string                    `json:"id"`
	Version         int32                      `json:"version"`
	SourceVersion   int32                      `json:"sourceVersion"`
	RegistryVersion int                        `json:"registryVersion"`
	Widgets         []DashboardWidget          `json:"widgets"`
	Layouts         DashboardResponsiveLayouts `json:"layouts"`
	WidgetConfigs   DashboardWidgetConfigs     `json:"widgetConfigs"`
	PublishedAt     *time.Time                 `json:"publishedAt"`
}

type PublishDashboardLayoutInput struct {
	Version          int32 `json:"version"`
	PublishedVersion int32 `json:"publishedVersion"`
}

var dashboardWidgets = []DashboardWidget{
	{ID: "plant-count", Type: "plant-count", Title: "Plants"},
	{ID: "active-device-count", Type: "active-device-count", Title: "Active devices"},
	{ID: "reporting-device-count", Type: "reporting-device-count", Title: "Reporting"},
	{ID: "attention-device-count", Type: "attention-device-count", Title: "Needs attention"},
	{ID: "plant-status", Type: "plant-status", Title: "Plant status"},
	{ID: timeseriesWidgetID, Type: "timeseries.line", Title: "Timeseries"},
}

var requiredDashboardWidgetIDs = []string{"plant-count", "active-device-count", "reporting-device-count", "attention-device-count", "plant-status"}

func DefaultDashboardLayouts() DashboardResponsiveLayouts {
	return DashboardResponsiveLayouts{
		"lg": {
			{I: "plant-count", X: 0, Y: 0, W: 3, H: 2},
			{I: "active-device-count", X: 3, Y: 0, W: 3, H: 2},
			{I: "reporting-device-count", X: 6, Y: 0, W: 3, H: 2},
			{I: "attention-device-count", X: 9, Y: 0, W: 3, H: 2},
			{I: "plant-status", X: 0, Y: 2, W: 12, H: 6},
		},
		"md": {
			{I: "plant-count", X: 0, Y: 0, W: 4, H: 2},
			{I: "active-device-count", X: 4, Y: 0, W: 4, H: 2},
			{I: "reporting-device-count", X: 0, Y: 2, W: 4, H: 2},
			{I: "attention-device-count", X: 4, Y: 2, W: 4, H: 2},
			{I: "plant-status", X: 0, Y: 4, W: 8, H: 6},
		},
		"sm": {
			{I: "plant-count", X: 0, Y: 0, W: 4, H: 2},
			{I: "active-device-count", X: 0, Y: 2, W: 4, H: 2},
			{I: "reporting-device-count", X: 0, Y: 4, W: 4, H: 2},
			{I: "attention-device-count", X: 0, Y: 6, W: 4, H: 2},
			{I: "plant-status", X: 0, Y: 8, W: 4, H: 8},
		},
	}
}

func ValidateDashboardLayouts(layouts DashboardResponsiveLayouts) error {
	columns := map[string]int{"lg": 12, "md": 8, "sm": 4}
	if len(layouts) != len(columns) {
		return ErrInvalid
	}
	validWidgets := make(map[string]bool, len(dashboardWidgets))
	for _, widget := range dashboardWidgets {
		validWidgets[widget.ID] = true
	}
	var expected map[string]bool
	for _, breakpoint := range []string{"lg", "md", "sm"} {
		cols := columns[breakpoint]
		items, ok := layouts[breakpoint]
		if !ok || len(items) < len(requiredDashboardWidgetIDs) || len(items) > len(dashboardWidgets) {
			return ErrInvalid
		}
		seen := make(map[string]bool, len(items))
		for index, item := range items {
			if !validWidgets[item.I] || seen[item.I] || item.X < 0 || item.Y < 0 || item.W < 1 || item.H < 1 || item.X+item.W > cols || item.Y > 1000 || item.H > 100 {
				return ErrInvalid
			}
			seen[item.I] = true
			for _, other := range items[:index] {
				if item.X < other.X+other.W && item.X+item.W > other.X && item.Y < other.Y+other.H && item.Y+item.H > other.Y {
					return ErrInvalid
				}
			}
		}
		for _, id := range requiredDashboardWidgetIDs {
			if !seen[id] {
				return ErrInvalid
			}
		}
		if expected == nil {
			expected = seen
		} else if len(expected) != len(seen) {
			return ErrInvalid
		} else {
			for id := range expected {
				if !seen[id] {
					return ErrInvalid
				}
			}
		}
	}
	return nil
}

func ValidateDashboardWidgetConfigs(layouts DashboardResponsiveLayouts, configs DashboardWidgetConfigs) error {
	if len(configs) > 1 {
		return ErrInvalid
	}
	for id := range configs {
		if id != timeseriesWidgetID {
			return ErrInvalid
		}
	}
	config, configured := configs[timeseriesWidgetID]
	chartPresent := false
	for _, item := range layouts["lg"] {
		chartPresent = chartPresent || item.I == timeseriesWidgetID
	}
	if chartPresent != configured {
		return ErrInvalid
	}
	if !configured {
		return nil
	}
	if config.Version != 1 || strings.TrimSpace(config.DataBinding.PlantID) != config.DataBinding.PlantID || strings.TrimSpace(config.DataBinding.DeviceID) != config.DataBinding.DeviceID || strings.TrimSpace(config.DataBinding.PointKey) != config.DataBinding.PointKey || config.DataBinding.PointKey == "" || len(config.DataBinding.PointKey) > 200 || len(config.Display.Unit) > 20 || config.Display.Decimals < 0 || config.Display.Decimals > 6 {
		return ErrInvalid
	}
	if _, err := parseUUID(config.DataBinding.PlantID); err != nil {
		return ErrInvalid
	}
	if _, err := parseUUID(config.DataBinding.DeviceID); err != nil {
		return ErrInvalid
	}
	switch config.DataBinding.TimeRangeHours {
	case 1, 6, 24, 168:
		return nil
	default:
		return ErrInvalid
	}
}

func (s *Service) DashboardLayout(ctx context.Context, principal auth.Principal) (DashboardLayout, error) {
	allowed, err := s.queries.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "read", ResourceType: "dashboard"})
	if err != nil {
		return DashboardLayout{}, fmt.Errorf("check dashboard read permission: %w", err)
	}
	if !allowed {
		return DashboardLayout{}, ErrForbidden
	}
	canEdit, err := s.queries.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "update", ResourceType: "dashboard"})
	if err != nil {
		return DashboardLayout{}, fmt.Errorf("check dashboard update permission: %w", err)
	}
	canPublish, err := s.queries.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "publish", ResourceType: "dashboard"})
	if err != nil {
		return DashboardLayout{}, fmt.Errorf("check dashboard publish permission: %w", err)
	}
	row, err := s.queries.GetUserDashboard(ctx, dbgen.GetUserDashboardParams{OrganizationID: principal.OrganizationID, OwnerUserID: principal.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return dashboardLayoutResponse(nil, 0, DefaultDashboardLayouts(), DashboardWidgetConfigs{}, nil, canEdit, canPublish), nil
	}
	if err != nil {
		return DashboardLayout{}, fmt.Errorf("get dashboard layout: %w", err)
	}
	result, err := dashboardLayoutFromRow(row)
	result.CanEdit, result.CanPublish = canEdit, canPublish
	return result, err
}

func (s *Service) PublishedDashboardLayout(ctx context.Context, principal auth.Principal) (PublishedDashboardLayout, error) {
	allowed, err := s.queries.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "read", ResourceType: "dashboard"})
	if err != nil {
		return PublishedDashboardLayout{}, fmt.Errorf("check dashboard read permission: %w", err)
	}
	if !allowed {
		return PublishedDashboardLayout{}, ErrForbidden
	}
	row, err := s.queries.GetUserDashboard(ctx, dbgen.GetUserDashboardParams{OrganizationID: principal.OrganizationID, OwnerUserID: principal.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return publishedDashboardLayoutResponse(nil, 0, 0, DefaultDashboardLayouts(), DashboardWidgetConfigs{}, nil), nil
	}
	if err != nil {
		return PublishedDashboardLayout{}, fmt.Errorf("get published dashboard layout: %w", err)
	}
	return publishedDashboardLayoutFromRow(row)
}

func (s *Service) SaveDashboardLayout(ctx context.Context, principal auth.Principal, input UpdateDashboardLayoutInput, sourceIP *netip.Addr) (DashboardLayout, error) {
	if input.WidgetConfigs == nil {
		input.WidgetConfigs = DashboardWidgetConfigs{}
	}
	if input.Version < 0 || ValidateDashboardLayouts(input.Layouts) != nil || ValidateDashboardWidgetConfigs(input.Layouts, input.WidgetConfigs) != nil || !principal.OrganizationID.Valid {
		return DashboardLayout{}, ErrInvalid
	}
	layoutJSON, err := json.Marshal(input.Layouts)
	if err != nil {
		return DashboardLayout{}, ErrInvalid
	}
	configJSON, err := json.Marshal(input.WidgetConfigs)
	if err != nil {
		return DashboardLayout{}, ErrInvalid
	}
	correlationID, err := newUUID()
	if err != nil {
		return DashboardLayout{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DashboardLayout{}, fmt.Errorf("begin dashboard update: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	allowed, err := q.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "update", ResourceType: "dashboard"})
	if err != nil {
		return DashboardLayout{}, fmt.Errorf("check dashboard update permission: %w", err)
	}
	if !allowed {
		return DashboardLayout{}, ErrForbidden
	}

	current, err := q.GetUserDashboardForUpdate(ctx, dbgen.GetUserDashboardParams{OrganizationID: principal.OrganizationID, OwnerUserID: principal.UserID})
	var row dbgen.GetUserDashboardRow
	action := "dashboard.updated"
	var before []byte
	if errors.Is(err, pgx.ErrNoRows) {
		if input.Version != 0 {
			return DashboardLayout{}, ErrDashboardVersionConflict
		}
		id, idErr := newUUID()
		if idErr != nil {
			return DashboardLayout{}, idErr
		}
		row, err = q.CreateUserDashboard(ctx, dbgen.CreateUserDashboardParams{ID: id, OrganizationID: principal.OrganizationID, OwnerUserID: principal.UserID, Layouts: layoutJSON, WidgetConfigs: configJSON})
		action = "dashboard.created"
	} else if err != nil {
		return DashboardLayout{}, fmt.Errorf("lock dashboard layout: %w", err)
	} else {
		if current.Version != input.Version {
			return DashboardLayout{}, ErrDashboardVersionConflict
		}
		before = current.Layouts
		row, err = q.UpdateUserDashboard(ctx, dbgen.UpdateUserDashboardParams{Layouts: layoutJSON, WidgetConfigs: configJSON, ID: current.ID})
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return DashboardLayout{}, ErrDashboardVersionConflict
		}
		return DashboardLayout{}, fmt.Errorf("save dashboard layout: %w", err)
	}
	result, err := dashboardLayoutFromRow(row)
	if err != nil {
		return DashboardLayout{}, err
	}
	result.CanEdit = true
	result.CanPublish, err = q.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "publish", ResourceType: "dashboard"})
	if err != nil {
		return DashboardLayout{}, fmt.Errorf("check dashboard publish permission: %w", err)
	}
	after, _ := json.Marshal(result)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: principal.OrganizationID, ActorUserID: principal.UserID, Action: action,
		TargetType: "dashboard", TargetID: row.ID, BeforeData: before, AfterData: after,
		SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return DashboardLayout{}, fmt.Errorf("audit dashboard update: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return DashboardLayout{}, fmt.Errorf("commit dashboard update: %w", err)
	}
	return result, nil
}

func (s *Service) PublishDashboardLayout(ctx context.Context, principal auth.Principal, input PublishDashboardLayoutInput, sourceIP *netip.Addr) (PublishedDashboardLayout, error) {
	if input.Version < 1 || input.PublishedVersion < 0 || !principal.OrganizationID.Valid {
		return PublishedDashboardLayout{}, ErrInvalid
	}
	correlationID, err := newUUID()
	if err != nil {
		return PublishedDashboardLayout{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PublishedDashboardLayout{}, fmt.Errorf("begin dashboard publish: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	allowed, err := q.HasUserPermission(ctx, dbgen.HasUserPermissionParams{UserID: principal.UserID, Action: "publish", ResourceType: "dashboard"})
	if err != nil {
		return PublishedDashboardLayout{}, fmt.Errorf("check dashboard publish permission: %w", err)
	}
	if !allowed {
		return PublishedDashboardLayout{}, ErrForbidden
	}
	current, err := q.GetUserDashboardForUpdate(ctx, dbgen.GetUserDashboardParams{OrganizationID: principal.OrganizationID, OwnerUserID: principal.UserID})
	if errors.Is(err, pgx.ErrNoRows) || err == nil && (current.Version != input.Version || current.PublishedVersion != input.PublishedVersion) {
		return PublishedDashboardLayout{}, ErrDashboardVersionConflict
	}
	if err != nil {
		return PublishedDashboardLayout{}, fmt.Errorf("lock dashboard for publish: %w", err)
	}
	before, err := publishedDashboardLayoutFromRow(current)
	if err != nil {
		return PublishedDashboardLayout{}, err
	}
	row, err := q.PublishUserDashboard(ctx, current.ID)
	if err != nil {
		return PublishedDashboardLayout{}, fmt.Errorf("publish dashboard layout: %w", err)
	}
	result, err := publishedDashboardLayoutFromRow(row)
	if err != nil {
		return PublishedDashboardLayout{}, err
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(result)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: principal.OrganizationID, ActorUserID: principal.UserID, Action: "dashboard.published",
		TargetType: "dashboard", TargetID: row.ID, BeforeData: beforeJSON, AfterData: afterJSON,
		SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return PublishedDashboardLayout{}, fmt.Errorf("audit dashboard publish: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return PublishedDashboardLayout{}, fmt.Errorf("commit dashboard publish: %w", err)
	}
	return result, nil
}

func dashboardLayoutFromRow(row dbgen.GetUserDashboardRow) (DashboardLayout, error) {
	var layouts DashboardResponsiveLayouts
	var configs DashboardWidgetConfigs
	if err := json.Unmarshal(row.Layouts, &layouts); err != nil || json.Unmarshal(row.WidgetConfigs, &configs) != nil || ValidateDashboardLayouts(layouts) != nil || ValidateDashboardWidgetConfigs(layouts, configs) != nil {
		return DashboardLayout{}, errors.New("invalid persisted dashboard layout")
	}
	id := uuidString(row.ID)
	updatedAt := row.UpdatedAt.Time
	return dashboardLayoutResponse(&id, row.Version, layouts, configs, &updatedAt, false, false), nil
}

func dashboardLayoutResponse(id *string, version int32, layouts DashboardResponsiveLayouts, configs DashboardWidgetConfigs, updatedAt *time.Time, canEdit, canPublish bool) DashboardLayout {
	return DashboardLayout{ID: id, Version: version, RegistryVersion: dashboardRegistryVersion, Widgets: append([]DashboardWidget(nil), dashboardWidgets...), Layouts: layouts, WidgetConfigs: configs, CanEdit: canEdit, CanPublish: canPublish, UpdatedAt: updatedAt}
}

func publishedDashboardLayoutFromRow(row dbgen.GetUserDashboardRow) (PublishedDashboardLayout, error) {
	id := uuidString(row.ID)
	if len(row.PublishedLayouts) == 0 {
		return publishedDashboardLayoutResponse(&id, 0, 0, DefaultDashboardLayouts(), DashboardWidgetConfigs{}, nil), nil
	}
	var layouts DashboardResponsiveLayouts
	var configs DashboardWidgetConfigs
	if err := json.Unmarshal(row.PublishedLayouts, &layouts); err != nil || json.Unmarshal(row.PublishedWidgetConfigs, &configs) != nil || ValidateDashboardLayouts(layouts) != nil || ValidateDashboardWidgetConfigs(layouts, configs) != nil {
		return PublishedDashboardLayout{}, errors.New("invalid persisted published dashboard layout")
	}
	publishedAt := row.PublishedAt.Time
	return publishedDashboardLayoutResponse(&id, row.PublishedVersion, row.PublishedFromVersion, layouts, configs, &publishedAt), nil
}

func publishedDashboardLayoutResponse(id *string, version, sourceVersion int32, layouts DashboardResponsiveLayouts, configs DashboardWidgetConfigs, publishedAt *time.Time) PublishedDashboardLayout {
	return PublishedDashboardLayout{ID: id, Version: version, SourceVersion: sourceVersion, RegistryVersion: dashboardRegistryVersion, Widgets: append([]DashboardWidget(nil), dashboardWidgets...), Layouts: layouts, WidgetConfigs: configs, PublishedAt: publishedAt}
}
