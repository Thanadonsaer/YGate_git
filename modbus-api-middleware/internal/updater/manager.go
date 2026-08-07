// Package updater holds the middleware's self-update lifecycle (stage,
// apply, rollback, restart-only) as a standalone Manager, so both the local
// web UI (internal/web/update.go) and the remote WS command channel
// (internal/realtimeclient) drive the exact same logic instead of two
// parallel implementations.
package updater

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const AppName = "chpp-middleware"

const windowsUpdaterScript = `param(
  [Parameter(Mandatory=$true)][string]$Source,
  [Parameter(Mandatory=$true)][string]$Destination,
  [Parameter(Mandatory=$true)][string]$ServiceName,
  [Parameter(Mandatory=$true)][string]$ResultFile,
  [string]$ExpectedVersion
)
$ErrorActionPreference = "Stop"
try {
  Start-Sleep -Seconds 2
  $service = Get-Service -Name $ServiceName
  if ($service.Status -ne "Stopped") {
    Stop-Service -Name $ServiceName -Force
    $service.WaitForStatus("Stopped", [TimeSpan]::FromSeconds(30))
  }
  $copied = $false
  $lastCopyError = $null
  for ($attempt = 1; $attempt -le 30; $attempt++) {
    try {
      Copy-Item -LiteralPath $Source -Destination $Destination -Force -ErrorAction Stop
      $copied = $true
      break
    } catch {
      $lastCopyError = $_.Exception
      Start-Sleep -Seconds 1
    }
  }
  if (-not $copied) { throw "copy failed after 30 attempts: $($lastCopyError.Message)" }
  Start-Service -Name $ServiceName
  (Get-Service -Name $ServiceName).WaitForStatus("Running", [TimeSpan]::FromSeconds(30))
  $reported = (& $Destination -version 2>&1 | Out-String).Trim()
  if ($ExpectedVersion -and $reported -notmatch [regex]::Escape($ExpectedVersion)) { throw "version mismatch: expected $ExpectedVersion, got $reported" }
  "SUCCESS $(Get-Date -Format o) version=$reported" | Set-Content -LiteralPath $ResultFile -ErrorAction SilentlyContinue
} catch {
  "FAILED $(Get-Date -Format o): $($_.Exception.Message)" | Set-Content -LiteralPath $ResultFile -ErrorAction SilentlyContinue
  try { Start-Service -Name $ServiceName -ErrorAction SilentlyContinue } catch {}
  exit 1
}
`

type Manifest struct {
	App     string `json:"app"`
	Version string `json:"version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Binary  string `json:"binary"`
	SHA256  string `json:"sha256"`
}

type Status struct {
	Enabled        bool      `json:"enabled"`
	CanApply       bool      `json:"canApply"`
	CurrentVersion string    `json:"currentVersion"`
	OS             string    `json:"os"`
	Arch           string    `json:"arch"`
	ServiceName    string    `json:"serviceName"`
	Staged         *Manifest `json:"staged,omitempty"`
	Backup         string    `json:"backup,omitempty"`
	Message        string    `json:"message,omitempty"`
	// LastApplyResult is the most recent apply-update.ps1/.sh outcome (see
	// startUpdater's -ResultFile) -- "SUCCESS ..." or "FAILED ...: <reason>".
	// The stop/copy/start script runs detached (fire-and-forget) from Apply,
	// so this file is the only place a failed Copy-Item (permission, file
	// lock, disk full, etc) is recorded; without surfacing it, the operator
	// only sees the service bounce back up on its old, unreplaced binary
	// with no error anywhere.
	LastApplyResult string `json:"lastApplyResult,omitempty"`
}

// Manager is safe to share between the local web UI and the realtime
// command handler -- both drive the same staged/backups directories on
// disk, so an Apply triggered from either surface sees the same state.
type Manager struct {
	Version  string
	CanApply bool
	Root     string // override for tests; empty uses the exe's own directory
}

func (m *Manager) Status() Status {
	staged, _ := readManifest(filepath.Join(m.root(), "staged", "update-manifest.json"))
	backup, _ := latestBackup(filepath.Join(m.root(), "backups"))
	backupName := ""
	if backup != "" {
		backupName = filepath.Base(backup)
	}
	msg := ""
	if !m.CanApply {
		msg = "Upload/stage enabled; Apply requires Windows Service or Linux systemd runtime"
	}
	lastResult := ""
	if data, err := os.ReadFile(filepath.Join(m.root(), "run", "last-result.txt")); err == nil {
		lastResult = strings.TrimSpace(string(data))
	}
	return Status{Enabled: true, CanApply: m.CanApply, CurrentVersion: m.Version, OS: runtime.GOOS, Arch: runtime.GOARCH, ServiceName: AppName, Staged: staged, Backup: backupName, Message: msg, LastApplyResult: lastResult}
}

// StageZip validates a patch zip (update-manifest.json + a binary matching
// its declared sha256) and stores it as the pending update, replacing
// whatever was staged before.
func (m *Manager) StageZip(data []byte) (*Manifest, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	var manifest *Manifest
	for _, f := range zr.File {
		base := filepath.Base(f.Name)
		if strings.EqualFold(base, "middleware.db") {
			return nil, fmt.Errorf("patch zip must not include middleware.db")
		}
		if base == "update-manifest.json" {
			r, err := f.Open()
			if err != nil {
				return nil, err
			}
			raw, err := io.ReadAll(r)
			_ = r.Close()
			if err != nil {
				return nil, err
			}
			manifest = &Manifest{}
			if err = json.Unmarshal(trimUTF8BOM(raw), manifest); err != nil {
				return nil, err
			}
		}
	}
	if manifest == nil {
		return nil, fmt.Errorf("update-manifest.json is required")
	}
	if err = validateManifest(manifest); err != nil {
		return nil, err
	}
	var binFile *zip.File
	for _, f := range zr.File {
		if filepath.Base(f.Name) == manifest.Binary {
			binFile = f
			break
		}
	}
	if binFile == nil {
		return nil, fmt.Errorf("binary %q not found in patch", manifest.Binary)
	}
	r, err := binFile.Open()
	if err != nil {
		return nil, err
	}
	bin, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(bin)
	if hex.EncodeToString(sum[:]) != strings.ToLower(strings.TrimSpace(manifest.SHA256)) {
		return nil, fmt.Errorf("binary sha256 mismatch")
	}
	stage := filepath.Join(m.root(), "staged")
	if err = os.RemoveAll(stage); err != nil {
		return nil, err
	}
	if err = os.MkdirAll(stage, 0755); err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(stage, manifest.Binary), bin, 0755); err != nil {
		return nil, err
	}
	b, _ := json.MarshalIndent(manifest, "", "  ")
	if err = os.WriteFile(filepath.Join(stage, "update-manifest.json"), b, 0644); err != nil {
		return nil, err
	}
	return manifest, nil
}

// Apply backs up the running binary and starts the OS-specific
// stop/copy/start script against whatever is currently staged. Returns
// once the script has been *started* (fire-and-forget) -- the actual
// service restart happens a couple seconds later.
func (m *Manager) Apply() (backupName string, err error) {
	if !m.CanApply {
		return "", fmt.Errorf("apply requires Windows Service or Linux systemd runtime")
	}
	manifest, err := readManifest(filepath.Join(m.root(), "staged", "update-manifest.json"))
	if err != nil || manifest == nil {
		return "", fmt.Errorf("no staged patch")
	}
	backup, err := m.backupCurrentBinary()
	if err != nil {
		return "", err
	}
	if err = m.startUpdater(filepath.Join(m.root(), "staged", manifest.Binary), manifest.Version); err != nil {
		return "", err
	}
	return filepath.Base(backup), nil
}

// Rollback starts the same stop/copy/start script pointed at the most
// recent pre-Apply backup.
func (m *Manager) Rollback() (backupName string, err error) {
	if !m.CanApply {
		return "", fmt.Errorf("rollback requires Windows Service or Linux systemd runtime")
	}
	backup, err := latestBackup(filepath.Join(m.root(), "backups"))
	if err != nil {
		return "", err
	}
	if err = m.startUpdater(backup, ""); err != nil {
		return "", err
	}
	return filepath.Base(backup), nil
}

// RestartOnly stops+starts the service without touching the binary at all
// (source == destination, so the script's copy step is a no-op overwrite).
func (m *Manager) RestartOnly() error {
	if !m.CanApply {
		return fmt.Errorf("restart requires Windows Service or Linux systemd runtime")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return m.startUpdater(exe, "")
}

func (m *Manager) root() string {
	if m.Root != "" {
		return m.Root
	}
	exe, err := os.Executable()
	if err != nil {
		return "updates"
	}
	return filepath.Join(filepath.Dir(exe), "updates")
}

func validateManifest(m *Manifest) error {
	if m.App != AppName {
		return fmt.Errorf("manifest app must be %s", AppName)
	}
	if m.Version == "" || m.OS != runtime.GOOS || m.Arch != runtime.GOARCH {
		return fmt.Errorf("manifest target mismatch: want %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if m.Binary == "" || filepath.Base(m.Binary) != m.Binary || strings.Contains(m.Binary, "..") {
		return fmt.Errorf("invalid binary name")
	}
	if _, err := hex.DecodeString(strings.TrimSpace(m.SHA256)); err != nil || len(strings.TrimSpace(m.SHA256)) != 64 {
		return fmt.Errorf("invalid binary sha256")
	}
	return nil
}

func readManifest(path string) (*Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err = json.Unmarshal(trimUTF8BOM(b), &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func trimUTF8BOM(b []byte) []byte {
	return bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf})
}

func (m *Manager) backupCurrentBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	in, err := os.Open(exe)
	if err != nil {
		return "", err
	}
	defer in.Close()
	backupDir := filepath.Join(m.root(), "backups")
	if err = os.MkdirAll(backupDir, 0755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("middleware-%s-%s%s", m.Version, time.Now().Format("20060102-150405"), filepath.Ext(exe))
	path := filepath.Join(backupDir, name)
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0755)
	if err != nil {
		return "", err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return path, err
}

func latestBackup(dir string) (string, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var files []os.DirEntry
	for _, item := range items {
		if !item.IsDir() {
			files = append(files, item)
		}
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no backup found")
	}
	sort.Slice(files, func(i, j int) bool {
		a, _ := files[i].Info()
		b, _ := files[j].Info()
		return a.ModTime().After(b.ModTime())
	})
	return filepath.Join(dir, files[0].Name()), nil
}

func (m *Manager) startUpdater(source, expectedVersion string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	scriptDir := filepath.Join(m.root(), "run")
	if err = os.MkdirAll(scriptDir, 0755); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		script := filepath.Join(scriptDir, "apply-update.ps1")
		result := filepath.Join(scriptDir, "last-result.txt")
		if err = os.WriteFile(script, []byte(windowsUpdaterScript), 0644); err != nil {
			return err
		}
		return exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script, "-Source", source, "-Destination", exe, "-ServiceName", AppName, "-ResultFile", result, "-ExpectedVersion", expectedVersion).Start()
	}
	script := filepath.Join(scriptDir, "apply-update.sh")
	body := fmt.Sprintf("#!/bin/sh\nsleep 2\nsystemctl stop %s >/dev/null 2>&1 || true\ninstall -m 0755 %q %q\nsystemctl start %s\n", AppName, source, exe, AppName)
	if err = os.WriteFile(script, []byte(body), 0755); err != nil {
		return err
	}
	if _, err = exec.LookPath("systemd-run"); err == nil {
		return exec.Command("systemd-run", "--unit", "chpp-middleware-update", "--collect", "sh", script).Start()
	}
	return exec.Command("sh", script).Start()
}
