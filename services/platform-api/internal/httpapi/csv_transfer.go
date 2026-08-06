package httpapi

import (
	"errors"
	"io"
	"log"
	"net/http"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
)

func writeCSVFile(w http.ResponseWriter, filename string, data []byte) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func writeCSVImportResult(w http.ResponseWriter, result core.CSVImportResult, err error) {
	switch {
	case err != nil:
		log.Printf("csv import failed: %v", err)
		http.Error(w, "invalid csv upload", http.StatusBadRequest)
	default:
		writeJSON(w, http.StatusOK, result)
	}
}

func readCSVUpload(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Error(w, "invalid csv upload", http.StatusBadRequest)
		return nil, false
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "csv file is required", http.StatusBadRequest)
		return nil, false
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "failed to read csv file", http.StatusBadRequest)
		return nil, false
	}
	return data, true
}

func registerMetadataCSVTemplateHandler() func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		data, err := core.RegisterMetadataCSVTemplate()
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeCSVFile(w, "register-metadata-template.csv", data)
	}
}

func exportRegisterMetadataCSVHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		data, err := service.ExportRegisterMetadataCSV(r.Context(), principal, "")
		switch {
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case err != nil:
			log.Printf("export register metadata csv failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeCSVFile(w, "register-metadata.csv", data)
		}
	}
}

func exportModelRegisterMetadataCSVHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		data, err := service.ExportRegisterMetadataCSV(r.Context(), principal, r.PathValue("modelId"))
		switch {
		case errors.Is(err, core.ErrNotFound):
			http.Error(w, "device model not found", http.StatusNotFound)
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case err != nil:
			log.Printf("export device model register metadata csv failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeCSVFile(w, "register-metadata.csv", data)
		}
	}
}

func importRegisterMetadataCSVHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		data, ok := readCSVUpload(w, r)
		if !ok {
			return
		}
		result, err := service.ImportRegisterMetadataCSV(r.Context(), principal, r.FormValue("organizationId"), data, remoteIP(r.RemoteAddr))
		writeCSVImportResult(w, result, err)
	}
}

func plantDeviceCSVTemplateHandler() func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		data, err := core.PlantDeviceCSVTemplate()
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeCSVFile(w, "plant-device-template.csv", data)
	}
}

func exportPlantDeviceCSVHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		data, err := service.ExportPlantDeviceCSV(r.Context(), principal, "")
		switch {
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case err != nil:
			log.Printf("export plant/device csv failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeCSVFile(w, "plants-devices.csv", data)
		}
	}
}

func exportOnePlantDeviceCSVHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		data, err := service.ExportPlantDeviceCSV(r.Context(), principal, r.PathValue("plantId"))
		switch {
		case errors.Is(err, core.ErrNotFound):
			http.Error(w, "plant not found", http.StatusNotFound)
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case err != nil:
			log.Printf("export plant/device csv failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeCSVFile(w, "plant-devices.csv", data)
		}
	}
}

func importPlantDeviceCSVHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		data, ok := readCSVUpload(w, r)
		if !ok {
			return
		}
		result, err := service.ImportPlantDeviceCSV(r.Context(), principal, r.FormValue("organizationId"), data, remoteIP(r.RemoteAddr))
		writeCSVImportResult(w, result, err)
	}
}
