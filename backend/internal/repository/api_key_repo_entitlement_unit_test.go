//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyRepository_SubscriptionEntitlementIDRoundTrip_SQLite(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "api-key-entitlement@test.com")

	group, err := client.Group.Create().
		SetName("subscription-entitlement-group").
		SetPlatform(service.PlatformOpenAI).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeSubscription).
		SetRateMultiplier(1).
		Save(ctx)
	require.NoError(t, err)

	now := time.Now().UTC()
	entitlement, err := client.SubscriptionEntitlement.Create().
		SetUserID(user.ID).
		SetName("binding entitlement").
		SetStatus(service.SubscriptionStatusActive).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(now.Add(24 * time.Hour)).
		Save(ctx)
	require.NoError(t, err)

	key := &service.APIKey{
		UserID:                    user.ID,
		Key:                       "sk-api-key-entitlement-roundtrip",
		Name:                      "Entitlement Key",
		GroupID:                   &group.ID,
		SubscriptionEntitlementID: &entitlement.ID,
		Status:                    service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, key))
	require.NotNil(t, key.SubscriptionEntitlementID)
	require.Equal(t, entitlement.ID, *key.SubscriptionEntitlementID)

	byID, err := repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.NotNil(t, byID.SubscriptionEntitlementID)
	require.Equal(t, entitlement.ID, *byID.SubscriptionEntitlementID)

	forAuth, err := repo.GetByKeyForAuth(ctx, key.Key)
	require.NoError(t, err)
	require.NotNil(t, forAuth.SubscriptionEntitlementID)
	require.Equal(t, entitlement.ID, *forAuth.SubscriptionEntitlementID)

	list, _, err := repo.ListByUserID(ctx, user.ID, pagination.PaginationParams{Page: 1, PageSize: 10}, service.APIKeyListFilters{})
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.NotNil(t, list[0].SubscriptionEntitlementID)
	require.Equal(t, entitlement.ID, *list[0].SubscriptionEntitlementID)

	key.SubscriptionEntitlementID = nil
	require.NoError(t, repo.Update(ctx, key, service.APIKeyUpdateFields{AccessBinding: true}))
	cleared, err := repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.Nil(t, cleared.SubscriptionEntitlementID)

	key.SubscriptionEntitlementID = &entitlement.ID
	require.NoError(t, repo.Update(ctx, key, service.APIKeyUpdateFields{AccessBinding: true}))
	restored, err := repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.NotNil(t, restored.SubscriptionEntitlementID)
	require.Equal(t, entitlement.ID, *restored.SubscriptionEntitlementID)
}

func TestAPIKeyRepository_CompareAndSwapGroupIDWithEntitlement_SQLite(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "api-key-entitlement-cas@test.com")

	groupA, err := client.Group.Create().
		SetName("subscription-entitlement-cas-a").
		SetPlatform(service.PlatformOpenAI).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeSubscription).
		SetRateMultiplier(1).
		Save(ctx)
	require.NoError(t, err)
	groupB, err := client.Group.Create().
		SetName("subscription-entitlement-cas-b").
		SetPlatform(service.PlatformOpenAI).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeSubscription).
		SetRateMultiplier(1).
		Save(ctx)
	require.NoError(t, err)

	now := time.Now().UTC()
	entitlement, err := client.SubscriptionEntitlement.Create().
		SetUserID(user.ID).
		SetName("cas entitlement").
		SetStatus(service.SubscriptionStatusActive).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(now.Add(24 * time.Hour)).
		Save(ctx)
	require.NoError(t, err)

	key := &service.APIKey{
		UserID:                    user.ID,
		Key:                       "sk-api-key-entitlement-cas",
		Name:                      "Entitlement CAS Key",
		GroupID:                   &groupA.ID,
		SubscriptionEntitlementID: &entitlement.ID,
		Status:                    service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, key))

	wrongEntitlementID := entitlement.ID + 999
	swapped, err := repo.CompareAndSwapGroupIDWithEntitlement(ctx, key.ID, groupA.ID, groupB.ID, &wrongEntitlementID, &wrongEntitlementID)
	require.NoError(t, err)
	require.False(t, swapped)

	swapped, err = repo.CompareAndSwapGroupIDWithEntitlement(ctx, key.ID, groupA.ID, groupB.ID, &entitlement.ID, &entitlement.ID)
	require.NoError(t, err)
	require.True(t, swapped)

	updated, err := repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.NotNil(t, updated.GroupID)
	require.Equal(t, groupB.ID, *updated.GroupID)
	require.NotNil(t, updated.SubscriptionEntitlementID)
	require.Equal(t, entitlement.ID, *updated.SubscriptionEntitlementID)
}
