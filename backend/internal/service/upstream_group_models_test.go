//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpstreamGroupModelClassification(t *testing.T) {
	require.Equal(t, []string{"Audio", "Claude", "Embedding", "GPT", "Gemini", "Grok", "Image", "Text", "Video"}, classifyUpstreamGroupModels([]string{
		"openai/gpt-5", "claude-sonnet-4-6", "gemini-2.5-pro", "grok-4", "gpt-image-2", "text-embedding-3-small", "whisper-1", "veo-3", "gpt-5",
	}))
	require.Equal(t, []string{"Gemini", "Text"}, classifyUpstreamGroupModels([]string{"gemini-2.5-pro"}), "image input must not imply image generation")
	require.Empty(t, classifyUpstreamGroupModels([]string{"custom-alias", "unknown"}))
	require.Equal(t, []string{"GPT", "Image"}, classifyUpstreamGroupModels([]string{"gpt-image-2"}))
}

func TestUpstreamGroupManagedModelParsing(t *testing.T) {
	var payload any
	require.NoError(t, json.Unmarshal([]byte(`{"selected_group_id":12,"models":[{"name":"gpt-5"},{"name":"claude-sonnet-4-6"}]}`), &payload))
	models, err := parseSub2APIGroupModelCatalog(payload, "12")
	require.NoError(t, err)
	require.Equal(t, []string{"claude-sonnet-4-6", "gpt-5"}, models)
	_, err = parseSub2APIGroupModelCatalog(payload, "13")
	require.Error(t, err, "upstream fallback to default group must not contaminate another group")
	_, err = parseSub2APIGroupModelCatalog(map[string]any{"selected_group_id": float64(12)}, "12")
	require.Error(t, err)
	require.NoError(t, json.Unmarshal([]byte(`[{"model_name":"gpt-5","enable_groups":["vip"]},{"model_name":"claude-sonnet-4-6","enable_groups":["default"]}]`), &payload))
	models, err = parseNewAPIGroupModelCatalog(payload, "vip")
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-5"}, models)
	_, err = parseNewAPIGroupModelCatalog([]any{map[string]any{"model_name": "gpt-5"}}, "vip")
	require.Error(t, err)
}

type groupModelFetchStub struct {
	calls int
	err   error
}

func (f *groupModelFetchStub) FetchUpstreamGroupSupportedModels(_ context.Context, _ *Account) ([]string, error) {
	f.calls++
	return []string{"gpt-5"}, f.err
}

type groupModelSourceRepoStub struct {
	UpstreamConnectionRepository
	UpstreamGroupModelRepository
	sources []UpstreamGroupModelAccountSource
	limit   int
	force   bool
}

func (r *groupModelSourceRepoStub) ListGroupModelAccountSources(_ context.Context, _ UpstreamGroupReference, _ int64, _ time.Time, limit int, force bool) ([]UpstreamGroupModelAccountSource, error) {
	r.limit, r.force = limit, force
	return r.sources, nil
}

func TestUpstreamGroupBoundModelsUseSourceSnapshotsWithoutAccountMutation(t *testing.T) {
	account := &Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
	repo := &groupModelSourceRepoStub{sources: []UpstreamGroupModelAccountSource{{Account: account, Fingerprint: "config-v1", KeyFingerprint: upstreamAPIKeyFingerprint("test-key")}}}
	s := NewUpstreamConnectionService(repo, nil, nil)
	fetch := &groupModelFetchStub{}
	s.modelFetcher = fetch
	snapshot := UpstreamGroupModels{Status: "error", ErrorCode: "unavailable"}
	ref := UpstreamGroupReference{ConnectionID: 1, RemoteKey: "id:12"}
	require.NoError(t, s.fetchBoundGroupModels(context.Background(), ref, 1, true, &snapshot))
	require.Equal(t, "bound_keys", snapshot.Coverage)
	require.Len(t, snapshot.AccountResults, 1)
	require.True(t, snapshot.AccountResults[0].Success)
	require.Equal(t, []string{"gpt-5"}, snapshot.AccountResults[0].Models)
	require.Equal(t, "config-v1", snapshot.AccountResults[0].Fingerprint)
	require.Equal(t, 1, fetch.calls)
	require.Equal(t, 8, repo.limit)
	require.True(t, repo.force)
	repo.sources[0].KeyFingerprint = "changed-key"
	snapshot = UpstreamGroupModels{Status: "error"}
	require.NoError(t, s.fetchBoundGroupModels(context.Background(), ref, 1, false, &snapshot))
	require.False(t, snapshot.AccountResults[0].Success)
	require.Equal(t, 1, fetch.calls)
	encoded, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "config-v1")
	require.NotContains(t, string(encoded), "test-key")
}

func TestUpstreamGroupModelClassificationUsesGatewayImageRules(t *testing.T) {
	for _, model := range []string{"gemini-2.5-flash-image", "gemini-2.5-flash-image-preview", "gemini-3-pro-image", "gemini-3.1-flash-image", "models/gemini-3.1-flash-image", "grok-imagine", "grok-imagine-edit"} {
		tags := classifyUpstreamGroupModels([]string{model})
		require.Contains(t, tags, "Image", model)
		require.NotContains(t, tags, "Text", model)
	}
	require.Equal(t, []string{"Rerank"}, classifyUpstreamGroupModels([]string{"bge-reranker-v2-m3"}))
	require.Equal(t, []string{"Embedding"}, classifyUpstreamGroupModels([]string{"bge-m3"}))
	require.Equal(t, []string{"Gemini", "Text"}, classifyUpstreamGroupModels([]string{"gemini-2.5-pro"}))
}

func TestUpstreamGroupModelEmptyCatalogIsExplicitAndDoesNotChangeAccountSync(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture-key", "base_url": "https://models.invalid"}}
	for _, body := range []string{`{"data":[]}`, `{"models":[]}`, `[]`, `{}`, `{"data":null}`, `{"data":[{"unexpected":"value"}]}`} {
		t.Run(body, func(t *testing.T) {
			validEmpty := body == `{"data":[]}` || body == `{"models":[]}` || body == `[]`
			for _, groupMode := range []bool{false, true} {
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}}
				svc := &AccountTestService{httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}
				var models []string
				var err error
				if groupMode {
					models, err = svc.FetchUpstreamGroupSupportedModels(context.Background(), account)
				} else {
					models, err = svc.FetchUpstreamSupportedModels(context.Background(), account)
				}
				if groupMode && validEmpty {
					require.NoError(t, err)
					require.NotNil(t, models)
					require.Empty(t, models)
				} else {
					require.Error(t, err)
				}
			}
		})
	}
}

func TestUpstreamGroupModelPublishedCatalogUsesSharedCache(t *testing.T) {
	var calls atomic.Int32
	var current atomic.Value
	current.Store("gpt-5")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, "/api/pricing", r.URL.Path)
		require.Equal(t, "Bearer fixture-token", r.Header.Get("Authorization"))
		require.Equal(t, "7", r.Header.Get("New-API-User"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"success":true,"data":[{"model_name":%q,"enable_groups":["main"]},{"model_name":"claude-sonnet-4-6","enable_groups":["backup"]}]}`, current.Load().(string))
	}))
	defer server.Close()
	enc := upstreamConnectionTestEncryptor{}
	secret, _ := json.Marshal(upstreamConnectionCredential{Version: 1, AccessToken: "fixture-token"})
	encrypted, err := enc.Encrypt(string(secret))
	require.NoError(t, err)
	s := NewUpstreamConnectionService(nil, enc, nil)
	c := &UpstreamConnection{ID: 1, Version: 1, Provider: "newapi", AuthMode: "access_token", ManagementBaseURL: server.URL, CredentialEncrypted: encrypted, RemoteUserID: "7"}
	models, source, err := s.fetchManagedGroupModels(context.Background(), c, UpstreamGroup{Name: "main"}, false)
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-5"}, models)
	require.Equal(t, "newapi:published_models", source)
	models, _, err = s.fetchManagedGroupModels(context.Background(), c, UpstreamGroup{Name: "backup"}, false)
	require.NoError(t, err)
	require.Equal(t, []string{"claude-sonnet-4-6"}, models)
	require.Equal(t, int32(1), calls.Load())
	current.Store("gpt-5-new")
	models, _, err = s.fetchManagedGroupModels(context.Background(), c, UpstreamGroup{Name: "main"}, true)
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-5-new"}, models)
	require.Equal(t, int32(2), calls.Load(), "manual refresh must bypass the five-minute cache")
	models, _, err = s.fetchManagedGroupModels(context.Background(), c, UpstreamGroup{Name: "main"}, false)
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-5-new"}, models)
	require.Equal(t, int32(2), calls.Load(), "ordinary reads share the newly refreshed catalog")
	c.Version = 2
	_, _, err = s.fetchManagedGroupModels(context.Background(), c, UpstreamGroup{Name: "main"}, false)
	require.NoError(t, err)
	require.Equal(t, int32(3), calls.Load(), "configuration changes invalidate the catalog cache")
}

type manualGroupModelRepo struct {
	UpstreamConnectionRepository
	UpstreamGroupModelRepository
	connection *UpstreamConnection
	result     UpstreamGroupModels
	manual     bool
}

func (r *manualGroupModelRepo) GetByID(context.Context, int64) (*UpstreamConnection, error) {
	return r.connection, nil
}
func (r *manualGroupModelRepo) ClaimGroupModels(_ context.Context, _ UpstreamGroupReference, _ int64, _ string, _ time.Time, force bool) (bool, bool, error) {
	return true, force || r.manual, nil
}
func (r *manualGroupModelRepo) SaveGroupModels(_ context.Context, _ UpstreamGroupReference, _ int64, _ string, value UpstreamGroupModels, now time.Time) (bool, error) {
	r.result = value
	r.result.ObservedAt = &now
	return true, nil
}
func (r *manualGroupModelRepo) GetGroupModels(context.Context, UpstreamGroupReference) (*UpstreamGroupModels, error) {
	result := r.result
	return &result, nil
}

func TestUpstreamGroupManualRefreshFetchesLivePublishedCatalog(t *testing.T) {
	var calls atomic.Int32
	var current atomic.Value
	current.Store("old-model")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = fmt.Fprintf(w, `{"success":true,"data":[{"model_name":%q,"enable_groups":["main"]}]}`, current.Load().(string))
	}))
	defer server.Close()
	enc := upstreamConnectionTestEncryptor{}
	secret, err := json.Marshal(upstreamConnectionCredential{Version: 1, AccessToken: "fixture-token"})
	require.NoError(t, err)
	encrypted, err := enc.Encrypt(string(secret))
	require.NoError(t, err)
	repo := &manualGroupModelRepo{connection: &UpstreamConnection{
		ID: 1, Version: 1, Provider: "newapi", AuthMode: "access_token", ManagementBaseURL: server.URL,
		RemoteUserID: "7", CredentialEncrypted: encrypted, Groups: []UpstreamGroup{{Name: "main"}},
	}}
	s := NewUpstreamConnectionService(repo, enc, nil)
	now := time.Now()
	s.now = func() time.Time { return now }
	ref := UpstreamGroupReference{ConnectionID: 1, RemoteKey: "name:main"}
	first, err := s.SyncGroupModels(context.Background(), ref, true)
	require.NoError(t, err)
	require.Equal(t, []string{"old-model"}, first.Models)
	current.Store("new-model")
	now = now.Add(31 * time.Second)
	second, err := s.SyncGroupModels(context.Background(), ref, true)
	require.NoError(t, err)
	require.Equal(t, int32(2), calls.Load())
	require.Equal(t, []string{"new-model"}, second.Models)
	require.True(t, second.ObservedAt.After(*first.ObservedAt))
	// A background worker resuming the persisted manual batch also needs a live fetch.
	repo.manual = true
	current.Store("resumed-model")
	now = now.Add(31 * time.Second)
	resumed, err := s.SyncGroupModels(context.Background(), ref, false)
	require.NoError(t, err)
	require.Equal(t, int32(3), calls.Load())
	require.Equal(t, []string{"resumed-model"}, resumed.Models)
}

func TestUpstreamGroupModelPublishedCatalogRejectsRedirect(t *testing.T) {
	var destinationCalls atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { destinationCalls.Add(1) }))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
	defer origin.Close()
	enc := upstreamConnectionTestEncryptor{}
	secret, _ := json.Marshal(upstreamConnectionCredential{Version: 1, AccessToken: "fixture-token"})
	encrypted, err := enc.Encrypt(string(secret))
	require.NoError(t, err)
	s := NewUpstreamConnectionService(nil, enc, nil)
	c := &UpstreamConnection{ID: 1, Version: 1, Provider: "sub2api", AuthMode: "access_token", ManagementBaseURL: origin.URL, CredentialEncrypted: encrypted}
	_, _, err = s.fetchManagedGroupModels(context.Background(), c, UpstreamGroup{RemoteID: "12"}, false)
	require.Error(t, err)
	require.Zero(t, destinationCalls.Load(), "credentials must not follow redirects")
}

func TestUpstreamGroupModelSnapshotBounds(t *testing.T) {
	require.True(t, validUpstreamModelSnapshot([]string{"normal-model"}))
	require.False(t, validUpstreamModelSnapshot([]string{"model\nheader"}))
	require.False(t, validUpstreamModelSnapshot(make([]string, 10001)))
}
