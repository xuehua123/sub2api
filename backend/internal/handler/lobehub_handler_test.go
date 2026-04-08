package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type lobehubLaunchServiceStub struct {
	createResult   *service.LobeHubLaunchTicketResult
	createErr      error
	createUserID   int64
	createAPIKeyID int64
	buildResult    *service.LobeHubBridgePayload
	buildErr       error
	buildTicketID  string
}

type lobehubOIDCServiceStub struct {
	discoveryResult    *service.LobeHubOIDCDiscoveryDocument
	discoveryErr       error
	jwksResult         *service.LobeHubJWKSet
	jwksErr            error
	authorizeSessionID string
	authorizeRequest   *service.LobeHubOIDCAuthorizeRequest
	authorizeResult    *service.LobeHubOIDCAuthorizeResult
	authorizeErr       error
	tokenRequest       *service.LobeHubOIDCTokenRequest
	tokenResult        *service.LobeHubOIDCTokenResponse
	tokenErr           error
	userInfoToken      string
	userInfoResult     *service.LobeHubOIDCUserInfo
	userInfoErr        error
}

type lobehubSSOServiceStub struct {
	prepareRequest  *service.LobeHubSSOContinuationRequest
	prepareUserID   int64
	prepareResult   *service.LobeHubSSOContinuationResult
	prepareErr      error
	refreshRequest  *service.LobeHubSSORefreshRequest
	refreshUserID   int64
	refreshResult   *service.LobeHubSSOContinuationResult
	refreshErr      error
	exchangeToken   string
	exchangeReturn  string
	exchangeResult  *service.LobeHubBootstrapExchangeResult
	exchangeErr     error
	consumeTicketID string
	consumeResult   *service.LobeHubBootstrapConsumeResult
	consumeErr      error
	compareToken    string
	compareObserved *service.LobeHubObservedSettings
	compareResult   *service.LobeHubConfigProbeResult
	compareErr      error
}

func (s *lobehubLaunchServiceStub) CreateLaunchTicket(_ context.Context, userID, apiKeyID int64) (*service.LobeHubLaunchTicketResult, error) {
	s.createUserID = userID
	s.createAPIKeyID = apiKeyID
	return s.createResult, s.createErr
}

func (s *lobehubLaunchServiceStub) BuildBridgePayload(_ context.Context, ticketID string) (*service.LobeHubBridgePayload, error) {
	s.buildTicketID = ticketID
	return s.buildResult, s.buildErr
}

func (s *lobehubOIDCServiceStub) GetDiscoveryDocument(context.Context) (*service.LobeHubOIDCDiscoveryDocument, error) {
	return s.discoveryResult, s.discoveryErr
}

func (s *lobehubOIDCServiceStub) GetJWKS(context.Context) (*service.LobeHubJWKSet, error) {
	return s.jwksResult, s.jwksErr
}

func (s *lobehubOIDCServiceStub) Authorize(_ context.Context, webSessionID string, req *service.LobeHubOIDCAuthorizeRequest) (*service.LobeHubOIDCAuthorizeResult, error) {
	s.authorizeSessionID = webSessionID
	s.authorizeRequest = req
	return s.authorizeResult, s.authorizeErr
}

func (s *lobehubOIDCServiceStub) ExchangeAuthorizationCode(_ context.Context, req *service.LobeHubOIDCTokenRequest) (*service.LobeHubOIDCTokenResponse, error) {
	s.tokenRequest = req
	return s.tokenResult, s.tokenErr
}

func (s *lobehubOIDCServiceStub) GetUserInfo(_ context.Context, accessToken string) (*service.LobeHubOIDCUserInfo, error) {
	s.userInfoToken = accessToken
	return s.userInfoResult, s.userInfoErr
}

func (s *lobehubSSOServiceStub) PrepareOIDCContinuation(_ context.Context, userID int64, req *service.LobeHubSSOContinuationRequest) (*service.LobeHubSSOContinuationResult, error) {
	s.prepareUserID = userID
	s.prepareRequest = req
	return s.prepareResult, s.prepareErr
}

func (s *lobehubSSOServiceStub) PrepareTargetRefresh(_ context.Context, userID int64, req *service.LobeHubSSORefreshRequest) (*service.LobeHubSSOContinuationResult, error) {
	s.refreshUserID = userID
	s.refreshRequest = req
	return s.refreshResult, s.refreshErr
}

func (s *lobehubSSOServiceStub) ExchangeBootstrap(_ context.Context, targetToken string, returnURL string) (*service.LobeHubBootstrapExchangeResult, error) {
	s.exchangeToken = targetToken
	s.exchangeReturn = returnURL
	return s.exchangeResult, s.exchangeErr
}

func (s *lobehubSSOServiceStub) ConsumeBootstrap(_ context.Context, ticketID string) (*service.LobeHubBootstrapConsumeResult, error) {
	s.consumeTicketID = ticketID
	return s.consumeResult, s.consumeErr
}

func (s *lobehubSSOServiceStub) CompareCurrentConfig(_ context.Context, targetToken string, observed *service.LobeHubObservedSettings) (*service.LobeHubConfigProbeResult, error) {
	s.compareToken = targetToken
	s.compareObserved = observed
	return s.compareResult, s.compareErr
}

func TestLobeHubHandler_CreateLaunchTicket(t *testing.T) {
	gin.SetMode(gin.TestMode)

	stub := &lobehubLaunchServiceStub{
		createResult: &service.LobeHubLaunchTicketResult{
			TicketID:  "ticket-1",
			BridgeURL: "/api/v1/lobehub/bridge?ticket=ticket-1",
		},
	}
	h := NewLobeHubHandler(stub, &lobehubOIDCServiceStub{}, &lobehubSSOServiceStub{})

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/lobehub/launch-ticket", bytes.NewBufferString(`{"api_key_id":9}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})

	h.CreateLaunchTicket(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(42), stub.createUserID)
	require.Equal(t, int64(9), stub.createAPIKeyID)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			TicketID  string `json:"ticket_id"`
			BridgeURL string `json:"bridge_url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, "ticket-1", resp.Data.TicketID)
	require.Equal(t, "/api/v1/lobehub/bridge?ticket=ticket-1", resp.Data.BridgeURL)
}

func TestLobeHubHandler_CreateLaunchTicket_RequiresAuthSubject(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewLobeHubHandler(&lobehubLaunchServiceStub{}, &lobehubOIDCServiceStub{}, &lobehubSSOServiceStub{})

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/lobehub/launch-ticket", bytes.NewBufferString(`{"api_key_id":9}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.CreateLaunchTicket(c)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestLobeHubHandler_Bridge(t *testing.T) {
	gin.SetMode(gin.TestMode)

	stub := &lobehubLaunchServiceStub{
		buildResult: &service.LobeHubBridgePayload{
			ContinueURL:       "https://chat.example.com/",
			TargetToken:       "target-1",
			BootstrapTicketID: "bootstrap-1",
			CookieDomain:      ".example.com",
		},
	}
	h := NewLobeHubHandler(stub, &lobehubOIDCServiceStub{}, &lobehubSSOServiceStub{})

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/lobehub/bridge?ticket=ticket-1", nil)
	c.Request.Host = "api.example.com"
	c.Request.Header.Set("X-Forwarded-Proto", "https")

	h.Bridge(c)

	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "ticket-1", stub.buildTicketID)
	require.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	require.Equal(t, "https://chat.example.com/", rec.Header().Get("Location"))
	allCookies := strings.Join(rec.Header().Values("Set-Cookie"), "\n")
	require.Contains(t, allCookies, "sub2api_lobehub_target=target-1")
	require.Contains(t, allCookies, "sub2api_lobehub_bootstrap=bootstrap-1")
	require.Contains(t, allCookies, "Domain=example.com")
}

func TestLobeHubHandler_Bridge_RequiresTicket(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewLobeHubHandler(&lobehubLaunchServiceStub{}, &lobehubOIDCServiceStub{}, &lobehubSSOServiceStub{})

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/lobehub/bridge", nil)

	h.Bridge(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestLobeHubHandler_Discovery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oidcStub := &lobehubOIDCServiceStub{
		discoveryResult: &service.LobeHubOIDCDiscoveryDocument{
			Issuer:                "https://sub2api.example.com/api/v1/lobehub/oidc",
			AuthorizationEndpoint: "https://sub2api.example.com/api/v1/lobehub/oidc/authorize",
			TokenEndpoint:         "https://sub2api.example.com/api/v1/lobehub/oidc/token",
			UserInfoEndpoint:      "https://sub2api.example.com/api/v1/lobehub/oidc/userinfo",
			JWKSURI:               "https://sub2api.example.com/api/v1/lobehub/oidc/jwks",
		},
	}
	h := NewLobeHubHandler(&lobehubLaunchServiceStub{}, oidcStub, &lobehubSSOServiceStub{})

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/lobehub/oidc/.well-known/openid-configuration", nil)

	h.Discovery(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")

	var body service.LobeHubOIDCDiscoveryDocument
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, oidcStub.discoveryResult.Issuer, body.Issuer)
}

func TestLobeHubHandler_Authorize(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oidcStub := &lobehubOIDCServiceStub{
		authorizeResult: &service.LobeHubOIDCAuthorizeResult{
			RedirectURL: "https://chat.example.com/api/auth/oauth2/callback/generic-oidc?code=code-1&state=state-1",
		},
	}
	h := NewLobeHubHandler(&lobehubLaunchServiceStub{}, oidcStub, &lobehubSSOServiceStub{})

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/lobehub/oidc/authorize?client_id=lobehub-client&redirect_uri=https%3A%2F%2Fchat.example.com%2Fapi%2Fauth%2Foauth2%2Fcallback%2Fgeneric-oidc&response_type=code&scope=openid+profile+email&state=state-1&nonce=nonce-1&return_url=https%3A%2F%2Fchat.example.com%2Fworkspace&code_challenge=challenge-1&code_challenge_method=S256", nil)
	req.AddCookie(&http.Cookie{Name: lobeHubOIDCSessionCookieName, Value: "session-1"})
	c.Request = req

	h.Authorize(c)

	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "https://chat.example.com/api/auth/oauth2/callback/generic-oidc?code=code-1&state=state-1", rec.Header().Get("Location"))
	require.Equal(t, "session-1", oidcStub.authorizeSessionID)
	require.Equal(t, "lobehub-client", oidcStub.authorizeRequest.ClientID)
	require.Equal(t, "nonce-1", oidcStub.authorizeRequest.Nonce)
	require.Equal(t, "https://chat.example.com/workspace", oidcStub.authorizeRequest.ReturnURL)
	require.Equal(t, "challenge-1", oidcStub.authorizeRequest.CodeChallenge)
}

func TestLobeHubHandler_Token_ClientSecretBasic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oidcStub := &lobehubOIDCServiceStub{
		tokenResult: &service.LobeHubOIDCTokenResponse{
			AccessToken: "access-token-1",
			TokenType:   "Bearer",
			ExpiresIn:   service.LobeHubOIDCAccessTokenTTLSeconds,
			Scope:       "openid profile email",
			IDToken:     "id-token-1",
		},
	}
	h := NewLobeHubHandler(&lobehubLaunchServiceStub{}, oidcStub, &lobehubSSOServiceStub{})

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", "code-1")
	form.Set("redirect_uri", "https://chat.example.com/api/auth/oauth2/callback/generic-oidc")
	form.Set("code_verifier", "verifier-1")

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/lobehub/oidc/token", strings.NewReader(form.Encode()))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.Request.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("lobehub-client:lobehub-secret")))

	h.Token(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "lobehub-client", oidcStub.tokenRequest.ClientID)
	require.Equal(t, "lobehub-secret", oidcStub.tokenRequest.ClientSecret)
	require.Equal(t, "verifier-1", oidcStub.tokenRequest.CodeVerifier)

	var body service.LobeHubOIDCTokenResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "access-token-1", body.AccessToken)
}

func TestLobeHubHandler_UserInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oidcStub := &lobehubOIDCServiceStub{
		userInfoResult: &service.LobeHubOIDCUserInfo{
			Subject:           "42",
			Email:             "user@example.com",
			EmailVerified:     true,
			Name:              "alice",
			PreferredUsername: "alice",
		},
	}
	h := NewLobeHubHandler(&lobehubLaunchServiceStub{}, oidcStub, &lobehubSSOServiceStub{})

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/lobehub/oidc/userinfo", nil)
	c.Request.Header.Set("Authorization", "Bearer access-token-1")

	h.UserInfo(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "access-token-1", oidcStub.userInfoToken)

	var body service.LobeHubOIDCUserInfo
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "42", body.Subject)
	require.Equal(t, "user@example.com", body.Email)
}

func TestLobeHubHandler_CreateOIDCWebSession(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ssoStub := &lobehubSSOServiceStub{
		prepareResult: &service.LobeHubSSOContinuationResult{
			ContinueURL:       "https://sub2api.example.com/api/v1/lobehub/oidc/authorize?client_id=lobehub-client",
			OIDCSessionID:     "session-1",
			TargetToken:       "target-1",
			BootstrapTicketID: "bootstrap-1",
			CookieDomain:      ".example.com",
		},
	}
	h := NewLobeHubHandler(&lobehubLaunchServiceStub{}, &lobehubOIDCServiceStub{}, ssoStub)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/lobehub/oidc-web-session", bytes.NewBufferString(`{"resume_token":"resume-1","return_url":"https://chat.example.com/workspace"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})

	h.CreateOIDCWebSession(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(42), ssoStub.prepareUserID)
	require.Equal(t, "resume-1", ssoStub.prepareRequest.ResumeToken)
	require.Contains(t, rec.Header().Values("Set-Cookie")[0], "lobehub_oidc_session=session-1")
	require.Contains(t, strings.Join(rec.Header().Values("Set-Cookie"), "\n"), "sub2api_lobehub_target=target-1")
	require.Contains(t, strings.Join(rec.Header().Values("Set-Cookie"), "\n"), "sub2api_lobehub_bootstrap=bootstrap-1")
}

func TestLobeHubHandler_BootstrapExchange(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ssoStub := &lobehubSSOServiceStub{
		exchangeResult: &service.LobeHubBootstrapExchangeResult{
			BootstrapTicketID: "bootstrap-2",
		},
	}
	h := NewLobeHubHandler(&lobehubLaunchServiceStub{}, &lobehubOIDCServiceStub{}, ssoStub)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/lobehub/bootstrap-exchange", bytes.NewBufferString(`{"return_url":"https://chat.example.com/workspace"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: lobeHubTargetCookieName, Value: "target-1"})
	c.Request = req

	h.BootstrapExchange(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "target-1", ssoStub.exchangeToken)
	require.Equal(t, "https://chat.example.com/workspace", ssoStub.exchangeReturn)
}

func TestLobeHubHandler_ConsumeBootstrap(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ssoStub := &lobehubSSOServiceStub{
		consumeResult: &service.LobeHubBootstrapConsumeResult{
			RedirectURL: "https://chat.example.com/workspace?settings=%7B%7D",
		},
	}
	h := NewLobeHubHandler(&lobehubLaunchServiceStub{}, &lobehubOIDCServiceStub{}, ssoStub)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/lobehub/bootstrap/consume?ticket=bootstrap-1", nil)

	h.ConsumeBootstrap(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "bootstrap-1", ssoStub.consumeTicketID)
	require.Contains(t, rec.Body.String(), "redirect_url")
}

func TestLobeHubHandler_CompareCurrentConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ssoStub := &lobehubSSOServiceStub{
		compareResult: &service.LobeHubConfigProbeResult{
			Matched:                  true,
			DesiredConfigFingerprint: "fp-1",
			CurrentConfigFingerprint: "fp-1",
		},
	}
	h := NewLobeHubHandler(&lobehubLaunchServiceStub{}, &lobehubOIDCServiceStub{}, ssoStub)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/lobehub/config-probe/compare", bytes.NewBufferString(`{
		"key_vaults":{"openai":{"api_key":"sk-user-1","base_url":"https://api.example.com/v1"}},
		"language_model":{"openai":{"enabled":true,"enabled_models":["gpt-4.1"]}}
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: lobeHubTargetCookieName, Value: "target-1"})
	c.Request = req

	h.CompareCurrentConfig(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "target-1", ssoStub.compareToken)
	require.NotNil(t, ssoStub.compareObserved)
	require.Equal(t, "sk-user-1", ssoStub.compareObserved.KeyVaults["openai"].APIKey)
	require.True(t, ssoStub.compareObserved.LanguageModel["openai"].Enabled)
	require.Contains(t, rec.Body.String(), `"matched":true`)
}
