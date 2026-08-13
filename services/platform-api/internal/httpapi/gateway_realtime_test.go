package httpapi

import "testing"

func TestGatewayCapabilitiesDisableTelemetryForUpdateBridge(t *testing.T) {
	if shouldStartTelemetryPull([]string{"update-bridge"}) {
		t.Fatal("update bridge must not start telemetry pull")
	}
	if !shouldStartTelemetryPull([]string{"full-middleware"}) {
		t.Fatal("full middleware must start telemetry pull")
	}
	if !shouldStartTelemetryPull(nil) {
		t.Fatal("legacy middleware without capabilities must keep telemetry pull")
	}
}
