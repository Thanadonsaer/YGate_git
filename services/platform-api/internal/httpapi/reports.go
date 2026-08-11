package httpapi

import (
	"errors"
	"net/http"
	"time"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
)

type createReportRequest struct {
	ReportType string    `json:"reportType"`
	PlantIDs   []string  `json:"plantIds"`
	From       time.Time `json:"from"`
	To         time.Time `json:"to"`
}

func exportReportHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request createReportRequest
		if !decodeJSON(w, r, &request, 32<<10) {
			return
		}
		data, err := service.ExportReportXLSX(r.Context(), principal, core.ReportRequest{
			ReportType: request.ReportType, PlantIDs: request.PlantIDs, From: request.From, To: request.To,
		}, remoteIP(r.RemoteAddr))
		if writeReportError(w, err) {
			return
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", `attachment; filename="ygate-report.xlsx"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}

func writeReportError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, core.ErrReportInvalid):
		http.Error(w, "invalid report request", http.StatusBadRequest)
	case errors.Is(err, core.ErrForbidden):
		http.Error(w, "permission denied", http.StatusForbidden)
	case errors.Is(err, core.ErrNotFound):
		http.Error(w, "plant not found", http.StatusNotFound)
	default:
		http.Error(w, "could not generate report", http.StatusInternalServerError)
	}
	return true
}
