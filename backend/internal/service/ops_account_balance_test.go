//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

type accountBalanceExtraRepoStub struct {
	AccountRepository
	updates map[string]any
}

func (r *accountBalanceExtraRepoStub) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updates = make(map[string]any, len(updates))
	for key, value := range updates {
		r.updates[key] = value
	}
	return nil
}

func TestAccountBalanceJoinEndpoint(t *testing.T) {
	tests := []struct {
		name         string
		baseURL      string
		endpointPath string
		keepV1       bool
		want         string
	}{
		{
			name:         "newapi token usage strips openai v1 path",
			baseURL:      "https://example.com/v1",
			endpointPath: "/api/usage/token",
			keepV1:       false,
			want:         "https://example.com/api/usage/token",
		},
		{
			name:         "sub2api usage keeps single v1 path",
			baseURL:      "https://example.com/v1",
			endpointPath: "/v1/usage",
			keepV1:       true,
			want:         "https://example.com/v1/usage",
		},
		{
			name:         "plain host gets requested path",
			baseURL:      "https://example.com/",
			endpointPath: "/v1/usage",
			keepV1:       true,
			want:         "https://example.com/v1/usage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := accountBalanceJoinEndpoint(tt.baseURL, tt.endpointPath, tt.keepV1)
			if got != tt.want {
				t.Fatalf("accountBalanceJoinEndpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseNewAPIAccountBalance(t *testing.T) {
	result, err := parseNewAPIAccountBalance("https://example.com/api/usage/token", map[string]any{
		"data": map[string]any{
			"total_available": 500000.0,
			"total_granted":   1000000.0,
			"total_used":      250000.0,
		},
	})
	if err != nil {
		t.Fatalf("parseNewAPIAccountBalance() error = %v", err)
	}
	assertFloatPtr(t, "balance", result.BalanceUSD, 1)
	assertFloatPtr(t, "granted", result.TotalGrantedUSD, 2)
	assertFloatPtr(t, "used", result.TotalUsedUSD, 0.5)
}

func TestParseSub2APIAccountBalance(t *testing.T) {
	result, err := parseSub2APIAccountBalance("https://example.com/v1/usage", map[string]any{
		"balance": "12.34",
		"used":    4.56,
		"quota":   16.9,
	})
	if err != nil {
		t.Fatalf("parseSub2APIAccountBalance() error = %v", err)
	}
	assertFloatPtr(t, "balance", result.BalanceUSD, 12.34)
	assertFloatPtr(t, "granted", result.TotalGrantedUSD, 16.9)
	assertFloatPtr(t, "used", result.TotalUsedUSD, 4.56)
}

func TestParseOpenAIBillingAccountBalance(t *testing.T) {
	result, err := parseOpenAIBillingAccountBalance("https://example.com/v1/dashboard/billing/subscription", map[string]any{
		"hard_limit_usd": 20.0,
		"used_usd":       7.25,
	})
	if err != nil {
		t.Fatalf("parseOpenAIBillingAccountBalance() error = %v", err)
	}
	assertFloatPtr(t, "balance", result.BalanceUSD, 12.75)
	assertFloatPtr(t, "granted", result.TotalGrantedUSD, 20)
	assertFloatPtr(t, "used", result.TotalUsedUSD, 7.25)
}

func TestOpenAIBillingProbeUsesCurrentPeriodUsageWhenSubscriptionOnlyHasLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/dashboard/billing/subscription":
			_, _ = w.Write([]byte(`{"hard_limit_usd":8850.002998}`))
		case "/v1/dashboard/billing/usage":
			if r.URL.Query().Get("start_date") == "" || r.URL.Query().Get("end_date") == "" {
				t.Fatalf("usage date range = %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"total_usage":858831.892}`))
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	service := &OpsService{}
	account := &Account{Credentials: map[string]any{
		"base_url": server.URL + "/v1",
		"api_key":  "test-key",
	}}
	result, attempt, err := service.tryProbeAccountBalanceMethod(context.Background(), account, AccountBalanceProbeMethodOpenAIBilling, time.Second)
	if err != nil {
		t.Fatalf("tryProbeAccountBalanceMethod() error = %v", err)
	}
	if attempt.Status != AccountBalanceProbeStatusOK {
		t.Fatalf("attempt status = %q, want ok", attempt.Status)
	}
	assertFloatPtr(t, "used", result.TotalUsedUSD, 8588.31892)
	assertFloatPtr(t, "balance", result.BalanceUSD, 261.684078)
	assertFloatPtr(t, "granted", result.TotalGrantedUSD, 8850.002998)
}

func TestAccountBalanceProbeMethodsForAccount(t *testing.T) {
	got := accountBalanceProbeMethodsForAccount(
		OpsAccountBalanceState{
			Method:         AccountBalanceProbeMethodAuto,
			DetectedMethod: AccountBalanceProbeMethodSub2APIUsage,
		},
		OpsAccountBalanceSettings{
			Probe: OpsAccountBalanceProbeSettings{
				MethodOrder: []string{
					AccountBalanceProbeMethodUpstreamManagement,
					AccountBalanceProbeMethodSub2APIUsage,
					AccountBalanceProbeMethodOpenAIBilling,
				},
			},
		},
		"",
	)
	want := []string{
		AccountBalanceProbeMethodSub2APIUsage,
		AccountBalanceProbeMethodUpstreamManagement,
		AccountBalanceProbeMethodOpenAIBilling,
	}
	if len(got) != len(want) {
		t.Fatalf("method count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("method[%d] = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}

func TestNormalizeOpsAccountBalanceSettingsMigratesRetiredNewAPITokenUsage(t *testing.T) {
	settings := defaultOpsAccountBalanceSettings()
	settings.Probe.MethodOrder = []string{
		AccountBalanceProbeMethodNewAPITokenUsage,
		AccountBalanceProbeMethodSub2APIUsage,
		AccountBalanceProbeMethodOpenAIBilling,
	}

	normalizeOpsAccountBalanceSettings(&settings)
	want := []string{
		AccountBalanceProbeMethodSub2APIUsage,
		AccountBalanceProbeMethodUpstreamManagement,
		AccountBalanceProbeMethodOpenAIBilling,
	}
	if !reflect.DeepEqual(settings.Probe.MethodOrder, want) {
		t.Fatalf("method order = %#v, want %#v", settings.Probe.MethodOrder, want)
	}
}

func TestAccountBalanceStateTreatsNullThresholdAsDefault(t *testing.T) {
	state := AccountBalanceStateFromAccount(&Account{
		ID: 1,
		Extra: map[string]any{
			accountBalanceProbeThresholdUSDExtraKey: nil,
		},
	})
	if state.ThresholdUSD != nil {
		t.Fatalf("ThresholdUSD = %#v, want nil", state.ThresholdUSD)
	}
}

func TestPersistAccountBalanceFailureClearsStaleProbeValues(t *testing.T) {
	balance := -52.55
	used := 109.3
	granted := 100000000.0
	repo := &accountBalanceExtraRepoStub{}
	service := &OpsService{accountRepo: repo}
	account := &Account{
		ID: 1,
		Extra: map[string]any{
			accountBalanceProbeDetectedMethodExtraKey: "openai_billing",
			accountBalanceProbeUnlimitedExtraKey:      true,
			accountBalanceProbeEndpointExtraKey:       "https://example.com/dashboard/billing/subscription",
			accountBalanceProbeBalanceUSDExtraKey:     balance,
			accountBalanceProbeTotalUsedUSDExtraKey:   used,
			accountBalanceProbeGrantedUSDExtraKey:     granted,
		},
	}

	state := service.persistAccountBalanceFailure(context.Background(), account, "upstream returned 401", nil)
	if state.Status != AccountBalanceProbeStatusFailed {
		t.Fatalf("status = %q, want %q", state.Status, AccountBalanceProbeStatusFailed)
	}
	if state.Unlimited || state.BalanceUSD != nil || state.TotalUsedUSD != nil || state.TotalGrantedUSD != nil || state.DetectedMethod != "" || state.Endpoint != "" {
		t.Fatalf("stale balance state remained after failure: %#v", state)
	}
	for _, key := range []string{
		accountBalanceProbeBalanceUSDExtraKey,
		accountBalanceProbeBalanceAmountExtraKey,
		accountBalanceProbeBalanceCurrencyExtraKey,
		accountBalanceProbeTotalUsedUSDExtraKey,
		accountBalanceProbeGrantedUSDExtraKey,
	} {
		if value, ok := repo.updates[key]; !ok || value != nil {
			t.Fatalf("persisted %s = %#v, want nil", key, value)
		}
	}
}

func TestUnlimitedAccountWithNumericBalanceIsNotLow(t *testing.T) {
	balance := -0.12
	state := OpsAccountBalanceState{
		Enabled:    true,
		Unlimited:  true,
		BalanceUSD: &balance,
	}
	if accountBalanceStateIsLow(state, defaultOpsAccountBalanceSettings()) {
		t.Fatal("unlimited account must not be treated as low balance")
	}
}

func TestParseNewAPIUnlimitedQuotaRejectsKeyLedgerAsAccountBalance(t *testing.T) {
	_, err := parseNewAPIAccountBalance("https://example.com/api/usage/token", map[string]any{
		"data": map[string]any{
			"unlimited_quota": true,
			"total_available": -612535910.0,
			"total_used":      612535910.0,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "API key quota") {
		t.Fatalf("parseNewAPIAccountBalance() error = %v, want API key quota rejection", err)
	}
}

func TestAccountBalanceStateInvalidatesHistoricalNewAPIKeySnapshot(t *testing.T) {
	for _, extra := range []map[string]any{
		{
			accountBalanceProbeDetectedMethodExtraKey: AccountBalanceProbeMethodNewAPITokenUsage,
			accountBalanceProbeBalanceUSDExtraKey:     -42.0,
		},
		{
			accountBalanceProbeMethodExtraKey:     AccountBalanceProbeMethodNewAPITokenUsage,
			accountBalanceProbeBalanceUSDExtraKey: -42.0,
		},
	} {
		state := AccountBalanceStateFromAccount(&Account{ID: 1, Extra: extra})
		if state.BalanceUSD != nil {
			t.Fatalf("BalanceUSD = %v, want nil for API key snapshot", *state.BalanceUSD)
		}
		if state.Status != AccountBalanceProbeStatusUnsupported || !strings.Contains(state.Error, "API key usage") {
			t.Fatalf("state = %#v, want unsupported API key snapshot", state)
		}
	}
}

func TestPersistAccountBalanceSuccessStoresNativeCurrencyAmount(t *testing.T) {
	staleBalance := -42.0
	repo := &accountBalanceExtraRepoStub{}
	service := &OpsService{accountRepo: repo}
	account := &Account{
		ID: 1,
		Extra: map[string]any{
			accountBalanceProbeBalanceUSDExtraKey: staleBalance,
		},
	}

	state := service.persistAccountBalanceSuccess(context.Background(), account, opsAccountBalanceMethodResult{
		Method:          AccountBalanceProbeMethodUpstreamManagement,
		Endpoint:        "https://example.com/api/user/self",
		BalanceCurrency: "CNY",
		BalanceAmount:   accountBalanceFloat64Ptr(12.5),
	})
	if state.BalanceUSD != nil || state.BalanceAmount == nil || *state.BalanceAmount != 12.5 || state.BalanceCurrency != "CNY" {
		t.Fatalf("state = %#v, want native CNY amount", state)
	}
	for _, key := range []string{
		accountBalanceProbeBalanceUSDExtraKey,
	} {
		if value, ok := repo.updates[key]; !ok || value != nil {
			t.Fatalf("persisted %s = %#v, want nil", key, value)
		}
	}
	if got := repo.updates[accountBalanceProbeBalanceAmountExtraKey]; got != 12.5 {
		t.Fatalf("native amount = %#v, want 12.5", got)
	}
	if got := repo.updates[accountBalanceProbeBalanceCurrencyExtraKey]; got != "CNY" {
		t.Fatalf("native currency = %#v, want CNY", got)
	}
}

func TestSortAccountBalanceMonitorItems(t *testing.T) {
	older := time.Date(2026, 6, 20, 1, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	low := 3.0
	high := 25.0
	items := []OpsAccountBalanceAccountItem{
		{
			AccountID:   2,
			AccountName: "beta",
			Platform:    "openai",
			BalanceProbe: OpsAccountBalanceState{
				Status:     AccountBalanceProbeStatusOK,
				BalanceUSD: &low,
				CheckedAt:  &older,
			},
		},
		{
			AccountID:   1,
			AccountName: "alpha",
			Platform:    "anthropic",
			BalanceProbe: OpsAccountBalanceState{
				Status:     AccountBalanceProbeStatusFailed,
				BalanceUSD: &high,
				CheckedAt:  &newer,
			},
		},
		{
			AccountID:   3,
			AccountName: "gamma",
			Platform:    "openai",
			BalanceProbe: OpsAccountBalanceState{
				Status:    AccountBalanceProbeStatusUnknown,
				Unlimited: true,
			},
		},
	}

	sortAccountBalanceMonitorItems(items, "balance_usd", "desc")
	if got := accountBalanceIDs(items); !reflect.DeepEqual(got, []int64{3, 1, 2}) {
		t.Fatalf("balance desc order = %#v, want [3 1 2]", got)
	}

	sortAccountBalanceMonitorItems(items, "checked_at", "asc")
	if got := accountBalanceIDs(items); !reflect.DeepEqual(got, []int64{2, 1, 3}) {
		t.Fatalf("checked_at asc order = %#v, want [2 1 3]", got)
	}
}

func TestMatchesAccountBalanceFilter(t *testing.T) {
	now := time.Date(2026, 6, 20, 1, 0, 0, 0, time.UTC)
	checkedAt := now.Add(-2 * time.Hour)
	balance := 3.0
	item := OpsAccountBalanceAccountItem{
		AccountID:   7,
		AccountName: "plus sub2api",
		Platform:    "openai",
		Type:        "apikey",
		Status:      "active",
		Schedulable: true,
		BalanceProbe: OpsAccountBalanceState{
			Method:         AccountBalanceProbeMethodAuto,
			Enabled:        true,
			DetectedMethod: AccountBalanceProbeMethodSub2APIUsage,
			Status:         AccountBalanceProbeStatusOK,
			BalanceUSD:     &balance,
			CheckedAt:      &checkedAt,
		},
	}
	settings := defaultOpsAccountBalanceSettings()
	settings.DefaultThresholdUSD = 10

	if !matchesAccountBalanceFilter(item, OpsAccountBalanceMonitorFilter{Method: AccountBalanceProbeMethodSub2APIUsage}, settings, now) {
		t.Fatal("detected method should match method filter")
	}
	if !matchesAccountBalanceFilter(item, OpsAccountBalanceMonitorFilter{ProbeStatus: AccountBalanceProbeStatusOK}, settings, now) {
		t.Fatal("probe status should match status filter")
	}
	if !matchesAccountBalanceFilter(item, OpsAccountBalanceMonitorFilter{OnlyLow: true, OnlyDue: true, OnlySchedulable: true}, settings, now) {
		t.Fatal("low, due, schedulable filters should match item")
	}
	if matchesAccountBalanceFilter(item, OpsAccountBalanceMonitorFilter{OnlyFailed: true}, settings, now) {
		t.Fatal("failed filter should reject healthy item")
	}
}

func TestNormalizeOpsAccountBalanceSettingsMigratesLegacyDefaultInterval(t *testing.T) {
	settings := defaultOpsAccountBalanceSettings()
	settings.Probe.IntervalMinutes = accountBalanceLegacyDefaultIntervalMinutes

	normalizeOpsAccountBalanceSettings(&settings)

	if settings.Probe.IntervalMinutes != 5 {
		t.Fatalf("interval = %d, want 5", settings.Probe.IntervalMinutes)
	}
}

func TestBuildOpsAccountBalanceWeComMarkdownUsesShanghaiTime(t *testing.T) {
	now := time.Date(2026, 6, 28, 1, 2, 3, 0, time.UTC)
	balance := 3.21
	settings := defaultOpsAccountBalanceSettings()
	text := buildOpsAccountBalanceLowWeComMarkdown(Account{
		ID:       7,
		Name:     "codex plus pro",
		Platform: "openai",
		Type:     "apikey",
	}, OpsAccountBalanceState{
		Method:     AccountBalanceProbeMethodAuto,
		Status:     AccountBalanceProbeStatusOK,
		BalanceUSD: &balance,
	}, settings, now)

	if !strings.Contains(text, "2026-06-28 09:02:03 Asia/Shanghai") {
		t.Fatalf("notification time = %q, want Asia/Shanghai time", text)
	}
	if strings.Contains(text, "UTC") {
		t.Fatalf("notification time should not expose UTC: %q", text)
	}
}

func TestBuildOpsAccountBalanceTestWeComMarkdownUsesShanghaiTime(t *testing.T) {
	text := buildOpsAccountBalanceTestWeComMarkdown(time.Date(2026, 6, 28, 1, 2, 3, 0, time.UTC))

	if !strings.Contains(text, "2026-06-28 09:02:03 Asia/Shanghai") {
		t.Fatalf("test notification time = %q, want Asia/Shanghai time", text)
	}
	if strings.Contains(text, "UTC") {
		t.Fatalf("test notification time should not expose UTC: %q", text)
	}
}

func accountBalanceIDs(items []OpsAccountBalanceAccountItem) []int64 {
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.AccountID)
	}
	return ids
}

func assertFloatPtr(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %.4f", name, want)
	}
	if diff := *got - want; diff < -0.0001 || diff > 0.0001 {
		t.Fatalf("%s = %.4f, want %.4f", name, *got, want)
	}
}

func accountBalanceFloat64Ptr(value float64) *float64 { return &value }
