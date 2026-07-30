package httpapi

import (
	"errors"
	"log"
	"net/http"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
)

type updateDeviceRequest struct {
	Name     string `json:"name"`
	IsActive *bool  `json:"isActive"`
}

type createDeviceRequest struct {
	ExternalID   string `json:"externalId"`
	Name         string `json:"name"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	DeviceType   string `json:"deviceType"`
	SourceTypeID *int32 `json:"sourceTypeId"`
	SerialNumber string `json:"serialNumber"`
}

type deviceModelRequest struct {
	OrganizationID string `json:"organizationId"`
	Manufacturer   string `json:"manufacturer"`
	Model          string `json:"model"`
	DeviceType     string `json:"deviceType"`
	SourceTypeID   *int32 `json:"sourceTypeId"`
	IsActive       *bool  `json:"isActive"`
}

type updateRegisterMetadataRequest struct {
	AddressKey  string  `json:"addressKey"`
	DisplayName string  `json:"displayName"`
	Unit        string  `json:"unit"`
	DataType    string  `json:"dataType"`
	Scale       float64 `json:"scale"`
	Offset      float64 `json:"offset"`
	Decimals    int32   `json:"decimals"`
	IsEnabled   *bool   `json:"isEnabled"`
	Notes       string  `json:"notes"`
}

func listDevicesHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		devices, err := service.Devices(r.Context(), principal, r.PathValue("plantId"))
		if errors.Is(err, core.ErrNotFound) {
			http.Error(w, "plant not found", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("list devices failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, devices)
	}
}

func createDeviceHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request createDeviceRequest
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		device, err := service.CreateDevice(r.Context(), principal, r.PathValue("plantId"), core.CreateDeviceInput(request), remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, core.ErrInvalid):
			http.Error(w, "invalid device data", http.StatusBadRequest)
		case errors.Is(err, core.ErrNotFound), errors.Is(err, core.ErrForbidden):
			http.Error(w, "plant not found", http.StatusNotFound)
		case errors.Is(err, core.ErrConflict):
			http.Error(w, "device already exists", http.StatusConflict)
		case err != nil:
			log.Printf("create device failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusCreated, device)
		}
	}
}

func listDeviceModelsHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		models, err := service.DeviceModels(r.Context(), principal)
		switch {
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case err != nil:
			log.Printf("list device models failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusOK, models)
		}
	}
}

func createDeviceModelHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request deviceModelRequest
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		isActive := true
		if request.IsActive != nil {
			isActive = *request.IsActive
		}
		model, err := service.CreateDeviceModel(r.Context(), principal, core.DeviceModelInput{
			OrganizationID: request.OrganizationID,
			Manufacturer:   request.Manufacturer,
			Model:          request.Model,
			DeviceType:     request.DeviceType,
			SourceTypeID:   request.SourceTypeID,
			IsActive:       isActive,
		}, remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, core.ErrInvalid):
			http.Error(w, "invalid device model data", http.StatusBadRequest)
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case errors.Is(err, core.ErrConflict):
			http.Error(w, "device model already exists", http.StatusConflict)
		case err != nil:
			log.Printf("create device model failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusCreated, model)
		}
	}
}

func updateDeviceModelHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request deviceModelRequest
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		if request.IsActive == nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		model, err := service.UpdateDeviceModel(r.Context(), principal, r.PathValue("modelId"), core.DeviceModelInput{
			Manufacturer: request.Manufacturer,
			Model:        request.Model,
			DeviceType:   request.DeviceType,
			SourceTypeID: request.SourceTypeID,
			IsActive:     *request.IsActive,
		}, remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, core.ErrInvalid):
			http.Error(w, "invalid device model data", http.StatusBadRequest)
		case errors.Is(err, core.ErrNotFound):
			http.Error(w, "device model not found", http.StatusNotFound)
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case errors.Is(err, core.ErrConflict):
			http.Error(w, "device model already exists", http.StatusConflict)
		case err != nil:
			log.Printf("update device model failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusOK, model)
		}
	}
}

func listModelRegisterMetadataHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		items, err := service.DeviceModelRegisterMetadata(r.Context(), principal, r.PathValue("modelId"))
		switch {
		case errors.Is(err, core.ErrNotFound):
			http.Error(w, "device model not found", http.StatusNotFound)
		case err != nil:
			log.Printf("list model register metadata failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusOK, items)
		}
	}
}

func updateModelRegisterMetadataHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request updateRegisterMetadataRequest
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		if request.IsEnabled == nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		item, err := service.SetDeviceModelRegisterMetadata(r.Context(), principal, r.PathValue("modelId"), core.UpdateDeviceModelRegisterMetadataInput{
			AddressKey: request.AddressKey, DisplayName: request.DisplayName, Unit: request.Unit, DataType: request.DataType,
			Scale: request.Scale, Offset: request.Offset, Decimals: request.Decimals, IsEnabled: *request.IsEnabled, Notes: request.Notes,
		}, remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, core.ErrInvalid):
			http.Error(w, "invalid register metadata", http.StatusBadRequest)
		case errors.Is(err, core.ErrNotFound):
			http.Error(w, "device model not found", http.StatusNotFound)
		case err != nil:
			log.Printf("update model register metadata failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusOK, item)
		}
	}
}

func deleteModelRegisterMetadataHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		err := service.DeleteDeviceModelRegisterMetadata(r.Context(), principal, r.PathValue("modelId"), r.PathValue("addressKey"), remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, core.ErrInvalid):
			http.Error(w, "invalid register metadata", http.StatusBadRequest)
		case errors.Is(err, core.ErrNotFound):
			http.Error(w, "register metadata not found", http.StatusNotFound)
		case err != nil:
			log.Printf("delete model register metadata failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func updateDeviceHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request updateDeviceRequest
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		if request.IsActive == nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		device, err := service.UpdateDevice(r.Context(), principal, r.PathValue("plantId"), r.PathValue("deviceId"), core.UpdateDeviceInput{Name: request.Name, IsActive: *request.IsActive}, remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, core.ErrInvalid):
			http.Error(w, "invalid device data", http.StatusBadRequest)
		case errors.Is(err, core.ErrNotFound):
			http.Error(w, "device not found", http.StatusNotFound)
		case err != nil:
			log.Printf("update device failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusOK, device)
		}
	}
}

func listRegisterMetadataHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		items, err := service.DeviceRegisterMetadata(r.Context(), principal, r.PathValue("plantId"), r.PathValue("deviceId"))
		switch {
		case errors.Is(err, core.ErrNotFound):
			http.Error(w, "device not found", http.StatusNotFound)
		case err != nil:
			log.Printf("list register metadata failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusOK, items)
		}
	}
}

func updateRegisterMetadataHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request updateRegisterMetadataRequest
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		if request.IsEnabled == nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		item, err := service.SetDeviceRegisterMetadata(r.Context(), principal, r.PathValue("plantId"), r.PathValue("deviceId"), core.UpdateDeviceRegisterMetadataInput{
			AddressKey: request.AddressKey, DisplayName: request.DisplayName, Unit: request.Unit, DataType: request.DataType,
			Scale: request.Scale, Offset: request.Offset, Decimals: request.Decimals, IsEnabled: *request.IsEnabled, Notes: request.Notes,
		}, remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, core.ErrInvalid):
			http.Error(w, "invalid register metadata", http.StatusBadRequest)
		case errors.Is(err, core.ErrNotFound):
			http.Error(w, "device not found", http.StatusNotFound)
		case err != nil:
			log.Printf("update register metadata failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusOK, item)
		}
	}
}
