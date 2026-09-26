package store

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"chpp/modbus-api-middleware/internal/domain"
)

// The same fixture is pinned on the Platform side as exactly what
// platform-api pushes (services/platform-api core snapshot contract test).
// Decoding it strictly here catches a field renamed or added on one side only.
func TestPlatformConfigSnapshotFixtureDecodesAndApplies(t *testing.T) {
	raw, err := os.ReadFile("../../../packages/api-contracts/fixtures/middleware-config-snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var snap domain.ConfigSnapshot
	if err = decoder.Decode(&snap); err != nil {
		t.Fatalf("platform snapshot has a field the Middleware does not know: %v", err)
	}
	got, err := normalizeConfigSnapshot(snap)
	if err != nil {
		t.Fatalf("platform snapshot rejected: %v", err)
	}
	if c := got.Connections[0]; c.UnitID != 3 || c.Host != "192.168.1.10" || c.DeviceSetID != 10 {
		t.Fatalf("connection = %+v", c)
	}
	if a := got.DeviceSets[0].Addresses[0]; !a.Enabled || a.Factor != 0.001 || a.Register != 32080 {
		t.Fatalf("address = %+v", a)
	}
	if snap.IdleHeartbeatSeconds != 30 || snap.SendIntervalSeconds != 60 || !snap.APIPollingEnabled {
		t.Fatalf("gateway settings = %+v", snap)
	}
}
