//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestUsageBillingRepositoryReserveBatchImageEntitlementIsIdempotent(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	billingRepo := NewUsageBillingRepository(client, integrationDB)
	jobRepo := NewBatchImageRepository(integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("batch-image-entitlement-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name:             "batch-image-linked-entitlement-" + uuid.NewString(),
		Platform:         service.PlatformGemini,
		SubscriptionType: service.SubscriptionTypeSubscription,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID:  user.ID,
		GroupID: &group.ID,
		Key:     "sk-batch-image-entitlement-" + uuid.NewString(),
		Name:    "batch image entitlement",
	})
	now := time.Now().UTC()
	windowStart := now.Add(-time.Hour)
	legacy := mustCreateSubscription(t, client, &service.UserSubscription{
		UserID:          user.ID,
		GroupID:         group.ID,
		StartsAt:        now.Add(-time.Hour),
		ExpiresAt:       now.Add(48 * time.Hour),
		DailyUsageUSD:   1,
		WeeklyUsageUSD:  1,
		MonthlyUsageUSD: 1,
	})
	_, err := integrationDB.ExecContext(ctx, `
		UPDATE user_subscriptions
		SET daily_window_start = $1, weekly_window_start = $1, monthly_window_start = $1
		WHERE id = $2
	`, windowStart, legacy.ID)
	require.NoError(t, err)
	entitlement := mustCreateUsageBillingEntitlement(t, client, &service.SubscriptionEntitlement{
		UserID:               user.ID,
		LegacySubscriptionID: &legacy.ID,
		Name:                 "batch image entitlement",
		StartsAt:             legacy.StartsAt,
		ExpiresAt:            legacy.ExpiresAt,
		DailyWindowStart:     &windowStart,
		WeeklyWindowStart:    &windowStart,
		MonthlyWindowStart:   &windowStart,
		DailyUsageUSD:        1,
		WeeklyUsageUSD:       1,
		MonthlyUsageUSD:      1,
	})
	holdAmount := 4.0
	batchID := "imgbatch_" + uuid.NewString()
	job, err := jobRepo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:        batchID,
		UserID:         user.ID,
		APIKeyID:       &apiKey.ID,
		Provider:       service.BatchImageProviderGeminiAPI,
		Model:          "gemini-2.5-flash-image",
		Status:         service.BatchImageJobStatusCreated,
		ItemCount:      1,
		EstimatedCost:  holdAmount,
		HoldAmount:     &holdAmount,
		GroupID:        &group.ID,
		SubscriptionID: &legacy.ID,
		EntitlementID:  &entitlement.ID,
		BillingType:    service.BillingTypeSubscription,
		BillingSource:  service.BillingSourceEntitlementQuota,
		Currency:       "USD",
	})
	require.NoError(t, err)

	cmd, err := serviceTestBuildBatchImageHoldCommand(job, service.BatchImageHoldRequestID(batchID))
	require.NoError(t, err)
	first, err := billingRepo.ReserveBatchImageBalance(ctx, cmd)
	require.NoError(t, err)
	require.True(t, first.Applied)
	require.Equal(t, service.BillingSourceEntitlementQuota, first.BillingSource)
	require.NotNil(t, first.HeldDailyWindowStart)
	require.NotNil(t, first.HeldWeeklyWindowStart)
	require.NotNil(t, first.HeldMonthlyWindowStart)

	second, err := billingRepo.ReserveBatchImageBalance(ctx, cmd)
	require.NoError(t, err)
	require.False(t, second.Applied)

	daily, weekly, monthly := usageBillingEntitlementUsage(t, ctx, entitlement.ID)
	require.InDelta(t, 5, daily, 0.000001)
	require.InDelta(t, 5, weekly, 0.000001)
	require.InDelta(t, 5, monthly, 0.000001)
	legacyDaily, legacyWeekly, legacyMonthly := usageBillingLegacySubscriptionUsage(t, ctx, legacy.ID)
	require.InDelta(t, daily, legacyDaily, 0.000001)
	require.InDelta(t, weekly, legacyWeekly, 0.000001)
	require.InDelta(t, monthly, legacyMonthly, 0.000001)
	require.InDelta(t, 100, usageBillingUserBalance(t, ctx, user.ID), 0.000001)

	persisted, err := jobRepo.GetBatchImageJobByBatchID(ctx, batchID)
	require.NoError(t, err)
	require.Equal(t, service.BillingSourceEntitlementQuota, persisted.BillingSource)
	require.Equal(t, &entitlement.ID, persisted.EntitlementID)
	require.NotNil(t, persisted.HeldDailyWindowStart)
	require.NotNil(t, persisted.HeldWeeklyWindowStart)
	require.NotNil(t, persisted.HeldMonthlyWindowStart)

	captureCmd, err := serviceTestBuildBatchImageHoldCommand(persisted, service.BatchImageCaptureRequestID(batchID))
	require.NoError(t, err)
	captureCmd.ActualAmount = 2
	captureCmd.RequestPayloadHash = "integration-capture"
	captured, err := billingRepo.CaptureBatchImageBalance(ctx, captureCmd)
	require.NoError(t, err)
	require.True(t, captured.Applied)

	daily, weekly, monthly = usageBillingEntitlementUsage(t, ctx, entitlement.ID)
	legacyDaily, legacyWeekly, legacyMonthly = usageBillingLegacySubscriptionUsage(t, ctx, legacy.ID)
	require.InDelta(t, 3, daily, 0.000001)
	require.InDelta(t, 3, weekly, 0.000001)
	require.InDelta(t, 3, monthly, 0.000001)
	require.InDelta(t, daily, legacyDaily, 0.000001)
	require.InDelta(t, weekly, legacyWeekly, 0.000001)
	require.InDelta(t, monthly, legacyMonthly, 0.000001)
}

func serviceTestBuildBatchImageHoldCommand(job *service.BatchImageJob, requestID string) (*service.BatchImageBalanceHoldCommand, error) {
	if job == nil || job.APIKeyID == nil {
		return nil, service.ErrBatchImageBillingHoldFailed
	}
	return &service.BatchImageBalanceHoldCommand{
		RequestID:                  requestID,
		APIKeyID:                   *job.APIKeyID,
		RequestPayloadHash:         "integration",
		UserID:                     job.UserID,
		BatchID:                    job.BatchID,
		GroupID:                    job.GroupID,
		SubscriptionID:             job.SubscriptionID,
		EntitlementID:              job.EntitlementID,
		BillingType:                job.BillingType,
		BillingSource:              job.BillingSource,
		EntitlementBalanceFallback: job.EntitlementBalanceFallback,
		HeldDailyWindowStart:       job.HeldDailyWindowStart,
		HeldWeeklyWindowStart:      job.HeldWeeklyWindowStart,
		HeldMonthlyWindowStart:     job.HeldMonthlyWindowStart,
		HoldAmount:                 *job.HoldAmount,
	}, nil
}
