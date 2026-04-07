//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type settingGetAllRepoStub struct {
	values map[string]string
}

func (s *settingGetAllRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *settingGetAllRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *settingGetAllRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *settingGetAllRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
}

func (s *settingGetAllRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *settingGetAllRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
	}
	return out, nil
}

func (s *settingGetAllRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestSettingService_GetAllSettings_IncludesLobeHubOIDCClientID(t *testing.T) {
	repo := &settingGetAllRepoStub{
		values: map[string]string{
			SettingKeyLobeHubEnabled:          "true",
			SettingKeyLobeHubChatURL:          "https://chat.example.com",
			SettingKeyLobeHubOIDCIssuer:       "https://api.example.com/api/v1/lobehub/oidc",
			SettingKeyLobeHubOIDCClientID:     "lobehub-client",
			SettingKeyLobeHubOIDCClientSecret: "lobehub-secret",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	require.NotNil(t, settings)
	require.True(t, settings.LobeHubEnabled)
	require.Equal(t, "https://chat.example.com", settings.LobeHubChatURL)
	require.Equal(t, "https://api.example.com/api/v1/lobehub/oidc", settings.LobeHubOIDCIssuer)
	require.Equal(t, "lobehub-client", settings.LobeHubOIDCClientID)
	require.True(t, settings.LobeHubOIDCClientSecretConfigured)
}
