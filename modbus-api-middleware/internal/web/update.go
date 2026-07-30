package web

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const updateAppName = "chpp-middleware"

const windowsUpdaterScript = `param(
  [Parameter(Mandatory=$true)][string]$Source,
  [Parameter(Mandatory=$true)][string]$Destination,
  [Parameter(Mandatory=$true)][string]$ServiceName,
  [Parameter(Mandatory=$true)][string]$ResultFile
)
$ErrorActionPreference = "Stop"
try {
  Start-Sleep -Seconds 2
  $service = Get-Service -Name $ServiceName
  if ($service.Status -ne "Stopped") {
    Stop-Service -Name $ServiceName -Force
    $service.WaitForStatus("Stopped", [TimeSpan]::FromSeconds(30))
  }
  Copy-Item -LiteralPath $Source -Destination $Destination -Force
  Start-Service -Name $ServiceName
  (Get-Service -Name $ServiceName).WaitForStatus("Running", [TimeSpan]::FromSeconds(30))
  "SUCCESS $(Get-Date -Format o)" | Set-Content -LiteralPath $ResultFile -ErrorAction SilentlyContinue
} catch {
  "FAILED $(Get-Date -Format o): $($_.Exception.Message)" | Set-Content -LiteralPath $ResultFile -ErrorAction SilentlyContinue
  try { Start-Service -Name $ServiceName -ErrorAction SilentlyContinue } catch {}
  exit 1
}
`

type updateManifest struct {
	App     string `json:"app"`
	Version string `json:"version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Binary  string `json:"binary"`
	SHA256  string `json:"sha256"`
}

type updateStatus struct {
	Enabled        bool            `json:"enabled"`
	CanApply       bool            `json:"canApply"`
	CurrentVersion string          `json:"currentVersion"`
	OS             string          `json:"os"`
	Arch           string          `json:"arch"`
	ServiceName    string          `json:"serviceName"`
	Staged         *updateManifest `json:"staged,omitempty"`
	Backup         string          `json:"backup,omitempty"`
	Message        string          `json:"message,omitempty"`
}

func (s *Server) updateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	staged, _ := readUpdateManifest(filepath.Join(s.updateRoot(), "staged", "update-manifest.json"))
	backup, _ := latestBackup(filepath.Join(s.updateRoot(), "backups"))
	backupName := ""
	if backup != "" {
		backupName = filepath.Base(backup)
	}
	msg := ""
	if !s.CanApplyUpdate {
		msg = "Upload/stage enabled; Apply requires Windows Service or Linux systemd runtime"
	}
	writeJSON(w, http.StatusOK, updateStatus{Enabled: true, CanApply: s.CanApplyUpdate, CurrentVersion: s.Version, OS: runtime.GOOS, Arch: runtime.GOARCH, ServiceName: updateAppName, Staged: staged, Backup: backupName, Message: msg})
}

func (s *Server) updateUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<20)
	file, _, err := r.FormFile("patch")
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("patch file is required"))
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	m, err := s.stageUpdateZip(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"staged": m})
}

func (s *Server) updateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.CanApplyUpdate {
		writeError(w, http.StatusBadRequest, fmt.Errorf("apply requires Windows Service or Linux systemd runtime"))
		return
	}
	m, err := readUpdateManifest(filepath.Join(s.updateRoot(), "staged", "update-manifest.json"))
	if err != nil || m == nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("no staged patch"))
		return
	}
	backup, err := s.backupCurrentBinary()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err = s.startUpdater(filepath.Join(s.updateRoot(), "staged", m.Binary)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "update started; service will restart", "backup": filepath.Base(backup), "staged": m})
}

func (s *Server) updateRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.CanApplyUpdate {
		writeError(w, http.StatusBadRequest, fmt.Errorf("rollback requires Windows Service or Linux systemd runtime"))
		return
	}
	backup, err := latestBackup(filepath.Join(s.updateRoot(), "backups"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err = s.startUpdater(backup); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "rollback started; service will restart", "backup": filepath.Base(backup)})
}

func (s *Server) updateRoot() string {
	if s.UpdateRoot != "" {
		return s.UpdateRoot
	}
	exe, err := os.Executable()
	if err != nil {
		return "updates"
	}
	return filepath.Join(filepath.Dir(exe), "updates")
}

func (s *Server) stageUpdateZip(data []byte) (*updateManifest, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	var manifest *updateManifest
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
			manifest = &updateManifest{}
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
	stage := filepath.Join(s.updateRoot(), "staged")
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

func validateManifest(m *updateManifest) error {
	if m.App != updateAppName {
		return fmt.Errorf("manifest app must be %s", updateAppName)
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

func readUpdateManifest(path string) (*updateManifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m updateManifest
	if err = json.Unmarshal(trimUTF8BOM(b), &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func trimUTF8BOM(b []byte) []byte {
	return bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf})
}

func (s *Server) backupCurrentBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	in, err := os.Open(exe)
	if err != nil {
		return "", err
	}
	defer in.Close()
	backupDir := filepath.Join(s.updateRoot(), "backups")
	if err = os.MkdirAll(backupDir, 0755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("middleware-%s-%s%s", s.Version, time.Now().Format("20060102-150405"), filepath.Ext(exe))
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

func (s *Server) startUpdater(source string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	scriptDir := filepath.Join(s.updateRoot(), "run")
	if err = os.MkdirAll(scriptDir, 0755); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		script := filepath.Join(scriptDir, "apply-update.ps1")
		result := filepath.Join(scriptDir, "last-result.txt")
		if err = os.WriteFile(script, []byte(windowsUpdaterScript), 0644); err != nil {
			return err
		}
		return exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script, "-Source", source, "-Destination", exe, "-ServiceName", updateAppName, "-ResultFile", result).Start()
	}
	script := filepath.Join(scriptDir, "apply-update.sh")
	body := fmt.Sprintf("#!/bin/sh\nsleep 2\nsystemctl stop %s >/dev/null 2>&1 || true\ninstall -m 0755 %q %q\nsystemctl start %s\n", updateAppName, source, exe, updateAppName)
	if err = os.WriteFile(script, []byte(body), 0755); err != nil {
		return err
	}
	if _, err = exec.LookPath("systemd-run"); err == nil {
		return exec.Command("systemd-run", "--unit", "chpp-middleware-update", "--collect", "sh", script).Start()
	}
	return exec.Command("sh", script).Start()
}
