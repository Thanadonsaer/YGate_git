package core

import (
	"context"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/database"
	"ygate/platform-api/internal/gatewayhub"
)

func TestImportFromSnapshotAgainstPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	orgID := mustUUID(t, "20000000-0000-4000-8000-000000000001")
	adminID := mustUUID(t, "20000000-0000-4000-8000-000000000011")
	if _, err = pool.Exec(ctx, "INSERT INTO organization(id,code,name) VALUES($1,'TEST-IMPORT','Test Import Org')", orgID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO app_user(id,organization_id,email,display_name,password_hash) VALUES($1,$2,'import-admin@test.invalid','Import Admin','unused')`, adminID, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO user_role(id,organization_id,user_id,role_id) VALUES($1,$2,$3,'00000000-0000-4000-8000-000000000201')`,
		mustUUID(t, "20000000-0000-4000-8000-000000000021"), pgtype.UUID{}, adminID); err != nil {
		t.Fatal(err)
	}

	service := New(pool, gatewayhub.New())
	admin := auth.Principal{UserID: adminID, OrganizationID: orgID}
	sourceIP, _ := netip.ParseAddr("127.0.0.1")

	snapshot := MiddlewareConfigSnapshot{
		Brands: []MiddlewareBrand{{BrandID: 1, BrandName: "TestBrand"}},
		DeviceSets: []MiddlewareDeviceSet{{
			DeviceSetID: 100, BrandID: 1, DevType: "Inverter", DevModel: "TB-5000",
			Addresses: []MiddlewareAddress{
				{AddressID: 1, DeviceSetID: 100, FunctionCode: 3, Register: 40001, Description: "Active power", CanonicalKey: "P_AC", Factor: 1, DataType: "FLOAT"},
				{AddressID: 2, DeviceSetID: 100, FunctionCode: 3, Register: 40003, Description: "State of charge", Factor: 0.1, DataType: "SHORT"},
			},
		}},
		Connections: []MiddlewareConnection{{ConnectionID: 1, ConnectionName: "Inv1", Host: "192.168.1.50", Port: 502, UnitID: 1, DeviceSetID: 100, Enabled: true}},
	}

	result, err := service.importFromSnapshot(ctx, admin, orgID.String(), snapshot, &sourceIP)
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
	models, err := service.DeviceModels(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, m := range models {
		if m.Manufacturer == "TestBrand" && m.DeviceType == "Inverter" && m.Model == "TB-5000" {
			found = true
		}
	}
	if !found {
		t.Error("expected a TestBrand/Inverter/TB-5000 device model to exist after import")
	}
	metadata, err := service.DeviceModelRegisterMetadata(ctx, admin, models[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	var floatDataType string
	for _, m := range metadata {
		if m.AddressKey == "P_AC" && m.ModbusDataType != nil {
			floatDataType = *m.ModbusDataType
		}
	}
	if floatDataType != "FLOAT32" {
		t.Errorf("P_AC ModbusDataType = %q, want FLOAT32 (normalized from legacy FLOAT alias)", floatDataType)
	}

	// Re-running the same import must not create a second Device Model.
	result2, err := service.importFromSnapshot(ctx, admin, orgID.String(), snapshot, &sourceIP)
	if err != nil {
		t.Fatalf("second importFromSnapshot: %v", err)
	}
	if result2.DeviceModelsCreated != 0 || result2.DeviceModelsReused != 1 {
		t.Errorf("second import: DeviceModelsCreated=%d DeviceModelsReused=%d, want 0 and 1 (idempotent)", result2.DeviceModelsCreated, result2.DeviceModelsReused)
	}
	if result2.RegisterMetadataUpserted != 2 {
		t.Errorf("second import RegisterMetadataUpserted = %d, want 2 (upsert, not duplicate)", result2.RegisterMetadataUpserted)
	}
}
