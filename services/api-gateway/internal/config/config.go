package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	ListenAddr     string
	PlatformURL    *url.URL
	AuthServiceURL *url.URL
	AllowedOrigins []string
}

func Load() (Config, error) {
	listenAddr := strings.TrimSpace(os.Getenv("GATEWAY_HTTP_ADDR"))
	if listenAddr == "" {
		listenAddr = "127.0.0.1:44440"
	}
	platformURL, err := parseUpstreamURL("GATEWAY_PLATFORM_URL", "http://127.0.0.1:44441")
	if err != nil {
		return Config{}, err
	}
	authServiceURL, err := parseUpstreamURL("GATEWAY_AUTH_SERVICE_URL", "http://127.0.0.1:44442")
	if err != nil {
		return Config{}, err
	}
	return Config{
		ListenAddr: listenAddr, PlatformURL: platformURL, AuthServiceURL: authServiceURL,
		AllowedOrigins: csvEnv("GATEWAY_ALLOWED_ORIGINS", []string{
			"http://localhost:8080", "http://127.0.0.1:8080",
		}),
	}, nil
}

func parseUpstreamURL(name, fallback string) (*url.URL, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		value = fallback
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("%s must be an absolute HTTP URL", name)
	}
	return parsed, nil
}

func csvEnv(name string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	result := make([]string, 0)
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}
