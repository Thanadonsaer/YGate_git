package configcache

import (
	"path/filepath"
	"testing"

	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/store"
)

func TestRebuildFromStoreThenLoadRoundTrip(t *testing.T) {
	st, err := store.OpenNormalized(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	brand, err := st.SaveBrand(domain.Brand{BrandName: "ABB"})
	if err != nil {
		t.Fatal(err)
	}
	set, err := st.SaveDeviceSet(domain.DeviceSet{
		BrandID: brand.BrandID, DevType: "Inverter", DevModel: "PVS100",
		Addresses: []domain.Address{{FunctionCode: 3, Register: 0, Description: "Active power", Factor: 1, DataType: "U16"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.SavePlant(domain.Plant{PlantCode: "VT1", PlantName: "VT1"}); err != nil {
		t.Fatal(err)
	}
	conn, err := st.SaveConnection(domain.ConnectionConfig{
		ConnectionName: "VT1-INV-01", Host: "127.0.0.1", Port: 502, DeviceSetID: set.DeviceSetID, PlantCode: "VT1", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	cache := New()
	if cache.Load() == nil {
		t.Fatal("New() cache Load() returned nil before any Swap")
	}

	snapshot, err := RebuildFromStore(st)
	if err != nil {
		t.Fatal(err)
	}
	cache.Swap(snapshot)

	loaded := cache.Load()
	if len(loaded.Brands) != 1 || loaded.Brands[0].BrandName != "ABB" {
		t.Fatalf("Brands=%+v", loaded.Brands)
	}
	ds, ok := loaded.DeviceSets[set.DeviceSetID]
	if !ok || ds.DevModel != "PVS100" || len(ds.Addresses) != 1 {
		t.Fatalf("DeviceSets[%d]=%+v ok=%v", set.DeviceSetID, ds, ok)
	}
	c, ok := loaded.Connections[conn.ConnectionID]
	if !ok || c.Host != "127.0.0.1" || !c.Enabled {
		t.Fatalf("Connections[%d]=%+v ok=%v", conn.ConnectionID, c, ok)
	}
	if len(loaded.Plants) != 1 || loaded.Plants[0].PlantCode != "VT1" {
		t.Fatalf("Plants=%+v", loaded.Plants)
	}
}

func TestSwapReplacesSnapshotAtomically(t *testing.T) {
	cache := New()
	first := &Config{Version: 1, DeviceSets: map[int64]domain.DeviceSet{}, Connections: map[int64]domain.ConnectionConfig{}}
	second := &Config{Version: 2, DeviceSets: map[int64]domain.DeviceSet{}, Connections: map[int64]domain.ConnectionConfig{}}
	cache.Swap(first)
	if cache.Load().Version != 1 {
		t.Fatalf("Version=%d want 1", cache.Load().Version)
	}
	cache.Swap(second)
	if cache.Load().Version != 2 {
		t.Fatalf("Version=%d want 2", cache.Load().Version)
	}
}
