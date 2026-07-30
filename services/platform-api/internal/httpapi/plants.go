package httpapi

import (
	"errors"
	"log"
	"net/http"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
)

type createPlantRequest struct {
	OrganizationID string   `json:"organizationId"`
	Code           string   `json:"code"`
	Name           string   `json:"name"`
	Timezone       string   `json:"timezone"`
	Latitude       *float64 `json:"latitude"`
	Longitude      *float64 `json:"longitude"`
	InstalledDcKW  *float64 `json:"installedDcKw"`
	InstalledAcKW  *float64 `json:"installedAcKw"`
}

type updatePlantRequest struct {
	Code          string   `json:"code"`
	Name          string   `json:"name"`
	Timezone      string   `json:"timezone"`
	Latitude      *float64 `json:"latitude"`
	Longitude     *float64 `json:"longitude"`
	InstalledDcKW *float64 `json:"installedDcKw"`
	InstalledAcKW *float64 `json:"installedAcKw"`
	IsActive      *bool    `json:"isActive"`
}

func listPlantsHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		plants, err := service.Plants(r.Context(), principal)
		if errors.Is(err, core.ErrForbidden) {
			http.Error(w, "permission denied", http.StatusForbidden)
			return
		}
		if err != nil {
			log.Printf("list plants failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, plants)
	}
}

func getPlantHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		plant, err := service.Plant(r.Context(), principal, r.PathValue("plantId"))
		if errors.Is(err, core.ErrNotFound) {
			http.Error(w, "plant not found", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("get plant failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, plant)
	}
}

func createPlantHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request createPlantRequest
		if !decodeJSON(w, r, &request, 32<<10) {
			return
		}
		plant, err := service.CreatePlant(r.Context(), principal, core.CreatePlantInput{
			OrganizationID: request.OrganizationID, Code: request.Code, Name: request.Name, Timezone: request.Timezone,
			Latitude: request.Latitude, Longitude: request.Longitude,
			InstalledDcKW: request.InstalledDcKW, InstalledAcKW: request.InstalledAcKW,
		}, remoteIP(r.RemoteAddr))
		if writePlantError(w, err) {
			return
		}
		writeJSON(w, http.StatusCreated, plant)
	}
}

func updatePlantHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request updatePlantRequest
		if !decodeJSON(w, r, &request, 32<<10) {
			return
		}
		if request.IsActive == nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		plant, err := service.UpdatePlant(r.Context(), principal, r.PathValue("plantId"), core.UpdatePlantInput{
			Code: request.Code, Name: request.Name, Timezone: request.Timezone,
			Latitude: request.Latitude, Longitude: request.Longitude,
			InstalledDcKW: request.InstalledDcKW, InstalledAcKW: request.InstalledAcKW, IsActive: *request.IsActive,
		}, remoteIP(r.RemoteAddr))
		if writePlantError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, plant)
	}
}

func writePlantError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, core.ErrInvalid):
		http.Error(w, "invalid plant data", http.StatusBadRequest)
	case errors.Is(err, core.ErrForbidden):
		http.Error(w, "permission denied", http.StatusForbidden)
	case errors.Is(err, core.ErrNotFound):
		http.Error(w, "plant not found", http.StatusNotFound)
	case errors.Is(err, core.ErrConflict):
		http.Error(w, "plant code already exists", http.StatusConflict)
	default:
		log.Printf("plant write failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
	return true
}
