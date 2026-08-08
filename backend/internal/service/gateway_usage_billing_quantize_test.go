//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

const (
	quantizedBillingEffectBalanceCache        = "balance_cache"
	quantizedBillingEffectSubscriptionCache   = "subscription_cache"
	quantizedBillingEffectRateLimitCache      = "rate_limit_cache"
	quantizedBillingEffectPlatformCache       = "platform_cache"
	quantizedBillingEffectBalanceNotification = "balance_notification"
	quantizedBillingEffectAccountNotification = "account_notification"
)

type quantizedBillingEffect struct {
	kind      string
	amount    float64
	oldAmount float64
}

type quantizedBillingCacheStub struct {
	BillingCache
	effects chan<- quantizedBillingEffect
}

func (s *quantizedBillingCacheStub) DeductUserBalance(_ context.Context, _ int64, amount float64) error {
	s.effects <- quantizedBillingEffect{kind: quantizedBillingEffectBalanceCache, amount: amount}
	return nil
}

func (s *quantizedBillingCacheStub) UpdateSubscriptionUsage(_ context.Context, _, _ int64, amount float64) error {
	s.effects <- quantizedBillingEffect{kind: quantizedBillingEffectSubscriptionCache, amount: amount}
	return nil
}

func (s *quantizedBillingCacheStub) UpdateAPIKeyRateLimitUsage(_ context.Context, _ int64, amount float64) error {
	s.effects <- quantizedBillingEffect{kind: quantizedBillingEffectRateLimitCache, amount: amount}
	return nil
}

func (s *quantizedBillingCacheStub) GetUserPlatformQuotaCache(context.Context, int64, string) (*UserPlatformQuotaCacheEntry, bool, error) {
	return nil, false, nil
}

func (s *quantizedBillingCacheStub) IncrUserPlatformQuotaUsageCache(_ context.Context, _ int64, _ string, amount float64, _ time.Duration, _ bool) error {
	s.effects <- quantizedBillingEffect{kind: quantizedBillingEffectPlatformCache, amount: amount}
	return nil
}

type quantizedBillingNotifierStub struct {
	effects chan<- quantizedBillingEffect
}

func (s *quantizedBillingNotifierStub) CheckBalanceAfterDeduction(_ context.Context, _ *User, oldBalance, cost float64) {
	s.effects <- quantizedBillingEffect{kind: quantizedBillingEffectBalanceNotification, amount: cost, oldAmount: oldBalance}
}

func (s *quantizedBillingNotifierStub) CheckAccountQuotaAfterIncrement(_ context.Context, _ *Account, cost float64, _ *AccountQuotaState) {
	s.effects <- quantizedBillingEffect{kind: quantizedBillingEffectAccountNotification, amount: cost}
}

type quantizedBillingRepoStub struct {
	UsageBillingRepository

	result *UsageBillingApplyResult
	err    error
	cmd    *UsageBillingCommand
}

func (s *quantizedBillingRepoStub) Apply(_ context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	s.cmd = cmd
	return s.result, s.err
}

type quantizedBillingAPIKeyUpdaterStub struct{}

func (*quantizedBillingAPIKeyUpdaterStub) UpdateQuotaUsed(context.Context, int64, float64) error {
	return nil
}

func (*quantizedBillingAPIKeyUpdaterStub) UpdateRateLimitUsage(context.Context, int64, float64) error {
	return nil
}

type quantizedBillingPlatformRepoStub struct {
	UserPlatformQuotaRepository
}

type quantizedBillingUserRepoStub struct {
	UserRepository
	deducted float64
}

func (s *quantizedBillingUserRepoStub) DeductBalance(_ context.Context, _ int64, amount float64) error {
	s.deducted = amount
	return nil
}

func newQuantizedBillingCacheService(t *testing.T, effects chan<- quantizedBillingEffect, flusherEnabled bool) (*BillingCacheService, *config.Config) {
	t.Helper()

	cfg := &config.Config{}
	cfg.Database.UserPlatformQuotaFlusherEnabled = flusherEnabled
	cacheService := NewBillingCacheService(
		&quantizedBillingCacheStub{effects: effects},
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		nil,
	)
	t.Cleanup(cacheService.Stop)
	return cacheService, cfg
}

func collectQuantizedBillingEffects(t *testing.T, effects <-chan quantizedBillingEffect, count int) map[string]quantizedBillingEffect {
	t.Helper()

	got := make(map[string]quantizedBillingEffect, count)
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for len(got) < count {
		select {
		case effect := <-effects:
			got[effect.kind] = effect
		case <-timer.C:
			t.Fatalf("timed out waiting for billing effects: got=%v want_count=%d", got, count)
		}
	}
	return got
}

func TestApplyUsageBillingPropagatesQuantizedCommandAmounts(t *testing.T) {
	const (
		rawActualCost         = 0.000078125
		accountRateMultiplier = 1.25
	)

	effects := make(chan quantizedBillingEffect, 8)
	cacheService, cfg := newQuantizedBillingCacheService(t, effects, true)
	newBalance := 100 - QuantizeUsageBillingAmount(rawActualCost)
	repo := &quantizedBillingRepoStub{result: &UsageBillingApplyResult{
		Applied:    true,
		NewBalance: &newBalance,
		QuotaState: &AccountQuotaState{},
	}}
	p := &postUsageBillingParams{
		Cost:                  &CostBreakdown{ActualCost: rawActualCost, TotalCost: rawActualCost},
		User:                  &User{ID: 101, Balance: 100},
		APIKey:                &APIKey{ID: 201, Quota: 1, RateLimit5h: 1},
		Account:               &Account{ID: 301, Type: AccountTypeAPIKey, Extra: map[string]any{"quota_limit": 1.0}},
		AccountRateMultiplier: accountRateMultiplier,
		APIKeyService:         &quantizedBillingAPIKeyUpdaterStub{},
		Platform:              PlatformOpenAI,
	}
	usageLog := &UsageLog{
		ActualCost: rawActualCost,
		Model:      "gpt-5.1",
	}
	wantCmd := buildUsageBillingCommand("quantized-success", usageLog, p)
	require.NotNil(t, wantCmd)

	result, err := applyUsageBilling(context.Background(), "quantized-success", usageLog, p, &billingDeps{
		billingCacheService:   cacheService,
		deferredService:       &DeferredService{},
		balanceNotifyService:  &quantizedBillingNotifierStub{effects: effects},
		userPlatformQuotaRepo: &quantizedBillingPlatformRepoStub{},
		cfg:                   cfg,
	}, repo)

	require.NoError(t, err)
	require.True(t, result.Applied)
	require.NotNil(t, repo.cmd)
	require.Equal(t, wantCmd.RequestFingerprint, repo.cmd.RequestFingerprint, "fingerprint must remain derived from raw amounts")
	require.Equal(t, rawActualCost, p.Cost.ActualCost, "post-processing must not mutate the raw fingerprint input")
	require.Equal(t, wantCmd.BalanceCost, usageLog.ActualCost)

	got := collectQuantizedBillingEffects(t, effects, 5)
	require.Equal(t, wantCmd.BalanceCost, got[quantizedBillingEffectBalanceCache].amount)
	require.Equal(t, wantCmd.APIKeyRateLimitCost, got[quantizedBillingEffectRateLimitCache].amount)
	require.Equal(t, wantCmd.BalanceCost, got[quantizedBillingEffectPlatformCache].amount)
	require.Equal(t, wantCmd.BalanceCost, got[quantizedBillingEffectBalanceNotification].amount)
	require.Equal(t, newBalance+wantCmd.BalanceCost, got[quantizedBillingEffectBalanceNotification].oldAmount)
	require.Equal(t, wantCmd.AccountQuotaCost, got[quantizedBillingEffectAccountNotification].amount)
}

func TestApplyUsageBillingPropagatesQuantizedSubscriptionAndRateLimitAmounts(t *testing.T) {
	const rawActualCost = 0.000078125

	effects := make(chan quantizedBillingEffect, 4)
	cacheService, cfg := newQuantizedBillingCacheService(t, effects, true)
	repo := &quantizedBillingRepoStub{result: &UsageBillingApplyResult{
		Applied:             true,
		SubscriptionVersion: 123,
	}}
	groupID := int64(401)
	p := &postUsageBillingParams{
		Cost:               &CostBreakdown{ActualCost: rawActualCost, TotalCost: rawActualCost},
		User:               &User{ID: 102},
		APIKey:             &APIKey{ID: 202, GroupID: &groupID, RateLimit5h: 1},
		Account:            &Account{ID: 302},
		Subscription:       &UserSubscription{ID: 402},
		IsSubscriptionBill: true,
		APIKeyService:      &quantizedBillingAPIKeyUpdaterStub{},
		Platform:           PlatformOpenAI,
	}
	usageLog := &UsageLog{ActualCost: rawActualCost, Model: "gpt-5.1"}
	wantCmd := buildUsageBillingCommand("quantized-subscription", usageLog, p)
	require.NotNil(t, wantCmd)

	result, err := applyUsageBilling(context.Background(), "quantized-subscription", usageLog, p, &billingDeps{
		billingCacheService:   cacheService,
		deferredService:       &DeferredService{},
		balanceNotifyService:  &quantizedBillingNotifierStub{effects: effects},
		userPlatformQuotaRepo: &quantizedBillingPlatformRepoStub{},
		cfg:                   cfg,
	}, repo)

	require.NoError(t, err)
	require.True(t, result.Applied)
	require.Equal(t, rawActualCost, p.Cost.ActualCost)
	require.Equal(t, wantCmd.SubscriptionCost, usageLog.ActualCost)

	got := collectQuantizedBillingEffects(t, effects, 2)
	require.Equal(t, wantCmd.SubscriptionCost, got[quantizedBillingEffectSubscriptionCache].amount)
	require.Equal(t, wantCmd.APIKeyRateLimitCost, got[quantizedBillingEffectRateLimitCache].amount)
	require.NotContains(t, got, quantizedBillingEffectBalanceCache)
	require.NotContains(t, got, quantizedBillingEffectPlatformCache)
}

func TestApplyUsageBillingPropagatesQuantizedEntitlementFallbackAmount(t *testing.T) {
	const rawActualCost = 0.000078125

	effects := make(chan quantizedBillingEffect, 4)
	cacheService, cfg := newQuantizedBillingCacheService(t, effects, true)
	newBalance := 100 - QuantizeUsageBillingAmount(rawActualCost)
	repo := &quantizedBillingRepoStub{result: &UsageBillingApplyResult{Applied: true, NewBalance: &newBalance}}
	p := &postUsageBillingParams{
		Cost:                       &CostBreakdown{ActualCost: rawActualCost, TotalCost: rawActualCost},
		User:                       &User{ID: 103, Balance: 100},
		APIKey:                     &APIKey{ID: 203},
		Account:                    &Account{ID: 303},
		Entitlement:                &SubscriptionEntitlement{ID: 403},
		EntitlementBalanceFallback: true,
		IsSubscriptionBill:         true,
	}
	usageLog := &UsageLog{ActualCost: rawActualCost, Model: "gpt-5.1"}
	wantCmd := buildUsageBillingCommand("quantized-entitlement-fallback", usageLog, p)
	require.NotNil(t, wantCmd)

	result, err := applyUsageBilling(context.Background(), "quantized-entitlement-fallback", usageLog, p, &billingDeps{
		billingCacheService:  cacheService,
		deferredService:      &DeferredService{},
		balanceNotifyService: &quantizedBillingNotifierStub{effects: effects},
		cfg:                  cfg,
	}, repo)

	require.NoError(t, err)
	require.True(t, result.Applied)
	require.NotNil(t, repo.cmd)
	require.Equal(t, wantCmd.SubscriptionCost, repo.cmd.SubscriptionCost)
	require.Equal(t, wantCmd.SubscriptionCost, usageLog.ActualCost)

	got := collectQuantizedBillingEffects(t, effects, 2)
	require.Equal(t, wantCmd.SubscriptionCost, got[quantizedBillingEffectBalanceCache].amount)
	require.Equal(t, wantCmd.SubscriptionCost, got[quantizedBillingEffectBalanceNotification].amount)
}

func TestApplyUsageBillingAppliedFalseOnlyNormalizesUsageLog(t *testing.T) {
	const rawActualCost = 0.000078125

	effects := make(chan quantizedBillingEffect, 1)
	cacheService, cfg := newQuantizedBillingCacheService(t, effects, true)
	repo := &quantizedBillingRepoStub{result: &UsageBillingApplyResult{Applied: false}}
	p := &postUsageBillingParams{
		Cost:    &CostBreakdown{ActualCost: rawActualCost, TotalCost: rawActualCost},
		User:    &User{ID: 104},
		APIKey:  &APIKey{ID: 204},
		Account: &Account{ID: 304},
	}
	usageLog := &UsageLog{ActualCost: rawActualCost}

	result, err := applyUsageBilling(context.Background(), "quantized-replay", usageLog, p, &billingDeps{
		billingCacheService: cacheService,
		deferredService:     &DeferredService{},
		cfg:                 cfg,
	}, repo)

	require.NoError(t, err)
	require.False(t, result.Applied)
	require.Equal(t, QuantizeUsageBillingAmount(rawActualCost), usageLog.ActualCost)
	require.Equal(t, rawActualCost, p.Cost.ActualCost)
	require.Empty(t, effects)
}

func TestApplyUsageBillingErrorLeavesUsageLogUnchanged(t *testing.T) {
	const rawActualCost = 0.000078125
	wantErr := context.DeadlineExceeded
	repo := &quantizedBillingRepoStub{err: wantErr}
	p := &postUsageBillingParams{
		Cost:    &CostBreakdown{ActualCost: rawActualCost, TotalCost: rawActualCost},
		User:    &User{ID: 105},
		APIKey:  &APIKey{ID: 205},
		Account: &Account{ID: 305},
	}
	usageLog := &UsageLog{ActualCost: rawActualCost}

	_, err := applyUsageBilling(context.Background(), "quantized-error", usageLog, p, &billingDeps{}, repo)

	require.ErrorIs(t, err, wantErr)
	require.Equal(t, rawActualCost, usageLog.ActualCost)
}

func TestApplyUsageBillingLegacyFallbackUsesQuantizedAmount(t *testing.T) {
	const rawActualCost = 0.000078125

	userRepo := &quantizedBillingUserRepoStub{}
	p := &postUsageBillingParams{
		Cost:    &CostBreakdown{ActualCost: rawActualCost, TotalCost: rawActualCost},
		User:    &User{ID: 106},
		APIKey:  &APIKey{ID: 206},
		Account: &Account{ID: 306},
	}
	usageLog := &UsageLog{ActualCost: rawActualCost}

	result, err := applyUsageBilling(context.Background(), "quantized-legacy", usageLog, p, &billingDeps{userRepo: userRepo}, nil)

	require.NoError(t, err)
	require.Nil(t, result)
	want := QuantizeUsageBillingAmount(rawActualCost)
	require.Equal(t, want, userRepo.deducted)
	require.Equal(t, want, usageLog.ActualCost)
	require.Equal(t, rawActualCost, p.Cost.ActualCost)
}

var _ billingBalanceNotifier = (*quantizedBillingNotifierStub)(nil)
