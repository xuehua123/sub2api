//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type grokNativeSearchUpstreamStub struct {
	HTTPUpstream
	response *http.Response
	err      error
}

func (s *grokNativeSearchUpstreamStub) Do(*http.Request, string, int64, int) (*http.Response, error) {
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
			svc := &GatewayService{httpUpstream: upstream}

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
	svc := &GatewayService{httpUpstream: &grokNativeSearchUpstreamStub{}}

	_, err := svc.DoGrokNativeResponsesJSON(context.Background(), account, []byte(`{}`))

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.False(t, failoverErr.RetryableOnSameAccount)
}
