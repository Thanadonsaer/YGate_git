package core

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/gatewayhub"
	"ygate/platform-api/internal/testdb"
)

// TestAuthorizationScopesAgainstPostgreSQL pins auth.has_permission's scope
// rules and the middleware reads that used to check "has this permission in
// any organization" and then load another organization's rows by id.
func TestAuthorizationScopesAgainstPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := testdb.Disposable(t, ctx, databaseURL)

	orgA := mustUUID(t, "11000000-0000-4000-8000-000000000001")
	orgB := mustUUID(t, "11000000-0000-4000-8000-000000000002")
	orgAdminA := mustUUID(t, "11000000-0000-4000-8000-000000000011")
	sysAdmin := mustUUID(t, "11000000-0000-4000-8000-000000000012")
	plantUser := mustUUID(t, "11000000-0000-4000-8000-000000000013")
	plantA := mustUUID(t, "11000000-0000-4000-8000-000000000031")
	plantA2 := mustUUID(t, "11000000-0000-4000-8000-000000000032")
	plantB := mustUUID(t, "11000000-0000-4000-8000-000000000033")
	gatewayA := mustUUID(t, "11000000-0000-4000-8000-000000000041")
	gatewayB := mustUUID(t, "11000000-0000-4000-8000-000000000042")
	modelB := mustUUID(t, "11000000-0000-4000-8000-000000000051")
	deviceB := mustUUID(t, "11000000-0000-4000-8000-000000000061")
	systemAdminRole := mustUUID(t, "00000000-0000-4000-8000-000000000201")
	orgAdminRole := mustUUID(t, "00000000-0000-4000-8000-000000000202")

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`INSERT INTO organization(id,code,name) VALUES($1,'AUTHZ-A','A'),($2,'AUTHZ-B','B')`, orgA, orgB)
	exec(`INSERT INTO auth.app_user(id,organization_id,email,display_name,password_hash) VALUES
        ($1,$4,'org-admin@test.invalid','x','unused'),($2,$4,'sys@test.invalid','x','unused'),($3,$4,'plant@test.invalid','x','unused')`, orgAdminA, sysAdmin, plantUser, orgA)
	exec(`INSERT INTO plant.plant(id,organization_id,code,name,timezone) VALUES($1,$4,'PA','A','UTC'),($2,$4,'PA2','A2','UTC'),($3,$5,'PB','B','UTC')`, plantA, plantA2, plantB, orgA, orgB)
	exec(`INSERT INTO auth.user_role(id,organization_id,user_id,role_id) VALUES(gen_random_uuid(),$1,$2,$3)`, orgA, orgAdminA, orgAdminRole)
	exec(`INSERT INTO auth.user_role(id,organization_id,user_id,role_id) VALUES(gen_random_uuid(),NULL,$1,$2)`, sysAdmin, systemAdminRole)
	exec(`INSERT INTO auth.user_role(id,organization_id,plant_id,user_id,role_id) VALUES(gen_random_uuid(),$1,$2,$3,$4)`, orgA, plantA, plantUser, orgAdminRole)
	exec(`INSERT INTO auth.middleware_client(id,organization_id,name,key_prefix,key_hash,auto_onboard) VALUES($1,$3,'GA','ygm_authza1','\x01'::bytea,false),($2,$4,'GB','ygm_authzb1','\x02'::bytea,false)`, gatewayA, gatewayB, orgA, orgB)
	exec(`INSERT INTO plant.register_profile(id,organization_id,name) VALUES($1,$2,'B')`, modelB, orgB)
	exec(`INSERT INTO plant.device_model(id,organization_id,manufacturer,model,device_type,register_profile_id) VALUES($1,$2,'T','B','INVERTER',$1)`, modelB, orgB)
	exec(`INSERT INTO plant.device(id,organization_id,plant_id,device_model_id,external_id,name) VALUES($1,$2,$3,$4,'B-1','B-1')`, deviceB, orgB, plantB, modelB)

	for _, tc := range []struct {
		name       string
		user       pgtype.UUID
		org, plant pgtype.UUID
		want       bool
	}{
		{"org admin in own org", orgAdminA, orgA, pgtype.UUID{}, true},
		{"org admin in other org", orgAdminA, orgB, pgtype.UUID{}, false},
		{"org admin is not global", orgAdminA, pgtype.UUID{}, pgtype.UUID{}, false},
		{"system admin anywhere", sysAdmin, orgB, pgtype.UUID{}, true},
		{"system admin global", sysAdmin, pgtype.UUID{}, pgtype.UUID{}, true},
		{"plant role on its plant", plantUser, orgA, plantA, true},
		{"plant role on sibling plant", plantUser, orgA, plantA2, false},
		{"plant role never counts org-wide", plantUser, orgA, pgtype.UUID{}, false},
	} {
		got, err := hasPermission(ctx, pool, auth.Principal{UserID: tc.user}, "read", "middleware_config", tc.org, tc.plant)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %v err %v, want %v", tc.name, got, err, tc.want)
		}
	}

	service := New(pool, gatewayhub.New())
	orgAdmin := auth.Principal{UserID: orgAdminA, OrganizationID: orgA}
	if _, err := service.MiddlewareConfig(ctx, orgAdmin, uuidString(gatewayA)); err != nil {
		t.Fatalf("own middleware config: %v", err)
	}
	if _, err := service.MiddlewareConfig(ctx, orgAdmin, uuidString(gatewayB)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-org middleware config err = %v, want ErrForbidden", err)
	}
	if _, err := service.MiddlewarePlants(ctx, orgAdmin, uuidString(gatewayB)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-org middleware plants err = %v, want ErrForbidden", err)
	}
	if _, err := service.RunMiddlewareCommand(ctx, orgAdmin, uuidString(plantB), uuidString(deviceB), "connectTest"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-org device command err = %v, want ErrForbidden", err)
	}
	if _, err := service.RunMiddlewareCommand(ctx, orgAdmin, uuidString(plantA), uuidString(deviceB), "connectTest"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("device under wrong plant err = %v, want ErrNotFound", err)
	}
}
