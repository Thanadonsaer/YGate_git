package httpapi

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
)

func auditEventsHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		limit, err := auditLimit(r.URL.Query().Get("limit"))
		if err != nil {
			http.Error(w, "invalid limit", http.StatusBadRequest)
			return
		}
		events, err := service.AuditEvents(r.Context(), principal, limit)
		if errors.Is(err, core.ErrForbidden) {
			http.Error(w, "permission denied", http.StatusForbidden)
			return
		}
		if err != nil {
			log.Printf("list audit events failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, events)
	}
}

func clearAuditEventsHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		result, err := service.ClearAuditEvents(r.Context(), principal, r.Header.Get("X-Hard-Delete-Confirm"), remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, core.ErrInvalid):
			http.Error(w, "invalid confirmation", http.StatusBadRequest)
		case errors.Is(err, core.ErrForbidden):
			http.Error(w, "permission denied", http.StatusForbidden)
		case err != nil:
			log.Printf("clear audit events failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusOK, result)
		}
	}
}

func auditLimit(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 100, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > 200 {
		return 0, strconv.ErrSyntax
	}
	return limit, nil
}
