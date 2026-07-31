package core

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/database"
	"ygate/platform-api/internal/gatewayhub"
)

func TestPlantAuthorizationLifecycleAgainstPostgreSQL(t *testing.T) {
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

	orgA := mustUUID(t, "10000000-0000-4000-8000-000000000001")
	orgB := mustUUID(t, "10000000-0000-4000-8000-000000000002")
	adminID := mustUUID(t, "10000000-0000-4000-8000-000000000011")
	viewerAID := mustUUID(t, "10000000-0000-4000-8000-000000000012")
	viewerBID := mustUUID(t, "10000000-0000-4000-8000-000000000013")
	noGrantID := mustUUID(t, "10000000-0000-4000-8000-000000000014")

	for _, organization := range []struct {
		id   pgtype.UUID
		code string
		name string
	}{{orgA, "TEST-A", "Test Organization A"}, {orgB, "TEST-B", "Test Organization B"}} {
		if _, err = pool.Exec(ctx, "INSERT INTO organization(id,code,name) VALUES($1,$2,$3)", organization.id, organization.code, organization.name); err != nil {
			t.Fatal(err)
		}
	}
	for _, user := range []struct {
		id    pgtype.UUID
		org   pgtype.UUID
		email string
	}{{adminID, orgA, "admin@test.invalid"}, {viewerAID, orgA, "viewer-a@test.invalid"}, {viewerBID, orgB, "viewer-b@test.invalid"}, {noGrantID, orgA, "none@test.invalid"}} {
		if _, err = pool.Exec(ctx, `INSERT INTO app_user(id,organization_id,email,display_name,password_hash) VALUES($1,$2,$3,$4,'unused')`, user.id, user.org, user.email, user.email); err != nil {
			t.Fatal(err)
		}
	}
	assignments := []struct {
		id, user, role, organization pgtype.UUID
	}{{mustUUID(t, "10000000-0000-4000-8000-000000000021"), adminID, mustUUID(t, "00000000-0000-4000-8000-000000000201"), pgtype.UUID{}},
		{mustUUID(t, "10000000-0000-4000-8000-000000000022"), viewerAID, mustUUID(t, "00000000-0000-4000-8000-000000000206"), orgA},
		{mustUUID(t, "10000000-0000-4000-8000-000000000023"), viewerBID, mustUUID(t, "00000000-0000-4000-8000-000000000206"), orgB}}
	for _, assignment := range assignments {
		if _, err = pool.Exec(ctx, `INSERT INTO user_role(id,organization_id,user_id,role_id) VALUES($1,$2,$3,$4)`, assignment.id, assignment.organization, assignment.user, assignment.role); err != nil {
			t.Fatal(err)
		}
	}

	service := New(pool, gatewayhub.New())
	admin := auth.Principal{UserID: adminID, OrganizationID: orgA}
	viewerA := auth.Principal{UserID: viewerAID, OrganizationID: orgA}
	viewerB := auth.Principal{UserID: viewerBID, OrganizationID: orgB}
	noGrant := auth.Principal{UserID: noGrantID, OrganizationID: orgA}

	dc, ac := 1200.5, 1000.0
	plantA, err := service.CreatePlant(ctx, admin, CreatePlantInput{OrganizationID: uuidString(orgA), Code: " plant-a ", Name: "Plant A", Timezone: "Asia/Bangkok", InstalledDcKW: &dc, InstalledAcKW: &ac}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plantA.Code != "PLANT-A" || plantA.OrganizationID != uuidString(orgA) {
		t.Fatalf("plant A = %+v", plantA)
	}
	plantB, err := service.CreatePlant(ctx, admin, CreatePlantInput{OrganizationID: uuidString(orgB), Code: "PLANT-B", Name: "Plant B", Timezone: "UTC"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	plants, err := service.Plants(ctx, viewerA)
	if err != nil || len(plants) != 1 || plants[0].ID != plantA.ID {
		t.Fatalf("viewer A plants = %+v, err = %v", plants, err)
	}
	if _, err = service.CreatePlant(ctx, viewerA, CreatePlantInput{Code: "DENIED", Name: "Denied", Timezone: "UTC"}, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer create error = %v", err)
	}
	if _, err = service.Plant(ctx, viewerB, plantA.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-organization read error = %v", err)
	}
	if _, err = service.Plants(ctx, noGrant); !errors.Is(err, ErrForbidden) {
		t.Fatalf("no-grant list error = %v", err)
	}
	if _, err = service.CreatePlant(ctx, admin, CreatePlantInput{OrganizationID: uuidString(orgA), Code: "PLANT-A", Name: "Duplicate", Timezone: "UTC"}, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate create error = %v", err)
	}

	updated, err := service.UpdatePlant(ctx, admin, plantB.ID, UpdatePlantInput{Code: plantB.Code, Name: "Plant B Disabled", Timezone: "UTC", IsActive: false}, nil)
	if err != nil || updated.IsActive || updated.Name != "Plant B Disabled" {
		t.Fatalf("updated plant = %+v, err = %v", updated, err)
	}
	var createdAudits, updatedAudits int
	if err = pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE action='plant.created'), count(*) FILTER (WHERE action='plant.updated' AND before_data IS NOT NULL AND after_data IS NOT NULL) FROM audit_log`).Scan(&createdAudits, &updatedAudits); err != nil {
		t.Fatal(err)
	}
	if createdAudits != 2 || updatedAudits != 1 {
		t.Fatalf("audit counts created=%d updated=%d", createdAudits, updatedAudits)
	}
}

func mustUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	id, err := parseUUID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
