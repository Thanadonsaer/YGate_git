package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
)

type dashboardReader interface {
	DashboardOverview(context.Context, auth.Principal, time.Duration, time.Time) (core.DashboardOverview, error)
}

type dashboardLayoutService interface {
	DashboardLayout(context.Context, auth.Principal) (core.DashboardLayout, error)
	PublishedDashboardLayout(context.Context, auth.Principal) (core.PublishedDashboardLayout, error)
	SaveDashboardLayout(context.Context, auth.Principal, core.UpdateDashboardLayoutInput, *netip.Addr) (core.DashboardLayout, error)
	PublishDashboardLayout(context.Context, auth.Principal, core.PublishDashboardLayoutInput, *netip.Addr) (core.PublishedDashboardLayout, error)
}

func dashboardOverviewHandler(service dashboardReader) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		seconds := 300
		var err error
		if value := r.URL.Query().Get("staleAfterSeconds"); value != "" {
			seconds, err = strconv.Atoi(value)
		}
		if err != nil {
			http.Error(w, "invalid stale threshold", http.StatusBadRequest)
			return
		}
		overview, err := service.DashboardOverview(r.Context(), principal, time.Duration(seconds)*time.Second, time.Now())
		switch {
		case errors.Is(err, core.ErrInvalid):
			http.Error(w, "invalid stale threshold", http.StatusBadRequest)
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case err != nil:
			log.Printf("dashboard overview failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusOK, overview)
		}
	}
}

func publishedDashboardLayoutHandler(service dashboardLayoutService) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		layout, err := service.PublishedDashboardLayout(r.Context(), principal)
		switch {
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case err != nil:
			log.Printf("published dashboard layout failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusOK, layout)
		}
	}
}

func dashboardLayoutHandler(service dashboardLayoutService) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		layout, err := service.DashboardLayout(r.Context(), principal)
		switch {
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case err != nil:
			log.Printf("dashboard layout failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusOK, layout)
		}
	}
}

func publishDashboardLayoutHandler(service dashboardLayoutService) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var input core.PublishDashboardLayoutInput
		if !decodeJSON(w, r, &input, 16<<10) {
			return
		}
		layout, err := service.PublishDashboardLayout(r.Context(), principal, input, remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, core.ErrInvalid):
			http.Error(w, "invalid dashboard publish request", http.StatusBadRequest)
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case errors.Is(err, core.ErrDashboardVersionConflict):
			http.Error(w, "dashboard changed; reload and try again", http.StatusConflict)
		case err != nil:
			log.Printf("dashboard publish failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusOK, layout)
		}
	}
}

func updateDashboardLayoutHandler(service dashboardLayoutService) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var input core.UpdateDashboardLayoutInput
		if !decodeJSON(w, r, &input, 64<<10) {
			return
		}
		layout, err := service.SaveDashboardLayout(r.Context(), principal, input, remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, core.ErrInvalid):
			http.Error(w, "invalid dashboard layout", http.StatusBadRequest)
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case errors.Is(err, core.ErrDashboardVersionConflict):
			http.Error(w, "dashboard layout changed; reload and try again", http.StatusConflict)
		case err != nil:
			log.Printf("dashboard layout update failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusOK, layout)
		}
	}
}
