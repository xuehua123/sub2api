//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type upstreamManagementAuthTestEncryptor struct{}

func (upstreamManagementAuthTestEncryptor) Encrypt(plaintext string) (string, error) {
	return "encrypted:" + base64.RawStdEncoding.EncodeToString([]byte(plaintext)), nil
}

func (upstreamManagementAuthTestEncryptor) Decrypt(ciphertext string) (string, error) {
	const prefix = "encrypted:"
	if len(ciphertext) < len(prefix) || ciphertext[:len(prefix)] != prefix {
		return "", fmt.Errorf("unexpected ciphertext")
	}
	decoded, err := base64.RawStdEncoding.DecodeString(ciphertext[len(prefix):])
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

type upstreamManagementAuthNonceEncryptor struct {
	sequence atomic.Int64
}

func (e *upstreamManagementAuthNonceEncryptor) Encrypt(plaintext string) (string, error) {
	return fmt.Sprintf("nonce-%d:%s", e.sequence.Add(1), base64.RawStdEncoding.EncodeToString([]byte(plaintext))), nil
}

func (upstreamManagementAuthNonceEncryptor) Decrypt(ciphertext string) (string, error) {
	_, encoded, found := strings.Cut(ciphertext, ":")
	if !found {
		return "", fmt.Errorf("unexpected ciphertext")
	}
	decoded, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func TestUpstreamManagementAuthRoundTripAndCredentialRedaction(t *testing.T) {
	t.Parallel()

	config := UpstreamRateMultiplierSyncConfig{
		Provider:     UpstreamManagementProviderNewAPI,
		AuthMode:     UpstreamManagementAuthModeAccessToken,
		Group:        "plus",
		RemoteUserID: 42,
	}
	ciphertext, err := EncryptUpstreamManagementAuth(
		upstreamManagementAuthTestEncryptor{},
		config,
		&UpstreamManagementAuthInput{AccessToken: "management-token"},
	)
	if err != nil {
		t.Fatalf("EncryptUpstreamManagementAuth() error = %v", err)
	}

	got, err := DecryptUpstreamManagementAuth(upstreamManagementAuthTestEncryptor{}, ciphertext)
	if err != nil {
		t.Fatalf("DecryptUpstreamManagementAuth() error = %v", err)
	}
	if got.AccessToken != "management-token" {
		t.Fatalf("access token = %q, want management-token", got.AccessToken)
	}

	redacted, status := redactCredentialMapForTest(map[string]any{
		upstreamManagementAuthCredentialKey: ciphertext,
	})
	if len(redacted) != 0 {
		t.Fatalf("redacted credentials = %#v, want empty", redacted)
	}
	if !status["has_"+upstreamManagementAuthCredentialKey] {
		t.Fatalf("credential status = %#v, want management auth presence", status)
	}
}

func TestApplyUpstreamManagementAuthInputPreservesRefreshTokenWhenReplacingAccessToken(t *testing.T) {
	t.Parallel()

	config := UpstreamRateMultiplierSyncConfig{
		Provider: UpstreamManagementProviderSub2API,
		AuthMode: UpstreamManagementAuthModeAccessToken,
		Group:    "team",
	}
	ciphertext, err := EncryptUpstreamManagementAuth(
		upstreamManagementAuthTestEncryptor{},
		config,
		&UpstreamManagementAuthInput{
			AccessToken:  "old-access-token",
			RefreshToken: "existing-refresh-token",
		},
	)
	require.NoError(t, err)

	credentials, err := applyUpstreamManagementAuthInput(
		map[string]any{upstreamManagementAuthCredentialKey: ciphertext},
		config,
		&UpstreamManagementAuthInput{AccessToken: "replacement-access-token"},
		upstreamManagementAuthTestEncryptor{},
	)
	require.NoError(t, err)

	updated, err := DecryptUpstreamManagementAuth(
		upstreamManagementAuthTestEncryptor{},
		upstreamManagementAuthCiphertext(credentials),
	)
	require.NoError(t, err)
	require.Equal(t, "replacement-access-token", updated.AccessToken)
	require.Equal(t, "existing-refresh-token", updated.RefreshToken)
}

func TestValidateConfiguredUpstreamManagementAuthRejectsUnsupportedAccountType(t *testing.T) {
	t.Parallel()

	config := UpstreamRateMultiplierSyncConfig{
		Provider:     UpstreamManagementProviderNewAPI,
		AuthMode:     UpstreamManagementAuthModeAccessToken,
		Group:        "plus",
		RemoteUserID: 42,
	}
	ciphertext, err := EncryptUpstreamManagementAuth(upstreamManagementAuthTestEncryptor{}, config, &UpstreamManagementAuthInput{AccessToken: "management-token"})
	if err != nil {
		t.Fatalf("EncryptUpstreamManagementAuth() error = %v", err)
	}
	account := &Account{
		Type: AccountTypeOAuth,
		Credentials: map[string]any{
			upstreamManagementAuthCredentialKey: ciphertext,
		},
		Extra: map[string]any{
			AccountExtraUpstreamRateMultiplierSyncEnabled:      true,
			AccountExtraUpstreamRateMultiplierSyncGroup:        "plus",
			AccountExtraUpstreamRateMultiplierSyncProvider:     string(UpstreamManagementProviderNewAPI),
			AccountExtraUpstreamRateMultiplierSyncAuthMode:     string(UpstreamManagementAuthModeAccessToken),
			AccountExtraUpstreamRateMultiplierSyncRemoteUserID: 42,
		},
	}

	err = validateConfiguredUpstreamManagementAuth(account, upstreamManagementAuthTestEncryptor{})
	if err == nil || err.Error() != "upstream rate multiplier sync only supports apikey and upstream accounts" {
		t.Fatalf("validateConfiguredUpstreamManagementAuth() error = %v", err)
	}
}

func TestManagementBaseURLUsesDedicatedCredential(t *testing.T) {
	t.Parallel()

	syncer := NewUpstreamRateMultiplierSyncService(nil, nil, http.DefaultClient, nil, nil, time.Hour)
	account := &Account{Credentials: map[string]any{
		"base_url":                             "https://api.example.com/v1",
		UpstreamManagementBaseURLCredentialKey: "https://console.example.com",
	}}

	baseURL, err := syncer.managementBaseURL(account)
	require.NoError(t, err)
	require.Equal(t, "https://console.example.com", baseURL)
}

func TestApplyUpstreamManagementBaseURLInput(t *testing.T) {
	t.Parallel()

	credentials := map[string]any{"base_url": "https://api.example.com/v1"}
	managementURL := " https://console.example.com "
	updated := applyUpstreamManagementBaseURLInput(credentials, &managementURL)
	require.Equal(t, "https://console.example.com", updated[UpstreamManagementBaseURLCredentialKey])
	require.NotContains(t, credentials, UpstreamManagementBaseURLCredentialKey)

	empty := ""
	updated = applyUpstreamManagementBaseURLInput(updated, &empty)
	require.NotContains(t, updated, UpstreamManagementBaseURLCredentialKey)
	require.Equal(t, "https://api.example.com/v1", updated["base_url"])
}

func TestUpstreamRateMultiplierSyncUsesNewAPIManagementToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/self/groups" {
			t.Fatalf("path = %s, want /api/user/self/groups", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer management-token" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("New-API-User"); got != "42" {
			t.Fatalf("New-API-User = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"plus": map[string]any{"ratio": 0.5},
			},
		})
	}))
	defer server.Close()

	one := 1.0
	ciphertext, err := EncryptUpstreamManagementAuth(
		upstreamManagementAuthTestEncryptor{},
		UpstreamRateMultiplierSyncConfig{
			Provider:     UpstreamManagementProviderNewAPI,
			AuthMode:     UpstreamManagementAuthModeAccessToken,
			Group:        "plus",
			RemoteUserID: 42,
		},
		&UpstreamManagementAuthInput{AccessToken: "management-token"},
	)
	if err != nil {
		t.Fatalf("EncryptUpstreamManagementAuth() error = %v", err)
	}

	repo := &upstreamRateMultiplierSyncRepoStub{accounts: []Account{{
		ID:             1,
		Type:           AccountTypeAPIKey,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: &one,
		Credentials: map[string]any{
			"base_url":                             "http://127.0.0.1:1/v1",
			UpstreamManagementBaseURLCredentialKey: server.URL,
			upstreamManagementAuthCredentialKey:    ciphertext,
		},
		Extra: map[string]any{
			AccountExtraUpstreamRateMultiplierSyncEnabled:      true,
			AccountExtraUpstreamRateMultiplierSyncGroup:        "plus",
			AccountExtraUpstreamRateMultiplierSyncProvider:     string(UpstreamManagementProviderNewAPI),
			AccountExtraUpstreamRateMultiplierSyncAuthMode:     string(UpstreamManagementAuthModeAccessToken),
			AccountExtraUpstreamRateMultiplierSyncRemoteUserID: float64(42),
		},
	}}}

	syncer := NewUpstreamRateMultiplierSyncService(repo, nil, server.Client(), upstreamManagementAuthTestEncryptor{}, nil, time.Hour)
	updated, err := syncer.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if updated != 1 || repo.multiplierUpdates[1] != 0.5 {
		t.Fatalf("updates = %d, multipliers = %#v", updated, repo.multiplierUpdates)
	}
}

func TestUpstreamRateMultiplierSyncUsesSub2APIGroupRates(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sub2api-token" {
			t.Fatalf("missing management token")
		}
		switch r.URL.Path {
		case "/api/v1/groups/rates":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"7": 0.2}})
		case "/api/v1/groups/available":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": 7, "name": "team"}}})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	two := 2.0
	ciphertext, err := EncryptUpstreamManagementAuth(
		upstreamManagementAuthTestEncryptor{},
		UpstreamRateMultiplierSyncConfig{
			Provider: UpstreamManagementProviderSub2API,
			AuthMode: UpstreamManagementAuthModeAccessToken,
			Group:    "team",
		},
		&UpstreamManagementAuthInput{AccessToken: "sub2api-token"},
	)
	if err != nil {
		t.Fatalf("EncryptUpstreamManagementAuth() error = %v", err)
	}
	repo := &upstreamRateMultiplierSyncRepoStub{accounts: []Account{{
		ID:             1,
		Type:           AccountTypeAPIKey,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: &two,
		Credentials: map[string]any{
			"base_url":                          server.URL + "/v1",
			upstreamManagementAuthCredentialKey: ciphertext,
		},
		Extra: map[string]any{
			AccountExtraUpstreamRateMultiplierSyncEnabled:  true,
			AccountExtraUpstreamRateMultiplierSyncGroup:    "team",
			AccountExtraUpstreamRateMultiplierSyncProvider: string(UpstreamManagementProviderSub2API),
			AccountExtraUpstreamRateMultiplierSyncAuthMode: string(UpstreamManagementAuthModeAccessToken),
		},
	}}}

	syncer := NewUpstreamRateMultiplierSyncService(repo, nil, server.Client(), upstreamManagementAuthTestEncryptor{}, nil, time.Hour)
	updated, err := syncer.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if updated != 1 || repo.multiplierUpdates[1] != 0.2 {
		t.Fatalf("updates = %d, multipliers = %#v", updated, repo.multiplierUpdates)
	}
}

func TestUpstreamRateMultiplierSyncFallsBackToSub2APIAvailableGroupRates(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sub2api-token" {
			t.Fatalf("missing management token")
		}
		switch r.URL.Path {
		case "/api/v1/groups/rates":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{}})
		case "/api/v1/groups/available":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": []any{map[string]any{"id": 7, "name": "team", "rate_multiplier": 0.35}},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	one := 1.0
	ciphertext, err := EncryptUpstreamManagementAuth(
		upstreamManagementAuthTestEncryptor{},
		UpstreamRateMultiplierSyncConfig{
			Provider: UpstreamManagementProviderSub2API,
			AuthMode: UpstreamManagementAuthModeAccessToken,
			Group:    "team",
		},
		&UpstreamManagementAuthInput{AccessToken: "sub2api-token"},
	)
	if err != nil {
		t.Fatalf("EncryptUpstreamManagementAuth() error = %v", err)
	}
	repo := &upstreamRateMultiplierSyncRepoStub{accounts: []Account{{
		ID:             1,
		Type:           AccountTypeAPIKey,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: &one,
		Credentials: map[string]any{
			"base_url":                          server.URL,
			upstreamManagementAuthCredentialKey: ciphertext,
		},
		Extra: map[string]any{
			AccountExtraUpstreamRateMultiplierSyncEnabled:  true,
			AccountExtraUpstreamRateMultiplierSyncGroup:    "team",
			AccountExtraUpstreamRateMultiplierSyncProvider: string(UpstreamManagementProviderSub2API),
			AccountExtraUpstreamRateMultiplierSyncAuthMode: string(UpstreamManagementAuthModeAccessToken),
		},
	}}}

	syncer := NewUpstreamRateMultiplierSyncService(repo, nil, server.Client(), upstreamManagementAuthTestEncryptor{}, nil, time.Hour)
	updated, err := syncer.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if updated != 1 || repo.multiplierUpdates[1] != 0.35 {
		t.Fatalf("updates = %d, multipliers = %#v", updated, repo.multiplierUpdates)
	}
}

func TestUpstreamRateMultiplierSyncUsesNewAPIManagementPassword(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, "operator", body["username"])
			require.Equal(t, "password", body["password"])
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "session-token"})
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": 77}})
		case "/api/user/self/groups":
			require.Equal(t, "77", r.Header.Get("New-API-User"))
			require.Contains(t, r.Header.Get("Cookie"), "session=session-token")
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"team": map[string]any{"ratio": 0.3}}})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	assertUpstreamManagementSyncUpdate(t, server, UpstreamRateMultiplierSyncConfig{
		Provider: UpstreamManagementProviderNewAPI,
		AuthMode: UpstreamManagementAuthModePassword,
		Group:    "team",
	}, &UpstreamManagementAuthInput{Username: "operator", Password: "password"}, 0.3)
}

func TestUpstreamRateMultiplierSyncUsesProviderManagementHeaders(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		provider UpstreamManagementProvider
		group    string
		ratio    float64
	}{
		{name: "rixapi", provider: UpstreamManagementProviderRixAPI, group: "pro", ratio: 0.7},
		{name: "shellapi", provider: UpstreamManagementProviderShellAPI, group: "plus", ratio: 0.6},
		{name: "veloera", provider: UpstreamManagementProviderVeloera, group: "vip", ratio: 0.5},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer management-token", r.Header.Get("Authorization"))
				require.Equal(t, "42", r.Header.Get("New-API-User"))
				if test.provider == UpstreamManagementProviderRixAPI {
					require.Equal(t, "42", r.Header.Get("Rix-Api-User"))
				} else {
					require.Empty(t, r.Header.Get("Rix-Api-User"))
				}
				if test.provider == UpstreamManagementProviderVeloera {
					require.Equal(t, "42", r.Header.Get("Veloera-User"))
				} else {
					require.Empty(t, r.Header.Get("Veloera-User"))
				}
				switch r.URL.Path {
				case "/api/user/self/groups":
					w.WriteHeader(http.StatusNotFound)
				case "/api/pricing":
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"group_data": map[string]any{test.group: map[string]any{"GroupRatio": test.ratio}}}})
				default:
					t.Fatalf("unexpected path %s", r.URL.Path)
				}
			}))
			defer server.Close()

			assertUpstreamManagementSyncUpdate(t, server, UpstreamRateMultiplierSyncConfig{
				Provider:     test.provider,
				AuthMode:     UpstreamManagementAuthModeAccessToken,
				Group:        test.group,
				RemoteUserID: 42,
			}, &UpstreamManagementAuthInput{AccessToken: "management-token"}, test.ratio)
		})
	}
}

func TestUpstreamAuthenticationRejectionMessageMatchesOfficialNewAPIPasswordError(t *testing.T) {
	t.Parallel()

	require.True(t, isUpstreamAuthenticationRejectionMessage(
		"Username or password is incorrect, or user has been banned",
	))
	require.False(t, isUpstreamAuthenticationRejectionMessage("temporary upstream failure"))
}

func assertUpstreamManagementSyncUpdate(t *testing.T, server *httptest.Server, config UpstreamRateMultiplierSyncConfig, input *UpstreamManagementAuthInput, want float64) {
	t.Helper()
	ciphertext, err := EncryptUpstreamManagementAuth(upstreamManagementAuthTestEncryptor{}, config, input)
	require.NoError(t, err)
	one := 1.0
	repo := &upstreamRateMultiplierSyncRepoStub{accounts: []Account{{
		ID:             1,
		Type:           AccountTypeAPIKey,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: &one,
		Credentials: map[string]any{
			"base_url":                          server.URL + "/v1",
			upstreamManagementAuthCredentialKey: ciphertext,
		},
		Extra: map[string]any{
			AccountExtraUpstreamRateMultiplierSyncEnabled:      true,
			AccountExtraUpstreamRateMultiplierSyncGroup:        config.Group,
			AccountExtraUpstreamRateMultiplierSyncProvider:     string(config.Provider),
			AccountExtraUpstreamRateMultiplierSyncAuthMode:     string(config.AuthMode),
			AccountExtraUpstreamRateMultiplierSyncRemoteUserID: config.RemoteUserID,
		},
	}}}

	syncer := NewUpstreamRateMultiplierSyncService(repo, nil, server.Client(), upstreamManagementAuthTestEncryptor{}, nil, time.Hour)
	updated, err := syncer.Reconcile(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(1), updated)
	require.Equal(t, want, repo.multiplierUpdates[1])
}

func redactCredentialMapForTest(in map[string]any) (map[string]any, map[string]bool) {
	out := make(map[string]any)
	status := make(map[string]bool)
	for key, value := range in {
		if IsSensitiveCredentialKey(key) {
			status["has_"+key] = value != nil && value != ""
			continue
		}
		out[key] = value
	}
	return out, status
}
