//go:build unit

package repository

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionEntitlementPlanRepository_AllScopeExpandsAutoGrantGroups(t *testing.T) {
	ctx := context.Background()
	_, client := newAPIKeyRepoSQLite(t)
	repo := NewSubscriptionEntitlementPlanRepository(client)

	openAI := mustCreateEntitlementPlanAutoGrantGroup(t, ctx, client, "openai-auto", service.PlatformOpenAI, service.StatusActive, false, true)
	gemini := mustCreateEntitlementPlanAutoGrantGroup(t, ctx, client, "gemini-auto", service.PlatformGemini, service.StatusActive, false, true)
	mustCreateEntitlementPlanAutoGrantGroup(t, ctx, client, "inactive-auto", service.PlatformOpenAI, service.StatusDisabled, false, true)
	mustCreateEntitlementPlanAutoGrantGroup(t, ctx, client, "exclusive-auto", service.PlatformOpenAI, service.StatusActive, true, true)
	mustCreateEntitlementPlanAutoGrantGroup(t, ctx, client, "manual-only", service.PlatformOpenAI, service.StatusActive, false, false)
	mustCreateEntitlementPlanAutoGrantGroup(t, ctx, client, "test-auto", service.PlatformOpenAI, service.StatusActive, false, true)

	plan := mustCreateEntitlementPlanRepoPlan(t, ctx, client, openAI.ID, service.PlanAccessScopeAllSubscriptionGroups, nil)

	got, err := repo.GetEntitlementPlan(ctx, plan.ID)

	require.NoError(t, err)
	require.Equal(t, []int64{openAI.ID, gemini.ID}, entitlementPlanGroupIDsForTest(got.Groups))
}

func TestSubscriptionEntitlementPlanRepository_PlatformScopeExpandsMatchingAutoGrantGroups(t *testing.T) {
	ctx := context.Background()
	_, client := newAPIKeyRepoSQLite(t)
	repo := NewSubscriptionEntitlementPlanRepository(client)

	openAI := mustCreateEntitlementPlanAutoGrantGroup(t, ctx, client, "openai-auto", service.PlatformOpenAI, service.StatusActive, false, true)
	mustCreateEntitlementPlanAutoGrantGroup(t, ctx, client, "gemini-auto", service.PlatformGemini, service.StatusActive, false, true)
	mustCreateEntitlementPlanAutoGrantGroup(t, ctx, client, "openai-exclusive-auto", service.PlatformOpenAI, service.StatusActive, true, true)
	mustCreateEntitlementPlanAutoGrantGroup(t, ctx, client, "openai-manual-only", service.PlatformOpenAI, service.StatusActive, false, false)

	plan := mustCreateEntitlementPlanRepoPlan(t, ctx, client, openAI.ID, service.PlanAccessScopePlatformSubscriptionGroups, []string{service.PlatformOpenAI})

	got, err := repo.GetEntitlementPlan(ctx, plan.ID)

	require.NoError(t, err)
	require.Equal(t, []int64{openAI.ID}, entitlementPlanGroupIDsForTest(got.Groups))
}

func mustCreateEntitlementPlanAutoGrantGroup(t *testing.T, ctx context.Context, client *dbent.Client, name, platform, status string, exclusive, autoGrant bool) *dbent.Group {
	t.Helper()
	group, err := client.Group.Create().
		SetName(name).
		SetPlatform(platform).
		SetStatus(status).
		SetSubscriptionType(service.SubscriptionTypeSubscription).
		SetSubscriptionEnabled(true).
		SetPlanAutoGrantEnabled(autoGrant).
		SetBalanceEnabled(false).
		SetIsExclusive(exclusive).
		SetRateMultiplier(1).
		Save(ctx)
	require.NoError(t, err)
	return group
}

func mustCreateEntitlementPlanRepoPlan(t *testing.T, ctx context.Context, client *dbent.Client, groupID int64, accessScope string, platforms []string) *dbent.SubscriptionPlan {
	t.Helper()
	create := client.SubscriptionPlan.Create().
		SetGroupID(groupID).
		SetName("auto grant plan").
		SetPrice(10).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetAccessScope(accessScope).
		SetOveragePolicy(service.SubscriptionEntitlementOverageBlock).
		SetForSale(true)
	if platforms != nil {
		create.SetAllowedPlatforms(platforms)
	}
	plan, err := create.Save(ctx)
	require.NoError(t, err)
	return plan
}

func entitlementPlanGroupIDsForTest(groups []service.SubscriptionEntitlementPlanGroup) []int64 {
	out := make([]int64, 0, len(groups))
	for _, group := range groups {
		out = append(out, group.GroupID)
	}
	return out
}
