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

	"github.com/jackc/pgx/v5/pgxpool"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
	"ygate/platform-api/internal/gatewayhub"
	"ygate/platform-api/internal/ingestion"
	"ygate/platform-api/internal/sessioncheck"
)

const sessionCookieName = "platform_session"

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// New constructs platform-api's HTTP handler. Login/logout/password
// management/session listing and the users/roles/permissions/api-keys/
// profile admin CRUD moved to auth-service (see
// services/auth-service/internal/httpapi) -- this service now only
// validates existing sessions (via internal/sessioncheck, read-only) to
// authorize its own plants/devices/scada/alarms/dashboard/audit/middleware/
// telemetry routes and the remaining hard-delete endpoints that stayed here.
func New(version string, ready func(context.Context) error, pool *pgxpool.Pool, sessionIdleTimeout time.Duration, registryService *core.Service, ingestionService *ingestion.Service, hub *gatewayhub.Hub, allowedOrigins ...string) http.Handler {
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
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"localhost:8080", "127.0.0.1:8080"}
	}
	if registryService != nil {
		mux.HandleFunc("GET /api/v1/site-settings", siteSettingsHandler(registryService))
		mux.HandleFunc("PUT /api/v1/site-settings", authenticated(pool, sessionIdleTimeout, true, updateSiteSettingsHandler(registryService)))
		mux.HandleFunc("POST /api/v1/site-settings/logo", authenticated(pool, sessionIdleTimeout, true, uploadSiteLogoHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/site-settings/logo", authenticated(pool, sessionIdleTimeout, true, deleteSiteLogoHandler(registryService)))
		mux.HandleFunc("GET /api/v1/site-settings/logo/{filename}", serveSiteLogoHandler(registryService))
		mux.HandleFunc("GET /api/v1/realtime", authenticated(pool, sessionIdleTimeout, false, realtimeHandler(allowedOrigins, registryService, registryService)))
		mux.HandleFunc("GET /api/v1/admin/audit", authenticated(pool, sessionIdleTimeout, false, auditEventsHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/admin/audit", authenticated(pool, sessionIdleTimeout, true, clearAuditEventsHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/admin/api-keys/{keyId}", authenticated(pool, sessionIdleTimeout, true, hardDeleteAPIKeyHandler(registryService)))
		mux.HandleFunc("GET /api/v1/admin/middlewares", authenticated(pool, sessionIdleTimeout, false, listMiddlewaresHandler(registryService)))
		mux.HandleFunc("POST /api/v1/admin/middlewares", authenticated(pool, sessionIdleTimeout, true, createMiddlewareHandler(registryService)))
		mux.HandleFunc("PUT /api/v1/admin/middlewares/{middlewareId}", authenticated(pool, sessionIdleTimeout, true, updateMiddlewareHandler(registryService)))
		mux.HandleFunc("GET /api/v1/admin/middlewares/{middlewareId}/config", authenticated(pool, sessionIdleTimeout, false, getMiddlewareConfigHandler(registryService)))
		mux.HandleFunc("POST /api/v1/admin/middlewares/{middlewareId}/import-config", authenticated(pool, sessionIdleTimeout, true, importMiddlewareConfigHandler(registryService)))
		mux.HandleFunc("POST /api/v1/admin/middlewares/{middlewareId}/push-config", authenticated(pool, sessionIdleTimeout, true, pushMiddlewareConfigHandler(registryService)))
		mux.HandleFunc("GET /api/v1/admin/middlewares/{middlewareId}/plants", authenticated(pool, sessionIdleTimeout, false, listMiddlewarePlantsHandler(registryService)))
		mux.HandleFunc("POST /api/v1/admin/middlewares/{middlewareId}/plants", authenticated(pool, sessionIdleTimeout, true, assignMiddlewarePlantHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/admin/middlewares/{middlewareId}/plants/{plantId}", authenticated(pool, sessionIdleTimeout, true, unassignMiddlewarePlantHandler(registryService)))
		mux.HandleFunc("POST /api/v1/admin/middlewares/{middlewareId}/update/stage", authenticated(pool, sessionIdleTimeout, true, stageMiddlewareUpdateHandler(registryService)))
		mux.HandleFunc("POST /api/v1/admin/middlewares/{middlewareId}/update/apply", authenticated(pool, sessionIdleTimeout, true, applyMiddlewareUpdateHandler(registryService)))
		mux.HandleFunc("POST /api/v1/admin/middlewares/{middlewareId}/update/rollback", authenticated(pool, sessionIdleTimeout, true, rollbackMiddlewareUpdateHandler(registryService)))
		mux.HandleFunc("POST /api/v1/admin/middlewares/{middlewareId}/restart", authenticated(pool, sessionIdleTimeout, true, restartMiddlewareHandler(registryService)))
		mux.HandleFunc("GET /api/v1/admin/middleware-patches", authenticated(pool, sessionIdleTimeout, false, listMiddlewarePatchesHandler(registryService)))
		mux.HandleFunc("POST /api/v1/admin/middleware-patches", authenticated(pool, sessionIdleTimeout, true, uploadMiddlewarePatchHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/admin/middleware-patches/{patchId}", authenticated(pool, sessionIdleTimeout, true, deleteMiddlewarePatchHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants", authenticated(pool, sessionIdleTimeout, false, listPlantsHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants/export-all", authenticated(pool, sessionIdleTimeout, false, exportPlantDeviceCSVHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants/import-template", authenticated(pool, sessionIdleTimeout, false, plantDeviceCSVTemplateHandler()))
		mux.HandleFunc("POST /api/v1/plants/import", authenticated(pool, sessionIdleTimeout, true, importPlantDeviceCSVHandler(registryService)))
		mux.HandleFunc("POST /api/v1/plants", authenticated(pool, sessionIdleTimeout, true, createPlantHandler(registryService)))
		mux.HandleFunc("GET /api/v1/device-models", authenticated(pool, sessionIdleTimeout, false, listDeviceModelsHandler(registryService)))
		mux.HandleFunc("POST /api/v1/device-models", authenticated(pool, sessionIdleTimeout, true, createDeviceModelHandler(registryService)))
		mux.HandleFunc("PUT /api/v1/device-models/{modelId}", authenticated(pool, sessionIdleTimeout, true, updateDeviceModelHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/device-models/{modelId}", authenticated(pool, sessionIdleTimeout, true, hardDeleteDeviceModelHandler(registryService)))
		mux.HandleFunc("GET /api/v1/device-models/{modelId}/register-metadata", authenticated(pool, sessionIdleTimeout, false, listModelRegisterMetadataHandler(registryService)))
		mux.HandleFunc("PUT /api/v1/device-models/{modelId}/register-metadata", authenticated(pool, sessionIdleTimeout, true, updateModelRegisterMetadataHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/device-models/{modelId}/register-metadata/{addressKey}", authenticated(pool, sessionIdleTimeout, true, deleteModelRegisterMetadataHandler(registryService)))
		mux.HandleFunc("GET /api/v1/device-models/register-metadata/export-all", authenticated(pool, sessionIdleTimeout, false, exportRegisterMetadataCSVHandler(registryService)))
		mux.HandleFunc("GET /api/v1/device-models/register-metadata/import-template", authenticated(pool, sessionIdleTimeout, false, registerMetadataCSVTemplateHandler()))
		mux.HandleFunc("POST /api/v1/device-models/register-metadata/import", authenticated(pool, sessionIdleTimeout, true, importRegisterMetadataCSVHandler(registryService)))
		mux.HandleFunc("GET /api/v1/device-models/{modelId}/register-metadata/export", authenticated(pool, sessionIdleTimeout, false, exportModelRegisterMetadataCSVHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants/{plantId}", authenticated(pool, sessionIdleTimeout, false, getPlantHandler(registryService)))
		mux.HandleFunc("PUT /api/v1/plants/{plantId}", authenticated(pool, sessionIdleTimeout, true, updatePlantHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/plants/{plantId}", authenticated(pool, sessionIdleTimeout, true, hardDeletePlantHandler(registryService)))
		mux.HandleFunc("POST /api/v1/plants/{plantId}/image", authenticated(pool, sessionIdleTimeout, true, uploadPlantImageHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/plants/{plantId}/image", authenticated(pool, sessionIdleTimeout, true, deletePlantImageHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants/{plantId}/image/{filename}", authenticated(pool, sessionIdleTimeout, false, servePlantImageHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants/{plantId}/export", authenticated(pool, sessionIdleTimeout, false, exportOnePlantDeviceCSVHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants/{plantId}/devices", authenticated(pool, sessionIdleTimeout, false, listDevicesHandler(registryService)))
		mux.HandleFunc("POST /api/v1/plants/{plantId}/devices", authenticated(pool, sessionIdleTimeout, true, createDeviceHandler(registryService)))
		mux.HandleFunc("PUT /api/v1/plants/{plantId}/devices/{deviceId}", authenticated(pool, sessionIdleTimeout, true, updateDeviceHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/plants/{plantId}/devices/{deviceId}", authenticated(pool, sessionIdleTimeout, true, hardDeleteDeviceHandler(registryService)))
		mux.HandleFunc("POST /api/v1/plants/{plantId}/devices/{deviceId}/test-connection", authenticated(pool, sessionIdleTimeout, true, deviceCommandHandler(registryService, "connectTest")))
		mux.HandleFunc("POST /api/v1/plants/{plantId}/devices/{deviceId}/test-read", authenticated(pool, sessionIdleTimeout, true, deviceCommandHandler(registryService, "readNow")))
		mux.HandleFunc("GET /api/v1/plants/{plantId}/device-register-metadata/{deviceId}", authenticated(pool, sessionIdleTimeout, false, listRegisterMetadataHandler(registryService)))
		mux.HandleFunc("PUT /api/v1/plants/{plantId}/device-register-metadata/{deviceId}", authenticated(pool, sessionIdleTimeout, true, updateRegisterMetadataHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants/{plantId}/telemetry/latest", authenticated(pool, sessionIdleTimeout, false, latestTelemetryHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants/{plantId}/devices/{deviceId}/telemetry/history", authenticated(pool, sessionIdleTimeout, false, telemetryHistoryHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants/{plantId}/alarms/rules", authenticated(pool, sessionIdleTimeout, false, listAlarmRulesHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants/{plantId}/alarms/notify-roles", authenticated(pool, sessionIdleTimeout, false, listAlarmNotifyRolesHandler(registryService)))
		mux.HandleFunc("POST /api/v1/plants/{plantId}/alarms/rules", authenticated(pool, sessionIdleTimeout, true, createAlarmRuleHandler(registryService)))
		mux.HandleFunc("PUT /api/v1/plants/{plantId}/alarms/rules/{ruleId}", authenticated(pool, sessionIdleTimeout, true, updateAlarmRuleHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/plants/{plantId}/alarms/rules/{ruleId}", authenticated(pool, sessionIdleTimeout, true, deleteAlarmRuleHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants/{plantId}/alarms/events", authenticated(pool, sessionIdleTimeout, false, listAlarmEventsHandler(registryService)))
		mux.HandleFunc("POST /api/v1/plants/{plantId}/alarms/events/{eventId}/ack", authenticated(pool, sessionIdleTimeout, true, acknowledgeAlarmEventHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants/{plantId}/alarms/logbook", authenticated(pool, sessionIdleTimeout, false, listEventLogbookHandler(registryService)))
		mux.HandleFunc("POST /api/v1/plants/{plantId}/alarms/logbook", authenticated(pool, sessionIdleTimeout, true, createEventLogbookHandler(registryService)))
		mux.HandleFunc("GET /api/v1/dashboard/overview", authenticated(pool, sessionIdleTimeout, false, dashboardOverviewHandler(registryService)))
		mux.HandleFunc("POST /api/v1/reports/export", authenticated(pool, sessionIdleTimeout, false, exportReportHandler(registryService)))
		mux.HandleFunc("GET /api/v1/scada/screens", authenticated(pool, sessionIdleTimeout, false, listScadaScreensHandler(registryService)))
		mux.HandleFunc("POST /api/v1/scada/screens", authenticated(pool, sessionIdleTimeout, true, createScadaScreenHandler(registryService)))
		mux.HandleFunc("GET /api/v1/scada/screens/{screenId}", authenticated(pool, sessionIdleTimeout, false, getScadaScreenHandler(registryService)))
		mux.HandleFunc("PUT /api/v1/scada/screens/{screenId}", authenticated(pool, sessionIdleTimeout, true, saveScadaScreenHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/scada/screens/{screenId}", authenticated(pool, sessionIdleTimeout, true, hardDeleteScadaScreenHandler(registryService)))
		mux.HandleFunc("GET /api/v1/scada/screens/{screenId}/published", authenticated(pool, sessionIdleTimeout, false, getPublishedScadaScreenHandler(registryService)))
		mux.HandleFunc("GET /api/v1/scada/screens/{screenId}/versions", authenticated(pool, sessionIdleTimeout, false, listScadaVersionsHandler(registryService)))
		mux.HandleFunc("POST /api/v1/scada/screens/{screenId}/publish", authenticated(pool, sessionIdleTimeout, true, publishScadaScreenHandler(registryService)))
		mux.HandleFunc("POST /api/v1/scada/screens/{screenId}/rollback", authenticated(pool, sessionIdleTimeout, true, rollbackScadaScreenHandler(registryService)))
		mux.HandleFunc("GET /api/v1/dashboard/layout", authenticated(pool, sessionIdleTimeout, false, dashboardLayoutHandler(registryService)))
		mux.HandleFunc("PUT /api/v1/dashboard/layout", authenticated(pool, sessionIdleTimeout, true, updateDashboardLayoutHandler(registryService)))
		mux.HandleFunc("GET /api/v1/dashboard/layout/published", authenticated(pool, sessionIdleTimeout, false, publishedDashboardLayoutHandler(registryService)))
		mux.HandleFunc("POST /api/v1/dashboard/layout/publish", authenticated(pool, sessionIdleTimeout, true, publishDashboardLayoutHandler(registryService)))
	}
	if ingestionService != nil {
		mux.HandleFunc("POST /api/v2/ingestion/register-readings", rawIngestionHandler(ingestionService))
		if registryService != nil {
			mux.HandleFunc("GET /api/v1/gateway/realtime", gatewayRealtimeHandler(ingestionService, registryService, hub))
			mux.HandleFunc("GET /api/v1/admin/middleware-patches/{patchId}/download", downloadMiddlewarePatchHandler(ingestionService, registryService))
		}
	}
	return mux
}

// authenticated now validates sessions via internal/sessioncheck instead of
// the full internal/auth.Service (which moved to auth-service -- see
// docs/superpowers/plans/2026-08-01-backend-microservices-phase1-auth-service.md).
// It still hands every downstream handler the same auth.Principal shape
// those handlers (and core.Service's methods) have always taken: only the
// exported fields sessioncheck.Principal mirrors are populated, which is
// exactly what plants/devices/scada/... handlers and core.Service ever read
// (permission checks and audit actor IDs, never the password hash). It also
// extends the session's idle-expiry via sessioncheck.Authenticate, matching
// auth.Service.Authenticate's old TouchSession behavior so actively-used
// sessions don't idle-timeout out from under a user.
func authenticated(pool *pgxpool.Pool, sessionIdleTimeout time.Duration, csrfRequired bool, next func(http.ResponseWriter, *http.Request, auth.Principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		principal, err := sessioncheck.Authenticate(r.Context(), pool, cookie.Value, sessionIdleTimeout)
		if errors.Is(err, sessioncheck.ErrUnauthenticated) {
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
		next(w, r, auth.Principal{
			SessionID:      principal.SessionID,
			OrganizationID: principal.OrganizationID,
			UserID:         principal.UserID,
			Email:          principal.Email,
			DisplayName:    principal.DisplayName,
		})
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
