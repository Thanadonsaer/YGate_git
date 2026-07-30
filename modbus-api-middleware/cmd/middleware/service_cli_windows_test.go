//go:build windows

package main

import (
	"strings"
	"testing"
)

func TestServiceBinaryPathStartsServerOnlyAsInstalledService(t *testing.T) {
	path := serviceBinaryPath(`C:\YGate\middleware.exe`, serviceConfig{
		DatabasePath:         `C:\YGate\middleware.db`,
		Listen:               "0.0.0.0:8081",
		LicenseFile:          `C:\YGate\license.json`,
		CleanupRetentionDays: 30,
	})
	for _, required := range []string{`"C:\YGate\middleware.exe"`, `-db "C:\YGate\middleware.db"`, "-listen 0.0.0.0:8081", "-require-license", `-license-file "C:\YGate\license.json"`} {
		if !strings.Contains(path, required) {
			t.Fatalf("binPath missing %q: %s", required, path)
		}
	}
}

func TestManageServiceRejectsUnknownAction(t *testing.T) {
	if err := manageService(serviceConfig{Action: "unknown"}); err == nil {
		t.Fatal("expected unknown action error")
	}
}
