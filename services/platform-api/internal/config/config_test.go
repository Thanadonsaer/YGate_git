package config

import (
	"net/url"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Setenv("PLATFORM_HTTP_ADDR", "")
	t.Setenv("PLATFORM_DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing database URL error")
	}

	t.Setenv("PLATFORM_DATABASE_URL", "postgres://platform@localhost/platform")
	t.Setenv("PLATFORM_SESSION_IDLE_TIMEOUT", "15m")
	t.Setenv("PLATFORM_SESSION_ABSOLUTE_TIMEOUT", "8h")
	t.Setenv("PLATFORM_COOKIE_SECURE", "false")
	t.Setenv("PLATFORM_PASSWORD_RESET_TTL", "20m")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "127.0.0.1:44441" || cfg.DatabaseURL != "postgres://platform@localhost/platform" || cfg.SessionIdleTimeout != 15*time.Minute || cfg.SessionAbsoluteTimeout != 8*time.Hour || cfg.PasswordResetTTL != 20*time.Minute || cfg.CookieSecure {
		t.Fatalf("config=%+v", cfg)
	}
}

func TestLoadRejectsIdleTimeoutAfterAbsoluteTimeout(t *testing.T) {
	t.Setenv("PLATFORM_DATABASE_URL", "postgres://platform@localhost/platform")
	t.Setenv("PLATFORM_SESSION_IDLE_TIMEOUT", "2h")
	t.Setenv("PLATFORM_SESSION_ABSOLUTE_TIMEOUT", "1h")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid session timeout error")
	}
}

func TestLoadValidatesSMTPRecoveryConfiguration(t *testing.T) {
	t.Setenv("PLATFORM_DATABASE_URL", "postgres://platform@localhost/platform")
	t.Setenv("PLATFORM_SMTP_ADDR", "smtp.example.com:587")
	if _, err := Load(); err == nil {
		t.Fatal("expected incomplete SMTP configuration error")
	}

	t.Setenv("PLATFORM_SMTP_FROM", "scada@example.com")
	t.Setenv("PLATFORM_PASSWORD_RESET_URL", "https://scada.example.com/reset-password")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SMTPAddr != "smtp.example.com:587" || cfg.PasswordResetTTL != 30*time.Minute {
		t.Fatalf("config=%+v", cfg)
	}
}

func TestLoadAcceptsDatabaseURLAndNormalizesSchemaParameter(t *testing.T) {
	t.Setenv("PLATFORM_DATABASE_URL", "")
	t.Setenv("DATABASE_URL", "postgresql://postgres:encoded%40password@localhost:5432/ygate_db?schema=public")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "127.0.0.1:44441" || parsed.Query().Get("schema") != "" || parsed.Query().Get("search_path") != "public" {
		t.Fatalf("config=%+v", cfg)
	}
	password, _ := parsed.User.Password()
	if password != "encoded@password" {
		t.Fatal("encoded database password was not preserved")
	}
}
