package httpapi

import (
	"errors"
	"log"
	"net/http"

	"ygate/auth-service/internal/auth"
	"ygate/auth-service/internal/core"
)

type createUserRequest struct {
	OrganizationID string `json:"organizationId"`
	Email          string `json:"email"`
	Username       string `json:"username"`
	DisplayName    string `json:"displayName"`
	Password       string `json:"password"`
	RoleID         string `json:"roleId"`
}

type updateUserStatusRequest struct {
	IsActive *bool `json:"isActive"`
}

type updateUserRequest struct {
	Email       string `json:"email"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	RoleID      string `json:"roleId"`
	IsActive    *bool  `json:"isActive"`
}

type resetUserPasswordRequest struct {
	NewPassword string `json:"newPassword"`
}

func listUsersHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		users, err := service.Users(r.Context(), principal)
		if errors.Is(err, core.ErrForbidden) {
			http.Error(w, "permission denied", http.StatusForbidden)
			return
		}
		if err != nil {
			log.Printf("list users failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, users)
	}
}

func listRolesHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		roles, err := service.Roles(r.Context(), principal)
		if errors.Is(err, core.ErrForbidden) {
			http.Error(w, "permission denied", http.StatusForbidden)
			return
		}
		if err != nil {
			log.Printf("list roles failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, roles)
	}
}

func createUserHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request createUserRequest
		if !decodeJSON(w, r, &request, 32<<10) {
			return
		}
		user, err := service.CreateUser(r.Context(), principal, core.CreateUserInput(request), remoteIP(r.RemoteAddr))
		if writeUserError(w, err) {
			return
		}
		writeJSON(w, http.StatusCreated, user)
	}
}

func updateUserStatusHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request updateUserStatusRequest
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		if request.IsActive == nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		user, err := service.SetUserActive(r.Context(), principal, r.PathValue("userId"), *request.IsActive, remoteIP(r.RemoteAddr))
		if writeUserError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, user)
	}
}

func updateUserHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request updateUserRequest
		if !decodeJSON(w, r, &request, 32<<10) {
			return
		}
		if request.IsActive == nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		user, err := service.UpdateUser(r.Context(), principal, r.PathValue("userId"), core.UpdateUserInput{Email: request.Email, Username: request.Username, DisplayName: request.DisplayName, RoleID: request.RoleID, IsActive: *request.IsActive}, remoteIP(r.RemoteAddr))
		if writeUserError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, user)
	}
}

func resetUserPasswordHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request resetUserPasswordRequest
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		if writeUserError(w, service.ResetUserPassword(r.Context(), principal, r.PathValue("userId"), request.NewPassword, remoteIP(r.RemoteAddr))) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func hardDeleteUserHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		if writeUserError(w, service.HardDeleteUser(r.Context(), principal, r.PathValue("userId"), r.Header.Get("X-Hard-Delete-Confirm"), remoteIP(r.RemoteAddr))) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func unlockUserHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		user, err := service.UnlockUser(r.Context(), principal, r.PathValue("userId"), remoteIP(r.RemoteAddr))
		if writeUserError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, user)
	}
}

func writeUserError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, core.ErrUserInvalid):
		http.Error(w, "invalid user data", http.StatusBadRequest)
	case errors.Is(err, core.ErrForbidden):
		http.Error(w, "permission denied", http.StatusForbidden)
	case errors.Is(err, core.ErrUserNotFound):
		http.Error(w, "user not found", http.StatusNotFound)
	case errors.Is(err, core.ErrUserConflict):
		http.Error(w, "user already exists", http.StatusConflict)
	default:
		log.Printf("user write failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
	return true
}
