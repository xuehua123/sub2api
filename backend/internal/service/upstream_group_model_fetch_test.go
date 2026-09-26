//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func groupModelHTTPResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
}

func groupModelFetchAccount(platform, typ string) *Account {
	return &Account{ID: 12, Platform: platform, Type: typ, Credentials: map[string]any{"api_key": "fixture-key", "base_url": "https://models.invalid"}}
}

func TestUpstreamGroupModelRejectsBusinessFailuresAndMalformedPages(t *testing.T) {
	for _, body := range []string{
		`{"success":false,"message":"private upstream detail","data":[]}`,
		`{"error":{"message":"private upstream detail"},"data":[]}`,
		`{"code":401,"data":[]}`, `{"code":"500","data":[]}`, `{"code":{"secret":"x"},"data":[]}`,
		`{"success":null,"data":[]}`, `{"success":"false","data":[]}`,
		`{"data":[{"id":"ok"},{}]}`, `{"data":[{"id":"ok"},null]}`, `{"data":null}`, `{}`, `null`,
	} {
		t.Run(body, func(t *testing.T) {
			u := &httpUpstreamRecorder{resp: groupModelHTTPResponse(body)}
			s := &AccountTestService{httpUpstream: u, cfg: upstreamModelSyncTestConfig()}
			models, err := s.FetchUpstreamGroupSupportedModels(context.Background(), groupModelFetchAccount(PlatformOpenAI, AccountTypeAPIKey))
			require.Error(t, err)
			require.Nil(t, models)
			require.NotContains(t, err.Error(), "private upstream detail")
		})
	}
	for _, body := range []string{`{"success":true,"data":[]}`, `{"code":0,"data":[]}`, `{"code":200,"data":[]}`, `{"error":null,"data":[]}`, `[]`} {
		models, _, _, err := parseUpstreamGroupModelPage([]byte(body), false)
		require.NoError(t, err)
		require.NotNil(t, models)
		require.Empty(t, models)
	}
}

func TestUpstreamGroupModelsFetchAllPages(t *testing.T) {
	for _, tc := range []struct {
		platform, first, last, cursorKey, cursor string
		want                                     []string
	}{
		{PlatformGemini, `{"models":[{"name":"models/gemini-first"}],"nextPageToken":"second +/=token"}`, `{"models":[{"name":"models/gemini-second"},{"name":"models/gemini-first"}]}`, "pageToken", "second +/=token", []string{"gemini-first", "gemini-second"}},
		{PlatformAnthropic, `{"data":[{"id":"claude-first"}],"has_more":true,"last_id":"claude-first"}`, `{"data":[{"id":"claude-second"}],"has_more":false,"last_id":"claude-second"}`, "after_id", "claude-first", []string{"claude-first", "claude-second"}},
	} {
		t.Run(tc.platform, func(t *testing.T) {
			u := &httpUpstreamRecorder{responses: []*http.Response{groupModelHTTPResponse(tc.first), groupModelHTTPResponse(tc.last)}}
			s := &AccountTestService{httpUpstream: u, cfg: upstreamModelSyncTestConfig()}
			proxyID := int64(1)
			a := groupModelFetchAccount(tc.platform, AccountTypeAPIKey)
			a.ProxyID = &proxyID
			a.Proxy = &Proxy{ID: 1, Protocol: "http", Host: "proxy.invalid", Port: 8080}
			models, err := s.FetchUpstreamGroupSupportedModels(context.Background(), a)
			require.NoError(t, err)
			require.Equal(t, tc.want, models)
			require.Len(t, u.requests, 2)
			require.Empty(t, u.requests[0].URL.Query().Get(tc.cursorKey))
			require.Equal(t, tc.cursor, u.requests[1].URL.Query().Get(tc.cursorKey))
			require.Equal(t, u.requests[0].URL.Host, u.requests[1].URL.Host)
			require.Equal(t, u.requests[0].URL.Path, u.requests[1].URL.Path)
			require.Equal(t, u.requests[0].Header, u.requests[1].Header)
			require.Equal(t, a.Proxy.URL(), u.lastProxyURL)
		})
	}
}

func TestUpstreamGroupModelsIncompletePaginationNeverPublishesPartialList(t *testing.T) {
	first := `{"data":[{"id":"first"}],"has_more":true,"last_id":"cursor-1"}`
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"business error", `{"success":false,"data":[]}`, 200},
		{"transport status", `{"error":"failure"}`, 502},
		{"malformed", `{"data":[{}]}`, 200},
		{"cursor loop", first, 200},
		{"missing cursor", `{"data":[{"id":"second"}],"has_more":true}`, 200},
		{"wrong cursor type", `{"data":[{"id":"second"}],"nextPageToken":42}`, 200},
		{"empty nonfinal page", `{"data":[],"has_more":true,"last_id":"cursor-2"}`, 200},
		{"conflicting protocols", `{"data":[{"id":"second"}],"nextPageToken":"x","has_more":true,"last_id":"y"}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			last := groupModelHTTPResponse(tc.body)
			last.StatusCode = tc.status
			u := &httpUpstreamRecorder{responses: []*http.Response{groupModelHTTPResponse(first), last}}
			s := &AccountTestService{httpUpstream: u, cfg: upstreamModelSyncTestConfig()}
			a := groupModelFetchAccount(PlatformAnthropic, AccountTypeAPIKey)
			models, err := s.FetchUpstreamGroupSupportedModels(context.Background(), a)
			require.Error(t, err)
			require.Nil(t, models)
			require.Len(t, u.requests, 2)
		})
	}
}

func TestUpstreamGroupModelsPaginationBudgets(t *testing.T) {
	t.Run("page limit", func(t *testing.T) {
		u := &httpUpstreamRecorder{}
		for i := 0; i < upstreamGroupModelMaxPages; i++ {
			u.responses = append(u.responses, groupModelHTTPResponse(fmt.Sprintf(`{"data":[{"id":"m-%d"}],"has_more":true,"last_id":"c-%d"}`, i, i)))
		}
		s := &AccountTestService{httpUpstream: u, cfg: upstreamModelSyncTestConfig()}
		models, err := s.FetchUpstreamGroupSupportedModels(context.Background(), groupModelFetchAccount(PlatformAnthropic, AccountTypeAPIKey))
		require.Error(t, err)
		require.Nil(t, models)
		require.Len(t, u.requests, upstreamGroupModelMaxPages)
	})
	t.Run("cumulative bytes", func(t *testing.T) {
		padding := strings.Repeat(" ", int(upstreamModelsBodyLimit/2))
		u := &httpUpstreamRecorder{responses: []*http.Response{
			groupModelHTTPResponse(`{"data":[{"id":"one"}],"has_more":true,"last_id":"one"}` + padding),
			groupModelHTTPResponse(`{"data":[{"id":"two"}],"has_more":false}` + padding),
		}}
		s := &AccountTestService{httpUpstream: u, cfg: upstreamModelSyncTestConfig()}
		models, err := s.FetchUpstreamGroupSupportedModels(context.Background(), groupModelFetchAccount(PlatformAnthropic, AccountTypeAPIKey))
		require.Error(t, err)
		require.Nil(t, models)
	})
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		u := &httpUpstreamRecorder{}
		s := &AccountTestService{httpUpstream: u, cfg: upstreamModelSyncTestConfig()}
		models, err := s.FetchUpstreamGroupSupportedModels(ctx, groupModelFetchAccount(PlatformOpenAI, AccountTypeAPIKey))
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, models)
		require.Empty(t, u.requests)
	})
}

func TestUpstreamGroupModelsSupportsPassthroughWithoutMutatingAccount(t *testing.T) {
	for _, platform := range []string{PlatformOpenAI, PlatformAnthropic, PlatformGemini, PlatformGrok, PlatformAntigravity} {
		t.Run(platform, func(t *testing.T) {
			a := groupModelFetchAccount(platform, AccountTypeUpstream)
			before, err := json.Marshal(a)
			require.NoError(t, err)
			u := &httpUpstreamRecorder{resp: groupModelHTTPResponse(`{"data":[{"id":"fixture-model"}]}`)}
			s := &AccountTestService{httpUpstream: u, cfg: upstreamModelSyncTestConfig()}
			models, err := s.FetchUpstreamGroupSupportedModels(context.Background(), a)
			require.NoError(t, err)
			require.Equal(t, []string{"fixture-model"}, models)
			require.Len(t, u.requests, 1)
			require.Equal(t, "models.invalid", u.lastReq.URL.Host)
			switch platform {
			case PlatformGemini:
				require.Equal(t, "fixture-key", u.lastReq.Header.Get("x-goog-api-key"))
			case PlatformAnthropic:
				require.Equal(t, "fixture-key", u.lastReq.Header.Get("x-api-key"))
			default:
				require.Equal(t, "Bearer fixture-key", u.lastReq.Header.Get("Authorization"))
			}
			if platform == PlatformAntigravity {
				require.Equal(t, "/v1/models", u.lastReq.URL.Path)
				require.Equal(t, "fixture-key", u.lastReq.Header.Get("x-api-key"))
			}
			after, err := json.Marshal(a)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after))
		})
	}
}

func TestUpstreamGroupModelFetchErrorProducesFailedSourceNotEmptySuccess(t *testing.T) {
	a := groupModelFetchAccount(PlatformOpenAI, AccountTypeAPIKey)
	repo := &groupModelSourceRepoStub{sources: []UpstreamGroupModelAccountSource{{Account: a, Fingerprint: "v1", KeyFingerprint: upstreamAPIKeyFingerprint("fixture-key")}}}
	u := &httpUpstreamRecorder{resp: groupModelHTTPResponse(`{"success":false,"data":[]}`)}
	s := NewUpstreamConnectionService(repo, nil, nil)
	s.modelFetcher = &AccountTestService{httpUpstream: u, cfg: upstreamModelSyncTestConfig()}
	snapshot := UpstreamGroupModels{}
	require.NoError(t, s.fetchBoundGroupModels(context.Background(), UpstreamGroupReference{ConnectionID: 1, RemoteKey: "id:1"}, 1, true, &snapshot))
	require.Len(t, snapshot.AccountResults, 1)
	require.False(t, snapshot.AccountResults[0].Success)
	require.Empty(t, snapshot.AccountResults[0].Models)
}
