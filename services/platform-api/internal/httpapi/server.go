package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
	"ygate/platform-api/internal/ingestion"
)

const (
	sessionCookieName = "platform_session"
	csrfCookieName    = "platform_csrf"
)

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

type LoginFunc func(context.Context, auth.LoginInput) (auth.LoginResult, error)

type RequestPasswordResetFunc func(context.Context, string, *netip.Addr) error
type ResetPasswordFunc func(context.Context, string, string, *netip.Addr) error

func New(version string, ready func(context.Context) error, authService *auth.Service, registryService *core.Service, ingestionService *ingestion.Service, cookieSecure bool, allowedOrigins ...string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: version})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if ready == nil {
			http.Error(w, "database readiness is not configured", http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := ready(ctx); err != nil {
			http.Error(w, "database is unavailable", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, http.StatusOK, healthResponse{Status: "ready", Version: version})
	})
	var login LoginFunc
	if authService != nil {
		login = authService.Login
		if len(allowedOrigins) == 0 {
			allowedOrigins = []string{"localhost:8080", "127.0.0.1:8080"}
		}
		mux.HandleFunc("GET /api/v1/auth/me", authenticated(authService, false, func(w http.ResponseWriter, _ *http.Request, principal auth.Principal) {
			writeJSON(w, http.StatusOK, principal.User())
		}))
		mux.HandleFunc("POST /api/v1/auth/logout", authenticated(authService, true, func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
			if err := authService.Logout(r.Context(), principal, remoteIP(r.RemoteAddr)); err != nil {
				log.Printf("logout failed: %v", err)
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			clearAuthCookies(w, cookieSecure)
			w.WriteHeader(http.StatusNoContent)
		}))
		mux.HandleFunc("POST /api/v1/auth/logout-all", authenticated(authService, true, func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
			if err := authService.LogoutAll(r.Context(), principal, remoteIP(r.RemoteAddr)); err != nil {
				log.Printf("logout all failed: %v", err)
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			clearAuthCookies(w, cookieSecure)
			w.WriteHeader(http.StatusNoContent)
		}))
		mux.HandleFunc("POST /api/v1/auth/change-password", authenticated(authService, true, changePasswordHandler(authService)))
		mux.HandleFunc("POST /api/v1/auth/forgot-password", forgotPasswordHandler(authService.RequestPasswordReset))
		mux.HandleFunc("POST /api/v1/auth/reset-password", resetPasswordHandler(authService.ResetPassword, cookieSecure))
		mux.HandleFunc("GET /api/v1/auth/sessions", authenticated(authService, false, sessionsHandler(authService)))
		mux.HandleFunc("DELETE /api/v1/auth/sessions", authenticated(authService, true, clearSessionsHandler(authService, cookieSecure)))
		mux.HandleFunc("GET /api/v1/realtime", authenticated(authService, false, realtimeHandler(allowedOrigins, registryService, registryService)))
		mux.HandleFunc("DELETE /api/v1/auth/sessions/{sessionId}", authenticated(authService, true, revokeSessionHandler(authService, cookieSecure)))
		if registryService != nil {
			mux.HandleFunc("GET /api/v1/auth/profile", authenticated(authService, false, ownProfileHandler(registryService)))
			mux.HandleFunc("PUT /api/v1/auth/profile", authenticated(authService, true, updateOwnProfileHandler(registryService)))
			mux.HandleFunc("GET /api/v1/admin/openapi", authenticated(authService, false, openAPIHandler(registryService)))
			mux.HandleFunc("GET /api/v1/admin/audit", authenticated(authService, false, auditEventsHandler(registryService)))
			mux.HandleFunc("DELETE /api/v1/admin/audit", authenticated(authService, true, clearAuditEventsHandler(registryService)))
			mux.HandleFunc("GET /api/v1/admin/api-keys", authenticated(authService, false, listAPIKeysHandler(registryService)))
			mux.HandleFunc("POST /api/v1/admin/api-keys", authenticated(authService, true, createAPIKeyHandler(registryService)))
			mux.HandleFunc("PUT /api/v1/admin/api-keys/{keyId}", authenticated(authService, true, updateAPIKeyHandler(registryService)))
			mux.HandleFunc("DELETE /api/v1/admin/api-keys/{keyId}", authenticated(authService, true, hardDeleteAPIKeyHandler(registryService)))
			mux.HandleFunc("POST /api/v1/admin/api-keys/{keyId}/status", authenticated(authService, true, updateAPIKeyStatusHandler(registryService)))
			mux.HandleFunc("GET /api/v1/admin/users", authenticated(authService, false, listUsersHandler(registryService)))
			mux.HandleFunc("POST /api/v1/admin/users", authenticated(authService, true, createUserHandler(registryService)))
			mux.HandleFunc("PUT /api/v1/admin/users/{userId}", authenticated(authService, true, updateUserHandler(registryService)))
			mux.HandleFunc("DELETE /api/v1/admin/users/{userId}", authenticated(authService, true, hardDeleteUserHandler(registryService)))
			mux.HandleFunc("GET /api/v1/admin/roles", authenticated(authService, false, listRolesHandler(registryService)))
			mux.HandleFunc("GET /api/v1/admin/roles/{roleId}", authenticated(authService, false, getRoleHandler(registryService)))
			mux.HandleFunc("POST /api/v1/admin/roles", authenticated(authService, true, createRoleHandler(registryService)))
			mux.HandleFunc("PUT /api/v1/admin/roles/{roleId}", authenticated(authService, true, updateRoleHandler(registryService)))
			mux.HandleFunc("DELETE /api/v1/admin/roles/{roleId}", authenticated(authService, true, deleteRoleHandler(registryService)))
			mux.HandleFunc("GET /api/v1/admin/permissions", authenticated(authService, false, listPermissionsHandler(registryService)))
			mux.HandleFunc("POST /api/v1/admin/users/{userId}/status", authenticated(authService, true, updateUserStatusHandler(registryService)))
			mux.HandleFunc("POST /api/v1/admin/users/{userId}/unlock", authenticated(authService, true, unlockUserHandler(registryService)))
			mux.HandleFunc("POST /api/v1/admin/users/{userId}/reset-password", authenticated(authService, true, resetUserPasswordHandler(registryService)))
			mux.HandleFunc("GET /api/v1/plants", authenticated(authService, false, listPlantsHandler(registryService)))
			mux.HandleFunc("POST /api/v1/plants", authenticated(authService, true, createPlantHandler(registryService)))
			mux.HandleFunc("GET /api/v1/device-models", authenticated(authService, false, listDeviceModelsHandler(registryService)))
			mux.HandleFunc("POST /api/v1/device-models", authenticated(authService, true, createDeviceModelHandler(registryService)))
			mux.HandleFunc("PUT /api/v1/device-models/{modelId}", authenticated(authService, true, updateDeviceModelHandler(registryService)))
			mux.HandleFunc("DELETE /api/v1/device-models/{modelId}", authenticated(authService, true, hardDeleteDeviceModelHandler(registryService)))
			mux.HandleFunc("GET /api/v1/device-models/{modelId}/register-metadata", authenticated(authService, false, listModelRegisterMetadataHandler(registryService)))
			mux.HandleFunc("PUT /api/v1/device-models/{modelId}/register-metadata", authenticated(authService, true, updateModelRegisterMetadataHandler(registryService)))
			mux.HandleFunc("DELETE /api/v1/device-models/{modelId}/register-metadata/{addressKey}", authenticated(authService, true, deleteModelRegisterMetadataHandler(registryService)))
			mux.HandleFunc("GET /api/v1/plants/{plantId}", authenticated(authService, false, getPlantHandler(registryService)))
			mux.HandleFunc("PUT /api/v1/plants/{plantId}", authenticated(authService, true, updatePlantHandler(registryService)))
			mux.HandleFunc("DELETE /api/v1/plants/{plantId}", authenticated(authService, true, hardDeletePlantHandler(registryService)))
			mux.HandleFunc("GET /api/v1/plants/{plantId}/devices", authenticated(authService, false, listDevicesHandler(registryService)))
			mux.HandleFunc("POST /api/v1/plants/{plantId}/devices", authenticated(authService, true, createDeviceHandler(registryService)))
			mux.HandleFunc("PUT /api/v1/plants/{plantId}/devices/{deviceId}", authenticated(authService, true, updateDeviceHandler(registryService)))
			mux.HandleFunc("DELETE /api/v1/plants/{plantId}/devices/{deviceId}", authenticated(authService, true, hardDeleteDeviceHandler(registryService)))
			mux.HandleFunc("GET /api/v1/plants/{plantId}/device-register-metadata/{deviceId}", authenticated(authService, false, listRegisterMetadataHandler(registryService)))
			mux.HandleFunc("PUT /api/v1/plants/{plantId}/device-register-metadata/{deviceId}", authenticated(authService, true, updateRegisterMetadataHandler(registryService)))
			mux.HandleFunc("GET /api/v1/plants/{plantId}/telemetry/latest", authenticated(authService, false, latestTelemetryHandler(registryService)))
			mux.HandleFunc("GET /api/v1/plants/{plantId}/devices/{deviceId}/telemetry/history", authenticated(authService, false, telemetryHistoryHandler(registryService)))
			mux.HandleFunc("GET /api/v1/plants/{plantId}/alarms/rules", authenticated(authService, false, listAlarmRulesHandler(registryService)))
			mux.HandleFunc("POST /api/v1/plants/{plantId}/alarms/rules", authenticated(authService, true, createAlarmRuleHandler(registryService)))
			mux.HandleFunc("PUT /api/v1/plants/{plantId}/alarms/rules/{ruleId}", authenticated(authService, true, updateAlarmRuleHandler(registryService)))
			mux.HandleFunc("DELETE /api/v1/plants/{plantId}/alarms/rules/{ruleId}", authenticated(authService, true, deleteAlarmRuleHandler(registryService)))
			mux.HandleFunc("GET /api/v1/plants/{plantId}/alarms/events", authenticated(authService, false, listAlarmEventsHandler(registryService)))
			mux.HandleFunc("POST /api/v1/plants/{plantId}/alarms/events/{eventId}/ack", authenticated(authService, true, acknowledgeAlarmEventHandler(registryService)))
			mux.HandleFunc("GET /api/v1/dashboard/overview", authenticated(authService, false, dashboardOverviewHandler(registryService)))
			mux.HandleFunc("GET /api/v1/scada/screens", authenticated(authService, false, listScadaScreensHandler(registryService)))
			mux.HandleFunc("POST /api/v1/scada/screens", authenticated(authService, true, createScadaScreenHandler(registryService)))
			mux.HandleFunc("GET /api/v1/scada/screens/{screenId}", authenticated(authService, false, getScadaScreenHandler(registryService)))
			mux.HandleFunc("PUT /api/v1/scada/screens/{screenId}", authenticated(authService, true, saveScadaScreenHandler(registryService)))
			mux.HandleFunc("DELETE /api/v1/scada/screens/{screenId}", authenticated(authService, true, hardDeleteScadaScreenHandler(registryService)))
			mux.HandleFunc("GET /api/v1/scada/screens/{screenId}/published", authenticated(authService, false, getPublishedScadaScreenHandler(registryService)))
			mux.HandleFunc("GET /api/v1/scada/screens/{screenId}/versions", authenticated(authService, false, listScadaVersionsHandler(registryService)))
			mux.HandleFunc("POST /api/v1/scada/screens/{screenId}/publish", authenticated(authService, true, publishScadaScreenHandler(registryService)))
			mux.HandleFunc("POST /api/v1/scada/screens/{screenId}/rollback", authenticated(authService, true, rollbackScadaScreenHandler(registryService)))
			mux.HandleFunc("GET /api/v1/dashboard/layout", authenticated(authService, false, dashboardLayoutHandler(registryService)))
			mux.HandleFunc("PUT /api/v1/dashboard/layout", authenticated(authService, true, updateDashboardLayoutHandler(registryService)))
			mux.HandleFunc("GET /api/v1/dashboard/layout/published", authenticated(authService, false, publishedDashboardLayoutHandler(registryService)))
			mux.HandleFunc("POST /api/v1/dashboard/layout/publish", authenticated(authService, true, publishDashboardLayoutHandler(registryService)))
		}
	}
	if ingestionService != nil {
		mux.HandleFunc("POST /api/v1/ingestion/telemetry", ingestionHandler(ingestionService))
		mux.HandleFunc("POST /api/middleware/readings", ingestionHandler(ingestionService))
		mux.HandleFunc("POST /api/v2/ingestion/register-readings", rawIngestionHandler(ingestionService))
	}
	mux.HandleFunc("POST /api/v1/auth/login", loginHandler(login, cookieSecure))
	return mux
}

func loginHandler(login LoginFunc, cookieSecure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if login == nil {
			http.Error(w, "authentication is unavailable", http.StatusServiceUnavailable)
			return
		}
		var request struct {
			Identifier string `json:"identifier"`
			Password   string `json:"password"`
		}
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		if len(request.Identifier) > 320 || len(request.Password) > 1024 {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		userAgent := []rune(r.UserAgent())
		if len(userAgent) > 512 {
			userAgent = userAgent[:512]
		}
		result, err := login(r.Context(), auth.LoginInput{
			Identifier: request.Identifier,
			Password:   request.Password,
			SourceIP:   remoteIP(r.RemoteAddr),
			UserAgent:  string(userAgent),
		})
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		case errors.Is(err, auth.ErrRateLimited):
			w.Header().Set("Retry-After", "900")
			http.Error(w, "too many login attempts", http.StatusTooManyRequests)
			return
		case err != nil:
			log.Printf("login failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		setAuthCookies(w, result, cookieSecure)
		writeJSON(w, http.StatusOK, result)
	}
}

func forgotPasswordHandler(requestReset RequestPasswordResetFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var request struct {
			Email string `json:"email"`
		}
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		if strings.TrimSpace(request.Email) == "" || len(request.Email) > 320 {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if err := requestReset(r.Context(), request.Email, remoteIP(r.RemoteAddr)); err != nil {
			log.Printf("password reset request failed: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}
}

func resetPasswordHandler(reset ResetPasswordFunc, cookieSecure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var request struct {
			Token       string `json:"token"`
			NewPassword string `json:"newPassword"`
		}
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		err := reset(r.Context(), request.Token, request.NewPassword, remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, auth.ErrWeakPassword):
			http.Error(w, "new password must be 12 to 72 bytes", http.StatusBadRequest)
		case errors.Is(err, auth.ErrInvalidResetToken):
			http.Error(w, "invalid or expired reset token", http.StatusBadRequest)
		case errors.Is(err, auth.ErrRateLimited):
			w.Header().Set("Retry-After", "900")
			http.Error(w, "too many reset attempts", http.StatusTooManyRequests)
		case err != nil:
			log.Printf("password reset failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			clearAuthCookies(w, cookieSecure)
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func sessionsHandler(service *auth.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		sessions, err := service.Sessions(r.Context(), principal)
		if err != nil {
			log.Printf("list sessions failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, sessions)
	}
}

func revokeSessionHandler(service *auth.Service, cookieSecure bool) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		current, err := service.RevokeOwnSession(r.Context(), principal, r.PathValue("sessionId"), remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, auth.ErrSessionNotFound):
			http.Error(w, "session not found", http.StatusNotFound)
		case err != nil:
			log.Printf("revoke session failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			if current {
				clearAuthCookies(w, cookieSecure)
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func clearSessionsHandler(service *auth.Service, cookieSecure bool) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		err := service.ClearOwnSessions(r.Context(), principal, r.Header.Get("X-Hard-Delete-Confirm"), remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, auth.ErrSessionConfirmation):
			http.Error(w, "invalid confirmation", http.StatusBadRequest)
		case err != nil:
			log.Printf("clear sessions failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			clearAuthCookies(w, cookieSecure)
			w.WriteHeader(http.StatusNoContent)
		}
	}
}
func authenticated(service *auth.Service, csrfRequired bool, next func(http.ResponseWriter, *http.Request, auth.Principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		principal, err := service.Authenticate(r.Context(), cookie.Value)
		if errors.Is(err, auth.ErrUnauthenticated) {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if err != nil {
			log.Printf("authenticate failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if csrfRequired && !principal.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
			http.Error(w, "invalid CSRF token", http.StatusForbidden)
			return
		}
		next(w, r, principal)
	}
}

func changePasswordHandler(service *auth.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request struct {
			CurrentPassword string `json:"currentPassword"`
			NewPassword     string `json:"newPassword"`
		}
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		err := service.ChangePassword(r.Context(), principal, request.CurrentPassword, request.NewPassword, remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, auth.ErrInvalidCurrentPassword):
			http.Error(w, "current password is invalid", http.StatusBadRequest)
		case errors.Is(err, auth.ErrWeakPassword):
			http.Error(w, "new password must be 12 to 72 bytes", http.StatusBadRequest)
		case err != nil:
			log.Printf("change password failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, value any, maxBytes int64) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(value); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return false
	}
	if err = decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return false
	}
	return true
}

func setAuthCookies(w http.ResponseWriter, result auth.LoginResult, secure bool) {
	maxAge := max(1, int(time.Until(result.ExpiresAt).Seconds()))
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: result.Token, Path: "/", Expires: result.ExpiresAt,
		MaxAge: maxAge, HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name: csrfCookieName, Value: result.CSRFToken, Path: "/", Expires: result.ExpiresAt,
		MaxAge: maxAge, HttpOnly: false, Secure: secure, SameSite: http.SameSiteStrictMode,
	})
}

func clearAuthCookies(w http.ResponseWriter, secure bool) {
	for _, cookie := range []*http.Cookie{
		{Name: sessionCookieName, HttpOnly: true},
		{Name: csrfCookieName, HttpOnly: false},
	} {
		cookie.Path = "/"
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(1, 0)
		cookie.Secure = secure
		cookie.SameSite = http.SameSiteStrictMode
		http.SetCookie(w, cookie)
	}
}

func remoteIP(remoteAddr string) *netip.Addr {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return nil
	}
	return &address
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
