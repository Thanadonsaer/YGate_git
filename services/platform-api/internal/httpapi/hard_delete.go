package httpapi

import (
	"errors"
	"log"
	"net/http"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
)

func hardDeletePlantHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return hardDeleteHandler("plant", func(r *http.Request, principal auth.Principal) error {
		return service.HardDeletePlant(r.Context(), principal, r.PathValue("plantId"), r.Header.Get("X-Hard-Delete-Confirm"), remoteIP(r.RemoteAddr))
	})
}

func hardDeleteDeviceHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return hardDeleteHandler("device", func(r *http.Request, principal auth.Principal) error {
		return service.HardDeleteDevice(r.Context(), principal, r.PathValue("plantId"), r.PathValue("deviceId"), r.Header.Get("X-Hard-Delete-Confirm"), remoteIP(r.RemoteAddr))
	})
}

func hardDeleteDeviceModelHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return hardDeleteHandler("device model", func(r *http.Request, principal auth.Principal) error {
		return service.HardDeleteDeviceModel(r.Context(), principal, r.PathValue("modelId"), r.Header.Get("X-Hard-Delete-Confirm"), remoteIP(r.RemoteAddr))
	})
}

func hardDeleteAPIKeyHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return hardDeleteHandler("api key", func(r *http.Request, principal auth.Principal) error {
		return service.HardDeleteAPIKey(r.Context(), principal, r.PathValue("keyId"), r.Header.Get("X-Hard-Delete-Confirm"), remoteIP(r.RemoteAddr))
	})
}

func hardDeleteHandler(resource string, remove func(*http.Request, auth.Principal) error) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		err := remove(r, principal)
		switch {
		case errors.Is(err, core.ErrInvalid), errors.Is(err, core.ErrAPIKeyInvalid):
			http.Error(w, "invalid confirmation", http.StatusBadRequest)
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case errors.Is(err, core.ErrNotFound), errors.Is(err, core.ErrAPIKeyNotFound):
			http.Error(w, resource+" not found", http.StatusNotFound)
		case err != nil:
			log.Printf("hard delete %s failed: %v", resource, err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}
