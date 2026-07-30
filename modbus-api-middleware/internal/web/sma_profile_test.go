package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"chpp/modbus-api-middleware/internal/store"
)

func TestInstallSMAProfileIsIdempotent(t *testing.T) {
	st, err := store.OpenNormalized(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	handler := (&Server{Store: st}).FullHandler()
	for i := 0; i < 2; i++ {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/device-profiles/sma", nil))
		if res.Code != http.StatusCreated && res.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
		}
	}
	sets, err := st.DeviceSets()
	if err != nil || len(sets) != 1 || len(sets[0].Addresses) != 36 {
		t.Fatalf("sets=%+v err=%v", sets, err)
	}
	if sets[0].AddressMode != "SMA" || sets[0].Addresses[0].Register != 30057 {
		t.Fatalf("SMA profile was normalized incorrectly: %+v", sets[0])
	}
}
