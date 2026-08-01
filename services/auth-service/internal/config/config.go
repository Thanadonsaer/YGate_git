package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr             string
	DatabaseURL            string
	SessionIdleTimeout     time.Duration
	SessionAbsoluteTimeout time.Duration
	CookieSecure           bool
	PasswordResetTTL       time.Duration
	PasswordResetURL       string
	SMTPAddr               string
	SMTPFrom               string
	SMTPUsername           string
	SMTPPassword           string
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddr:   strings.TrimSpace(os.Getenv("AUTH_HTTP_ADDR")),
		DatabaseURL:  strings.TrimSpace(os.Getenv("AUTH_DATABASE_URL")),
		CookieSecure: true,
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:44442"
	}
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("AUTH_DATABASE_URL or DATABASE_URL is required")
	}
	var err error
	if cfg.DatabaseURL, err = normalizeDatabaseURL(cfg.DatabaseURL); err != nil {
		return Config{}, err
	}
	if cfg.SessionIdleTimeout, err = durationEnv("AUTH_SESSION_IDLE_TIMEOUT", 30*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.SessionAbsoluteTimeout, err = durationEnv("AUTH_SESSION_ABSOLUTE_TIMEOUT", 24*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.SessionIdleTimeout > cfg.SessionAbsoluteTimeout {
		return Config{}, fmt.Errorf("AUTH_SESSION_IDLE_TIMEOUT must not exceed AUTH_SESSION_ABSOLUTE_TIMEOUT")
	}
	if cfg.PasswordResetTTL, err = durationEnv("AUTH_PASSWORD_RESET_TTL", 30*time.Minute); err != nil {
		return Config{}, err
	}
	cfg.PasswordResetURL = strings.TrimSpace(os.Getenv("AUTH_PASSWORD_RESET_URL"))
	cfg.SMTPAddr = strings.TrimSpace(os.Getenv("AUTH_SMTP_ADDR"))
	cfg.SMTPFrom = strings.TrimSpace(os.Getenv("AUTH_SMTP_FROM"))
	cfg.SMTPUsername = strings.TrimSpace(os.Getenv("AUTH_SMTP_USERNAME"))
	cfg.SMTPPassword = os.Getenv("AUTH_SMTP_PASSWORD")
	if cfg.SMTPAddr != "" && (cfg.SMTPFrom == "" || cfg.PasswordResetURL == "") {
		return Config{}, fmt.Errorf("AUTH_SMTP_FROM and AUTH_PASSWORD_RESET_URL are required when AUTH_SMTP_ADDR is set")
	}
	if value := strings.TrimSpace(os.Getenv("AUTH_COOKIE_SECURE")); value != "" {
		if cfg.CookieSecure, err = strconv.ParseBool(value); err != nil {
			return Config{}, fmt.Errorf("AUTH_COOKIE_SECURE: %w", err)
		}
	}
	return cfg, nil
}

func normalizeDatabaseURL(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return "", fmt.Errorf("database URL must be a valid postgres URL")
	}
	query := parsed.Query()
	if schema := query.Get("schema"); schema != "" {
		if query.Get("search_path") == "" {
			query.Set("search_path", schema)
		}
		query.Del("schema")
		parsed.RawQuery = query.Encode()
	}
	return parsed.String(), nil
}

func durationEnv(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return duration, nil
}
