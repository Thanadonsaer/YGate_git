package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

type healthResponse struct {
	Status string `json:"status"`
}

// authPrefixes are the path prefixes whose business logic lives in
// auth-service (see services/auth-service/internal/httpapi/server.go's
// route registrations): session/credential management under /api/v1/auth/,
// plus the users/roles/permissions/api-keys admin CRUD and the openapi doc
// endpoint that moved there too. Everything else still goes to platform-api.
var authPrefixes = []string{
	"/api/v1/auth/",
	"/api/v1/admin/users",
	"/api/v1/admin/roles",
	"/api/v1/admin/permissions",
	"/api/v1/admin/api-keys",
	"/api/v1/admin/openapi",
}

func New(platformURL, authServiceURL *url.URL, allowedOrigins []string) http.Handler {
	platformProxy := newProxy(platformURL)
	authProxy := newProxy(authServiceURL)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /gateway/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
	})
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// DELETE /api/v1/admin/api-keys/{keyId} is a hard-delete endpoint that
		// stayed on platform-api even though the rest of the api-keys CRUD
		// (list/create/update/status) moved to auth-service -- so it needs a
		// method-aware exception to the prefix rule below, or it would 405 on
		// auth-service instead of reaching platform-api's handler.
		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/admin/api-keys/") {
			platformProxy.ServeHTTP(w, r)
			return
		}
		for _, prefix := range authPrefixes {
			if strings.HasPrefix(r.URL.Path, prefix) {
				authProxy.ServeHTTP(w, r)
				return
			}
		}
		platformProxy.ServeHTTP(w, r)
	}))
	return middleware(mux, allowedOrigins)
}

func newProxy(target *url.URL) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(target)
			request.SetXForwarded()
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			log.Printf("proxy to %s failed: %v", target, err)
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
		},
	}
}

func middleware(next http.Handler, allowedOrigins []string) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Request-ID", requestID())
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Add("Vary", "Origin")
			if _, ok := allowed[origin]; !ok {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token, X-Hard-Delete-Confirm")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(value[:])
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
