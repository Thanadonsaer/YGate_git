package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthAndReadiness(t *testing.T) {
	h := New("test", func(context.Context) error { return nil }, nil, 0, nil, nil, nil)

	for _, test := range []struct {
		path, status string
	}{
		{path: "/healthz", status: "ok"},
		{path: "/readyz", status: "ready"},
	} {
		res := httptest.NewRecorder()
		h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, test.path, nil))
		if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"status":"`+test.status+`"`) || !strings.Contains(res.Body.String(), `"version":"test"`) {
			t.Fatalf("GET %s status=%d body=%s", test.path, res.Code, res.Body.String())
		}
	}

	res := httptest.NewRecorder()
	New("test", func(context.Context) error { return errors.New("down") }, nil, 0, nil, nil, nil).ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("failed readiness status=%d body=%s", res.Code, res.Body.String())
	}
}
