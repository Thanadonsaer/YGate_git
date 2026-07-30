package core

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/database/dbgen"
)

type DashboardOverview struct {
	GeneratedAt          time.Time              `json:"generatedAt"`
	StaleAfterSeconds    int64                  `json:"staleAfterSeconds"`
	PlantCount           int64                  `json:"plantCount"`
	ActiveDeviceCount    int64                  `json:"activeDeviceCount"`
	ReportingDeviceCount int64                  `json:"reportingDeviceCount"`
	StaleDeviceCount     int64                  `json:"staleDeviceCount"`
	OfflineDeviceCount   int64                  `json:"offlineDeviceCount"`
	Plants               []DashboardPlantStatus `json:"plants"`
}

type DashboardPlantStatus struct {
	PlantID              string     `json:"plantId"`
	Code                 string     `json:"code"`
	Name                 string     `json:"name"`
	Timezone             string     `json:"timezone"`
	IsActive             bool       `json:"isActive"`
	DeviceCount          int64      `json:"deviceCount"`
	ActiveDeviceCount    int64      `json:"activeDeviceCount"`
	ReportingDeviceCount int64      `json:"reportingDeviceCount"`
	StaleDeviceCount     int64      `json:"staleDeviceCount"`
	OfflineDeviceCount   int64      `json:"offlineDeviceCount"`
	LastObservedAt       *time.Time `json:"lastObservedAt"`
	CommunicationStatus  string     `json:"communicationStatus"`
}

func (s *Service) DashboardOverview(ctx context.Context, principal auth.Principal, staleAfter time.Duration, now time.Time) (DashboardOverview, error) {
	if staleAfter < 30*time.Second || staleAfter > 24*time.Hour {
		return DashboardOverview{}, ErrInvalid
	}
	allowed, err := s.queries.HasUserPermission(ctx, dbgen.HasUserPermissionParams{
		UserID: principal.UserID, Action: "read", ResourceType: "device",
	})
	if err != nil {
		return DashboardOverview{}, fmt.Errorf("check dashboard permission: %w", err)
	}
	if !allowed {
		return DashboardOverview{}, ErrForbidden
	}
	now = now.UTC()
	rows, err := s.queries.ListDashboardPlantStatus(ctx, dbgen.ListDashboardPlantStatusParams{
		StaleBefore: pgtype.Timestamptz{Time: now.Add(-staleAfter), Valid: true}, UserID: principal.UserID,
	})
	if err != nil {
		return DashboardOverview{}, fmt.Errorf("list dashboard status: %w", err)
	}
	overview := DashboardOverview{GeneratedAt: now, StaleAfterSeconds: int64(staleAfter / time.Second), PlantCount: int64(len(rows)), Plants: make([]DashboardPlantStatus, 0, len(rows))}
	for _, row := range rows {
		plant := DashboardPlantStatus{
			PlantID: uuidString(row.PlantID), Code: row.Code, Name: row.Name, Timezone: row.Timezone, IsActive: row.IsActive,
			DeviceCount: row.DeviceCount, ActiveDeviceCount: row.ActiveDeviceCount, ReportingDeviceCount: row.ReportingDeviceCount,
			StaleDeviceCount: row.StaleDeviceCount, OfflineDeviceCount: row.OfflineDeviceCount,
			LastObservedAt: timePointer(row.LastObservedAt),
		}
		plant.CommunicationStatus = dashboardCommunicationStatus(plant)
		overview.Plants = append(overview.Plants, plant)
		if plant.IsActive {
			overview.ActiveDeviceCount += plant.ActiveDeviceCount
			overview.ReportingDeviceCount += plant.ReportingDeviceCount
			overview.StaleDeviceCount += plant.StaleDeviceCount
			overview.OfflineDeviceCount += plant.OfflineDeviceCount
		}
	}
	return overview, nil
}

func dashboardCommunicationStatus(plant DashboardPlantStatus) string {
	switch {
	case !plant.IsActive:
		return "DISABLED"
	case plant.ActiveDeviceCount == 0:
		return "NO_DEVICES"
	case plant.ReportingDeviceCount == 0:
		return "OFFLINE"
	case plant.StaleDeviceCount > 0 || plant.OfflineDeviceCount > 0:
		return "DEGRADED"
	default:
		return "ONLINE"
	}
}

func timePointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	copy := value.Time
	return &copy
}
