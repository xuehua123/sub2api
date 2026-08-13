//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type grokNativeSearchUpstreamStub struct {
	HTTPUpstream
	response *http.Response
	err      error
	request  *http.Request
}

func (s *grokNativeSearchUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.request = req
	return s.response, s.err
}

type grokNativeSearchReadErrorBody struct{}

func (grokNativeSearchReadErrorBody) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (grokNativeSearchReadErrorBody) Close() error             { return nil }

func grokNativeSearchPoolAccount(poolMode bool, retryCodes ...any) *Account {
	credentials := map[string]any{
		"api_key":   "xai-test",
		"base_url":  "https://api.x.ai/v1",
		"pool_mode": poolMode,
	}
	if retryCodes != nil {
		credentials["pool_mode_retry_status_codes"] = retryCodes
	}
	return &Account{
		ID:          501,
		Platform:    PlatformGrok,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: credentials,
	}
}

func TestDoGrokNativeResponsesJSONMarksOnlyConfiguredPoolErrorsForSameAccountRetry(t *testing.T) {
	tests := []struct {
		name        string
		account     *Account
		response    *http.Response
		upstreamErr error
		wantRetry   bool
	}{
		{
			name:        "pool transport 502",
			account:     grokNativeSearchPoolAccount(true, float64(http.StatusBadGateway)),
			upstreamErr: errors.New("dial failed"),
			wantRetry:   true,
		},
		{
			name:    "pool read 502",
			account: grokNativeSearchPoolAccount(true, float64(http.StatusBadGateway)),
			response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       grokNativeSearchReadErrorBody{},
			},
			wantRetry: true,
		},
		{
			name:    "pool configured status",
			account: grokNativeSearchPoolAccount(true, float64(http.StatusServiceUnavailable)),
			response: &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(strings.NewReader(`{"error":"temporary"}`)),
			},
			wantRetry: true,
		},
		{
			name:    "pool unconfigured status",
			account: grokNativeSearchPoolAccount(true, float64(http.StatusServiceUnavailable)),
			response: &http.Response{
				StatusCode: http.StatusBadGateway,
				Body:       io.NopCloser(strings.NewReader(`{"error":"temporary"}`)),
			},
			wantRetry: false,
		},
		{
			name:    "non pool status",
			account: grokNativeSearchPoolAccount(false, float64(http.StatusBadGateway)),
			response: &http.Response{
				StatusCode: http.StatusBadGateway,
				Body:       io.NopCloser(strings.NewReader(`{"error":"temporary"}`)),
			},
			wantRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &grokNativeSearchUpstreamStub{response: tt.response, err: tt.upstreamErr}
			svc := &OpenAIGatewayService{httpUpstream: upstream}

			_, err := svc.DoGrokNativeResponsesJSON(context.Background(), tt.account, []byte(`{"model":"grok-4.1-fast"}`))

			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, tt.wantRetry, failoverErr.RetryableOnSameAccount)
		})
	}
}

func TestDoGrokNativeResponsesJSONCredentialFailureDoesNotRetrySameAccount(t *testing.T) {
	account := grokNativeSearchPoolAccount(true, float64(http.StatusUnauthorized))
	delete(account.Credentials, "api_key")
	svc := &OpenAIGatewayService{httpUpstream: &grokNativeSearchUpstreamStub{}}

	_, err := svc.DoGrokNativeResponsesJSON(context.Background(), account, []byte(`{}`))

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.False(t, failoverErr.RetryableOnSameAccount)
}

func TestDoGrokNativeResponsesJSONAppliesSelectedAccountModelMapping(t *testing.T) {
	account := grokNativeSearchPoolAccount(false)
	account.Credentials["model_mapping"] = map[string]any{"grok-4.5": "grok-4.6-latest"}
	upstream := &grokNativeSearchUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"output":[]}`)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	_, err := svc.DoGrokNativeResponsesJSON(context.Background(), account, []byte(`{"model":"grok-4.5","input":"query"}`))

	require.NoError(t, err)
	require.NotNil(t, upstream.request)
	require.Equal(t, "grok-4.6", gjson.GetBytes(readRequestBody(t, upstream.request), "model").String())
}

func TestDoGrokNativeResponsesJSONReconcilesSuccessfulQuotaHeaders(t *testing.T) {
	account := grokNativeSearchPoolAccount(false)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	headers := make(http.Header)
	headers.Set("x-ratelimit-limit-requests", "100")
	headers.Set("x-ratelimit-remaining-requests", "25")
	upstream := &grokNativeSearchUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(`{"output":[]}`)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream, accountRepo: repo}

	_, err := svc.DoGrokNativeResponsesJSON(context.Background(), account, []byte(`{"model":"grok-4.5","input":"query"}`))

	require.NoError(t, err)
	require.Contains(t, repo.updates[account.ID], grokQuotaSnapshotExtraKey)
	snapshot, ok := repo.updates[account.ID][grokQuotaSnapshotExtraKey].(*xai.QuotaSnapshot)
	require.True(t, ok)
	require.Equal(t, "grok-4.5", snapshot.Model)
}

func TestDoGrokNativeResponsesJSONReconcilesRateLimitError(t *testing.T) {
	account := grokNativeSearchPoolAccount(false)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	headers := make(http.Header)
	headers.Set("x-ratelimit-limit-requests", "100")
	headers.Set("x-ratelimit-remaining-requests", "0")
	headers.Set("x-ratelimit-reset-requests", "60")
	upstream := &grokNativeSearchUpstreamStub{response: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited"}}`)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream, accountRepo: repo}

	_, err := svc.DoGrokNativeResponsesJSON(context.Background(), account, []byte(`{"model":"grok-4.5","input":"query"}`))

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Contains(t, repo.updates[account.ID], grokQuotaSnapshotExtraKey)
	require.Positive(t, repo.rateLimitedCalls)
}

func TestDoGrokNativeResponsesJSONFailsOverOnlyExplicitUnavailable400(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantFailover bool
	}{
		{name: "unknown provider", body: `{"error":{"message":"unknown provider for model gpt-5.5"}}`, wantFailover: true},
		{name: "structured model not found", body: `{"error":{"code":"model_not_found","message":"unavailable"}}`, wantFailover: true},
		{name: "ordinary validation", body: `{"error":{"message":"input must be a string"}}`, wantFailover: false},
		{name: "content policy", body: `{"error":{"message":"prompt violates content policy"}}`, wantFailover: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := grokNativeSearchPoolAccount(true, float64(http.StatusBadRequest))
			upstream := &grokNativeSearchUpstreamStub{response: &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}}
			svc := &OpenAIGatewayService{httpUpstream: upstream}

			_, err := svc.DoGrokNativeResponsesJSON(context.Background(), account, []byte(`{"model":"grok-4.5"}`))

			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			if tt.wantFailover {
				require.ErrorAs(t, err, &failoverErr)
				require.False(t, failoverErr.RetryableOnSameAccount, "deterministic provider/model errors must switch accounts immediately")
				return
			}
			require.False(t, errors.As(err, &failoverErr))
		})
	}
}

func readRequestBody(t *testing.T, req *http.Request) []byte {
	t.Helper()
	require.NotNil(t, req)
	require.NotNil(t, req.Body)
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	return body
}
