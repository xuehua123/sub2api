//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestUsageBillingRepositoryApply_DeduplicatesBalanceBilling(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-" + uuid.NewString(),
		Name:   "billing",
		Quota:  1,
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "usage-billing-account-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:           requestID,
		APIKeyID:            apiKey.ID,
		UserID:              user.ID,
		AccountID:           account.ID,
		AccountType:         service.AccountTypeAPIKey,
		BalanceCost:         1.25,
		APIKeyQuotaCost:     1.25,
		APIKeyRateLimitCost: 1.25,
	}

	result1, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.NotNil(t, result1)
	require.True(t, result1.Applied)
	require.True(t, result1.APIKeyQuotaExhausted)

	result2, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.NotNil(t, result2)
	require.False(t, result2.Applied)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 98.75, balance, 0.000001)

	var quotaUsed float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT quota_used FROM api_keys WHERE id = $1", apiKey.ID).Scan(&quotaUsed))
	require.InDelta(t, 1.25, quotaUsed, 0.000001)

	var usage5h float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT usage_5h FROM api_keys WHERE id = $1", apiKey.ID).Scan(&usage5h))
	require.InDelta(t, 1.25, usage5h, 0.000001)

	var status string
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status FROM api_keys WHERE id = $1", apiKey.ID).Scan(&status))
	require.Equal(t, service.StatusAPIKeyQuotaExhausted, status)

	var dedupCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1 AND api_key_id = $2", requestID, apiKey.ID).Scan(&dedupCount))
	require.Equal(t, 1, dedupCount)
}

func TestUsageBillingRepositoryApply_DeduplicatesSubscriptionBilling(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-sub-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name:             "usage-billing-group-" + uuid.NewString(),
		Platform:         service.PlatformAnthropic,
		SubscriptionType: service.SubscriptionTypeSubscription,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID:  user.ID,
		GroupID: &group.ID,
		Key:     "sk-usage-billing-sub-" + uuid.NewString(),
		Name:    "billing-sub",
	})
	subscription := mustCreateSubscription(t, client, &service.UserSubscription{
		UserID:  user.ID,
		GroupID: group.ID,
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:        requestID,
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		AccountID:        0,
		SubscriptionID:   &subscription.ID,
		SubscriptionCost: 2.5,
	}

	result1, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, result1.Applied)

	result2, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, result2.Applied)

	var dailyUsage float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT daily_usage_usd FROM user_subscriptions WHERE id = $1", subscription.ID).Scan(&dailyUsage))
	require.InDelta(t, 2.5, dailyUsage, 0.000001)
}

func TestUsageBillingRepositoryApply_DeduplicatesEntitlementBilling(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-ent-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-ent-" + uuid.NewString(),
		Name:   "billing-ent",
	})
	now := time.Now().UTC()
	entitlement := mustCreateUsageBillingEntitlement(t, client, &service.SubscriptionEntitlement{
		UserID:             user.ID,
		Name:               "usage billing entitlement",
		StartsAt:           now.Add(-time.Hour),
		ExpiresAt:          now.Add(48 * time.Hour),
		DailyWindowStart:   ptrUsageBillingTime(now.Add(-time.Hour)),
		WeeklyWindowStart:  ptrUsageBillingTime(now.Add(-time.Hour)),
		MonthlyWindowStart: ptrUsageBillingTime(now.Add(-time.Hour)),
		DailyUsageUSD:      1,
		WeeklyUsageUSD:     2,
		MonthlyUsageUSD:    3,
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:        requestID,
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		EntitlementID:    &entitlement.ID,
		SubscriptionCost: 2.5,
	}

	result1, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, result1.Applied)
	require.Greater(t, result1.EntitlementVersion, int64(0))

	result2, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, result2.Applied)

	daily, weekly, monthly := usageBillingEntitlementUsage(t, ctx, entitlement.ID)
	require.InDelta(t, 3.5, daily, 0.000001)
	require.InDelta(t, 4.5, weekly, 0.000001)
	require.InDelta(t, 5.5, monthly, 0.000001)
}

func TestUsageBillingRepositoryApply_EntitlementFingerprintConflictRollsBack(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-ent-conflict-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-ent-conflict-" + uuid.NewString(),
		Name:   "billing-ent-conflict",
	})
	entitlement := mustCreateUsageBillingEntitlement(t, client, &service.SubscriptionEntitlement{
		UserID:    user.ID,
		Name:      "usage billing entitlement conflict",
		StartsAt:  time.Now().UTC().Add(-time.Hour),
		ExpiresAt: time.Now().UTC().Add(48 * time.Hour),
	})
	requestID := uuid.NewString()

	_, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:        requestID,
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		EntitlementID:    &entitlement.ID,
		SubscriptionCost: 1,
	})
	require.NoError(t, err)

	_, err = repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:                  requestID,
		APIKeyID:                   apiKey.ID,
		UserID:                     user.ID,
		EntitlementID:              &entitlement.ID,
		SubscriptionCost:           2,
		EntitlementBalanceFallback: true,
	})
	require.ErrorIs(t, err, service.ErrUsageBillingRequestConflict)

	_, _, monthly := usageBillingEntitlementUsage(t, ctx, entitlement.ID)
	require.InDelta(t, 1, monthly, 0.000001)
	require.InDelta(t, 100, usageBillingUserBalance(t, ctx, user.ID), 0.000001)
}

func TestUsageBillingRepositoryApply_EntitlementResetsExpiredWindows(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-ent-reset-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-ent-reset-" + uuid.NewString(),
		Name:   "billing-ent-reset",
	})
	limit := 5.0
	now := time.Now().UTC()
	entitlement := mustCreateUsageBillingEntitlement(t, client, &service.SubscriptionEntitlement{
		UserID:             user.ID,
		Name:               "usage billing entitlement reset",
		StartsAt:           now.Add(-90 * 24 * time.Hour),
		ExpiresAt:          now.Add(48 * time.Hour),
		DailyWindowStart:   ptrUsageBillingTime(now.Add(-25 * time.Hour)),
		WeeklyWindowStart:  ptrUsageBillingTime(now.Add(-8 * 24 * time.Hour)),
		MonthlyWindowStart: ptrUsageBillingTime(now.Add(-31 * 24 * time.Hour)),
		DailyLimitUSD:      &limit,
		WeeklyLimitUSD:     &limit,
		MonthlyLimitUSD:    &limit,
		DailyUsageUSD:      4,
		WeeklyUsageUSD:     4,
		MonthlyUsageUSD:    4,
	})

	_, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:        uuid.NewString(),
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		EntitlementID:    &entitlement.ID,
		SubscriptionCost: 3,
	})
	require.NoError(t, err)

	daily, weekly, monthly := usageBillingEntitlementUsage(t, ctx, entitlement.ID)
	require.InDelta(t, 3, daily, 0.000001)
	require.InDelta(t, 3, weekly, 0.000001)
	require.InDelta(t, 3, monthly, 0.000001)
}

func TestUsageBillingRepositoryApply_EntitlementQuotaExceededRollsBack(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-ent-limit-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-ent-limit-" + uuid.NewString(),
		Name:   "billing-ent-limit",
	})
	limit := 5.0
	now := time.Now().UTC()
	entitlement := mustCreateUsageBillingEntitlement(t, client, &service.SubscriptionEntitlement{
		UserID:             user.ID,
		Name:               "usage billing entitlement limit",
		StartsAt:           now.Add(-time.Hour),
		ExpiresAt:          now.Add(48 * time.Hour),
		MonthlyWindowStart: ptrUsageBillingTime(now.Add(-time.Hour)),
		MonthlyLimitUSD:    &limit,
		MonthlyUsageUSD:    4,
	})
	requestID := uuid.NewString()

	_, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:        requestID,
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		EntitlementID:    &entitlement.ID,
		SubscriptionCost: 2,
	})
	require.ErrorIs(t, err, service.ErrSubscriptionEntitlementQuotaExceeded)

	_, _, monthly := usageBillingEntitlementUsage(t, ctx, entitlement.ID)
	require.InDelta(t, 4, monthly, 0.000001)
	require.Equal(t, 0, usageBillingDedupCount(t, ctx, requestID, apiKey.ID))
	require.InDelta(t, 100, usageBillingUserBalance(t, ctx, user.ID), 0.000001)
}

func TestUsageBillingRepositoryApply_EntitlementConcurrentQuotaExceeded(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-ent-concurrent-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-ent-concurrent-" + uuid.NewString(),
		Name:   "billing-ent-concurrent",
	})
	limit := 5.0
	now := time.Now().UTC()
	entitlement := mustCreateUsageBillingEntitlement(t, client, &service.SubscriptionEntitlement{
		UserID:             user.ID,
		Name:               "usage billing entitlement concurrent",
		StartsAt:           now.Add(-time.Hour),
		ExpiresAt:          now.Add(48 * time.Hour),
		MonthlyWindowStart: ptrUsageBillingTime(now.Add(-time.Hour)),
		MonthlyLimitUSD:    &limit,
	})

	start := make(chan struct{})
	var wg sync.WaitGroup
	var successCount int64
	var quotaExceededCount int64
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := repo.Apply(ctx, &service.UsageBillingCommand{
				RequestID:        uuid.NewString(),
				APIKeyID:         apiKey.ID,
				UserID:           user.ID,
				EntitlementID:    &entitlement.ID,
				SubscriptionCost: 3,
			})
			switch {
			case err == nil:
				atomic.AddInt64(&successCount, 1)
			case errors.Is(err, service.ErrSubscriptionEntitlementQuotaExceeded):
				atomic.AddInt64(&quotaExceededCount, 1)
			default:
				require.NoError(t, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	require.Equal(t, int64(1), atomic.LoadInt64(&successCount))
	require.Equal(t, int64(1), atomic.LoadInt64(&quotaExceededCount))
	_, _, monthly := usageBillingEntitlementUsage(t, ctx, entitlement.ID)
	require.InDelta(t, 3, monthly, 0.000001)
}

func TestUsageBillingRepositoryApply_EntitlementBalanceFallbackRequiresPolicy(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-ent-fallback-block-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-ent-fallback-block-" + uuid.NewString(),
		Name:   "billing-ent-fallback-block",
	})
	limit := 5.0
	now := time.Now().UTC()
	entitlement := mustCreateUsageBillingEntitlement(t, client, &service.SubscriptionEntitlement{
		UserID:             user.ID,
		Name:               "usage billing entitlement fallback block",
		StartsAt:           now.Add(-time.Hour),
		ExpiresAt:          now.Add(48 * time.Hour),
		MonthlyWindowStart: ptrUsageBillingTime(now.Add(-time.Hour)),
		MonthlyLimitUSD:    &limit,
		MonthlyUsageUSD:    5,
		OveragePolicy:      service.SubscriptionEntitlementOverageBlock,
	})

	_, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:                  uuid.NewString(),
		APIKeyID:                   apiKey.ID,
		UserID:                     user.ID,
		EntitlementID:              &entitlement.ID,
		SubscriptionCost:           1,
		EntitlementBalanceFallback: true,
	})
	require.ErrorIs(t, err, service.ErrSubscriptionEntitlementQuotaExceeded)
	_, _, monthly := usageBillingEntitlementUsage(t, ctx, entitlement.ID)
	require.InDelta(t, 5, monthly, 0.000001)
	require.InDelta(t, 100, usageBillingUserBalance(t, ctx, user.ID), 0.000001)
}

func TestUsageBillingRepositoryApply_EntitlementBalanceFallbackRequiresCommandFlag(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-ent-fallback-flag-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-ent-fallback-flag-" + uuid.NewString(),
		Name:   "billing-ent-fallback-flag",
	})
	limit := 5.0
	now := time.Now().UTC()
	entitlement := mustCreateUsageBillingEntitlement(t, client, &service.SubscriptionEntitlement{
		UserID:             user.ID,
		Name:               "usage billing entitlement fallback flag",
		StartsAt:           now.Add(-time.Hour),
		ExpiresAt:          now.Add(48 * time.Hour),
		MonthlyWindowStart: ptrUsageBillingTime(now.Add(-time.Hour)),
		MonthlyLimitUSD:    &limit,
		MonthlyUsageUSD:    5,
		OveragePolicy:      service.SubscriptionEntitlementOverageBalanceFallback,
	})

	_, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:        uuid.NewString(),
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		EntitlementID:    &entitlement.ID,
		SubscriptionCost: 1,
	})
	require.ErrorIs(t, err, service.ErrSubscriptionEntitlementQuotaExceeded)
	_, _, monthly := usageBillingEntitlementUsage(t, ctx, entitlement.ID)
	require.InDelta(t, 5, monthly, 0.000001)
	require.InDelta(t, 100, usageBillingUserBalance(t, ctx, user.ID), 0.000001)
}

func TestUsageBillingRepositoryApply_EntitlementBalanceFallbackDeductsBalance(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-ent-fallback-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-ent-fallback-" + uuid.NewString(),
		Name:   "billing-ent-fallback",
	})
	limit := 5.0
	now := time.Now().UTC()
	entitlement := mustCreateUsageBillingEntitlement(t, client, &service.SubscriptionEntitlement{
		UserID:             user.ID,
		Name:               "usage billing entitlement fallback",
		StartsAt:           now.Add(-time.Hour),
		ExpiresAt:          now.Add(48 * time.Hour),
		MonthlyWindowStart: ptrUsageBillingTime(now.Add(-time.Hour)),
		MonthlyLimitUSD:    &limit,
		MonthlyUsageUSD:    5,
		OveragePolicy:      service.SubscriptionEntitlementOverageBalanceFallback,
	})

	result, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:                  uuid.NewString(),
		APIKeyID:                   apiKey.ID,
		UserID:                     user.ID,
		EntitlementID:              &entitlement.ID,
		SubscriptionCost:           2,
		EntitlementBalanceFallback: true,
	})
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.Zero(t, result.EntitlementVersion)
	require.NotNil(t, result.NewBalance)
	require.InDelta(t, 98, *result.NewBalance, 0.000001)
	_, _, monthly := usageBillingEntitlementUsage(t, ctx, entitlement.ID)
	require.InDelta(t, 5, monthly, 0.000001)
	require.InDelta(t, 98, usageBillingUserBalance(t, ctx, user.ID), 0.000001)
}

func TestUsageBillingRepositoryApply_EntitlementBalanceFallbackFailureRollsBack(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	t.Run("insufficient_balance", func(t *testing.T) {
		user := mustCreateUser(t, client, &service.User{
			Email:        fmt.Sprintf("usage-billing-ent-fallback-poor-user-%d@example.com", time.Now().UnixNano()),
			PasswordHash: "hash",
			Balance:      0.5,
		})
		apiKey := mustCreateApiKey(t, client, &service.APIKey{
			UserID: user.ID,
			Key:    "sk-usage-billing-ent-fallback-poor-" + uuid.NewString(),
			Name:   "billing-ent-fallback-poor",
		})
		limit := 5.0
		now := time.Now().UTC()
		entitlement := mustCreateUsageBillingEntitlement(t, client, &service.SubscriptionEntitlement{
			UserID:             user.ID,
			Name:               "usage billing entitlement fallback poor",
			StartsAt:           now.Add(-time.Hour),
			ExpiresAt:          now.Add(48 * time.Hour),
			MonthlyWindowStart: ptrUsageBillingTime(now.Add(-time.Hour)),
			MonthlyLimitUSD:    &limit,
			MonthlyUsageUSD:    5,
			OveragePolicy:      service.SubscriptionEntitlementOverageBalanceFallback,
		})
		requestID := uuid.NewString()

		_, err := repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:                  requestID,
			APIKeyID:                   apiKey.ID,
			UserID:                     user.ID,
			EntitlementID:              &entitlement.ID,
			SubscriptionCost:           1,
			EntitlementBalanceFallback: true,
		})
		require.ErrorIs(t, err, service.ErrInsufficientBalance)
		_, _, monthly := usageBillingEntitlementUsage(t, ctx, entitlement.ID)
		require.InDelta(t, 5, monthly, 0.000001)
		require.InDelta(t, 0.5, usageBillingUserBalance(t, ctx, user.ID), 0.000001)
		require.Equal(t, 0, usageBillingDedupCount(t, ctx, requestID, apiKey.ID))
	})

	t.Run("soft_deleted_user", func(t *testing.T) {
		user := mustCreateUser(t, client, &service.User{
			Email:        fmt.Sprintf("usage-billing-ent-fallback-deleted-user-%d@example.com", time.Now().UnixNano()),
			PasswordHash: "hash",
			Balance:      100,
		})
		apiKey := mustCreateApiKey(t, client, &service.APIKey{
			UserID: user.ID,
			Key:    "sk-usage-billing-ent-fallback-deleted-" + uuid.NewString(),
			Name:   "billing-ent-fallback-deleted",
		})
		limit := 5.0
		now := time.Now().UTC()
		entitlement := mustCreateUsageBillingEntitlement(t, client, &service.SubscriptionEntitlement{
			UserID:             user.ID,
			Name:               "usage billing entitlement fallback deleted",
			StartsAt:           now.Add(-time.Hour),
			ExpiresAt:          now.Add(48 * time.Hour),
			MonthlyWindowStart: ptrUsageBillingTime(now.Add(-time.Hour)),
			MonthlyLimitUSD:    &limit,
			MonthlyUsageUSD:    5,
			OveragePolicy:      service.SubscriptionEntitlementOverageBalanceFallback,
		})
		_, err := integrationDB.ExecContext(ctx, "UPDATE users SET deleted_at = NOW() WHERE id = $1", user.ID)
		require.NoError(t, err)
		requestID := uuid.NewString()

		_, err = repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:                  requestID,
			APIKeyID:                   apiKey.ID,
			UserID:                     user.ID,
			EntitlementID:              &entitlement.ID,
			SubscriptionCost:           1,
			EntitlementBalanceFallback: true,
		})
		require.ErrorIs(t, err, service.ErrUserNotFound)
		_, _, monthly := usageBillingEntitlementUsage(t, ctx, entitlement.ID)
		require.InDelta(t, 5, monthly, 0.000001)
		require.Equal(t, 0, usageBillingDedupCount(t, ctx, requestID, apiKey.ID))
	})
}

func TestUsageBillingRepositoryApply_RequestFingerprintConflict(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-conflict-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-conflict-" + uuid.NewString(),
		Name:   "billing-conflict",
	})

	requestID := uuid.NewString()
	_, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:   requestID,
		APIKeyID:    apiKey.ID,
		UserID:      user.ID,
		BalanceCost: 1.25,
	})
	require.NoError(t, err)

	_, err = repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:   requestID,
		APIKeyID:    apiKey.ID,
		UserID:      user.ID,
		BalanceCost: 2.50,
	})
	require.ErrorIs(t, err, service.ErrUsageBillingRequestConflict)
}

func TestUsageBillingRepositoryApply_UpdatesAccountQuota(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-account-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-account-" + uuid.NewString(),
		Name:   "billing-account",
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "usage-billing-account-quota-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
		Extra: map[string]any{
			"quota_limit": 100.0,
		},
	})

	_, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:        uuid.NewString(),
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		AccountID:        account.ID,
		AccountType:      service.AccountTypeAPIKey,
		AccountQuotaCost: 3.5,
	})
	require.NoError(t, err)

	var quotaUsed float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COALESCE((extra->>'quota_used')::numeric, 0) FROM accounts WHERE id = $1", account.ID).Scan(&quotaUsed))
	require.InDelta(t, 3.5, quotaUsed, 0.000001)
}

func TestUsageBillingRepositoryApply_EnqueuesSchedulerOutboxOnQuotaCrossing(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	newFixture := func(t *testing.T, extra map[string]any) (int64, int64) {
		t.Helper()
		user := mustCreateUser(t, client, &service.User{
			Email:        fmt.Sprintf("usage-billing-outbox-user-%d-%s@example.com", time.Now().UnixNano(), uuid.NewString()),
			PasswordHash: "hash",
		})
		apiKey := mustCreateApiKey(t, client, &service.APIKey{
			UserID: user.ID,
			Key:    "sk-usage-billing-outbox-" + uuid.NewString(),
			Name:   "billing-outbox",
		})
		account := mustCreateAccount(t, client, &service.Account{
			Name:  "usage-billing-outbox-" + uuid.NewString(),
			Type:  service.AccountTypeAPIKey,
			Extra: extra,
		})
		return apiKey.ID, account.ID
	}

	outboxCountFor := func(t *testing.T, accountID int64) int {
		t.Helper()
		var count int
		require.NoError(t, integrationDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM scheduler_outbox WHERE event_type = $1 AND account_id = $2",
			service.SchedulerOutboxEventAccountChanged, accountID,
		).Scan(&count))
		return count
	}

	t.Run("daily_first_crossing_enqueues", func(t *testing.T) {
		apiKeyID, accountID := newFixture(t, map[string]any{
			"quota_daily_limit": 10.0,
		})
		// Below the daily limit: do not enqueue outbox.
		_, err := repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			AccountID:        accountID,
			AccountType:      service.AccountTypeAPIKey,
			AccountQuotaCost: 4,
		})
		require.NoError(t, err)
		require.Equal(t, 0, outboxCountFor(t, accountID), "below limit should not enqueue")

		// Crossing the daily limit should enqueue once.
		_, err = repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			AccountID:        accountID,
			AccountType:      service.AccountTypeAPIKey,
			AccountQuotaCost: 8,
		})
		require.NoError(t, err)
		require.Equal(t, 1, outboxCountFor(t, accountID), "crossing daily limit should enqueue once")

		// Subsequent increments beyond the limit should not enqueue again.
		_, err = repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			AccountID:        accountID,
			AccountType:      service.AccountTypeAPIKey,
			AccountQuotaCost: 2,
		})
		require.NoError(t, err)
		require.Equal(t, 1, outboxCountFor(t, accountID), "subsequent increments beyond limit should not re-enqueue")
	})

	t.Run("weekly_first_crossing_enqueues", func(t *testing.T) {
		apiKeyID, accountID := newFixture(t, map[string]any{
			"quota_weekly_limit": 10.0,
		})
		_, err := repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			AccountID:        accountID,
			AccountType:      service.AccountTypeAPIKey,
			AccountQuotaCost: 15,
		})
		require.NoError(t, err)
		require.Equal(t, 1, outboxCountFor(t, accountID), "single-shot crossing weekly limit should enqueue once")
	})
}

func TestDashboardAggregationRepositoryCleanupUsageBillingDedup_BatchDeletesOldRows(t *testing.T) {
	ctx := context.Background()
	repo := newDashboardAggregationRepositoryWithSQL(integrationDB)

	oldRequestID := "dedup-old-" + uuid.NewString()
	newRequestID := "dedup-new-" + uuid.NewString()
	oldCreatedAt := time.Now().UTC().AddDate(0, 0, -400)
	newCreatedAt := time.Now().UTC().Add(-time.Hour)

	_, err := integrationDB.ExecContext(ctx, `
		INSERT INTO usage_billing_dedup (request_id, api_key_id, request_fingerprint, created_at)
		VALUES ($1, 1, $2, $3), ($4, 1, $5, $6)
	`,
		oldRequestID, strings.Repeat("a", 64), oldCreatedAt,
		newRequestID, strings.Repeat("b", 64), newCreatedAt,
	)
	require.NoError(t, err)

	require.NoError(t, repo.CleanupUsageBillingDedup(ctx, time.Now().UTC().AddDate(0, 0, -365)))

	var oldCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1", oldRequestID).Scan(&oldCount))
	require.Equal(t, 0, oldCount)

	var newCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1", newRequestID).Scan(&newCount))
	require.Equal(t, 1, newCount)

	var archivedCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup_archive WHERE request_id = $1", oldRequestID).Scan(&archivedCount))
	require.Equal(t, 1, archivedCount)
}

func TestUsageBillingRepositoryApply_DeduplicatesAgainstArchivedKey(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)
	aggRepo := newDashboardAggregationRepositoryWithSQL(integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-archive-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-archive-" + uuid.NewString(),
		Name:   "billing-archive",
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:   requestID,
		APIKeyID:    apiKey.ID,
		UserID:      user.ID,
		BalanceCost: 1.25,
	}

	result1, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, result1.Applied)

	_, err = integrationDB.ExecContext(ctx, `
		UPDATE usage_billing_dedup
		SET created_at = $1
		WHERE request_id = $2 AND api_key_id = $3
	`, time.Now().UTC().AddDate(0, 0, -400), requestID, apiKey.ID)
	require.NoError(t, err)
	require.NoError(t, aggRepo.CleanupUsageBillingDedup(ctx, time.Now().UTC().AddDate(0, 0, -365)))

	result2, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, result2.Applied)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 98.75, balance, 0.000001)
}

func ptrUsageBillingTime(v time.Time) *time.Time {
	return &v
}

func mustCreateUsageBillingEntitlement(t *testing.T, client *dbent.Client, ent *service.SubscriptionEntitlement) *service.SubscriptionEntitlement {
	t.Helper()
	ctx := context.Background()
	if ent.Name == "" {
		ent.Name = "usage billing entitlement"
	}
	if ent.Status == "" {
		ent.Status = service.SubscriptionStatusActive
	}
	if ent.SourceType == "" {
		ent.SourceType = service.SubscriptionEntitlementSourceUnknown
	}
	if ent.OveragePolicy == "" {
		ent.OveragePolicy = service.SubscriptionEntitlementOverageBlock
	}
	if ent.StartsAt.IsZero() {
		ent.StartsAt = time.Now().UTC().Add(-time.Hour)
	}
	if ent.ExpiresAt.IsZero() {
		ent.ExpiresAt = time.Now().UTC().Add(24 * time.Hour)
	}

	create := client.SubscriptionEntitlement.Create().
		SetUserID(ent.UserID).
		SetName(ent.Name).
		SetStatus(ent.Status).
		SetSourceType(ent.SourceType).
		SetStartsAt(ent.StartsAt).
		SetExpiresAt(ent.ExpiresAt).
		SetDailyUsageUsd(ent.DailyUsageUSD).
		SetWeeklyUsageUsd(ent.WeeklyUsageUSD).
		SetMonthlyUsageUsd(ent.MonthlyUsageUSD).
		SetOveragePolicy(ent.OveragePolicy)
	if ent.PlanID != nil {
		create.SetPlanID(*ent.PlanID)
	}
	if ent.LegacySubscriptionID != nil {
		create.SetLegacySubscriptionID(*ent.LegacySubscriptionID)
	}
	if ent.PrimaryGroupID != nil {
		create.SetPrimaryGroupID(*ent.PrimaryGroupID)
	}
	if ent.DailyWindowStart != nil {
		create.SetDailyWindowStart(*ent.DailyWindowStart)
	}
	if ent.WeeklyWindowStart != nil {
		create.SetWeeklyWindowStart(*ent.WeeklyWindowStart)
	}
	if ent.MonthlyWindowStart != nil {
		create.SetMonthlyWindowStart(*ent.MonthlyWindowStart)
	}
	if ent.DailyLimitUSD != nil {
		create.SetDailyLimitUsd(*ent.DailyLimitUSD)
	}
	if ent.WeeklyLimitUSD != nil {
		create.SetWeeklyLimitUsd(*ent.WeeklyLimitUSD)
	}
	if ent.MonthlyLimitUSD != nil {
		create.SetMonthlyLimitUsd(*ent.MonthlyLimitUSD)
	}
	if ent.AssignedBy != nil {
		create.SetAssignedBy(*ent.AssignedBy)
	}
	if !ent.AssignedAt.IsZero() {
		create.SetAssignedAt(ent.AssignedAt)
	}
	if ent.Notes != "" {
		create.SetNotes(ent.Notes)
	}

	created, err := create.Save(ctx)
	require.NoError(t, err)

	ent.ID = created.ID
	ent.CreatedAt = created.CreatedAt
	ent.UpdatedAt = created.UpdatedAt
	return ent
}

func usageBillingEntitlementUsage(t *testing.T, ctx context.Context, entitlementID int64) (daily, weekly, monthly float64) {
	t.Helper()
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT daily_usage_usd, weekly_usage_usd, monthly_usage_usd
		FROM subscription_entitlements
		WHERE id = $1
	`, entitlementID).Scan(&daily, &weekly, &monthly))
	return daily, weekly, monthly
}

func usageBillingUserBalance(t *testing.T, ctx context.Context, userID int64) float64 {
	t.Helper()
	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", userID).Scan(&balance))
	return balance
}

func usageBillingDedupCount(t *testing.T, ctx context.Context, requestID string, apiKeyID int64) int {
	t.Helper()
	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM usage_billing_dedup
		WHERE request_id = $1 AND api_key_id = $2
	`, requestID, apiKeyID).Scan(&count))
	return count
}
