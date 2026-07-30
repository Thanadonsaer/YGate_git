package web

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUpdateUploadStagesValidPatch(t *testing.T) {
	root := t.TempDir()
	s := &Server{Version: "0.1.1", UpdateRoot: root}
	body, contentType := updatePatchBody(t, updateManifest{App: updateAppName, Version: "0.1.1", OS: runtime.GOOS, Arch: runtime.GOARCH, Binary: binaryName(), SHA256: shaHex([]byte("binary"))}, []byte("binary"), "")
	req := httptest.NewRequest(http.MethodPost, "/api/update/upload", body)
	req.Header.Set("Content-Type", contentType)
	res := httptest.NewRecorder()
	s.updateUpload(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "staged", binaryName())); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateUploadRejectsBadInputs(t *testing.T) {
	cases := []struct {
		name      string
		manifest  updateManifest
		bin       []byte
		extraName string
	}{
		{"wrong target", updateManifest{App: updateAppName, Version: "0.1.1", OS: "other", Arch: runtime.GOARCH, Binary: binaryName(), SHA256: shaHex([]byte("binary"))}, []byte("binary"), ""},
		{"wrong hash", updateManifest{App: updateAppName, Version: "0.1.1", OS: runtime.GOOS, Arch: runtime.GOARCH, Binary: binaryName(), SHA256: shaHex([]byte("nope"))}, []byte("binary"), ""},
		{"db included", updateManifest{App: updateAppName, Version: "0.1.1", OS: runtime.GOOS, Arch: runtime.GOARCH, Binary: binaryName(), SHA256: shaHex([]byte("binary"))}, []byte("binary"), "middleware.db"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{Version: "0.1.1", UpdateRoot: t.TempDir()}
			body, contentType := updatePatchBody(t, tc.manifest, tc.bin, tc.extraName)
			req := httptest.NewRequest(http.MethodPost, "/api/update/upload", body)
			req.Header.Set("Content-Type", contentType)
			res := httptest.NewRecorder()
			s.updateUpload(res, req)
			if res.Code < 400 {
				t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
			}
		})
	}
}

func updatePatchBody(t *testing.T, manifest updateManifest, bin []byte, extraName string) (*bytes.Buffer, string) {
	t.Helper()
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	mw, err := zw.Create("update-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.NewEncoder(mw).Encode(manifest); err != nil {
		t.Fatal(err)
	}
	bw, err := zw.Create(manifest.Binary)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bw.Write(bin); err != nil {
		t.Fatal(err)
	}
	if extraName != "" {
		ew, err := zw.Create(extraName)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = ew.Write([]byte("extra"))
	}
	if err = zw.Close(); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	mw2 := multipart.NewWriter(&body)
	fw, err := mw2.CreateFormFile("patch", "patch.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fw.Write(zipBuf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err = mw2.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, mw2.FormDataContentType()
}

func shaHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "middleware.exe"
	}
	return "middleware"
}

func TestWindowsUpdaterWaitsForServiceAndReportsResult(t *testing.T) {
	for _, required := range []string{"WaitForStatus(\"Stopped\"", "Copy-Item -LiteralPath", "WaitForStatus(\"Running\"", "$ResultFile"} {
		if !strings.Contains(windowsUpdaterScript, required) {
			t.Fatalf("updater script missing %q", required)
		}
	}
	if strings.Contains(windowsUpdaterScript, "timeout /t") {
		t.Fatal("updater must not use timeout in a non-interactive service")
	}
}

func TestUpdatePageGuardsOptionalElementsAndStandaloneApply(t *testing.T) {
	page, err := files.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)
	for _, required := range []string{
		"if (setBrand) setBrand.innerHTML",
		"if (brandFilter)",
		"if (connectionSet) connectionSet.innerHTML",
		"if (!state.update.canApply)",
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("update page missing guard %q", required)
		}
	}
}

func TestUpdateUploadAcceptsManifestBOM(t *testing.T) {
	root := t.TempDir()
	s := &Server{Version: "0.1.1", UpdateRoot: root}
	body, contentType := updatePatchBodyWithBOM(t, updateManifest{App: updateAppName, Version: "0.1.1", OS: runtime.GOOS, Arch: runtime.GOARCH, Binary: binaryName(), SHA256: shaHex([]byte("binary"))}, []byte("binary"))
	req := httptest.NewRequest(http.MethodPost, "/api/update/upload", body)
	req.Header.Set("Content-Type", contentType)
	res := httptest.NewRecorder()
	s.updateUpload(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}
func updatePatchBodyWithBOM(t *testing.T, manifest updateManifest, bin []byte) (*bytes.Buffer, string) {
	t.Helper()
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	mw, err := zw.Create("update-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = mw.Write([]byte{0xef, 0xbb, 0xbf})
	if err = json.NewEncoder(mw).Encode(manifest); err != nil {
		t.Fatal(err)
	}
	bw, err := zw.Create(manifest.Binary)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bw.Write(bin); err != nil {
		t.Fatal(err)
	}
	if err = zw.Close(); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	mw2 := multipart.NewWriter(&body)
	fw, err := mw2.CreateFormFile("patch", "patch.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fw.Write(zipBuf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err = mw2.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, mw2.FormDataContentType()
}
