package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// ChannelMonitorProbeHeaderName marks synthetic channel monitor probes.
	// It is advisory only: authentication, entitlement, billing, and routing still
	// follow the normal API-key gateway path.
	ChannelMonitorProbeHeaderName    = "X-Sub2API-Channel-Monitor-Probe"
	ChannelMonitorProbeTSHeaderName  = "X-Sub2API-Channel-Monitor-Probe-Ts"
	ChannelMonitorProbeSigHeaderName = "X-Sub2API-Channel-Monitor-Probe-Sig"
	// ChannelMonitorProbeExcludedAccountsHeaderName carries account IDs that
	// returned an unusable 2xx response in an earlier monitor attempt.
	ChannelMonitorProbeExcludedAccountsHeaderName = "X-Sub2API-Channel-Monitor-Excluded-Accounts"
	// ChannelMonitorProbeSelectedAccountHeaderName is returned only on a valid
	// monitor probe so the checker can exclude an unusable account on retry.
	ChannelMonitorProbeSelectedAccountHeaderName = "X-Sub2API-Channel-Monitor-Selected-Account"
	channelMonitorProbeHeaderValue               = "1"
	channelMonitorProbeSecretBytes               = 32
	channelMonitorProbeSignatureTTL              = 5 * time.Minute

	// ChannelMonitorProbeMaxAccountSwitches lets monitor probes try all accounts
	// in normal-sized groups before reporting a channel failure.
	ChannelMonitorProbeMaxAccountSwitches = 100
)

type channelMonitorProbeContextKey struct{}
type channelMonitorProbeExcludedAccountsContextKey struct{}

//nolint:gochecknoglobals // Signing key is process-wide; configured from server secret for multi-instance validation.
var (
	channelMonitorProbeSigningKeyMu sync.RWMutex
	channelMonitorProbeSigningKey   []byte
)

// ChannelMonitorProbeHeaderValue returns the marker value sent by the monitor checker.
func ChannelMonitorProbeHeaderValue() string {
	return channelMonitorProbeHeaderValue
}

// IsChannelMonitorProbeHeader reports whether a request carries the monitor probe marker.
func IsChannelMonitorProbeHeader(value string) bool {
	return value == channelMonitorProbeHeaderValue
}

// ConfigureChannelMonitorProbeSecret sets the stable server-side secret used to
// sign monitor probe markers. All instances with the same secret can validate
// each other's monitor probes.
func ConfigureChannelMonitorProbeSecret(secret string) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return
	}
	key := sha256.Sum256([]byte("sub2api:channel-monitor-probe:" + secret))
	channelMonitorProbeSigningKeyMu.Lock()
	defer channelMonitorProbeSigningKeyMu.Unlock()
	channelMonitorProbeSigningKey = key[:]
}

// AddChannelMonitorProbeHeaders signs a synthetic monitor probe request.
func AddChannelMonitorProbeHeaders(headers map[string]string, method, path string, now time.Time) {
	if headers == nil {
		return
	}
	ts := strconv.FormatInt(now.Unix(), 10)
	sig, ok := signChannelMonitorProbe(method, path, ts)
	if !ok {
		return
	}
	headers[ChannelMonitorProbeHeaderName] = ChannelMonitorProbeHeaderValue()
	headers[ChannelMonitorProbeTSHeaderName] = ts
	headers[ChannelMonitorProbeSigHeaderName] = hex.EncodeToString(sig)
}

// AddChannelMonitorProbeExcludedAccounts adds the prior monitor-attempt account
// exclusions. The gateway accepts this header only after validating the signed
// monitor marker, so regular clients cannot influence account selection.
func AddChannelMonitorProbeExcludedAccounts(headers map[string]string, accountIDs map[int64]struct{}) {
	if headers == nil || len(accountIDs) == 0 {
		return
	}
	ids := make([]int64, 0, min(len(accountIDs), ChannelMonitorProbeMaxAccountSwitches))
	for id := range accountIDs {
		if id > 0 {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) > ChannelMonitorProbeMaxAccountSwitches {
		ids = ids[:ChannelMonitorProbeMaxAccountSwitches]
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	if len(parts) > 0 {
		headers[ChannelMonitorProbeExcludedAccountsHeaderName] = strings.Join(parts, ",")
	}
}

// ParseChannelMonitorProbeExcludedAccounts parses the bounded account list
// supplied by a previously validated monitor probe.
func ParseChannelMonitorProbeExcludedAccounts(raw string) map[int64]struct{} {
	accountIDs := make(map[int64]struct{})
	for _, part := range strings.Split(raw, ",") {
		if len(accountIDs) >= ChannelMonitorProbeMaxAccountSwitches {
			break
		}
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil && id > 0 {
			accountIDs[id] = struct{}{}
		}
	}
	return accountIDs
}

// IsValidChannelMonitorProbe verifies that a monitor probe marker was produced
// by this process recently. It prevents normal API clients from opting into the
// channel-monitor failover policy with a forged marker header.
func IsValidChannelMonitorProbe(method, path, value, ts, sig string, now time.Time) bool {
	if !IsChannelMonitorProbeHeader(value) {
		return false
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	signedAt := time.Unix(unix, 0)
	if now.Sub(signedAt) > channelMonitorProbeSignatureTTL || signedAt.Sub(now) > channelMonitorProbeSignatureTTL {
		return false
	}
	expected, ok := signChannelMonitorProbe(method, path, ts)
	if !ok {
		return false
	}
	got, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, expected) == 1
}

func signChannelMonitorProbe(method, path, ts string) ([]byte, bool) {
	secret, ok := getChannelMonitorProbeSecret()
	if !ok {
		return nil, false
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(strings.ToUpper(method)))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(path))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(ts))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(channelMonitorProbeHeaderValue))
	return mac.Sum(nil), true
}

func getChannelMonitorProbeSecret() ([]byte, bool) {
	channelMonitorProbeSigningKeyMu.RLock()
	if len(channelMonitorProbeSigningKey) > 0 {
		key := append([]byte(nil), channelMonitorProbeSigningKey...)
		channelMonitorProbeSigningKeyMu.RUnlock()
		return key, true
	}
	channelMonitorProbeSigningKeyMu.RUnlock()

	channelMonitorProbeSigningKeyMu.Lock()
	defer channelMonitorProbeSigningKeyMu.Unlock()
	if len(channelMonitorProbeSigningKey) > 0 {
		return append([]byte(nil), channelMonitorProbeSigningKey...), true
	}
	secret := make([]byte, channelMonitorProbeSecretBytes)
	if _, err := rand.Read(secret); err != nil {
		return nil, false
	}
	channelMonitorProbeSigningKey = secret
	return append([]byte(nil), channelMonitorProbeSigningKey...), true
}

// WithChannelMonitorProbe marks a request context as a synthetic channel monitor probe.
func WithChannelMonitorProbe(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, channelMonitorProbeContextKey{}, true)
}

// WithChannelMonitorProbeExcludedAccounts carries request-local account
// exclusions between monitor retries. Copy the set so callers cannot mutate the
// context after it is attached.
func WithChannelMonitorProbeExcludedAccounts(ctx context.Context, accountIDs map[int64]struct{}) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	copyIDs := make(map[int64]struct{}, len(accountIDs))
	for id := range accountIDs {
		if id > 0 && len(copyIDs) < ChannelMonitorProbeMaxAccountSwitches {
			copyIDs[id] = struct{}{}
		}
	}
	return context.WithValue(ctx, channelMonitorProbeExcludedAccountsContextKey{}, copyIDs)
}

// ChannelMonitorProbeExcludedAccounts returns a fresh exclusion set for the
// current signed monitor request.
func ChannelMonitorProbeExcludedAccounts(ctx context.Context) map[int64]struct{} {
	if ctx == nil {
		return make(map[int64]struct{})
	}
	stored, _ := ctx.Value(channelMonitorProbeExcludedAccountsContextKey{}).(map[int64]struct{})
	accountIDs := make(map[int64]struct{}, len(stored))
	for id := range stored {
		accountIDs[id] = struct{}{}
	}
	return accountIDs
}

// IsChannelMonitorProbe reports whether the current request is a synthetic monitor probe.
func IsChannelMonitorProbe(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(channelMonitorProbeContextKey{}).(bool)
	return v
}

// AccountSwitchLimitForContext returns the failover switch cap for the request.
func AccountSwitchLimitForContext(ctx context.Context, configured int) int {
	if IsChannelMonitorProbe(ctx) {
		return ChannelMonitorProbeMaxAccountSwitches
	}
	return configured
}
