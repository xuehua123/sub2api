//go:build unit

package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

type lobehubUserReaderStub struct {
	user *User
	err  error
}

func (s *lobehubUserReaderStub) GetByID(context.Context, int64) (*User, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.user, nil
}

type lobehubOIDCStateStoreStub struct {
	webSessions             map[string]*LobeHubOIDCWebSession
	createWebSessionID      string
	createdWebSession       *LobeHubOIDCWebSession
	createdWebSessionTTL    time.Duration
	createAuthorizationID   string
	createdAuthorization    *LobeHubOIDCAuthorizationCode
	createdAuthorizationTTL time.Duration
	authorizationCodes      map[string]*LobeHubOIDCAuthorizationCode
	createAccessTokenID     string
	createdAccessToken      *LobeHubOIDCAccessToken
	createdAccessTokenTTL   time.Duration
	accessTokens            map[string]*LobeHubOIDCAccessToken
	createResumeID          string
	createdResume           *LobeHubOIDCResumeToken
	createdResumeTTL        time.Duration
	resumeTokens            map[string]*LobeHubOIDCResumeToken
	createBootstrapID       string
	createdBootstrap        *LobeHubBootstrapTicket
	createdBootstrapTTL     time.Duration
	bootstrapTickets        map[string]*LobeHubBootstrapTicket
}

func (s *lobehubOIDCStateStoreStub) CreateWebSession(_ context.Context, session *LobeHubOIDCWebSession, ttl time.Duration) (string, error) {
	s.createdWebSession = session
	s.createdWebSessionTTL = ttl
	return s.createWebSessionID, nil
}

func (s *lobehubOIDCStateStoreStub) GetWebSession(_ context.Context, sessionID string) (*LobeHubOIDCWebSession, error) {
	if session, ok := s.webSessions[sessionID]; ok {
		return session, nil
	}
	return nil, ErrLobeHubOIDCSessionNotFound
}

func (s *lobehubOIDCStateStoreStub) DeleteWebSession(context.Context, string) error {
	return nil
}

func (s *lobehubOIDCStateStoreStub) CreateAuthorizationCode(_ context.Context, code *LobeHubOIDCAuthorizationCode, ttl time.Duration) (string, error) {
	s.createdAuthorization = code
	s.createdAuthorizationTTL = ttl
	return s.createAuthorizationID, nil
}

func (s *lobehubOIDCStateStoreStub) ConsumeAuthorizationCode(_ context.Context, code string) (*LobeHubOIDCAuthorizationCode, error) {
	if payload, ok := s.authorizationCodes[code]; ok {
		delete(s.authorizationCodes, code)
		return payload, nil
	}
	return nil, ErrLobeHubOIDCInvalidGrant
}

func (s *lobehubOIDCStateStoreStub) CreateAccessToken(_ context.Context, token *LobeHubOIDCAccessToken, ttl time.Duration) (string, error) {
	s.createdAccessToken = token
	s.createdAccessTokenTTL = ttl
	return s.createAccessTokenID, nil
}

func (s *lobehubOIDCStateStoreStub) GetAccessToken(_ context.Context, token string) (*LobeHubOIDCAccessToken, error) {
	if payload, ok := s.accessTokens[token]; ok {
		return payload, nil
	}
	return nil, ErrLobeHubOIDCInvalidToken
}

func (s *lobehubOIDCStateStoreStub) CreateResumeToken(_ context.Context, resume *LobeHubOIDCResumeToken, ttl time.Duration) (string, error) {
	s.createdResume = resume
	s.createdResumeTTL = ttl
	return s.createResumeID, nil
}

func (s *lobehubOIDCStateStoreStub) GetResumeToken(_ context.Context, resumeID string) (*LobeHubOIDCResumeToken, error) {
	if payload, ok := s.resumeTokens[resumeID]; ok {
		return payload, nil
	}
	return nil, ErrLobeHubOIDCResumeTokenNotFound
}

func (s *lobehubOIDCStateStoreStub) DeleteResumeToken(_ context.Context, resumeID string) error {
	delete(s.resumeTokens, resumeID)
	return nil
}

func (s *lobehubOIDCStateStoreStub) CreateBootstrapTicket(_ context.Context, ticket *LobeHubBootstrapTicket, ttl time.Duration) (string, error) {
	s.createdBootstrap = ticket
	s.createdBootstrapTTL = ttl
	return s.createBootstrapID, nil
}

func (s *lobehubOIDCStateStoreStub) ConsumeBootstrapTicket(_ context.Context, ticketID string) (*LobeHubBootstrapTicket, error) {
	if payload, ok := s.bootstrapTickets[ticketID]; ok {
		delete(s.bootstrapTickets, ticketID)
		return payload, nil
	}
	return nil, ErrLobeHubBootstrapTicketNotFound
}

type lobehubOIDCSigningKeyProviderStub struct {
	privateKey *rsa.PrivateKey
	keyID      string
}

func (s *lobehubOIDCSigningKeyProviderStub) GetSigningKey(context.Context) (*rsa.PrivateKey, string, error) {
	return s.privateKey, s.keyID, nil
}

func TestLobeHubOIDCService_GetDiscoveryDocument(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	svc := NewLobeHubOIDCService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{
			LobeHubEnabled:          true,
			LobeHubChatURL:          "https://chat.example.com",
			LobeHubOIDCIssuer:       "https://sub2api.example.com/api/v1/lobehub/oidc",
			LobeHubOIDCClientID:     "lobehub-client",
			LobeHubOIDCClientSecret: "lobehub-secret",
		}},
		&lobehubUserReaderStub{user: &User{ID: 42, Email: "user@example.com", Username: "alice", Status: StatusActive}},
		&lobehubOIDCStateStoreStub{},
		&lobehubOIDCSigningKeyProviderStub{privateKey: privateKey, keyID: "kid-1"},
		func() time.Time { return time.Unix(1_712_345_678, 0).UTC() },
	)

	discovery, err := svc.GetDiscoveryDocument(context.Background())
	require.NoError(t, err)
	require.Equal(t, "https://sub2api.example.com/api/v1/lobehub/oidc", discovery.Issuer)
	require.Equal(t, "https://sub2api.example.com/api/v1/lobehub/oidc/authorize", discovery.AuthorizationEndpoint)
	require.Equal(t, "https://sub2api.example.com/api/v1/lobehub/oidc/token", discovery.TokenEndpoint)
	require.Equal(t, "https://sub2api.example.com/api/v1/lobehub/oidc/userinfo", discovery.UserInfoEndpoint)
	require.Equal(t, "https://sub2api.example.com/api/v1/lobehub/oidc/jwks", discovery.JWKSURI)
	require.Contains(t, discovery.TokenEndpointAuthMethodsSupported, "client_secret_post")
	require.Contains(t, discovery.TokenEndpointAuthMethodsSupported, "client_secret_basic")
	require.Contains(t, discovery.CodeChallengeMethodsSupported, "S256")

	jwks, err := svc.GetJWKS(context.Background())
	require.NoError(t, err)
	require.Len(t, jwks.Keys, 1)
	require.Equal(t, "kid-1", jwks.Keys[0].KeyID)
	require.Equal(t, "RSA", jwks.Keys[0].KeyType)
	require.Equal(t, "sig", jwks.Keys[0].Use)
	require.NotEmpty(t, jwks.Keys[0].N)
	require.NotEmpty(t, jwks.Keys[0].E)
}

func TestLobeHubOIDCService_AuthorizeCreatesCodeForCurrentWebSession(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	store := &lobehubOIDCStateStoreStub{
		webSessions:           map[string]*LobeHubOIDCWebSession{"session-1": {UserID: 42, APIKeyID: 9, CreatedAt: time.Unix(1_712_345_600, 0).UTC()}},
		createAuthorizationID: "code-1",
	}
	svc := NewLobeHubOIDCService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{
			LobeHubEnabled:          true,
			LobeHubChatURL:          "https://chat.example.com",
			LobeHubOIDCIssuer:       "https://sub2api.example.com/api/v1/lobehub/oidc",
			LobeHubOIDCClientID:     "lobehub-client",
			LobeHubOIDCClientSecret: "lobehub-secret",
		}},
		&lobehubUserReaderStub{user: &User{ID: 42, Email: "user@example.com", Username: "alice", Status: StatusActive}},
		store,
		&lobehubOIDCSigningKeyProviderStub{privateKey: privateKey, keyID: "kid-1"},
		func() time.Time { return time.Unix(1_712_345_678, 0).UTC() },
	)

	result, err := svc.Authorize(context.Background(), "session-1", &LobeHubOIDCAuthorizeRequest{
		ClientID:            "lobehub-client",
		RedirectURI:         "https://chat.example.com/api/auth/oauth2/callback/generic-oidc",
		ResponseType:        "code",
		Scope:               "openid profile email",
		State:               "state-1",
		Nonce:               "nonce-1",
		CodeChallenge:       "challenge-1",
		CodeChallengeMethod: "S256",
	})
	require.NoError(t, err)
	require.Equal(t, LobeHubOIDCAuthorizationCodeTTL, store.createdAuthorizationTTL)
	require.NotNil(t, store.createdAuthorization)
	require.Equal(t, int64(42), store.createdAuthorization.UserID)
	require.Equal(t, "https://chat.example.com/api/auth/oauth2/callback/generic-oidc", store.createdAuthorization.RedirectURI)
	require.Equal(t, "openid profile email", store.createdAuthorization.Scope)
	require.Equal(t, "nonce-1", store.createdAuthorization.Nonce)
	require.Equal(t, "challenge-1", store.createdAuthorization.CodeChallenge)
	require.Equal(t, "S256", store.createdAuthorization.CodeChallengeMethod)

	redirectURL, err := url.Parse(result.RedirectURL)
	require.NoError(t, err)
	require.Equal(t, "code-1", redirectURL.Query().Get("code"))
	require.Equal(t, "state-1", redirectURL.Query().Get("state"))
}

func TestLobeHubOIDCService_ExchangeCodeAndUserInfo(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	codeVerifier := "verifier-1234567890"
	codeChallengeSum := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(codeChallengeSum[:])

	store := &lobehubOIDCStateStoreStub{
		authorizationCodes: map[string]*LobeHubOIDCAuthorizationCode{
			"code-1": {
				UserID:              42,
				RedirectURI:         "https://chat.example.com/api/auth/oauth2/callback/generic-oidc",
				Scope:               "openid profile email",
				Nonce:               "nonce-1",
				CodeChallenge:       codeChallenge,
				CodeChallengeMethod: "S256",
				CreatedAt:           time.Unix(1_712_345_678, 0).UTC(),
			},
		},
		createAccessTokenID: "access-token-1",
		accessTokens: map[string]*LobeHubOIDCAccessToken{
			"access-token-1": {
				UserID:    42,
				Scope:     "openid profile email",
				CreatedAt: time.Unix(1_712_345_678, 0).UTC(),
			},
		},
	}

	svc := NewLobeHubOIDCService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{
			LobeHubEnabled:          true,
			LobeHubChatURL:          "https://chat.example.com",
			LobeHubOIDCIssuer:       "https://sub2api.example.com/api/v1/lobehub/oidc",
			LobeHubOIDCClientID:     "lobehub-client",
			LobeHubOIDCClientSecret: "lobehub-secret",
		}},
		&lobehubUserReaderStub{user: &User{ID: 42, Email: "user@example.com", Username: "alice", Status: StatusActive}},
		store,
		&lobehubOIDCSigningKeyProviderStub{privateKey: privateKey, keyID: "kid-1"},
		func() time.Time { return time.Unix(1_712_345_678, 0).UTC() },
	)

	tokenResponse, err := svc.ExchangeAuthorizationCode(context.Background(), &LobeHubOIDCTokenRequest{
		GrantType:    "authorization_code",
		Code:         "code-1",
		RedirectURI:  "https://chat.example.com/api/auth/oauth2/callback/generic-oidc",
		ClientID:     "lobehub-client",
		ClientSecret: "lobehub-secret",
		CodeVerifier: codeVerifier,
	})
	require.NoError(t, err)
	require.Equal(t, "Bearer", tokenResponse.TokenType)
	require.Equal(t, "access-token-1", tokenResponse.AccessToken)
	require.Equal(t, LobeHubOIDCAccessTokenTTLSeconds, tokenResponse.ExpiresIn)
	require.Equal(t, "openid profile email", tokenResponse.Scope)
	require.NotEmpty(t, tokenResponse.IDToken)
	require.NotNil(t, store.createdAccessToken)
	require.Equal(t, int64(42), store.createdAccessToken.UserID)
	require.Equal(t, LobeHubOIDCAccessTokenTTL, store.createdAccessTokenTTL)

	claims := jwt.MapClaims{}
	parsedToken, err := jwt.ParseWithClaims(tokenResponse.IDToken, claims, func(token *jwt.Token) (any, error) {
		return &privateKey.PublicKey, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Name}), jwt.WithTimeFunc(func() time.Time {
		return time.Unix(1_712_345_678, 0).UTC()
	}))
	require.NoError(t, err)
	require.True(t, parsedToken.Valid)
	require.Equal(t, "https://sub2api.example.com/api/v1/lobehub/oidc", claims["iss"])
	require.Equal(t, "lobehub-client", claims["aud"])
	require.Equal(t, "42", claims["sub"])
	require.Equal(t, "nonce-1", claims["nonce"])
	require.Equal(t, "user@example.com", claims["email"])
	require.Equal(t, "alice", claims["preferred_username"])

	userInfo, err := svc.GetUserInfo(context.Background(), "access-token-1")
	require.NoError(t, err)
	require.Equal(t, "42", userInfo.Subject)
	require.Equal(t, "user@example.com", userInfo.Email)
	require.True(t, userInfo.EmailVerified)
	require.Equal(t, "alice", userInfo.PreferredUsername)
	require.Equal(t, "alice", userInfo.Name)
}

func TestLobeHubOIDCService_AuthorizeAllowsLegacyCallbackPath(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	svc := NewLobeHubOIDCService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{
			LobeHubEnabled:          true,
			LobeHubChatURL:          "https://chat.example.com/base",
			LobeHubOIDCIssuer:       "https://sub2api.example.com/api/v1/lobehub/oidc",
			LobeHubOIDCClientID:     "lobehub-client",
			LobeHubOIDCClientSecret: "lobehub-secret",
		}},
		&lobehubUserReaderStub{user: &User{ID: 42, Email: "user@example.com", Username: "alice", Status: StatusActive}},
		&lobehubOIDCStateStoreStub{
			webSessions:           map[string]*LobeHubOIDCWebSession{"session-1": {UserID: 42, APIKeyID: 9, CreatedAt: time.Unix(1_712_345_600, 0).UTC()}},
			createAuthorizationID: "code-1",
		},
		&lobehubOIDCSigningKeyProviderStub{privateKey: privateKey, keyID: "kid-1"},
		time.Now,
	)

	result, err := svc.Authorize(context.Background(), "session-1", &LobeHubOIDCAuthorizeRequest{
		ClientID:            "lobehub-client",
		RedirectURI:         "https://chat.example.com/api/auth/callback/generic-oidc",
		ResponseType:        "code",
		Scope:               "openid profile email",
		State:               "state-1",
		CodeChallenge:       "challenge-1",
		CodeChallengeMethod: "S256",
	})
	require.NoError(t, err)
	require.True(t, strings.Contains(result.RedirectURL, "code=code-1"))
}

func TestLobeHubOIDCService_AuthorizeRequiresPKCE(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	store := &lobehubOIDCStateStoreStub{
		webSessions:           map[string]*LobeHubOIDCWebSession{"session-1": {UserID: 42, APIKeyID: 9, CreatedAt: time.Unix(1_712_345_600, 0).UTC()}},
		createAuthorizationID: "code-1",
	}
	svc := NewLobeHubOIDCService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{
			LobeHubEnabled:          true,
			LobeHubChatURL:          "https://chat.example.com",
			LobeHubOIDCIssuer:       "https://sub2api.example.com/api/v1/lobehub/oidc",
			LobeHubOIDCClientID:     "lobehub-client",
			LobeHubOIDCClientSecret: "lobehub-secret",
		}},
		&lobehubUserReaderStub{user: &User{ID: 42, Email: "user@example.com", Username: "alice", Status: StatusActive}},
		store,
		&lobehubOIDCSigningKeyProviderStub{privateKey: privateKey, keyID: "kid-1"},
		time.Now,
	)

	_, err = svc.Authorize(context.Background(), "session-1", &LobeHubOIDCAuthorizeRequest{
		ClientID:     "lobehub-client",
		RedirectURI:  "https://chat.example.com/api/auth/oauth2/callback/generic-oidc",
		ResponseType: "code",
		Scope:        "openid profile email",
		State:        "state-1",
	})
	var protocolErr *LobeHubOIDCProtocolError
	require.ErrorAs(t, err, &protocolErr)
	require.Equal(t, ErrLobeHubOIDCInvalidRequest.Code, protocolErr.Code)
	require.Nil(t, store.createdAuthorization)
}

func TestLobeHubOIDCService_AuthorizeCreatesResumeRedirectWhenWebSessionMissing(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	store := &lobehubOIDCStateStoreStub{
		createResumeID: "resume-1",
	}
	svc := NewLobeHubOIDCService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{
			LobeHubEnabled:          true,
			LobeHubChatURL:          "https://chat.example.com",
			LobeHubOIDCIssuer:       "https://sub2api.example.com/api/v1/lobehub/oidc",
			LobeHubOIDCClientID:     "lobehub-client",
			LobeHubOIDCClientSecret: "lobehub-secret",
			FrontendURL:             "https://app.example.com",
		}},
		&lobehubUserReaderStub{user: &User{ID: 42, Email: "user@example.com", Username: "alice", Status: StatusActive}},
		store,
		&lobehubOIDCSigningKeyProviderStub{privateKey: privateKey, keyID: "kid-1"},
		func() time.Time { return time.Unix(1_712_345_678, 0).UTC() },
	)

	result, err := svc.Authorize(context.Background(), "", &LobeHubOIDCAuthorizeRequest{
		ClientID:            "lobehub-client",
		RedirectURI:         "https://chat.example.com/api/auth/oauth2/callback/generic-oidc",
		ResponseType:        "code",
		Scope:               "openid profile email",
		State:               "state-1",
		Nonce:               "nonce-1",
		ReturnURL:           "https://chat.example.com/workspace",
		CodeChallenge:       "challenge-1",
		CodeChallengeMethod: "S256",
	})
	require.NoError(t, err)
	require.Equal(t, "https://app.example.com/auth/lobehub-sso?resume=resume-1&return_url=https%3A%2F%2Fchat.example.com%2Fworkspace", result.RedirectURL)
	require.Equal(t, LobeHubOIDCResumeTokenTTL, store.createdResumeTTL)
	require.NotNil(t, store.createdResume)
	require.Equal(t, "lobehub-client", store.createdResume.ClientID)
	require.Equal(t, "https://chat.example.com/api/auth/oauth2/callback/generic-oidc", store.createdResume.RedirectURI)
	require.Equal(t, "code", store.createdResume.ResponseType)
	require.Equal(t, "openid profile email", store.createdResume.Scope)
	require.Equal(t, "state-1", store.createdResume.State)
	require.Equal(t, "nonce-1", store.createdResume.Nonce)
	require.Equal(t, "https://chat.example.com/workspace", store.createdResume.ReturnURL)
	require.Equal(t, "challenge-1", store.createdResume.CodeChallenge)
	require.Equal(t, "S256", store.createdResume.CodeChallengeMethod)
}
