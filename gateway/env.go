package gateway

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const defaultListenAddr = ":8080"

func LoadConfigFromEnv() (Config, error) {
	cfg := Config{
		ListenAddr:         getenvDefault("LISTEN_ADDR", defaultListenAddr),
		UpstreamURL:        os.Getenv("LOBEHUB_UPSTREAM_URL"),
		Sub2APIAPIBaseURL:  os.Getenv("SUB2API_API_BASE_URL"),
		Sub2APIFrontendURL: os.Getenv("SUB2API_FRONTEND_URL"),
		ProviderID:         getenvDefault("LOBEHUB_PROVIDER_ID", defaultProviderID),
		BootstrapPath:      getenvDefault("LOBEHUB_BOOTSTRAP_PATH", defaultBootstrapPath),
		UserStatePath:      getenvDefault("LOBEHUB_USER_STATE_PATH", defaultUserStatePath),
	}

	cacheTTL := strings.TrimSpace(os.Getenv("PUBLIC_SETTINGS_CACHE_TTL"))
	if cacheTTL == "" {
		cfg.PublicSettingsCacheTTL = defaultSettingsCacheTTL
	} else {
		ttl, err := time.ParseDuration(cacheTTL)
		if err != nil {
			return Config{}, fmt.Errorf("PUBLIC_SETTINGS_CACHE_TTL: %w", err)
		}
		cfg.PublicSettingsCacheTTL = ttl
	}

	for _, required := range []struct {
		name  string
		value string
	}{
		{name: "LOBEHUB_UPSTREAM_URL", value: cfg.UpstreamURL},
		{name: "SUB2API_API_BASE_URL", value: cfg.Sub2APIAPIBaseURL},
		{name: "SUB2API_FRONTEND_URL", value: cfg.Sub2APIFrontendURL},
	} {
		if strings.TrimSpace(required.value) == "" {
			return Config{}, fmt.Errorf("%s is required", required.name)
		}
	}

	return cfg, nil
}

func getenvDefault(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}
