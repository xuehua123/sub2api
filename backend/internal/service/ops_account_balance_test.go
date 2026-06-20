//go:build unit

package service

import (
	"reflect"
	"testing"
	"time"
)

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

func TestAccountBalanceProbeMethodsForAccount(t *testing.T) {
	got := accountBalanceProbeMethodsForAccount(
		OpsAccountBalanceState{
			Method:         AccountBalanceProbeMethodAuto,
			DetectedMethod: AccountBalanceProbeMethodSub2APIUsage,
		},
		OpsAccountBalanceSettings{
			Probe: OpsAccountBalanceProbeSettings{
				MethodOrder: []string{
					AccountBalanceProbeMethodNewAPITokenUsage,
					AccountBalanceProbeMethodSub2APIUsage,
					AccountBalanceProbeMethodOpenAIBilling,
				},
			},
		},
		"",
	)
	want := []string{
		AccountBalanceProbeMethodSub2APIUsage,
		AccountBalanceProbeMethodNewAPITokenUsage,
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
