//go:build unit

package connectivity

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProbeRateLimiterEnforcesBurstAndRefillsFromRPM(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter, err := NewProbeRateLimiter(ProbeRateLimiterOptions{
		Secret:          []byte("test-only-secret"),
		Capacity:        16,
		IdleTTL:         10 * time.Minute,
		CleanupInterval: time.Minute,
		Now:             func() time.Time { return now },
	})
	require.NoError(t, err)

	for range 3 {
		require.True(t, limiter.Allow("8.8.8.8", 3, 3))
	}
	require.False(t, limiter.Allow("8.8.8.8", 3, 3))

	now = now.Add(20 * time.Second)
	require.True(t, limiter.Allow("8.8.8.8", 3, 3))
	require.False(t, limiter.Allow("8.8.8.8", 3, 3))
}

func TestProbeRateLimiterStoresOnlyHMACKeys(t *testing.T) {
	limiter, err := NewProbeRateLimiter(ProbeRateLimiterOptions{
		Secret:   []byte("test-only-secret"),
		Capacity: 4,
	})
	require.NoError(t, err)
	require.True(t, limiter.Allow("203.0.113.42", 60, 2))
	require.Len(t, limiter.buckets, 1)

	for key := range limiter.buckets {
		require.NotContains(t, string(key[:]), "203.0.113.42")
	}
}

func TestProbeRateLimiterBoundsCapacityAndExpiresIdleBuckets(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter, err := NewProbeRateLimiter(ProbeRateLimiterOptions{
		Secret:          []byte("test-only-secret"),
		Capacity:        2,
		IdleTTL:         time.Minute,
		CleanupInterval: 30 * time.Second,
		Now:             func() time.Time { return now },
	})
	require.NoError(t, err)

	require.True(t, limiter.Allow("8.8.8.8", 60, 1))
	require.True(t, limiter.Allow("1.1.1.1", 60, 1))
	require.False(t, limiter.Allow("9.9.9.9", 60, 1))
	require.Len(t, limiter.buckets, 2)

	now = now.Add(2 * time.Minute)
	require.True(t, limiter.Allow("9.9.9.9", 60, 1))
	require.Len(t, limiter.buckets, 1)
}

func TestProbeRateLimiterRejectsInvalidRequests(t *testing.T) {
	_, err := NewProbeRateLimiter(ProbeRateLimiterOptions{Secret: []byte("secret"), Capacity: 0})
	require.Error(t, err)

	limiter, err := NewProbeRateLimiter(ProbeRateLimiterOptions{Secret: []byte("secret"), Capacity: 1})
	require.NoError(t, err)
	require.False(t, limiter.Allow("", 60, 1))
	require.False(t, limiter.Allow("8.8.8.8", 0, 1))
	require.False(t, limiter.Allow("8.8.8.8", 60, 0))
}
