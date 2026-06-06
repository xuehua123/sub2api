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
	require.Contains(t, rec.Title, "成功率低于阈值")
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
