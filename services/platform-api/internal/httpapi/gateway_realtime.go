package httpapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/core"
	"ygate/platform-api/internal/gatewayhub"
	"ygate/platform-api/internal/ingestion"
	"ygate/platform-api/internal/telemetrypull"
)

// gatewayEnvelope is the generic shape of every message on the gateway
// realtime socket; unused fields for a given type are simply left zero.
type gatewayEnvelope struct {
	Type            string `json:"type"`
	AppliedVersion  int64  `json:"appliedVersion,omitempty"`
	SoftwareVersion string `json:"softwareVersion,omitempty"`
	Version         int64  `json:"version,omitempty"`
	Status          string `json:"status,omitempty"`
	Reason          string `json:"reason,omitempty"`
	CommandID       string `json:"commandId,omitempty"`
	Phase           string `json:"phase,omitempty"`
	DownloadedBytes int64  `json:"downloadedBytes,omitempty"`
	TotalBytes      int64  `json:"totalBytes,omitempty"`
}

// gatewayRealtimeHandler accepts the outbound-initiated WebSocket connection
// from a modbus-api-middleware instance. Auth is the same X-Api-Key/
// middleware_client lookup the ingestion endpoints already use -- this is a
// machine client, not a browser session, so it does not go through
// authenticated()/session cookies like realtimeHandler does.
//
// Exactly one goroutine (this handler's own for-select loop) ever calls
// connection.Write: heartbeats, hub-relayed config/command pushes, and the
// hello catch-up push are all funneled through that single loop. A second,
// read-only goroutine only calls connection.Read. Concurrent writes from
// two goroutines on the same coder/websocket connection are not safe, so
// this split must be preserved.
func gatewayRealtimeHandler(ingestionService *ingestion.Service, registryService *core.Service, hub *gatewayhub.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, err := ingestionService.Authenticate(r.Context(), r.Header.Get("X-Api-Key"))
		if err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
		if err != nil {
			return
		}
		connection.SetReadLimit(8 << 20) // 8MiB -- config-export can carry a full legacy register catalog (~1450 addresses, ~545KB observed)
		defer connection.CloseNow()
		ctx := r.Context()
		gatewayID := gatewayIDString(client.ID)

		out, resolve, unregister := hub.Register(gatewayID)
		defer unregister()

		go telemetrypull.Run(ctx, hub, ingestionService, client, gatewayID)

		incoming := make(chan []byte, 8)
		readDone := make(chan struct{})
		go func() {
			defer close(readDone)
			for {
				_, data, err := connection.Read(ctx)
				if err != nil {
					return
				}
				select {
				case incoming <- data:
				case <-ctx.Done():
					return
				}
			}
		}()

		heartbeat := time.NewTicker(20 * time.Second)
		defer heartbeat.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-readDone:
				return
			case <-heartbeat.C:
				if err := connection.Write(ctx, websocket.MessageText, []byte(`{"type":"heartbeat"}`)); err != nil {
					return
				}
			case payload := <-out:
				if err := connection.Write(ctx, websocket.MessageText, payload); err != nil {
					return
				}
			case data := <-incoming:
				var envelope gatewayEnvelope
				if err := json.Unmarshal(data, &envelope); err != nil {
					continue
				}
				switch envelope.Type {
				case "hello":
					payload, shouldPush, err := registryService.HandleGatewayHello(ctx, client.ID, envelope.AppliedVersion, envelope.SoftwareVersion)
					if err != nil {
						log.Printf("gateway hello failed for %s: %v", gatewayID, err)
						continue
					}
					if shouldPush {
						if err := connection.Write(ctx, websocket.MessageText, payload); err != nil {
							return
						}
					}
				case "config.ack":
					if err := registryService.RecordConfigAck(ctx, client.ID, core.ConfigAckInput{
						Version: envelope.Version, Status: envelope.Status, Reason: envelope.Reason,
					}); err != nil {
						log.Printf("record config ack failed for %s: %v", gatewayID, err)
					}
				case "command.result":
					resolve(envelope.CommandID, json.RawMessage(data))
				case "command.progress":
					hub.Progress(gatewayID, envelope.CommandID, json.RawMessage(data))
				}
			}
		}
	}
}

func gatewayIDString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	b := id.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
