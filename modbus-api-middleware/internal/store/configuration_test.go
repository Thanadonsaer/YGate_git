//go:build !mips && !mipsle

package store

import (
	"path/filepath"
	"testing"

	"chpp/modbus-api-middleware/internal/domain"
)

func TestTableFieldsDriveRelationsAndDefaults(t *testing.T) {
	s, err := OpenNormalized(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	brand, err := s.SaveBrand(domain.Brand{BrandName: "Huawei"})
	if err != nil {
		t.Fatal(err)
	}
	set, err := s.SaveDeviceSet(domain.DeviceSet{
		BrandID:  brand.BrandID,
		DevType:  "Inverter",
		DevModel: "SUN2000-100KTL-M1",
		Addresses: []domain.Address{
			{FunctionCode: 4, Register: 32080, Description: "Active power", Factor: .001, DataType: "SW_INT"},
			{FunctionCode: 3, Register: 40196, Description: "Active power adjustment", Factor: 1, DataType: "SHORT"},
			{FunctionCode: 4, Register: 32087, Description: "Cabinet temperature", Factor: .1, DataType: "SHORT"},
			{FunctionCode: 3, Register: 32002, Description: "Collect DSP data", Factor: 1, DataType: "USHORT"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := s.SaveConnection(domain.ConnectionConfig{ConnectionName: "AICA-INV-01", Host: "192.168.1.200", Port: 15034, UnitID: 5, DeviceSetID: set.DeviceSetID})
	if err != nil {
		t.Fatal(err)
	}

	brands, err := s.Brands()
	if err != nil {
		t.Fatal(err)
	}
	if len(brands) != 1 || len(brands[0].BrandDevSetIDList) != 1 || brands[0].BrandDevSetIDList[0] != set.DeviceSetID {
		t.Fatalf("brands=%+v", brands)
	}
	resolved, err := s.Connection(connection.ConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.DeviceSetID != set.DeviceSetID || resolved.DeviceSetName != "Huawei SUN2000-100KTL-M1" || resolved.UnitID != 5 || resolved.SlaveID != 5 || resolved.DevDn != "AICA-INV-01" || resolved.PlantCode != "AICA" {
		t.Fatalf("resolved=%+v", resolved)
	}
	if len(set.AddressIDList) != 4 || set.DevTypeID != 1 || set.AddressMode != "ZERO_BASED" {
		t.Fatalf("set=%+v", set)
	}
	if set.Addresses[0].FunctionCode != 3 || set.Addresses[0].Register != 2080 || set.Addresses[0].CanonicalKey != "3:2080" || set.Addresses[0].Length != 2 {
		t.Fatalf("address defaults=%+v", set.Addresses[0])
	}
	if set.Addresses[1].FunctionCode != 4 || set.Addresses[1].Register != 196 {
		t.Fatalf("full register normalization failed=%+v", set.Addresses[1])
	}
	if set.Addresses[3].FunctionCode != 3 || set.Addresses[3].Register != 2002 || set.Addresses[3].CanonicalKey != "3:2002" {
		t.Fatalf("unknown address key=%+v", set.Addresses[3])
	}
}

func TestEmptyListsEncodeAsArraysAndBrandUpserts(t *testing.T) {
	s, err := OpenNormalized(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	brands, err := s.Brands()
	if err != nil {
		t.Fatal(err)
	}
	sets, err := s.DeviceSets()
	if err != nil {
		t.Fatal(err)
	}
	connections, err := s.Connections()
	if err != nil {
		t.Fatal(err)
	}
	if brands == nil || sets == nil || connections == nil {
		t.Fatalf("empty lists must be non-nil: brands=%#v sets=%#v connections=%#v", brands, sets, connections)
	}

	first, err := s.SaveBrand(domain.Brand{BrandName: "Huawei"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.SaveBrand(domain.Brand{BrandName: "Huawei", BrandDescription: "updated"})
	if err != nil {
		t.Fatal(err)
	}
	if first.BrandID != second.BrandID || second.BrandDescription != "updated" {
		t.Fatalf("upsert failed: first=%+v second=%+v", first, second)
	}
}

func TestDeviceSetMaxBlockSizeMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.DB.Exec(`CREATE TABLE device_sets (
 dev_set_id INTEGER PRIMARY KEY AUTOINCREMENT,
 brand_id INTEGER NOT NULL,
 dev_type_id INTEGER NOT NULL,
 dev_type TEXT NOT NULL,
 dev_model TEXT NOT NULL,
 address_mode TEXT NOT NULL DEFAULT 'ZERO_BASED',
 byte_order TEXT NOT NULL DEFAULT 'BIG_ENDIAN',
 word_order TEXT NOT NULL DEFAULT 'HIGH_LOW',
 UNIQUE(brand_id, dev_type_id, dev_model));`)
	legacy.Close()
	if err != nil {
		t.Fatal(err)
	}
	s, err := OpenNormalized(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ok, err := s.hasColumn("device_sets", "max_block_size")
	if err != nil || !ok {
		t.Fatalf("migration ok=%v err=%v", ok, err)
	}
}

func TestAddressUnitColumnsMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.DB.Exec(`CREATE TABLE addresses (
 address_id INTEGER PRIMARY KEY AUTOINCREMENT,
 dev_set_id INTEGER NOT NULL,
 address_fc INTEGER NOT NULL,
 address_register INTEGER NOT NULL,
 address_description TEXT NOT NULL,
 canonical_key TEXT NOT NULL,
 source_tag TEXT NOT NULL DEFAULT '',
 address_factor REAL NOT NULL DEFAULT 1,
 address_offset REAL NOT NULL DEFAULT 0,
 address_data_type TEXT NOT NULL,
 address_length INTEGER NOT NULL DEFAULT 1,
 word_order TEXT NOT NULL DEFAULT '',
 address_remark TEXT NOT NULL DEFAULT '',
 enabled INTEGER NOT NULL DEFAULT 1);`)
	legacy.Close()
	if err != nil {
		t.Fatal(err)
	}
	s, err := OpenNormalized(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, column := range []string{"source_unit", "canonical_unit"} {
		ok, err := s.hasColumn("addresses", column)
		if err != nil || !ok {
			t.Fatalf("%s migration ok=%v err=%v", column, ok, err)
		}
	}
}
func TestConnectionEnabledPersistsWhenDisabled(t *testing.T) {
	s, err := OpenNormalized(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	brand, err := s.SaveBrand(domain.Brand{BrandName: "ABB"})
	if err != nil {
		t.Fatal(err)
	}
	set, err := s.SaveDeviceSet(domain.DeviceSet{BrandID: brand.BrandID, DevType: "Inverter", DevModel: "PVS100", AddressMode: "RAW", Addresses: []domain.Address{{FunctionCode: 3, Register: 40084, Description: "Watts", Factor: .01, DataType: "SHORT"}}})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := s.SaveConnection(domain.ConnectionConfig{ConnectionName: "AICA-INV-01", Host: "192.168.1.200", Port: 502, UnitID: 1, DeviceSetID: set.DeviceSetID})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetConnectionEnabled(connection.ConnectionID, false); err != nil {
		t.Fatal(err)
	}
	resolved, err := s.Connection(connection.ConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Enabled {
		t.Fatalf("disabled connection read back as enabled: %+v", resolved)
	}
	sets, err := s.DeviceSets()
	if err != nil || sets[0].AddressMode != "VENDOR_RAW" || sets[0].Addresses[0].CanonicalKey != "3:40084" {
		t.Fatalf("set=%+v err=%v", sets, err)
	}
}

func TestConnectionAcceptsSlaveIDAlias(t *testing.T) {
	s, err := OpenNormalized(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	brand, err := s.SaveBrand(domain.Brand{BrandName: "ABB"})
	if err != nil {
		t.Fatal(err)
	}
	set, err := s.SaveDeviceSet(domain.DeviceSet{
		BrandID:   brand.BrandID,
		DevType:   "Grid-Meter",
		DevModel:  "-",
		Addresses: []domain.Address{{FunctionCode: 3, Register: 7000, Description: "Inst_kW", Factor: .001, DataType: "FLOAT"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := s.SaveConnection(domain.ConnectionConfig{ConnectionName: "AICA-GMT-01", Host: "192.168.1.200", Port: 15150, SlaveID: 9, DeviceSetID: set.DeviceSetID})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := s.Connection(connection.ConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.UnitID != 9 || resolved.SlaveID != 9 {
		t.Fatalf("resolved=%+v", resolved)
	}
}

func TestSMAAddressesStayDirectAndAllowFourWords(t *testing.T) {
	s, err := OpenNormalized(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	brand, err := s.SaveBrand(domain.Brand{BrandName: "SMA"})
	if err != nil {
		t.Fatal(err)
	}
	set, err := s.SaveDeviceSet(domain.DeviceSet{
		BrandID: brand.BrandID, DevType: "Inverter", DevModel: "Sunny Central", AddressMode: "SMA",
		Addresses: []domain.Address{{FunctionCode: 3, Register: 30057, Description: "Serial number", Factor: 1, DataType: "U32"}, {FunctionCode: 3, Register: 30513, Description: "Total yield", Factor: 1, DataType: "U64", Length: 4}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if set.AddressMode != "SMA" || set.MaxBlockSize != 30 || set.Addresses[0].Register != 30057 || set.Addresses[1].Length != 4 {
		t.Fatalf("set=%+v", set)
	}
}

func TestDeletePlantDeletesItsConnections(t *testing.T) {
	s, err := OpenNormalized(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	plant, err := s.SavePlant(domain.Plant{PlantCode: "AICA", PlantName: "AICA Plant"})
	if err != nil {
		t.Fatal(err)
	}
	brand, err := s.SaveBrand(domain.Brand{BrandName: "Huawei"})
	if err != nil {
		t.Fatal(err)
	}
	set, err := s.SaveDeviceSet(domain.DeviceSet{
		BrandID:  brand.BrandID,
		DevType:  "Inverter",
		DevModel: "SUN2000",
		Addresses: []domain.Address{
			{FunctionCode: 4, Register: 32080, Description: "Active power", Factor: .001, DataType: "SW_INT"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveConnection(domain.ConnectionConfig{ConnectionName: "AICA-INV-01", Host: "192.168.1.200", Port: 15034, UnitID: 1, DeviceSetID: set.DeviceSetID, PlantCode: "AICA", PlantName: "AICA Plant"}); err != nil {
		t.Fatal(err)
	}

	if err = s.DeletePlant(plant.PlantID); err != nil {
		t.Fatal(err)
	}
	connections, err := s.Connections()
	if err != nil {
		t.Fatal(err)
	}
	if len(connections) != 0 {
		t.Fatalf("connections=%+v", connections)
	}
	sets, err := s.DeviceSets()
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 1 {
		t.Fatalf("device sets should remain, got %+v", sets)
	}
}
