package delivery

import (
	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/store"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestDelivery(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	r := testReading()
	_, _ = st.Enqueue("key", "hash", r)
	var key string
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key = r.Header.Get("X-Api-Key")
		payload, _ := io.ReadAll(r.Body)
		body = string(payload)
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	n, err := (&Worker{Store: st, Endpoint: srv.URL, APIKey: "secret", BatchSize: 20, Client: srv.Client()}).SendOnce()
	if err != nil || n != 1 || key != "secret" {
		t.Fatalf("n=%d key=%s err=%v", n, key, err)
	}
	if !strings.Contains(body, `"schemaVersion":"2.0"`) || !strings.Contains(body, `"registerAddressMap"`) || strings.Contains(body, `"dataItemMap"`) {
		t.Fatalf("unexpected v2 body: %s", body)
	}
	ready, err := st.Ready(20)
	if err != nil || len(ready) != 0 {
		t.Fatalf("delivered remained ready: %+v %v", ready, err)
	}
	logs, err := st.DeliveryLogs(1)
	if err != nil || len(logs) != 1 || logs[0].LastHTTPStatus != 201 || logs[0].LastResponse != `{"ok":true}` {
		t.Fatalf("logs=%+v err=%v", logs, err)
	}
}

func TestDeliveryRunsBeforeSendAfterConfig(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(201) }))
	defer srv.Close()
	called := false
	worker := &Worker{Store: st, Endpoint: srv.URL, APIKey: "secret", BatchSize: 20, Client: srv.Client(), BeforeSend: func() error {
		called = true
		r := testReading()
		_, err := st.Enqueue("key", "hash", r)
		return err
	}}
	n, err := worker.SendOnce()
	if err != nil || n != 1 || !called {
		t.Fatalf("n=%d called=%v err=%v", n, called, err)
	}
}

func TestDeliverySkipsBeforeSendWithoutConfig(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	called := false
	_, _ = (&Worker{Store: st, BeforeSend: func() error { called = true; return nil }}).SendOnce()
	if called {
		t.Fatal("BeforeSend called without API config")
	}
}

func testReading() domain.Reading {
	return domain.Reading{GatewayID: "G1", DevDn: "D1", PlantCode: "P1", DevTypeID: 1, CollectTime: 1, RegisterAddressMap: map[string]float64{"1": 1}}
}
