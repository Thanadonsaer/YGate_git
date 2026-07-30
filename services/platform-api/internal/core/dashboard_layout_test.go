package core

import (
	"errors"
	"testing"
)

func TestValidateDashboardLayouts(t *testing.T) {
	if err := ValidateDashboardLayouts(DefaultDashboardLayouts()); err != nil {
		t.Fatalf("default layout error = %v", err)
	}
	tests := map[string]func(DashboardResponsiveLayouts){
		"missing breakpoint": func(layouts DashboardResponsiveLayouts) { delete(layouts, "sm") },
		"duplicate widget":   func(layouts DashboardResponsiveLayouts) { layouts["lg"][1].I = layouts["lg"][0].I },
		"unknown widget":     func(layouts DashboardResponsiveLayouts) { layouts["lg"][0].I = "unknown" },
		"outside columns":    func(layouts DashboardResponsiveLayouts) { layouts["sm"][0].W = 5 },
		"overlap":            func(layouts DashboardResponsiveLayouts) { layouts["lg"][1].X = 0 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			layouts := DefaultDashboardLayouts()
			mutate(layouts)
			if err := ValidateDashboardLayouts(layouts); !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestValidateDashboardWidgetConfigs(t *testing.T) {
	layouts := DefaultDashboardLayouts()
	addTestChart(layouts)
	valid := DashboardWidgetConfigs{timeseriesWidgetID: {
		Version:     1,
		DataBinding: DashboardWidgetDataBinding{PlantID: "10000000-0000-4000-8000-000000000001", DeviceID: "10000000-0000-4000-8000-000000000002", PointKey: "active_power", TimeRangeHours: 24},
		Display:     DashboardWidgetDisplay{Unit: "kW", Decimals: 1},
	}}
	if err := ValidateDashboardWidgetConfigs(layouts, valid); err != nil {
		t.Fatalf("valid config error = %v", err)
	}
	config := valid[timeseriesWidgetID]
	config.DataBinding.TimeRangeHours = 2
	invalid := DashboardWidgetConfigs{timeseriesWidgetID: config}
	if err := ValidateDashboardWidgetConfigs(layouts, invalid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid range error = %v", err)
	}
	withoutChart := DefaultDashboardLayouts()
	for breakpoint, items := range withoutChart {
		withoutChart[breakpoint] = items[:len(items)-1]
	}
	if err := ValidateDashboardWidgetConfigs(withoutChart, valid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("orphan config error = %v", err)
	}
}

func addTestChart(layouts DashboardResponsiveLayouts) {
	for breakpoint, item := range map[string]DashboardLayoutItem{
		"lg": {I: timeseriesWidgetID, X: 0, Y: 8, W: 12, H: 5},
		"md": {I: timeseriesWidgetID, X: 0, Y: 10, W: 8, H: 5},
		"sm": {I: timeseriesWidgetID, X: 0, Y: 16, W: 4, H: 6},
	} {
		layouts[breakpoint] = append(layouts[breakpoint], item)
	}
}
