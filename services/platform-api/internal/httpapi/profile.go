package httpapi

import (
	"errors"
	"net/http"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
)

type updateSelfProfileRequest struct {
	Email       string `json:"email"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}

func ownProfileHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		profile, err := service.OwnProfile(r.Context(), principal)
		if writeProfileError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, profile)
	}
}

func updateOwnProfileHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request updateSelfProfileRequest
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		profile, err := service.UpdateOwnProfile(r.Context(), principal, core.UpdateSelfProfileInput(request), remoteIP(r.RemoteAddr))
		if writeProfileError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, profile)
	}
}

func writeProfileError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, core.ErrUserInvalid):
		http.Error(w, "invalid profile data", http.StatusBadRequest)
	case errors.Is(err, core.ErrUserConflict):
		http.Error(w, "email or username already exists", http.StatusConflict)
	case errors.Is(err, core.ErrUserNotFound):
		http.Error(w, "user not found", http.StatusNotFound)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
	return true
}
