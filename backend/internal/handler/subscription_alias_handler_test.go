//go:build unit

package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionentitlementgroup"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type subscriptionAliasRuntimeStub struct {
	enabled bool
}

func (s subscriptionAliasRuntimeStub) GetSubscriptionEntitlementsRuntime(context.Context) service.SubscriptionEntitlementsRuntime {
	return service.SubscriptionEntitlementsRuntime{Enabled: s.enabled}
}

type subscriptionAliasFixture struct {
	router *gin.Engine
	client *dbent.Client
	repo   service.SubscriptionEntitlementRepository
	userID int64
	now    time.Time
	groupA int64
	groupB int64
	planID int64
}

func TestSubscriptionHandler_V2OffListAndActiveKeepLegacyShape(t *testing.T) {
	fx := newSubscriptionAliasFixture(t, false)
	legacyID := mustCreateAliasLegacySubscription(t, fx.client, fx.userID, fx.groupA, fx.now, service.SubscriptionStatusActive)
	mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
		UserID:               fx.userID,
		LegacySubscriptionID: &legacyID,
		PrimaryGroupID:       &fx.groupB,
		Name:                 "ignored while flag off",
		Status:               service.SubscriptionStatusActive,
		StartsAt:             fx.now.Add(-time.Hour),
		ExpiresAt:            fx.now.Add(24 * time.Hour),
		OveragePolicy:        service.SubscriptionEntitlementOverageBalanceFallback,
	}, []int64{fx.groupB})

	for _, path := range []string{"/subscriptions", "/subscriptions/active"} {
		resp := performJSONRequest(fx.router, http.MethodGet, path, nil)

		require.Equal(t, http.StatusOK, resp.Code)
		rows := decodeSubscriptionAliasSlice(t, resp.Body.Bytes())
		require.Len(t, rows, 1)
		require.Equal(t, float64(legacyID), rows[0]["id"])
		require.Equal(t, float64(fx.groupA), rows[0]["group_id"])
		require.NotContains(t, rows[0], "entitlement_id")
		require.NotContains(t, rows[0], "plan_id")
		require.NotContains(t, rows[0], "plan_name")
		require.NotContains(t, rows[0], "groups")
		require.NotContains(t, rows[0], "overage_policy")
	}
}

func TestSubscriptionHandler_V2OnListReturnsLegacyLinkedEntitlementAliases(t *testing.T) {
	fx := newSubscriptionAliasFixture(t, true)
	legacyID := mustCreateAliasLegacySubscription(t, fx.client, fx.userID, fx.groupA, fx.now, service.SubscriptionStatusActive)
	windowStart := timezone.StartOfDay(fx.now)
	primaryGroupID := fx.groupB
	entitlementID := mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
		UserID:               fx.userID,
		PlanID:               &fx.planID,
		LegacySubscriptionID: &legacyID,
		PrimaryGroupID:       &primaryGroupID,
		Name:                 "Shared Pro",
		SourceType:           service.SubscriptionEntitlementSourcePaymentOrder,
		Status:               service.SubscriptionStatusActive,
		StartsAt:             fx.now.Add(-time.Hour),
		ExpiresAt:            fx.now.Add(24 * time.Hour),
		DailyWindowStart:     &windowStart,
		WeeklyWindowStart:    &windowStart,
		MonthlyWindowStart:   &windowStart,
		DailyLimitUSD:        ptr(10.0),
		WeeklyLimitUSD:       ptr(20.0),
		MonthlyLimitUSD:      ptr(30.0),
		DailyUsageUSD:        1.25,
		WeeklyUsageUSD:       2.5,
		MonthlyUsageUSD:      9.75,
		OveragePolicy:        service.SubscriptionEntitlementOverageBalanceFallback,
		SourceID:             ptr(int64(1234)),
		SourceExternalID:     ptr("external-secret"),
		Notes:                "internal note",
	}, []int64{fx.groupA, fx.groupB})
	mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
		UserID:        fx.userID,
		Name:          "entitlement only",
		Status:        service.SubscriptionStatusActive,
		StartsAt:      fx.now.Add(-time.Hour),
		ExpiresAt:     fx.now.Add(24 * time.Hour),
		OveragePolicy: service.SubscriptionEntitlementOverageBlock,
	}, []int64{fx.groupA})

	resp := performJSONRequest(fx.router, http.MethodGet, "/subscriptions", nil)

	require.Equal(t, http.StatusOK, resp.Code)
	require.NotContains(t, resp.Body.String(), "source_id")
	require.NotContains(t, resp.Body.String(), "source_external_id")
	require.NotContains(t, resp.Body.String(), "source_redeem_code_id")
	require.NotContains(t, resp.Body.String(), "notes")
	require.NotContains(t, resp.Body.String(), "fulfillment")
	rows := decodeSubscriptionAliasSlice(t, resp.Body.Bytes())
	require.Len(t, rows, 1)
	got := rows[0]
	require.Equal(t, float64(legacyID), got["id"])
	require.Equal(t, float64(fx.userID), got["user_id"])
	require.Equal(t, float64(primaryGroupID), got["group_id"])
	require.Equal(t, float64(entitlementID), got["entitlement_id"])
	require.Equal(t, float64(fx.planID), got["plan_id"])
	require.Equal(t, "Shared Pro", got["plan_name"])
	require.Equal(t, service.SubscriptionEntitlementOverageBalanceFallback, got["overage_policy"])
	require.Equal(t, 10.0, got["daily_limit_usd"])
	require.Equal(t, 20.0, got["weekly_limit_usd"])
	require.Equal(t, 30.0, got["monthly_limit_usd"])
	require.Equal(t, 1.25, got["daily_usage_usd"])
	require.Equal(t, 2.5, got["weekly_usage_usd"])
	require.Equal(t, 9.75, got["monthly_usage_usd"])
	group := got["group"].(map[string]any)
	require.Equal(t, float64(primaryGroupID), group["id"])
	require.Equal(t, 10.0, group["daily_limit_usd"])
	require.Equal(t, 20.0, group["weekly_limit_usd"])
	require.Equal(t, 30.0, group["monthly_limit_usd"])
	groups := got["groups"].([]any)
	require.Len(t, groups, 2)
	require.Equal(t, float64(fx.groupA), groups[0].(map[string]any)["id"])
	require.Equal(t, float64(fx.groupB), groups[1].(map[string]any)["id"])
}

func TestSubscriptionHandler_V2OnActiveFiltersEntitlementWindowAndStatus(t *testing.T) {
	fx := newSubscriptionAliasFixture(t, true)
	activeLegacyID := mustCreateAliasLegacySubscription(t, fx.client, fx.userID, fx.groupA, fx.now, service.SubscriptionStatusActive)
	activeID := mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
		UserID:               fx.userID,
		LegacySubscriptionID: &activeLegacyID,
		Name:                 "active",
		Status:               service.SubscriptionStatusActive,
		StartsAt:             fx.now.Add(-48 * time.Hour),
		ExpiresAt:            fx.now.Add(48 * time.Hour),
	}, []int64{fx.groupA})

	for _, item := range []struct {
		name      string
		status    string
		startsAt  time.Time
		expiresAt time.Time
		deleted   bool
	}{
		{name: "future", status: service.SubscriptionStatusActive, startsAt: fx.now.Add(time.Hour), expiresAt: fx.now.Add(2 * time.Hour)},
		{name: "expired", status: service.SubscriptionStatusActive, startsAt: fx.now.Add(-2 * time.Hour), expiresAt: fx.now.Add(-time.Hour)},
		{name: "suspended", status: service.SubscriptionStatusSuspended, startsAt: fx.now.Add(-time.Hour), expiresAt: fx.now.Add(time.Hour)},
		{name: "deleted", status: service.SubscriptionStatusActive, startsAt: fx.now.Add(-time.Hour), expiresAt: fx.now.Add(time.Hour), deleted: true},
	} {
		legacyID := mustCreateAliasLegacySubscription(t, fx.client, fx.userID, fx.groupA, fx.now, service.SubscriptionStatusActive)
		id := mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
			UserID:               fx.userID,
			LegacySubscriptionID: &legacyID,
			Name:                 item.name,
			Status:               item.status,
			StartsAt:             item.startsAt,
			ExpiresAt:            item.expiresAt,
		}, []int64{fx.groupA})
		if item.deleted {
			_, err := fx.client.SubscriptionEntitlement.UpdateOneID(id).SetDeletedAt(fx.now).Save(context.Background())
			require.NoError(t, err)
		}
	}

	resp := performJSONRequest(fx.router, http.MethodGet, "/subscriptions/active", nil)

	require.Equal(t, http.StatusOK, resp.Code)
	rows := decodeSubscriptionAliasSlice(t, resp.Body.Bytes())
	require.Len(t, rows, 1)
	require.Equal(t, float64(activeID), rows[0]["entitlement_id"])
}

func TestSubscriptionHandler_V2OnActiveAliasesUseLegacyUsageWhenHigher(t *testing.T) {
	fx := newSubscriptionAliasFixture(t, true)
	legacyID := mustCreateAliasLegacySubscription(t, fx.client, fx.userID, fx.groupA, fx.now, service.SubscriptionStatusActive)
	windowStart := timezone.StartOfDay(fx.now)
	mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
		UserID:               fx.userID,
		LegacySubscriptionID: &legacyID,
		Name:                 "legacy usage alias",
		Status:               service.SubscriptionStatusActive,
		StartsAt:             fx.now.Add(-time.Hour),
		ExpiresAt:            fx.now.Add(24 * time.Hour),
		DailyWindowStart:     &windowStart,
		WeeklyWindowStart:    &windowStart,
		MonthlyWindowStart:   &windowStart,
		DailyLimitUSD:        ptr(10.0),
		WeeklyLimitUSD:       ptr(20.0),
		MonthlyLimitUSD:      ptr(30.0),
		DailyUsageUSD:        0.1,
		WeeklyUsageUSD:       0.2,
		MonthlyUsageUSD:      0.3,
	}, []int64{fx.groupA})

	resp := performJSONRequest(fx.router, http.MethodGet, "/subscriptions/active", nil)

	require.Equal(t, http.StatusOK, resp.Code)
	rows := decodeSubscriptionAliasSlice(t, resp.Body.Bytes())
	require.Len(t, rows, 1)
	require.Equal(t, 10.0, rows[0]["daily_limit_usd"])
	require.Equal(t, 20.0, rows[0]["weekly_limit_usd"])
	require.Equal(t, 30.0, rows[0]["monthly_limit_usd"])
	require.Equal(t, 0.5, rows[0]["daily_usage_usd"])
	require.Equal(t, 1.5, rows[0]["weekly_usage_usd"])
	require.Equal(t, 2.5, rows[0]["monthly_usage_usd"])
}

func TestSubscriptionHandler_V2OnActiveAliasesIgnoreLegacyUsageFromUngraftedGroup(t *testing.T) {
	fx := newSubscriptionAliasFixture(t, true)
	legacyID := mustCreateAliasLegacySubscription(t, fx.client, fx.userID, fx.groupA, fx.now, service.SubscriptionStatusActive)
	windowStart := timezone.StartOfDay(fx.now)
	mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
		UserID:               fx.userID,
		LegacySubscriptionID: &legacyID,
		Name:                 "new plan scope",
		Status:               service.SubscriptionStatusActive,
		StartsAt:             fx.now.Add(-time.Hour),
		ExpiresAt:            fx.now.Add(24 * time.Hour),
		DailyWindowStart:     &windowStart,
		WeeklyWindowStart:    &windowStart,
		MonthlyWindowStart:   &windowStart,
		DailyLimitUSD:        ptr(10.0),
		WeeklyLimitUSD:       ptr(20.0),
		MonthlyLimitUSD:      ptr(30.0),
		DailyUsageUSD:        0.1,
		WeeklyUsageUSD:       0.2,
		MonthlyUsageUSD:      0.3,
	}, []int64{fx.groupB})

	resp := performJSONRequest(fx.router, http.MethodGet, "/subscriptions/active", nil)

	require.Equal(t, http.StatusOK, resp.Code)
	rows := decodeSubscriptionAliasSlice(t, resp.Body.Bytes())
	require.Len(t, rows, 1)
	require.Equal(t, 0.1, rows[0]["daily_usage_usd"])
	require.Equal(t, 0.2, rows[0]["weekly_usage_usd"])
	require.Equal(t, 0.3, rows[0]["monthly_usage_usd"])
}

func TestSubscriptionHandler_V2OnActiveAliasesIgnoreLegacyUsageFromDifferentCycle(t *testing.T) {
	fx := newSubscriptionAliasFixture(t, true)
	legacyID := mustCreateAliasLegacySubscription(t, fx.client, fx.userID, fx.groupA, fx.now, service.SubscriptionStatusActive)
	legacyWindowStart := fx.now.Add(-24 * time.Hour)
	_, err := fx.client.UserSubscription.UpdateOneID(legacyID).
		SetMonthlyWindowStart(legacyWindowStart).
		SetMonthlyUsageUsd(25).
		Save(context.Background())
	require.NoError(t, err)
	entitlementWindowStart := fx.now.Add(-30 * time.Minute)
	mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
		UserID:               fx.userID,
		LegacySubscriptionID: &legacyID,
		Name:                 "advanced cycle",
		Status:               service.SubscriptionStatusActive,
		StartsAt:             fx.now.Add(-time.Hour),
		ExpiresAt:            fx.now.Add(24 * time.Hour),
		MonthlyWindowStart:   &entitlementWindowStart,
		MonthlyLimitUSD:      ptr(30.0),
		MonthlyUsageUSD:      0.3,
	}, []int64{fx.groupA})

	resp := performJSONRequest(fx.router, http.MethodGet, "/subscriptions/active", nil)

	require.Equal(t, http.StatusOK, resp.Code)
	rows := decodeSubscriptionAliasSlice(t, resp.Body.Bytes())
	require.Len(t, rows, 1)
	require.Equal(t, 0.3, rows[0]["monthly_usage_usd"])
}

func TestSubscriptionHandler_V2OnPrimaryGroupSelection(t *testing.T) {
	fx := newSubscriptionAliasFixture(t, true)
	primaryLegacyID := mustCreateAliasLegacySubscription(t, fx.client, fx.userID, fx.groupA, fx.now, service.SubscriptionStatusActive)
	primaryGroupID := fx.groupA
	primaryEntitlementID := mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
		UserID:               fx.userID,
		LegacySubscriptionID: &primaryLegacyID,
		PrimaryGroupID:       &primaryGroupID,
		Name:                 "explicit primary",
		Status:               service.SubscriptionStatusActive,
		StartsAt:             fx.now.Add(-48 * time.Hour),
		ExpiresAt:            fx.now.Add(48 * time.Hour),
	}, []int64{fx.groupB, fx.groupA})

	fallbackLegacyID := mustCreateAliasLegacySubscription(t, fx.client, fx.userID, fx.groupB, fx.now, service.SubscriptionStatusActive)
	fallbackEntitlementID := mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
		UserID:               fx.userID,
		LegacySubscriptionID: &fallbackLegacyID,
		Name:                 "fallback primary",
		Status:               service.SubscriptionStatusActive,
		StartsAt:             fx.now.Add(-2 * time.Hour),
		ExpiresAt:            fx.now.Add(48 * time.Hour),
	}, []int64{fx.groupB, fx.groupA})

	disabledPrimaryLegacyID := mustCreateAliasLegacySubscription(t, fx.client, fx.userID, fx.groupB, fx.now, service.SubscriptionStatusActive)
	disabledPrimaryEntitlementID := mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
		UserID:               fx.userID,
		LegacySubscriptionID: &disabledPrimaryLegacyID,
		PrimaryGroupID:       &primaryGroupID,
		Name:                 "disabled primary",
		Status:               service.SubscriptionStatusActive,
		StartsAt:             fx.now.Add(-2 * time.Hour),
		ExpiresAt:            fx.now.Add(48 * time.Hour),
	}, []int64{fx.groupB, fx.groupA})
	_, err := fx.client.SubscriptionEntitlementGroup.Update().
		Where(
			subscriptionentitlementgroup.EntitlementIDEQ(disabledPrimaryEntitlementID),
			subscriptionentitlementgroup.GroupIDEQ(fx.groupA),
		).
		SetEnabled(false).
		Save(context.Background())
	require.NoError(t, err)

	resp := performJSONRequest(fx.router, http.MethodGet, "/subscriptions/active", nil)

	require.Equal(t, http.StatusOK, resp.Code)
	rows := decodeSubscriptionAliasSlice(t, resp.Body.Bytes())
	require.Len(t, rows, 3)
	byEntitlementID := make(map[int64]map[string]any, len(rows))
	for _, row := range rows {
		byEntitlementID[int64(row["entitlement_id"].(float64))] = row
	}
	require.Equal(t, float64(fx.groupA), byEntitlementID[primaryEntitlementID]["group_id"])
	require.Equal(t, float64(fx.groupB), byEntitlementID[fallbackEntitlementID]["group_id"])
	require.Equal(t, float64(fx.groupB), byEntitlementID[disabledPrimaryEntitlementID]["group_id"])
}

func TestSubscriptionHandler_V2OnProgressAndSummaryRemainLegacy(t *testing.T) {
	fx := newSubscriptionAliasFixture(t, true)
	legacyID := mustCreateAliasLegacySubscription(t, fx.client, fx.userID, fx.groupA, fx.now, service.SubscriptionStatusActive)
	entitlementOnlyID := mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
		UserID:          fx.userID,
		Name:            "entitlement only",
		Status:          service.SubscriptionStatusActive,
		StartsAt:        fx.now.Add(-time.Hour),
		ExpiresAt:       fx.now.Add(time.Hour),
		DailyUsageUSD:   11,
		WeeklyUsageUSD:  22,
		MonthlyUsageUSD: 33,
	}, []int64{fx.groupB})
	require.NotZero(t, entitlementOnlyID)

	progressResp := performJSONRequest(fx.router, http.MethodGet, "/subscriptions/progress", nil)
	require.Equal(t, http.StatusOK, progressResp.Code)
	require.NotContains(t, progressResp.Body.String(), "entitlement_id")
	progressRows := decodeSubscriptionAliasSlice(t, progressResp.Body.Bytes())
	require.Len(t, progressRows, 1)
	progressSub := progressRows[0]["subscription"].(map[string]any)
	require.Equal(t, float64(legacyID), progressSub["id"])
	require.Equal(t, float64(fx.groupA), progressSub["group_id"])

	summaryResp := performJSONRequest(fx.router, http.MethodGet, "/subscriptions/summary", nil)
	require.Equal(t, http.StatusOK, summaryResp.Code)
	require.NotContains(t, summaryResp.Body.String(), "entitlement_id")
	var summaryEnvelope struct {
		Data struct {
			ActiveCount   int              `json:"active_count"`
			Subscriptions []map[string]any `json:"subscriptions"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(summaryResp.Body.Bytes(), &summaryEnvelope))
	require.Equal(t, 1, summaryEnvelope.Data.ActiveCount)
	require.Len(t, summaryEnvelope.Data.Subscriptions, 1)
	require.Equal(t, float64(legacyID), summaryEnvelope.Data.Subscriptions[0]["id"])
}

func newSubscriptionAliasFixture(t *testing.T, v2Enabled bool) subscriptionAliasFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:subscription_alias_"+strconv.FormatInt(time.Now().UnixNano(), 10)+"?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	now := time.Now().UTC().Truncate(time.Second)
	userID := mustCreateHandlerUser(t, client, "subscription-alias-user@test.local")
	groupA := mustCreateEntitlementHandlerGroup(t, client, "alias-openai", service.PlatformOpenAI)
	groupB := mustCreateEntitlementHandlerGroup(t, client, "alias-gemini", service.PlatformGemini)
	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(groupA).
		SetName("Alias plan").
		SetPrice(9.99).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetAccessScope("explicit").
		SetOveragePolicy(service.SubscriptionEntitlementOverageBlock).
		Save(ctx)
	require.NoError(t, err)

	userSubRepo := repository.NewUserSubscriptionRepository(client)
	entitlementRepo := repository.NewSubscriptionEntitlementRepository(client)
	entitlementSvc := service.NewSubscriptionEntitlementService(entitlementRepo, nil)
	entitlementSvc.SetNowFunc(func() time.Time { return now })
	subscriptionSvc := service.NewSubscriptionService(nil, userSubRepo, nil, client, nil)
	subscriptionSvc.SetSubscriptionEntitlementAliasDependencies(subscriptionAliasRuntimeStub{enabled: v2Enabled}, entitlementSvc)
	h := NewSubscriptionHandler(subscriptionSvc)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: userID, Concurrency: 5})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleUser)
		c.Next()
	})
	router.GET("/subscriptions", h.List)
	router.GET("/subscriptions/active", h.GetActive)
	router.GET("/subscriptions/progress", h.GetProgress)
	router.GET("/subscriptions/summary", h.GetSummary)

	return subscriptionAliasFixture{
		router: router,
		client: client,
		repo:   entitlementRepo,
		userID: userID,
		now:    now,
		groupA: groupA,
		groupB: groupB,
		planID: plan.ID,
	}
}

func mustCreateAliasLegacySubscription(t *testing.T, client *dbent.Client, userID, groupID int64, now time.Time, status string) int64 {
	t.Helper()
	sub, err := client.UserSubscription.Create().
		SetUserID(userID).
		SetGroupID(groupID).
		SetStatus(status).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(now.Add(24 * time.Hour)).
		SetDailyUsageUsd(0.5).
		SetWeeklyUsageUsd(1.5).
		SetMonthlyUsageUsd(2.5).
		Save(context.Background())
	require.NoError(t, err)
	return sub.ID
}

func decodeSubscriptionAliasSlice(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var envelope struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	return envelope.Data
}
