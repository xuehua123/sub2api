//go:build unit

package service

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplangroup"
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
		{name: "invalid validity unit", mutate: func(req *CreatePlanRequest) { req.ValidityUnit = "wek" }, wantErr: "valid validity unit"},
		{name: "negative original price", mutate: func(req *CreatePlanRequest) { v := -1.0; req.OriginalPrice = &v }, wantErr: "original price"},
		{name: "zero original price", mutate: func(req *CreatePlanRequest) { v := 0.0; req.OriginalPrice = &v }},
		{name: "zero limit", mutate: func(req *CreatePlanRequest) { v := 0.0; req.DailyLimitUSD = &v }, wantErr: "limit"},
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
