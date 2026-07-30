package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestValidateRoleInput(t *testing.T) {
	permissionA := "00000000-0000-4000-8000-000000000110"
	permissionB := "00000000-0000-4000-8000-000000000111"
	name, description, ids, err := validateRoleInput("  Plant Reviewer  ", "  Reviews plant data  ", []string{permissionA, permissionA, permissionB})
	if err != nil {
		t.Fatal(err)
	}
	if name != "Plant Reviewer" || description != "Reviews plant data" {
		t.Fatalf("normalized = %q, %q", name, description)
	}
	if len(ids) != 2 {
		t.Fatalf("expected duplicate permission ids to be deduplicated, got %d", len(ids))
	}
}

func TestValidateRoleInputRejectsBadValues(t *testing.T) {
	for _, test := range []struct {
		name          string
		roleName      string
		description   string
		permissionIDs []string
	}{
		{name: "empty name", roleName: "  ", description: "ok"},
		{name: "name too long", roleName: strings.Repeat("a", 101), description: "ok"},
		{name: "description too long", roleName: "ok", description: strings.Repeat("a", 501)},
		{name: "invalid permission id", roleName: "ok", description: "ok", permissionIDs: []string{"not-a-uuid"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, _, err := validateRoleInput(test.roleName, test.description, test.permissionIDs); !errors.Is(err, ErrRoleInvalid) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestOrgPointerAndOrgOrNil(t *testing.T) {
	if orgPointer(pgtype.UUID{}) != nil {
		t.Fatal("expected nil pointer for invalid uuid")
	}
	if orgOrNil(pgtype.UUID{}) != nil {
		t.Fatal("expected nil value for invalid uuid")
	}
	id := mustUUID(t, "00000000-0000-4000-8000-000000000201")
	if got := orgPointer(id); got == nil || *got != uuidString(id) {
		t.Fatalf("orgPointer = %v", got)
	}
	if got := orgOrNil(id); got == nil {
		t.Fatal("expected non-nil value for valid uuid")
	}
}
