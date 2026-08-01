package updater

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWindowsUpdaterWaitsForServiceAndReportsResult(t *testing.T) {
	for _, required := range []string{"WaitForStatus(\"Stopped\"", "Copy-Item -LiteralPath", "WaitForStatus(\"Running\"", "$ResultFile"} {
		if !strings.Contains(windowsUpdaterScript, required) {
			t.Fatalf("updater script missing %q", required)
		}
	}
	if strings.Contains(windowsUpdaterScript, "timeout /t") {
		t.Fatal("updater must not use timeout in a non-interactive service")
	}
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "middleware.exe"
	}
	return "middleware"
}

func buildPatchZip(t *testing.T, manifest Manifest, bin []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mw, err := zw.Create("update-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.NewEncoder(mw).Encode(manifest); err != nil {
		t.Fatal(err)
	}
	bw, err := zw.Create(manifest.Binary)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bw.Write(bin); err != nil {
		t.Fatal(err)
	}
	if err = zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestManagerStageApplyRollbackRestart(t *testing.T) {
	root := t.TempDir()
	bin := []byte("binary-contents")
	sum := sha256.Sum256(bin)
	manifest := Manifest{App: AppName, Version: "0.1.1", OS: runtime.GOOS, Arch: runtime.GOARCH, Binary: binaryName(), SHA256: hex.EncodeToString(sum[:])}
	mgr := &Manager{Version: "0.1.0", CanApply: false, Root: root}

	if _, err := mgr.StageZip(buildPatchZip(t, manifest, bin)); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "staged", manifest.Binary)); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Apply(); err == nil {
		t.Fatal("expected Apply to fail when CanApply is false")
	}

	mgr.CanApply = true
	// os.Executable() in a `go test` binary is a real, readable file, so
	// Apply's backupCurrentBinary step succeeds; startUpdater then just
	// spawns a script (fire-and-forget) which we don't wait on here.
	if _, err := mgr.Apply(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "backups")); err != nil {
		t.Fatalf("expected a backup directory: %v", err)
	}
	if _, err := mgr.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
}
