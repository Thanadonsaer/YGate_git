package httpapi

import (
	"errors"
	"log"
	"net/http"

	"ygate/auth-service/internal/auth"
	"ygate/auth-service/internal/core"
)

type saveRoleRequest struct {
	Global         bool     `json:"global"`
	OrganizationID string   `json:"organizationId"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	PermissionIDs  []string `json:"permissionIds"`
}

func listPermissionsHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		permissions, err := service.Permissions(r.Context(), principal)
		if writeRoleError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, permissions)
	}
}

func getRoleHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		role, err := service.RoleDetail(r.Context(), principal, r.PathValue("roleId"))
		if writeRoleError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, role)
	}
}

func createRoleHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request saveRoleRequest
		if !decodeJSON(w, r, &request, 32<<10) {
			return
		}
		role, err := service.CreateRole(r.Context(), principal, core.CreateRoleInput{
			Global: request.Global, OrganizationID: request.OrganizationID,
			Name: request.Name, Description: request.Description, PermissionIDs: request.PermissionIDs,
		}, remoteIP(r.RemoteAddr))
		if writeRoleError(w, err) {
			return
		}
		writeJSON(w, http.StatusCreated, role)
	}
}

func updateRoleHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request saveRoleRequest
		if !decodeJSON(w, r, &request, 32<<10) {
			return
		}
		role, err := service.UpdateRole(r.Context(), principal, r.PathValue("roleId"), core.UpdateRoleInput{
			Name: request.Name, Description: request.Description, PermissionIDs: request.PermissionIDs,
		}, remoteIP(r.RemoteAddr))
		if writeRoleError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, role)
	}
}

func deleteRoleHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		if writeRoleError(w, service.DeleteRole(r.Context(), principal, r.PathValue("roleId"), remoteIP(r.RemoteAddr))) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeRoleError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, core.ErrRoleInvalid):
		http.Error(w, "invalid role data", http.StatusBadRequest)
	case errors.Is(err, core.ErrForbidden):
		http.Error(w, "permission denied", http.StatusForbidden)
	case errors.Is(err, core.ErrRoleNotFound):
		http.Error(w, "role not found", http.StatusNotFound)
	case errors.Is(err, core.ErrRoleConflict):
		http.Error(w, "role already exists", http.StatusConflict)
	case errors.Is(err, core.ErrRoleInUse):
		http.Error(w, "role is assigned to users", http.StatusConflict)
	default:
		log.Printf("role write failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
	return true
}
