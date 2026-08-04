//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geoip"
	"github.com/stretchr/testify/require"
)

type blockingConnectivitySettingRepo struct {
	mu          sync.Mutex
	values      map[string]string
	readStarted chan struct{}
	releaseRead chan struct{}
	writeDone   chan struct{}
}

type reorderingConnectivitySettingRepo struct {
	mu           sync.Mutex
	values       map[string]string
	firstWritten chan struct{}
	releaseFirst chan struct{}
}

func (r *reorderingConnectivitySettingRepo) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
}

func (r *reorderingConnectivitySettingRepo) GetValue(context.Context, string) (string, error) {
	panic("unexpected GetValue call")
}

func (r *reorderingConnectivitySettingRepo) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (r *reorderingConnectivitySettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
}

func (r *reorderingConnectivitySettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	r.mu.Lock()
	for key, value := range settings {
		r.values[key] = value
	}
	r.mu.Unlock()
	if settings[SettingKeyAPIBaseURL] == "https://first.example.com/v1" {
		close(r.firstWritten)
		<-r.releaseFirst
	}
	return nil
}

func (r *reorderingConnectivitySettingRepo) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (r *reorderingConnectivitySettingRepo) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func (r *blockingConnectivitySettingRepo) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
}

func (r *blockingConnectivitySettingRepo) GetValue(context.Context, string) (string, error) {
	panic("unexpected GetValue call")
}

func (r *blockingConnectivitySettingRepo) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (r *blockingConnectivitySettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			result[key] = value
		}
	}
	r.mu.Unlock()

	close(r.readStarted)
	<-r.releaseRead
	return result, nil
}

func (r *blockingConnectivitySettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	r.mu.Lock()
	for key, value := range settings {
		r.values[key] = value
	}
	r.mu.Unlock()
	close(r.writeDone)
	return nil
}

func (r *blockingConnectivitySettingRepo) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (r *blockingConnectivitySettingRepo) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func TestConnectivityPublicSettingsBuildsOnlyAllowedPublicEndpoints(t *testing.T) {
	repo := &settingPublicRepoStub{values: map[string]string{
		SettingKeyConnectivityTestEnabled:         "true",
		SettingKeyConnectivityClientIPEnabled:     "false",
		SettingKeyConnectivityProbeSamples:        "10",
		SettingKeyConnectivityProbeWarmup:         "1",
		SettingKeyConnectivityProbeMaxConcurrency: "3",
		SettingKeyConnectivityProbeTimeoutMS:      "10000",
		SettingKeyConnectivityProbeAllowedOrigins: `[
			"https://api.example.com",
			"https://private.example.com"
		]`,
		SettingKeyAPIBaseURL: "https://API.Example.com/v1/",
		SettingKeyCustomEndpoints: `[
			{"name":"同源备用","endpoint":"https://api.example.com/compatible/v1","description":""},
			{"name":"私网","endpoint":"https://private.example.com/v1","description":""},
			{"name":"未授权","endpoint":"https://other.example.com/v1","description":""}
		]`,
	}}
	svc := NewSettingService(repo, &config.Config{})
	svc.connectivityResolver = func(_ context.Context, host string) ([]netip.Addr, error) {
		switch host {
		case "api.example.com":
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		case "private.example.com":
			return []netip.Addr{netip.MustParseAddr("10.0.0.8")}, nil
		default:
			return nil, nil
		}
	}

	require.NoError(t, svc.LoadConnectivityProbeSettings(context.Background()))
	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)

	require.True(t, settings.ConnectivityTestEnabled)
	require.False(t, settings.ConnectivityClientIPEnabled)
	require.Equal(t, 10, settings.ConnectivityProbeSamples)
	require.Equal(t, 1, settings.ConnectivityProbeWarmup)
	require.Equal(t, 3, settings.ConnectivityProbeMaxConcurrency)
	require.Equal(t, 10000, settings.ConnectivityProbeTimeoutMS)
	require.Equal(t, DefaultConnectivityGradeThresholds(), settings.ConnectivityGradeThresholds)
	require.Equal(t, []ConnectivityTestEndpoint{
		{
			Name:      "API 端点",
			APIURL:    "https://api.example.com/v1",
			ProbeURL:  "https://api.example.com/.well-known/sub2api/edge-probe",
			IsDefault: true,
		},
		{
			Name:      "同源备用",
			APIURL:    "https://api.example.com/compatible/v1",
			ProbeURL:  "https://api.example.com/.well-known/sub2api/edge-probe",
			IsDefault: false,
		},
	}, settings.ConnectivityTestEndpoints)
}

func TestConnectivityPublicSettingsDoesNotResolveUnusedAllowedOrigin(t *testing.T) {
	repo := &settingPublicRepoStub{values: map[string]string{
		SettingKeyConnectivityTestEnabled:         "true",
		SettingKeyConnectivityProbeSamples:        "10",
		SettingKeyConnectivityProbeWarmup:         "1",
		SettingKeyConnectivityProbeMaxConcurrency: "3",
		SettingKeyConnectivityProbeTimeoutMS:      "10000",
		SettingKeyConnectivityProbeAllowedOrigins: `["https://stale.example.com","https://api.example.com"]`,
		SettingKeyAPIBaseURL:                      "https://api.example.com/v1",
		SettingKeyCustomEndpoints:                 `[]`,
	}}
	svc := NewSettingService(repo, &config.Config{})
	resolvedHosts := []string{}
	svc.connectivityResolver = func(_ context.Context, host string) ([]netip.Addr, error) {
		resolvedHosts = append(resolvedHosts, host)
		if host == "stale.example.com" {
			return nil, errors.New("unused origin must not be resolved")
		}
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}

	require.NoError(t, svc.LoadConnectivityProbeSettings(context.Background()))
	snapshot := svc.ConnectivityProbeSnapshot()
	require.True(t, snapshot.Enabled)
	require.Len(t, snapshot.Endpoints, 1)
	require.Equal(t, []string{"api.example.com"}, resolvedHosts)
}

func TestConnectivityPublicSettingsDefaultsClosed(t *testing.T) {
	svc := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)

	require.False(t, settings.ConnectivityTestEnabled)
	require.False(t, settings.ConnectivityClientIPEnabled)
	require.Empty(t, settings.ConnectivityTestEndpoints)
	require.Equal(t, 10, settings.ConnectivityProbeSamples)
	require.Equal(t, 1, settings.ConnectivityProbeWarmup)
	require.Equal(t, 3, settings.ConnectivityProbeMaxConcurrency)
	require.Equal(t, 10000, settings.ConnectivityProbeTimeoutMS)
}

func TestConnectivitySettingsWriteCannotBeOverwrittenByStaleRefresh(t *testing.T) {
	repo := &blockingConnectivitySettingRepo{
		values: map[string]string{
			SettingKeyConnectivityTestEnabled:         "true",
			SettingKeyConnectivityProbeAllowedOrigins: `["https://old.example.com"]`,
			SettingKeyAPIBaseURL:                      "https://old.example.com/v1",
		},
		readStarted: make(chan struct{}),
		releaseRead: make(chan struct{}),
		writeDone:   make(chan struct{}),
	}
	svc := NewSettingService(repo, &config.Config{})
	svc.connectivityResolver = func(_ context.Context, _ string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}

	loadDone := make(chan error, 1)
	go func() {
		loadDone <- svc.LoadConnectivityProbeSettings(context.Background())
	}()
	<-repo.readStarted

	updateDone := make(chan error, 1)
	go func() {
		updateDone <- svc.UpdateSettings(context.Background(), &SystemSettings{
			ConnectivityTestEnabled:         true,
			ConnectivityGradeThresholds:     DefaultConnectivityGradeThresholds(),
			ConnectivityProbeAllowedOrigins: []string{"https://new.example.com"},
			APIBaseURL:                      "https://new.example.com/v1",
			ReferralCreditConversionRate:    1,
		})
	}()
	<-repo.writeDone

	var updateErr error
	updateReturnedBeforeRefresh := false
	select {
	case updateErr = <-updateDone:
		updateReturnedBeforeRefresh = true
	case <-time.After(100 * time.Millisecond):
	}
	close(repo.releaseRead)
	require.NoError(t, <-loadDone)
	if !updateReturnedBeforeRefresh {
		updateErr = <-updateDone
	}
	require.NoError(t, updateErr)

	require.False(t, updateReturnedBeforeRefresh, "settings write returned while an older refresh could still overwrite its snapshot")
	require.Equal(t, []ConnectivityTestEndpoint{
		{
			Name:      "API 端点",
			APIURL:    "https://new.example.com/v1",
			ProbeURL:  "https://new.example.com/.well-known/sub2api/edge-probe",
			IsDefault: true,
		},
	}, svc.ConnectivityProbeSnapshot().Endpoints)
}

func TestConcurrentConnectivitySettingsWritesKeepSnapshotAtLastDatabaseWrite(t *testing.T) {
	repo := &reorderingConnectivitySettingRepo{
		values:       map[string]string{},
		firstWritten: make(chan struct{}),
		releaseFirst: make(chan struct{}),
	}
	svc := NewSettingService(repo, &config.Config{})
	svc.connectivityResolver = func(_ context.Context, _ string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	first := connectivityTestSystemSettings(
		"https://first.example.com/v1",
		[]string{"https://first.example.com"},
	)
	first.ReferralCreditConversionRate = 1
	second := connectivityTestSystemSettings(
		"https://second.example.com/v1",
		[]string{"https://second.example.com"},
	)
	second.ReferralCreditConversionRate = 1

	firstDone := make(chan error, 1)
	go func() { firstDone <- svc.UpdateSettings(context.Background(), first) }()
	<-repo.firstWritten
	secondDone := make(chan error, 1)
	go func() { secondDone <- svc.UpdateSettings(context.Background(), second) }()

	var secondErr error
	secondFinishedBeforeRelease := false
	select {
	case secondErr = <-secondDone:
		secondFinishedBeforeRelease = true
	case <-time.After(100 * time.Millisecond):
	}
	close(repo.releaseFirst)
	require.NoError(t, <-firstDone)
	if !secondFinishedBeforeRelease {
		secondErr = <-secondDone
	}
	require.NoError(t, secondErr)

	repo.mu.Lock()
	storedAPIBaseURL := repo.values[SettingKeyAPIBaseURL]
	repo.mu.Unlock()
	require.Equal(t, "https://second.example.com/v1", storedAPIBaseURL)
	require.Equal(t, "https://second.example.com/v1", svc.ConnectivityProbeSnapshot().Endpoints[0].APIURL)
}

func TestBuildConnectivityEndpointsAllowsElevenCustomURLsWithoutDefault(t *testing.T) {
	custom := make([]connectivityCustomEndpoint, 0, maxConnectivityEndpoints)
	for i := range maxConnectivityEndpoints {
		custom = append(custom, connectivityCustomEndpoint{
			Name:     fmt.Sprintf("端点 %d", i+1),
			Endpoint: fmt.Sprintf("https://api.example.com/v%d", i+1),
		})
	}
	payload, err := json.Marshal(custom)
	require.NoError(t, err)

	svc := NewSettingService(&settingPublicRepoStub{}, &config.Config{})
	svc.connectivityResolver = func(_ context.Context, _ string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	endpoints, err := svc.buildConnectivityEndpoints(context.Background(), connectivitySettings{
		AllowedOrigins:  []string{"https://api.example.com"},
		CustomEndpoints: string(payload),
		Samples:         10,
		Warmup:          1,
		Burst:           defaultConnectivityProbeBurst,
	})

	require.NoError(t, err)
	require.Len(t, endpoints, maxConnectivityEndpoints)
	require.False(t, endpoints[0].IsDefault)
}

func TestConnectivityURLNormalizationRejectsAmbiguousPaths(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{name: "scoped public IPv6", url: "https://[2606:4700:4700::1111%25eth0]/v1"},
		{name: "encoded path separator", url: "https://api.example.com/v1%2Fadmin"},
		{name: "backslash path", url: `https://api.example.com/v1\admin`},
		{name: "parent path segment", url: "https://api.example.com/v1/../admin"},
		{name: "current path segment", url: "https://api.example.com/v1/./models"},
		{name: "space in path", url: "https://api.example.com/v 1"},
		{name: "non ASCII path", url: "https://api.example.com/模型"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := normalizeConnectivityAPIURL(tt.url)
			require.Error(t, err)
		})
	}

	_, _, err := normalizeConnectivityOrigin("https://api.example.com/./")
	require.Error(t, err)
}

func TestConnectivityURLNormalizationCanonicalizesNumericPorts(t *testing.T) {
	apiURL, origin, err := normalizeConnectivityAPIURL("https://API.Example.com:00443/v1/")
	require.NoError(t, err)
	require.Equal(t, "https://api.example.com/v1", apiURL)
	require.Equal(t, "https://api.example.com", origin)

	apiURL, origin, err = normalizeConnectivityAPIURL("https://api.example.com:008443/v1")
	require.NoError(t, err)
	require.Equal(t, "https://api.example.com:8443/v1", apiURL)
	require.Equal(t, "https://api.example.com:8443", origin)
}

func TestConnectivityAddressValidationRejectsSpecialUseIPv6(t *testing.T) {
	for _, value := range []string{
		"::192.0.2.1",
		"64:ff9b::a00:1",
		"64:ff9b:1::1",
		"100::1",
		"2001::1",
		"2002:0808:0808::1",
		"3fff::1",
		"5f00::1",
	} {
		t.Run(value, func(t *testing.T) {
			require.False(t, isPublicConnectivityAddr(netip.MustParseAddr(value)))
		})
	}

	require.True(t, isPublicConnectivityAddr(netip.MustParseAddr("2606:4700:4700::1111")))
	require.False(t, isPublicConnectivityAddr(netip.MustParseAddr("2606:4700:4700::1111%eth0")))
}

func TestConnectivityProbeClientIPRequiresEnabledSettingsAndSafeDeployment(t *testing.T) {
	repo := &settingPublicRepoStub{values: map[string]string{
		SettingKeyConnectivityTestEnabled:         "true",
		SettingKeyConnectivityClientIPEnabled:     "true",
		SettingKeyConnectivityProbeAllowedOrigins: `["https://8.8.8.8"]`,
		SettingKeyAPIBaseURL:                      "https://8.8.8.8/v1",
	}}
	cfg := &config.Config{
		Server: config.ServerConfig{
			TrustedProxiesConfigured: true,
			TrustedProxies:           []string{"203.0.113.0/24"},
		},
		Connectivity: config.ConnectivityConfig{
			ClientIPDeniedCIDRs: []string{"9.9.9.0/24"},
			ClientIPMaxHops:     8,
		},
	}
	svc := NewSettingService(repo, cfg)
	require.NoError(t, svc.LoadConnectivityProbeSettings(context.Background()))

	req := httptest.NewRequest("GET", "/.well-known/sub2api/edge-probe", nil)
	req.RemoteAddr = "203.0.113.10:443"
	req.Header.Set("X-Forwarded-For", "8.8.4.4")

	clientIP, ok := svc.ConnectivityProbeClientIP(req)
	require.True(t, ok)
	require.Equal(t, "8.8.4.4", clientIP)

	svcWithoutDeniedNodes := NewSettingService(repo, &config.Config{
		Server: cfg.Server,
		Connectivity: config.ConnectivityConfig{
			ClientIPMaxHops: 8,
		},
	})
	require.NoError(t, svcWithoutDeniedNodes.LoadConnectivityProbeSettings(context.Background()))
	_, ok = svcWithoutDeniedNodes.ConnectivityProbeClientIP(req)
	require.False(t, ok)
	unsafeSnapshot := svcWithoutDeniedNodes.ConnectivityProbeSnapshot()
	require.False(t, unsafeSnapshot.ClientIPEnabled)
}

func TestConnectivitySettingsRejectInvalidThresholdsBeforeWrite(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})
	thresholds := DefaultConnectivityGradeThresholds()
	thresholds.Excellent.MinSuccessRate = thresholds.Good.MinSuccessRate

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		ConnectivityGradeThresholds:  thresholds,
		ReferralCreditConversionRate: 1,
	})

	require.ErrorContains(t, err, "success-rate thresholds")
	require.Nil(t, repo.updates)
}

func TestConnectivitySettingsRejectAllowedOriginWithPathBeforeWrite(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		ConnectivityGradeThresholds:     DefaultConnectivityGradeThresholds(),
		ConnectivityProbeAllowedOrigins: []string{"https://api.example.com/v1"},
		ReferralCreditConversionRate:    1,
	})

	require.ErrorContains(t, err, "must not contain a path")
	require.Nil(t, repo.updates)
}

func TestConnectivitySettingsRejectEnabledWithoutEligibleOriginBeforeWrite(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		ConnectivityTestEnabled:         true,
		ConnectivityGradeThresholds:     DefaultConnectivityGradeThresholds(),
		ConnectivityProbeAllowedOrigins: []string{"https://other.example.com"},
		APIBaseURL:                      "https://api.example.com/v1",
		ReferralCreditConversionRate:    1,
	})

	require.ErrorContains(t, err, "at least one eligible API origin")
	require.Nil(t, repo.updates)
}

func TestConnectivitySettingsRejectMoreThanElevenEligibleURLsBeforeWrite(t *testing.T) {
	custom := make([]connectivityCustomEndpoint, 0, maxConnectivityEndpoints+1)
	for i := range maxConnectivityEndpoints + 1 {
		custom = append(custom, connectivityCustomEndpoint{
			Name:     fmt.Sprintf("端点 %d", i+1),
			Endpoint: fmt.Sprintf("https://api.example.com/v%d", i+1),
		})
	}
	payload, err := json.Marshal(custom)
	require.NoError(t, err)

	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})
	err = svc.UpdateSettings(context.Background(), &SystemSettings{
		ConnectivityTestEnabled:         true,
		ConnectivityGradeThresholds:     DefaultConnectivityGradeThresholds(),
		ConnectivityProbeAllowedOrigins: []string{"https://api.example.com"},
		CustomEndpoints:                 string(payload),
		ReferralCreditConversionRate:    1,
	})

	require.ErrorContains(t, err, "must not expose more than 11 URLs")
	require.Nil(t, repo.updates)
}

func TestConnectivitySettingsRejectBurstBelowSingleRunBudgetBeforeWrite(t *testing.T) {
	custom, err := json.Marshal([]connectivityCustomEndpoint{{
		Name:     "备用端点",
		Endpoint: "https://alt.example.com/v1",
	}})
	require.NoError(t, err)
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err = svc.UpdateSettings(context.Background(), &SystemSettings{
		ConnectivityTestEnabled:         true,
		ConnectivityGradeThresholds:     DefaultConnectivityGradeThresholds(),
		ConnectivityProbeSamples:        5,
		ConnectivityProbeWarmup:         0,
		ConnectivityProbeAllowedOrigins: []string{"https://api.example.com", "https://alt.example.com"},
		ConnectivityProbeBurst:          9,
		APIBaseURL:                      "https://api.example.com/v1",
		CustomEndpoints:                 string(custom),
		ReferralCreditConversionRate:    1,
	})

	require.ErrorContains(t, err, "burst must cover the single-run request budget of 10")
	require.Nil(t, repo.updates)
}

func TestConnectivityStoredBurstBelowSingleRunBudgetDisablesSnapshot(t *testing.T) {
	repo := &settingPublicRepoStub{values: map[string]string{
		SettingKeyConnectivityTestEnabled:         "true",
		SettingKeyConnectivityProbeSamples:        "5",
		SettingKeyConnectivityProbeWarmup:         "0",
		SettingKeyConnectivityProbeBurst:          "4",
		SettingKeyConnectivityProbeAllowedOrigins: `["https://api.example.com"]`,
		SettingKeyAPIBaseURL:                      "https://api.example.com/v1",
	}}
	svc := NewSettingService(repo, &config.Config{})
	svc.connectivityResolver = func(_ context.Context, _ string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}

	require.NoError(t, svc.LoadConnectivityProbeSettings(context.Background()))
	require.False(t, svc.ConnectivityProbeSnapshot().Enabled)
}

func TestValidateConnectivitySettingsUpdateRejectsDNSFailureForChangedActiveOrigin(t *testing.T) {
	svc := NewSettingService(&settingUpdateRepoStub{}, &config.Config{})
	svc.connectivityResolver = func(_ context.Context, host string) ([]netip.Addr, error) {
		require.Equal(t, "new.example.com", host)
		return nil, errors.New("nxdomain")
	}
	previous := connectivityTestSystemSettings("https://old.example.com/v1", []string{"https://old.example.com"})
	next := connectivityTestSystemSettings("https://new.example.com/v1", []string{"https://new.example.com"})

	err := svc.ValidateConnectivitySettingsUpdate(context.Background(), previous, next)

	require.ErrorContains(t, err, "new.example.com")
	require.ErrorContains(t, err, "resolve host")
}

func TestValidateConnectivitySettingsUpdateSkipsDNSWhenEndpointDefinitionIsUnchanged(t *testing.T) {
	svc := NewSettingService(&settingUpdateRepoStub{}, &config.Config{})
	resolverCalls := 0
	svc.connectivityResolver = func(_ context.Context, _ string) ([]netip.Addr, error) {
		resolverCalls++
		return nil, errors.New("unexpected resolver call")
	}
	previous := connectivityTestSystemSettings("https://api.example.com/v1", []string{"https://api.example.com"})
	next := *previous
	next.ConnectivityProbeSamples = 12

	require.NoError(t, svc.ValidateConnectivitySettingsUpdate(context.Background(), previous, &next))
	require.Zero(t, resolverCalls)
}

func TestValidateConnectivitySettingsUpdateIgnoresUnusedAllowedOrigin(t *testing.T) {
	svc := NewSettingService(&settingUpdateRepoStub{}, &config.Config{})
	resolvedHosts := []string{}
	svc.connectivityResolver = func(_ context.Context, host string) ([]netip.Addr, error) {
		resolvedHosts = append(resolvedHosts, host)
		if host == "stale.example.com" {
			return nil, errors.New("stale origin must not be resolved")
		}
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	previous := connectivityTestSystemSettings("https://api.example.com/v1", []string{"https://api.example.com"})
	next := connectivityTestSystemSettings(
		"https://api.example.com/v1",
		[]string{"https://api.example.com", "https://stale.example.com"},
	)

	require.NoError(t, svc.ValidateConnectivitySettingsUpdate(context.Background(), previous, next))
	require.Equal(t, []string{"api.example.com"}, resolvedHosts)
}

func connectivityTestSystemSettings(apiBaseURL string, allowedOrigins []string) *SystemSettings {
	return &SystemSettings{
		ConnectivityTestEnabled:         true,
		ConnectivityGradeThresholds:     DefaultConnectivityGradeThresholds(),
		ConnectivityProbeSamples:        10,
		ConnectivityProbeWarmup:         1,
		ConnectivityProbeMaxConcurrency: 3,
		ConnectivityProbeTimeoutMS:      10000,
		ConnectivityProbeAllowedOrigins: append([]string(nil), allowedOrigins...),
		ConnectivityProbeIPRPM:          360,
		ConnectivityProbeBurst:          250,
		APIBaseURL:                      apiBaseURL,
	}
}

func TestConnectivitySettingsPersistNormalizedValues(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})
	svc.connectivityResolver = func(_ context.Context, _ string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		ConnectivityTestEnabled:         true,
		ConnectivityGradeThresholds:     DefaultConnectivityGradeThresholds(),
		ConnectivityProbeSamples:        12,
		ConnectivityProbeWarmup:         2,
		ConnectivityProbeMaxConcurrency: 2,
		ConnectivityProbeTimeoutMS:      9000,
		ConnectivityProbeAllowedOrigins: []string{" HTTPS://API.Example.com:443/ "},
		ConnectivityProbeIPRPM:          400,
		ConnectivityProbeBurst:          250,
		APIBaseURL:                      "https://api.example.com/v1",
		ReferralCreditConversionRate:    1,
	})

	require.NoError(t, err)
	require.Equal(t, "true", repo.updates[SettingKeyConnectivityTestEnabled])
	require.JSONEq(t, `["https://api.example.com"]`, repo.updates[SettingKeyConnectivityProbeAllowedOrigins])
	require.Equal(t, "12", repo.updates[SettingKeyConnectivityProbeSamples])
	require.Equal(t, "2", repo.updates[SettingKeyConnectivityProbeWarmup])
	require.Equal(t, "2", repo.updates[SettingKeyConnectivityProbeMaxConcurrency])
	require.Equal(t, "9000", repo.updates[SettingKeyConnectivityProbeTimeoutMS])
	require.Equal(t, "400", repo.updates[SettingKeyConnectivityProbeIPRPM])
	require.Equal(t, "250", repo.updates[SettingKeyConnectivityProbeBurst])
	require.JSONEq(t, mustMarshalConnectivityThresholds(t, DefaultConnectivityGradeThresholds()), repo.updates[SettingKeyConnectivityGradeThresholds])
}

func mustMarshalConnectivityThresholds(t *testing.T, thresholds ConnectivityGradeThresholds) string {
	t.Helper()
	payload, err := json.Marshal(thresholds)
	require.NoError(t, err)
	return string(payload)
}

type geoipResolverStub struct {
	location *geoip.Location
	err      error
	ready    bool
	lookedUp []netip.Addr
}

type blockingGeoIPResolver struct {
	lookupStarted chan struct{}
	releaseLookup chan struct{}
	closeCalled   chan struct{}
	closed        atomic.Bool
}

func (s *blockingGeoIPResolver) Lookup(netip.Addr) (*geoip.Location, error) {
	close(s.lookupStarted)
	<-s.releaseLookup
	if s.closed.Load() {
		return nil, errors.New("resolver closed during lookup")
	}
	return &geoip.Location{CountryCode: "CN", Country: "中国"}, nil
}

func (s *blockingGeoIPResolver) Ready() bool { return !s.closed.Load() }

func (s *blockingGeoIPResolver) Close() error {
	s.closed.Store(true)
	close(s.closeCalled)
	return nil
}

func (s *geoipResolverStub) Lookup(addr netip.Addr) (*geoip.Location, error) {
	s.lookedUp = append(s.lookedUp, addr)
	if s.err != nil {
		return nil, s.err
	}
	if s.location == nil {
		return nil, nil
	}
	copied := *s.location
	return &copied, nil
}

func (s *geoipResolverStub) Ready() bool  { return s.ready }
func (s *geoipResolverStub) Close() error { return nil }

func newConnectivityClientContextTestService(t *testing.T, cfg *config.Config, values map[string]string) *SettingService {
	t.Helper()
	svc := NewSettingService(&settingPublicRepoStub{values: values}, cfg)
	require.NoError(t, svc.LoadConnectivityProbeSettings(context.Background()))
	return svc
}

func safeConnectivityClientConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			TrustedProxiesConfigured: true,
			TrustedProxies:           []string{"203.0.113.0/24"},
		},
		Connectivity: config.ConnectivityConfig{
			ClientIPDeniedCIDRs: []string{"9.9.9.0/24"},
			ClientIPMaxHops:     8,
		},
	}
}

func connectivityProbeRequest() *httptest.ResponseRecorder {
	// unused; kept for symmetry
	return nil
}

func TestConnectivityProbeClientContextReturnsIPAndLocation(t *testing.T) {
	values := map[string]string{
		SettingKeyConnectivityTestEnabled:         "true",
		SettingKeyConnectivityClientIPEnabled:     "true",
		SettingKeyConnectivityProbeAllowedOrigins: `["https://8.8.8.8"]`,
		SettingKeyAPIBaseURL:                      "https://8.8.8.8/v1",
	}
	svc := newConnectivityClientContextTestService(t, safeConnectivityClientConfig(), values)
	stub := &geoipResolverStub{
		ready:    true,
		location: &geoip.Location{CountryCode: "CN", Country: "中国", Region: "广东", City: "深圳"},
	}
	svc.connectivityGeoIP = stub

	req := httptest.NewRequest("GET", "/.well-known/sub2api/edge-probe", nil)
	req.RemoteAddr = "203.0.113.10:443"
	req.Header.Set("X-Forwarded-For", "8.8.4.4")

	ctx := svc.ConnectivityProbeClientContext(req)
	require.NotNil(t, ctx)
	require.Equal(t, "8.8.4.4", ctx.IP)
	require.NotNil(t, ctx.Location)
	require.Equal(t, "CN", ctx.Location.CountryCode)
	require.Equal(t, "中国", ctx.Location.Country)
	require.Equal(t, "广东", ctx.Location.Region)
	require.Equal(t, "深圳", ctx.Location.City)
	require.Len(t, stub.lookedUp, 1)
	require.Equal(t, netip.MustParseAddr("8.8.4.4"), stub.lookedUp[0])
}

func TestConnectivityProbeClientContextKeepsIPWhenGeoIPFails(t *testing.T) {
	values := map[string]string{
		SettingKeyConnectivityTestEnabled:         "true",
		SettingKeyConnectivityClientIPEnabled:     "true",
		SettingKeyConnectivityProbeAllowedOrigins: `["https://8.8.8.8"]`,
		SettingKeyAPIBaseURL:                      "https://8.8.8.8/v1",
	}
	svc := newConnectivityClientContextTestService(t, safeConnectivityClientConfig(), values)
	svc.connectivityGeoIP = &geoipResolverStub{ready: true, err: errors.New("boom")}
	now := time.Unix(1000, 0)
	svc.geoipFailureCache.now = func() time.Time { return now }

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.10:443"
	req.Header.Set("X-Forwarded-For", "8.8.4.4")

	ctx := svc.ConnectivityProbeClientContext(req)
	require.NotNil(t, ctx)
	require.Equal(t, "8.8.4.4", ctx.IP)
	require.Nil(t, ctx.Location)

	ctx = svc.ConnectivityProbeClientContext(req)
	require.NotNil(t, ctx)
	require.Equal(t, "8.8.4.4", ctx.IP)
	require.Nil(t, ctx.Location)
	stub := svc.connectivityGeoIP.(*geoipResolverStub)
	require.Len(t, stub.lookedUp, 1, "the same failing IP should be short-cached")

	now = now.Add(defaultGeoIPFailureCacheTTL)
	ctx = svc.ConnectivityProbeClientContext(req)
	require.NotNil(t, ctx)
	require.Nil(t, ctx.Location)
	require.Len(t, stub.lookedUp, 2, "the lookup should be retried after the failure cache expires")
}

func TestConnectivityProbeClientContextKeepsIPWhenGeoIPHasNoMatch(t *testing.T) {
	values := map[string]string{
		SettingKeyConnectivityTestEnabled:         "true",
		SettingKeyConnectivityClientIPEnabled:     "true",
		SettingKeyConnectivityProbeAllowedOrigins: `["https://8.8.8.8"]`,
		SettingKeyAPIBaseURL:                      "https://8.8.8.8/v1",
	}
	svc := newConnectivityClientContextTestService(t, safeConnectivityClientConfig(), values)
	svc.connectivityGeoIP = &geoipResolverStub{ready: true, location: nil}

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.10:443"
	req.Header.Set("X-Forwarded-For", "8.8.4.4")

	ctx := svc.ConnectivityProbeClientContext(req)
	require.NotNil(t, ctx)
	require.Equal(t, "8.8.4.4", ctx.IP)
	require.Nil(t, ctx.Location)

	ctx = svc.ConnectivityProbeClientContext(req)
	require.NotNil(t, ctx)
	require.Nil(t, ctx.Location)
	stub := svc.connectivityGeoIP.(*geoipResolverStub)
	require.Len(t, stub.lookedUp, 2, "a database miss is not a read failure and must not be cached")
}

func TestGeoIPFailureCacheIsBoundedAndClearable(t *testing.T) {
	cache := newGeoIPFailureCache(time.Minute, 2)
	cache.record(netip.MustParseAddr("8.8.8.8"))
	cache.record(netip.MustParseAddr("1.1.1.1"))
	cache.record(netip.MustParseAddr("9.9.9.9"))
	cache.mu.Lock()
	entryCount := len(cache.entries)
	cache.mu.Unlock()
	require.LessOrEqual(t, entryCount, 2)

	addr := netip.MustParseAddr("9.9.9.9")
	require.True(t, cache.contains(addr))
	cache.clear(addr)
	require.False(t, cache.contains(addr))
}

func TestSettingServiceCloseWaitsForInFlightGeoIPLookup(t *testing.T) {
	values := map[string]string{
		SettingKeyConnectivityTestEnabled:         "true",
		SettingKeyConnectivityClientIPEnabled:     "true",
		SettingKeyConnectivityProbeAllowedOrigins: `["https://8.8.8.8"]`,
		SettingKeyAPIBaseURL:                      "https://8.8.8.8/v1",
	}
	svc := newConnectivityClientContextTestService(t, safeConnectivityClientConfig(), values)
	resolver := &blockingGeoIPResolver{
		lookupStarted: make(chan struct{}),
		releaseLookup: make(chan struct{}),
		closeCalled:   make(chan struct{}),
	}
	svc.connectivityGeoIPConfigured = true
	svc.connectivityGeoIP = resolver

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.10:443"
	req.Header.Set("X-Forwarded-For", "8.8.4.4")
	contextDone := make(chan *ConnectivityClientContext, 1)
	go func() { contextDone <- svc.ConnectivityProbeClientContext(req) }()
	<-resolver.lookupStarted

	closeDone := make(chan struct{})
	go func() {
		svc.Close()
		close(closeDone)
	}()
	select {
	case <-resolver.closeCalled:
		t.Fatal("service closed the GeoIP resolver while a lookup was in flight")
	case <-time.After(50 * time.Millisecond):
	}

	close(resolver.releaseLookup)
	ctx := <-contextDone
	require.NotNil(t, ctx)
	require.NotNil(t, ctx.Location)
	<-closeDone
	require.Equal(t, "unavailable", svc.ConnectivityGeoIPStatus())
}

func TestConnectivityProbeClientContextReturnsIPOnlyWhenGeoIPUnconfigured(t *testing.T) {
	values := map[string]string{
		SettingKeyConnectivityTestEnabled:         "true",
		SettingKeyConnectivityClientIPEnabled:     "true",
		SettingKeyConnectivityProbeAllowedOrigins: `["https://8.8.8.8"]`,
		SettingKeyAPIBaseURL:                      "https://8.8.8.8/v1",
	}
	svc := newConnectivityClientContextTestService(t, safeConnectivityClientConfig(), values)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.10:443"
	req.Header.Set("X-Forwarded-For", "8.8.4.4")

	ctx := svc.ConnectivityProbeClientContext(req)
	require.NotNil(t, ctx)
	require.Equal(t, "8.8.4.4", ctx.IP)
	require.Nil(t, ctx.Location)
}

func TestConnectivityProbeClientContextFailClosed(t *testing.T) {
	values := map[string]string{
		SettingKeyConnectivityTestEnabled:         "true",
		SettingKeyConnectivityClientIPEnabled:     "true",
		SettingKeyConnectivityProbeAllowedOrigins: `["https://8.8.8.8"]`,
		SettingKeyAPIBaseURL:                      "https://8.8.8.8/v1",
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.10:443"
	req.Header.Set("X-Forwarded-For", "8.8.4.4")

	t.Run("ip display disabled", func(t *testing.T) {
		off := map[string]string{}
		for key, value := range values {
			off[key] = value
		}
		off[SettingKeyConnectivityClientIPEnabled] = "false"
		svc := newConnectivityClientContextTestService(t, safeConnectivityClientConfig(), off)
		require.Nil(t, svc.ConnectivityProbeClientContext(req))
	})

	t.Run("denied cidrs missing", func(t *testing.T) {
		unsafe := &config.Config{
			Server: config.ServerConfig{
				TrustedProxiesConfigured: true,
				TrustedProxies:           []string{"203.0.113.0/24"},
			},
			Connectivity: config.ConnectivityConfig{ClientIPMaxHops: 8},
		}
		svc := newConnectivityClientContextTestService(t, unsafe, values)
		require.Nil(t, svc.ConnectivityProbeClientContext(req))
	})

	t.Run("feature disabled", func(t *testing.T) {
		off := map[string]string{}
		for key, value := range values {
			off[key] = value
		}
		off[SettingKeyConnectivityTestEnabled] = "false"
		svc := newConnectivityClientContextTestService(t, safeConnectivityClientConfig(), off)
		require.Nil(t, svc.ConnectivityProbeClientContext(req))
	})
}

func TestConnectivityGeoIPStatus(t *testing.T) {
	t.Run("not configured", func(t *testing.T) {
		svc := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{})
		require.Equal(t, "not_configured", svc.ConnectivityGeoIPStatus())
	})

	t.Run("configured and ready", func(t *testing.T) {
		svc := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{
			Connectivity: config.ConnectivityConfig{GeoIPDatabasePath: "/x.mmdb"},
		})
		// Open fails (no such file), so status must reflect that.
		require.Equal(t, "unavailable", svc.ConnectivityGeoIPStatus())
	})

	t.Run("ready after successful open", func(t *testing.T) {
		svc := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{})
		svc.connectivityGeoIPConfigured = true
		svc.connectivityGeoIP = &geoipResolverStub{ready: true}
		require.Equal(t, "ready", svc.ConnectivityGeoIPStatus())
	})
}
