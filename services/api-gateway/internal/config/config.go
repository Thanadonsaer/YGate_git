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
	AllowedOrigins []string
}

func Load() (Config, error) {
	listenAddr := strings.TrimSpace(os.Getenv("GATEWAY_HTTP_ADDR"))
	if listenAddr == "" {
		listenAddr = "127.0.0.1:44440"
	}
	platformValue := strings.TrimSpace(os.Getenv("GATEWAY_PLATFORM_URL"))
	if platformValue == "" {
		platformValue = "http://127.0.0.1:44441"
	}
	platformURL, err := url.Parse(platformValue)
	if err != nil || (platformURL.Scheme != "http" && platformURL.Scheme != "https") || platformURL.Host == "" {
		return Config{}, fmt.Errorf("GATEWAY_PLATFORM_URL must be an absolute HTTP URL")
	}
	return Config{
		ListenAddr: listenAddr, PlatformURL: platformURL,
		AllowedOrigins: csvEnv("GATEWAY_ALLOWED_ORIGINS", []string{
			"http://localhost:8080", "http://127.0.0.1:8080",
		}),
	}, nil
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
