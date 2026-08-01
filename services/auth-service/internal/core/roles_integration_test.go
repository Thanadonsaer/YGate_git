package core

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ygate/auth-service/internal/auth"
	"ygate/auth-service/internal/database"
)

func TestRoleAdministrationLifecycleAgainstPostgreSQL(t *testing.T) {
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

	orgA := mustUUID(t, "20000000-0000-4000-8000-000000000001")
	platformAdminID := mustUUID(t, "20000000-0000-4000-8000-000000000011")
	orgAdminID := mustUUID(t, "20000000-0000-4000-8000-000000000012")
	viewerID := mustUUID(t, "20000000-0000-4000-8000-000000000013")
	assignedUserID := mustUUID(t, "20000000-0000-4000-8000-000000000014")

	if _, err = pool.Exec(ctx, "INSERT INTO organization(id,code,name) VALUES($1,$2,$3)", orgA, "TEST-ROLES-A", "Test Roles Organization A"); err != nil {
		t.Fatal(err)
	}
	for _, user := range []struct {
		id    pgtype.UUID
		email string
	}{{platformAdminID, "roles-admin@test.invalid"}, {orgAdminID, "roles-orgadmin@test.invalid"}, {viewerID, "roles-viewer@test.invalid"}, {assignedUserID, "roles-assigned@test.invalid"}} {
		if _, err = pool.Exec(ctx, `INSERT INTO app_user(id,organization_id,email,display_name,password_hash) VALUES($1,$2,$3,$4,'unused')`, user.id, orgA, user.email, user.email); err != nil {
			t.Fatal(err)
		}
	}
	assignments := []struct {
		id, user, role, organization pgtype.UUID
	}{
		{mustUUID(t, "20000000-0000-4000-8000-000000000021"), platformAdminID, mustUUID(t, "00000000-0000-4000-8000-000000000201"), pgtype.UUID{}},
		{mustUUID(t, "20000000-0000-4000-8000-000000000022"), orgAdminID, mustUUID(t, "00000000-0000-4000-8000-000000000202"), orgA},
		{mustUUID(t, "20000000-0000-4000-8000-000000000023"), viewerID, mustUUID(t, "00000000-0000-4000-8000-000000000206"), orgA},
	}
	for _, assignment := range assignments {
		if _, err = pool.Exec(ctx, `INSERT INTO user_role(id,organization_id,user_id,role_id) VALUES($1,$2,$3,$4)`, assignment.id, assignment.organization, assignment.user, assignment.role); err != nil {
			t.Fatal(err)
		}
	}

	service := New(pool)
	platformAdmin := auth.Principal{UserID: platformAdminID, OrganizationID: orgA}
	orgAdmin := auth.Principal{UserID: orgAdminID, OrganizationID: orgA}
	viewer := auth.Principal{UserID: viewerID, OrganizationID: orgA}

	permissions, err := service.Permissions(ctx, orgAdmin)
	if err != nil || len(permissions) == 0 {
		t.Fatalf("permissions = %v, err = %v", permissions, err)
	}
	if _, err = service.Permissions(ctx, viewer); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer permissions error = %v", err)
	}

	// Org-scoped create: an Organization Admin can create a role in their own org.
	created, err := service.CreateRole(ctx, orgAdmin, CreateRoleInput{
		Name: "Plant Reviewer", Description: "Reviews plant data", PermissionIDs: []string{permissions[0].ID, permissions[1].ID},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if created.IsSystem || created.OrganizationID == nil || *created.OrganizationID != uuidString(orgA) || len(created.PermissionIDs) != 2 {
		t.Fatalf("created role = %+v", created)
	}
	if _, err = service.CreateRole(ctx, viewer, CreateRoleInput{Name: "Denied", Description: ""}, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer create error = %v", err)
	}

	// Global create requires the global grant; an org-scoped admin cannot create a global role.
	if _, err = service.CreateRole(ctx, orgAdmin, CreateRoleInput{Global: true, Name: "Cross Tenant", Description: ""}, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("org admin global create error = %v", err)
	}
	globalRole, err := service.CreateRole(ctx, platformAdmin, CreateRoleInput{Global: true, Name: "Cross Tenant Reviewer", Description: "", PermissionIDs: []string{permissions[0].ID}}, nil)
	if err != nil || globalRole.OrganizationID != nil {
		t.Fatalf("global role = %+v, err = %v", globalRole, err)
	}

	// System roles are immutable regardless of caller permission.
	if _, err = service.UpdateRole(ctx, platformAdmin, "00000000-0000-4000-8000-000000000206", UpdateRoleInput{Name: "Viewer", Description: "x"}, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("system role update error = %v", err)
	}
	if err = service.DeleteRole(ctx, platformAdmin, "00000000-0000-4000-8000-000000000206", nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("system role delete error = %v", err)
	}

	// Update replaces the permission set wholesale.
	updated, err := service.UpdateRole(ctx, orgAdmin, created.ID, UpdateRoleInput{Name: "Plant Reviewer v2", Description: "Updated", PermissionIDs: []string{permissions[0].ID}}, nil)
	if err != nil || updated.Name != "Plant Reviewer v2" || len(updated.PermissionIDs) != 1 {
		t.Fatalf("updated role = %+v, err = %v", updated, err)
	}
	detail, err := service.RoleDetail(ctx, orgAdmin, created.ID)
	if err != nil || len(detail.PermissionIDs) != 1 || detail.PermissionIDs[0] != permissions[0].ID {
		t.Fatalf("role detail = %+v, err = %v", detail, err)
	}

	// A role assigned to a user cannot be deleted until unassigned.
	if _, err = pool.Exec(ctx, `INSERT INTO user_role(id,organization_id,user_id,role_id) VALUES($1,$2,$3,$4)`,
		mustUUID(t, "20000000-0000-4000-8000-000000000024"), orgA, assignedUserID, mustUUID(t, created.ID)); err != nil {
		t.Fatal(err)
	}
	if err = service.DeleteRole(ctx, orgAdmin, created.ID, nil); !errors.Is(err, ErrRoleInUse) {
		t.Fatalf("in-use delete error = %v", err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM user_role WHERE role_id=$1 AND user_id=$2`, mustUUID(t, created.ID), assignedUserID); err != nil {
		t.Fatal(err)
	}
	if err = service.DeleteRole(ctx, orgAdmin, created.ID, nil); err != nil {
		t.Fatalf("delete after unassignment: %v", err)
	}
	if _, err = service.RoleDetail(ctx, orgAdmin, created.ID); !errors.Is(err, ErrRoleNotFound) {
		t.Fatalf("post-delete read error = %v", err)
	}
}
