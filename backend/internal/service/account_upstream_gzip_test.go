package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountIsUpstreamGzipEnabled_DefaultsOpenAIOAuthOff(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	}

	require.False(t, account.IsUpstreamGzipEnabled())
}

func TestAccountIsUpstreamGzipEnabled_ExtraOverridesDefault(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			AccountExtraUpstreamGzipEnabled: true,
		},
	}

	require.True(t, account.IsUpstreamGzipEnabled())
}

func TestAccountIsUpstreamGzipEnabled_DefaultsNonOAuthOn(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
	}

	require.True(t, account.IsUpstreamGzipEnabled())
}

func TestAccountOpenAIHTTPProtocolOverride(t *testing.T) {
	tests := []struct {
		name string
		raw  any
		want string
	}{
		{name: "h1", raw: "h1", want: OpenAIHTTPProtocolOverrideH1},
		{name: "http1.1", raw: "http/1.1", want: OpenAIHTTPProtocolOverrideH1},
		{name: "h2", raw: "h2", want: OpenAIHTTPProtocolOverrideH2},
		{name: "http2", raw: "http/2", want: OpenAIHTTPProtocolOverrideH2},
		{name: "invalid", raw: "spdy", want: ""},
		{name: "non-string", raw: true, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
				Extra: map[string]any{
					AccountExtraOpenAIHTTPProtocol: tt.raw,
				},
			}

			require.Equal(t, tt.want, account.OpenAIHTTPProtocolOverride())
		})
	}
}
