package store

import (
	"strings"
	"testing"

	"chpp/modbus-api-middleware/internal/domain"
)

// No build tag: this runs on a plain `go test`, so the rules both store
// adapters share are covered even though the file store only builds on mips.
func TestNormalizeConfigSnapshot(t *testing.T) {
	valid := func() domain.ConfigSnapshot {
		return domain.ConfigSnapshot{
			Brands: []domain.Brand{{BrandID: 1, BrandName: " Huawei "}},
			DeviceSets: []domain.DeviceSet{{
				DeviceSetID: 10, BrandID: 1, DevType: "Inverter", DevModel: "SUN2000",
				ByteOrder: "BIG_ENDIAN", WordOrder: "HIGH_LOW", MaxBlockSize: 30,
				Addresses: []domain.Address{{AddressID: 5, FunctionCode: 3, Register: 32080, Description: "Active power", Factor: 1, DataType: "U32"}},
			}},
			Plants: []domain.Plant{{PlantCode: " vt1 ", PlantName: "VT1"}},
			Connections: []domain.ConnectionConfig{{
				ConnectionID: 100, ConnectionName: "VT1-INV-01", Host: "127.0.0.1", Port: 502,
				SlaveID: 45, DeviceSetID: 10, PlantCode: "VT1", Enabled: true,
			}},
		}
	}

	got, err := normalizeConfigSnapshot(valid())
	if err != nil {
		t.Fatal(err)
	}
	if c := got.Connections[0]; c.UnitID != 45 {
		t.Fatalf("UnitID=%d want 45 promoted from legacy SlaveID (unit 0 is Modbus broadcast)", c.UnitID)
	}
	if got.Brands[0].BrandName != "Huawei" || got.Plants[0].PlantCode != "VT1" {
		t.Fatalf("not trimmed/upper-cased: %+v %+v", got.Brands[0], got.Plants[0])
	}
	if a := got.DeviceSets[0].Addresses[0]; a.AddressID != 5 || a.DeviceSetID != 10 {
		t.Fatalf("address ids lost: %+v", a)
	}

	for name, tc := range map[string]struct {
		mutate func(*domain.ConfigSnapshot)
		want   string
	}{
		"dangling deviceSetId": {func(s *domain.ConfigSnapshot) { s.Connections[0].DeviceSetID = 99 }, "unknown deviceSetId"},
		"dangling brandId":     {func(s *domain.ConfigSnapshot) { s.DeviceSets[0].BrandID = 99 }, "unknown brandId"},
		"no addresses":         {func(s *domain.ConfigSnapshot) { s.DeviceSets[0].Addresses = nil }, "at least one address"},
		"bad word order":       {func(s *domain.ConfigSnapshot) { s.DeviceSets[0].WordOrder = "SIDEWAYS" }, "wordOrder"},
		"bad port":             {func(s *domain.ConfigSnapshot) { s.Connections[0].Port = 0 }, "valid port"},
		"blank plant":          {func(s *domain.ConfigSnapshot) { s.Plants[0].PlantName = " " }, "plantName"},
	} {
		snap := valid()
		tc.mutate(&snap)
		if _, err := normalizeConfigSnapshot(snap); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err=%v want containing %q", name, err, tc.want)
		}
	}
}
