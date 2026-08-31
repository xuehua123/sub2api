//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserCanBindGroupHonorsAllowlistOnlyMode(t *testing.T) {
	t.Parallel()

	legacy := &User{AllowedGroups: []int64{7}}
	require.True(t, legacy.CanBindGroup(1, false), "public groups remain implicit in legacy mode")
	require.True(t, legacy.CanBindGroup(7, true))
	require.False(t, legacy.CanBindGroup(8, true))

	restricted := &User{AllowedGroups: []int64{7}, RestrictToAllowedGroups: true}
	require.True(t, restricted.AllowsGroupType(true))
	require.False(t, restricted.AllowsGroupType(false))
	require.True(t, restricted.CanBindGroup(7, true))
	require.False(t, restricted.CanBindGroup(7, false), "public groups stay unavailable even if a stale allowlist entry exists")
	require.False(t, restricted.CanBindGroup(8, false), "public groups are unavailable in exclusive-only mode")
	require.False(t, restricted.CanBindGroup(8, true))
}

func TestRestrictedUserEntitlementSourceCanUseExclusiveGroup(t *testing.T) {
	t.Parallel()

	user := &User{RestrictToAllowedGroups: true}
	group := &Group{ID: 9, IsExclusive: true, Status: StatusActive, SubscriptionEnabled: true}
	entitlements := []AvailableAPIKeyGroupEntitlement{{ID: 21, Name: "Dedicated plan"}}

	sources := buildAvailableGroupAccessSources(user, group, entitlements)
	require.Len(t, sources, 1)
	require.Equal(t, APIKeyAccessSourceEntitlement, sources[0].Type)
	require.Equal(t, int64(21), *sources[0].EntitlementID)
}

func TestRestrictedUserStillSeesPublicGroupAsDisabled(t *testing.T) {
	t.Parallel()

	user := &User{RestrictToAllowedGroups: true}
	reason, unavailable := groupUnavailableByUserPolicy(user, &Group{ID: 3, IsExclusive: false})
	require.True(t, unavailable)
	require.Equal(t, "EXCLUSIVE_GROUPS_ONLY", reason)

	_, unavailable = groupUnavailableByUserPolicy(user, &Group{ID: 4, IsExclusive: true})
	require.False(t, unavailable)
}

func TestAPIKeyResolvedGroupPolicyRechecksRuntimeFallbacks(t *testing.T) {
	t.Parallel()

	user := &User{RestrictToAllowedGroups: true, AllowedGroups: []int64{7}}
	key := &APIKey{User: user, AccessSource: APIKeyAccessSourceBalance}
	ctx := WithAPIKeyGroupAccessPolicy(context.Background(), key, nil, nil)

	require.True(t, IsAPIKeyResolvedGroupAllowed(ctx, &Group{ID: 7, IsExclusive: true}))
	require.False(t, IsAPIKeyResolvedGroupAllowed(ctx, &Group{ID: 8, IsExclusive: true}))
	require.False(t, IsAPIKeyResolvedGroupAllowed(ctx, &Group{ID: 7, IsExclusive: false}))
	require.True(t, IsAPIKeyResolvedGroupAllowed(context.Background(), &Group{ID: 8}), "internal callers without API-key policy retain legacy behavior")
}

func TestAPIKeyResolvedGroupPolicyUsesEntitlementOrLegacySubscriptionCoverage(t *testing.T) {
	t.Parallel()

	user := &User{RestrictToAllowedGroups: true}
	key := &APIKey{User: user, AccessSource: APIKeyAccessSourceEntitlement}
	entitlement := &SubscriptionEntitlement{GroupGrants: []SubscriptionEntitlementGroupGrant{{GroupID: 9, Enabled: true}}}
	entitlementCtx := WithAPIKeyGroupAccessPolicy(context.Background(), key, entitlement, nil)
	require.True(t, IsAPIKeyResolvedGroupAllowed(entitlementCtx, &Group{ID: 9, IsExclusive: true}))
	require.False(t, IsAPIKeyResolvedGroupAllowed(entitlementCtx, &Group{ID: 10, IsExclusive: true}))

	legacyKey := &APIKey{User: user, AccessSource: APIKeyAccessSourceBalance}
	legacyCtx := WithAPIKeyGroupAccessPolicy(context.Background(), legacyKey, nil, &UserSubscription{GroupID: 11})
	require.True(t, IsAPIKeyResolvedGroupAllowed(legacyCtx, &Group{ID: 11, IsExclusive: true}))
	require.False(t, IsAPIKeyResolvedGroupAllowed(legacyCtx, &Group{ID: 12, IsExclusive: true}))
}

func TestPublicGroupRestrictionCannotBeBypassedByEntitlementCoverage(t *testing.T) {
	t.Parallel()

	user := &User{RestrictPublicGroups: true, AllowedGroups: []int64{7}}
	key := &APIKey{User: user, AccessSource: APIKeyAccessSourceEntitlement}
	entitlement := &SubscriptionEntitlement{GroupGrants: []SubscriptionEntitlementGroupGrant{{GroupID: 8, Enabled: true}}}
	ctx := WithAPIKeyGroupAccessPolicy(context.Background(), key, entitlement, nil)

	require.False(t, IsAPIKeyResolvedGroupAllowed(ctx, &Group{ID: 8, IsExclusive: false}))
	require.Empty(t, buildAvailableGroupAccessSources(user, &Group{ID: 8, Status: StatusActive, SubscriptionEnabled: true}, []AvailableAPIKeyGroupEntitlement{{ID: 21}}))
}

func TestEntitlementEnabledGrantGroupIDsIgnoresDisabledAndUnfilteredEdges(t *testing.T) {
	t.Parallel()

	entitlement := &SubscriptionEntitlement{
		GroupGrants: []SubscriptionEntitlementGroupGrant{
			{GroupID: 7, Enabled: true},
			{GroupID: 8, Enabled: false},
			{GroupID: 7, Enabled: true},
		},
		Groups: []Group{{ID: 8}, {ID: 9}},
	}

	require.Equal(t, []int64{7}, entitlementEnabledGrantGroupIDs(entitlement))
	require.Equal(t, []int64{7}, entitlementCoveredGroupIDs(entitlement), "configured grants must prevent the unfiltered Groups edge from restoring disabled access")

	filtered := &SubscriptionEntitlement{
		GroupGrantsConfigured: true,
		Groups:                []Group{{ID: 8}},
	}
	require.Empty(t, entitlementCoveredGroupIDs(filtered))
}

func TestRestrictedUserCannotCreateAPIKeyWithoutGroup(t *testing.T) {
	t.Parallel()

	svc := &APIKeyService{userRepo: &userRepoStub{user: &User{ID: 5, RestrictToAllowedGroups: true}}}
	_, err := svc.Create(context.Background(), 5, CreateAPIKeyRequest{Name: "no group"})
	require.ErrorIs(t, err, ErrGroupNotAllowed)
}

func TestRestrictedUserCannotClearAPIKeyGroup(t *testing.T) {
	t.Parallel()

	groupID := int64(7)
	repo := &updateFieldsAPIKeyRepoStub{key: &APIKey{
		ID:           1,
		UserID:       5,
		Key:          "sk-restricted",
		GroupID:      &groupID,
		AccessSource: APIKeyAccessSourceBalance,
		Status:       StatusActive,
	}}
	svc := &APIKeyService{
		apiKeyRepo: repo,
		userRepo:   &userRepoStub{user: &User{ID: 5, RestrictToAllowedGroups: true}},
	}

	_, err := svc.Update(context.Background(), 1, 5, UpdateAPIKeyRequest{ClearGroup: true})
	require.ErrorIs(t, err, ErrGroupNotAllowed)
	require.Empty(t, repo.updateFields)
}

func TestGetUserGroupAccessPolicyIncludesActiveSubscriptionGroups(t *testing.T) {
	t.Parallel()

	svc := &APIKeyService{
		userRepo: &userRepoStub{user: &User{
			ID:                      5,
			AllowedGroups:           []int64{7},
			RestrictToAllowedGroups: true,
			RestrictPublicGroups:    true,
		}},
		userSubRepo: &autoSwitchUserSubRepoStub{list: []UserSubscription{{
			ID:      12,
			UserID:  5,
			GroupID: 9,
		}}},
	}

	allowed, restricted, err := svc.GetUserGroupAccessPolicy(context.Background(), 5)
	require.NoError(t, err)
	require.True(t, restricted)
	require.Contains(t, allowed, int64(7))
	require.Contains(t, allowed, int64(9))

	explicit, restrictPublic, err := svc.GetUserGroupVisibility(context.Background(), 5)
	require.NoError(t, err)
	require.True(t, restrictPublic)
	require.Contains(t, explicit, int64(7))
	require.NotContains(t, explicit, int64(9), "subscription coverage must not widen the explicit public-group allowlist")
}

func TestValidateUserPaymentAccessRejectsDisabledUser(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateUserPaymentAccess(&User{Status: StatusActive}))

	err := validateUserPaymentAccess(&User{Status: StatusActive, PaymentDisabled: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "USER_PAYMENT_DISABLED")
}
