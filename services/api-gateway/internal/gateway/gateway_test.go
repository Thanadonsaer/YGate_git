package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestGatewayHealthAndProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/me" || r.Header.Get("X-Forwarded-For") == "" || r.Header.Get("X-Real-IP") != "192.0.2.10" {
			t.Fatalf("path=%s forwarded=%q real=%q", r.URL.Path, r.Header.Get("X-Forwarded-For"), r.Header.Get("X-Real-IP"))
		}
		w.WriteHeader(http.StatusTeapot)
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	handler := New(upstreamURL, upstreamURL, []string{"http://localhost:8080"})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/gateway/healthz", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"ok"`) {
		t.Fatalf("health status=%d body=%s", response.Code, response.Body.String())
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.RemoteAddr = "192.0.2.10:4321"
	request.Header.Set("Origin", "http://localhost:8080")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusTeapot || response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:8080" || response.Header().Get("X-Request-ID") == "" {
		t.Fatalf("proxy status=%d headers=%v", response.Code, response.Header())
	}
}

func TestGatewayCORS(t *testing.T) {
	upstreamURL, _ := url.Parse("http://127.0.0.1:44441")
	handler := New(upstreamURL, upstreamURL, []string{"http://localhost:8080"})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.Header.Set("Origin", "https://unknown.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("unknown origin status=%d", response.Code)
	}

	request = httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	request.Header.Set("Origin", "http://localhost:8080")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Credentials") != "true" || !strings.Contains(response.Header().Get("Access-Control-Allow-Methods"), "PUT") || !strings.Contains(response.Header().Get("Access-Control-Allow-Headers"), "X-Hard-Delete-Confirm") {
		t.Fatalf("preflight status=%d headers=%v", response.Code, response.Header())
	}
}

func TestGatewayRoutesByPathPrefix(t *testing.T) {
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer platform.Close()
	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer auth.Close()
	platformURL, _ := url.Parse(platform.URL)
	authURL, _ := url.Parse(auth.URL)
	handler := New(platformURL, authURL, []string{"http://localhost:8080"})

	for _, test := range []struct {
		method, path string
		wantAuth     bool
	}{
		{http.MethodPost, "/api/v1/auth/login", true},
		{http.MethodGet, "/api/v1/admin/users", true},
		{http.MethodGet, "/api/v1/admin/roles", true},
		{http.MethodGet, "/api/v1/admin/permissions", true},
		{http.MethodGet, "/api/v1/admin/api-keys", true},
		{http.MethodPut, "/api/v1/admin/api-keys/abc", true},
		{http.MethodGet, "/api/v1/admin/openapi", true},
		{http.MethodDelete, "/api/v1/admin/api-keys/abc", false}, // hard-delete stayed on platform-api
		{http.MethodGet, "/api/v1/plants", false},
		{http.MethodGet, "/api/v1/realtime", false},
	} {
		request := httptest.NewRequest(test.method, test.path, nil)
		request.Header.Set("Origin", "http://localhost:8080")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		wantCode := http.StatusOK
		if test.wantAuth {
			wantCode = http.StatusCreated
		}
		if response.Code != wantCode {
			t.Fatalf("%s %s: got status=%d want=%d", test.method, test.path, response.Code, wantCode)
		}
	}
}

func TestGatewayProxiesWebSocketUpgrade(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer connection.CloseNow()
		_ = wsjson.Write(r.Context(), connection, map[string]string{"type": "connection.ready"})
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	proxy := httptest.NewServer(New(upstreamURL, upstreamURL, []string{"http://localhost:8080"}))
	defer proxy.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(proxy.URL, "http")+"/api/v1/realtime", &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"http://localhost:8080"}},
	})
	if err != nil {
		t.Fatalf("dial status=%v err=%v", response, err)
	}
	defer connection.CloseNow()
	var message map[string]string
	if err = wsjson.Read(ctx, connection, &message); err != nil || message["type"] != "connection.ready" {
		t.Fatalf("message=%v err=%v", message, err)
	}
}
