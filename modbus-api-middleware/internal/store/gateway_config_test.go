package store

import (
	"path/filepath"
	"testing"

	"chpp/modbus-api-middleware/internal/domain"
)

func TestGatewayConfigRoundTrip(t *testing.T) {
	s, err := OpenNormalized(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	saved, err := s.SaveGatewayConfig(domain.GatewayConfig{GatewayID: " MOXA-VT1-01 ", Endpoint: " http://127.0.0.1:3000/api/middleware/readings ", APIKey: " secret ", APIPollingEnabled: true, SendTimeoutSeconds: 12})
	if err != nil {
		t.Fatal(err)
	}
	if saved.GatewayID != "MOXA-VT1-01" || saved.APIKey != "secret" {
		t.Fatalf("saved=%+v", saved)
	}
	got, err := s.GatewayConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Endpoint != "http://127.0.0.1:3000/api/middleware/readings" || got.APIKey != "secret" || !got.APIPollingEnabled || got.SendTimeoutSeconds != 12 {
		t.Fatalf("got=%+v", got)
	}
}

func TestGatewayConfigDefaultsAPIPollingDisabled(t *testing.T) {
	s, err := OpenNormalized(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	got, err := s.GatewayConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.APIPollingEnabled {
		t.Fatalf("api polling enabled by default: %+v", got)
	}
}

func TestGatewayConfigRejectsBadEndpoint(t *testing.T) {
	s, err := OpenNormalized(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err = s.SaveGatewayConfig(domain.GatewayConfig{Endpoint: "192.168.1.108/api"}); err == nil {
		t.Fatal("expected endpoint validation error")
	}
}
