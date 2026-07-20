//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateOpsEnterpriseWeChatWebhookURL(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateOpsEnterpriseWeChatWebhookURL("https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc"))
	require.Error(t, validateOpsEnterpriseWeChatWebhookURL("https://example.com/cgi-bin/webhook/send?key=abc"))
	require.Error(t, validateOpsEnterpriseWeChatWebhookURL("http://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc"))
	require.Error(t, validateOpsEnterpriseWeChatWebhookURL("https://qyapi.weixin.qq.com/other?key=abc"))
}

func TestStripInternalAccountHealthWindows(t *testing.T) {
	t.Parallel()

	windows := map[string]*OpsAccountHealthWindowStats{
		OpsAccountHealthWindow1m:     {Window: OpsAccountHealthWindow1m, RequestCount: 10},
		OpsAccountHealthWindowPrev1m: {Window: OpsAccountHealthWindowPrev1m, RequestCount: 8},
		OpsAccountHealthWindow5m:     {Window: OpsAccountHealthWindow5m, RequestCount: 40},
	}
	// Trend must still see prev_1m before strip.
	trend := computeSuccessRateTrend1mFromWindows(windows)
	require.Equal(t, int64(10), trend.CurrentRequestCount)
	require.Equal(t, int64(8), trend.PreviousRequestCount)

	stripInternalAccountHealthWindows(windows)
	require.NotContains(t, windows, OpsAccountHealthWindowPrev1m)
	require.Contains(t, windows, OpsAccountHealthWindow1m)
	require.Contains(t, windows, OpsAccountHealthWindow5m)
	stripInternalAccountHealthWindows(nil) // no panic
}

func TestComputeSuccessRateTrend1mFromWindows(t *testing.T) {
	t.Parallel()

	// Full-window aggregates (as produced by SQL), not capped recent samples.
	windows := map[string]*OpsAccountHealthWindowStats{
		OpsAccountHealthWindow1m: {
			Window:             OpsAccountHealthWindow1m,
			RequestCount:       20,
			SuccessCount:       12,
			SuccessRatePercent: 60,
		},
		OpsAccountHealthWindowPrev1m: {
			Window:             OpsAccountHealthWindowPrev1m,
			RequestCount:       20,
			SuccessCount:       19,
			SuccessRatePercent: 95,
		},
	}
	trend := computeSuccessRateTrend1mFromWindows(windows)
	require.NotNil(t, trend)
	require.Equal(t, "down", trend.Direction)
	require.Equal(t, int64(20), trend.CurrentRequestCount)
	require.Equal(t, int64(20), trend.PreviousRequestCount)
	require.InDelta(t, -35.0, trend.DeltaPercent, 0.01)

	// High volume still works — trend uses full counts, not a 60-sample cap.
	busy := map[string]*OpsAccountHealthWindowStats{
		OpsAccountHealthWindow1m: {
			RequestCount:       5000,
			SuccessRatePercent: 99.2,
		},
		OpsAccountHealthWindowPrev1m: {
			RequestCount:       4800,
			SuccessRatePercent: 97.0,
		},
	}
	up := computeSuccessRateTrend1mFromWindows(busy)
	require.Equal(t, "up", up.Direction)
	require.Equal(t, int64(5000), up.CurrentRequestCount)

	// too few samples => unknown
	sparse := computeSuccessRateTrend1mFromWindows(map[string]*OpsAccountHealthWindowStats{
		OpsAccountHealthWindow1m:     {RequestCount: 2, SuccessRatePercent: 50},
		OpsAccountHealthWindowPrev1m: {RequestCount: 2, SuccessRatePercent: 100},
	})
	require.Equal(t, "unknown", sparse.Direction)
}

func TestMaskAndPreserveOpsAccountHealthWebhook(t *testing.T) {
	t.Parallel()

	existing := defaultOpsAccountHealthSettings()
	existing.Notification.EnterpriseWeChatWebhookURL = "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=secret"

	masked := maskOpsAccountHealthSettings(existing)
	require.Equal(t, opsEnterpriseWeChatWebhookMask, masked.Notification.EnterpriseWeChatWebhookURL)

	next := defaultOpsAccountHealthSettings()
	next.Notification.EnterpriseWeChatWebhookURL = opsEnterpriseWeChatWebhookMask
	preserveMaskedOpsAccountHealthWebhook(&next, existing)
	require.Equal(t, existing.Notification.EnterpriseWeChatWebhookURL, next.Notification.EnterpriseWeChatWebhookURL)

	cleared := defaultOpsAccountHealthSettings()
	cleared.Notification.EnterpriseWeChatWebhookURL = ""
	preserveMaskedOpsAccountHealthWebhook(&cleared, existing)
	require.Empty(t, cleared.Notification.EnterpriseWeChatWebhookURL)
}

func TestDecideOpsAccountHealth_ClosedProbeSuccessCanOpen(t *testing.T) {
	t.Parallel()

	checkedAt := time.Now().UTC().Add(-5 * time.Minute)
	latency := int64(1234)
	item := &OpsAccountHealthItem{
		AccountID: 1,
		IsOpened:  false,
		Windows:   defaultOpsAccountHealthWindows(),
		Recent:    []*OpsAccountHealthSample{},
		Probe: &OpsAccountHealthProbe{
			Status:    "success",
			CheckedAt: &checkedAt,
			LatencyMs: &latency,
		},
	}
	settings := defaultOpsAccountHealthSettings()
	settings.Recovery.WindowMinutes = 30
	settings.Recovery.NotifyClosedAccounts = true

	rec := decideOpsAccountHealth(item, settings)
	require.Equal(t, OpsAccountHealthActionCanOpen, rec.Action)
	require.True(t, rec.RecoveryReady)
	require.Equal(t, OpsAccountHealthNotifyDigest, rec.NotifyMode)
}

func TestDecideOpsAccountHealth_ErrorStateProbeSuccessCanOpen(t *testing.T) {
	t.Parallel()

	checkedAt := time.Now().UTC().Add(-3 * time.Minute)
	item := &OpsAccountHealthItem{
		AccountID:    1,
		IsOpened:     false,
		HasError:     true,
		ErrorMessage: "previous upstream auth failure",
		Windows:      defaultOpsAccountHealthWindows(),
		Recent:       []*OpsAccountHealthSample{},
		Probe: &OpsAccountHealthProbe{
			Status:    "success",
			CheckedAt: &checkedAt,
		},
	}
	settings := defaultOpsAccountHealthSettings()
	settings.Recovery.WindowMinutes = 30
	settings.Recovery.NotifyClosedAccounts = true

	rec := decideOpsAccountHealth(item, settings)
	require.Equal(t, OpsAccountHealthActionCanOpen, rec.Action)
	require.True(t, rec.RecoveryReady)
	require.Equal(t, OpsAccountHealthNotifyDigest, rec.NotifyMode)
	require.Contains(t, rec.Title, "探测已恢复")
}

func TestDecideOpsAccountHealth_ClosedProbeSuccessDoesNotOverrideBadRecentTraffic(t *testing.T) {
	t.Parallel()

	checkedAt := time.Now().UTC().Add(-5 * time.Minute)
	item := &OpsAccountHealthItem{
		AccountID: 1,
		IsOpened:  false,
		Windows:   defaultOpsAccountHealthWindows(),
		Recent:    []*OpsAccountHealthSample{},
		Probe: &OpsAccountHealthProbe{
			Status:    "success",
			CheckedAt: &checkedAt,
		},
	}
	item.Windows[OpsAccountHealthWindow10m] = &OpsAccountHealthWindowStats{
		Window:             OpsAccountHealthWindow10m,
		RequestCount:       32,
		SuccessCount:       23,
		ErrorCount:         9,
		UpstreamErrorCount: 9,
	}
	settings := defaultOpsAccountHealthSettings()
	settings.Degrade.WindowMinutes = 10
	settings.Degrade.MinRequests = 20
	settings.Degrade.SuccessRateMinPercent = 90
	settings.Recovery.WindowMinutes = 30
	settings.Recovery.NotifyClosedAccounts = true

	normalizeAccountHealthMetrics(&OpsAccountHealthMetrics{Windows: item.Windows, Recent: item.Recent})
	rec := decideOpsAccountHealth(item, settings)
	require.Equal(t, OpsAccountHealthActionKeepClosed, rec.Action)
	require.False(t, rec.RecoveryReady)
	require.Equal(t, OpsAccountHealthNotifyNone, rec.NotifyMode)
	require.Contains(t, rec.Title, "短期变差")
}

func TestDecideOpsAccountHealth_OpenedSingleWindowDegradeOnlyWatches(t *testing.T) {
	t.Parallel()

	item := &OpsAccountHealthItem{
		AccountID: 1,
		IsOpened:  true,
		Windows:   defaultOpsAccountHealthWindows(),
		Recent:    []*OpsAccountHealthSample{},
	}
	item.Windows[OpsAccountHealthWindow10m] = accountHealthTestWindow(OpsAccountHealthWindow10m, 32, 23, 9, 9)

	settings := defaultOpsAccountHealthSettings()
	settings.Degrade.WindowMinutes = 10
	settings.Degrade.MinRequests = 20
	settings.Degrade.SuccessRateMinPercent = 90

	normalizeAccountHealthMetrics(&OpsAccountHealthMetrics{Windows: item.Windows, Recent: item.Recent})
	rec := decideOpsAccountHealth(item, settings)
	require.Equal(t, OpsAccountHealthActionWatch, rec.Action)
	require.Equal(t, "P2", rec.Severity)
	require.Equal(t, OpsAccountHealthNotifyNone, rec.NotifyMode)
	require.Contains(t, rec.Title, "短期变差")
}

func TestDecideOpsAccountHealth_OpenedSustainedDegradeDigestCloseNow(t *testing.T) {
	t.Parallel()

	item := &OpsAccountHealthItem{
		AccountID: 1,
		IsOpened:  true,
		Windows:   defaultOpsAccountHealthWindows(),
		Recent:    []*OpsAccountHealthSample{},
	}
	item.Windows[OpsAccountHealthWindow5m] = accountHealthTestWindow(OpsAccountHealthWindow5m, 24, 18, 6, 4)
	item.Windows[OpsAccountHealthWindow10m] = accountHealthTestWindow(OpsAccountHealthWindow10m, 50, 40, 10, 8)
	item.Windows[OpsAccountHealthWindow30m] = accountHealthTestWindow(OpsAccountHealthWindow30m, 120, 102, 18, 14)

	settings := defaultOpsAccountHealthSettings()
	settings.Degrade.WindowMinutes = 10
	settings.Degrade.MinRequests = 20
	settings.Degrade.SuccessRateMinPercent = 90

	normalizeAccountHealthMetrics(&OpsAccountHealthMetrics{Windows: item.Windows, Recent: item.Recent})
	rec := decideOpsAccountHealth(item, settings)
	require.Equal(t, OpsAccountHealthActionCloseNow, rec.Action)
	require.Equal(t, "P2", rec.Severity)
	require.Equal(t, OpsAccountHealthNotifyDigest, rec.NotifyMode)
	require.Contains(t, rec.Title, "持续变差")
	require.Contains(t, rec.Reason, "5m")
	require.Contains(t, rec.Reason, "10m")
	require.Contains(t, rec.Reason, "30m")
}

func TestAccountHealthProbeFromAccountSummarizesProbeHistory(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	account := &Account{
		ID: 1,
		Extra: map[string]any{
			accountHealthProbeStatusExtraKey:    "failed",
			accountHealthProbeCheckedAtExtraKey: now.Format(time.RFC3339Nano),
			accountHealthProbeLatencyMsExtraKey: 900,
			accountHealthProbeErrorExtraKey:     "upstream timeout",
			accountHealthProbeHistoryExtraKey: []*OpsAccountHealthSample{
				{Kind: "success", CreatedAt: now.Add(-4 * time.Minute), DurationMs: accountHealthTestIntPtr(1000)},
				{Kind: "success", CreatedAt: now.Add(-2 * time.Minute), DurationMs: accountHealthTestIntPtr(800)},
				{Kind: "error", CreatedAt: now, DurationMs: accountHealthTestIntPtr(900), Message: "upstream timeout"},
			},
		},
	}

	probe := accountHealthProbeFromAccount(account)
	require.NotNil(t, probe)
	require.Equal(t, "failed", probe.Status)
	require.Equal(t, int64(3), probe.RequestCount)
	require.Equal(t, int64(2), probe.SuccessCount)
	require.Equal(t, int64(1), probe.ErrorCount)
	require.InDelta(t, 66.666, probe.SuccessRatePercent, 0.01)
	require.InDelta(t, 33.333, probe.ErrorRatePercent, 0.01)
	require.NotNil(t, probe.AvgLatencyMs)
	require.InDelta(t, 900, *probe.AvgLatencyMs, 0.01)
	require.Len(t, probe.Recent, 3)
}

func TestAccountHealthProbeAutoDisabledFromAccount(t *testing.T) {
	t.Parallel()

	require.False(t, accountHealthProbeAutoDisabledFromAccount(&Account{}))
	require.False(t, accountHealthProbeAutoDisabledFromAccount(&Account{Extra: map[string]any{
		accountHealthProbeAutoDisabledKey: false,
	}}))
	require.True(t, accountHealthProbeAutoDisabledFromAccount(&Account{Extra: map[string]any{
		accountHealthProbeAutoDisabledKey: true,
	}}))
}

func TestUpdateAccountHealthProbeAutoStoresAccountExtra(t *testing.T) {
	t.Parallel()

	repo := &accountHealthAutoProbeRepo{
		mockAccountRepoForGemini: mockAccountRepoForGemini{
			accountsByID: map[int64]*Account{
				9: {ID: 9, Extra: map[string]any{}},
			},
		},
	}
	svc := &OpsService{accountRepo: repo}

	state, err := svc.UpdateAccountHealthProbeAuto(context.Background(), 9, false)

	require.NoError(t, err)
	require.Equal(t, int64(9), state.AccountID)
	require.True(t, state.ProbeAutoDisabled)
	require.Equal(t, int64(9), repo.updateID)
	require.Equal(t, true, repo.updates[accountHealthProbeAutoDisabledKey])
}

func TestUpdateAccountHealthProbeModelStoresAccountExtra(t *testing.T) {
	t.Parallel()

	repo := &accountHealthAutoProbeRepo{
		mockAccountRepoForGemini: mockAccountRepoForGemini{
			accountsByID: map[int64]*Account{
				9: {ID: 9, Extra: map[string]any{}},
			},
		},
	}
	svc := &OpsService{accountRepo: repo}

	state, err := svc.UpdateAccountHealthProbeModel(context.Background(), 9, " gpt-5.4-mini ")

	require.NoError(t, err)
	require.Equal(t, int64(9), state.AccountID)
	require.Equal(t, "gpt-5.4-mini", state.ProbeModelID)
	require.Equal(t, "gpt-5.4-mini", state.ProbeModelEffective)
	require.False(t, state.InheritsGlobalModel)
	require.True(t, state.HasAccountOverride)
	require.Equal(t, int64(9), repo.updateID)
	require.Equal(t, "gpt-5.4-mini", repo.updates[accountHealthProbeConfigModelIDKey])
}

func TestResolveAccountHealthProbeModelIDOrder(t *testing.T) {
	t.Parallel()

	repo := &accountHealthAutoProbeRepo{
		mockAccountRepoForGemini: mockAccountRepoForGemini{
			accountsByID: map[int64]*Account{
				9: {ID: 9, Extra: map[string]any{accountHealthProbeConfigModelIDKey: "account-model"}},
			},
		},
	}
	svc := &OpsService{accountRepo: repo}

	require.Equal(t, "manual-model", svc.ResolveAccountHealthProbeModelID(context.Background(), 9, "manual-model"))
	require.Equal(t, "account-model", svc.ResolveAccountHealthProbeModelID(context.Background(), 9, ""))
	require.Equal(t, opsAccountHealthDefaultProbeModel, (&OpsService{}).ResolveAccountHealthProbeModelID(context.Background(), 0, ""))
}

func TestAccountHealthProbePersistContextIgnoresParentCancellation(t *testing.T) {
	t.Parallel()

	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	persistCtx, cancelPersist := accountHealthProbePersistContext(parent)
	defer cancelPersist()

	require.NoError(t, persistCtx.Err())
	deadline, ok := persistCtx.Deadline()
	require.True(t, ok)
	require.Greater(t, time.Until(deadline), time.Duration(0))
}

func TestUpdateOpsAlertRuntimeSettingsPreservesAccountHealth(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newMockSettingRepo()
	svc := &OpsService{settingRepo: repo}

	existing := defaultOpsAlertRuntimeSettings()
	existing.EvaluationIntervalSeconds = 60
	existing.AccountHealth.Mode = "all"
	existing.AccountHealth.RateLimitPerHour = 7
	existing.AccountHealth.Notification.EnterpriseWeChatEnabled = true
	existing.AccountHealth.Notification.EnterpriseWeChatWebhookURL = "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=secret"
	seedOpsAlertRuntimeSettings(t, repo, existing)

	next := *existing
	next.EvaluationIntervalSeconds = 120
	next.AccountHealth = defaultOpsAccountHealthSettings()
	next.AccountHealth.Mode = "opened_only"
	next.AccountHealth.RateLimitPerHour = 99
	next.AccountHealth.Notification.EnterpriseWeChatWebhookURL = "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=stale"

	updated, err := svc.UpdateOpsAlertRuntimeSettings(ctx, &next)
	require.NoError(t, err)
	require.Equal(t, 120, updated.EvaluationIntervalSeconds)
	require.Equal(t, "all", updated.AccountHealth.Mode)
	require.Equal(t, 7, updated.AccountHealth.RateLimitPerHour)
	require.Equal(t, "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=secret", updated.AccountHealth.Notification.EnterpriseWeChatWebhookURL)
}

func TestUpdateOpsAccountHealthSettingsPreservesRuntimeAndMaskedWebhook(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newMockSettingRepo()
	svc := &OpsService{settingRepo: repo}

	existing := defaultOpsAlertRuntimeSettings()
	existing.EvaluationIntervalSeconds = 300
	existing.AccountHealth.Notification.EnterpriseWeChatEnabled = true
	existing.AccountHealth.Notification.EnterpriseWeChatWebhookURL = "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=secret"
	seedOpsAlertRuntimeSettings(t, repo, existing)

	next := defaultOpsAccountHealthSettings()
	next.Mode = "opened_only"
	next.RateLimitPerHour = 5
	next.Notification.EnterpriseWeChatEnabled = true
	next.Notification.EnterpriseWeChatWebhookURL = opsEnterpriseWeChatWebhookMask

	updated, err := svc.UpdateOpsAccountHealthSettings(ctx, next)
	require.NoError(t, err)
	require.Equal(t, "opened_only", updated.Mode)
	require.Equal(t, 5, updated.RateLimitPerHour)
	require.Equal(t, "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=secret", updated.Notification.EnterpriseWeChatWebhookURL)

	stored, err := svc.GetOpsAlertRuntimeSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, 300, stored.EvaluationIntervalSeconds)
	require.Equal(t, "opened_only", stored.AccountHealth.Mode)
	require.Equal(t, "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=secret", stored.AccountHealth.Notification.EnterpriseWeChatWebhookURL)

	cleared := stored.AccountHealth
	cleared.Notification.EnterpriseWeChatEnabled = false
	cleared.Notification.EnterpriseWeChatWebhookURL = ""
	_, err = svc.UpdateOpsAccountHealthSettings(ctx, cleared)
	require.NoError(t, err)

	stored, err = svc.GetOpsAlertRuntimeSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, 300, stored.EvaluationIntervalSeconds)
	require.Empty(t, stored.AccountHealth.Notification.EnterpriseWeChatWebhookURL)
}

func seedOpsAlertRuntimeSettings(t *testing.T, repo *mockSettingRepo, cfg *OpsAlertRuntimeSettings) {
	t.Helper()
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, repo.Set(context.Background(), SettingKeyOpsAlertRuntimeSettings, string(raw)))
}

func accountHealthTestIntPtr(v int) *int {
	return &v
}

func accountHealthTestWindow(window string, requestCount, successCount, errorCount, upstreamErrorCount int64) *OpsAccountHealthWindowStats {
	return &OpsAccountHealthWindowStats{
		Window:             window,
		RequestCount:       requestCount,
		SuccessCount:       successCount,
		ErrorCount:         errorCount,
		UpstreamErrorCount: upstreamErrorCount,
	}
}

type accountHealthAutoProbeRepo struct {
	mockAccountRepoForGemini
	updateID int64
	updates  map[string]any
}

func (r *accountHealthAutoProbeRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	r.updateID = id
	r.updates = updates
	return nil
}
