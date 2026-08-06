package main

import (
	"testing"
)

func TestBootstrapInputFromEnvironment(t *testing.T) {
	t.Setenv("PLATFORM_BOOTSTRAP_EMAIL", "admin@example.com")
	t.Setenv("PLATFORM_BOOTSTRAP_USERNAME", "Admin")
	t.Setenv("PLATFORM_BOOTSTRAP_DISPLAY_NAME", "Platform Admin")
	t.Setenv("PLATFORM_BOOTSTRAP_PASSWORD", "long-test-password")
	t.Setenv("PLATFORM_BOOTSTRAP_ORGANIZATION_CODE", "ORG-001")
	t.Setenv("PLATFORM_BOOTSTRAP_ORGANIZATION_NAME", "Example Organization")
	input, err := bootstrapInputFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if input.email != "admin@example.com" || input.username != "admin" || input.organizationCode != "ORG-001" || input.role != "System Admin" {
		t.Fatalf("input=%+v", input)
	}
}

func TestBootstrapInputRejectsWeakPassword(t *testing.T) {
	t.Setenv("PLATFORM_BOOTSTRAP_EMAIL", "admin@example.com")
	t.Setenv("PLATFORM_BOOTSTRAP_DISPLAY_NAME", "Platform Admin")
	t.Setenv("PLATFORM_BOOTSTRAP_PASSWORD", "short")
	t.Setenv("PLATFORM_BOOTSTRAP_ORGANIZATION_CODE", "ORG-001")
	t.Setenv("PLATFORM_BOOTSTRAP_ORGANIZATION_NAME", "Example Organization")
	if _, err := bootstrapInputFromEnvironment(); err == nil {
		t.Fatal("expected weak password error")
	}
}

func TestMiddlewareInputFromEnvironment(t *testing.T) {
	t.Setenv("PLATFORM_MIDDLEWARE_NAME", "Site Gateway")
	t.Setenv("PLATFORM_MIDDLEWARE_ORGANIZATION_CODE", "YGATE")
	t.Setenv("PLATFORM_MIDDLEWARE_AUTO_ONBOARD", "true")
	input, err := middlewareInputFromEnvironment()
	if err != nil || input.name != "Site Gateway" || input.organizationCode != "YGATE" || !input.autoOnboard {
		t.Fatalf("input=%+v err=%v", input, err)
	}
}
