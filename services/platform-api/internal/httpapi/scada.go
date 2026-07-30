package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/netip"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
)

type scadaService interface {
	ScadaScreens(context.Context, auth.Principal, string) ([]core.ScadaScreenSummary, error)
	CreateScadaScreen(context.Context, auth.Principal, core.CreateScadaScreenInput, *netip.Addr) (core.ScadaScreen, error)
	ScadaScreen(context.Context, auth.Principal, string) (core.ScadaScreen, error)
	SaveScadaScreen(context.Context, auth.Principal, string, core.UpdateScadaScreenInput, *netip.Addr) (core.ScadaScreen, error)
	PublishedScadaScreen(context.Context, auth.Principal, string) (core.PublishedScadaScreen, error)
	ScadaScreenVersions(context.Context, auth.Principal, string) ([]core.ScadaScreenVersion, error)
	PublishScadaScreen(context.Context, auth.Principal, string, core.PublishScadaScreenInput, *netip.Addr) (core.PublishedScadaScreen, error)
	RollbackScadaScreen(context.Context, auth.Principal, string, core.RollbackScadaScreenInput, *netip.Addr) (core.PublishedScadaScreen, error)
	HardDeleteScadaScreen(context.Context, auth.Principal, string, string, *netip.Addr) error
}

func listScadaScreensHandler(service scadaService) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		screens, err := service.ScadaScreens(r.Context(), principal, r.URL.Query().Get("plantId"))
		if writeScadaError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, screens)
	}
}

func createScadaScreenHandler(service scadaService) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var input core.CreateScadaScreenInput
		if !decodeJSON(w, r, &input, 16<<10) {
			return
		}
		screen, err := service.CreateScadaScreen(r.Context(), principal, input, remoteIP(r.RemoteAddr))
		if writeScadaError(w, err) {
			return
		}
		writeJSON(w, http.StatusCreated, screen)
	}
}

func getScadaScreenHandler(service scadaService) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		screen, err := service.ScadaScreen(r.Context(), principal, r.PathValue("screenId"))
		if writeScadaError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, screen)
	}
}

func saveScadaScreenHandler(service scadaService) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var input core.UpdateScadaScreenInput
		if !decodeJSON(w, r, &input, 512<<10) {
			return
		}
		screen, err := service.SaveScadaScreen(r.Context(), principal, r.PathValue("screenId"), input, remoteIP(r.RemoteAddr))
		if writeScadaError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, screen)
	}
}

func getPublishedScadaScreenHandler(service scadaService) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		screen, err := service.PublishedScadaScreen(r.Context(), principal, r.PathValue("screenId"))
		if writeScadaError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, screen)
	}
}

func listScadaVersionsHandler(service scadaService) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		versions, err := service.ScadaScreenVersions(r.Context(), principal, r.PathValue("screenId"))
		if writeScadaError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, versions)
	}
}

func publishScadaScreenHandler(service scadaService) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var input core.PublishScadaScreenInput
		if !decodeJSON(w, r, &input, 16<<10) {
			return
		}
		screen, err := service.PublishScadaScreen(r.Context(), principal, r.PathValue("screenId"), input, remoteIP(r.RemoteAddr))
		if writeScadaError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, screen)
	}
}

func rollbackScadaScreenHandler(service scadaService) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var input core.RollbackScadaScreenInput
		if !decodeJSON(w, r, &input, 16<<10) {
			return
		}
		screen, err := service.RollbackScadaScreen(r.Context(), principal, r.PathValue("screenId"), input, remoteIP(r.RemoteAddr))
		if writeScadaError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, screen)
	}
}

func hardDeleteScadaScreenHandler(service scadaService) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		err := service.HardDeleteScadaScreen(r.Context(), principal, r.PathValue("screenId"), r.Header.Get("X-Hard-Delete-Confirm"), remoteIP(r.RemoteAddr))
		if writeScadaError(w, err) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeScadaError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, core.ErrInvalid):
		http.Error(w, "invalid scada data", http.StatusBadRequest)
	case errors.Is(err, core.ErrForbidden):
		http.Error(w, "permission denied", http.StatusForbidden)
	case errors.Is(err, core.ErrScadaNotFound):
		http.Error(w, "scada screen not found", http.StatusNotFound)
	case errors.Is(err, core.ErrScadaConflict), errors.Is(err, core.ErrScadaVersionConflict):
		http.Error(w, "scada screen changed or already exists", http.StatusConflict)
	default:
		log.Printf("scada request failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
	return true
}
