package httpapi

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
)

type updateSiteSettingsRequest struct {
	SiteName    string `json:"siteName"`
	AccentColor string `json:"accentColor"`
}

// siteSettingsHandler is intentionally unauthenticated: the login screen
// renders the configured logo/name before a session exists.
func siteSettingsHandler(service *core.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := service.SiteSettings(r.Context())
		if err != nil {
			log.Printf("read site settings failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	}
}

func updateSiteSettingsHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request updateSiteSettingsRequest
		if !decodeJSON(w, r, &request, 8<<10) {
			return
		}
		settings, err := service.UpdateSiteSettings(r.Context(), principal, core.UpdateSiteSettingsInput{
			SiteName: request.SiteName, AccentColor: request.AccentColor,
		}, remoteIP(r.RemoteAddr))
		writeSiteSettingsResult(w, settings, err)
	}
}

func uploadSiteLogoHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20+4<<10)
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			http.Error(w, "logo file exceeds 2 MiB", http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("logo")
		if err != nil {
			http.Error(w, "logo file is required", http.StatusBadRequest)
			return
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "failed to read upload", http.StatusBadRequest)
			return
		}
		settings, err := service.UploadSiteLogo(r.Context(), principal, data, remoteIP(r.RemoteAddr))
		writeSiteSettingsResult(w, settings, err)
	}
}

func deleteSiteLogoHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		settings, err := service.DeleteSiteLogo(r.Context(), principal, remoteIP(r.RemoteAddr))
		writeSiteSettingsResult(w, settings, err)
	}
}

func writeSiteSettingsResult(w http.ResponseWriter, settings core.SiteSettings, err error) {
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, settings)
	case errors.Is(err, core.ErrInvalid):
		http.Error(w, "invalid request", http.StatusBadRequest)
	case errors.Is(err, core.ErrForbidden):
		http.Error(w, "permission denied", http.StatusForbidden)
	default:
		log.Printf("site settings request failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// serveSiteLogoHandler is unauthenticated for the same reason
// siteSettingsHandler is: the login screen renders the logo pre-session.
func serveSiteLogoHandler(service *core.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := service.SiteLogoFilePath(r.PathValue("filename"))
		file, err := os.Open(path)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		defer file.Close()
		var modTime time.Time
		if info, err := file.Stat(); err == nil {
			modTime = info.ModTime()
		}
		w.Header().Set("Cache-Control", "public, max-age=300")
		http.ServeContent(w, r, r.PathValue("filename"), modTime, file)
	}
}
