package httpapi

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
)

func uploadScadaImageHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
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
		imageURL, err := service.UploadScadaImage(r.Context(), principal, r.PathValue("screenId"), data)
		switch {
		case err == nil:
			writeJSON(w, http.StatusCreated, map[string]string{"url": imageURL})
		case errors.Is(err, core.ErrInvalid):
			http.Error(w, "invalid image", http.StatusBadRequest)
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case errors.Is(err, core.ErrScadaNotFound):
			http.Error(w, "screen not found", http.StatusNotFound)
		default:
			log.Printf("scada image upload failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
	}
}

func serveScadaImageHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		path, err := service.ScadaImageFilePath(r.Context(), principal, r.PathValue("screenId"), r.PathValue("filename"))
		if errors.Is(err, core.ErrScadaNotFound) || errors.Is(err, core.ErrForbidden) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("serve scada image failed: %v", err)
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
