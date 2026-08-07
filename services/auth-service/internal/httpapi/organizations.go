package httpapi

import (
	"errors"
	"log"
	"net/http"

	"ygate/auth-service/internal/auth"
	"ygate/auth-service/internal/core"
)

type saveOrganizationRequest struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	IsActive bool   `json:"isActive"`
}

func listOrganizationsHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		organizations, err := service.Organizations(r.Context(), principal)
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, organizations)
	}
}

func createOrganizationHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request saveOrganizationRequest
		if !decodeJSON(w, r, &request, 32<<10) {
			return
		}
		organization, err := service.CreateOrganization(r.Context(), principal, core.CreateOrganizationInput{
			Code: request.Code, Name: request.Name,
		}, remoteIP(r.RemoteAddr))
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, http.StatusCreated, organization)
	}
}

func updateOrganizationHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request saveOrganizationRequest
		if !decodeJSON(w, r, &request, 32<<10) {
			return
		}
		organization, err := service.UpdateOrganization(r.Context(), principal, r.PathValue("organizationId"), core.UpdateOrganizationInput{
			Code: request.Code, Name: request.Name, IsActive: request.IsActive,
		}, remoteIP(r.RemoteAddr))
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, organization)
	}
}

func writeOrganizationError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, core.ErrOrganizationInvalid):
		http.Error(w, "invalid organization data", http.StatusBadRequest)
	case errors.Is(err, core.ErrForbidden):
		http.Error(w, "permission denied", http.StatusForbidden)
	case errors.Is(err, core.ErrOrganizationNotFound):
		http.Error(w, "organization not found", http.StatusNotFound)
	case errors.Is(err, core.ErrOrganizationConflict):
		http.Error(w, "organization code already in use", http.StatusConflict)
	default:
		log.Printf("organization write failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
	return true
}
