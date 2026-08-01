package httpapi

import (
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"ygate/auth-service/internal/auth"
	"ygate/auth-service/internal/core"
)

type createAPIKeyRequest struct {
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
	AutoOnboard    bool   `json:"autoOnboard"`
}

type updateAPIKeyStatusRequest struct {
	IsActive *bool `json:"isActive"`
}

type updateAPIKeyRequest struct {
	Name        string `json:"name"`
	AutoOnboard bool   `json:"autoOnboard"`
	IsActive    *bool  `json:"isActive"`
}

func openAPIHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		if err := service.CanReadAPIContract(r.Context(), principal); errors.Is(err, core.ErrForbidden) {
			http.Error(w, "permission denied", http.StatusForbidden)
			return
		} else if err != nil {
			log.Printf("openapi permission failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		document, err := readOpenAPIContract()
		if err != nil {
			log.Printf("read openapi failed: %v", err)
			http.Error(w, "openapi contract unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(document)
	}
}

func listAPIKeysHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		clients, err := service.APIKeys(r.Context(), principal)
		if errors.Is(err, core.ErrForbidden) {
			http.Error(w, "permission denied", http.StatusForbidden)
			return
		}
		if err != nil {
			log.Printf("list api keys failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, clients)
	}
}

func createAPIKeyHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request createAPIKeyRequest
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		created, err := service.CreateAPIKey(r.Context(), principal, core.CreateAPIKeyInput(request), remoteIP(r.RemoteAddr))
		if writeAPIKeyError(w, err) {
			return
		}
		writeJSON(w, http.StatusCreated, created)
	}
}

func updateAPIKeyStatusHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request updateAPIKeyStatusRequest
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		if request.IsActive == nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		client, err := service.SetAPIKeyActive(r.Context(), principal, r.PathValue("keyId"), *request.IsActive, remoteIP(r.RemoteAddr))
		if writeAPIKeyError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, client)
	}
}

func updateAPIKeyHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request updateAPIKeyRequest
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		if request.IsActive == nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		client, err := service.UpdateAPIKey(r.Context(), principal, r.PathValue("keyId"), core.UpdateAPIKeyInput{Name: request.Name, AutoOnboard: request.AutoOnboard, IsActive: *request.IsActive}, remoteIP(r.RemoteAddr))
		if writeAPIKeyError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, client)
	}
}

func writeAPIKeyError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, core.ErrAPIKeyInvalid):
		http.Error(w, "invalid api key data", http.StatusBadRequest)
	case errors.Is(err, core.ErrForbidden):
		http.Error(w, "permission denied", http.StatusForbidden)
	case errors.Is(err, core.ErrAPIKeyNotFound):
		http.Error(w, "api key not found", http.StatusNotFound)
	case errors.Is(err, core.ErrAPIKeyConflict):
		http.Error(w, "api key already exists", http.StatusConflict)
	default:
		log.Printf("api key write failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
	return true
}

func readOpenAPIContract() ([]byte, error) {
	if name := strings.TrimSpace(os.Getenv("PLATFORM_OPENAPI_FILE")); name != "" {
		return os.ReadFile(name)
	}
	for _, name := range []string{
		filepath.Join("packages", "api-contracts", "platform-api.yaml"),
		filepath.Join("..", "..", "packages", "api-contracts", "platform-api.yaml"),
		filepath.Join("..", "..", "..", "packages", "api-contracts", "platform-api.yaml"),
	} {
		content, err := os.ReadFile(name)
		if err == nil {
			return content, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return nil, os.ErrNotExist
}
