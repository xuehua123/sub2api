//go:build unit

package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type lobehubAPIKeyReaderStub struct {
	keys map[int64]*APIKey
}

func (s *lobehubAPIKeyReaderStub) GetByID(_ context.Context, id int64) (*APIKey, error) {
	if key, ok := s.keys[id]; ok {
		return key, nil
	}
	return nil, ErrAPIKeyNotFound
}

type lobehubUserPreferenceStoreStub struct {
	user                *User
	updatedUserID       int64
	updatedDefaultKeyID *int64
	updateCalls         int
}

func (s *lobehubUserPreferenceStoreStub) GetByID(context.Context, int64) (*User, error) {
	return s.user, nil
}

func (s *lobehubUserPreferenceStoreStub) UpdateDefaultChatAPIKeyID(_ context.Context, userID int64, apiKeyID *int64) error {
	s.updatedUserID = userID
	s.updatedDefaultKeyID = apiKeyID
	s.updateCalls++
	if s.user != nil {
		s.user.DefaultChatAPIKeyID = apiKeyID
	}
	return nil
}

func TestLobeHubSSOService_PrepareOIDCContinuationUsesDefaultKey(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	now := time.Unix(1_712_400_000, 0).UTC()
	defaultKeyID := int64(9)
	userStore := &lobehubUserPreferenceStoreStub{
		user: &User{
			ID:                  42,
			Email:               "user@example.com",
			Username:            "alice",
			Status:              StatusActive,
			DefaultChatAPIKeyID: &defaultKeyID,
		},
	}
	stateStore := &lobehubOIDCStateStoreStub{
		resumeTokens: map[string]*LobeHubOIDCResumeToken{
			"resume-1": {
				ClientID:            "lobehub-client",
				RedirectURI:         "https://chat.example.com/api/auth/oauth2/callback/generic-oidc",
				ResponseType:        "code",
				Scope:               "openid profile email",
				State:               "state-1",
				Nonce:               "nonce-1",
				CodeChallenge:       "challenge-1",
				CodeChallengeMethod: "S256",
				CreatedAt:           now,
			},
		},
		createWebSessionID: "session-1",
		createBootstrapID:  "bootstrap-1",
	}
	svc := NewLobeHubSSOService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{
			LobeHubEnabled:              true,
			LobeHubChatURL:              "https://chat.example.com",
			LobeHubOIDCIssuer:           "https://sub2api.example.com/api/v1/lobehub/oidc",
			LobeHubOIDCClientID:         "lobehub-client",
			LobeHubOIDCClientSecret:     "lobehub-secret",
			LobeHubDefaultProvider:      "openai",
			LobeHubDefaultModel:         "gpt-4.1",
			LobeHubRuntimeConfigVersion: "runtime-v1",
			APIBaseURL:                  "https://api.example.com",
		}},
		userStore,
		&lobehubAPIKeyReaderStub{keys: map[int64]*APIKey{
			9: {
				ID:     9,
				UserID: 42,
				Key:    "sk-user-1",
				Name:   "default",
				Status: StatusActive,
			},
		}},
		stateStore,
		&lobehubOIDCSigningKeyProviderStub{privateKey: privateKey, keyID: "kid-1"},
		func() time.Time { return now },
	)

	result, err := svc.PrepareOIDCContinuation(context.Background(), 42, &LobeHubSSOContinuationRequest{
		ResumeToken: "resume-1",
		ReturnURL:   "https://chat.example.com/chat/thread-1",
	})
	require.NoError(t, err)
	require.Equal(t, "session-1", result.OIDCSessionID)
	require.Equal(t, "bootstrap-1", result.BootstrapTicketID)
	require.Equal(t, ".example.com", result.CookieDomain)
	require.NotEmpty(t, result.TargetToken)

	continueURL, err := url.Parse(result.ContinueURL)
	require.NoError(t, err)
	require.Equal(t, "/api/v1/lobehub/oidc/authorize", continueURL.Path)
	require.Equal(t, "lobehub-client", continueURL.Query().Get("client_id"))
	require.Equal(t, "state-1", continueURL.Query().Get("state"))
	require.Equal(t, "nonce-1", continueURL.Query().Get("nonce"))
	require.Equal(t, "https://chat.example.com/chat/thread-1", continueURL.Query().Get("return_url"))

	require.NotNil(t, stateStore.createdBootstrap)
	require.Equal(t, int64(42), stateStore.createdBootstrap.UserID)
	require.Equal(t, int64(9), stateStore.createdBootstrap.APIKeyID)
	require.Equal(t, "https://chat.example.com/chat/thread-1", stateStore.createdBootstrap.ReturnURL)
	require.Equal(t, "runtime-v1", stateStore.createdBootstrap.RuntimeConfigVersion)
	require.NotNil(t, stateStore.createdWebSession)
	require.Equal(t, int64(42), stateStore.createdWebSession.UserID)
	require.Equal(t, int64(9), stateStore.createdWebSession.APIKeyID)
}

func TestSanitizeLobeHubReturnURL_RejectsCrossOriginURL(t *testing.T) {
	_, err := sanitizeLobeHubReturnURL("https://chat.example.com", "http://chat.example.com/workspace")
	require.ErrorIs(t, err, ErrLobeHubInvalidReturnURL)
}

func TestResolveSharedCookieDomain_UsesRegistrableDomainOnlyForSubdomains(t *testing.T) {
	require.Equal(t, ".example.com", resolveSharedCookieDomain("https://chat.example.com"))
	require.Equal(t, ".example.co.uk", resolveSharedCookieDomain("https://chat.example.co.uk"))
	require.Empty(t, resolveSharedCookieDomain("https://example.com"))
	require.Empty(t, resolveSharedCookieDomain("http://127.0.0.1:3210"))
}

func TestLobeHubSSOService_PrepareOIDCContinuationRequiresDefaultKeySelection(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	now := time.Unix(1_712_400_000, 0).UTC()
	svc := NewLobeHubSSOService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{
			LobeHubEnabled:              true,
			LobeHubChatURL:              "https://chat.example.com",
			LobeHubOIDCIssuer:           "https://sub2api.example.com/api/v1/lobehub/oidc",
			LobeHubOIDCClientID:         "lobehub-client",
			LobeHubOIDCClientSecret:     "lobehub-secret",
			LobeHubDefaultProvider:      "openai",
			LobeHubRuntimeConfigVersion: "runtime-v1",
			APIBaseURL:                  "https://api.example.com",
		}},
		&lobehubUserPreferenceStoreStub{
			user: &User{
				ID:       42,
				Email:    "user@example.com",
				Username: "alice",
				Status:   StatusActive,
			},
		},
		&lobehubAPIKeyReaderStub{keys: map[int64]*APIKey{}},
		&lobehubOIDCStateStoreStub{
			resumeTokens: map[string]*LobeHubOIDCResumeToken{
				"resume-1": {
					ClientID:            "lobehub-client",
					RedirectURI:         "https://chat.example.com/api/auth/oauth2/callback/generic-oidc",
					ResponseType:        "code",
					Scope:               "openid profile email",
					State:               "state-1",
					CodeChallenge:       "challenge-1",
					CodeChallengeMethod: "S256",
					CreatedAt:           now,
				},
			},
		},
		&lobehubOIDCSigningKeyProviderStub{privateKey: privateKey, keyID: "kid-1"},
		func() time.Time { return now },
	)

	_, err = svc.PrepareOIDCContinuation(context.Background(), 42, &LobeHubSSOContinuationRequest{
		ResumeToken: "resume-1",
		ReturnURL:   "https://chat.example.com/chat",
	})
	require.ErrorIs(t, err, ErrLobeHubDefaultChatAPIKeyRequired)
}

func TestLobeHubSSOService_PrepareTargetRefreshDoesNotPersistInvalidExplicitKey(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	now := time.Unix(1_712_400_000, 0).UTC()
	currentDefaultKeyID := int64(7)
	explicitKeyID := int64(9)
	userStore := &lobehubUserPreferenceStoreStub{
		user: &User{
			ID:                  42,
			Email:               "user@example.com",
			Username:            "alice",
			Status:              StatusActive,
			DefaultChatAPIKeyID: &currentDefaultKeyID,
		},
	}
	svc := NewLobeHubSSOService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{
			LobeHubEnabled:              true,
			LobeHubChatURL:              "https://chat.example.com",
			LobeHubOIDCIssuer:           "https://sub2api.example.com/api/v1/lobehub/oidc",
			LobeHubOIDCClientID:         "lobehub-client",
			LobeHubOIDCClientSecret:     "lobehub-secret",
			LobeHubDefaultProvider:      "openai",
			LobeHubRuntimeConfigVersion: "runtime-v1",
			APIBaseURL:                  "https://api.example.com",
		}},
		userStore,
		&lobehubAPIKeyReaderStub{keys: map[int64]*APIKey{
			9: {
				ID:     9,
				UserID: 99,
				Key:    "sk-other-user",
				Status: StatusActive,
			},
		}},
		&lobehubOIDCStateStoreStub{},
		&lobehubOIDCSigningKeyProviderStub{privateKey: privateKey, keyID: "kid-1"},
		func() time.Time { return now },
	)

	_, err = svc.PrepareTargetRefresh(context.Background(), 42, &LobeHubSSORefreshRequest{
		ReturnURL: "https://chat.example.com/workspace",
		APIKeyID:  &explicitKeyID,
	})
	require.ErrorIs(t, err, ErrLobeHubDefaultChatAPIKeyRequired)
	require.Equal(t, 0, userStore.updateCalls)
	require.Equal(t, &currentDefaultKeyID, userStore.user.DefaultChatAPIKeyID)
}

func TestLobeHubSSOService_PrepareTargetRefreshClearsDeletedDefaultKey(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	now := time.Unix(1_712_400_000, 0).UTC()
	defaultKeyID := int64(9)
	userStore := &lobehubUserPreferenceStoreStub{
		user: &User{
			ID:                  42,
			Email:               "user@example.com",
			Username:            "alice",
			Status:              StatusActive,
			DefaultChatAPIKeyID: &defaultKeyID,
		},
	}
	svc := NewLobeHubSSOService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{
			LobeHubEnabled:              true,
			LobeHubChatURL:              "https://chat.example.com",
			LobeHubOIDCIssuer:           "https://sub2api.example.com/api/v1/lobehub/oidc",
			LobeHubOIDCClientID:         "lobehub-client",
			LobeHubOIDCClientSecret:     "lobehub-secret",
			LobeHubDefaultProvider:      "openai",
			LobeHubRuntimeConfigVersion: "runtime-v1",
			APIBaseURL:                  "https://api.example.com",
		}},
		userStore,
		&lobehubAPIKeyReaderStub{keys: map[int64]*APIKey{}},
		&lobehubOIDCStateStoreStub{},
		&lobehubOIDCSigningKeyProviderStub{privateKey: privateKey, keyID: "kid-1"},
		func() time.Time { return now },
	)

	_, err = svc.PrepareTargetRefresh(context.Background(), 42, &LobeHubSSORefreshRequest{
		ReturnURL: "https://chat.example.com/workspace",
	})
	require.ErrorIs(t, err, ErrLobeHubDefaultChatAPIKeyRequired)
	require.Equal(t, 1, userStore.updateCalls)
	require.Equal(t, int64(42), userStore.updatedUserID)
	require.Nil(t, userStore.updatedDefaultKeyID)
	require.Nil(t, userStore.user.DefaultChatAPIKeyID)
}

func TestLobeHubSSOService_ExchangeBootstrapAndConsume(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	now := time.Unix(1_712_400_000, 0).UTC()
	defaultKeyID := int64(9)
	stateStore := &lobehubOIDCStateStoreStub{
		createBootstrapID: "bootstrap-2",
		bootstrapTickets:  map[string]*LobeHubBootstrapTicket{},
	}
	svc := NewLobeHubSSOService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{
			LobeHubEnabled:              true,
			LobeHubChatURL:              "https://chat.example.com",
			LobeHubOIDCIssuer:           "https://sub2api.example.com/api/v1/lobehub/oidc",
			LobeHubOIDCClientID:         "lobehub-client",
			LobeHubOIDCClientSecret:     "lobehub-secret",
			LobeHubDefaultProvider:      "openai",
			LobeHubDefaultModel:         "gpt-4.1",
			LobeHubRuntimeConfigVersion: "runtime-v1",
			APIBaseURL:                  "https://api.example.com",
		}},
		&lobehubUserPreferenceStoreStub{
			user: &User{
				ID:                  42,
				Email:               "user@example.com",
				Username:            "alice",
				Status:              StatusActive,
				DefaultChatAPIKeyID: &defaultKeyID,
			},
		},
		&lobehubAPIKeyReaderStub{keys: map[int64]*APIKey{
			9: {
				ID:     9,
				UserID: 42,
				Key:    "sk-user-1",
				Status: StatusActive,
			},
		}},
		stateStore,
		&lobehubOIDCSigningKeyProviderStub{privateKey: privateKey, keyID: "kid-1"},
		func() time.Time { return now },
	)

	prepare, err := svc.PrepareTargetRefresh(context.Background(), 42, &LobeHubSSORefreshRequest{
		ReturnURL: "https://chat.example.com/workspace",
	})
	require.NoError(t, err)
	require.NotEmpty(t, prepare.TargetToken)

	stateStore.bootstrapTickets["bootstrap-2"] = stateStore.createdBootstrap

	exchanged, err := svc.ExchangeBootstrap(context.Background(), prepare.TargetToken, "https://chat.example.com/workspace")
	require.NoError(t, err)
	require.Equal(t, "bootstrap-2", exchanged.BootstrapTicketID)

	redirectResult, err := svc.ConsumeBootstrap(context.Background(), "bootstrap-2")
	require.NoError(t, err)
	require.Equal(t, "openai", redirectResult.ProviderID)
	require.False(t, redirectResult.FetchOnClient)
	require.Equal(t, map[string]string{
		"apiKey":  "sk-user-1",
		"baseURL": "https://api.example.com/v1",
	}, redirectResult.KeyVaults)
	redirectURL, err := url.Parse(redirectResult.RedirectURL)
	require.NoError(t, err)
	require.Equal(t, "https", redirectURL.Scheme)
	require.Equal(t, "chat.example.com", redirectURL.Host)
	require.Equal(t, "/workspace", redirectURL.Path)
	settingsJSON := redirectURL.Query().Get("settings")
	require.NotContains(t, settingsJSON, `"apiKey":"sk-user-1"`)
	require.NotContains(t, settingsJSON, `"baseURL":"https://api.example.com/v1"`)
	require.Contains(t, settingsJSON, `"provider":"openai"`)
	require.Contains(t, settingsJSON, `"model":"gpt-4.1"`)
	require.Contains(t, settingsJSON, `"enabledModels":["gpt-4.1"]`)
}

func TestLobeHubSSOService_ExchangeBootstrapAndConsumeIncludesConfiguredEnabledModels(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	now := time.Unix(1_712_400_000, 0).UTC()
	defaultKeyID := int64(9)
	stateStore := &lobehubOIDCStateStoreStub{
		createBootstrapID: "bootstrap-3",
		bootstrapTickets:  map[string]*LobeHubBootstrapTicket{},
	}
	svc := NewLobeHubSSOService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{
			LobeHubEnabled:              true,
			LobeHubChatURL:              "https://chat.example.com",
			LobeHubOIDCIssuer:           "https://sub2api.example.com/api/v1/lobehub/oidc",
			LobeHubOIDCClientID:         "lobehub-client",
			LobeHubOIDCClientSecret:     "lobehub-secret",
			LobeHubDefaultProvider:      "openai",
			LobeHubDefaultModel:         "gpt-5.4-mini",
			LobeHubEnabledModels:        []string{"gpt-5.4-mini", "gpt-image-2", "gpt-5.4-mini"},
			LobeHubRuntimeConfigVersion: "runtime-v1",
			APIBaseURL:                  "https://api.example.com",
		}},
		&lobehubUserPreferenceStoreStub{
			user: &User{
				ID:                  42,
				Email:               "user@example.com",
				Username:            "alice",
				Status:              StatusActive,
				DefaultChatAPIKeyID: &defaultKeyID,
			},
		},
		&lobehubAPIKeyReaderStub{keys: map[int64]*APIKey{
			9: {
				ID:     9,
				UserID: 42,
				Key:    "sk-user-1",
				Status: StatusActive,
			},
		}},
		stateStore,
		&lobehubOIDCSigningKeyProviderStub{privateKey: privateKey, keyID: "kid-1"},
		func() time.Time { return now },
	)

	prepare, err := svc.PrepareTargetRefresh(context.Background(), 42, &LobeHubSSORefreshRequest{
		ReturnURL: "https://chat.example.com/workspace",
	})
	require.NoError(t, err)

	stateStore.bootstrapTickets["bootstrap-3"] = stateStore.createdBootstrap

	_, err = svc.ExchangeBootstrap(context.Background(), prepare.TargetToken, "https://chat.example.com/workspace")
	require.NoError(t, err)

	redirectResult, err := svc.ConsumeBootstrap(context.Background(), "bootstrap-3")
	require.NoError(t, err)

	redirectURL, err := url.Parse(redirectResult.RedirectURL)
	require.NoError(t, err)
	settingsJSON := redirectURL.Query().Get("settings")
	require.Contains(t, settingsJSON, `"model":"gpt-5.4-mini"`)
	require.Contains(t, settingsJSON, `"enabledModels":["gpt-5.4-mini"]`)
	require.NotContains(t, settingsJSON, "gpt-image-2")
}

func TestLobeHubSSOService_CompareCurrentConfigMatchesImportedSettings(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	now := time.Unix(1_712_400_000, 0).UTC()
	defaultKeyID := int64(9)
	svc := NewLobeHubSSOService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{
			LobeHubEnabled:              true,
			LobeHubChatURL:              "https://chat.example.com",
			LobeHubOIDCIssuer:           "https://sub2api.example.com/api/v1/lobehub/oidc",
			LobeHubOIDCClientID:         "lobehub-client",
			LobeHubOIDCClientSecret:     "lobehub-secret",
			LobeHubDefaultProvider:      "openai",
			LobeHubDefaultModel:         "gpt-4.1",
			LobeHubRuntimeConfigVersion: "runtime-v1",
			APIBaseURL:                  "https://api.example.com",
		}},
		&lobehubUserPreferenceStoreStub{
			user: &User{
				ID:                  42,
				Email:               "user@example.com",
				Username:            "alice",
				Status:              StatusActive,
				DefaultChatAPIKeyID: &defaultKeyID,
			},
		},
		&lobehubAPIKeyReaderStub{keys: map[int64]*APIKey{
			9: {
				ID:     9,
				UserID: 42,
				Key:    "sk-user-1",
				Status: StatusActive,
			},
		}},
		&lobehubOIDCStateStoreStub{},
		&lobehubOIDCSigningKeyProviderStub{privateKey: privateKey, keyID: "kid-1"},
		func() time.Time { return now },
	)

	prepare, err := svc.PrepareTargetRefresh(context.Background(), 42, &LobeHubSSORefreshRequest{
		ReturnURL: "https://chat.example.com/workspace",
	})
	require.NoError(t, err)

	result, err := svc.CompareCurrentConfig(context.Background(), prepare.TargetToken, &LobeHubObservedSettings{
		KeyVaults: map[string]LobeHubObservedKeyVault{
			"openai": {
				APIKey:  "sk-user-1",
				BaseURL: "https://api.example.com/v1",
			},
		},
		LanguageModel: map[string]LobeHubObservedLanguageModel{
			"openai": {
				Enabled:       true,
				EnabledModels: []string{"gpt-4.1"},
			},
		},
	})
	require.NoError(t, err)
	require.True(t, result.Matched)
	require.Equal(t, buildLobeHubDesiredConfigFingerprint(42, 9, &SystemSettings{
		LobeHubDefaultProvider:      "openai",
		LobeHubDefaultModel:         "gpt-4.1",
		LobeHubRuntimeConfigVersion: "runtime-v1",
		APIBaseURL:                  "https://api.example.com",
	}), result.DesiredConfigFingerprint)
}

func TestLobeHubSSOService_CompareCurrentConfigReturnsMismatchWhenObservedSettingsDiffer(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	now := time.Unix(1_712_400_000, 0).UTC()
	defaultKeyID := int64(9)
	svc := NewLobeHubSSOService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{
			LobeHubEnabled:              true,
			LobeHubChatURL:              "https://chat.example.com",
			LobeHubOIDCIssuer:           "https://sub2api.example.com/api/v1/lobehub/oidc",
			LobeHubOIDCClientID:         "lobehub-client",
			LobeHubOIDCClientSecret:     "lobehub-secret",
			LobeHubDefaultProvider:      "openai",
			LobeHubDefaultModel:         "gpt-4.1",
			LobeHubRuntimeConfigVersion: "runtime-v1",
			APIBaseURL:                  "https://api.example.com",
		}},
		&lobehubUserPreferenceStoreStub{
			user: &User{
				ID:                  42,
				Email:               "user@example.com",
				Username:            "alice",
				Status:              StatusActive,
				DefaultChatAPIKeyID: &defaultKeyID,
			},
		},
		&lobehubAPIKeyReaderStub{keys: map[int64]*APIKey{
			9: {
				ID:     9,
				UserID: 42,
				Key:    "sk-user-1",
				Status: StatusActive,
			},
		}},
		&lobehubOIDCStateStoreStub{},
		&lobehubOIDCSigningKeyProviderStub{privateKey: privateKey, keyID: "kid-1"},
		func() time.Time { return now },
	)

	prepare, err := svc.PrepareTargetRefresh(context.Background(), 42, &LobeHubSSORefreshRequest{
		ReturnURL: "https://chat.example.com/workspace",
	})
	require.NoError(t, err)

	result, err := svc.CompareCurrentConfig(context.Background(), prepare.TargetToken, &LobeHubObservedSettings{
		KeyVaults: map[string]LobeHubObservedKeyVault{
			"openai": {
				APIKey:  "sk-someone-else",
				BaseURL: "https://api.example.com/v1",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, result.Matched)
	require.Equal(t, prepare.TargetToken, prepare.TargetToken)
}

func TestLobeHubSSOService_CompareCurrentConfigReturnsMismatchWhenObservedModelDiffers(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	now := time.Unix(1_712_400_000, 0).UTC()
	defaultKeyID := int64(9)
	svc := NewLobeHubSSOService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{
			LobeHubEnabled:              true,
			LobeHubChatURL:              "https://chat.example.com",
			LobeHubOIDCIssuer:           "https://sub2api.example.com/api/v1/lobehub/oidc",
			LobeHubOIDCClientID:         "lobehub-client",
			LobeHubOIDCClientSecret:     "lobehub-secret",
			LobeHubDefaultProvider:      "openai",
			LobeHubDefaultModel:         "gpt-4.1",
			LobeHubRuntimeConfigVersion: "runtime-v1",
			APIBaseURL:                  "https://api.example.com",
		}},
		&lobehubUserPreferenceStoreStub{
			user: &User{
				ID:                  42,
				Email:               "user@example.com",
				Username:            "alice",
				Status:              StatusActive,
				DefaultChatAPIKeyID: &defaultKeyID,
			},
		},
		&lobehubAPIKeyReaderStub{keys: map[int64]*APIKey{
			9: {
				ID:     9,
				UserID: 42,
				Key:    "sk-user-1",
				Status: StatusActive,
			},
		}},
		&lobehubOIDCStateStoreStub{},
		&lobehubOIDCSigningKeyProviderStub{privateKey: privateKey, keyID: "kid-1"},
		func() time.Time { return now },
	)

	prepare, err := svc.PrepareTargetRefresh(context.Background(), 42, &LobeHubSSORefreshRequest{
		ReturnURL: "https://chat.example.com/workspace",
	})
	require.NoError(t, err)

	result, err := svc.CompareCurrentConfig(context.Background(), prepare.TargetToken, &LobeHubObservedSettings{
		KeyVaults: map[string]LobeHubObservedKeyVault{
			"openai": {
				APIKey:  "sk-user-1",
				BaseURL: "https://api.example.com/v1",
			},
		},
		LanguageModel: map[string]LobeHubObservedLanguageModel{
			"openai": {
				Enabled:       true,
				EnabledModels: []string{"gpt-4o-mini"},
			},
		},
	})
	require.NoError(t, err)
	require.False(t, result.Matched)
}

func TestLobeHubSSOService_CompareCurrentConfigReturnsMismatchWhenConfiguredEnabledModelsAreMissing(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	now := time.Unix(1_712_400_000, 0).UTC()
	defaultKeyID := int64(9)
	svc := NewLobeHubSSOService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{
			LobeHubEnabled:              true,
			LobeHubChatURL:              "https://chat.example.com",
			LobeHubOIDCIssuer:           "https://sub2api.example.com/api/v1/lobehub/oidc",
			LobeHubOIDCClientID:         "lobehub-client",
			LobeHubOIDCClientSecret:     "lobehub-secret",
			LobeHubDefaultProvider:      "openai",
			LobeHubDefaultModel:         "gpt-5.4-mini",
			LobeHubEnabledModels:        []string{"gpt-5.4-mini", "gpt-image-2"},
			LobeHubRuntimeConfigVersion: "runtime-v1",
			APIBaseURL:                  "https://api.example.com",
		}},
		&lobehubUserPreferenceStoreStub{
			user: &User{
				ID:                  42,
				Email:               "user@example.com",
				Username:            "alice",
				Status:              StatusActive,
				DefaultChatAPIKeyID: &defaultKeyID,
			},
		},
		&lobehubAPIKeyReaderStub{keys: map[int64]*APIKey{
			9: {
				ID:     9,
				UserID: 42,
				Key:    "sk-user-1",
				Status: StatusActive,
			},
		}},
		&lobehubOIDCStateStoreStub{},
		&lobehubOIDCSigningKeyProviderStub{privateKey: privateKey, keyID: "kid-1"},
		func() time.Time { return now },
	)

	prepare, err := svc.PrepareTargetRefresh(context.Background(), 42, &LobeHubSSORefreshRequest{
		ReturnURL: "https://chat.example.com/workspace",
	})
	require.NoError(t, err)

	result, err := svc.CompareCurrentConfig(context.Background(), prepare.TargetToken, &LobeHubObservedSettings{
		KeyVaults: map[string]LobeHubObservedKeyVault{
			"openai": {
				APIKey:  "sk-user-1",
				BaseURL: "https://api.example.com/v1",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, result.Matched)
}
