package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestUserInputFromEnvironment(t *testing.T) {
	t.Setenv("PLATFORM_BOOTSTRAP_EMAIL", "admin@example.com")
	t.Setenv("PLATFORM_BOOTSTRAP_USERNAME", "Admin")
	t.Setenv("PLATFORM_BOOTSTRAP_DISPLAY_NAME", "Platform Admin")
	t.Setenv("PLATFORM_BOOTSTRAP_PASSWORD", "long-test-password")
	t.Setenv("PLATFORM_BOOTSTRAP_ORGANIZATION_CODE", "ORG-001")
	t.Setenv("PLATFORM_BOOTSTRAP_ORGANIZATION_NAME", "Example Organization")
	input, err := UserInputFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if input.Email != "admin@example.com" || input.Username != "admin" || input.OrganizationCode != "ORG-001" || input.Role != "System Admin" {
		t.Fatalf("input=%+v", input)
	}
}

func TestUserInputRejectsWeakPassword(t *testing.T) {
	t.Setenv("PLATFORM_BOOTSTRAP_EMAIL", "admin@example.com")
	t.Setenv("PLATFORM_BOOTSTRAP_DISPLAY_NAME", "Platform Admin")
	t.Setenv("PLATFORM_BOOTSTRAP_PASSWORD", "short")
	t.Setenv("PLATFORM_BOOTSTRAP_ORGANIZATION_CODE", "ORG-001")
	t.Setenv("PLATFORM_BOOTSTRAP_ORGANIZATION_NAME", "Example Organization")
	if _, err := UserInputFromEnvironment(); err == nil {
		t.Fatal("expected weak password error")
	}
}

func TestMiddlewareInputFromEnvironment(t *testing.T) {
	t.Setenv("PLATFORM_MIDDLEWARE_NAME", "Site Gateway")
	t.Setenv("PLATFORM_MIDDLEWARE_ORGANIZATION_CODE", "YGATE")
	t.Setenv("PLATFORM_MIDDLEWARE_AUTO_ONBOARD", "true")
	input, err := MiddlewareInputFromEnvironment()
	if err != nil || input.Name != "Site Gateway" || input.OrganizationCode != "YGATE" || !input.AutoOnboard {
		t.Fatalf("input=%+v err=%v", input, err)
	}
}

type existsRow bool

func (r existsRow) Scan(dest ...any) error {
	*(dest[0].(*bool)) = bool(r)
	return nil
}

type fakePool struct {
	admin bool
	began bool
}

func (p *fakePool) QueryRow(context.Context, string, ...any) pgx.Row { return existsRow(p.admin) }

func (p *fakePool) Begin(context.Context) (pgx.Tx, error) {
	p.began = true
	return nil, errors.New("bootstrap must not write when an admin already exists")
}

func TestEnsureAdminSkipsWhenAdminExists(t *testing.T) {
	pool := &fakePool{admin: true}
	if err := EnsureAdmin(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	if pool.began {
		t.Fatal("EnsureAdmin started a transaction despite an existing System Admin")
	}
}

func TestEnsureAdminReportsUnusableEnvironment(t *testing.T) {
	t.Setenv("PLATFORM_BOOTSTRAP_EMAIL", "")
	err := EnsureAdmin(context.Background(), &fakePool{admin: false})
	if err == nil || !strings.Contains(err.Error(), "PLATFORM_BOOTSTRAP_") {
		t.Fatalf("err=%v", err)
	}
}
