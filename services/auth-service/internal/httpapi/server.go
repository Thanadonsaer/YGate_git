package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"ygate/auth-service/internal/auth"
	"ygate/auth-service/internal/core"
)

const (
	sessionCookieName = "platform_session"
	csrfCookieName    = "platform_csrf"
)

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// New constructs auth-service's HTTP handler: session/credential management
// (login, logout, password reset/change, session listing/revocation) plus
// the admin CRUD surface that moved here with users/roles/api-keys/profile
// domain logic in a prior step. This mirrors platform-api's
// internal/httpapi/server.go's overall shape (New(...) building a mux,
// authenticated() as the session-check middleware) -- see that file's git
// history for the shared lineage. platform-api's own DELETE
// /api/v1/admin/api-keys/{keyId} hard-delete route stays in platform-api;
// the users hard-delete route moved here along with the rest of the users
// domain logic (see users.go/roles.go/admin_integrations.go/profile.go in
// this package).
func New(version string, ready func(context.Context) error, authService *auth.Service, registryService *core.Service, cookieSecure bool) http.Handler {
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
		mux.HandleFunc("GET /api/v1/auth/me", authenticated(authService, false, meHandler(authService.Permissions)))
		mux.HandleFunc("POST /api/v1/auth/logout", authenticated(authService, true, logoutHandler(authService, cookieSecure)))
		mux.HandleFunc("POST /api/v1/auth/logout-all", authenticated(authService, true, logoutAllHandler(authService, cookieSecure)))
		mux.HandleFunc("POST /api/v1/auth/change-password", authenticated(authService, true, changePasswordHandler(authService)))
		mux.HandleFunc("POST /api/v1/auth/forgot-password", forgotPasswordHandler(authService.RequestPasswordReset))
		mux.HandleFunc("POST /api/v1/auth/reset-password", resetPasswordHandler(authService.ResetPassword, cookieSecure))
		mux.HandleFunc("GET /api/v1/auth/sessions", authorized(authService, false, "session:read", sessionsHandler(authService)))
		mux.HandleFunc("DELETE /api/v1/auth/sessions", authorized(authService, true, "session:read", clearSessionsHandler(authService, cookieSecure)))
		mux.HandleFunc("DELETE /api/v1/auth/sessions/{sessionId}", authorized(authService, true, "session:read", revokeSessionHandler(authService, cookieSecure)))
		if registryService != nil {
			mux.HandleFunc("GET /api/v1/auth/profile", authenticated(authService, false, ownProfileHandler(registryService)))
			mux.HandleFunc("PUT /api/v1/auth/profile", authenticated(authService, true, updateOwnProfileHandler(registryService)))
			mux.HandleFunc("GET /api/v1/admin/openapi", authenticated(authService, false, openAPIHandler(registryService)))
			mux.HandleFunc("GET /api/v1/admin/api-keys", authenticated(authService, false, listAPIKeysHandler(registryService)))
			mux.HandleFunc("POST /api/v1/admin/api-keys", authenticated(authService, true, createAPIKeyHandler(registryService)))
			mux.HandleFunc("PUT /api/v1/admin/api-keys/{keyId}", authenticated(authService, true, updateAPIKeyHandler(registryService)))
			mux.HandleFunc("POST /api/v1/admin/api-keys/{keyId}/status", authenticated(authService, true, updateAPIKeyStatusHandler(registryService)))
			mux.HandleFunc("GET /api/v1/admin/users", authenticated(authService, false, listUsersHandler(registryService)))
			mux.HandleFunc("POST /api/v1/admin/users", authenticated(authService, true, createUserHandler(registryService)))
			mux.HandleFunc("PUT /api/v1/admin/users/{userId}", authenticated(authService, true, updateUserHandler(registryService)))
			mux.HandleFunc("DELETE /api/v1/admin/users/{userId}", authenticated(authService, true, hardDeleteUserHandler(registryService)))
			mux.HandleFunc("POST /api/v1/admin/users/{userId}/status", authenticated(authService, true, updateUserStatusHandler(registryService)))
			mux.HandleFunc("POST /api/v1/admin/users/{userId}/unlock", authenticated(authService, true, unlockUserHandler(registryService)))
			mux.HandleFunc("POST /api/v1/admin/users/{userId}/reset-password", authenticated(authService, true, resetUserPasswordHandler(registryService)))
			mux.HandleFunc("GET /api/v1/admin/roles", authenticated(authService, false, listRolesHandler(registryService)))
			mux.HandleFunc("GET /api/v1/admin/roles/{roleId}", authenticated(authService, false, getRoleHandler(registryService)))
			mux.HandleFunc("POST /api/v1/admin/roles", authenticated(authService, true, createRoleHandler(registryService)))
			mux.HandleFunc("PUT /api/v1/admin/roles/{roleId}", authenticated(authService, true, updateRoleHandler(registryService)))
			mux.HandleFunc("DELETE /api/v1/admin/roles/{roleId}", authenticated(authService, true, deleteRoleHandler(registryService)))
			mux.HandleFunc("GET /api/v1/admin/permissions", authenticated(authService, false, listPermissionsHandler(registryService)))
			mux.HandleFunc("GET /api/v1/admin/organizations", authenticated(authService, false, listOrganizationsHandler(registryService)))
			mux.HandleFunc("POST /api/v1/admin/organizations", authenticated(authService, true, createOrganizationHandler(registryService)))
			mux.HandleFunc("PUT /api/v1/admin/organizations/{organizationId}", authenticated(authService, true, updateOrganizationHandler(registryService)))
		}
	}
	mux.HandleFunc("POST /api/v1/auth/register", registerHandler(authService))
	mux.HandleFunc("POST /api/v1/auth/resend-verification", resendVerificationHandler(authService))
	mux.HandleFunc("GET /api/v1/auth/verify-email", verifyEmailHandler(authService))
	mux.HandleFunc("POST /api/v1/auth/login", loginHandler(login, cookieSecure))
	return mux
}

// authenticated is auth-service's own copy of the session-check middleware --
// unlike platform-api's (which now only needs read-only validation via
// internal/sessioncheck), auth-service owns session writes (idle-timeout
// touch) and the full auth.Service, so it authenticates directly against it.
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

func authorized(service *auth.Service, csrfRequired bool, required string, next func(http.ResponseWriter, *http.Request, auth.Principal)) http.HandlerFunc {
	return authenticated(service, csrfRequired, func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		grants, err := service.Permissions(r.Context(), principal.UserID)
		if err != nil {
			log.Printf("load permission %s failed: %v", required, err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		for _, grant := range grants {
			if grant == required {
				next(w, r, principal)
				return
			}
		}
		http.Error(w, "permission denied", http.StatusForbidden)
	})
}
