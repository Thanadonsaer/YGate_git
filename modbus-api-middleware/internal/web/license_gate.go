package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"chpp/modbus-api-middleware/internal/license"
)

func (s *Server) licenseGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.LicensePublicKey == "" || s.hasValidLicense() {
			next.ServeHTTP(w, r)
			return
		}
		writeError(w, http.StatusForbidden, fmt.Errorf("license required; activate from Terminal with -activate-license"))
	})
}

func (s *Server) hasValidLicense() bool {
	for _, file := range s.licenseFiles() {
		if _, err := license.CheckFile(file, s.LicensePublicKey); err == nil {
			return true
		}
	}
	return false
}

func (s *Server) licenseFiles() []string {
	file := strings.TrimSpace(s.LicenseFile)
	exe, _ := os.Executable()
	def := filepath.Join(filepath.Dir(exe), "license.json")
	candidates := []string{file}
	if file == "" {
		candidates = []string{def}
	} else if !filepath.IsAbs(file) {
		candidates = append(candidates, filepath.Join(filepath.Dir(def), file))
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate = strings.TrimSpace(candidate); candidate != "" && !seen[candidate] {
			seen[candidate] = true
			out = append(out, candidate)
		}
	}
	return out
}
