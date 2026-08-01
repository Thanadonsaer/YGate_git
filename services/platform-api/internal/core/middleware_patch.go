package core

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/database/dbgen"
)

// middlewarePatchAppName must match modbus-api-middleware/internal/web/
// update.go's updateAppName constant -- platform-api and modbus-api-
// middleware are separate Go modules (no shared package), so this manifest
// shape is duplicated by necessity, not by choice (see design spec).
const middlewarePatchAppName = "chpp-middleware"

var ErrMiddlewarePatchNotFound = errors.New("middleware patch not found")

type MiddlewarePatch struct {
	ID             string    `json:"id"`
	Version        string    `json:"version"`
	OS             string    `json:"os"`
	Arch           string    `json:"arch"`
	BinaryFilename string    `json:"binaryFilename"`
	SHA256         string    `json:"sha256"`
	FileSizeBytes  int64     `json:"fileSizeBytes"`
	UploadedBy     *string   `json:"uploadedBy"`
	CreatedAt      time.Time `json:"createdAt"`
}

type patchManifest struct {
	App     string `json:"app"`
	Version string `json:"version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Binary  string `json:"binary"`
	SHA256  string `json:"sha256"`
}

// UploadMiddlewarePatch validates a patch zip (same update-manifest.json +
// signed-binary shape modbus-api-middleware's own stageUpdateZip validates)
// and stores it under s.patchDir for later Stage-to-a-Middleware.
func (s *Service) UploadMiddlewarePatch(ctx context.Context, principal auth.Principal, data []byte, sourceIP *netip.Addr) (MiddlewarePatch, error) {
	if err := s.requireGlobalPermission(ctx, principal, "create", "middleware_patch"); err != nil {
		return MiddlewarePatch{}, err
	}
	manifest, err := parsePatchZip(data)
	if err != nil {
		return MiddlewarePatch{}, fmt.Errorf("%w: %v", ErrMiddlewareInvalid, err)
	}

	id, err := newUUID()
	if err != nil {
		return MiddlewarePatch{}, err
	}
	if err = os.MkdirAll(s.patchDir, 0o755); err != nil {
		return MiddlewarePatch{}, fmt.Errorf("create patch dir: %w", err)
	}
	storagePath := filepath.Join(s.patchDir, uuidString(id)+".zip")
	if err = os.WriteFile(storagePath, data, 0o644); err != nil {
		return MiddlewarePatch{}, fmt.Errorf("store patch file: %w", err)
	}

	correlationID, err := newUUID()
	if err != nil {
		return MiddlewarePatch{}, err
	}
	patch := MiddlewarePatch{
		ID: uuidString(id), Version: manifest.Version, OS: manifest.OS, Arch: manifest.Arch,
		BinaryFilename: manifest.Binary, SHA256: strings.ToLower(manifest.SHA256), FileSizeBytes: int64(len(data)),
	}
	if _, err = s.pool.Exec(ctx, `
INSERT INTO middleware_patch (id, version, os, arch, binary_filename, sha256, file_size_bytes, storage_path, uploaded_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		id, patch.Version, patch.OS, patch.Arch, patch.BinaryFilename, patch.SHA256, patch.FileSizeBytes, storagePath, principal.UserID); err != nil {
		_ = os.Remove(storagePath)
		return MiddlewarePatch{}, mapMiddlewareWriteError(err)
	}
	after, _ := json.Marshal(patch)
	_ = s.queries.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		ActorUserID: principal.UserID, Action: "middleware_patch.uploaded",
		TargetType: "middleware_patch", TargetID: id, AfterData: after, SourceIp: sourceIP, CorrelationID: correlationID,
	})
	return patch, nil
}

func parsePatchZip(data []byte) (patchManifest, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return patchManifest{}, err
	}
	var manifest *patchManifest
	for _, f := range zr.File {
		if filepath.Base(f.Name) == "update-manifest.json" {
			r, err := f.Open()
			if err != nil {
				return patchManifest{}, err
			}
			raw, err := io.ReadAll(r)
			_ = r.Close()
			if err != nil {
				return patchManifest{}, err
			}
			manifest = &patchManifest{}
			if err = json.Unmarshal(raw, manifest); err != nil {
				return patchManifest{}, err
			}
		}
	}
	if manifest == nil {
		return patchManifest{}, fmt.Errorf("update-manifest.json is required")
	}
	if manifest.App != middlewarePatchAppName {
		return patchManifest{}, fmt.Errorf("manifest app must be %s", middlewarePatchAppName)
	}
	if manifest.Version == "" || manifest.OS == "" || manifest.Arch == "" {
		return patchManifest{}, fmt.Errorf("manifest version/os/arch are required")
	}
	if manifest.Binary == "" || filepath.Base(manifest.Binary) != manifest.Binary || strings.Contains(manifest.Binary, "..") {
		return patchManifest{}, fmt.Errorf("invalid binary name")
	}
	sum, err := hex.DecodeString(strings.TrimSpace(manifest.SHA256))
	if err != nil || len(sum) != sha256.Size {
		return patchManifest{}, fmt.Errorf("invalid binary sha256")
	}
	var binFile *zip.File
	for _, f := range zr.File {
		if filepath.Base(f.Name) == manifest.Binary {
			binFile = f
			break
		}
	}
	if binFile == nil {
		return patchManifest{}, fmt.Errorf("binary %q not found in patch", manifest.Binary)
	}
	r, err := binFile.Open()
	if err != nil {
		return patchManifest{}, err
	}
	binary, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		return patchManifest{}, err
	}
	actual := sha256.Sum256(binary)
	if hex.EncodeToString(actual[:]) != strings.ToLower(strings.TrimSpace(manifest.SHA256)) {
		return patchManifest{}, fmt.Errorf("binary sha256 mismatch")
	}
	return *manifest, nil
}

func (s *Service) ListMiddlewarePatches(ctx context.Context, principal auth.Principal) ([]MiddlewarePatch, error) {
	if err := s.requireGlobalPermission(ctx, principal, "read", "middleware_patch"); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT mp.id, mp.version, mp.os, mp.arch, mp.binary_filename, mp.sha256, mp.file_size_bytes, u.display_name, mp.created_at
FROM middleware_patch mp
LEFT JOIN app_user u ON u.id = mp.uploaded_by
ORDER BY mp.created_at DESC
LIMIT 200`)
	if err != nil {
		return nil, fmt.Errorf("list middleware patches: %w", err)
	}
	defer rows.Close()
	patches := []MiddlewarePatch{}
	for rows.Next() {
		var id pgtype.UUID
		var uploadedBy pgtype.Text
		var patch MiddlewarePatch
		if err = rows.Scan(&id, &patch.Version, &patch.OS, &patch.Arch, &patch.BinaryFilename, &patch.SHA256, &patch.FileSizeBytes, &uploadedBy, &patch.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan middleware patch: %w", err)
		}
		patch.ID = uuidString(id)
		if uploadedBy.Valid {
			patch.UploadedBy = &uploadedBy.String
		}
		patches = append(patches, patch)
	}
	return patches, rows.Err()
}

func (s *Service) DeleteMiddlewarePatch(ctx context.Context, principal auth.Principal, patchID string, sourceIP *netip.Addr) error {
	if err := s.requireGlobalPermission(ctx, principal, "delete", "middleware_patch"); err != nil {
		return err
	}
	id, err := parseUUID(patchID)
	if err != nil {
		return ErrMiddlewarePatchNotFound
	}
	var storagePath string
	err = s.pool.QueryRow(ctx, `DELETE FROM middleware_patch WHERE id=$1 RETURNING storage_path`, id).Scan(&storagePath)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMiddlewarePatchNotFound
	}
	if err != nil {
		return fmt.Errorf("delete middleware patch: %w", err)
	}
	_ = os.Remove(storagePath)
	correlationID, err := newUUID()
	if err != nil {
		return err
	}
	_ = s.queries.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		ActorUserID: principal.UserID, Action: "middleware_patch.deleted",
		TargetType: "middleware_patch", TargetID: id, SourceIp: sourceIP, CorrelationID: correlationID,
	})
	return nil
}

// MiddlewarePatchFilePath resolves patchID to its stored zip path for the
// download endpoint. Caller is responsible for authenticating the request
// (via the same X-Api-Key middleware auth the WS handshake uses, not a
// cookie session -- a Middleware, not a browser, calls this).
func (s *Service) MiddlewarePatchFilePath(ctx context.Context, patchID string) (path, filename string, err error) {
	id, err := parseUUID(patchID)
	if err != nil {
		return "", "", ErrMiddlewarePatchNotFound
	}
	err = s.pool.QueryRow(ctx, `SELECT storage_path, binary_filename FROM middleware_patch WHERE id=$1`, id).Scan(&path, &filename)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrMiddlewarePatchNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("lookup middleware patch: %w", err)
	}
	return path, filename, nil
}

// runMiddlewareLifecycleCommand sends kind (update.stage/update.apply/
// update.rollback/service.restart) over the same command.request/result WS
// channel ImportMiddlewareConfig uses, after checking the global
// middleware_patch/update permission and auditing the action.
func (s *Service) runMiddlewareLifecycleCommand(ctx context.Context, principal auth.Principal, middlewareID, kind string, extra map[string]any, auditAction string, sourceIP *netip.Addr) error {
	if err := s.requireGlobalPermission(ctx, principal, "update", "middleware_patch"); err != nil {
		return err
	}
	mwUUID, err := parseUUID(middlewareID)
	if err != nil {
		return ErrMiddlewareNotFound
	}
	var mwOrgID pgtype.UUID
	if err = s.pool.QueryRow(ctx, `SELECT organization_id FROM middleware_client WHERE id=$1`, mwUUID).Scan(&mwOrgID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrMiddlewareNotFound
		}
		return fmt.Errorf("lookup middleware: %w", err)
	}

	commandID, err := newUUID()
	if err != nil {
		return err
	}
	body := map[string]any{"type": "command.request", "commandId": uuidString(commandID), "kind": kind}
	for k, v := range extra {
		body[k] = v
	}
	payload, _ := json.Marshal(body)
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	raw, err := s.hub.RunCommand(runCtx, uuidString(mwUUID), uuidString(commandID), payload)
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("middleware command timed out: %w", ErrMiddlewareCommandNAK)
	}
	if err != nil {
		return ErrMiddlewareOffline
	}
	var envelope struct {
		Ok    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err = json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("decode command result: %w", err)
	}
	if !envelope.Ok {
		return fmt.Errorf("middleware %s failed: %s: %w", kind, envelope.Error, ErrMiddlewareCommandNAK)
	}

	correlationID, err := newUUID()
	if err != nil {
		return err
	}
	_ = s.queries.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: mwOrgID, ActorUserID: principal.UserID, Action: auditAction,
		TargetType: "middleware_client", TargetID: mwUUID, SourceIp: sourceIP, CorrelationID: correlationID,
	})
	return nil
}

func (s *Service) StageMiddlewareUpdate(ctx context.Context, principal auth.Principal, middlewareID, patchID string, sourceIP *netip.Addr) error {
	if s.publicBaseURL == "" {
		return fmt.Errorf("%w: PLATFORM_PUBLIC_BASE_URL is not configured", ErrMiddlewareInvalid)
	}
	id, err := parseUUID(patchID)
	if err != nil {
		return ErrMiddlewarePatchNotFound
	}
	var patch MiddlewarePatch
	err = s.pool.QueryRow(ctx, `SELECT version, os, arch, binary_filename, sha256 FROM middleware_patch WHERE id=$1`, id).
		Scan(&patch.Version, &patch.OS, &patch.Arch, &patch.BinaryFilename, &patch.SHA256)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMiddlewarePatchNotFound
	}
	if err != nil {
		return fmt.Errorf("lookup middleware patch: %w", err)
	}
	downloadURL := fmt.Sprintf("%s/api/v1/admin/middleware-patches/%s/download", s.publicBaseURL, patchID)
	return s.runMiddlewareLifecycleCommand(ctx, principal, middlewareID, "update.stage", map[string]any{
		// "patchVersion" not "version" -- the command.request envelope on the
		// middleware side embeds domain.ConfigSnapshot, whose own "version"
		// key is an int64 config version; colliding names would break JSON
		// decoding of this message entirely.
		"downloadUrl": downloadURL, "sha256": patch.SHA256, "patchVersion": patch.Version,
		"os": patch.OS, "arch": patch.Arch, "binary": patch.BinaryFilename,
	}, "middleware_client.update_staged", sourceIP)
}

func (s *Service) ApplyMiddlewareUpdate(ctx context.Context, principal auth.Principal, middlewareID string, sourceIP *netip.Addr) error {
	return s.runMiddlewareLifecycleCommand(ctx, principal, middlewareID, "update.apply", nil, "middleware_client.update_applied", sourceIP)
}

func (s *Service) RollbackMiddlewareUpdate(ctx context.Context, principal auth.Principal, middlewareID string, sourceIP *netip.Addr) error {
	return s.runMiddlewareLifecycleCommand(ctx, principal, middlewareID, "update.rollback", nil, "middleware_client.update_rolled_back", sourceIP)
}

func (s *Service) RestartMiddleware(ctx context.Context, principal auth.Principal, middlewareID string, sourceIP *netip.Addr) error {
	return s.runMiddlewareLifecycleCommand(ctx, principal, middlewareID, "service.restart", nil, "middleware_client.restarted", sourceIP)
}
