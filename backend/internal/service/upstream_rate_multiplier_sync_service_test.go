//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type upstreamRateMultiplierSyncRepoStub struct {
	accounts          []Account
	multiplierUpdates map[int64]float64
	priorityUpdates   map[int64]int
}

func (r *upstreamRateMultiplierSyncRepoStub) ListActiveSchedulableForRateMultiplierPriority(context.Context) ([]Account, error) {
	return append([]Account(nil), r.accounts...), nil
}

func (r *upstreamRateMultiplierSyncRepoStub) UpdateRateMultipliers(_ context.Context, multipliers map[int64]float64) (int64, error) {
	r.multiplierUpdates = make(map[int64]float64, len(multipliers))
	for id, multiplier := range multipliers {
		r.multiplierUpdates[id] = multiplier
		for index := range r.accounts {
			if r.accounts[index].ID != id {
				continue
			}
			value := multiplier
			r.accounts[index].RateMultiplier = &value
		}
	}
	return int64(len(multipliers)), nil
}

func (r *upstreamRateMultiplierSyncRepoStub) UpdateRateMultiplierPriorities(_ context.Context, priorities map[int64]int) (int64, error) {
	r.priorityUpdates = make(map[int64]int, len(priorities))
	for id, priority := range priorities {
		r.priorityUpdates[id] = priority
	}
	return int64(len(priorities)), nil
}

func TestUpstreamRateMultiplierSyncReconcileDeduplicatesEndpointAndReconcilesPriority(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/self/groups" {
			t.Fatalf("path = %s, want /api/user/self/groups", r.URL.Path)
		}
		requests.Add(1)
		_, _ = w.Write([]byte(`{"success":true,"data":{"plus":{"ratio":0.5},"team":{"ratio":2}}}`))
	}))
	defer server.Close()

	one := 1.0
	two := 2.0
	ciphertext, err := EncryptUpstreamManagementAuth(upstreamManagementAuthTestEncryptor{}, UpstreamRateMultiplierSyncConfig{Provider: UpstreamManagementProviderNewAPI, AuthMode: UpstreamManagementAuthModeAccessToken, Group: "plus", RemoteUserID: 42}, &UpstreamManagementAuthInput{AccessToken: "management-token"})
	if err != nil {
		t.Fatalf("EncryptUpstreamManagementAuth() error = %v", err)
	}
	repo := &upstreamRateMultiplierSyncRepoStub{accounts: []Account{
		{ID: 1, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, RateMultiplier: &one, Credentials: map[string]any{"base_url": server.URL + "/v1", upstreamManagementAuthCredentialKey: ciphertext}, Extra: map[string]any{AccountExtraUpstreamRateMultiplierSyncEnabled: true, AccountExtraUpstreamRateMultiplierSyncGroup: "plus", AccountExtraUpstreamRateMultiplierSyncProvider: string(UpstreamManagementProviderNewAPI), AccountExtraUpstreamRateMultiplierSyncAuthMode: string(UpstreamManagementAuthModeAccessToken), AccountExtraUpstreamRateMultiplierSyncRemoteUserID: float64(42)}},
		{ID: 2, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, RateMultiplier: &one, Credentials: map[string]any{"base_url": server.URL + "/v1", upstreamManagementAuthCredentialKey: ciphertext}, Extra: map[string]any{AccountExtraUpstreamRateMultiplierSyncEnabled: true, AccountExtraUpstreamRateMultiplierSyncGroup: "plus", AccountExtraUpstreamRateMultiplierSyncProvider: string(UpstreamManagementProviderNewAPI), AccountExtraUpstreamRateMultiplierSyncAuthMode: string(UpstreamManagementAuthModeAccessToken), AccountExtraUpstreamRateMultiplierSyncRemoteUserID: float64(42)}},
		{ID: 3, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, RateMultiplier: &two, Credentials: map[string]any{"base_url": server.URL, upstreamManagementAuthCredentialKey: ciphertext}, Extra: map[string]any{AccountExtraUpstreamRateMultiplierSyncEnabled: true, AccountExtraUpstreamRateMultiplierSyncGroup: "team", AccountExtraUpstreamRateMultiplierSyncProvider: string(UpstreamManagementProviderNewAPI), AccountExtraUpstreamRateMultiplierSyncAuthMode: string(UpstreamManagementAuthModeAccessToken), AccountExtraUpstreamRateMultiplierSyncRemoteUserID: float64(42)}},
		{ID: 4, Type: AccountTypeAPIKey, Status: StatusDisabled, Schedulable: true, RateMultiplier: &one, Credentials: map[string]any{"base_url": server.URL, upstreamManagementAuthCredentialKey: ciphertext}, Extra: map[string]any{AccountExtraUpstreamRateMultiplierSyncEnabled: true, AccountExtraUpstreamRateMultiplierSyncGroup: "plus", AccountExtraUpstreamRateMultiplierSyncProvider: string(UpstreamManagementProviderNewAPI), AccountExtraUpstreamRateMultiplierSyncAuthMode: string(UpstreamManagementAuthModeAccessToken), AccountExtraUpstreamRateMultiplierSyncRemoteUserID: float64(42)}},
	}}
	priority := NewRateMultiplierPriorityService(repo, rateMultiplierPrioritySettingsStub{enabled: true}, time.Hour)
	syncer := NewUpstreamRateMultiplierSyncService(repo, nil, server.Client(), upstreamManagementAuthTestEncryptor{}, priority, time.Hour)

	updated, err := syncer.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if updated != 2 {
		t.Fatalf("updated = %d, want 2", updated)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2 distinct endpoint/group probes", requests.Load())
	}
	if got := repo.multiplierUpdates[1]; got != 0.5 {
		t.Fatalf("account 1 multiplier = %v, want 0.5", got)
	}
	if got := repo.multiplierUpdates[2]; got != 0.5 {
		t.Fatalf("account 2 multiplier = %v, want 0.5", got)
	}
	if _, changed := repo.multiplierUpdates[3]; changed {
		t.Fatalf("account 3 multiplier should remain unchanged")
	}
	if got := repo.priorityUpdates[1]; got != rateMultiplierPriorityBase {
		t.Fatalf("account 1 priority = %d, want %d", got, rateMultiplierPriorityBase)
	}
	if got := repo.priorityUpdates[2]; got != rateMultiplierPriorityBase {
		t.Fatalf("account 2 priority = %d, want %d", got, rateMultiplierPriorityBase)
	}
	if got := repo.priorityUpdates[3]; got != rateMultiplierPriorityBase+DefaultRateMultiplierPriorityStep {
		t.Fatalf("account 3 priority = %d, want %d", got, rateMultiplierPriorityBase+DefaultRateMultiplierPriorityStep)
	}
}

func TestUpstreamRateMultiplierSyncFailurePreservesExistingMultiplier(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	one := 1.0
	ciphertext, err := EncryptUpstreamManagementAuth(upstreamManagementAuthTestEncryptor{}, UpstreamRateMultiplierSyncConfig{Provider: UpstreamManagementProviderNewAPI, AuthMode: UpstreamManagementAuthModeAccessToken, Group: "plus", RemoteUserID: 42}, &UpstreamManagementAuthInput{AccessToken: "management-token"})
	if err != nil {
		t.Fatalf("EncryptUpstreamManagementAuth() error = %v", err)
	}
	repo := &upstreamRateMultiplierSyncRepoStub{accounts: []Account{
		{ID: 1, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, RateMultiplier: &one, Credentials: map[string]any{"base_url": server.URL, upstreamManagementAuthCredentialKey: ciphertext}, Extra: map[string]any{AccountExtraUpstreamRateMultiplierSyncEnabled: true, AccountExtraUpstreamRateMultiplierSyncGroup: "plus", AccountExtraUpstreamRateMultiplierSyncProvider: string(UpstreamManagementProviderNewAPI), AccountExtraUpstreamRateMultiplierSyncAuthMode: string(UpstreamManagementAuthModeAccessToken), AccountExtraUpstreamRateMultiplierSyncRemoteUserID: float64(42)}},
	}}
	syncer := NewUpstreamRateMultiplierSyncService(repo, nil, server.Client(), upstreamManagementAuthTestEncryptor{}, nil, time.Hour)

	updated, err := syncer.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if updated != 0 || len(repo.multiplierUpdates) != 0 {
		t.Fatalf("failed sync changed multipliers: updated=%d changes=%#v", updated, repo.multiplierUpdates)
	}
}

func TestUpstreamRateMultiplierSyncSkipsUnsupportedAccountTypes(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"success":true,"data":{"group_ratio":{"plus":0.5}}}`))
	}))
	defer server.Close()

	one := 1.0
	repo := &upstreamRateMultiplierSyncRepoStub{accounts: []Account{
		{ID: 1, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, RateMultiplier: &one, Credentials: map[string]any{"base_url": server.URL}, Extra: map[string]any{AccountExtraUpstreamRateMultiplierSyncEnabled: true, AccountExtraUpstreamRateMultiplierSyncGroup: "plus"}},
	}}
	syncer := NewUpstreamRateMultiplierSyncService(repo, nil, server.Client(), upstreamManagementAuthTestEncryptor{}, nil, time.Hour)

	updated, err := syncer.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if updated != 0 || requests.Load() != 0 || len(repo.multiplierUpdates) != 0 {
		t.Fatalf("unsupported account type was synchronized: updated=%d requests=%d changes=%#v", updated, requests.Load(), repo.multiplierUpdates)
	}
}

func TestNormalizeUpstreamRateMultiplierSyncExtra(t *testing.T) {
	normalized, err := NormalizeUpstreamRateMultiplierSyncExtra(map[string]any{
		AccountExtraUpstreamRateMultiplierSyncEnabled:      true,
		AccountExtraUpstreamRateMultiplierSyncGroup:        "  plus  ",
		AccountExtraUpstreamRateMultiplierSyncProvider:     string(UpstreamManagementProviderNewAPI),
		AccountExtraUpstreamRateMultiplierSyncAuthMode:     string(UpstreamManagementAuthModeAccessToken),
		AccountExtraUpstreamRateMultiplierSyncRemoteUserID: 42,
	})
	if err != nil {
		t.Fatalf("NormalizeUpstreamRateMultiplierSyncExtra() error = %v", err)
	}
	if got := normalized[AccountExtraUpstreamRateMultiplierSyncGroup]; got != "plus" {
		t.Fatalf("group = %#v, want plus", got)
	}

	if _, err := NormalizeUpstreamRateMultiplierSyncExtra(map[string]any{
		AccountExtraUpstreamRateMultiplierSyncEnabled: true,
	}); err == nil {
		t.Fatal("enabled sync without group should fail")
	}
}

func TestUpstreamRateMultiplierDiscoveryDetectsNewAPIAndListsGroups(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			w.WriteHeader(http.StatusNotFound)
		case "/api/user/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "management-session"})
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":42,"access_token":"management-token"}}`))
		case "/api/user/self/groups":
			if cookie, err := r.Cookie("session"); err != nil || cookie.Value != "management-session" {
				t.Fatal("expected upstream management session cookie")
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"team":{"ratio":2},"plus":{"ratio":0.5}}}`))
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	syncer := NewUpstreamRateMultiplierSyncService(nil, nil, server.Client(), upstreamManagementAuthTestEncryptor{}, nil, time.Hour)
	account := &Account{Credentials: map[string]any{"base_url": server.URL + "/v1"}}
	discovery, err := syncer.DiscoverGroups(context.Background(), account, UpstreamManagementAuthModePassword, 0, &UpstreamManagementAuthInput{
		Username: "manager@example.com",
		Password: "management-password",
	})
	if err != nil {
		t.Fatalf("DiscoverGroups() error = %v", err)
	}
	if discovery.Provider != UpstreamManagementProviderNewAPI || discovery.AuthMode != UpstreamManagementAuthModePassword {
		t.Fatalf("discovery adapter = %#v", discovery)
	}
	if discovery.RemoteUserID != 42 {
		t.Fatalf("remote user id = %d, want 42", discovery.RemoteUserID)
	}
	if got := discovery.Groups; len(got) != 2 || got[0].Name != "plus" || got[0].RateMultiplier != 0.5 || got[1].Name != "team" || got[1].RateMultiplier != 2 {
		t.Fatalf("groups = %#v", got)
	}
	if _, persisted := account.Credentials[upstreamManagementAuthCredentialKey]; persisted {
		t.Fatal("discovery must not persist plaintext credentials")
	}
}

func TestUpstreamRateMultiplierDiscoveryReusesNewAPILoginAcrossProviderHeaderProbes(t *testing.T) {
	var newAPILogins int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			w.WriteHeader(http.StatusNotFound)
		case "/api/user/login":
			newAPILogins++
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "management-session"})
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":42,"access_token":"management-token"}}`))
		case "/api/user/self/groups":
			if r.Header.Get("Rix-Api-User") != "42" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"plus":{"ratio":0.5}}}`))
		case "/api/pricing":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	syncer := NewUpstreamRateMultiplierSyncService(nil, nil, server.Client(), upstreamManagementAuthTestEncryptor{}, nil, time.Hour)
	account := &Account{Credentials: map[string]any{"base_url": server.URL + "/v1"}}
	discovery, err := syncer.DiscoverGroups(context.Background(), account, UpstreamManagementAuthModePassword, 0, &UpstreamManagementAuthInput{
		Username: "manager@example.com",
		Password: "management-password",
	})
	if err != nil {
		t.Fatalf("DiscoverGroups() error = %v", err)
	}
	if discovery.Provider != UpstreamManagementProviderRixAPI {
		t.Fatalf("provider = %q, want rixapi", discovery.Provider)
	}
	if newAPILogins != 1 {
		t.Fatalf("NewAPI login attempts = %d, want 1", newAPILogins)
	}
}

func TestUpstreamRateMultiplierDiscoveryUsesStoredCredentialsForExistingAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer management-token" {
			t.Fatalf("authorization = %q", got)
		}
		switch r.URL.Path {
		case "/api/v1/groups/rates":
			w.WriteHeader(http.StatusNotFound)
		case "/api/v1/groups/available":
			_, _ = w.Write([]byte(`{"success":true,"data":[{"name":"plus","rate_multiplier":0.5},{"name":"team","rate_multiplier":1}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	ciphertext, err := EncryptUpstreamManagementAuth(upstreamManagementAuthTestEncryptor{}, UpstreamRateMultiplierSyncConfig{
		Provider: UpstreamManagementProviderSub2API,
		AuthMode: UpstreamManagementAuthModeAccessToken,
		Group:    "plus",
	}, &UpstreamManagementAuthInput{AccessToken: "management-token"})
	if err != nil {
		t.Fatalf("EncryptUpstreamManagementAuth() error = %v", err)
	}
	syncer := NewUpstreamRateMultiplierSyncService(nil, nil, server.Client(), upstreamManagementAuthTestEncryptor{}, nil, time.Hour)
	account := &Account{Credentials: map[string]any{
		"base_url":                          server.URL,
		upstreamManagementAuthCredentialKey: ciphertext,
	}}
	discovery, err := syncer.DiscoverGroups(context.Background(), account, UpstreamManagementAuthModeAccessToken, 0, nil)
	if err != nil {
		t.Fatalf("DiscoverGroups() error = %v", err)
	}
	if discovery.Provider != UpstreamManagementProviderSub2API || len(discovery.Groups) != 2 {
		t.Fatalf("discovery = %#v", discovery)
	}
}
