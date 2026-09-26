package core

import (
	"context"
	"os"
	"testing"
	"time"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/gatewayhub"
	"ygate/platform-api/internal/testdb"
)

// A device with a logbook entry used to fail hard delete with a RESTRICT
// foreign-key error (surfacing as a 500), and so did its whole Plant.
func TestHardDeleteDeviceAndPlantWithLogbookAgainstPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := testdb.Disposable(t, ctx, databaseURL)

	org := mustUUID(t, "12000000-0000-4000-8000-000000000001")
	admin := mustUUID(t, "12000000-0000-4000-8000-000000000011")
	plant := mustUUID(t, "12000000-0000-4000-8000-000000000031")
	model := mustUUID(t, "12000000-0000-4000-8000-000000000051")
	device := mustUUID(t, "12000000-0000-4000-8000-000000000061")
	device2 := mustUUID(t, "12000000-0000-4000-8000-000000000062")
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`INSERT INTO organization(id,code,name) VALUES($1,'HD','HD')`, org)
	exec(`INSERT INTO auth.app_user(id,organization_id,email,display_name,password_hash) VALUES($1,$2,'hd@test.invalid','x','unused')`, admin, org)
	exec(`INSERT INTO auth.user_role(id,organization_id,user_id,role_id) VALUES(gen_random_uuid(),NULL,$1,'00000000-0000-4000-8000-000000000201')`, admin)
	exec(`INSERT INTO plant.plant(id,organization_id,code,name,timezone) VALUES($1,$2,'HD1','HD1','UTC')`, plant, org)
	exec(`INSERT INTO plant.register_profile(id,organization_id,name) VALUES($1,$2,'HD')`, model, org)
	exec(`INSERT INTO plant.device_model(id,organization_id,manufacturer,model,device_type,register_profile_id) VALUES($1,$2,'T','HD','INVERTER',$1)`, model, org)
	exec(`INSERT INTO plant.device(id,organization_id,plant_id,device_model_id,external_id,name) VALUES($1,$3,$4,$5,'D1','D1'),($2,$3,$4,$5,'D2','D2')`, device, device2, org, plant, model)
	exec(`INSERT INTO alarm.event_logbook(id,organization_id,plant_id,device_id,event_type,title,starts_at,created_by) VALUES
        (gen_random_uuid(),$1,$2,$3,'FAULT','trip',now(),$5),(gen_random_uuid(),$1,$2,$4,'NOTE','n',now(),$5)`, org, plant, device, device2, admin)

	service := New(pool, gatewayhub.New())
	principal := auth.Principal{UserID: admin, OrganizationID: org}
	if err := service.HardDeleteDevice(ctx, principal, uuidString(plant), uuidString(device), "DELETE", nil); err != nil {
		t.Fatalf("hard delete device with logbook entry: %v", err)
	}
	var kept int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM alarm.event_logbook WHERE plant_id=$1 AND device_id IS NULL AND title='trip'`, plant).Scan(&kept); err != nil || kept != 1 {
		t.Fatalf("logbook entry kept as plant-level = %d, err %v", kept, err)
	}
	if err := service.HardDeletePlant(ctx, principal, uuidString(plant), "DELETE", nil); err != nil {
		t.Fatalf("hard delete plant with device logbook entry: %v", err)
	}
}
