package core

import "testing"

func TestDashboardCommunicationStatus(t *testing.T) {
	tests := []struct {
		plant DashboardPlantStatus
		want  string
	}{
		{plant: DashboardPlantStatus{IsActive: false}, want: "DISABLED"},
		{plant: DashboardPlantStatus{IsActive: true}, want: "NO_DEVICES"},
		{plant: DashboardPlantStatus{IsActive: true, ActiveDeviceCount: 2}, want: "OFFLINE"},
		// Every device stale: nothing is reporting, so this is OFFLINE, not DEGRADED.
		{plant: DashboardPlantStatus{IsActive: true, ActiveDeviceCount: 2, ReportingDeviceCount: 0, StaleDeviceCount: 2}, want: "OFFLINE"},
		{plant: DashboardPlantStatus{IsActive: true, ActiveDeviceCount: 2, ReportingDeviceCount: 1, StaleDeviceCount: 1}, want: "DEGRADED"},
		{plant: DashboardPlantStatus{IsActive: true, ActiveDeviceCount: 2, ReportingDeviceCount: 1, OfflineDeviceCount: 1}, want: "DEGRADED"},
		{plant: DashboardPlantStatus{IsActive: true, ActiveDeviceCount: 2, ReportingDeviceCount: 2}, want: "ONLINE"},
	}
	for _, test := range tests {
		if got := dashboardCommunicationStatus(test.plant); got != test.want {
			t.Fatalf("plant=%+v got=%s want=%s", test.plant, got, test.want)
		}
	}
}
