//go:build unit

package tlsfingerprint

import (
	"context"
	"errors"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const fingerprintTimeoutTestLimit = 50 * time.Millisecond

type tlsContextDialer interface {
	DialTLSContext(ctx context.Context, network, addr string) (net.Conn, error)
}

func TestDirectDialerAppliesDialTimeout(t *testing.T) {
	dialer := NewDialerWithOptions(nil, DialerOptions{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
		DialTimeout:         fingerprintTimeoutTestLimit,
		TLSHandshakeTimeout: time.Second,
	})

	started := time.Now()
	conn, err := dialer.DialTLSContext(context.Background(), "tcp", "example.com:443")
	if conn != nil {
		_ = conn.Close()
	}
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), time.Second)
}

func TestDialerAppliesTLSHandshakeTimeout(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = serverConn.Close() })

	dialer := NewDialerWithOptions(nil, DialerOptions{
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return clientConn, nil
		},
		DialTimeout:         time.Second,
		TLSHandshakeTimeout: fingerprintTimeoutTestLimit,
	})

	started := time.Now()
	conn, err := dialer.DialTLSContext(context.Background(), "tcp", "example.com:443")
	if conn != nil {
		_ = conn.Close()
	}
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), time.Second)
}

func TestProxyDialersApplyTunnelTimeout(t *testing.T) {
	tests := []struct {
		name      string
		proxyURL  string
		newDialer func(*url.URL, DialerOptions) tlsContextDialer
	}{
		{
			name:     "http",
			proxyURL: "http://proxy.example:8080",
			newDialer: func(proxyURL *url.URL, options DialerOptions) tlsContextDialer {
				return NewHTTPProxyDialerWithOptions(nil, proxyURL, options)
			},
		},
		{
			name:     "socks5",
			proxyURL: "socks5://proxy.example:1080",
			newDialer: func(proxyURL *url.URL, options DialerOptions) tlsContextDialer {
				return NewSOCKS5ProxyDialerWithOptions(nil, proxyURL, options)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = serverConn.Close() })
			proxyURL, err := url.Parse(tt.proxyURL)
			require.NoError(t, err)

			dialer := tt.newDialer(proxyURL, DialerOptions{
				DialContext: func(context.Context, string, string) (net.Conn, error) {
					return clientConn, nil
				},
				DialTimeout:         fingerprintTimeoutTestLimit,
				TLSHandshakeTimeout: time.Second,
			})

			started := time.Now()
			conn, err := dialer.DialTLSContext(context.Background(), "tcp", "example.com:443")
			if conn != nil {
				_ = conn.Close()
			}
			require.Error(t, err)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Less(t, time.Since(started), time.Second)
		})
	}
}

func TestProxyDialersPreserveContextCancellation(t *testing.T) {
	tests := []struct {
		name      string
		proxyURL  string
		newDialer func(*url.URL, DialerOptions) tlsContextDialer
	}{
		{
			name:     "http",
			proxyURL: "http://proxy.example:8080",
			newDialer: func(proxyURL *url.URL, options DialerOptions) tlsContextDialer {
				return NewHTTPProxyDialerWithOptions(nil, proxyURL, options)
			},
		},
		{
			name:     "socks5",
			proxyURL: "socks5://proxy.example:1080",
			newDialer: func(proxyURL *url.URL, options DialerOptions) tlsContextDialer {
				return NewSOCKS5ProxyDialerWithOptions(nil, proxyURL, options)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = serverConn.Close() })
			proxyURL, err := url.Parse(tt.proxyURL)
			require.NoError(t, err)

			connected := make(chan struct{})
			dialer := tt.newDialer(proxyURL, DialerOptions{
				DialContext: func(context.Context, string, string) (net.Conn, error) {
					close(connected)
					return clientConn, nil
				},
				DialTimeout:         time.Second,
				TLSHandshakeTimeout: time.Second,
			})

			ctx, cancel := context.WithCancel(context.Background())
			result := make(chan error, 1)
			go func() {
				conn, err := dialer.DialTLSContext(ctx, "tcp", "example.com:443")
				if conn != nil {
					_ = conn.Close()
				}
				result <- err
			}()

			<-connected
			cancel()

			select {
			case err := <-result:
				require.Error(t, err)
				require.True(t, errors.Is(err, context.Canceled), "expected context cancellation, got %v", err)
			case <-time.After(time.Second):
				t.Fatal("proxy dial did not stop after context cancellation")
			}
		})
	}
}
