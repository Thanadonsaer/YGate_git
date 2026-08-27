package core

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestValidateUserInput(t *testing.T) {
	email, username, displayName, err := validateUserInput(" Admin@Example.COM ", " Ops.User ", "  Ops User  ", "Ygate@2026Review!")
	if err != nil {
		t.Fatal(err)
	}
	if email != "admin@example.com" || username != "ops.user" || displayName != "Ops User" {
		t.Fatalf("normalized=%q %q %q", email, username, displayName)
	}
}

func TestValidateUserInputRejectsBadValues(t *testing.T) {
	for _, test := range []struct{ email, username, displayName, password string }{
		{email: "bad", displayName: "User", password: "Ygate@2026Review!"},
		{email: "user@example.com", username: "has space", displayName: "User", password: "Ygate@2026Review!"},
		{email: "user@example.com", displayName: "", password: "Ygate@2026Review!"},
		{email: "user@example.com", displayName: "User", password: "short"},
	} {
		_, _, _, err := validateUserInput(test.email, test.username, test.displayName, test.password)
		if !errors.Is(err, ErrUserInvalid) {
			t.Fatalf("input=%+v err=%v", test, err)
		}
	}
}

func TestOptionalOrganizationIDForAuditAllowsUnassignedUser(t *testing.T) {
	if got, err := optionalOrganizationIDForAudit(""); err != nil || got.Valid {
		t.Fatalf("unassigned organization = %#v, err=%v", got, err)
	}
	if got, err := optionalOrganizationIDForAudit("10000000-0000-4000-8000-000000000001"); err != nil || !got.Valid {
		t.Fatalf("assigned organization = %#v, err=%v", got, err)
	}
}

func TestValidateUserProfileDoesNotRequirePassword(t *testing.T) {
	email, username, displayName, err := validateUserProfile(" USER@EXAMPLE.COM ", " operator ", " Operator ")
	if err != nil || email != "user@example.com" || username != "operator" || displayName != "Operator" {
		t.Fatalf("profile=%q %q %q err=%v", email, username, displayName, err)
	}
}

func TestHasRoleMatchesExactBaselineRole(t *testing.T) {
	roles := []string{"Engineer", "Platform Admin"}
	if !hasRole(roles, "Platform Admin") || hasRole(roles, "Admin") {
		t.Fatalf("roles=%v", roles)
	}
}

func TestCanResetUserPasswordRequiresGlobalPermissionForPlatformAdmin(t *testing.T) {
	if canResetUserPassword([]string{"System Admin"}, false) {
		t.Fatal("organization-scoped permission must not reset a platform admin password")
	}
	if !canResetUserPassword([]string{"System Admin"}, true) {
		t.Fatal("global reset permission should reset a platform admin password")
	}
	if !canResetUserPassword([]string{"Operator"}, false) {
		t.Fatal("ordinary organization users remain governed by organization permission")
	}
}

func TestSelfProfileIncludesRolesInJSON(t *testing.T) {
	data, err := json.Marshal(SelfProfile{Roles: []string{"Operator"}})
	if err != nil || string(data) == "{}" || !strings.Contains(string(data), `"roles":["Operator"]`) {
		t.Fatalf("profile JSON=%s err=%v", data, err)
	}
}
