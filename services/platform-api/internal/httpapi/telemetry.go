package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
)

type latestTelemetryReader interface {
	LatestTelemetry(context.Context, auth.Principal, string) ([]core.LatestTelemetry, error)
}

type historyTelemetryReader interface {
	TelemetryHistory(context.Context, auth.Principal, string, string, core.TelemetryHistoryInput) (core.TelemetryHistoryPage, error)
}

func latestTelemetryHandler(service latestTelemetryReader) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		readings, err := service.LatestTelemetry(r.Context(), principal, r.PathValue("plantId"))
		if errors.Is(err, core.ErrNotFound) {
			http.Error(w, "plant not found", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("latest telemetry failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, readings)
	}
}

func telemetryHistoryHandler(service historyTelemetryReader) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		from, fromErr := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		to, toErr := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		limit := 100
		var limitErr error
		if value := r.URL.Query().Get("limit"); value != "" {
			limit, limitErr = strconv.Atoi(value)
		}
		if fromErr != nil || toErr != nil || limitErr != nil {
			http.Error(w, "invalid telemetry history query", http.StatusBadRequest)
			return
		}
		page, err := service.TelemetryHistory(r.Context(), principal, r.PathValue("plantId"), r.PathValue("deviceId"), core.TelemetryHistoryInput{
			From: from, To: to, Limit: limit, Cursor: r.URL.Query().Get("cursor"),
		})
		switch {
		case errors.Is(err, core.ErrInvalid):
			http.Error(w, "invalid telemetry history query", http.StatusBadRequest)
		case errors.Is(err, core.ErrNotFound):
			http.Error(w, "plant or device not found", http.StatusNotFound)
		case err != nil:
			log.Printf("telemetry history failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusOK, page)
		}
	}
}
