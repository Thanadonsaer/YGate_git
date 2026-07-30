package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
)

type realtimeMessage struct {
	Type   string    `json:"type"`
	SentAt time.Time `json:"sentAt"`
	UserID string    `json:"userId"`
}

type realtimeTelemetryMessage struct {
	Type    string                 `json:"type"`
	SentAt  time.Time              `json:"sentAt"`
	UserID  string                 `json:"userId"`
	PlantID string                 `json:"plantId"`
	Data    []core.LatestTelemetry `json:"data"`
}

type realtimeAlarmMessage struct {
	Type    string            `json:"type"`
	SentAt  time.Time         `json:"sentAt"`
	UserID  string            `json:"userId"`
	PlantID string            `json:"plantId"`
	Data    []core.AlarmEvent `json:"data"`
}

var realtimeTelemetryInterval = 2 * time.Second
var realtimeAlarmInterval = 3 * time.Second

func realtimeHandler(originPatterns []string, telemetry latestTelemetryReader, alarms alarmEventReader) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		plantID := strings.TrimSpace(r.URL.Query().Get("plantId"))
		var initial []core.LatestTelemetry
		var err error
		if plantID != "" {
			if telemetry == nil {
				http.Error(w, "telemetry is unavailable", http.StatusServiceUnavailable)
				return
			}
			initial, err = telemetry.LatestTelemetry(r.Context(), principal, plantID)
			if errors.Is(err, core.ErrNotFound) {
				http.Error(w, "plant not found", http.StatusNotFound)
				return
			}
			if err != nil {
				log.Printf("realtime telemetry authorization failed: %v", err)
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
		}
		connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: originPatterns})
		if err != nil {
			return
		}
		defer connection.CloseNow()

		ctx := connection.CloseRead(r.Context())
		if err = wsjson.Write(ctx, connection, realtimeMessage{
			Type: "connection.ready", SentAt: time.Now().UTC(), UserID: principal.User().ID,
		}); err != nil {
			return
		}
		var lastSnapshot []byte
		if plantID != "" {
			lastSnapshot, _ = json.Marshal(initial)
			if err = wsjson.Write(ctx, connection, realtimeTelemetryMessage{
				Type: "telemetry.snapshot", SentAt: time.Now().UTC(), UserID: principal.User().ID,
				PlantID: plantID, Data: initial,
			}); err != nil {
				return
			}
		}
		var lastAlarmEventID int64
		if plantID != "" && alarms != nil {
			if lastAlarmEventID, err = alarms.LatestAlarmEventID(r.Context(), principal, plantID); err != nil {
				log.Printf("realtime alarm baseline failed: %v", err)
				_ = connection.Close(websocket.StatusInternalError, "alarms unavailable")
				return
			}
		}

		heartbeat := time.NewTicker(20 * time.Second)
		defer heartbeat.Stop()
		var telemetryTick <-chan time.Time
		if plantID != "" {
			ticker := time.NewTicker(realtimeTelemetryInterval)
			defer ticker.Stop()
			telemetryTick = ticker.C
		}
		var alarmTick <-chan time.Time
		if plantID != "" && alarms != nil {
			ticker := time.NewTicker(realtimeAlarmInterval)
			defer ticker.Stop()
			alarmTick = ticker.C
		}
		for {
			select {
			case <-ctx.Done():
				return
			case sentAt := <-heartbeat.C:
				if err = wsjson.Write(ctx, connection, realtimeMessage{
					Type: "connection.heartbeat", SentAt: sentAt.UTC(), UserID: principal.User().ID,
				}); err != nil {
					return
				}
			case sentAt := <-telemetryTick:
				readings, readErr := telemetry.LatestTelemetry(ctx, principal, plantID)
				if readErr != nil {
					_ = connection.Close(websocket.StatusInternalError, "telemetry unavailable")
					return
				}
				snapshot, _ := json.Marshal(readings)
				if bytes.Equal(snapshot, lastSnapshot) {
					continue
				}
				lastSnapshot = snapshot
				if err = wsjson.Write(ctx, connection, realtimeTelemetryMessage{
					Type: "telemetry.snapshot", SentAt: sentAt.UTC(), UserID: principal.User().ID,
					PlantID: plantID, Data: readings,
				}); err != nil {
					return
				}
			case sentAt := <-alarmTick:
				events, readErr := alarms.AlarmEventsSince(ctx, principal, plantID, lastAlarmEventID)
				if readErr != nil {
					_ = connection.Close(websocket.StatusInternalError, "alarms unavailable")
					return
				}
				if len(events) == 0 {
					continue
				}
				lastAlarmEventID = events[len(events)-1].ID
				if err = wsjson.Write(ctx, connection, realtimeAlarmMessage{
					Type: "alarm.event", SentAt: sentAt.UTC(), UserID: principal.User().ID,
					PlantID: plantID, Data: events,
				}); err != nil {
					return
				}
			}
		}
	}
}
