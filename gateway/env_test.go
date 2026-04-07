package gateway

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfigFromEnvAppliesDefaults(t *testing.T) {
	t.Setenv("LOBEHUB_UPSTREAM_URL", "http://localhost:3210")
	t.Setenv("SUB2API_API_BASE_URL", "https://api.example.com/api/v1")
	t.Setenv("SUB2API_FRONTEND_URL", "https://app.example.com")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv returned error: %v", err)
	}

	if cfg.ProviderID != defaultProviderID {
		t.Fatalf("expected default provider %s, got %s", defaultProviderID, cfg.ProviderID)
	}
	if cfg.BootstrapPath != defaultBootstrapPath {
		t.Fatalf("expected bootstrap path %s, got %s", defaultBootstrapPath, cfg.BootstrapPath)
	}
	if cfg.PublicSettingsCacheTTL != defaultSettingsCacheTTL {
		t.Fatalf("expected cache ttl %s, got %s", defaultSettingsCacheTTL, cfg.PublicSettingsCacheTTL)
	}
}

func TestLoadConfigFromEnvParsesExplicitOverrides(t *testing.T) {
	t.Setenv("LOBEHUB_UPSTREAM_URL", "http://localhost:3210")
	t.Setenv("SUB2API_API_BASE_URL", "https://api.example.com/api/v1")
	t.Setenv("SUB2API_FRONTEND_URL", "https://app.example.com")
	t.Setenv("LOBEHUB_PROVIDER_ID", "custom-provider")
	t.Setenv("LOBEHUB_BOOTSTRAP_PATH", "/bootstrap")
	t.Setenv("PUBLIC_SETTINGS_CACHE_TTL", "45s")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv returned error: %v", err)
	}

	if cfg.ProviderID != "custom-provider" {
		t.Fatalf("expected custom provider, got %s", cfg.ProviderID)
	}
	if cfg.BootstrapPath != "/bootstrap" {
		t.Fatalf("expected custom bootstrap path, got %s", cfg.BootstrapPath)
	}
	if cfg.PublicSettingsCacheTTL != 45*time.Second {
		t.Fatalf("expected 45s cache ttl, got %s", cfg.PublicSettingsCacheTTL)
	}
}

func TestLoadConfigFromEnvParsesUserStatePathOverride(t *testing.T) {
	t.Setenv("LOBEHUB_UPSTREAM_URL", "http://localhost:3210")
	t.Setenv("SUB2API_API_BASE_URL", "https://api.example.com/api/v1")
	t.Setenv("SUB2API_FRONTEND_URL", "https://app.example.com")
	t.Setenv("LOBEHUB_USER_STATE_PATH", "/trpc/custom/userState")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv returned error: %v", err)
	}

	if cfg.UserStatePath != "/trpc/custom/userState" {
		t.Fatalf("expected custom user state path, got %s", cfg.UserStatePath)
	}
}

func TestLoadConfigFromEnvReturnsErrorWhenRequiredValueMissing(t *testing.T) {
	for _, key := range []string{
		"LOBEHUB_UPSTREAM_URL",
		"SUB2API_API_BASE_URL",
		"SUB2API_FRONTEND_URL",
	} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("LOBEHUB_UPSTREAM_URL", "http://localhost:3210")
			t.Setenv("SUB2API_API_BASE_URL", "https://api.example.com/api/v1")
			t.Setenv("SUB2API_FRONTEND_URL", "https://app.example.com")
			if err := os.Unsetenv(key); err != nil {
				t.Fatalf("Unsetenv failed: %v", err)
			}

			if _, err := LoadConfigFromEnv(); err == nil {
				t.Fatalf("expected error when %s is missing", key)
			}
		})
	}
}
