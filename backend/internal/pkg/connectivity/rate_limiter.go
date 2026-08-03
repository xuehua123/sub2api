package connectivity

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"math"
	"sync"
	"time"
)

const (
	defaultProbeRateLimiterIdleTTL         = 10 * time.Minute
	defaultProbeRateLimiterCleanupInterval = time.Minute
	maxProbeRateLimiterCapacity            = 1_000_000
)

type ProbeRateLimiterOptions struct {
	Secret          []byte
	Capacity        int
	IdleTTL         time.Duration
	CleanupInterval time.Duration
	Now             func() time.Time
}

type probeRateLimitBucket struct {
	tokens     float64
	lastRefill time.Time
	lastSeen   time.Time
}

// ProbeRateLimiter is a process-local bounded token bucket. Identities are
// HMACed before they become map keys, so raw client or peer IPs are not kept.
type ProbeRateLimiter struct {
	mu              sync.Mutex
	secret          []byte
	capacity        int
	idleTTL         time.Duration
	cleanupInterval time.Duration
	now             func() time.Time
	lastCleanup     time.Time
	buckets         map[[sha256.Size]byte]*probeRateLimitBucket
}

func NewProbeRateLimiter(options ProbeRateLimiterOptions) (*ProbeRateLimiter, error) {
	if len(options.Secret) == 0 {
		return nil, errors.New("probe rate limiter secret is required")
	}
	if options.Capacity < 1 || options.Capacity > maxProbeRateLimiterCapacity {
		return nil, errors.New("probe rate limiter capacity must be between 1 and 1000000")
	}
	if options.IdleTTL == 0 {
		options.IdleTTL = defaultProbeRateLimiterIdleTTL
	}
	if options.CleanupInterval == 0 {
		options.CleanupInterval = defaultProbeRateLimiterCleanupInterval
	}
	if options.IdleTTL <= 0 || options.CleanupInterval <= 0 {
		return nil, errors.New("probe rate limiter durations must be positive")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	now := options.Now()
	return &ProbeRateLimiter{
		secret:          append([]byte(nil), options.Secret...),
		capacity:        options.Capacity,
		idleTTL:         options.IdleTTL,
		cleanupInterval: options.CleanupInterval,
		now:             options.Now,
		lastCleanup:     now,
		buckets:         make(map[[sha256.Size]byte]*probeRateLimitBucket),
	}, nil
}

func (l *ProbeRateLimiter) Allow(identity string, requestsPerMinute, burst int) bool {
	if l == nil || identity == "" || requestsPerMinute <= 0 || burst <= 0 {
		return false
	}
	key := l.key(identity)
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanup(now)

	bucket := l.buckets[key]
	if bucket == nil {
		if len(l.buckets) >= l.capacity {
			return false
		}
		bucket = &probeRateLimitBucket{
			tokens:     float64(burst),
			lastRefill: now,
			lastSeen:   now,
		}
		l.buckets[key] = bucket
	}

	elapsed := now.Sub(bucket.lastRefill)
	if elapsed > 0 {
		refillPerSecond := float64(requestsPerMinute) / 60
		bucket.tokens = math.Min(float64(burst), bucket.tokens+elapsed.Seconds()*refillPerSecond)
		bucket.lastRefill = now
	} else if bucket.tokens > float64(burst) {
		bucket.tokens = float64(burst)
	}
	bucket.lastSeen = now
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

func (l *ProbeRateLimiter) key(identity string) [sha256.Size]byte {
	mac := hmac.New(sha256.New, l.secret)
	_, _ = mac.Write([]byte("sub2api-connectivity-probe-v1\x00"))
	_, _ = mac.Write([]byte(identity))
	var key [sha256.Size]byte
	copy(key[:], mac.Sum(nil))
	return key
}

func (l *ProbeRateLimiter) cleanup(now time.Time) {
	if now.Sub(l.lastCleanup) < l.cleanupInterval {
		return
	}
	for key, bucket := range l.buckets {
		if now.Sub(bucket.lastSeen) >= l.idleTTL {
			delete(l.buckets, key)
		}
	}
	l.lastCleanup = now
}
