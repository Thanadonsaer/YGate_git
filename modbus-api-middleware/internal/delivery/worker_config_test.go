package delivery

import (
	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/store"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestDeliveryUsesSavedGatewayConfig(t *testing.T) {
	st, err := store.OpenNormalized(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	r := testReading()
	_, _ = st.Enqueue("key", "hash", r)
	var key string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { key = r.Header.Get("X-Api-Key"); w.WriteHeader(201) }))
	defer srv.Close()
	if _, err = st.SaveGatewayConfig(domain.GatewayConfig{Endpoint: srv.URL, APIKey: "saved-secret", APIPollingEnabled: true}); err != nil {
		t.Fatal(err)
	}
	n, err := (&Worker{Store: st, BatchSize: 20, Client: srv.Client()}).SendOnce()
	if err != nil || n != 1 || key != "saved-secret" {
		t.Fatalf("n=%d key=%s err=%v", n, key, err)
	}
}

func TestDeliverySkipsSavedGatewayConfigWhenAPIPollingDisabled(t *testing.T) {
	st, err := store.OpenNormalized(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(201) }))
	defer srv.Close()
	if _, err = st.SaveGatewayConfig(domain.GatewayConfig{Endpoint: srv.URL, APIKey: "saved-secret"}); err != nil {
		t.Fatal(err)
	}
	n, err := (&Worker{Store: st, BatchSize: 20, Client: srv.Client(), BeforeSend: func() error { called = true; return nil }}).SendOnce()
	if err != nil || n != 0 || called {
		t.Fatalf("n=%d called=%v err=%v", n, called, err)
	}
}
