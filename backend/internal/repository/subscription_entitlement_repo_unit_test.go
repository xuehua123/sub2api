//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionentitlementgroup"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionEntitlementRepository_ListByUserID_SQLite(t *testing.T) {
	_, client := newAPIKeyRepoSQLite(t)
	repo := &subscriptionEntitlementRepository{client: client}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	user := mustCreateAPIKeyRepoUser(t, ctx, client, "ent-list-user@test.com")
	otherUser := mustCreateAPIKeyRepoUser(t, ctx, client, "ent-list-other@test.com")
	groupA := mustCreateSubscriptionEntitlementRepoGroup(t, ctx, client, "ent-list-a", service.PlatformOpenAI)
	groupB := mustCreateSubscriptionEntitlementRepoGroup(t, ctx, client, "ent-list-b", service.PlatformGemini)

	older := &service.SubscriptionEntitlement{
		UserID:     user.ID,
		Name:       "older entitlement",
		Status:     service.SubscriptionStatusActive,
		StartsAt:   now.Add(-48 * time.Hour),
		ExpiresAt:  now.Add(24 * time.Hour),
		SourceType: service.SubscriptionEntitlementSourcePaymentOrder,
	}
	require.NoError(t, repo.Create(ctx, older, []int64{groupB.ID, groupA.ID}))

	newer := &service.SubscriptionEntitlement{
		UserID:     user.ID,
		Name:       "newer entitlement",
		Status:     service.SubscriptionStatusExpired,
		StartsAt:   now.Add(-72 * time.Hour),
		ExpiresAt:  now.Add(48 * time.Hour),
		SourceType: service.SubscriptionEntitlementSourceRedeemCode,
	}
	require.NoError(t, repo.Create(ctx, newer, []int64{groupA.ID}))

	other := &service.SubscriptionEntitlement{
		UserID:    otherUser.ID,
		Name:      "other user entitlement",
		Status:    service.SubscriptionStatusActive,
		StartsAt:  now.Add(-time.Hour),
		ExpiresAt: now.Add(72 * time.Hour),
	}
	require.NoError(t, repo.Create(ctx, other, []int64{groupA.ID}))

	deleted := &service.SubscriptionEntitlement{
		UserID:    user.ID,
		Name:      "deleted entitlement",
		Status:    service.SubscriptionStatusActive,
		StartsAt:  now.Add(-time.Hour),
		ExpiresAt: now.Add(96 * time.Hour),
	}
	require.NoError(t, repo.Create(ctx, deleted, []int64{groupA.ID}))
	_, err := client.SubscriptionEntitlement.UpdateOneID(deleted.ID).SetDeletedAt(now).Save(ctx)
	require.NoError(t, err)

	got, err := repo.ListByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, newer.ID, got[0].ID, "list should sort by expires_at desc")
	require.Equal(t, older.ID, got[1].ID)
	require.Equal(t, []int64{groupB.ID, groupA.ID}, subscriptionEntitlementGrantGroupIDs(got[1].GroupGrants))
}

func TestSubscriptionEntitlementRepository_AllDisabledGrantsRemainConfigured_SQLite(t *testing.T) {
	_, client := newAPIKeyRepoSQLite(t)
	repo := &subscriptionEntitlementRepository{client: client}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	user := mustCreateAPIKeyRepoUser(t, ctx, client, "ent-disabled-grants@test.com")
	group := mustCreateSubscriptionEntitlementRepoGroup(t, ctx, client, "ent-disabled-grant", service.PlatformOpenAI)
	entitlement := &service.SubscriptionEntitlement{
		UserID:     user.ID,
		Name:       "disabled grants entitlement",
		Status:     service.SubscriptionStatusActive,
		StartsAt:   now.Add(-time.Hour),
		ExpiresAt:  now.Add(time.Hour),
		SourceType: service.SubscriptionEntitlementSourceAdminAssign,
	}
	require.NoError(t, repo.Create(ctx, entitlement, []int64{group.ID}))

	_, err := client.SubscriptionEntitlementGroup.Update().
		Where(subscriptionentitlementgroup.EntitlementIDEQ(entitlement.ID)).
		SetEnabled(false).
		Save(ctx)
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, entitlement.ID)
	require.NoError(t, err)
	require.True(t, got.GroupGrantsConfigured)
	require.Empty(t, got.GroupGrants, "disabled grant rows must not be exposed as usable grants")
	require.Len(t, got.Groups, 1, "unfiltered group details may remain loaded for display")
	require.Equal(t, group.ID, got.Groups[0].ID)
}

func TestSubscriptionEntitlementRepository_ResetDailyUsageStaleExpectedPreservesCurrentUsage_SQLite(t *testing.T) {
	_, client := newAPIKeyRepoSQLite(t)
	repo := &subscriptionEntitlementRepository{client: client}
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	staleWindowStart := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	currentWindowStart := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

	user := mustCreateAPIKeyRepoUser(t, ctx, client, "ent-reset-cas@test.com")
	entitlement := &service.SubscriptionEntitlement{
		UserID:             user.ID,
		Name:               "daily reset CAS",
		Status:             service.SubscriptionStatusActive,
		StartsAt:           now.AddDate(0, 0, -5),
		ExpiresAt:          now.AddDate(0, 0, 5),
		DailyWindowStart:   &staleWindowStart,
		WeeklyWindowStart:  &staleWindowStart,
		MonthlyWindowStart: &staleWindowStart,
		DailyUsageUSD:      8,
	}
	require.NoError(t, repo.Create(ctx, entitlement, nil))

	require.NoError(t, repo.ResetDailyUsage(ctx, entitlement.ID, &staleWindowStart, currentWindowStart))
	_, err := client.SubscriptionEntitlement.UpdateOneID(entitlement.ID).SetDailyUsageUsd(4.5).Save(ctx)
	require.NoError(t, err)

	// A second request still carrying yesterday's window must be a stale no-op.
	require.NoError(t, repo.ResetDailyUsage(ctx, entitlement.ID, &staleWindowStart, currentWindowStart))
	got, err := repo.GetByID(ctx, entitlement.ID)
	require.NoError(t, err)
	require.Equal(t, 4.5, got.DailyUsageUSD)
	require.NotNil(t, got.DailyWindowStart)
	require.Equal(t, currentWindowStart, *got.DailyWindowStart)

	err = repo.ResetDailyUsage(ctx, entitlement.ID+999, &staleWindowStart, currentWindowStart)
	require.ErrorIs(t, err, service.ErrSubscriptionEntitlementNotFound)
}

func mustCreateSubscriptionEntitlementRepoGroup(t *testing.T, ctx context.Context, client *dbent.Client, name, platform string) *dbent.Group {
	t.Helper()
	group, err := client.Group.Create().
		SetName(name).
		SetPlatform(platform).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeSubscription).
		SetRateMultiplier(1).
		Save(ctx)
	require.NoError(t, err)
	return group
}

func subscriptionEntitlementGrantGroupIDs(grants []service.SubscriptionEntitlementGroupGrant) []int64 {
	ids := make([]int64, 0, len(grants))
	for _, grant := range grants {
		ids = append(ids, grant.GroupID)
	}
	return ids
}
