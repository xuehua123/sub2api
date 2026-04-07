//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type settingPublicRepoStub struct {
	values map[string]string
}

func (s *settingPublicRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *settingPublicRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *settingPublicRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *settingPublicRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *settingPublicRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *settingPublicRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *settingPublicRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestSettingService_GetPublicSettings_ExposesRegistrationEmailSuffixWhitelist(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyRegistrationEnabled:              "true",
			SettingKeyEmailVerifyEnabled:               "true",
			SettingKeyRegistrationEmailSuffixWhitelist: `["@EXAMPLE.com"," @foo.bar ","@invalid_domain",""]`,
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"@example.com", "@foo.bar"}, settings.RegistrationEmailSuffixWhitelist)
}

func TestProvideSettingService_IncludesBuildVersionInInjectedSettings(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeySiteName: "Sub2API",
		},
	}

	svc := ProvideSettingService(repo, nil, &config.Config{}, BuildInfo{Version: "0.1.108"})

	payload, err := svc.GetPublicSettingsForInjection(context.Background())
	require.NoError(t, err)

	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, "0.1.108", decoded["version"])
}

func TestSettingService_GetPublicSettings_ExposesLobeHubSettings(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyLobeHubEnabled:              "true",
			SettingKeyLobeHubChatURL:              "https://chat.example.com",
			SettingKeyLobeHubOIDCIssuer:           "https://api.example.com",
			SettingKeyLobeHubDefaultProvider:      "openai",
			SettingKeyLobeHubDefaultModel:         "gpt-4.1",
			SettingKeyLobeHubRuntimeConfigVersion: "2026-04-07",
			SettingKeyHideLobeHubImportButton:     "true",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.LobeHubEnabled)
	require.Equal(t, "https://chat.example.com", settings.LobeHubChatURL)
	require.Equal(t, "https://api.example.com", settings.LobeHubOIDCIssuer)
	require.Equal(t, "openai", settings.LobeHubDefaultProvider)
	require.Equal(t, "gpt-4.1", settings.LobeHubDefaultModel)
	require.Equal(t, "2026-04-07", settings.LobeHubRuntimeConfigVersion)
	require.True(t, settings.HideLobeHubImportButton)
}
