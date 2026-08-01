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
func New(version string, ready func(context.Context) error, pool *pgxpool.Pool, registryService *core.Service, ingestionService *ingestion.Service, hub *gatewayhub.Hub, allowedOrigins ...string) http.Handler {
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
		mux.HandleFunc("GET /api/v1/realtime", authenticated(pool, false, realtimeHandler(allowedOrigins, registryService, registryService)))
		mux.HandleFunc("GET /api/v1/admin/audit", authenticated(pool, false, auditEventsHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/admin/audit", authenticated(pool, true, clearAuditEventsHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/admin/api-keys/{keyId}", authenticated(pool, true, hardDeleteAPIKeyHandler(registryService)))
		mux.HandleFunc("GET /api/v1/admin/middlewares", authenticated(pool, false, listMiddlewaresHandler(registryService)))
		mux.HandleFunc("POST /api/v1/admin/middlewares", authenticated(pool, true, createMiddlewareHandler(registryService)))
		mux.HandleFunc("PUT /api/v1/admin/middlewares/{middlewareId}", authenticated(pool, true, updateMiddlewareHandler(registryService)))
		mux.HandleFunc("GET /api/v1/admin/middlewares/{middlewareId}/config", authenticated(pool, false, getMiddlewareConfigHandler(registryService)))
		mux.HandleFunc("POST /api/v1/admin/middlewares/{middlewareId}/import-config", authenticated(pool, true, importMiddlewareConfigHandler(registryService)))
		mux.HandleFunc("GET /api/v1/admin/middlewares/{middlewareId}/plants", authenticated(pool, false, listMiddlewarePlantsHandler(registryService)))
		mux.HandleFunc("POST /api/v1/admin/middlewares/{middlewareId}/plants", authenticated(pool, true, assignMiddlewarePlantHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/admin/middlewares/{middlewareId}/plants/{plantId}", authenticated(pool, true, unassignMiddlewarePlantHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants", authenticated(pool, false, listPlantsHandler(registryService)))
		mux.HandleFunc("POST /api/v1/plants", authenticated(pool, true, createPlantHandler(registryService)))
		mux.HandleFunc("GET /api/v1/device-models", authenticated(pool, false, listDeviceModelsHandler(registryService)))
		mux.HandleFunc("POST /api/v1/device-models", authenticated(pool, true, createDeviceModelHandler(registryService)))
		mux.HandleFunc("PUT /api/v1/device-models/{modelId}", authenticated(pool, true, updateDeviceModelHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/device-models/{modelId}", authenticated(pool, true, hardDeleteDeviceModelHandler(registryService)))
		mux.HandleFunc("GET /api/v1/device-models/{modelId}/register-metadata", authenticated(pool, false, listModelRegisterMetadataHandler(registryService)))
		mux.HandleFunc("PUT /api/v1/device-models/{modelId}/register-metadata", authenticated(pool, true, updateModelRegisterMetadataHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/device-models/{modelId}/register-metadata/{addressKey}", authenticated(pool, true, deleteModelRegisterMetadataHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants/{plantId}", authenticated(pool, false, getPlantHandler(registryService)))
		mux.HandleFunc("PUT /api/v1/plants/{plantId}", authenticated(pool, true, updatePlantHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/plants/{plantId}", authenticated(pool, true, hardDeletePlantHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants/{plantId}/devices", authenticated(pool, false, listDevicesHandler(registryService)))
		mux.HandleFunc("POST /api/v1/plants/{plantId}/devices", authenticated(pool, true, createDeviceHandler(registryService)))
		mux.HandleFunc("PUT /api/v1/plants/{plantId}/devices/{deviceId}", authenticated(pool, true, updateDeviceHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/plants/{plantId}/devices/{deviceId}", authenticated(pool, true, hardDeleteDeviceHandler(registryService)))
		mux.HandleFunc("POST /api/v1/plants/{plantId}/devices/{deviceId}/test-connection", authenticated(pool, true, deviceCommandHandler(registryService, "connectTest")))
		mux.HandleFunc("POST /api/v1/plants/{plantId}/devices/{deviceId}/test-read", authenticated(pool, true, deviceCommandHandler(registryService, "readNow")))
		mux.HandleFunc("GET /api/v1/plants/{plantId}/device-register-metadata/{deviceId}", authenticated(pool, false, listRegisterMetadataHandler(registryService)))
		mux.HandleFunc("PUT /api/v1/plants/{plantId}/device-register-metadata/{deviceId}", authenticated(pool, true, updateRegisterMetadataHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants/{plantId}/telemetry/latest", authenticated(pool, false, latestTelemetryHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants/{plantId}/devices/{deviceId}/telemetry/history", authenticated(pool, false, telemetryHistoryHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants/{plantId}/alarms/rules", authenticated(pool, false, listAlarmRulesHandler(registryService)))
		mux.HandleFunc("POST /api/v1/plants/{plantId}/alarms/rules", authenticated(pool, true, createAlarmRuleHandler(registryService)))
		mux.HandleFunc("PUT /api/v1/plants/{plantId}/alarms/rules/{ruleId}", authenticated(pool, true, updateAlarmRuleHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/plants/{plantId}/alarms/rules/{ruleId}", authenticated(pool, true, deleteAlarmRuleHandler(registryService)))
		mux.HandleFunc("GET /api/v1/plants/{plantId}/alarms/events", authenticated(pool, false, listAlarmEventsHandler(registryService)))
		mux.HandleFunc("POST /api/v1/plants/{plantId}/alarms/events/{eventId}/ack", authenticated(pool, true, acknowledgeAlarmEventHandler(registryService)))
		mux.HandleFunc("GET /api/v1/dashboard/overview", authenticated(pool, false, dashboardOverviewHandler(registryService)))
		mux.HandleFunc("GET /api/v1/scada/screens", authenticated(pool, false, listScadaScreensHandler(registryService)))
		mux.HandleFunc("POST /api/v1/scada/screens", authenticated(pool, true, createScadaScreenHandler(registryService)))
		mux.HandleFunc("GET /api/v1/scada/screens/{screenId}", authenticated(pool, false, getScadaScreenHandler(registryService)))
		mux.HandleFunc("PUT /api/v1/scada/screens/{screenId}", authenticated(pool, true, saveScadaScreenHandler(registryService)))
		mux.HandleFunc("DELETE /api/v1/scada/screens/{screenId}", authenticated(pool, true, hardDeleteScadaScreenHandler(registryService)))
		mux.HandleFunc("GET /api/v1/scada/screens/{screenId}/published", authenticated(pool, false, getPublishedScadaScreenHandler(registryService)))
		mux.HandleFunc("GET /api/v1/scada/screens/{screenId}/versions", authenticated(pool, false, listScadaVersionsHandler(registryService)))
		mux.HandleFunc("POST /api/v1/scada/screens/{screenId}/publish", authenticated(pool, true, publishScadaScreenHandler(registryService)))
		mux.HandleFunc("POST /api/v1/scada/screens/{screenId}/rollback", authenticated(pool, true, rollbackScadaScreenHandler(registryService)))
		mux.HandleFunc("GET /api/v1/dashboard/layout", authenticated(pool, false, dashboardLayoutHandler(registryService)))
		mux.HandleFunc("PUT /api/v1/dashboard/layout", authenticated(pool, true, updateDashboardLayoutHandler(registryService)))
		mux.HandleFunc("GET /api/v1/dashboard/layout/published", authenticated(pool, false, publishedDashboardLayoutHandler(registryService)))
		mux.HandleFunc("POST /api/v1/dashboard/layout/publish", authenticated(pool, true, publishDashboardLayoutHandler(registryService)))
	}
	if ingestionService != nil {
		mux.HandleFunc("POST /api/v1/ingestion/telemetry", ingestionHandler(ingestionService))
		mux.HandleFunc("POST /api/middleware/readings", ingestionHandler(ingestionService))
		mux.HandleFunc("POST /api/v2/ingestion/register-readings", rawIngestionHandler(ingestionService))
		if registryService != nil {
			mux.HandleFunc("GET /api/v1/gateway/realtime", gatewayRealtimeHandler(ingestionService, registryService, hub))
		}
	}
	return mux
}

// authenticated now validates sessions via internal/sessioncheck's read-only
// lookup instead of the full internal/auth.Service (which moved to
// auth-service -- see docs/superpowers/plans/2026-08-01-backend-microservices-phase1-auth-service.md).
// It still hands every downstream handler the same auth.Principal shape
// those handlers (and core.Service's methods) have always taken: only the
// exported fields sessioncheck.Principal mirrors are populated, which is
// exactly what plants/devices/scada/... handlers and core.Service ever read
// (permission checks and audit actor IDs, never the password hash).
func authenticated(pool *pgxpool.Pool, csrfRequired bool, next func(http.ResponseWriter, *http.Request, auth.Principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		principal, err := sessioncheck.Authenticate(r.Context(), pool, cookie.Value)
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
