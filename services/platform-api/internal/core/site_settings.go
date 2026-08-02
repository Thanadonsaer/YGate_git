package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

type SiteSettings struct {
	SiteName    string    `json:"siteName"`
	LogoURL     *string   `json:"logoUrl"`
	AccentColor string    `json:"accentColor"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type UpdateSiteSettingsInput struct {
	SiteName    string
	AccentColor string
}

var validAccentColors = map[string]bool{"teal": true, "indigo": true, "emerald": true, "amber": true, "rose": true}

// logoExtensions allowlists the image types accepted for an uploaded site
// logo, keyed by the MIME type http.DetectContentType sniffs from the file's
// first 512 bytes (never trust the client-supplied filename/extension).
var logoExtensions = map[string]string{
	"image/png":     ".png",
	"image/jpeg":    ".jpg",
	"image/svg+xml": ".svg",
	"image/webp":    ".webp",
	"image/gif":     ".gif",
}

const maxLogoBytes = 2 << 20 // 2 MiB

// SiteSettings is unauthenticated -- the login screen renders the configured
// logo/name before a session exists.
func (s *Service) SiteSettings(ctx context.Context) (SiteSettings, error) {
	var settings SiteSettings
	var logoURL pgtype.Text
	var updatedAt pgtype.Timestamptz
	if err := s.pool.QueryRow(ctx, `SELECT site_name, logo_url, accent_color, updated_at FROM site_setting WHERE id`).
		Scan(&settings.SiteName, &logoURL, &settings.AccentColor, &updatedAt); err != nil {
		return SiteSettings{}, fmt.Errorf("read site settings: %w", err)
	}
	if logoURL.Valid {
		settings.LogoURL = &logoURL.String
	}
	settings.UpdatedAt = updatedAt.Time
	return settings, nil
}

func (s *Service) UpdateSiteSettings(ctx context.Context, principal auth.Principal, input UpdateSiteSettingsInput, sourceIP *netip.Addr) (SiteSettings, error) {
	if err := s.requireGlobalPermission(ctx, principal, "update", "site_setting"); err != nil {
		return SiteSettings{}, err
	}
	input.SiteName = strings.TrimSpace(input.SiteName)
	if input.SiteName == "" || len(input.SiteName) > 100 || !validAccentColors[input.AccentColor] {
		return SiteSettings{}, ErrInvalid
	}
	before, err := s.SiteSettings(ctx)
	if err != nil {
		return SiteSettings{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return SiteSettings{}, fmt.Errorf("begin update site settings: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE site_setting SET site_name=$1, accent_color=$2, updated_by=$3, updated_at=now() WHERE id`,
		input.SiteName, input.AccentColor, principal.UserID); err != nil {
		return SiteSettings{}, fmt.Errorf("update site settings: %w", err)
	}
	after, err := s.SiteSettings(ctx)
	if err != nil {
		return SiteSettings{}, err
	}
	if err = s.appendSiteSettingAudit(ctx, tx, principal, "site_setting.updated", before, after, sourceIP); err != nil {
		return SiteSettings{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return SiteSettings{}, fmt.Errorf("commit update site settings: %w", err)
	}
	return after, nil
}

// UploadSiteLogo stores an uploaded image under s.logoDir and points
// site_setting.logo_url at it, replacing (and deleting) any previous logo
// file. The stored URL is a relative API path -- the frontend prefixes it
// with the gateway origin, same as any other same-origin API response.
func (s *Service) UploadSiteLogo(ctx context.Context, principal auth.Principal, data []byte, sourceIP *netip.Addr) (SiteSettings, error) {
	if err := s.requireGlobalPermission(ctx, principal, "update", "site_setting"); err != nil {
		return SiteSettings{}, err
	}
	if len(data) == 0 || len(data) > maxLogoBytes {
		return SiteSettings{}, ErrInvalid
	}
	sniffLen := len(data)
	if sniffLen > 512 {
		sniffLen = 512
	}
	ext, ok := logoExtensions[http.DetectContentType(data[:sniffLen])]
	if !ok {
		return SiteSettings{}, ErrInvalid
	}
	before, err := s.SiteSettings(ctx)
	if err != nil {
		return SiteSettings{}, err
	}
	id, err := newUUID()
	if err != nil {
		return SiteSettings{}, err
	}
	if err = os.MkdirAll(s.logoDir, 0o755); err != nil {
		return SiteSettings{}, fmt.Errorf("create logo dir: %w", err)
	}
	filename := uuidString(id) + ext
	if err = os.WriteFile(filepath.Join(s.logoDir, filename), data, 0o644); err != nil {
		return SiteSettings{}, fmt.Errorf("store logo file: %w", err)
	}
	logoURL := "/api/v1/site-settings/logo/" + filename
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		_ = os.Remove(filepath.Join(s.logoDir, filename))
		return SiteSettings{}, fmt.Errorf("begin upload site logo: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE site_setting SET logo_url=$1, updated_by=$2, updated_at=now() WHERE id`, logoURL, principal.UserID); err != nil {
		_ = os.Remove(filepath.Join(s.logoDir, filename))
		return SiteSettings{}, fmt.Errorf("update site logo: %w", err)
	}
	after, err := s.SiteSettings(ctx)
	if err != nil {
		return SiteSettings{}, err
	}
	if err = s.appendSiteSettingAudit(ctx, tx, principal, "site_setting.logo_uploaded", before, after, sourceIP); err != nil {
		return SiteSettings{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return SiteSettings{}, fmt.Errorf("commit upload site logo: %w", err)
	}
	s.removeSiteLogoFile(before.LogoURL)
	return after, nil
}

func (s *Service) DeleteSiteLogo(ctx context.Context, principal auth.Principal, sourceIP *netip.Addr) (SiteSettings, error) {
	if err := s.requireGlobalPermission(ctx, principal, "update", "site_setting"); err != nil {
		return SiteSettings{}, err
	}
	before, err := s.SiteSettings(ctx)
	if err != nil {
		return SiteSettings{}, err
	}
	if before.LogoURL == nil {
		return before, nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return SiteSettings{}, fmt.Errorf("begin delete site logo: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE site_setting SET logo_url=NULL, updated_by=$1, updated_at=now() WHERE id`, principal.UserID); err != nil {
		return SiteSettings{}, fmt.Errorf("clear site logo: %w", err)
	}
	after, err := s.SiteSettings(ctx)
	if err != nil {
		return SiteSettings{}, err
	}
	if err = s.appendSiteSettingAudit(ctx, tx, principal, "site_setting.logo_removed", before, after, sourceIP); err != nil {
		return SiteSettings{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return SiteSettings{}, fmt.Errorf("commit delete site logo: %w", err)
	}
	s.removeSiteLogoFile(before.LogoURL)
	return after, nil
}

// SiteLogoFilePath resolves the on-disk path for a logo filename as served
// at GET /api/v1/site-settings/logo/{filename}. filepath.Base strips any
// directory components so the request can't escape s.logoDir.
func (s *Service) SiteLogoFilePath(filename string) string {
	return filepath.Join(s.logoDir, filepath.Base(filename))
}

func (s *Service) removeSiteLogoFile(logoURL *string) {
	if logoURL == nil {
		return
	}
	_ = os.Remove(s.SiteLogoFilePath(filepath.Base(*logoURL)))
}

func (s *Service) appendSiteSettingAudit(ctx context.Context, tx pgx.Tx, principal auth.Principal, action string, before, after SiteSettings, sourceIP *netip.Addr) error {
	correlationID, err := newUUID()
	if err != nil {
		return err
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if err = s.queries.WithTx(tx).CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: pgtype.UUID{}, ActorUserID: principal.UserID,
		Action: action, TargetType: "site_setting", TargetID: principal.UserID,
		BeforeData: beforeJSON, AfterData: afterJSON, SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return nil
}
