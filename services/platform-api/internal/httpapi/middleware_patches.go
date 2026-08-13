package httpapi

import (
	"io"
	"net/http"
	"os"
	"time"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
	"ygate/platform-api/internal/ingestion"
)

type stageMiddlewareUpdateRequest struct {
	PatchID string `json:"patchId"`
}

type middlewareUpdateBatchRequest struct {
	Action        string   `json:"action"`
	PatchID       string   `json:"patchId,omitempty"`
	MiddlewareIDs []string `json:"middlewareIds"`
}

func listMiddlewarePatchesHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		patches, err := service.ListMiddlewarePatches(r.Context(), principal)
		if writeMiddlewareError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, patches)
	}
}

func uploadMiddlewarePatchHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		r.Body = http.MaxBytesReader(w, r.Body, 128<<20)
		if err := r.ParseMultipartForm(128 << 20); err != nil {
			http.Error(w, "invalid multipart upload", http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("patch")
		if err != nil {
			http.Error(w, "patch file is required", http.StatusBadRequest)
			return
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "failed to read upload", http.StatusBadRequest)
			return
		}
		patch, err := service.UploadMiddlewarePatch(r.Context(), principal, data, remoteIP(r.RemoteAddr))
		if writeMiddlewareError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, patch)
	}
}

func deleteMiddlewarePatchHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		err := service.DeleteMiddlewarePatch(r.Context(), principal, r.PathValue("patchId"), remoteIP(r.RemoteAddr))
		if writeMiddlewareError(w, err) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// downloadMiddlewarePatchHandler is called by a Middleware (not a browser).
// The original route uses X-Api-Key; the cacheable .zip route uses a short-
// lived patch-scoped token generated for one stage operation.
func downloadMiddlewarePatchHandler(ingestionService *ingestion.Service, service *core.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		tokenAuthorized := token != "" && service.AuthorizeMiddlewarePatchDownload(r.PathValue("patchId"), token, time.Now())
		if !tokenAuthorized {
			if _, err := ingestionService.Authenticate(r.Context(), r.Header.Get("X-Api-Key")); err != nil {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}
		}
		path, filename, err := service.MiddlewarePatchFilePath(r.Context(), r.PathValue("patchId"))
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		file, err := os.Open(path)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		defer file.Close()
		var modTime time.Time
		if info, err := file.Stat(); err == nil {
			modTime = info.ModTime()
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+".zip\"")
		if tokenAuthorized {
			w.Header().Set("Cache-Control", "public, max-age=300, immutable")
		}
		http.ServeContent(w, r, filename+".zip", modTime, file)
	}
}

func stageMiddlewareUpdateHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request stageMiddlewareUpdateRequest
		if !decodeJSON(w, r, &request, 4<<10) {
			return
		}
		job, err := service.CreateMiddlewareUpdateBatch(r.Context(), principal, "stage", request.PatchID, []string{r.PathValue("middlewareId")}, remoteIP(r.RemoteAddr))
		if writeMiddlewareError(w, err) {
			return
		}
		writeJSON(w, http.StatusAccepted, job)
	}
}

func applyMiddlewareUpdateHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		err := service.ApplyMiddlewareUpdate(r.Context(), principal, r.PathValue("middlewareId"), remoteIP(r.RemoteAddr))
		if writeMiddlewareError(w, err) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func createMiddlewareUpdateBatchHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request middlewareUpdateBatchRequest
		if !decodeJSON(w, r, &request, 64<<10) {
			return
		}
		job, err := service.CreateMiddlewareUpdateBatch(r.Context(), principal, request.Action, request.PatchID, request.MiddlewareIDs, remoteIP(r.RemoteAddr))
		if writeMiddlewareError(w, err) {
			return
		}
		writeJSON(w, http.StatusAccepted, job)
	}
}

func getMiddlewareUpdateBatchHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		job, err := service.MiddlewareUpdateBatch(r.Context(), principal, r.PathValue("jobId"))
		if writeMiddlewareError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, job)
	}
}

func rollbackMiddlewareUpdateHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		err := service.RollbackMiddlewareUpdate(r.Context(), principal, r.PathValue("middlewareId"), remoteIP(r.RemoteAddr))
		if writeMiddlewareError(w, err) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func restartMiddlewareHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		err := service.RestartMiddleware(r.Context(), principal, r.PathValue("middlewareId"), remoteIP(r.RemoteAddr))
		if writeMiddlewareError(w, err) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
