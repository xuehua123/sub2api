//go:build unit

package repository

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildUpstreamTransportWithTLSFingerprintSetsTimeoutsForEveryRoute(t *testing.T) {
	tests := []struct {
		name     string
		proxyURL string
	}{
		{name: "direct"},
		{name: "http proxy", proxyURL: "http://127.0.0.1:8080"},
		{name: "socks5 proxy", proxyURL: "socks5h://127.0.0.1:1080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var proxyURL *url.URL
			if tt.proxyURL != "" {
				var err error
				proxyURL, err = url.Parse(tt.proxyURL)
				require.NoError(t, err)
			}

			transport, err := buildUpstreamTransportWithTLSFingerprint(defaultPoolSettings(nil), proxyURL, nil)
			require.NoError(t, err)
			require.NotNil(t, transport.DialContext)
			require.NotNil(t, transport.DialTLSContext)
			require.Equal(t, defaultUpstreamTLSHandshakeTimeout, transport.TLSHandshakeTimeout)
		})
	}
}
