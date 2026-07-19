//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type upstreamConnectionBalanceSourceStub struct {
	items      []*UpstreamConnection
	probeCalls []int64
}

type failingGetMultipleSettingRepo struct {
	*mockSettingRepo
}

func (r *failingGetMultipleSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, errors.New("settings unavailable")
}

func (s *upstreamConnectionBalanceSourceStub) List(_ context.Context, params UpstreamConnectionListParams) ([]*UpstreamConnection, int64, error) {
	start := (params.Page - 1) * params.PageSize
	if start >= len(s.items) {
		return []*UpstreamConnection{}, int64(len(s.items)), nil
	}
	end := start + params.PageSize
	if end > len(s.items) {
		end = len(s.items)
	}
	return s.items[start:end], int64(len(s.items)), nil
}

func (s *upstreamConnectionBalanceSourceStub) Get(_ context.Context, id int64) (*UpstreamConnection, error) {
	for _, item := range s.items {
		if item != nil && item.ID == id {
			return item, nil
		}
	}
	return nil, ErrUpstreamConnectionNotFound
}

func (s *upstreamConnectionBalanceSourceStub) Probe(_ context.Context, id int64) (*UpstreamConnection, error) {
	s.probeCalls = append(s.probeCalls, id)
	return s.Get(context.Background(), id)
}

func TestGetOpsAlertRuntimeSettingsIgnoresRetiredAccountBalanceSettings(t *testing.T) {
	t.Parallel()

	repo := newMockSettingRepo()
	retiredEnvelope := map[string]any{
		"evaluation_interval_seconds": 60,
		"account_balance": map[string]any{
			"enabled":               false,
			"default_threshold_usd": 23.5,
			"rate_limit_per_hour":   4,
			"notification": map[string]any{
				"enterprise_wechat_enabled":     true,
				"enterprise_wechat_webhook_url": "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=retired",
				"mention_all_on_low_balance":    true,
			},
		},
	}
	raw, err := json.Marshal(retiredEnvelope)
	require.NoError(t, err)
	require.NoError(t, repo.Set(context.Background(), SettingKeyOpsAlertRuntimeSettings, string(raw)))

	svc := &OpsService{settingRepo: repo}
	cfg, err := svc.GetOpsAlertRuntimeSettings(context.Background())
	require.NoError(t, err)
	require.NotNil(t, cfg.UpstreamConnectionBalance)
	defaults := defaultOpsUpstreamConnectionBalanceSettings()
	require.Equal(t, defaults, *cfg.UpstreamConnectionBalance)
}

func TestUpdateOpsUpstreamConnectionBalanceSettingsUsesIndependentSetting(t *testing.T) {
	t.Parallel()

	repo := newMockSettingRepo()
	runtimeSettings := defaultOpsAlertRuntimeSettings()
	runtimeSettings.UpstreamConnectionBalance = nil
	runtimeRaw, err := json.Marshal(runtimeSettings)
	require.NoError(t, err)
	require.NoError(t, repo.Set(context.Background(), SettingKeyOpsAlertRuntimeSettings, string(runtimeRaw)))

	svc := &OpsService{settingRepo: repo}
	settings := defaultOpsUpstreamConnectionBalanceSettings()
	settings.Enabled = false
	settings.DefaultThresholdUSD = 27.5
	updated, err := svc.UpdateOpsUpstreamConnectionBalanceSettings(context.Background(), settings)
	require.NoError(t, err)
	require.False(t, updated.Enabled)
	require.Equal(t, 27.5, updated.DefaultThresholdUSD)

	storedRuntimeRaw, err := repo.GetValue(context.Background(), SettingKeyOpsAlertRuntimeSettings)
	require.NoError(t, err)
	require.Equal(t, string(runtimeRaw), storedRuntimeRaw)
	storedSettingsRaw, err := repo.GetValue(context.Background(), SettingKeyOpsUpstreamConnectionBalance)
	require.NoError(t, err)
	var storedSettings OpsUpstreamConnectionBalanceSettings
	require.NoError(t, json.Unmarshal([]byte(storedSettingsRaw), &storedSettings))
	require.False(t, storedSettings.Enabled)
	require.Equal(t, 27.5, storedSettings.DefaultThresholdUSD)

	resolved, err := svc.GetOpsAlertRuntimeSettings(context.Background())
	require.NoError(t, err)
	require.NotNil(t, resolved.UpstreamConnectionBalance)
	require.False(t, resolved.UpstreamConnectionBalance.Enabled)
	require.Equal(t, 27.5, resolved.UpstreamConnectionBalance.DefaultThresholdUSD)
}

func TestListUpstreamConnectionBalanceMonitorUsesSharedWalletSnapshots(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	lowWallet := 6.25
	healthyWallet := 80.0
	source := &upstreamConnectionBalanceSourceStub{items: []*UpstreamConnection{
		{
			ID: 11, Name: "active upstream", Provider: UpstreamConnectionProviderNewAPI,
			Status: UpstreamConnectionStatusReady, SyncEnabled: true, SyncIntervalSeconds: 300,
			BindingCount: 2, BoundAccountIDs: []int64{101, 102}, WalletUSD: &lowWallet,
			WalletAmount: &lowWallet, WalletCurrency: "USD", WalletObservedAt: connectionBalanceTimePtr(now.Add(-time.Minute)),
		},
		{
			ID: 12, Name: "unbound upstream", Provider: UpstreamConnectionProviderSub2API,
			Status: UpstreamConnectionStatusReady, SyncEnabled: true, SyncIntervalSeconds: 300,
			BindingCount: 0, WalletUSD: &healthyWallet, WalletObservedAt: connectionBalanceTimePtr(now.Add(-time.Minute)),
		},
	}}
	repo := newMockSettingRepo()
	svc := &OpsService{settingRepo: repo}
	svc.SetUpstreamConnectionBalanceSource(source)

	response, err := svc.ListUpstreamConnectionBalanceMonitor(context.Background(), OpsUpstreamConnectionBalanceMonitorFilter{
		Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Len(t, response.Items, 2)
	require.Equal(t, 2, response.Summary.TotalConnections)
	require.Equal(t, 1, response.Summary.MonitoredConnections)
	require.Equal(t, 1, response.Summary.LowBalanceConnections)
	require.Equal(t, int64(11), response.Items[0].ConnectionID)
	require.True(t, response.Items[0].Alert.Enabled)
	require.True(t, response.Items[0].Alert.Low)
	require.True(t, response.Items[0].Alert.SnapshotFresh)
	require.Equal(t, []int64{101, 102}, response.Items[0].BoundAccountIDs)
	require.False(t, response.Items[1].Alert.Eligible)
}

func TestListUpstreamConnectionBalanceMonitorReportsNoMonitoredConnectionsWhenGloballyDisabled(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	wallet := 80.0
	source := &upstreamConnectionBalanceSourceStub{items: []*UpstreamConnection{{
		ID: 13, Name: "paused upstream", Provider: UpstreamConnectionProviderNewAPI,
		Status: UpstreamConnectionStatusReady, SyncEnabled: true, SyncIntervalSeconds: 300,
		BindingCount: 1, BoundAccountIDs: []int64{103}, WalletUSD: &wallet,
		WalletAmount: &wallet, WalletCurrency: "USD", WalletObservedAt: connectionBalanceTimePtr(now.Add(-time.Minute)),
	}}}
	repo := newMockSettingRepo()
	settings := defaultOpsUpstreamConnectionBalanceSettings()
	settings.Enabled = false
	raw, err := json.Marshal(settings)
	require.NoError(t, err)
	require.NoError(t, repo.Set(context.Background(), SettingKeyOpsUpstreamConnectionBalance, string(raw)))
	svc := &OpsService{settingRepo: repo}
	svc.SetUpstreamConnectionBalanceSource(source)

	response, err := svc.ListUpstreamConnectionBalanceMonitor(context.Background(), OpsUpstreamConnectionBalanceMonitorFilter{
		Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, 0, response.Summary.MonitoredConnections)
	require.True(t, response.Items[0].Alert.Enabled)
}

func TestUpdateUpstreamConnectionBalanceAlertPersistsOverrideWithoutTouchingConnection(t *testing.T) {
	t.Parallel()

	source := &upstreamConnectionBalanceSourceStub{items: []*UpstreamConnection{{ID: 21, Name: "upstream"}}}
	repo := newMockSettingRepo()
	svc := &OpsService{settingRepo: repo}
	svc.SetUpstreamConnectionBalanceSource(source)
	enabled := false
	threshold := 42.75

	state, err := svc.UpdateUpstreamConnectionBalanceAlert(context.Background(), 21, OpsUpstreamConnectionBalanceAlertUpdate{
		Enabled: &enabled, ThresholdUSD: &threshold,
	})
	require.NoError(t, err)
	require.False(t, state.Enabled)
	require.Equal(t, 42.75, state.ThresholdUSD)

	raw, err := repo.GetValue(context.Background(), upstreamConnectionBalanceOverrideKey(21))
	require.NoError(t, err)
	var override OpsUpstreamConnectionBalanceOverride
	require.NoError(t, json.Unmarshal([]byte(raw), &override))
	require.False(t, override.Enabled)
	require.Equal(t, 42.75, *override.ThresholdUSD)
	require.Empty(t, source.probeCalls)
}

func TestUpdateUpstreamConnectionBalanceAlertRestoresDefaultByDeletingOverride(t *testing.T) {
	t.Parallel()

	source := &upstreamConnectionBalanceSourceStub{items: []*UpstreamConnection{{ID: 22, Name: "upstream"}}}
	repo := newMockSettingRepo()
	svc := &OpsService{settingRepo: repo}
	svc.SetUpstreamConnectionBalanceSource(source)
	disabled := false
	customThreshold := 18.5

	_, err := svc.UpdateUpstreamConnectionBalanceAlert(context.Background(), 22, OpsUpstreamConnectionBalanceAlertUpdate{
		Enabled: &disabled, ThresholdUSD: &customThreshold,
	})
	require.NoError(t, err)

	enabled := true
	useDefault := true
	state, err := svc.UpdateUpstreamConnectionBalanceAlert(context.Background(), 22, OpsUpstreamConnectionBalanceAlertUpdate{
		Enabled: &enabled, UseDefaultThreshold: &useDefault,
	})
	require.NoError(t, err)
	require.True(t, state.Enabled)
	require.True(t, state.UsesDefault)
	require.Equal(t, defaultOpsUpstreamConnectionBalanceSettings().DefaultThresholdUSD, state.ThresholdUSD)
	raw, err := repo.GetValue(context.Background(), upstreamConnectionBalanceOverrideKey(22))
	require.NoError(t, err)
	require.Empty(t, raw)
}

func TestListUpstreamConnectionBalanceMonitorRejectsCorruptOverride(t *testing.T) {
	t.Parallel()

	source := &upstreamConnectionBalanceSourceStub{items: []*UpstreamConnection{{ID: 23, Name: "upstream"}}}
	repo := newMockSettingRepo()
	require.NoError(t, repo.Set(context.Background(), upstreamConnectionBalanceOverrideKey(23), "not-json"))
	svc := &OpsService{settingRepo: repo}
	svc.SetUpstreamConnectionBalanceSource(source)

	_, err := svc.ListUpstreamConnectionBalanceMonitor(context.Background(), OpsUpstreamConnectionBalanceMonitorFilter{Page: 1, PageSize: 20})
	require.ErrorContains(t, err, "invalid upstream connection balance override")
}

func TestProbeUpstreamConnectionBalanceDelegatesToSharedConnection(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	wallet := 12.0
	source := &upstreamConnectionBalanceSourceStub{items: []*UpstreamConnection{{
		ID: 31, Name: "upstream", Status: UpstreamConnectionStatusReady, SyncEnabled: true,
		SyncIntervalSeconds: 300, BindingCount: 1, WalletUSD: &wallet, WalletObservedAt: &now,
	}}}
	svc := &OpsService{settingRepo: newMockSettingRepo()}
	svc.SetUpstreamConnectionBalanceSource(source)

	item, err := svc.ProbeUpstreamConnectionBalance(context.Background(), 31)
	require.NoError(t, err)
	require.Equal(t, []int64{31}, source.probeCalls)
	require.Equal(t, int64(31), item.ConnectionID)
	require.Equal(t, &wallet, item.WalletUSD)
}

func TestProbeUpstreamConnectionBalanceDoesNotProbeWhenCooldownStateCannotLoad(t *testing.T) {
	t.Parallel()

	source := &upstreamConnectionBalanceSourceStub{items: []*UpstreamConnection{{ID: 32, Name: "upstream"}}}
	svc := &OpsService{settingRepo: &failingGetMultipleSettingRepo{mockSettingRepo: newMockSettingRepo()}}
	svc.SetUpstreamConnectionBalanceSource(source)

	_, err := svc.ProbeUpstreamConnectionBalance(context.Background(), 32)
	require.ErrorContains(t, err, "settings unavailable")
	require.Empty(t, source.probeCalls)
}

func TestUpstreamConnectionBalanceSnapshotFreshRejectsStaleWallet(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	observedAt := now.Add(-20 * time.Minute)
	connection := &UpstreamConnection{SyncEnabled: true, SyncIntervalSeconds: 60, WalletObservedAt: &observedAt}
	require.False(t, upstreamConnectionBalanceSnapshotFresh(connection, now))

	observedAt = now.Add(-2 * time.Minute)
	require.True(t, upstreamConnectionBalanceSnapshotFresh(connection, now))
}

func TestUpstreamConnectionBalanceZeroThresholdStillAlertsAtZero(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	wallet := 0.0
	settings := defaultOpsUpstreamConnectionBalanceSettings()
	settings.DefaultThresholdUSD = 0
	item := buildOpsUpstreamConnectionBalanceItem(&UpstreamConnection{
		ID: 41, Name: "depleted upstream", Status: UpstreamConnectionStatusReady,
		SyncEnabled: true, SyncIntervalSeconds: 300, BindingCount: 1,
		WalletUSD: &wallet, WalletObservedAt: &now,
	}, settings, nil, nil, now)

	require.True(t, item.Alert.Enabled)
	require.True(t, item.Alert.Eligible)
	require.True(t, item.Alert.SnapshotFresh)
	require.True(t, item.Alert.Low)
}

func connectionBalanceTimePtr(value time.Time) *time.Time {
	return &value
}
