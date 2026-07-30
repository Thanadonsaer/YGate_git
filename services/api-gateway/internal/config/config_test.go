package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("GATEWAY_HTTP_ADDR", "")
	t.Setenv("GATEWAY_PLATFORM_URL", "")
	t.Setenv("GATEWAY_ALLOWED_ORIGINS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "127.0.0.1:44440" || cfg.PlatformURL.String() != "http://127.0.0.1:44441" || len(cfg.AllowedOrigins) != 2 {
		t.Fatalf("config=%+v", cfg)
	}
}

func TestLoadRejectsInvalidPlatformURL(t *testing.T) {
	t.Setenv("GATEWAY_PLATFORM_URL", "postgres://localhost/platform")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid platform URL error")
	}
}
