//go:build integration

package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/subscriptionentitlementevent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestEntitlementLifecycleConcurrentAutoAdvance(t *testing.T) {
	client, userID, _, groupID := newEntitlementConcurrencyFixture(t)
	repo := NewSubscriptionEntitlementRepository(client).(*subscriptionEntitlementRepository)
	svc := service.NewSubscriptionEntitlementService(repo, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	now := time.Now().UTC().Truncate(time.Second)
	start := now.Add(-10 * 24 * time.Hour)
	limit := 100.0
	svc.SetNowFunc(func() time.Time { return now })
	ent := &service.SubscriptionEntitlement{UserID: userID, PrimaryGroupID: &groupID, Status: service.SubscriptionStatusActive,
		StartsAt: start, ExpiresAt: start.Add(90 * 24 * time.Hour), MonthlyWindowStart: &start, MonthlyUsageUSD: 100, MonthlyLimitUSD: &limit, AutoAdvanceMonthly: true}
	require.NoError(t, repo.Create(ctx, ent, []int64{groupID}))
	stale, err := repo.GetByID(ctx, ent.ID)
	require.NoError(t, err)
	var wg sync.WaitGroup
	failures := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := svc.TryAutoAdvanceMonthlyCycle(ctx, stale); failures <- err }()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	got, err := repo.GetByID(ctx, ent.ID)
	require.NoError(t, err)
	require.Equal(t, now.Add(60*24*time.Hour), got.ExpiresAt)
	require.Zero(t, got.MonthlyUsageUSD)
	events, err := svc.EntitlementEvents(ctx, userID, ent.ID, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "automatic_advance", events[0].Kind)
	require.NoError(t, svc.AcknowledgeEntitlementEvent(ctx, userID, ent.ID, events[0].ID))
	visible, err := svc.GetUserEntitlementByID(ctx, userID, ent.ID)
	require.NoError(t, err)
	require.Equal(t, events[0].ID, visible.LastSeenEventID)
	require.NotNil(t, visible.LatestCycle)
	// A second legitimate advance at the same clock instant must retain its
	// distinct microsecond marker through the PostgreSQL round trip.
	_, err = repo.ApplyEntitlementUsage(ctx, ent.ID, 100, now)
	require.NoError(t, err)
	beforeSecond, err := repo.GetByID(ctx, ent.ID)
	require.NoError(t, err)
	second, err := svc.TryAutoAdvanceMonthlyCycle(ctx, beforeSecond)
	require.NoError(t, err)
	require.True(t, second.MonthlyWindowStart.After(*beforeSecond.MonthlyWindowStart))
	stored, err := repo.GetByID(ctx, ent.ID)
	require.NoError(t, err)
	require.Equal(t, *second.MonthlyWindowStart, *stored.MonthlyWindowStart)
	_, err = svc.SetAutoAdvanceMonthly(ctx, userID+1, ent.ID, false)
	require.ErrorIs(t, err, service.ErrSubscriptionEntitlementNotFound)
	require.Error(t, svc.AcknowledgeEntitlementEvent(ctx, userID, ent.ID, events[0].ID+99999))
	_, err = svc.SetAutoAdvanceMonthly(ctx, userID, ent.ID, false)
	require.NoError(t, err)
	got, err = repo.GetByID(ctx, ent.ID)
	require.NoError(t, err)
	require.False(t, got.AutoAdvanceMonthly)
}

func TestEntitlementLifecycleRenewalReceiptsAndReplay(t *testing.T) {
	client, userID, planID, _ := newEntitlementConcurrencyFixture(t)
	_, err := client.SubscriptionPlan.UpdateOneID(planID).SetMonthlyLimitUsd(100).Save(context.Background())
	require.NoError(t, err)
	repo := NewSubscriptionEntitlementRepository(client).(*subscriptionEntitlementRepository)
	svc := service.NewSubscriptionEntitlementService(repo, NewSubscriptionEntitlementPlanRepository(client))
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	svc.SetNowFunc(func() time.Time { return now })
	input := service.AssignEntitlementFromPlanInput{UserID: userID, PlanID: planID, SourceType: service.SubscriptionEntitlementSourcePaymentOrder, OrderID: 1, Now: now}
	first, _, err := svc.AssignOrExtendFromPlan(ctx, input)
	require.NoError(t, err)
	_, err = svc.SetAutoAdvanceMonthly(ctx, userID, first.ID, true)
	require.NoError(t, err)
	input.OrderID = 2
	second, _, err := svc.AssignOrExtendFromPlan(ctx, input)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.True(t, second.AutoAdvanceMonthly)
	_, _, err = svc.AssignOrExtendFromPlan(ctx, input)
	require.NoError(t, err)
	events, err := svc.EntitlementEvents(ctx, userID, first.ID, 0)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, "renewed", events[0].Kind)
	require.Equal(t, first.ExpiresAt, *events[0].PreviousExpiresAt)
	require.Equal(t, second.ExpiresAt, events[0].NewExpiresAt)
	require.Equal(t, int64(second.ExpiresAt.Sub(first.ExpiresAt)/time.Second), events[0].ValiditySeconds)
	_, err = svc.EntitlementEvents(ctx, userID+1, first.ID, 0)
	require.ErrorIs(t, err, service.ErrSubscriptionEntitlementNotFound)
	older, err := svc.EntitlementEvents(ctx, userID, first.ID, events[0].ID)
	require.NoError(t, err)
	require.Len(t, older, 1)
	require.Equal(t, "granted", older[0].Kind)
	count, err := client.SubscriptionEntitlementEvent.Query().Where(subscriptionentitlementevent.EntitlementIDEQ(first.ID)).Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count)
}
