package web

import (
	"fmt"
	"io"
	"net/http"

	"chpp/modbus-api-middleware/internal/updater"
)

// updateAppName kept for compatibility with call sites/tests that expect a
// web-package-local name; it's the same value as updater.AppName.
const updateAppName = updater.AppName

// manager builds an updater.Manager from this Server's own Version/
// CanApplyUpdate/UpdateRoot fields -- cheap, so every handler just builds
// one on demand rather than the Server needing a separately-wired field.
func (s *Server) manager() *updater.Manager {
	return &updater.Manager{Version: s.Version, CanApply: s.CanApplyUpdate, Root: s.UpdateRoot}
}

func (s *Server) updateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.manager().Status())
}

func (s *Server) updateUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<20)
	file, _, err := r.FormFile("patch")
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("patch file is required"))
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	m, err := s.manager().StageZip(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"staged": m})
}

func (s *Server) updateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	backup, err := s.manager().Apply()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "update started; service will restart", "backup": backup})
}

func (s *Server) updateRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	backup, err := s.manager().Rollback()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "rollback started; service will restart", "backup": backup})
}
