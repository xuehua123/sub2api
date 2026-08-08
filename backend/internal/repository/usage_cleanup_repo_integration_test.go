//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageCleanupRepositoryDeleteUsageLogsBatchFiltersByEntitlement(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	usageRepo := newUsageLogRepositoryWithSQL(client, tx)
	cleanupRepo := newUsageCleanupRepositoryWithSQL(client, tx)

	unique := uuid.NewString()
	user := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("usage-cleanup-%s@example.com", unique)})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-cleanup-" + unique, Name: "cleanup"})
	account := mustCreateAccount(t, client, &service.Account{Name: "usage-cleanup-" + unique})
	targetEntitlement := mustCreateUsageBillingEntitlement(t, client, &service.SubscriptionEntitlement{
		UserID: user.ID,
		Name:   "cleanup-target-" + unique,
	})
	otherEntitlement := mustCreateUsageBillingEntitlement(t, client, &service.SubscriptionEntitlement{
		UserID: user.ID,
		Name:   "cleanup-other-" + unique,
	})

	baseTime := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	createLog := func(entitlementID int64, offset time.Duration) *service.UsageLog {
		log := &service.UsageLog{
			UserID:        user.ID,
			APIKeyID:      apiKey.ID,
			AccountID:     account.ID,
			RequestID:     uuid.NewString(),
			Model:         "gpt-5",
			EntitlementID: &entitlementID,
			CreatedAt:     baseTime.Add(offset),
		}
		inserted, err := usageRepo.Create(ctx, log)
		require.NoError(t, err)
		require.True(t, inserted)
		return log
	}

	target := createLog(targetEntitlement.ID, 0)
	differentEntitlement := createLog(otherEntitlement.ID, time.Second)
	endBoundary := createLog(targetEntitlement.ID, 2*time.Second)

	startTime := baseTime.Add(-time.Second)
	endTime := baseTime.Add(2 * time.Second)
	targetEntitlementID := targetEntitlement.ID
	deleted, err := cleanupRepo.DeleteUsageLogsBatch(ctx, service.UsageCleanupFilters{
		StartTime:     startTime,
		EndTime:       endTime,
		EntitlementID: &targetEntitlementID,
	}, 10)

	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
	_, err = usageRepo.GetByID(ctx, target.ID)
	require.Error(t, err, "the only row matching all cleanup filters must be deleted")
	got, getErr := usageRepo.GetByID(ctx, differentEntitlement.ID)
	require.NoError(t, getErr, "row with another entitlement must survive")
	require.Equal(t, differentEntitlement.ID, got.ID)
	got, getErr = usageRepo.GetByID(ctx, endBoundary.ID)
	require.NoError(t, getErr, "row exactly at the exclusive end boundary must survive")
	require.Equal(t, endBoundary.ID, got.ID)
}
