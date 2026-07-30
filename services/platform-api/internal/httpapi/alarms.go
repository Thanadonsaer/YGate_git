package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
)

type alarmEventReader interface {
	AlarmEventsSince(context.Context, auth.Principal, string, int64) ([]core.AlarmEvent, error)
	LatestAlarmEventID(context.Context, auth.Principal, string) (int64, error)
}

type saveAlarmRuleRequest struct {
	DeviceID string   `json:"deviceId"`
	PointKey string   `json:"pointKey"`
	Label    string   `json:"label"`
	MinValue *float64 `json:"minValue"`
	MaxValue *float64 `json:"maxValue"`
	Severity string   `json:"severity"`
	IsActive *bool    `json:"isActive"`
}

type acknowledgeAlarmEventRequest struct {
	Note string `json:"note"`
}

func listAlarmRulesHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		rules, err := service.ListAlarmRules(r.Context(), principal, r.PathValue("plantId"))
		if writeAlarmError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, rules)
	}
}

func createAlarmRuleHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request saveAlarmRuleRequest
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		rule, err := service.CreateAlarmRule(r.Context(), principal, r.PathValue("plantId"), core.CreateAlarmRuleInput{
			DeviceID: request.DeviceID, PointKey: request.PointKey, Label: request.Label,
			MinValue: request.MinValue, MaxValue: request.MaxValue, Severity: request.Severity,
		}, remoteIP(r.RemoteAddr))
		if writeAlarmError(w, err) {
			return
		}
		writeJSON(w, http.StatusCreated, rule)
	}
}

func updateAlarmRuleHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request saveAlarmRuleRequest
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		if request.IsActive == nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		rule, err := service.UpdateAlarmRule(r.Context(), principal, r.PathValue("plantId"), r.PathValue("ruleId"), core.UpdateAlarmRuleInput{
			Label: request.Label, MinValue: request.MinValue, MaxValue: request.MaxValue,
			Severity: request.Severity, IsActive: *request.IsActive,
		}, remoteIP(r.RemoteAddr))
		if writeAlarmError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, rule)
	}
}

func deleteAlarmRuleHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		if writeAlarmError(w, service.DeleteAlarmRule(r.Context(), principal, r.PathValue("plantId"), r.PathValue("ruleId"), remoteIP(r.RemoteAddr))) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func listAlarmEventsHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		events, err := service.ListAlarmEvents(r.Context(), principal, r.PathValue("plantId"), limit)
		if writeAlarmError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, events)
	}
}

func acknowledgeAlarmEventHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request acknowledgeAlarmEventRequest
		if !decodeJSON(w, r, &request, 4<<10) {
			return
		}
		event, err := service.AcknowledgeAlarmEvent(r.Context(), principal, r.PathValue("plantId"), r.PathValue("eventId"), request.Note, remoteIP(r.RemoteAddr))
		if writeAlarmError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, event)
	}
}

func writeAlarmError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, core.ErrAlarmRuleInvalid):
		http.Error(w, "invalid alarm rule data", http.StatusBadRequest)
	case errors.Is(err, core.ErrForbidden):
		http.Error(w, "permission denied", http.StatusForbidden)
	case errors.Is(err, core.ErrNotFound), errors.Is(err, core.ErrAlarmRuleNotFound):
		http.Error(w, "plant or alarm rule not found", http.StatusNotFound)
	case errors.Is(err, core.ErrAlarmEventNotFound):
		http.Error(w, "alarm event not found", http.StatusNotFound)
	default:
		log.Printf("alarm write failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
	return true
}
