package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
)

type stubDashboardReader struct{ staleAfter time.Duration }

func (s *stubDashboardReader) DashboardOverview(_ context.Context, _ auth.Principal, staleAfter time.Duration, _ time.Time) (core.DashboardOverview, error) {
	s.staleAfter = staleAfter
	return core.DashboardOverview{}, nil
}

// Degraded must not trip until data has been missing for 10 minutes after the
// poll command, so the threshold the handler hands core is the contract here.
func TestDashboardOverviewStaleDefault(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  time.Duration
	}{
		{"default is ten minutes", "", 10 * time.Minute},
		{"explicit override still wins", "?staleAfterSeconds=60", time.Minute},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &stubDashboardReader{}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/overview"+test.query, nil)
			dashboardOverviewHandler(service)(recorder, request, auth.Principal{})
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
			}
			if service.staleAfter != test.want {
				t.Fatalf("staleAfter=%s want=%s", service.staleAfter, test.want)
			}
		})
	}
}
