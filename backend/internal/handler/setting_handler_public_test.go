//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type settingHandlerPublicRepoStub struct {
	values map[string]string
}

func (s *settingHandlerPublicRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	panic("unexpected Get call")
}

func (s *settingHandlerPublicRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *settingHandlerPublicRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *settingHandlerPublicRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *settingHandlerPublicRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *settingHandlerPublicRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *settingHandlerPublicRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestSettingHandler_GetPublicSettings_ExposesForceEmailOnThirdPartySignup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &settingHandlerPublicRepoStub{
		values: map[string]string{
			service.SettingKeyForceEmailOnThirdPartySignup: "true",
		},
	}
	h := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			ForceEmailOnThirdPartySignup bool `json:"force_email_on_third_party_signup"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.True(t, resp.Data.ForceEmailOnThirdPartySignup)
}

func TestSettingHandler_GetPublicSettings_ExposesWeChatOAuthModeCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewSettingHandler(service.NewSettingService(&settingHandlerPublicRepoStub{
		values: map[string]string{
			service.SettingKeyWeChatConnectEnabled:             "true",
			service.SettingKeyWeChatConnectAppID:               "wx-mp-app",
			service.SettingKeyWeChatConnectAppSecret:           "wx-mp-secret",
			service.SettingKeyWeChatConnectMode:                "mp",
			service.SettingKeyWeChatConnectScopes:              "snsapi_base",
			service.SettingKeyWeChatConnectOpenEnabled:         "true",
			service.SettingKeyWeChatConnectMPEnabled:           "true",
			service.SettingKeyWeChatConnectRedirectURL:         "https://api.example.com/api/v1/auth/oauth/wechat/callback",
			service.SettingKeyWeChatConnectFrontendRedirectURL: "/auth/wechat/callback",
		},
	}, &config.Config{}), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			WeChatOAuthEnabled     bool `json:"wechat_oauth_enabled"`
			WeChatOAuthOpenEnabled bool `json:"wechat_oauth_open_enabled"`
			WeChatOAuthMPEnabled   bool `json:"wechat_oauth_mp_enabled"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.True(t, resp.Data.WeChatOAuthEnabled)
	require.True(t, resp.Data.WeChatOAuthOpenEnabled)
	require.True(t, resp.Data.WeChatOAuthMPEnabled)
}

func TestSettingHandler_EdgeProbeReturnsFixedNoStorePayloadWithoutSettingsLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewSettingHandler(service.NewSettingService(&panicSettingRepository{}, &config.Config{}), "test-version")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/.well-known/sub2api/edge-probe?nonce=test", nil)

	h.EdgeProbe(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
	require.Equal(t, "no-store, max-age=0", recorder.Header().Get("Cache-Control"))
	require.Empty(t, recorder.Header().Values("Set-Cookie"))
	require.LessOrEqual(t, recorder.Body.Len(), 128)
	require.JSONEq(t, `{"ok":true,"client_ip":null}`, recorder.Body.String())
}

func TestSettingHandler_EdgeProbeReturnsVerifiedClientIPWhenExplicitlyEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &settingHandlerPublicRepoStub{values: map[string]string{
		service.SettingKeyConnectivityTestEnabled:         "true",
		service.SettingKeyConnectivityClientIPEnabled:     "true",
		service.SettingKeyConnectivityProbeAllowedOrigins: `["https://8.8.8.8"]`,
		service.SettingKeyAPIBaseURL:                      "https://8.8.8.8/v1",
	}}
	settingService := service.NewSettingService(repo, &config.Config{
		Server: config.ServerConfig{
			TrustedProxiesConfigured: true,
			TrustedProxies:           []string{"203.0.113.0/24"},
		},
		Connectivity: config.ConnectivityConfig{
			ClientIPDeniedCIDRs: []string{"9.9.9.0/24"},
			ClientIPMaxHops:     8,
		},
	})
	require.NoError(t, settingService.LoadConnectivityProbeSettings(context.Background()))

	h := NewSettingHandler(settingService, "test-version")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/.well-known/sub2api/edge-probe?nonce=test", nil)
	c.Request.RemoteAddr = "203.0.113.10:443"
	c.Request.Header.Set("X-Forwarded-For", "8.8.4.4")

	h.EdgeProbe(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"ok":true,"client_ip":"8.8.4.4"}`, recorder.Body.String())
}

type panicSettingRepository struct{}

func (*panicSettingRepository) Get(context.Context, string) (*service.Setting, error) {
	panic("edge probe must not read settings")
}

func (*panicSettingRepository) GetValue(context.Context, string) (string, error) {
	panic("edge probe must not read settings")
}

func (*panicSettingRepository) Set(context.Context, string, string) error {
	panic("edge probe must not write settings")
}

func (*panicSettingRepository) GetMultiple(context.Context, []string) (map[string]string, error) {
	panic("edge probe must not read settings")
}

func (*panicSettingRepository) SetMultiple(context.Context, map[string]string) error {
	panic("edge probe must not write settings")
}

func (*panicSettingRepository) GetAll(context.Context) (map[string]string, error) {
	panic("edge probe must not read settings")
}

func (*panicSettingRepository) Delete(context.Context, string) error {
	panic("edge probe must not write settings")
}
