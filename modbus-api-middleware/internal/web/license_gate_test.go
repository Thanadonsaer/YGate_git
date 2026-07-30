package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestLicenseGateBlocksUntilActivated(t *testing.T) {
	s := &Server{LicensePublicKey: "not-a-valid-public-key", LicenseFile: "missing-license.json"}
	h := s.FullHandler()

	web := httptest.NewRecorder()
	h.ServeHTTP(web, httptest.NewRequest(http.MethodGet, "/", nil))
	if web.Code != http.StatusForbidden {
		t.Fatalf("web status=%d body=%q", web.Code, web.Body.String())
	}

	api := httptest.NewRecorder()
	h.ServeHTTP(api, httptest.NewRequest(http.MethodGet, "/api/brands", nil))
	if api.Code != http.StatusForbidden {
		t.Fatalf("api status=%d body=%s", api.Code, api.Body.String())
	}
}

func TestServiceManagementRoutesAreNotExposed(t *testing.T) {
	s := &Server{}
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/service"},
		{http.MethodGet, "/api/service/status"},
		{http.MethodPost, "/api/service/action"},
	} {
		res := httptest.NewRecorder()
		s.FullHandler().ServeHTTP(res, httptest.NewRequest(tc.method, tc.path, nil))
		if res.Code != http.StatusNotFound {
			t.Fatalf("%s %s status=%d body=%s", tc.method, tc.path, res.Code, res.Body.String())
		}
	}
}

func TestLicenseFilesChecksExeDirForRelativePath(t *testing.T) {
	s := &Server{LicenseFile: "license.json"}
	files := s.licenseFiles()
	if len(files) != 2 {
		t.Fatalf("files=%v", files)
	}
	if files[0] != "license.json" {
		t.Fatalf("first file=%q", files[0])
	}
	if !filepath.IsAbs(files[1]) || filepath.Base(files[1]) != "license.json" {
		t.Fatalf("exe-dir file=%q", files[1])
	}
}
