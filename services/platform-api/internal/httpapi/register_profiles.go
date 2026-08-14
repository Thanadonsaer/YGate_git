package httpapi

import (
	"errors"
	"log"
	"net/http"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
)

type registerProfileRequest struct {
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
	Manufacturer   string `json:"manufacturer"`
	Description    string `json:"description"`
}

type registerProfileAddressRequest struct {
	core.RegisterProfileAddressInput
}

func listRegisterProfilesHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		profiles, err := service.ListRegisterProfiles(r.Context(), principal)
		switch {
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case err != nil:
			log.Printf("list register profiles failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusOK, profiles)
		}
	}
}

func createRegisterProfileHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request registerProfileRequest
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		profile, err := service.CreateRegisterProfile(r.Context(), principal, core.CreateRegisterProfileInput{
			OrganizationID: request.OrganizationID, Name: request.Name, Manufacturer: request.Manufacturer, Description: request.Description,
		}, remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, core.ErrInvalid):
			http.Error(w, "invalid register profile", http.StatusBadRequest)
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case errors.Is(err, core.ErrConflict):
			http.Error(w, "register profile already exists", http.StatusConflict)
		case err != nil:
			log.Printf("create register profile failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusCreated, profile)
		}
	}
}

func listRegisterProfileAddressesHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		addresses, err := service.ListRegisterProfileAddresses(r.Context(), principal, r.PathValue("profileId"))
		switch {
		case errors.Is(err, core.ErrNotFound):
			http.Error(w, "register profile not found", http.StatusNotFound)
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case err != nil:
			log.Printf("list register profile addresses failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusOK, addresses)
		}
	}
}

func upsertRegisterProfileAddressHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request registerProfileAddressRequest
		if !decodeJSON(w, r, &request, 64<<10) {
			return
		}
		address, err := service.UpsertRegisterProfileAddress(r.Context(), principal, r.PathValue("profileId"), request.RegisterProfileAddressInput, remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, core.ErrInvalid):
			http.Error(w, "invalid register profile address or mapping", http.StatusBadRequest)
		case errors.Is(err, core.ErrNotFound):
			http.Error(w, "register profile not found", http.StatusNotFound)
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case err != nil:
			log.Printf("upsert register profile address failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusOK, address)
		}
	}
}

func assignRegisterProfileHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request struct {
			ProfileID string `json:"profileId"`
		}
		if !decodeJSON(w, r, &request, 8<<10) {
			return
		}
		err := service.AssignRegisterProfile(r.Context(), principal, r.PathValue("modelId"), request.ProfileID, remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, core.ErrNotFound):
			http.Error(w, "device model or profile not found", http.StatusNotFound)
		case errors.Is(err, core.ErrInvalid):
			http.Error(w, "profile belongs to another organization", http.StatusBadRequest)
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case err != nil:
			log.Printf("assign register profile failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}
