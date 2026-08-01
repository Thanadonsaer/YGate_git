package httpapi

import (
	"errors"
	"log"
	"net/http"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
)

type createMiddlewareRequest struct {
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
	SiteName       string `json:"siteName"`
}

type assignMiddlewarePlantRequest struct {
	PlantID string `json:"plantId"`
}

type updateMiddlewareRequest struct {
	Name     string `json:"name"`
	SiteName string `json:"siteName"`
	IsActive *bool  `json:"isActive"`
}

func listMiddlewaresHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		gateways, err := service.Middlewares(r.Context(), principal)
		if writeMiddlewareError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, gateways)
	}
}

func createMiddlewareHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request createMiddlewareRequest
		if !decodeJSON(w, r, &request, 32<<10) {
			return
		}
		gateway, err := service.CreateMiddleware(r.Context(), principal, core.CreateMiddlewareInput{
			OrganizationID: request.OrganizationID, Name: request.Name, SiteName: request.SiteName,
		}, remoteIP(r.RemoteAddr))
		if writeMiddlewareError(w, err) {
			return
		}
		writeJSON(w, http.StatusCreated, gateway)
	}
}

func updateMiddlewareHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request updateMiddlewareRequest
		if !decodeJSON(w, r, &request, 32<<10) {
			return
		}
		if request.IsActive == nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		gateway, err := service.UpdateMiddleware(r.Context(), principal, r.PathValue("middlewareId"), core.UpdateMiddlewareInput{
			Name: request.Name, SiteName: request.SiteName, IsActive: *request.IsActive,
		}, remoteIP(r.RemoteAddr))
		if writeMiddlewareError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, gateway)
	}
}

func getMiddlewareConfigHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		snapshot, err := service.MiddlewareConfig(r.Context(), principal, r.PathValue("middlewareId"))
		if writeMiddlewareError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
	}
}

func importMiddlewareConfigHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		result, err := service.ImportMiddlewareConfig(r.Context(), principal, r.PathValue("middlewareId"), remoteIP(r.RemoteAddr))
		if writeMiddlewareError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func pushMiddlewareConfigHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		err := service.PushMiddlewareConfig(r.Context(), principal, r.PathValue("middlewareId"), remoteIP(r.RemoteAddr))
		if writeMiddlewareError(w, err) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func listMiddlewarePlantsHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		plants, err := service.MiddlewarePlants(r.Context(), principal, r.PathValue("middlewareId"))
		if writeMiddlewareError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, plants)
	}
}

func assignMiddlewarePlantHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request assignMiddlewarePlantRequest
		if !decodeJSON(w, r, &request, 4<<10) {
			return
		}
		err := service.AssignMiddlewarePlant(r.Context(), principal, r.PathValue("middlewareId"), request.PlantID, remoteIP(r.RemoteAddr))
		if writeMiddlewareError(w, err) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func unassignMiddlewarePlantHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		err := service.UnassignMiddlewarePlant(r.Context(), principal, r.PathValue("middlewareId"), r.PathValue("plantId"), remoteIP(r.RemoteAddr))
		if writeMiddlewareError(w, err) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeMiddlewareError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, core.ErrMiddlewareInvalid), errors.Is(err, core.ErrInvalid):
		http.Error(w, "invalid request data", http.StatusBadRequest)
	case errors.Is(err, core.ErrForbidden):
		http.Error(w, "permission denied", http.StatusForbidden)
	case errors.Is(err, core.ErrMiddlewareNotFound), errors.Is(err, core.ErrNotFound), errors.Is(err, core.ErrMiddlewarePatchNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, core.ErrMiddlewareConflict):
		http.Error(w, "middleware gateway already exists", http.StatusConflict)
	case errors.Is(err, core.ErrMiddlewareOffline):
		http.Error(w, "middleware gateway is offline", http.StatusServiceUnavailable)
	case errors.Is(err, core.ErrMiddlewareCommandNAK):
		http.Error(w, "middleware gateway did not respond in time", http.StatusGatewayTimeout)
	default:
		log.Printf("middleware write failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
	return true
}
