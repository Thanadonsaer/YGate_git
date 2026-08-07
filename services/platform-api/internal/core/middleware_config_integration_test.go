package core

import (
	"context"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/gatewayhub"
	"ygate/platform-api/internal/testdb"
)

func TestImportFromSnapshotAgainstPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := testdb.Disposable(t, ctx, databaseURL)
	var err error

	orgID := mustUUID(t, "20000000-0000-4000-8000-000000000001")
	adminID := mustUUID(t, "20000000-0000-4000-8000-000000000011")
	if _, err = pool.Exec(ctx, "INSERT INTO organization(id,code,name) VALUES($1,'TEST-IMPORT','Test Import Org')", orgID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO auth.app_user(id,organization_id,email,display_name,password_hash) VALUES($1,$2,'import-admin@test.invalid','Import Admin','unused')`, adminID, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO auth.user_role(id,organization_id,user_id,role_id) VALUES($1,$2,$3,'00000000-0000-4000-8000-000000000201')`,
		mustUUID(t, "20000000-0000-4000-8000-000000000021"), pgtype.UUID{}, adminID); err != nil {
		t.Fatal(err)
	}

	service := New(pool, gatewayhub.New())
	admin := auth.Principal{UserID: adminID, OrganizationID: orgID}
	sourceIP, _ := netip.ParseAddr("127.0.0.1")

	plant, err := service.CreatePlant(ctx, admin, CreatePlantInput{
		OrganizationID: orgID.String(), Code: "TESTPLANT", Name: "Test Plant", Timezone: "Asia/Bangkok",
	}, &sourceIP)
	if err != nil {
		t.Fatal(err)
	}

	snapshot := MiddlewareConfigSnapshot{
		Brands: []MiddlewareBrand{{BrandID: 1, BrandName: "TestBrand"}},
		DeviceSets: []MiddlewareDeviceSet{{
			DeviceSetID: 100, BrandID: 1, DevType: "Inverter", DevModel: "TB-5000",
			Addresses: []MiddlewareAddress{
				{AddressID: 1, DeviceSetID: 100, FunctionCode: 3, Register: 40001, Description: "Active power", CanonicalKey: "P_AC", Factor: 1, DataType: "FLOAT"},
				{AddressID: 2, DeviceSetID: 100, FunctionCode: 3, Register: 40003, Description: "State of charge", Factor: 0.1, DataType: "SHORT"},
			},
		}},
		Connections: []MiddlewareConnection{{
			ConnectionID: 1, ConnectionName: "Inv1", Host: "192.168.1.50", Port: 502, UnitID: 1, DeviceSetID: 100,
			DevDn: "INV1", PlantCode: "TESTPLANT", Enabled: true,
		}},
	}

	result, err := service.importFromSnapshot(ctx, admin, orgID.String(), "", snapshot, &sourceIP)
	if err != nil {
		t.Fatalf("importFromSnapshot: %v", err)
	}
	if result.DeviceModelsCreated != 1 {
		t.Errorf("DeviceModelsCreated = %d, want 1", result.DeviceModelsCreated)
	}
	if result.RegisterMetadataUpserted != 2 {
		t.Errorf("RegisterMetadataUpserted = %d, want 2", result.RegisterMetadataUpserted)
	}
	if len(result.ConnectionsFound) != 1 || result.ConnectionsFound[0].Host != "192.168.1.50" {
		t.Errorf("ConnectionsFound = %+v, want one entry for 192.168.1.50", result.ConnectionsFound)
	}
	if result.DevicesCreated != 1 || result.DevicesUpdated != 0 || result.DevicesSkipped != 0 {
		t.Errorf("DevicesCreated=%d DevicesUpdated=%d DevicesSkipped=%d, want 1/0/0", result.DevicesCreated, result.DevicesUpdated, result.DevicesSkipped)
	}
	devices, err := service.Devices(ctx, admin, plant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].ExternalID != "INV1" || devices[0].ModbusHost == nil || *devices[0].ModbusHost != "192.168.1.50" || devices[0].ModbusPort == nil || *devices[0].ModbusPort != 502 || devices[0].ModbusUnitID != 1 {
		t.Errorf("devices = %+v, want one INV1 device with host=192.168.1.50 port=502 unit=1", devices)
	}
	models, err := service.DeviceModels(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	var modelID string
	for _, m := range models {
		if m.Manufacturer == "TestBrand" && m.DeviceType == "Inverter" && m.Model == "TB-5000" {
			modelID = m.ID
		}
	}
	if modelID == "" {
		t.Fatal("expected a TestBrand/Inverter/TB-5000 device model to exist after import")
	}
	metadata, err := service.DeviceModelRegisterMetadata(ctx, admin, modelID)
	if err != nil {
		t.Fatal(err)
	}
	// importFromSnapshot keys register metadata by the Modbus register number
	// ("reg40001"), deliberately not by the middleware's own CanonicalKey --
	// the register number is the identifier once imported. Looking up "P_AC"
	// here matched nothing, so the FLOAT -> FLOAT32 normalization this asserts
	// was never actually being checked.
	const powerAddressKey = "reg40001"
	var floatDataType string
	found := false
	for _, m := range metadata {
		if m.AddressKey == powerAddressKey {
			found = true
			if m.ModbusDataType != nil {
				floatDataType = *m.ModbusDataType
			}
		}
	}
	if !found {
		t.Fatalf("no register metadata for %s; got %+v", powerAddressKey, metadata)
	}
	if floatDataType != "FLOAT32" {
		t.Errorf("%s ModbusDataType = %q, want FLOAT32 (normalized from legacy FLOAT alias)", powerAddressKey, floatDataType)
	}

	// Re-running the same import must not create a second Device Model.
	result2, err := service.importFromSnapshot(ctx, admin, orgID.String(), "", snapshot, &sourceIP)
	if err != nil {
		t.Fatalf("second importFromSnapshot: %v", err)
	}
	if result2.DeviceModelsCreated != 0 || result2.DeviceModelsReused != 1 {
		t.Errorf("second import: DeviceModelsCreated=%d DeviceModelsReused=%d, want 0 and 1 (idempotent)", result2.DeviceModelsCreated, result2.DeviceModelsReused)
	}
	if result2.RegisterMetadataUpserted != 2 {
		t.Errorf("second import RegisterMetadataUpserted = %d, want 2 (upsert, not duplicate)", result2.RegisterMetadataUpserted)
	}
	if result2.DevicesCreated != 0 || result2.DevicesUpdated != 1 {
		t.Errorf("second import: DevicesCreated=%d DevicesUpdated=%d, want 0 and 1 (update existing, not duplicate)", result2.DevicesCreated, result2.DevicesUpdated)
	}
	devicesAfterSecondImport, err := service.Devices(ctx, admin, plant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(devicesAfterSecondImport) != 1 {
		t.Errorf("devices after second import = %d, want 1 (updated in place, not duplicated)", len(devicesAfterSecondImport))
	}

	// A Middleware's local device_sets row commonly has stray whitespace on
	// brand/type/model text (real-world free-text fields) -- matching must
	// still find the existing Device Model, not create a duplicate one just
	// because the lookup key wasn't trimmed the same way CreateDeviceModel
	// trims on write.
	whitespaceSnapshot := snapshot
	whitespaceSnapshot.Brands = []MiddlewareBrand{{BrandID: 1, BrandName: "  TestBrand  "}}
	whitespaceSnapshot.DeviceSets = []MiddlewareDeviceSet{{
		DeviceSetID: 100, BrandID: 1, DevType: " Inverter ", DevModel: " TB-5000 ",
		Addresses: snapshot.DeviceSets[0].Addresses,
	}}
	result3, err := service.importFromSnapshot(ctx, admin, orgID.String(), "", whitespaceSnapshot, &sourceIP)
	if err != nil {
		t.Fatalf("third importFromSnapshot (whitespace variant): %v", err)
	}
	if result3.DeviceModelsCreated != 0 || result3.DeviceModelsReused != 1 {
		t.Errorf("third import (whitespace variant): DeviceModelsCreated=%d DeviceModelsReused=%d, want 0 and 1 (must match the existing trimmed model, not create a duplicate)", result3.DeviceModelsCreated, result3.DeviceModelsReused)
	}
	modelsAfterThirdImport, err := service.DeviceModels(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	matches := 0
	for _, m := range modelsAfterThirdImport {
		if m.Manufacturer == "TestBrand" && m.DeviceType == "Inverter" && m.Model == "TB-5000" {
			matches++
		}
	}
	if matches != 1 {
		t.Errorf("TestBrand/Inverter/TB-5000 device models = %d, want 1 (whitespace variant must not have created a duplicate)", matches)
	}
}
