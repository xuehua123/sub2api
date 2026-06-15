//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSubscriptionService_EntitlementAliasRuntimeDefaultsLegacy(t *testing.T) {
	svc := NewSubscriptionService(nil, nil, nil, nil, nil)

	require.False(t, svc.ShouldUseSubscriptionEntitlementAliases(context.Background()))

	svc.SetSubscriptionEntitlementAliasDependencies(subscriptionAliasRuntimeProviderStub{
		runtime: SubscriptionEntitlementsRuntime{Enabled: true},
	}, nil)
	require.False(t, svc.ShouldUseSubscriptionEntitlementAliases(context.Background()))
}

func TestSubscriptionService_ListEntitlementAliasesFiltersEntitlementOnlyRecords(t *testing.T) {
	now := time.Now()
	repo := newFakeSubscriptionEntitlementRepo(now)
	entSvc := NewSubscriptionEntitlementService(repo, nil)
	svc := NewSubscriptionService(nil, nil, nil, nil, nil)
	svc.SetSubscriptionEntitlementAliasDependencies(subscriptionAliasRuntimeProviderStub{
		runtime: SubscriptionEntitlementsRuntime{Enabled: true},
	}, entSvc)
	legacyID := int64(10)
	require.NoError(t, repo.Create(context.Background(), &SubscriptionEntitlement{
		UserID:               1,
		LegacySubscriptionID: &legacyID,
		Status:               SubscriptionStatusActive,
		StartsAt:             now.Add(-time.Hour),
		ExpiresAt:            now.Add(time.Hour),
	}, []int64{100}))
	require.NoError(t, repo.Create(context.Background(), &SubscriptionEntitlement{
		UserID:    1,
		Status:    SubscriptionStatusActive,
		StartsAt:  now.Add(-time.Hour),
		ExpiresAt: now.Add(time.Hour),
	}, []int64{101}))

	got, err := svc.ListUserSubscriptionEntitlementAliases(context.Background(), 1)

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, legacyID, *got[0].LegacySubscriptionID)
}

func TestSubscriptionService_ListActiveEntitlementAliasesFiltersWindowAndLegacyID(t *testing.T) {
	now := time.Now()
	repo := newFakeSubscriptionEntitlementRepo(now)
	entSvc := NewSubscriptionEntitlementService(repo, nil)
	svc := NewSubscriptionService(nil, nil, nil, nil, nil)
	svc.SetSubscriptionEntitlementAliasDependencies(subscriptionAliasRuntimeProviderStub{
		runtime: SubscriptionEntitlementsRuntime{Enabled: true},
	}, entSvc)
	activeLegacyID := int64(10)
	expiredLegacyID := int64(11)
	require.NoError(t, repo.Create(context.Background(), &SubscriptionEntitlement{
		UserID:               1,
		LegacySubscriptionID: &activeLegacyID,
		Status:               SubscriptionStatusActive,
		StartsAt:             now.Add(-time.Hour),
		ExpiresAt:            now.Add(time.Hour),
	}, []int64{100}))
	require.NoError(t, repo.Create(context.Background(), &SubscriptionEntitlement{
		UserID:               1,
		LegacySubscriptionID: &expiredLegacyID,
		Status:               SubscriptionStatusActive,
		StartsAt:             now.Add(-2 * time.Hour),
		ExpiresAt:            now.Add(-time.Hour),
	}, []int64{101}))
	require.NoError(t, repo.Create(context.Background(), &SubscriptionEntitlement{
		UserID:    1,
		Status:    SubscriptionStatusActive,
		StartsAt:  now.Add(-time.Hour),
		ExpiresAt: now.Add(time.Hour),
	}, []int64{102}))

	got, err := svc.ListActiveUserSubscriptionEntitlementAliases(context.Background(), 1, now)

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, activeLegacyID, *got[0].LegacySubscriptionID)
}
