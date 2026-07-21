//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var _ OpsRepository = (*stubOpsRepo)(nil)

type stubOpsRepo struct {
	OpsRepository
	overview *OpsDashboardOverview
	err      error
}

func (s *stubOpsRepo) GetDashboardOverview(ctx context.Context, filter *OpsDashboardFilter) (*OpsDashboardOverview, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.overview != nil {
		return s.overview, nil
	}
	return &OpsDashboardOverview{}, nil
}

func (s *stubOpsRepo) GetAccountHealthMetrics(ctx context.Context, filter *OpsAccountHealthFilter) (map[int64]*OpsAccountHealthMetrics, error) {
	return map[int64]*OpsAccountHealthMetrics{}, nil
}

func TestComputeGroupAvailableRatio(t *testing.T) {
	t.Parallel()

	t.Run("正常情况: 10个账号, 8个可用 = 80%", func(t *testing.T) {
		t.Parallel()

		got := computeGroupAvailableRatio(&GroupAvailability{
			TotalAccounts:  10,
			AvailableCount: 8,
		})
		require.InDelta(t, 80.0, got, 0.0001)
	})

	t.Run("边界情况: TotalAccounts = 0 应返回 0", func(t *testing.T) {
		t.Parallel()

		got := computeGroupAvailableRatio(&GroupAvailability{
			TotalAccounts:  0,
			AvailableCount: 8,
		})
		require.Equal(t, 0.0, got)
	})

	t.Run("边界情况: AvailableCount = 0 应返回 0%", func(t *testing.T) {
		t.Parallel()

		got := computeGroupAvailableRatio(&GroupAvailability{
			TotalAccounts:  10,
			AvailableCount: 0,
		})
		require.Equal(t, 0.0, got)
	})
}

func TestCountAccountsByCondition(t *testing.T) {
	t.Parallel()

	t.Run("测试限流账号统计: acc.IsRateLimited", func(t *testing.T) {
		t.Parallel()

		accounts := map[int64]*AccountAvailability{
			1: {IsRateLimited: true},
			2: {IsRateLimited: false},
			3: {IsRateLimited: true},
		}

		got := countAccountsByCondition(accounts, func(acc *AccountAvailability) bool {
			return acc.IsRateLimited
		})
		require.Equal(t, int64(2), got)
	})

	t.Run("测试错误账号统计（排除临时不可调度）: acc.HasError && acc.TempUnschedulableUntil == nil", func(t *testing.T) {
		t.Parallel()

		until := time.Now().UTC().Add(5 * time.Minute)
		accounts := map[int64]*AccountAvailability{
			1: {HasError: true},
			2: {HasError: true, TempUnschedulableUntil: &until},
			3: {HasError: false},
		}

		got := countAccountsByCondition(accounts, func(acc *AccountAvailability) bool {
			return acc.HasError && acc.TempUnschedulableUntil == nil
		})
		require.Equal(t, int64(1), got)
	})

	t.Run("边界情况: 空 map 应返回 0", func(t *testing.T) {
		t.Parallel()

		got := countAccountsByCondition(map[int64]*AccountAvailability{}, func(acc *AccountAvailability) bool {
			return acc.IsRateLimited
		})
		require.Equal(t, int64(0), got)
	})
}

// TestComputeRuleMetric_AccountTempUnscheduledCount verifies the new
// account_temp_unscheduled_count metric counts accounts currently in the
// temp-unscheduled window and ignores those whose window has expired or
// were never temp-unscheduled.
func TestComputeRuleMetric_AccountTempUnscheduledCount(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	futureUntil := now.Add(5 * time.Minute)
	pastUntil := now.Add(-1 * time.Minute)

	availability := &OpsAccountAvailability{
		Accounts: map[int64]*AccountAvailability{
			// currently temp-unscheduled (window active)
			1: {TempUnschedulableUntil: &futureUntil},
			2: {TempUnschedulableUntil: &futureUntil},
			// temp-unsched window already expired → should NOT count
			3: {TempUnschedulableUntil: &pastUntil},
			// never temp-unscheduled
			4: {HasError: true},
			5: {IsRateLimited: true},
		},
	}

	opsService := &OpsService{
		getAccountAvailability: func(_ context.Context, _ string, _ *int64) (*OpsAccountAvailability, error) {
			return availability, nil
		},
	}
	svc := &OpsAlertEvaluatorService{
		opsService: opsService,
		opsRepo:    &stubOpsRepo{},
	}

	rule := &OpsAlertRule{MetricType: "account_temp_unscheduled_count"}
	val, ok := svc.computeRuleMetric(context.Background(), rule, nil,
		now.Add(-5*time.Minute), now, "", nil)

	require.True(t, ok)
	require.InDelta(t, 2.0, val, 0.0001, "only 2 accounts have an active temp-unsched window")
}

func TestComputeRuleMetricNewIndicators(t *testing.T) {
	t.Parallel()

	groupID := int64(101)
	platform := "openai"

	availability := &OpsAccountAvailability{
		Group: &GroupAvailability{
			GroupID:        groupID,
			TotalAccounts:  10,
			AvailableCount: 8,
		},
		Accounts: map[int64]*AccountAvailability{
			1: {IsRateLimited: true},
			2: {IsRateLimited: true},
			3: {HasError: true},
			4: {HasError: true, TempUnschedulableUntil: timePtr(time.Now().UTC().Add(2 * time.Minute))},
			5: {HasError: false, IsRateLimited: false},
		},
	}

	opsService := &OpsService{
		getAccountAvailability: func(_ context.Context, _ string, _ *int64) (*OpsAccountAvailability, error) {
			return availability, nil
		},
	}

	svc := &OpsAlertEvaluatorService{
		opsService: opsService,
		opsRepo:    &stubOpsRepo{overview: &OpsDashboardOverview{}},
	}

	start := time.Now().UTC().Add(-5 * time.Minute)
	end := time.Now().UTC()
	ctx := context.Background()

	tests := []struct {
		name       string
		metricType string
		groupID    *int64
		wantValue  float64
		wantOK     bool
	}{
		{
			name:       "group_available_accounts",
			metricType: "group_available_accounts",
			groupID:    &groupID,
			wantValue:  8,
			wantOK:     true,
		},
		{
			name:       "group_available_ratio",
			metricType: "group_available_ratio",
			groupID:    &groupID,
			wantValue:  80.0,
			wantOK:     true,
		},
		{
			name:       "account_rate_limited_count",
			metricType: "account_rate_limited_count",
			groupID:    nil,
			wantValue:  2,
			wantOK:     true,
		},
		{
			name:       "account_error_count",
			metricType: "account_error_count",
			groupID:    nil,
			wantValue:  1,
			wantOK:     true,
		},
		{
			name:       "group_available_accounts without group_id returns false",
			metricType: "group_available_accounts",
			groupID:    nil,
			wantValue:  0,
			wantOK:     false,
		},
		{
			name:       "group_available_ratio without group_id returns false",
			metricType: "group_available_ratio",
			groupID:    nil,
			wantValue:  0,
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rule := &OpsAlertRule{
				MetricType: tt.metricType,
			}
			gotValue, gotOK := svc.computeRuleMetric(ctx, rule, nil, start, end, platform, tt.groupID)
			require.Equal(t, tt.wantOK, gotOK)
			if !tt.wantOK {
				return
			}
			require.InDelta(t, tt.wantValue, gotValue, 0.0001)
		})
	}
}

func TestScheduleAccountHealthRecoveryProbesDoesNotBlock(t *testing.T) {
	t.Parallel()

	settings := defaultOpsAccountHealthSettings()
	settings.Probe.Enabled = true
	settings.Probe.IntervalMinutes = 30
	settings.Probe.MaxPerRun = 2
	settings.Probe.TimeoutSeconds = 1
	settings.Recovery.Enabled = true

	svc := &OpsAlertEvaluatorService{
		accountTestService:   &AccountTestService{},
		accountHealthProbeAt: map[int64]time.Time{},
	}
	items := []*OpsAccountHealthItem{
		{AccountID: 1, IsOpened: false, Recommendation: OpsAccountHealthRecommendation{Action: OpsAccountHealthActionNeedsProbe}},
		{AccountID: 2, IsOpened: false, Recommendation: OpsAccountHealthRecommendation{Action: OpsAccountHealthActionKeepClosed}},
		{AccountID: 3, IsOpened: false, Recommendation: OpsAccountHealthRecommendation{Action: OpsAccountHealthActionUnavailable}},
	}

	startedAt := time.Now()
	scheduled := svc.scheduleAccountHealthRecoveryProbes(context.Background(), items, settings)

	require.Equal(t, 2, scheduled)
	require.Less(t, time.Since(startedAt), 500*time.Millisecond)
	require.Len(t, svc.accountHealthProbeAt, 2)
}

func TestShouldProbeAccountHealthRecoveryContinuesAfterCanOpen(t *testing.T) {
	t.Parallel()

	settings := defaultOpsAccountHealthSettings()
	checkedAt := time.Now().UTC().Add(-2 * time.Minute)
	item := &OpsAccountHealthItem{
		AccountID: 1,
		IsOpened:  false,
		Probe: &OpsAccountHealthProbe{
			Status:    "success",
			CheckedAt: &checkedAt,
		},
		Recommendation: OpsAccountHealthRecommendation{
			Action:        OpsAccountHealthActionCanOpen,
			RecoveryReady: true,
		},
	}

	require.True(t, shouldProbeAccountHealthRecovery(item, settings))
}

func TestShouldProbeAccountHealthRecoverySkipsAccountDisabledAutoProbe(t *testing.T) {
	t.Parallel()

	settings := defaultOpsAccountHealthSettings()
	item := &OpsAccountHealthItem{
		AccountID:         1,
		IsOpened:          false,
		ProbeAutoDisabled: true,
		Recommendation:    OpsAccountHealthRecommendation{Action: OpsAccountHealthActionNeedsProbe},
	}

	require.False(t, shouldProbeAccountHealthRecovery(item, settings))
}

func TestShouldProbeAccountHealthRecoveryDoesNotRequireRecoveryEnabled(t *testing.T) {
	t.Parallel()

	settings := defaultOpsAccountHealthSettings()
	settings.Probe.Enabled = true
	settings.Recovery.Enabled = false
	item := &OpsAccountHealthItem{
		AccountID: 1,
		IsOpened:  false,
	}
	require.True(t, shouldProbeAccountHealthRecovery(item, settings))
}

func TestShouldProbeAccountHealthRecoveryOpenedUnavailable(t *testing.T) {
	t.Parallel()

	settings := defaultOpsAccountHealthSettings()
	settings.Probe.Enabled = true
	item := &OpsAccountHealthItem{
		AccountID:   2,
		IsOpened:    true,
		IsAvailable: false,
	}
	require.True(t, shouldProbeAccountHealthRecovery(item, settings))
	item.IsAvailable = true
	require.False(t, shouldProbeAccountHealthRecovery(item, settings))
}

func TestScheduleAccountHealthRecoveryProbesWorksWhenRecoveryDisabled(t *testing.T) {
	t.Parallel()

	settings := defaultOpsAccountHealthSettings()
	settings.Probe.Enabled = true
	settings.Probe.MaxPerRun = 3
	settings.Probe.TimeoutSeconds = 1
	settings.Recovery.Enabled = false

	svc := &OpsAlertEvaluatorService{
		accountTestService:   &AccountTestService{},
		accountHealthProbeAt: map[int64]time.Time{},
	}
	items := []*OpsAccountHealthItem{
		{AccountID: 1, IsOpened: false},
		{AccountID: 2, IsOpened: false},
	}
	scheduled := svc.scheduleAccountHealthRecoveryProbes(context.Background(), items, settings)
	require.Equal(t, 2, scheduled)
}

func TestBuildOpsAccountHealthDigestWeComTextIncludesSustainedWindows(t *testing.T) {
	t.Parallel()

	item := &OpsAccountHealthItem{
		AccountID:   7,
		AccountName: "codex plus pro",
		Windows:     defaultOpsAccountHealthWindows(),
		Recommendation: OpsAccountHealthRecommendation{
			Action:   OpsAccountHealthActionCloseNow,
			Severity: "P2",
			Title:    "账号持续变差，建议处理",
			Reason:   "5m/10m/30m all degraded",
		},
	}
	item.Windows[OpsAccountHealthWindow5m] = accountHealthTestWindow(OpsAccountHealthWindow5m, 24, 18, 6, 4)
	item.Windows[OpsAccountHealthWindow10m] = accountHealthTestWindow(OpsAccountHealthWindow10m, 50, 40, 10, 8)
	item.Windows[OpsAccountHealthWindow30m] = accountHealthTestWindow(OpsAccountHealthWindow30m, 120, 102, 18, 14)
	normalizeAccountHealthMetrics(&OpsAccountHealthMetrics{Windows: item.Windows})

	text := buildOpsAccountHealthDigestWeComText([]*OpsAccountHealthItem{item}, time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC))

	require.Contains(t, text, "账号健康汇总")
	require.Contains(t, text, "codex plus pro")
	require.Contains(t, text, "建议关闭")
	require.Contains(t, text, "5m")
	require.Contains(t, text, "10m")
	require.Contains(t, text, "30m")
}

func TestBuildOpsAccountHealthWeComTextUsesMarkdownSections(t *testing.T) {
	t.Parallel()

	item := &OpsAccountHealthItem{
		AccountID:     7,
		AccountName:   "codex plus pro",
		Platform:      "openai",
		GroupID:       3,
		GroupName:     "plus pro分组",
		IsOpened:      true,
		IsSchedulable: true,
		IsAvailable:   false,
		HasError:      true,
		Windows:       defaultOpsAccountHealthWindows(),
		Recommendation: OpsAccountHealthRecommendation{
			Action:   OpsAccountHealthActionCloseNow,
			Severity: "P1",
			Title:    "账号处于错误状态",
			Reason:   "upstream returned invalid_grant",
		},
	}
	item.Windows[OpsAccountHealthWindow1m] = accountHealthTestWindow(OpsAccountHealthWindow1m, 24, 14, 10, 7)
	normalizeAccountHealthMetrics(&OpsAccountHealthMetrics{Windows: item.Windows})

	text := buildOpsAccountHealthWeComText(item, time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC))

	require.Contains(t, text, "## <font color=\"warning\">[P1] 账号处于错误状态</font>")
	require.Contains(t, text, "**建议动作**")
	require.Contains(t, text, "**状态概览**")
	require.Contains(t, text, "**窗口指标**")
	require.Contains(t, text, "codex plus pro")
	require.Contains(t, text, "1m：请求 **24**")
	require.NotContains(t, text, "打开=true")
}

func TestBuildOpsAccountHealthWeComTextUsesShanghaiTime(t *testing.T) {
	t.Parallel()

	item := &OpsAccountHealthItem{
		AccountID:   7,
		AccountName: "codex plus pro",
		Windows:     defaultOpsAccountHealthWindows(),
		Recommendation: OpsAccountHealthRecommendation{
			Action:   OpsAccountHealthActionCloseNow,
			Severity: "P1",
			Title:    "account unhealthy",
		},
	}

	text := buildOpsAccountHealthWeComText(item, time.Date(2026, 6, 28, 1, 2, 3, 0, time.UTC))

	require.Contains(t, text, "2026-06-28 09:02:03 Asia/Shanghai")
	require.NotContains(t, text, "UTC")
}

func TestBuildOpsAlertEventWeComTextUsesShanghaiTime(t *testing.T) {
	t.Parallel()

	text := buildOpsAlertEventWeComText(nil, &OpsAlertEvent{
		Severity: "P1",
		Title:    "high error rate",
	}, time.Date(2026, 6, 28, 1, 2, 3, 0, time.UTC))

	require.Contains(t, text, "2026-06-28 09:02:03 Asia/Shanghai")
	require.NotContains(t, text, "UTC")
	require.NotContains(t, text, "2026-06-28T01:02:03Z")
}

func TestShouldMentionAllForOpsAlertEnterpriseWeChat(t *testing.T) {
	t.Parallel()

	notification := OpsAccountHealthNotificationSettings{
		MentionAllOnImmediate: true,
	}

	require.True(t, shouldMentionAllForOpsAlertEnterpriseWeChat(notification, &OpsAlertEvent{Severity: "P0"}))
	require.True(t, shouldMentionAllForOpsAlertEnterpriseWeChat(notification, &OpsAlertEvent{Severity: "critical"}))
	require.False(t, shouldMentionAllForOpsAlertEnterpriseWeChat(notification, &OpsAlertEvent{Severity: "P1"}))
	require.False(t, shouldMentionAllForOpsAlertEnterpriseWeChat(OpsAccountHealthNotificationSettings{}, &OpsAlertEvent{Severity: "P0"}))
}

func TestSendOpsEnterpriseWeChatTextUsesShortTimeout(t *testing.T) {
	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	var sawDeadline bool
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		deadline, ok := req.Context().Deadline()
		require.True(t, ok, "enterprise wechat request should carry a short deadline")
		sawDeadline = true
		require.LessOrEqual(t, time.Until(deadline), opsEnterpriseWeChatSendTimeout+time.Second)
		require.Greater(t, time.Until(deadline), time.Duration(0))
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"errcode":0,"errmsg":"ok"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})

	err := sendOpsEnterpriseWeChatText(
		context.Background(),
		"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test",
		"hello",
		false,
	)
	require.NoError(t, err)
	require.True(t, sawDeadline)
}

func TestSendOpsEnterpriseWeChatMarkdownPayload(t *testing.T) {
	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	var sawMarkdown bool
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		payload := string(body)
		require.Contains(t, payload, `"msgtype":"markdown"`)
		require.Contains(t, payload, `"markdown"`)
		require.Contains(t, payload, `hello`)
		require.Contains(t, payload, `\u003c@all\u003e`)
		sawMarkdown = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"errcode":0,"errmsg":"ok"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})

	err := sendOpsEnterpriseWeChatMarkdown(
		context.Background(),
		"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test",
		"## hello",
		true,
	)
	require.NoError(t, err)
	require.True(t, sawMarkdown)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
