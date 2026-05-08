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
