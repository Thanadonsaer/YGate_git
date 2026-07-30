package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/core"
)

type stubTelemetryReader struct {
	readings []core.LatestTelemetry
}

func (s stubTelemetryReader) LatestTelemetry(context.Context, auth.Principal, string) ([]core.LatestTelemetry, error) {
	return s.readings, nil
}

func TestRealtimeWebSocketAuthenticatesOriginAndSendsReady(t *testing.T) {
	var userID pgtype.UUID
	if err := userID.Scan("018f8318-4c72-7ad4-9f57-2894d89cb123"); err != nil {
		t.Fatal(err)
	}
	principal := auth.Principal{UserID: userID}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		realtimeHandler([]string{"web.example.com"}, nil, nil)(w, r, principal)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"https://web.example.com"}},
	})
	if err != nil {
		t.Fatalf("dial status=%v err=%v", response, err)
	}
	defer connection.CloseNow()
	var message realtimeMessage
	if err = wsjson.Read(ctx, connection, &message); err != nil {
		t.Fatal(err)
	}
	if message.Type != "connection.ready" || message.UserID != "018f8318-4c72-7ad4-9f57-2894d89cb123" || message.SentAt.IsZero() {
		t.Fatalf("message=%+v", message)
	}
}

func TestRealtimeWebSocketRejectsUnknownOrigin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		realtimeHandler([]string{"web.example.com"}, nil, nil)(w, r, auth.Principal{})
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"https://other.example.com"}},
	})
	if connection != nil {
		connection.CloseNow()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("response=%v err=%v", response, err)
	}
}

func TestRealtimeWebSocketSendsPlantTelemetrySnapshot(t *testing.T) {
	var userID pgtype.UUID
	if err := userID.Scan("018f8318-4c72-7ad4-9f57-2894d89cb123"); err != nil {
		t.Fatal(err)
	}
	reader := stubTelemetryReader{readings: []core.LatestTelemetry{{
		ID: "reading-1", PlantID: "plant-1", DeviceID: "device-1",
		DataItemMap: map[string]float64{"active_power": 42.5}, ParameterCount: 1,
	}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		realtimeHandler([]string{"web.example.com"}, reader, nil)(w, r, auth.Principal{UserID: userID})
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"?plantId=plant-1", &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"https://web.example.com"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	var ready realtimeMessage
	if err = wsjson.Read(ctx, connection, &ready); err != nil || ready.Type != "connection.ready" {
		t.Fatalf("ready=%+v err=%v", ready, err)
	}
	var snapshot realtimeTelemetryMessage
	if err = wsjson.Read(ctx, connection, &snapshot); err != nil || snapshot.Type != "telemetry.snapshot" || snapshot.PlantID != "plant-1" || len(snapshot.Data) != 1 || snapshot.Data[0].DataItemMap["active_power"] != 42.5 {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
}
