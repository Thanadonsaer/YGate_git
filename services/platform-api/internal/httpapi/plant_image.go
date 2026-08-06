package httpapi

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
)

func uploadPlantImageHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20+4<<10)
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			http.Error(w, "invalid image upload", http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("image")
		if err != nil {
			http.Error(w, "image file is required", http.StatusBadRequest)
			return
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "failed to read image", http.StatusBadRequest)
			return
		}
		plant, err := service.UploadPlantImage(r.Context(), principal, r.PathValue("plantId"), data, remoteIP(r.RemoteAddr))
		writePlantImageResult(w, plant, err)
	}
}

func deletePlantImageHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		plant, err := service.DeletePlantImage(r.Context(), principal, r.PathValue("plantId"), remoteIP(r.RemoteAddr))
		writePlantImageResult(w, plant, err)
	}
}

func servePlantImageHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		path, err := service.PlantImageFilePath(r.Context(), principal, r.PathValue("plantId"), r.PathValue("filename"))
		if errors.Is(err, core.ErrNotFound) || errors.Is(err, core.ErrForbidden) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("serve plant image failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		file, err := os.Open(path)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Cache-Control", "private, max-age=300")
		http.ServeContent(w, r, r.PathValue("filename"), info.ModTime(), file)
	}
}

func writePlantImageResult(w http.ResponseWriter, plant core.Plant, err error) {
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, plant)
	case errors.Is(err, core.ErrInvalid):
		http.Error(w, "invalid image", http.StatusBadRequest)
	case errors.Is(err, core.ErrForbidden):
		http.Error(w, "permission denied", http.StatusForbidden)
	case errors.Is(err, core.ErrNotFound):
		http.Error(w, "plant not found", http.StatusNotFound)
	default:
		log.Printf("plant image request failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

var _ = time.Second
