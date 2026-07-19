package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestAccountFromServiceShallow_RedactsSensitiveCredentials(t *testing.T) {
	src := &service.Account{
		ID:       42,
		Name:     "demo",
		Platform: "anthropic",
		Type:     "oauth",
		Credentials: map[string]any{
			"access_token":  "at-secret",
			"refresh_token": "rt-secret",
			"id_token":      "id-secret",
			"api_key":       "sk-secret",
			"base_url":      "https://api.example.com",
			"model_mapping": map[string]any{"foo": "bar"},
		},
	}

	got := AccountFromServiceShallow(src)
	require.NotNil(t, got)

	// 敏感键不在 Credentials 里
	require.NotContains(t, got.Credentials, "access_token")
	require.NotContains(t, got.Credentials, "refresh_token")
	require.NotContains(t, got.Credentials, "id_token")
	require.NotContains(t, got.Credentials, "api_key")
	// 非敏感键保留
	require.Equal(t, "https://api.example.com", got.Credentials["base_url"])
	require.Equal(t, map[string]any{"foo": "bar"}, got.Credentials["model_mapping"])

	// 状态 map 标记敏感键存在
	require.True(t, got.CredentialsStatus["has_access_token"])
	require.True(t, got.CredentialsStatus["has_refresh_token"])
	require.True(t, got.CredentialsStatus["has_id_token"])
	require.True(t, got.CredentialsStatus["has_api_key"])

	// JSON 序列化校验：响应体里不会出现敏感子串
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "rt-secret")
	require.NotContains(t, string(raw), "at-secret")
	require.NotContains(t, string(raw), "sk-secret")
	require.NotContains(t, string(raw), "id-secret")
	// 状态标识应序列化进 JSON
	require.Contains(t, string(raw), "credentials_status")
	require.Contains(t, string(raw), "has_refresh_token")

	// 原始 service.Account 不应被改动
	require.Equal(t, "rt-secret", src.Credentials["refresh_token"])
}

func TestAccountFromServiceShallow_NilCredentialsOmitsStatus(t *testing.T) {
	src := &service.Account{ID: 1, Name: "n", Platform: "anthropic", Type: "oauth"}
	got := AccountFromServiceShallow(src)
	require.NotNil(t, got)
	require.Nil(t, got.Credentials)
	require.Nil(t, got.CredentialsStatus)
}

func TestAccountFromServiceShallow_HidesLegacyUpstreamProbeState(t *testing.T) {
	src := &service.Account{
		ID:   7,
		Name: "legacy upstream",
		Credentials: map[string]any{
			"upstream_management_auth":     "encrypted-secret",
			"upstream_management_base_url": "https://legacy.example.com",
			"base_url":                     "https://api.example.com",
		},
		Extra: map[string]any{
			"balance_probe_status":                  "ok",
			"upstream_billing_probe_enabled":        true,
			"upstream_rate_multiplier_sync_enabled": true,
			"unrelated":                             "kept",
		},
	}

	got := AccountFromServiceShallow(src)
	require.NotNil(t, got)
	require.NotContains(t, got.Credentials, "upstream_management_auth")
	require.NotContains(t, got.Credentials, "upstream_management_base_url")
	require.NotContains(t, got.CredentialsStatus, "has_upstream_management_auth")
	require.Equal(t, map[string]any{"unrelated": "kept"}, got.Extra)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "encrypted-secret")
	require.NotContains(t, string(raw), "upstream_management")
	require.NotContains(t, string(raw), "balance_probe")
	require.NotContains(t, string(raw), "upstream_billing_probe")
	require.NotContains(t, string(raw), "upstream_rate_multiplier_sync")
}
