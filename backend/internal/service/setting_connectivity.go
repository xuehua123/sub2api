package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/maphash"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/pkg/connectivity"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	clientip "github.com/Wei-Shaw/sub2api/internal/pkg/ip"
)

const (
	defaultConnectivityProbeSamples        = 10
	defaultConnectivityProbeWarmup         = 1
	defaultConnectivityProbeMaxConcurrency = 3
	defaultConnectivityProbeTimeoutMS      = 10000
	defaultConnectivityProbeIPRPM          = 360
	defaultConnectivityProbeBurst          = 250
	maxConnectivityEndpoints               = 11
	connectivityProbeRefreshInterval       = 5 * time.Minute
	connectivityProbeRefreshTimeout        = 10 * time.Second
	defaultGeoIPFailureCacheTTL            = time.Minute
	defaultGeoIPFailureCacheCapacity       = 4096
)

type geoIPFailureCache struct {
	mu       sync.Mutex
	seed     maphash.Seed
	ttl      time.Duration
	capacity int
	entries  map[uint64]time.Time
	now      func() time.Time
}

func newGeoIPFailureCache(ttl time.Duration, capacity int) *geoIPFailureCache {
	return &geoIPFailureCache{
		seed:     maphash.MakeSeed(),
		ttl:      ttl,
		capacity: capacity,
		entries:  make(map[uint64]time.Time),
		now:      time.Now,
	}
}

func (c *geoIPFailureCache) key(addr netip.Addr) uint64 {
	var hash maphash.Hash
	hash.SetSeed(c.seed)
	_, _ = hash.Write(addr.AsSlice())
	return hash.Sum64()
}

func (c *geoIPFailureCache) contains(addr netip.Addr) bool {
	if c == nil || c.ttl <= 0 || c.capacity <= 0 {
		return false
	}
	key := c.key(addr)
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	expiresAt, ok := c.entries[key]
	if !ok {
		return false
	}
	if !now.Before(expiresAt) {
		delete(c.entries, key)
		return false
	}
	return true
}

func (c *geoIPFailureCache) record(addr netip.Addr) {
	if c == nil || c.ttl <= 0 || c.capacity <= 0 {
		return
	}
	key := c.key(addr)
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.capacity {
		for cachedKey, expiresAt := range c.entries {
			if !now.Before(expiresAt) {
				delete(c.entries, cachedKey)
			}
		}
	}
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.capacity {
		for cachedKey := range c.entries {
			delete(c.entries, cachedKey)
			break
		}
	}
	c.entries[key] = now.Add(c.ttl)
}

func (c *geoIPFailureCache) clear(addr netip.Addr) {
	if c == nil {
		return
	}
	key := c.key(addr)
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

type ConnectivityGradeThreshold struct {
	MinSuccessRate float64 `json:"min_success_rate"`
	MaxP95MS       float64 `json:"max_p95_ms"`
	MaxMADMS       float64 `json:"max_mad_ms"`
}

type ConnectivityGradeThresholds struct {
	GradingVersion         string                     `json:"grading_version"`
	MinimumSuccessRate     float64                    `json:"minimum_success_rate"`
	MaxConsecutiveTimeouts int                        `json:"max_consecutive_timeouts"`
	Excellent              ConnectivityGradeThreshold `json:"excellent"`
	Good                   ConnectivityGradeThreshold `json:"good"`
}

type ConnectivityTestEndpoint struct {
	Name      string `json:"name"`
	APIURL    string `json:"api_url"`
	ProbeURL  string `json:"probe_url"`
	IsDefault bool   `json:"is_default"`
}

type ConnectivityProbeSnapshot struct {
	Enabled         bool
	ClientIPEnabled bool
	Thresholds      ConnectivityGradeThresholds
	Samples         int
	Warmup          int
	MaxConcurrency  int
	TimeoutMS       int
	IPRPM           int
	Burst           int
	Endpoints       []ConnectivityTestEndpoint
}

type connectivityResolverFunc func(ctx context.Context, host string) ([]netip.Addr, error)

type connectivitySettings struct {
	Enabled         bool
	ClientIPEnabled bool
	Thresholds      ConnectivityGradeThresholds
	Samples         int
	Warmup          int
	MaxConcurrency  int
	TimeoutMS       int
	IPRPM           int
	Burst           int
	AllowedOrigins  []string
	APIBaseURL      string
	CustomEndpoints string
	Valid           bool
}

type connectivityCustomEndpoint struct {
	Name        string `json:"name"`
	Endpoint    string `json:"endpoint"`
	Description string `json:"description"`
}

func DefaultConnectivityGradeThresholds() ConnectivityGradeThresholds {
	return ConnectivityGradeThresholds{
		GradingVersion:         "1",
		MinimumSuccessRate:     0.8,
		MaxConsecutiveTimeouts: 2,
		Excellent: ConnectivityGradeThreshold{
			MinSuccessRate: 1,
			MaxP95MS:       250,
			MaxMADMS:       50,
		},
		Good: ConnectivityGradeThreshold{
			MinSuccessRate: 0.9,
			MaxP95MS:       500,
			MaxMADMS:       120,
		},
	}
}

func defaultConnectivityProbeSnapshot() *ConnectivityProbeSnapshot {
	return &ConnectivityProbeSnapshot{
		Thresholds:     DefaultConnectivityGradeThresholds(),
		Samples:        defaultConnectivityProbeSamples,
		Warmup:         defaultConnectivityProbeWarmup,
		MaxConcurrency: defaultConnectivityProbeMaxConcurrency,
		TimeoutMS:      defaultConnectivityProbeTimeoutMS,
		IPRPM:          defaultConnectivityProbeIPRPM,
		Burst:          defaultConnectivityProbeBurst,
		Endpoints:      []ConnectivityTestEndpoint{},
	}
}

func defaultConnectivityResolver(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

// ValidateConnectivitySettingsUpdate resolves only endpoint origins whose
// effective definition changed. Ordinary settings updates remain independent
// from transient DNS failures, while newly enabled or changed probe targets
// must pass the public-address boundary before they are persisted.
func (s *SettingService) ValidateConnectivitySettingsUpdate(ctx context.Context, previous, next *SystemSettings) error {
	if next == nil {
		return infraerrors.BadRequest("INVALID_CONNECTIVITY_SETTINGS", "connectivity settings are required")
	}
	if !connectivityEndpointDefinitionChanged(previous, next) || !next.ConnectivityTestEnabled {
		return nil
	}

	candidate := *next
	candidate.ConnectivityProbeAllowedOrigins = append([]string(nil), next.ConnectivityProbeAllowedOrigins...)
	if err := normalizeConnectivitySystemSettings(&candidate); err != nil {
		return infraerrors.BadRequest("INVALID_CONNECTIVITY_SETTINGS", err.Error())
	}
	if ctx == nil {
		ctx = context.Background()
	}
	validationCtx, cancel := context.WithTimeout(ctx, connectivityProbeRefreshTimeout)
	defer cancel()
	if err := s.validateConnectivityEndpointHosts(validationCtx, connectivitySettingsFromSystem(&candidate)); err != nil {
		return infraerrors.BadRequest("INVALID_CONNECTIVITY_SETTINGS", err.Error())
	}
	return nil
}

func connectivityEndpointDefinitionChanged(previous, next *SystemSettings) bool {
	if previous == nil || next == nil {
		return true
	}
	return previous.ConnectivityTestEnabled != next.ConnectivityTestEnabled ||
		previous.APIBaseURL != next.APIBaseURL ||
		previous.CustomEndpoints != next.CustomEndpoints ||
		!slices.Equal(previous.ConnectivityProbeAllowedOrigins, next.ConnectivityProbeAllowedOrigins)
}

func (s *SettingService) validateConnectivityEndpointHosts(ctx context.Context, settings connectivitySettings) error {
	allowed := make(map[string]struct{}, len(settings.AllowedOrigins))
	for _, raw := range settings.AllowedOrigins {
		origin, _, err := normalizeConnectivityOrigin(raw)
		if err != nil {
			return fmt.Errorf("invalid connectivity allowed origin %q: %w", raw, err)
		}
		allowed[origin] = struct{}{}
	}

	candidates := []string{}
	if strings.TrimSpace(settings.APIBaseURL) != "" {
		candidates = append(candidates, settings.APIBaseURL)
	}
	if raw := strings.TrimSpace(settings.CustomEndpoints); raw != "" {
		var custom []connectivityCustomEndpoint
		if err := json.Unmarshal([]byte(raw), &custom); err != nil {
			return fmt.Errorf("parse custom endpoints for connectivity DNS validation: %w", err)
		}
		for _, endpoint := range custom {
			candidates = append(candidates, endpoint.Endpoint)
		}
	}

	validated := make(map[string]struct{}, len(candidates))
	for _, raw := range candidates {
		_, origin, err := normalizeConnectivityAPIURL(raw)
		if err != nil {
			continue
		}
		if _, selected := allowed[origin]; !selected {
			continue
		}
		if _, done := validated[origin]; done {
			continue
		}
		_, host, err := normalizeConnectivityOrigin(origin)
		if err != nil {
			return fmt.Errorf("invalid connectivity origin %q: %w", origin, err)
		}
		if err := s.validateConnectivityHost(ctx, host); err != nil {
			return fmt.Errorf("connectivity origin %s failed DNS safety validation: %w", origin, err)
		}
		validated[origin] = struct{}{}
	}
	return nil
}

func (s *SettingService) LoadConnectivityProbeSettings(ctx context.Context) error {
	if s == nil || s.settingRepo == nil {
		return nil
	}
	s.connectivityMu.Lock()
	defer s.connectivityMu.Unlock()

	values, err := s.settingRepo.GetMultiple(ctx, connectivitySettingKeys())
	if err != nil {
		s.connectivitySnapshot.Store(defaultConnectivityProbeSnapshot())
		return fmt.Errorf("load connectivity probe settings: %w", err)
	}
	s.refreshConnectivityProbeSnapshot(ctx, connectivitySettingsFromMap(values))
	return nil
}

// StartConnectivityProbeRefresh periodically revalidates DNS and DB-backed
// settings outside the request path. A failed refresh keeps the fail-closed
// snapshot produced by LoadConnectivityProbeSettings.
func (s *SettingService) StartConnectivityProbeRefresh(ctx context.Context) {
	if s == nil || s.settingRepo == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.connectivityRefresh.Do(func() {
		go func() {
			ticker := time.NewTicker(connectivityProbeRefreshInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					refreshCtx, cancel := context.WithTimeout(ctx, connectivityProbeRefreshTimeout)
					err := s.LoadConnectivityProbeSettings(refreshCtx)
					cancel()
					if err != nil {
						slog.Warn("connectivity probe settings refresh failed", "error", err)
					}
				}
			}
		}()
	})
}

func (s *SettingService) connectivityProbeSnapshot() *ConnectivityProbeSnapshot {
	if s == nil {
		return defaultConnectivityProbeSnapshot()
	}
	snapshot, _ := s.connectivitySnapshot.Load().(*ConnectivityProbeSnapshot)
	if snapshot == nil {
		return defaultConnectivityProbeSnapshot()
	}
	clone := *snapshot
	clone.Endpoints = append([]ConnectivityTestEndpoint(nil), snapshot.Endpoints...)
	return &clone
}

// ConnectivityProbeSnapshot returns a detached copy of the in-memory probe
// configuration. It never reads the settings repository.
func (s *SettingService) ConnectivityProbeSnapshot() *ConnectivityProbeSnapshot {
	return s.connectivityProbeSnapshot()
}

// ConnectivityClientLocation is the coarse-grained geographic estimate exposed
// to the browser. It intentionally excludes coordinates, postal code, timezone,
// ASN, ISP, internal nodes, and proxy-chain information.
type ConnectivityClientLocation struct {
	CountryCode string `json:"country_code"`
	Country     string `json:"country"`
	Region      string `json:"region"`
	City        string `json:"city"`
}

// ConnectivityClientContext is the aggregated probe result for one request.
// Location is nil when region lookup is disabled, unconfigured, or failed; the
// IP is still exposed in that case.
type ConnectivityClientContext struct {
	IP       string
	Location *ConnectivityClientLocation
}

// ConnectivityProbeClientContext returns an aggregated IP/location context only
// when the DB-backed feature switch and the deployment-level trust boundary both
// permit exposure. It resolves the verified public IP via the trusted proxy
// chain and, when the GeoIP database is ready, attaches a coarse location. A
// nil context means no verified IP should be shown (fail closed).
func (s *SettingService) ConnectivityProbeClientContext(req *http.Request) *ConnectivityClientContext {
	if s == nil || s.connectivityClientIP == nil || !s.connectivityClientIP.CanExposeClientIP() {
		return nil
	}
	snapshot := s.connectivityProbeSnapshot()
	if !snapshot.Enabled || !snapshot.ClientIPEnabled {
		return nil
	}
	verified := s.connectivityClientIP.Resolve(req)
	if !verified.OK {
		return nil
	}
	ctx := &ConnectivityClientContext{IP: verified.IP.String()}
	if s.geoipFailureCache != nil && s.geoipFailureCache.contains(verified.IP) {
		return ctx
	}

	s.connectivityGeoIPMu.RLock()
	resolver := s.connectivityGeoIP
	if resolver != nil && resolver.Ready() {
		location, geoErr := resolver.Lookup(verified.IP)
		s.connectivityGeoIPMu.RUnlock()
		if geoErr != nil {
			s.geoipFailureCache.record(verified.IP)
			if s.geoipFailureLimiter != nil {
				s.geoipFailureLimiter.Warn("connectivity geoip lookup failed", "error", geoErr)
			}
		} else if location != nil {
			s.geoipFailureCache.clear(verified.IP)
			ctx.Location = &ConnectivityClientLocation{
				CountryCode: location.CountryCode,
				Country:     location.Country,
				Region:      location.Region,
				City:        location.City,
			}
		} else {
			s.geoipFailureCache.clear(verified.IP)
		}
	} else {
		s.connectivityGeoIPMu.RUnlock()
	}
	return ctx
}

// Close releases the local GeoIP database handle during shutdown. It is safe
// to call multiple times and is a no-op when no database is open.
func (s *SettingService) Close() {
	if s == nil {
		return
	}
	s.connectivityGeoIPMu.Lock()
	defer s.connectivityGeoIPMu.Unlock()
	if s.connectivityGeoIP == nil {
		return
	}
	_ = s.connectivityGeoIP.Close()
	s.connectivityGeoIP = nil
}

// ConnectivityGeoIPStatus reports the read-only GeoIP runtime status for the
// admin panel: "ready", "not_configured", or "unavailable". It never exposes
// the database file path.
func (s *SettingService) ConnectivityGeoIPStatus() string {
	if s == nil || !s.connectivityGeoIPConfigured {
		return "not_configured"
	}
	s.connectivityGeoIPMu.RLock()
	defer s.connectivityGeoIPMu.RUnlock()
	if s.connectivityGeoIP != nil && s.connectivityGeoIP.Ready() {
		return "ready"
	}
	return "unavailable"
}

// ConnectivityProbeClientIP returns an address only when both the DB-backed
// feature switch and the deployment-level trust boundary permit exposure. It is
// kept for compatibility; new code should use ConnectivityProbeClientContext.
func (s *SettingService) ConnectivityProbeClientIP(req *http.Request) (string, bool) {
	ctx := s.ConnectivityProbeClientContext(req)
	if ctx == nil {
		return "", false
	}
	return ctx.IP, true
}

// ConnectivityProbeRateLimitIP resolves the same verified address without
// consulting the browser-display switch. Callers must HMAC the result before
// retaining it in an in-memory limiter.
func (s *SettingService) ConnectivityProbeRateLimitIP(req *http.Request) (netip.Addr, bool) {
	if s == nil || s.connectivityClientIP == nil {
		return netip.Addr{}, false
	}
	verified := s.connectivityClientIP.Resolve(req)
	return verified.IP, verified.OK
}

func (s *SettingService) refreshConnectivityProbeSnapshot(ctx context.Context, settings connectivitySettings) {
	snapshot := defaultConnectivityProbeSnapshot()
	snapshot.Thresholds = settings.Thresholds
	snapshot.Samples = settings.Samples
	snapshot.Warmup = settings.Warmup
	snapshot.MaxConcurrency = settings.MaxConcurrency
	snapshot.TimeoutMS = settings.TimeoutMS
	snapshot.IPRPM = settings.IPRPM
	snapshot.Burst = settings.Burst
	if !settings.Valid || !settings.Enabled {
		s.connectivitySnapshot.Store(snapshot)
		return
	}

	endpoints, err := s.buildConnectivityEndpoints(ctx, settings)
	if err != nil {
		slog.Warn("connectivity testing disabled by invalid endpoint configuration", "error", err)
		s.connectivitySnapshot.Store(snapshot)
		return
	}
	snapshot.Enabled = len(endpoints) > 0
	clientIPSafetyReady := s.connectivityClientIP != nil && s.connectivityClientIP.CanExposeClientIP()
	snapshot.ClientIPEnabled = snapshot.Enabled && settings.ClientIPEnabled && clientIPSafetyReady
	snapshot.Endpoints = endpoints
	if snapshot.Enabled && settings.ClientIPEnabled && !clientIPSafetyReady {
		slog.Warn("connectivity client IP exposure disabled by incomplete trusted-proxy or denied-CIDR configuration")
	}
	if settings.Enabled && len(endpoints) == 0 {
		slog.Warn("connectivity testing has no eligible public endpoints")
	}
	s.connectivitySnapshot.Store(snapshot)
}

func connectivitySettingKeys() []string {
	return []string{
		SettingKeyConnectivityTestEnabled,
		SettingKeyConnectivityClientIPEnabled,
		SettingKeyConnectivityGradeThresholds,
		SettingKeyConnectivityProbeSamples,
		SettingKeyConnectivityProbeWarmup,
		SettingKeyConnectivityProbeMaxConcurrency,
		SettingKeyConnectivityProbeTimeoutMS,
		SettingKeyConnectivityProbeAllowedOrigins,
		SettingKeyConnectivityProbeIPRPM,
		SettingKeyConnectivityProbeBurst,
		SettingKeyAPIBaseURL,
		SettingKeyCustomEndpoints,
	}
}

func connectivitySettingsFromMap(values map[string]string) connectivitySettings {
	settings := connectivitySettings{
		Enabled:         values[SettingKeyConnectivityTestEnabled] == "true",
		ClientIPEnabled: values[SettingKeyConnectivityClientIPEnabled] == "true",
		Thresholds:      DefaultConnectivityGradeThresholds(),
		Samples:         defaultConnectivityProbeSamples,
		Warmup:          defaultConnectivityProbeWarmup,
		MaxConcurrency:  defaultConnectivityProbeMaxConcurrency,
		TimeoutMS:       defaultConnectivityProbeTimeoutMS,
		IPRPM:           defaultConnectivityProbeIPRPM,
		Burst:           defaultConnectivityProbeBurst,
		AllowedOrigins:  []string{},
		APIBaseURL:      values[SettingKeyAPIBaseURL],
		CustomEndpoints: values[SettingKeyCustomEndpoints],
		Valid:           true,
	}

	if raw := strings.TrimSpace(values[SettingKeyConnectivityGradeThresholds]); raw != "" {
		if err := json.Unmarshal([]byte(raw), &settings.Thresholds); err != nil || validateConnectivityThresholds(settings.Thresholds) != nil {
			settings.Thresholds = DefaultConnectivityGradeThresholds()
			settings.Valid = false
		}
	}
	settings.Samples, settings.Valid = parseConnectivityInt(values, SettingKeyConnectivityProbeSamples, defaultConnectivityProbeSamples, 5, 20, settings.Valid)
	settings.Warmup, settings.Valid = parseConnectivityInt(values, SettingKeyConnectivityProbeWarmup, defaultConnectivityProbeWarmup, 0, 2, settings.Valid)
	settings.MaxConcurrency, settings.Valid = parseConnectivityInt(values, SettingKeyConnectivityProbeMaxConcurrency, defaultConnectivityProbeMaxConcurrency, 1, 3, settings.Valid)
	settings.TimeoutMS, settings.Valid = parseConnectivityInt(values, SettingKeyConnectivityProbeTimeoutMS, defaultConnectivityProbeTimeoutMS, 2000, 15000, settings.Valid)
	settings.IPRPM, settings.Valid = parseConnectivityInt(values, SettingKeyConnectivityProbeIPRPM, defaultConnectivityProbeIPRPM, 1, 10000, settings.Valid)
	settings.Burst, settings.Valid = parseConnectivityInt(values, SettingKeyConnectivityProbeBurst, defaultConnectivityProbeBurst, 1, 1000, settings.Valid)

	if raw := strings.TrimSpace(values[SettingKeyConnectivityProbeAllowedOrigins]); raw != "" {
		if err := json.Unmarshal([]byte(raw), &settings.AllowedOrigins); err != nil || settings.AllowedOrigins == nil {
			settings.AllowedOrigins = []string{}
			settings.Valid = false
		}
	}
	return settings
}

func parseConnectivityInt(values map[string]string, key string, fallback, minValue, maxValue int, valid bool) (int, bool) {
	raw := strings.TrimSpace(values[key])
	if raw == "" {
		return fallback, valid
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minValue || value > maxValue {
		return fallback, false
	}
	return value, valid
}

func validateConnectivityThresholds(value ConnectivityGradeThresholds) error {
	for _, number := range []float64{
		value.MinimumSuccessRate,
		value.Excellent.MinSuccessRate,
		value.Excellent.MaxP95MS,
		value.Excellent.MaxMADMS,
		value.Good.MinSuccessRate,
		value.Good.MaxP95MS,
		value.Good.MaxMADMS,
	} {
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return errors.New("connectivity thresholds must contain finite numbers")
		}
	}
	if strings.TrimSpace(value.GradingVersion) == "" || len(value.GradingVersion) > 32 {
		return errors.New("connectivity grading_version is required and must not exceed 32 characters")
	}
	if value.MinimumSuccessRate < 0 || value.MinimumSuccessRate > 1 ||
		value.Good.MinSuccessRate <= value.MinimumSuccessRate || value.Good.MinSuccessRate > 1 ||
		value.Excellent.MinSuccessRate <= value.Good.MinSuccessRate || value.Excellent.MinSuccessRate > 1 {
		return errors.New("connectivity success-rate thresholds must be strictly increasing")
	}
	if value.Excellent.MaxP95MS <= 0 || value.Good.MaxP95MS <= value.Excellent.MaxP95MS {
		return errors.New("connectivity P95 thresholds must be positive and strictly increasing")
	}
	if value.Excellent.MaxMADMS < 0 || value.Good.MaxMADMS <= value.Excellent.MaxMADMS {
		return errors.New("connectivity MAD thresholds must be non-negative and strictly increasing")
	}
	if value.MaxConsecutiveTimeouts < 1 || value.MaxConsecutiveTimeouts > 20 {
		return errors.New("connectivity max_consecutive_timeouts must be between 1 and 20")
	}
	return nil
}

func normalizeConnectivitySystemSettings(settings *SystemSettings) error {
	if settings == nil {
		return errors.New("connectivity settings are required")
	}
	if settings.ConnectivityGradeThresholds == (ConnectivityGradeThresholds{}) {
		settings.ConnectivityGradeThresholds = DefaultConnectivityGradeThresholds()
	}
	if err := validateConnectivityThresholds(settings.ConnectivityGradeThresholds); err != nil {
		return err
	}

	var err error
	settings.ConnectivityProbeSamples, err = normalizeConnectivityInt(settings.ConnectivityProbeSamples, defaultConnectivityProbeSamples, 5, 20, "samples")
	if err != nil {
		return err
	}
	settings.ConnectivityProbeWarmup, err = normalizeConnectivityIntAllowZero(settings.ConnectivityProbeWarmup, 0, 2, "warmup")
	if err != nil {
		return err
	}
	settings.ConnectivityProbeMaxConcurrency, err = normalizeConnectivityInt(settings.ConnectivityProbeMaxConcurrency, defaultConnectivityProbeMaxConcurrency, 1, 3, "max_concurrency")
	if err != nil {
		return err
	}
	settings.ConnectivityProbeTimeoutMS, err = normalizeConnectivityInt(settings.ConnectivityProbeTimeoutMS, defaultConnectivityProbeTimeoutMS, 2000, 15000, "timeout_ms")
	if err != nil {
		return err
	}
	settings.ConnectivityProbeIPRPM, err = normalizeConnectivityInt(settings.ConnectivityProbeIPRPM, defaultConnectivityProbeIPRPM, 1, 10000, "ip_rpm")
	if err != nil {
		return err
	}
	settings.ConnectivityProbeBurst, err = normalizeConnectivityInt(settings.ConnectivityProbeBurst, defaultConnectivityProbeBurst, 1, 1000, "burst")
	if err != nil {
		return err
	}

	normalizedOrigins := make([]string, 0, len(settings.ConnectivityProbeAllowedOrigins))
	allowedSet := make(map[string]struct{}, len(settings.ConnectivityProbeAllowedOrigins))
	for _, raw := range settings.ConnectivityProbeAllowedOrigins {
		origin, _, normalizeErr := normalizeConnectivityOrigin(raw)
		if normalizeErr != nil {
			return fmt.Errorf("invalid connectivity allowed origin %q: %w", raw, normalizeErr)
		}
		if _, exists := allowedSet[origin]; exists {
			continue
		}
		allowedSet[origin] = struct{}{}
		normalizedOrigins = append(normalizedOrigins, origin)
	}
	settings.ConnectivityProbeAllowedOrigins = normalizedOrigins

	uniqueOrigins, eligibleURLs, err := connectivityEligibleCounts(settings.APIBaseURL, settings.CustomEndpoints, allowedSet)
	if err != nil {
		return err
	}
	if settings.ConnectivityTestEnabled && uniqueOrigins == 0 {
		return errors.New("connectivity testing requires at least one eligible API origin")
	}
	if eligibleURLs > maxConnectivityEndpoints {
		return fmt.Errorf("connectivity testing must not expose more than %d URLs", maxConnectivityEndpoints)
	}
	requestBudget := uniqueOrigins * (settings.ConnectivityProbeWarmup + settings.ConnectivityProbeSamples)
	if requestBudget > 250 {
		return errors.New("connectivity probe request budget exceeds 250")
	}
	if settings.ConnectivityTestEnabled && settings.ConnectivityProbeBurst < requestBudget {
		return fmt.Errorf("connectivity probe burst must cover the single-run request budget of %d", requestBudget)
	}
	return nil
}

func normalizeConnectivityInt(value, fallback, minValue, maxValue int, name string) (int, error) {
	if value == 0 {
		value = fallback
	}
	if value < minValue || value > maxValue {
		return 0, fmt.Errorf("connectivity probe %s must be between %d and %d", name, minValue, maxValue)
	}
	return value, nil
}

func normalizeConnectivityIntAllowZero(value, minValue, maxValue int, name string) (int, error) {
	if value < minValue || value > maxValue {
		return 0, fmt.Errorf("connectivity probe %s must be between %d and %d", name, minValue, maxValue)
	}
	return value, nil
}

func connectivityEligibleCounts(apiBaseURL, customEndpoints string, allowedSet map[string]struct{}) (int, int, error) {
	candidates := []string{}
	if strings.TrimSpace(apiBaseURL) != "" {
		candidates = append(candidates, apiBaseURL)
	}
	if raw := strings.TrimSpace(customEndpoints); raw != "" {
		var custom []connectivityCustomEndpoint
		if err := json.Unmarshal([]byte(raw), &custom); err != nil {
			return 0, 0, fmt.Errorf("parse custom endpoints for connectivity budget: %w", err)
		}
		for _, endpoint := range custom {
			candidates = append(candidates, endpoint.Endpoint)
		}
	}
	uniqueOrigins := make(map[string]struct{}, len(candidates))
	uniqueURLs := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		apiURL, origin, err := normalizeConnectivityAPIURL(candidate)
		if err != nil {
			continue
		}
		if _, allowed := allowedSet[origin]; allowed {
			uniqueOrigins[origin] = struct{}{}
			uniqueURLs[apiURL] = struct{}{}
		}
	}
	return len(uniqueOrigins), len(uniqueURLs), nil
}

func connectivitySettingsFromSystem(settings *SystemSettings) connectivitySettings {
	if settings == nil {
		return connectivitySettingsFromMap(nil)
	}
	return connectivitySettings{
		Enabled:         settings.ConnectivityTestEnabled,
		ClientIPEnabled: settings.ConnectivityClientIPEnabled,
		Thresholds:      settings.ConnectivityGradeThresholds,
		Samples:         settings.ConnectivityProbeSamples,
		Warmup:          settings.ConnectivityProbeWarmup,
		MaxConcurrency:  settings.ConnectivityProbeMaxConcurrency,
		TimeoutMS:       settings.ConnectivityProbeTimeoutMS,
		IPRPM:           settings.ConnectivityProbeIPRPM,
		Burst:           settings.ConnectivityProbeBurst,
		AllowedOrigins:  append([]string(nil), settings.ConnectivityProbeAllowedOrigins...),
		APIBaseURL:      settings.APIBaseURL,
		CustomEndpoints: settings.CustomEndpoints,
		Valid:           true,
	}
}

func (s *SettingService) buildConnectivityEndpoints(ctx context.Context, settings connectivitySettings) ([]ConnectivityTestEndpoint, error) {
	allowedOrigins := make(map[string]string, len(settings.AllowedOrigins))
	for _, raw := range settings.AllowedOrigins {
		origin, host, err := normalizeConnectivityOrigin(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid connectivity allowed origin %q: %w", raw, err)
		}
		allowedOrigins[origin] = host
	}

	candidates := make([]connectivityCustomEndpoint, 0, maxConnectivityEndpoints)
	if strings.TrimSpace(settings.APIBaseURL) != "" {
		candidates = append(candidates, connectivityCustomEndpoint{Name: "API 端点", Endpoint: settings.APIBaseURL})
	}
	if raw := strings.TrimSpace(settings.CustomEndpoints); raw != "" {
		var custom []connectivityCustomEndpoint
		if err := json.Unmarshal([]byte(raw), &custom); err != nil {
			return nil, fmt.Errorf("parse custom endpoints: %w", err)
		}
		candidates = append(candidates, custom...)
	}

	result := make([]ConnectivityTestEndpoint, 0, len(candidates))
	seenURLs := make(map[string]struct{}, len(candidates))
	validatedOrigins := make(map[string]bool, len(allowedOrigins))
	for index, candidate := range candidates {
		apiURL, origin, err := normalizeConnectivityAPIURL(candidate.Endpoint)
		if err != nil {
			continue
		}
		host, allowed := allowedOrigins[origin]
		if !allowed {
			continue
		}
		eligible, checked := validatedOrigins[origin]
		if !checked {
			if err := s.validateConnectivityHost(ctx, host); err != nil {
				slog.Warn("connectivity allowed origin excluded", "origin", origin, "error", err)
				validatedOrigins[origin] = false
				continue
			}
			validatedOrigins[origin] = true
			eligible = true
		}
		if !eligible {
			continue
		}
		if _, exists := seenURLs[apiURL]; exists {
			continue
		}
		if len(result) >= maxConnectivityEndpoints {
			return nil, fmt.Errorf("too many eligible connectivity endpoints")
		}
		seenURLs[apiURL] = struct{}{}
		name := strings.TrimSpace(candidate.Name)
		if name == "" {
			name = "API 端点"
		}
		result = append(result, ConnectivityTestEndpoint{
			Name:      name,
			APIURL:    apiURL,
			ProbeURL:  origin + connectivity.ProbePath,
			IsDefault: index == 0 && strings.TrimSpace(settings.APIBaseURL) != "",
		})
	}

	uniqueOrigins := make(map[string]struct{}, len(result))
	for _, endpoint := range result {
		probeURL, err := url.Parse(endpoint.ProbeURL)
		if err == nil {
			uniqueOrigins[probeURL.Scheme+"://"+probeURL.Host] = struct{}{}
		}
	}
	requestBudget := len(uniqueOrigins) * (settings.Warmup + settings.Samples)
	if requestBudget > 250 {
		return nil, errors.New("connectivity probe request budget exceeds 250")
	}
	if settings.Burst < requestBudget {
		return nil, fmt.Errorf("connectivity probe burst must cover the single-run request budget of %d", requestBudget)
	}
	return result, nil
}

func (s *SettingService) validateConnectivityHost(ctx context.Context, host string) error {
	if addr, err := netip.ParseAddr(host); err == nil {
		if !isPublicConnectivityAddr(addr.Unmap()) {
			return errors.New("IP literal is not globally routable")
		}
		return nil
	}
	resolver := s.connectivityResolver
	if resolver == nil {
		resolver = defaultConnectivityResolver
	}
	addrs, err := resolver(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve host: %w", err)
	}
	if len(addrs) == 0 {
		return errors.New("host resolved to no addresses")
	}
	for _, addr := range addrs {
		if !isPublicConnectivityAddr(addr.Unmap()) {
			return fmt.Errorf("host resolved to non-public address %s", addr)
		}
	}
	return nil
}

func normalizeConnectivityOrigin(raw string) (origin string, host string, err error) {
	apiURL, origin, err := normalizeConnectivityAPIURL(raw)
	if err != nil {
		return "", "", err
	}
	parsed, err := url.Parse(apiURL)
	if err != nil {
		return "", "", err
	}
	if parsed.EscapedPath() != "" && parsed.EscapedPath() != "/" {
		return "", "", errors.New("origin must not contain a path")
	}
	return origin, parsed.Hostname(), nil
}

func normalizeConnectivityAPIURL(raw string) (apiURL string, origin string, err error) {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, `\`) {
		return "", "", errors.New("URL must not contain backslashes")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || !parsed.IsAbs() || parsed.Opaque != "" {
		return "", "", errors.New("URL must be absolute")
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", "", errors.New("URL must use HTTPS")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", errors.New("URL must not contain userinfo, query, or fragment")
	}
	if parsed.RawPath != "" || strings.Contains(parsed.EscapedPath(), "%") {
		return "", "", errors.New("URL path must not contain percent-encoded bytes")
	}
	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment == "." || segment == ".." {
			return "", "", errors.New("URL path must not contain dot segments")
		}
	}
	hostname := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if err := validateConnectivityHostname(hostname); err != nil {
		return "", "", err
	}
	port := parsed.Port()
	if port != "" {
		portValue, parseErr := strconv.Atoi(port)
		if parseErr != nil || portValue < 1 || portValue > 65535 {
			return "", "", errors.New("URL port is invalid")
		}
		if portValue == 443 {
			port = ""
		} else {
			port = strconv.Itoa(portValue)
		}
	}
	host := hostname
	if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	}
	origin = "https://" + host
	pathValue := strings.TrimRight(parsed.Path, "/")
	if pathValue == "." {
		pathValue = ""
	}
	normalized := &url.URL{Scheme: "https", Host: host, Path: pathValue}
	return normalized.String(), origin, nil
}

func validateConnectivityHostname(host string) error {
	if host == "" || strings.HasSuffix(host, ".") {
		return errors.New("hostname is empty or non-canonical")
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		if !isPublicConnectivityAddr(addr.Unmap()) {
			return errors.New("IP literal is not globally routable")
		}
		return nil
	}
	if host == "localhost" || !strings.Contains(host, ".") ||
		strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".home.arpa") {
		return errors.New("local or single-label hostnames are not allowed")
	}
	if len(host) > 253 {
		return errors.New("hostname is too long")
	}
	hasLetter := false
	for _, r := range host {
		if r > unicode.MaxASCII {
			return errors.New("hostname must use ASCII")
		}
		if r >= 'a' && r <= 'z' {
			hasLetter = true
		}
	}
	if !hasLetter {
		return errors.New("ambiguous numeric hostnames are not allowed")
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return errors.New("hostname label is invalid")
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return errors.New("hostname contains invalid characters")
			}
		}
	}
	return nil
}

func isPublicConnectivityAddr(addr netip.Addr) bool {
	return clientip.IsPublicInternetAddr(addr)
}
