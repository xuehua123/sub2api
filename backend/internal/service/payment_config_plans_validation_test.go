//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionentitlement"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionentitlementgroup"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplangroup"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestPaymentConfigPlanValidation(t *testing.T) {
	t.Parallel()

	valid := CreatePlanRequest{
		Name:         "Pro",
		GroupIDs:     []int64{1},
		Price:        9.99,
		ValidityDays: 30,
		ValidityUnit: "day",
	}

	tests := []struct {
		name    string
		mutate  func(*CreatePlanRequest)
		wantErr string
	}{
		{name: "valid"},
		{name: "empty name", mutate: func(req *CreatePlanRequest) { req.Name = "" }, wantErr: "plan name"},
		{name: "zero price", mutate: func(req *CreatePlanRequest) { req.Price = 0 }, wantErr: "price"},
		{name: "zero validity", mutate: func(req *CreatePlanRequest) { req.ValidityDays = 0 }, wantErr: "validity days"},
		{name: "effective validity above maximum", mutate: func(req *CreatePlanRequest) {
			req.ValidityDays = 101
			req.ValidityUnit = "year"
		}, wantErr: "must not exceed 36500 days"},
		{name: "invalid validity unit", mutate: func(req *CreatePlanRequest) { req.ValidityUnit = "wek" }, wantErr: "valid validity unit"},
		{name: "negative original price", mutate: func(req *CreatePlanRequest) { v := -1.0; req.OriginalPrice = &v }, wantErr: "original price"},
		{name: "zero original price", mutate: func(req *CreatePlanRequest) { v := 0.0; req.OriginalPrice = &v }},
		{name: "zero limit", mutate: func(req *CreatePlanRequest) { v := 0.0; req.DailyLimitUSD = &v }, wantErr: "limit"},
		{name: "weekly limit shorter than week", mutate: func(req *CreatePlanRequest) {
			v := 100.0
			req.ValidityDays = 1
			req.ValidityUnit = "day"
			req.WeeklyLimitUSD = &v
		}, wantErr: "7 days"},
		{name: "monthly limit shorter than month", mutate: func(req *CreatePlanRequest) {
			v := 100.0
			req.ValidityDays = 7
			req.ValidityUnit = "day"
			req.MonthlyLimitUSD = &v
		}, wantErr: "30 days"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := valid
			if tt.mutate != nil {
				tt.mutate(&req)
			}
			err := validatePlanRequired(req)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestPaymentConfigPlanPatchValidation(t *testing.T) {
	t.Parallel()

	neg := -5.0
	zero := 0.0
	validPrice := 9.99
	validLimit := OptionalFloat64{Set: true, Value: &validPrice}

	tests := []struct {
		name    string
		req     UpdatePlanRequest
		wantErr string
	}{
		{name: "all nil", req: UpdatePlanRequest{}},
		{name: "empty name", req: UpdatePlanRequest{Name: pcPlanStrPtr("")}, wantErr: "plan name"},
		{name: "negative original price", req: UpdatePlanRequest{OriginalPrice: OptionalFloat64{Set: true, Value: &neg}}, wantErr: "original price"},
		{name: "zero group id", req: UpdatePlanRequest{GroupID: pcPlanInt64Ptr(0)}, wantErr: "group"},
		{name: "negative price", req: UpdatePlanRequest{Price: &neg}, wantErr: "price"},
		{name: "zero validity days", req: UpdatePlanRequest{ValidityDays: pcPlanIntPtr(0)}, wantErr: "validity days"},
		{name: "invalid validity unit", req: UpdatePlanRequest{ValidityUnit: pcPlanStrPtr("wek")}, wantErr: "valid validity unit"},
		{name: "clear limit", req: UpdatePlanRequest{DailyLimitUSD: OptionalFloat64{Set: true, Value: nil}}},
		{name: "zero limit", req: UpdatePlanRequest{WeeklyLimitUSD: OptionalFloat64{Set: true, Value: &zero}}, wantErr: "limit"},
		{name: "valid limit", req: UpdatePlanRequest{MonthlyLimitUSD: validLimit}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validatePlanPatch(tt.req)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateEffectivePlanValidityRange(t *testing.T) {
	t.Parallel()

	existing := &dbent.SubscriptionPlan{ValidityDays: 100, ValidityUnit: "day"}
	years := "year"
	err := validateEffectivePlanLimitPeriods(existing, UpdatePlanRequest{ValidityUnit: &years})
	require.NoError(t, err)

	oneHundredOne := 101
	err = validateEffectivePlanLimitPeriods(existing, UpdatePlanRequest{ValidityDays: &oneHundredOne, ValidityUnit: &years})
	require.Error(t, err)
	require.Equal(t, "PLAN_VALIDITY_TOO_LONG", infraerrors.Reason(err))
}

func TestValidatePlanValidityRangeExactMaxAcrossUnits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		validityDays int
		validityUnit string
		wantErr      bool
	}{
		{name: "36500 days", validityDays: 36500, validityUnit: "days"},
		{name: "36501 days", validityDays: 36501, validityUnit: "day", wantErr: true},
		{name: "5214 weeks", validityDays: 5214, validityUnit: "weeks"},
		{name: "5215 weeks", validityDays: 5215, validityUnit: "week", wantErr: true},
		{name: "1216 months", validityDays: 1216, validityUnit: "months"},
		{name: "1217 months", validityDays: 1217, validityUnit: "month", wantErr: true},
		{name: "100 years", validityDays: 100, validityUnit: "years"},
		{name: "101 years", validityDays: 101, validityUnit: "year", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validatePlanValidityRange(tt.validityDays, tt.validityUnit)
			if tt.wantErr {
				require.Error(t, err)
				require.Equal(t, "PLAN_VALIDITY_TOO_LONG", infraerrors.Reason(err))
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPaymentConfigCreatePlanPersistsExplicitGroupsAndLimits(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client}
	anthropic := createPaymentConfigPlanTestGroup(t, client, "anthropic-sub", PlatformAnthropic, 20)
	gemini := createPaymentConfigPlanTestGroup(t, client, "gemini-sub", PlatformGemini, 10)
	daily := 1.25
	weekly := 5.5

	resp, err := svc.CreatePlan(ctx, CreatePlanRequest{
		Name:           "Multi Group",
		GroupIDs:       []int64{gemini.ID, anthropic.ID, gemini.ID},
		AccessScope:    PlanAccessScopeExplicit,
		Price:          19.99,
		ValidityDays:   30,
		ValidityUnit:   "days",
		DailyLimitUSD:  &daily,
		WeeklyLimitUSD: &weekly,
		OveragePolicy:  SubscriptionEntitlementOverageBalanceFallback,
		ForSale:        true,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, PlanAccessScopeExplicit, resp.AccessScope)
	require.Equal(t, []int64{gemini.ID, anthropic.ID}, resp.GroupIDs)
	require.Equal(t, gemini.ID, resp.GroupID)
	require.Equal(t, SubscriptionEntitlementOverageBalanceFallback, resp.OveragePolicy)
	require.NotNil(t, resp.DailyLimitUSD)
	require.InDelta(t, daily, *resp.DailyLimitUSD, 0.000001)
	require.NotNil(t, resp.WeeklyLimitUSD)
	require.InDelta(t, weekly, *resp.WeeklyLimitUSD, 0.000001)

	rows, err := client.SubscriptionPlanGroup.Query().
		Where(subscriptionplangroup.PlanIDEQ(resp.ID)).
		Order(subscriptionplangroup.BySortOrder()).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, gemini.ID, rows[0].GroupID)
	require.Equal(t, 0, rows[0].SortOrder)
	require.Equal(t, anthropic.ID, rows[1].GroupID)
	require.Equal(t, 1, rows[1].SortOrder)
}

func TestPaymentConfigUpdatePlanPatchPreservesReplacesAndClearsGroups(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client}
	anthropic := createPaymentConfigPlanTestGroup(t, client, "anthropic-sub", PlatformAnthropic, 20)
	gemini := createPaymentConfigPlanTestGroup(t, client, "gemini-sub", PlatformGemini, 10)

	created, err := svc.CreatePlan(ctx, CreatePlanRequest{
		Name:         "Patch Groups",
		GroupIDs:     []int64{anthropic.ID, gemini.ID},
		Price:        9.99,
		ValidityDays: 30,
		ValidityUnit: "day",
		ForSale:      true,
	})
	require.NoError(t, err)

	price := 10.99
	preserved, err := svc.UpdatePlan(ctx, created.ID, UpdatePlanRequest{Price: &price})
	require.NoError(t, err)
	require.Equal(t, []int64{anthropic.ID, gemini.ID}, preserved.GroupIDs)

	replaced, err := svc.UpdatePlan(ctx, created.ID, UpdatePlanRequest{GroupIDs: []int64{gemini.ID}})
	require.NoError(t, err)
	require.Equal(t, []int64{gemini.ID}, replaced.GroupIDs)
	require.Equal(t, gemini.ID, replaced.GroupID)

	legacyReplaced, err := svc.UpdatePlan(ctx, created.ID, UpdatePlanRequest{GroupID: pcPlanInt64Ptr(anthropic.ID)})
	require.NoError(t, err)
	require.Equal(t, []int64{anthropic.ID}, legacyReplaced.GroupIDs)
	require.Equal(t, anthropic.ID, legacyReplaced.GroupID)

	allScope := PlanAccessScopeAllSubscriptionGroups
	cleared, err := svc.UpdatePlan(ctx, created.ID, UpdatePlanRequest{
		AccessScope: &allScope,
		GroupIDs:    []int64{},
	})
	require.NoError(t, err)
	require.Equal(t, PlanAccessScopeAllSubscriptionGroups, cleared.AccessScope)
	require.ElementsMatch(t, []int64{anthropic.ID, gemini.ID}, cleared.GroupIDs)

	rows, err := client.SubscriptionPlanGroup.Query().Where(subscriptionplangroup.PlanIDEQ(created.ID)).All(ctx)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestPaymentConfigPlanScopeValidation(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client}
	anthropic := createPaymentConfigPlanTestGroup(t, client, "anthropic-sub", PlatformAnthropic, 20)
	gemini := createPaymentConfigPlanTestGroup(t, client, "gemini-sub", PlatformGemini, 10)

	_, err := svc.CreatePlan(ctx, CreatePlanRequest{
		Name:         "Missing Groups",
		AccessScope:  PlanAccessScopeExplicit,
		Price:        9.99,
		ValidityDays: 30,
		ValidityUnit: "day",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "group_ids")

	_, err = svc.CreatePlan(ctx, CreatePlanRequest{
		Name:         "Missing Platforms",
		AccessScope:  PlanAccessScopePlatformSubscriptionGroups,
		Price:        9.99,
		ValidityDays: 30,
		ValidityUnit: "day",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "allowed_platforms")

	_, err = svc.CreatePlan(ctx, CreatePlanRequest{
		Name:             "Platform With Explicit Groups",
		AccessScope:      PlanAccessScopePlatformSubscriptionGroups,
		GroupIDs:         []int64{anthropic.ID},
		AllowedPlatforms: []string{PlatformAnthropic},
		Price:            9.99,
		ValidityDays:     30,
		ValidityUnit:     "day",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "group_ids")

	resp, err := svc.CreatePlan(ctx, CreatePlanRequest{
		Name:             "Anthropic Platform",
		AccessScope:      PlanAccessScopePlatformSubscriptionGroups,
		AllowedPlatforms: []string{PlatformAnthropic, PlatformAnthropic},
		Price:            9.99,
		ValidityDays:     30,
		ValidityUnit:     "day",
	})
	require.NoError(t, err)
	require.Equal(t, []string{PlatformAnthropic}, resp.AllowedPlatforms)
	require.Equal(t, []int64{anthropic.ID}, resp.GroupIDs)
	require.Equal(t, anthropic.ID, resp.GroupID)

	allResp, err := svc.CreatePlan(ctx, CreatePlanRequest{
		Name:         "All Groups",
		AccessScope:  PlanAccessScopeAllSubscriptionGroups,
		Price:        12.99,
		ValidityDays: 30,
		ValidityUnit: "day",
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{anthropic.ID, gemini.ID}, allResp.GroupIDs)

	allScope := PlanAccessScopeAllSubscriptionGroups
	_, err = svc.UpdatePlan(ctx, resp.ID, UpdatePlanRequest{
		AccessScope: &allScope,
		GroupID:     pcPlanInt64Ptr(anthropic.ID),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "group_ids")
}

func TestPaymentConfigPlanOverageAndNullableLimitUpdate(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client}
	group := createPaymentConfigPlanTestGroup(t, client, "anthropic-sub", PlatformAnthropic, 0)
	daily := 3.0

	_, err := svc.CreatePlan(ctx, CreatePlanRequest{
		Name:          "Bad Overage",
		GroupIDs:      []int64{group.ID},
		Price:         9.99,
		ValidityDays:  30,
		ValidityUnit:  "day",
		OveragePolicy: "charge_card",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "overage")

	created, err := svc.CreatePlan(ctx, CreatePlanRequest{
		Name:          "Nullable Limit",
		GroupIDs:      []int64{group.ID},
		Price:         9.99,
		ValidityDays:  30,
		ValidityUnit:  "day",
		DailyLimitUSD: &daily,
	})
	require.NoError(t, err)
	require.NotNil(t, created.DailyLimitUSD)

	updated, err := svc.UpdatePlan(ctx, created.ID, UpdatePlanRequest{
		DailyLimitUSD: OptionalFloat64{Set: true, Value: nil},
	})
	require.NoError(t, err)
	require.Nil(t, updated.DailyLimitUSD)
}

func TestPaymentConfigUpdatePlanSyncsActiveEntitlementLimits(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client}
	group := createPaymentConfigPlanTestGroup(t, client, "anthropic-sub", PlatformAnthropic, 0)

	created, err := svc.CreatePlan(ctx, CreatePlanRequest{
		Name:         "Template",
		GroupIDs:     []int64{group.ID},
		Price:        9.99,
		ValidityDays: 30,
		ValidityUnit: "day",
	})
	require.NoError(t, err)

	user, err := client.User.Create().
		SetEmail("plan-sync@example.test").
		SetPasswordHash("hash").
		Save(ctx)
	require.NoError(t, err)
	now := time.Now()
	ent, err := client.SubscriptionEntitlement.Create().
		SetUserID(user.ID).
		SetPlanID(created.ID).
		SetPrimaryGroupID(group.ID).
		SetName(created.Name).
		SetSourceType(SubscriptionEntitlementSourceAdminAssign).
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(now.Add(30 * 24 * time.Hour)).
		SetDailyWindowStart(now.Add(-time.Hour)).
		SetWeeklyWindowStart(now.Add(-time.Hour)).
		SetMonthlyWindowStart(now.Add(-time.Hour)).
		SetOveragePolicy(SubscriptionEntitlementOverageBlock).
		SetPlanSnapshot(map[string]any{"price": created.Price}).
		Save(ctx)
	require.NoError(t, err)

	monthly := 1000.0
	overagePolicy := SubscriptionEntitlementOverageBalanceFallback
	updated, err := svc.UpdatePlan(ctx, created.ID, UpdatePlanRequest{
		Name:            pcPlanStrPtr("Template Updated"),
		MonthlyLimitUSD: OptionalFloat64{Set: true, Value: &monthly},
		OveragePolicy:   &overagePolicy,
	})
	require.NoError(t, err)
	require.NotNil(t, updated.MonthlyLimitUSD)
	require.Equal(t, monthly, *updated.MonthlyLimitUSD)

	got, err := client.SubscriptionEntitlement.Query().
		Where(subscriptionentitlement.IDEQ(ent.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, "Template Updated", got.Name)
	require.NotNil(t, got.MonthlyLimitUsd)
	require.Equal(t, monthly, *got.MonthlyLimitUsd)
	require.Equal(t, SubscriptionEntitlementOverageBalanceFallback, got.OveragePolicy)
}

func TestPaymentConfigUpdatePlanSyncsActiveEntitlementGroups(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client}
	oldGroup := createPaymentConfigPlanTestGroup(t, client, "old-sub", PlatformOpenAI, 0)
	newGroup := createPaymentConfigPlanTestGroup(t, client, "new-sub", PlatformOpenAI, 1)

	created, err := svc.CreatePlan(ctx, CreatePlanRequest{
		Name:         "Group Sync",
		GroupIDs:     []int64{oldGroup.ID},
		Price:        9.99,
		ValidityDays: 30,
		ValidityUnit: "day",
	})
	require.NoError(t, err)

	user, err := client.User.Create().
		SetEmail("plan-group-sync@example.test").
		SetPasswordHash("hash").
		Save(ctx)
	require.NoError(t, err)
	now := time.Now()
	activeEnt, err := client.SubscriptionEntitlement.Create().
		SetUserID(user.ID).
		SetPlanID(created.ID).
		SetPrimaryGroupID(oldGroup.ID).
		SetName(created.Name).
		SetSourceType(SubscriptionEntitlementSourceAdminAssign).
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(now.Add(30 * 24 * time.Hour)).
		SetDailyWindowStart(now.Add(-time.Hour)).
		SetWeeklyWindowStart(now.Add(-time.Hour)).
		SetMonthlyWindowStart(now.Add(-time.Hour)).
		SetOveragePolicy(SubscriptionEntitlementOverageBlock).
		SetPlanSnapshot(map[string]any{"price": created.Price}).
		Save(ctx)
	require.NoError(t, err)
	expiredEnt, err := client.SubscriptionEntitlement.Create().
		SetUserID(user.ID).
		SetPlanID(created.ID).
		SetPrimaryGroupID(oldGroup.ID).
		SetName(created.Name).
		SetSourceType(SubscriptionEntitlementSourceAdminAssign).
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(now.Add(-60 * 24 * time.Hour)).
		SetExpiresAt(now.Add(-24 * time.Hour)).
		SetDailyWindowStart(now.Add(-60 * 24 * time.Hour)).
		SetWeeklyWindowStart(now.Add(-60 * 24 * time.Hour)).
		SetMonthlyWindowStart(now.Add(-60 * 24 * time.Hour)).
		SetOveragePolicy(SubscriptionEntitlementOverageBlock).
		SetPlanSnapshot(map[string]any{"price": created.Price}).
		Save(ctx)
	require.NoError(t, err)
	for _, entitlementID := range []int64{activeEnt.ID, expiredEnt.ID} {
		_, err = client.SubscriptionEntitlementGroup.Create().
			SetEntitlementID(entitlementID).
			SetGroupID(oldGroup.ID).
			SetSortOrder(0).
			SetEnabled(true).
			Save(ctx)
		require.NoError(t, err)
	}

	updated, err := svc.UpdatePlan(ctx, created.ID, UpdatePlanRequest{GroupIDs: []int64{newGroup.ID}})
	require.NoError(t, err)
	require.Equal(t, []int64{newGroup.ID}, updated.GroupIDs)

	activeGroups, err := client.SubscriptionEntitlementGroup.Query().
		Where(subscriptionentitlementgroup.EntitlementIDEQ(activeEnt.ID)).
		Order(subscriptionentitlementgroup.BySortOrder()).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, activeGroups, 1)
	require.Equal(t, newGroup.ID, activeGroups[0].GroupID)

	refreshedActiveEnt, err := client.SubscriptionEntitlement.Get(ctx, activeEnt.ID)
	require.NoError(t, err)
	require.NotNil(t, refreshedActiveEnt.PrimaryGroupID)
	require.Equal(t, newGroup.ID, *refreshedActiveEnt.PrimaryGroupID)

	expiredGroups, err := client.SubscriptionEntitlementGroup.Query().
		Where(subscriptionentitlementgroup.EntitlementIDEQ(expiredEnt.ID)).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, expiredGroups, 1)
	require.Equal(t, oldGroup.ID, expiredGroups[0].GroupID)
}

func TestPlanAutoGrantSyncPlatformsForGroupChangeRules(t *testing.T) {
	activeAuto := &Group{
		ID:                   11,
		Platform:             PlatformOpenAI,
		Status:               StatusActive,
		SubscriptionEnabled:  true,
		PlanAutoGrantEnabled: true,
	}
	activeManual := &Group{
		ID:                   activeAuto.ID,
		Platform:             PlatformOpenAI,
		Status:               StatusActive,
		SubscriptionEnabled:  true,
		PlanAutoGrantEnabled: false,
	}
	disabled := *activeAuto
	disabled.Status = StatusDisabled
	disabledManual := *activeManual
	disabledManual.Status = StatusDisabled

	require.Equal(t, []string{PlatformOpenAI}, planAutoGrantSyncPlatformsForGroupChange(activeManual, activeAuto))
	require.Equal(t, []string{PlatformOpenAI}, planAutoGrantSyncPlatformsForGroupChange(activeAuto, activeAuto), "saving an already auto-granted group should re-sync current dynamic scopes")
	require.Equal(t, []string{PlatformOpenAI}, planAutoGrantSyncPlatformsForGroupChange(activeAuto, activeManual), "unchecking auto-grant must remove existing entitlement scopes")
	require.Equal(t, []string{PlatformOpenAI}, planAutoGrantSyncPlatformsForGroupChange(activeAuto, &disabled), "disabling the group should remove it from dynamic scopes")
	require.Equal(t, []string{PlatformOpenAI}, planAutoGrantSyncPlatformsForGroupChange(activeManual, &disabledManual), "disabling a non-auto-grant group must retain the upstream cleanup trigger")

	addID, removeID := explicitPlanAutoGrantGroupChangeIDs(activeAuto, activeManual)
	require.Zero(t, addID)
	require.Zero(t, removeID, "capability-only changes must not delete an administrator's explicit plan membership")

	_, removeID = explicitPlanAutoGrantGroupChangeIDs(activeAuto, &disabled)
	require.Equal(t, activeAuto.ID, removeID)
}

func TestDynamicPlanAutoGrantGroupSyncAddsNewGroupToActiveEntitlements(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	oldGroup := createPaymentConfigPlanTestGroup(t, client, "old-auto-openai", PlatformOpenAI, 0)

	svc := &PaymentConfigService{entClient: client}
	created, err := svc.CreatePlan(ctx, CreatePlanRequest{
		Name:         "Dynamic All",
		AccessScope:  PlanAccessScopeAllSubscriptionGroups,
		Price:        9.99,
		ValidityDays: 30,
		ValidityUnit: "day",
	})
	require.NoError(t, err)
	_, err = client.SubscriptionPlan.UpdateOneID(created.ID).
		SetForSale(false).
		Save(ctx)
	require.NoError(t, err)

	user, err := client.User.Create().
		SetEmail("dynamic-auto-grant@example.test").
		SetPasswordHash("hash").
		Save(ctx)
	require.NoError(t, err)
	now := time.Now()
	activeEnt, err := client.SubscriptionEntitlement.Create().
		SetUserID(user.ID).
		SetPlanID(created.ID).
		SetPrimaryGroupID(oldGroup.ID).
		SetName(created.Name).
		SetSourceType(SubscriptionEntitlementSourceAdminAssign).
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(now.Add(30 * 24 * time.Hour)).
		SetDailyWindowStart(now.Add(-time.Hour)).
		SetWeeklyWindowStart(now.Add(-time.Hour)).
		SetMonthlyWindowStart(now.Add(-time.Hour)).
		SetOveragePolicy(SubscriptionEntitlementOverageBlock).
		SetPlanSnapshot(map[string]any{"price": created.Price}).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.SubscriptionEntitlementGroup.Create().
		SetEntitlementID(activeEnt.ID).
		SetGroupID(oldGroup.ID).
		SetSortOrder(0).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	serviceGroup := func(g *dbent.Group, status string) *Group {
		return &Group{
			ID:                   g.ID,
			Platform:             g.Platform,
			Status:               status,
			SubscriptionEnabled:  g.SubscriptionEnabled,
			PlanAutoGrantEnabled: g.PlanAutoGrantEnabled,
			IsExclusive:          g.IsExclusive,
		}
	}
	newGroup := createPaymentConfigPlanTestGroup(t, client, "new-auto-openai", PlatformOpenAI, 1)
	require.NoError(t, syncDynamicPlanAutoGrantScopesForGroupChange(ctx, client, nil, serviceGroup(newGroup, StatusActive)))

	activeGroups, err := client.SubscriptionEntitlementGroup.Query().
		Where(subscriptionentitlementgroup.EntitlementIDEQ(activeEnt.ID)).
		Order(subscriptionentitlementgroup.BySortOrder()).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, activeGroups, 2)
	require.Equal(t, oldGroup.ID, activeGroups[0].GroupID)
	require.Equal(t, newGroup.ID, activeGroups[1].GroupID)

	_, err = client.Group.UpdateOneID(newGroup.ID).
		SetPlanAutoGrantEnabled(false).
		Save(ctx)
	require.NoError(t, err)
	manualNewGroup := serviceGroup(newGroup, StatusActive)
	manualNewGroup.PlanAutoGrantEnabled = false
	require.NoError(t, syncDynamicPlanAutoGrantScopesForGroupChange(ctx, client, serviceGroup(newGroup, StatusActive), manualNewGroup))

	activeGroups, err = client.SubscriptionEntitlementGroup.Query().
		Where(subscriptionentitlementgroup.EntitlementIDEQ(activeEnt.ID)).
		Order(subscriptionentitlementgroup.BySortOrder()).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, activeGroups, 1)
	require.Equal(t, oldGroup.ID, activeGroups[0].GroupID)

	_, err = client.Group.UpdateOneID(oldGroup.ID).
		SetStatus(StatusDisabled).
		Save(ctx)
	require.NoError(t, err)
	require.NoError(t, syncDynamicPlanAutoGrantScopesForGroupChange(ctx, client, serviceGroup(oldGroup, StatusActive), serviceGroup(oldGroup, StatusDisabled)))

	activeGroups, err = client.SubscriptionEntitlementGroup.Query().
		Where(subscriptionentitlementgroup.EntitlementIDEQ(activeEnt.ID)).
		All(ctx)
	require.NoError(t, err)
	require.Empty(t, activeGroups)
	refreshedActiveEnt, err := client.SubscriptionEntitlement.Get(ctx, activeEnt.ID)
	require.NoError(t, err)
	require.Nil(t, refreshedActiveEnt.PrimaryGroupID)
}

func TestExplicitPlanAutoGrantGroupSaveSyncsActiveEntitlements(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	oldGroup := createPaymentConfigPlanTestGroup(t, client, "explicit-old-auto", PlatformOpenAI, 0)
	newGroup := createPaymentConfigPlanTestGroup(t, client, "explicit-new-auto", PlatformAnthropic, 1)

	svc := &PaymentConfigService{entClient: client}
	created, err := svc.CreatePlan(ctx, CreatePlanRequest{
		Name:         "Explicit Auto",
		GroupIDs:     []int64{oldGroup.ID},
		Price:        9.99,
		ValidityDays: 30,
		ValidityUnit: "day",
	})
	require.NoError(t, err)

	user, err := client.User.Create().
		SetEmail("explicit-auto-grant@example.test").
		SetPasswordHash("hash").
		Save(ctx)
	require.NoError(t, err)
	now := time.Now()
	activeEnt, err := client.SubscriptionEntitlement.Create().
		SetUserID(user.ID).
		SetPlanID(created.ID).
		SetPrimaryGroupID(oldGroup.ID).
		SetName(created.Name).
		SetSourceType(SubscriptionEntitlementSourceAdminAssign).
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(now.Add(30 * 24 * time.Hour)).
		SetDailyWindowStart(now.Add(-time.Hour)).
		SetWeeklyWindowStart(now.Add(-time.Hour)).
		SetMonthlyWindowStart(now.Add(-time.Hour)).
		SetOveragePolicy(SubscriptionEntitlementOverageBlock).
		SetPlanSnapshot(map[string]any{"price": created.Price}).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.SubscriptionEntitlementGroup.Create().
		SetEntitlementID(activeEnt.ID).
		SetGroupID(oldGroup.ID).
		SetSortOrder(0).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	serviceGroup := func(g *dbent.Group, status string) *Group {
		return &Group{
			ID:                   g.ID,
			Platform:             g.Platform,
			Status:               status,
			SubscriptionEnabled:  g.SubscriptionEnabled,
			PlanAutoGrantEnabled: g.PlanAutoGrantEnabled,
			IsExclusive:          g.IsExclusive,
		}
	}
	require.NoError(t, syncDynamicPlanAutoGrantScopesForGroupChange(ctx, client, serviceGroup(newGroup, StatusActive), serviceGroup(newGroup, StatusActive)))

	planGroups, err := client.SubscriptionPlanGroup.Query().
		Where(subscriptionplangroup.PlanIDEQ(created.ID)).
		Order(subscriptionplangroup.BySortOrder()).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, planGroups, 2)
	require.Equal(t, oldGroup.ID, planGroups[0].GroupID)
	require.Equal(t, newGroup.ID, planGroups[1].GroupID)

	activeGroups, err := client.SubscriptionEntitlementGroup.Query().
		Where(subscriptionentitlementgroup.EntitlementIDEQ(activeEnt.ID)).
		Order(subscriptionentitlementgroup.BySortOrder()).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, activeGroups, 2)
	require.Equal(t, oldGroup.ID, activeGroups[0].GroupID)
	require.Equal(t, newGroup.ID, activeGroups[1].GroupID)

	_, err = client.Group.UpdateOneID(newGroup.ID).
		SetPlanAutoGrantEnabled(false).
		Save(ctx)
	require.NoError(t, err)
	manualNewGroup := serviceGroup(newGroup, StatusActive)
	manualNewGroup.PlanAutoGrantEnabled = false
	require.NoError(t, syncDynamicPlanAutoGrantScopesForGroupChange(ctx, client, serviceGroup(newGroup, StatusActive), manualNewGroup))

	planGroups, err = client.SubscriptionPlanGroup.Query().
		Where(subscriptionplangroup.PlanIDEQ(created.ID)).
		Order(subscriptionplangroup.BySortOrder()).
		All(ctx)
	require.NoError(t, err)
	// An explicit plan has no provenance marker for auto-added groups. Turning
	// off auto-grant must therefore preserve an administrator's explicit plan
	// membership rather than deleting a potentially manual selection.
	require.Len(t, planGroups, 2)
	require.Equal(t, oldGroup.ID, planGroups[0].GroupID)
	require.Equal(t, newGroup.ID, planGroups[1].GroupID)

	activeGroups, err = client.SubscriptionEntitlementGroup.Query().
		Where(subscriptionentitlementgroup.EntitlementIDEQ(activeEnt.ID)).
		Order(subscriptionentitlementgroup.BySortOrder()).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, activeGroups, 2)
	require.Equal(t, oldGroup.ID, activeGroups[0].GroupID)
	require.Equal(t, newGroup.ID, activeGroups[1].GroupID)
}

func TestPaymentConfigUpdatePlanRejectsLimitPeriodLongerThanValidity(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client}
	group := createPaymentConfigPlanTestGroup(t, client, "anthropic-sub", PlatformAnthropic, 0)
	weekly := 100.0

	created, err := svc.CreatePlan(ctx, CreatePlanRequest{
		Name:           "Weekly Plan",
		GroupIDs:       []int64{group.ID},
		Price:          9.99,
		ValidityDays:   7,
		ValidityUnit:   "day",
		WeeklyLimitUSD: &weekly,
	})
	require.NoError(t, err)

	oneDay := 1
	_, err = svc.UpdatePlan(ctx, created.ID, UpdatePlanRequest{ValidityDays: &oneDay})
	require.Error(t, err)
	require.Contains(t, err.Error(), "7 days")

	_, err = svc.UpdatePlan(ctx, created.ID, UpdatePlanRequest{
		ValidityDays:   &oneDay,
		WeeklyLimitUSD: OptionalFloat64{Set: true, Value: nil},
	})
	require.NoError(t, err)
}

func TestPaymentConfigUpdatePlanClearsOriginalPrice(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client}
	group := createPaymentConfigPlanTestGroup(t, client, "anthropic-sub", PlatformAnthropic, 0)
	originalPrice := 29.99

	created, err := svc.CreatePlan(ctx, CreatePlanRequest{
		Name:          "Clear Original Price",
		GroupIDs:      []int64{group.ID},
		Price:         9.99,
		OriginalPrice: &originalPrice,
		ValidityDays:  30,
		ValidityUnit:  "day",
	})
	require.NoError(t, err)
	require.NotNil(t, created.OriginalPrice)

	updated, err := svc.UpdatePlan(ctx, created.ID, UpdatePlanRequest{
		OriginalPrice: OptionalFloat64{Set: true, Value: nil},
	})
	require.NoError(t, err)
	require.Nil(t, updated.OriginalPrice)

	dbPlan, err := client.SubscriptionPlan.Get(ctx, created.ID)
	require.NoError(t, err)
	require.Nil(t, dbPlan.OriginalPrice)
}

func TestPaymentConfigUpdatePlanAllowsForSalePatchWhenExistingGroupIsDisabled(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client}
	group := createPaymentConfigPlanTestGroup(t, client, "disable-before-delist", PlatformOpenAI, 0)

	created, err := svc.CreatePlan(ctx, CreatePlanRequest{
		Name:         "Delist After Group Disabled",
		GroupIDs:     []int64{group.ID},
		Price:        9.99,
		ValidityDays: 30,
		ValidityUnit: "day",
		ForSale:      true,
	})
	require.NoError(t, err)

	_, err = client.Group.UpdateOneID(group.ID).
		SetSubscriptionEnabled(false).
		Save(ctx)
	require.NoError(t, err)

	forSale := false
	updated, err := svc.UpdatePlan(ctx, created.ID, UpdatePlanRequest{ForSale: &forSale})
	require.NoError(t, err)
	require.False(t, updated.ForSale)
	require.Empty(t, updated.GroupIDs)
	require.Empty(t, updated.Groups)

	plan, err := svc.GetPlan(ctx, created.ID)
	require.NoError(t, err)
	access, err := svc.ResolvePlanOrderAccess(ctx, plan)
	require.NoError(t, err)
	require.Equal(t, group.ID, access.PrimaryGroupID)
	require.Empty(t, access.GroupIDs)
	require.Empty(t, access.Groups)
}

func TestPaymentServiceValidateSubOrderRejectsPlanWithoutActiveGroups(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	configSvc := &PaymentConfigService{entClient: client}
	group := createPaymentConfigPlanTestGroup(t, client, "no-active-groups", PlatformOpenAI, 0)
	created, err := configSvc.CreatePlan(ctx, CreatePlanRequest{
		Name:         "No Active Groups",
		GroupIDs:     []int64{group.ID},
		Price:        9.99,
		ValidityDays: 30,
		ValidityUnit: "day",
		ForSale:      true,
	})
	require.NoError(t, err)

	_, err = client.Group.UpdateOneID(group.ID).SetSubscriptionEnabled(false).Save(ctx)
	require.NoError(t, err)

	paymentSvc := &PaymentService{configService: configSvc}
	_, _, err = paymentSvc.validateSubOrder(ctx, CreateOrderRequest{PlanID: created.ID})
	require.Error(t, err)
	require.Equal(t, "PLAN_NOT_AVAILABLE", infraerrors.Reason(err))
}

func TestPaymentConfigUpdatePlanToDynamicScopeClearsExplicitGroupRows(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client}
	oldGroup := createPaymentConfigPlanTestGroup(t, client, "legacy-explicit", PlatformOpenAI, 0)
	_, err := client.Group.UpdateOneID(oldGroup.ID).SetPlanAutoGrantEnabled(false).Save(ctx)
	require.NoError(t, err)
	newGroup := createPaymentConfigPlanTestGroup(t, client, "dynamic-target", PlatformAnthropic, 1)

	created, err := svc.CreatePlan(ctx, CreatePlanRequest{
		Name:         "Dynamic Scope",
		GroupIDs:     []int64{oldGroup.ID},
		Price:        9.99,
		ValidityDays: 30,
		ValidityUnit: "day",
	})
	require.NoError(t, err)

	scope := PlanAccessScopeAllSubscriptionGroups
	updated, err := svc.UpdatePlan(ctx, created.ID, UpdatePlanRequest{AccessScope: &scope})
	require.NoError(t, err)
	require.Equal(t, []int64{newGroup.ID}, updated.GroupIDs)

	persisted, err := client.SubscriptionPlanGroup.Query().
		Where(subscriptionplangroup.PlanIDEQ(created.ID)).
		All(ctx)
	require.NoError(t, err)
	require.Empty(t, persisted)
}

func createPaymentConfigPlanTestGroup(t *testing.T, client *dbent.Client, name, platform string, sortOrder int) *dbent.Group {
	t.Helper()
	group, err := client.Group.Create().
		SetName(name).
		SetPlatform(platform).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetSubscriptionEnabled(true).
		SetPlanAutoGrantEnabled(true).
		SetRateMultiplier(1).
		SetSortOrder(sortOrder).
		Save(context.Background())
	require.NoError(t, err)
	return group
}

func pcPlanStrPtr(value string) *string { return &value }

func pcPlanIntPtr(value int) *int { return &value }

func pcPlanInt64Ptr(value int64) *int64 { return &value }

func ptrFloat(value float64) *float64 { return &value }

// --- normalizePlanCurrency tests ---
// Empty must stay empty (not coerced to the default payment currency),
// so existing plans keep rendering without any currency label.

func TestNormalizePlanCurrency_EmptyKeepsEmpty(t *testing.T) {
	currency, err := normalizePlanCurrency("")
	require.NoError(t, err)
	require.Equal(t, "", currency)
}

func TestNormalizePlanCurrency_WhitespaceKeepsEmpty(t *testing.T) {
	currency, err := normalizePlanCurrency("   ")
	require.NoError(t, err)
	require.Equal(t, "", currency)
}

func TestNormalizePlanCurrency_LowercaseNormalized(t *testing.T) {
	currency, err := normalizePlanCurrency("nzd")
	require.NoError(t, err)
	require.Equal(t, "NZD", currency)
}

func TestNormalizePlanCurrency_ValidUppercase(t *testing.T) {
	currency, err := normalizePlanCurrency("USD")
	require.NoError(t, err)
	require.Equal(t, "USD", currency)
}

func TestNormalizePlanCurrency_TooShort(t *testing.T) {
	_, err := normalizePlanCurrency("NZ")
	require.Error(t, err)
	require.Contains(t, err.Error(), "currency")
}

func TestNormalizePlanCurrency_TooLong(t *testing.T) {
	_, err := normalizePlanCurrency("NZDD")
	require.Error(t, err)
	require.Contains(t, err.Error(), "currency")
}

func TestNormalizePlanCurrency_NonLetter(t *testing.T) {
	_, err := normalizePlanCurrency("N2D")
	require.Error(t, err)
	require.Contains(t, err.Error(), "currency")
}
