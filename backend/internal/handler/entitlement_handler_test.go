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

type entitlementHandlerFixture struct {
	router *gin.Engine
	client *dbent.Client
	repo   service.SubscriptionEntitlementRepository
	userID int64
	now    time.Time
	groupA int64
	groupB int64
	planID int64
}

func TestEntitlementHandler_ListUserEntitlementsSafeDTO(t *testing.T) {
	fx := newEntitlementHandlerFixture(t)
	windowStart := timezone.StartOfDay(fx.now)
	dailyLimit := 10.0
	weeklyLimit := 40.0
	monthlyLimit := 100.0
	legacyID := mustCreateLegacySubscription(t, fx.client, fx.userID, fx.groupA, fx.now)
	entitlementID := mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
		UserID:               fx.userID,
		PlanID:               &fx.planID,
		LegacySubscriptionID: &legacyID,
		PrimaryGroupID:       &fx.groupB,
		Name:                 "Pro shared plan",
		SourceType:           service.SubscriptionEntitlementSourcePaymentOrder,
		Status:               service.SubscriptionStatusActive,
		StartsAt:             fx.now.Add(-24 * time.Hour),
		ExpiresAt:            fx.now.Add(30 * 24 * time.Hour),
		DailyWindowStart:     &windowStart,
		WeeklyWindowStart:    &windowStart,
		MonthlyWindowStart:   &windowStart,
		DailyLimitUSD:        &dailyLimit,
		WeeklyLimitUSD:       &weeklyLimit,
		MonthlyLimitUSD:      &monthlyLimit,
		DailyUsageUSD:        1.25,
		WeeklyUsageUSD:       8.5,
		MonthlyUsageUSD:      22.75,
		OveragePolicy:        service.SubscriptionEntitlementOverageBalanceFallback,
		SourceID:             ptr(int64(9001)),
		SourceExternalID:     ptr("external-secret"),
		AssignedBy:           &fx.userID,
		Notes:                "internal audit note",
		PlanSnapshot:         map[string]any{"secret": "snapshot"},
	}, []int64{fx.groupB, fx.groupA})
	otherUser := mustCreateHandlerUser(t, fx.client, "other-entitlement-user@test.local")
	mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
		UserID:    otherUser,
		Name:      "other user",
		Status:    service.SubscriptionStatusActive,
		StartsAt:  fx.now.Add(-time.Hour),
		ExpiresAt: fx.now.Add(time.Hour),
	}, []int64{fx.groupA})
	deletedID := mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
		UserID:    fx.userID,
		Name:      "deleted",
		Status:    service.SubscriptionStatusActive,
		StartsAt:  fx.now.Add(-time.Hour),
		ExpiresAt: fx.now.Add(time.Hour),
	}, []int64{fx.groupA})
	_, err := fx.client.SubscriptionEntitlement.UpdateOneID(deletedID).SetDeletedAt(fx.now).Save(context.Background())
	require.NoError(t, err)

	resp := performJSONRequest(fx.router, http.MethodGet, "/entitlements", nil)

	require.Equal(t, http.StatusOK, resp.Code)
	require.NotContains(t, resp.Body.String(), "source_id")
	require.NotContains(t, resp.Body.String(), "source_external_id")
	require.NotContains(t, resp.Body.String(), "source_redeem_code_id")
	require.NotContains(t, resp.Body.String(), "assigned_by")
	require.NotContains(t, resp.Body.String(), "notes")
	require.NotContains(t, resp.Body.String(), "plan_snapshot")
	require.NotContains(t, resp.Body.String(), "fulfillment")
	data := decodeEntitlementSlice(t, resp.Body.Bytes())
	require.Len(t, data, 1)
	got := data[0]
	require.Equal(t, float64(entitlementID), got["id"])
	require.Equal(t, float64(fx.planID), got["plan_id"])
	require.Equal(t, "Pro shared plan", got["plan_name"])
	require.Equal(t, "Pro shared plan", got["name"])
	require.Equal(t, service.SubscriptionStatusActive, got["status"])
	require.Equal(t, service.SubscriptionEntitlementOverageBalanceFallback, got["overage_policy"])
	require.Equal(t, float64(legacyID), got["legacy_subscription_id"])
	require.Equal(t, 1.25, got["daily_usage_usd"])
	require.Equal(t, float64(10), got["daily_limit_usd"])
	expectedDailyResetSeconds := timezone.StartOfDay(fx.now).AddDate(0, 0, 1).Sub(fx.now).Seconds()
	require.Equal(t, expectedDailyResetSeconds, got["daily_resets_in_seconds"])
	groups, ok := got["groups"].([]any)
	require.True(t, ok)
	require.Len(t, groups, 2)
	require.Equal(t, float64(fx.groupB), groups[0].(map[string]any)["id"])
	require.Equal(t, float64(fx.groupA), groups[1].(map[string]any)["id"])
}

func TestEntitlementHandler_ActiveFiltersByStatusAndWindow(t *testing.T) {
	fx := newEntitlementHandlerFixture(t)
	activeID := mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
		UserID:    fx.userID,
		Name:      "active",
		Status:    service.SubscriptionStatusActive,
		StartsAt:  fx.now.Add(-48 * time.Hour),
		ExpiresAt: fx.now.Add(48 * time.Hour),
	}, []int64{fx.groupA})
	for _, item := range []struct {
		name      string
		status    string
		startsAt  time.Time
		expiresAt time.Time
		deleted   bool
	}{
		{name: "future", status: service.SubscriptionStatusActive, startsAt: fx.now.Add(24 * time.Hour), expiresAt: fx.now.Add(48 * time.Hour)},
		{name: "expired", status: service.SubscriptionStatusActive, startsAt: fx.now.Add(-48 * time.Hour), expiresAt: fx.now.Add(-24 * time.Hour)},
		{name: "suspended", status: service.SubscriptionStatusSuspended, startsAt: fx.now.Add(-48 * time.Hour), expiresAt: fx.now.Add(48 * time.Hour)},
		{name: "deleted", status: service.SubscriptionStatusActive, startsAt: fx.now.Add(-48 * time.Hour), expiresAt: fx.now.Add(48 * time.Hour), deleted: true},
	} {
		id := mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
			UserID:    fx.userID,
			Name:      item.name,
			Status:    item.status,
			StartsAt:  item.startsAt,
			ExpiresAt: item.expiresAt,
		}, []int64{fx.groupA})
		if item.deleted {
			_, err := fx.client.SubscriptionEntitlement.UpdateOneID(id).SetDeletedAt(fx.now).Save(context.Background())
			require.NoError(t, err)
		}
	}

	resp := performJSONRequest(fx.router, http.MethodGet, "/entitlements/active", nil)

	require.Equal(t, http.StatusOK, resp.Code)
	data := decodeEntitlementSlice(t, resp.Body.Bytes())
	require.Len(t, data, 1)
	require.Equal(t, float64(activeID), data[0]["id"])
}

func TestEntitlementHandler_ListHidesHistoricalEntitlementsWhenRuntimeDisabled(t *testing.T) {
	fx := newEntitlementHandlerFixture(t)
	h := NewEntitlementHandler(
		service.NewSubscriptionEntitlementService(fx.repo, nil),
		apiKeyHandlerRuntimeStub{runtime: service.SubscriptionEntitlementsRuntime{Enabled: false}},
	)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: fx.userID, Concurrency: 5})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleUser)
		c.Next()
	})
	router.GET("/entitlements", h.List)
	router.GET("/entitlements/active", h.GetActive)

	for _, path := range []string{"/entitlements", "/entitlements/active"} {
		resp := performJSONRequest(router, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, resp.Code)
		require.Empty(t, decodeEntitlementSlice(t, resp.Body.Bytes()))
	}
}

func TestEntitlementHandler_GetProgressOwnerAndUnlimited(t *testing.T) {
	fx := newEntitlementHandlerFixture(t)
	entitlementID := mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
		UserID:          fx.userID,
		Name:            "unlimited",
		Status:          service.SubscriptionStatusActive,
		StartsAt:        fx.now.Add(-time.Hour),
		ExpiresAt:       fx.now.Add(24 * time.Hour),
		DailyUsageUSD:   3.5,
		WeeklyUsageUSD:  4.5,
		MonthlyUsageUSD: 5.5,
		OveragePolicy:   service.SubscriptionEntitlementOverageBlock,
	}, []int64{fx.groupA})
	otherUser := mustCreateHandlerUser(t, fx.client, "progress-other@test.local")
	otherID := mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
		UserID:    otherUser,
		Name:      "other",
		Status:    service.SubscriptionStatusActive,
		StartsAt:  fx.now.Add(-time.Hour),
		ExpiresAt: fx.now.Add(24 * time.Hour),
	}, []int64{fx.groupA})

	resp := performJSONRequest(fx.router, http.MethodGet, "/entitlements/"+strconv.FormatInt(entitlementID, 10)+"/progress", nil)

	require.Equal(t, http.StatusOK, resp.Code)
	got := decodeEntitlementObject(t, resp.Body.Bytes())
	require.Equal(t, float64(entitlementID), got["id"])
	require.Nil(t, got["daily_limit_usd"])
	require.Nil(t, got["weekly_limit_usd"])
	require.Nil(t, got["monthly_limit_usd"])
	require.Equal(t, 3.5, got["daily_usage_usd"])
	require.Nil(t, got["daily_resets_at"])
	require.Nil(t, got["daily_resets_in_seconds"])

	resp = performJSONRequest(fx.router, http.MethodGet, "/entitlements/"+strconv.FormatInt(otherID, 10)+"/progress", nil)
	require.Equal(t, http.StatusNotFound, resp.Code)
}

func newEntitlementHandlerFixture(t *testing.T) entitlementHandlerFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:entitlement_handler_"+strconv.FormatInt(time.Now().UnixNano(), 10)+"?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	userID := mustCreateHandlerUser(t, client, "entitlement-user@test.local")
	groupA := mustCreateEntitlementHandlerGroup(t, client, "ent-group-a", service.PlatformOpenAI)
	groupB := mustCreateEntitlementHandlerGroup(t, client, "ent-group-b", service.PlatformGemini)
	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(groupA).
		SetName("Pro shared plan").
		SetPrice(9.99).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetAccessScope("explicit").
		SetOveragePolicy(service.SubscriptionEntitlementOverageBalanceFallback).
		Save(ctx)
	require.NoError(t, err)

	repo := repository.NewSubscriptionEntitlementRepository(client)
	svc := service.NewSubscriptionEntitlementService(repo, nil)
	now := time.Now().UTC().Truncate(time.Second)
	svc.SetNowFunc(func() time.Time { return now })
	h := NewEntitlementHandler(svc, apiKeyHandlerRuntimeStub{runtime: service.SubscriptionEntitlementsRuntime{Enabled: true}})
	h.SetNowFunc(func() time.Time { return now })
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: userID, Concurrency: 5})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleUser)
		c.Next()
	})
	router.GET("/entitlements", h.List)
	router.GET("/entitlements/active", h.GetActive)
	router.GET("/entitlements/:id/progress", h.GetProgress)
	router.POST("/entitlements/:id/advance-monthly-cycle", h.AdvanceMonthlyCycle)

	return entitlementHandlerFixture{
		router: router,
		client: client,
		repo:   repo,
		userID: userID,
		now:    now,
		groupA: groupA,
		groupB: groupB,
		planID: plan.ID,
	}
}

func mustCreateHandlerUser(t *testing.T, client *dbent.Client, email string) int64 {
	t.Helper()
	user, err := client.User.Create().
		SetEmail(email).
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(context.Background())
	require.NoError(t, err)
	return user.ID
}

func mustCreateEntitlementHandlerGroup(t *testing.T, client *dbent.Client, name, platform string) int64 {
	t.Helper()
	group, err := client.Group.Create().
		SetName(name).
		SetPlatform(platform).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeSubscription).
		SetRateMultiplier(1).
		Save(context.Background())
	require.NoError(t, err)
	return group.ID
}

func mustCreateLegacySubscription(t *testing.T, client *dbent.Client, userID, groupID int64, now time.Time) int64 {
	t.Helper()
	sub, err := client.UserSubscription.Create().
		SetUserID(userID).
		SetGroupID(groupID).
		SetStatus(service.SubscriptionStatusActive).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(now.Add(24 * time.Hour)).
		Save(context.Background())
	require.NoError(t, err)
	return sub.ID
}

func mustCreateEntitlementForHandler(t *testing.T, repo service.SubscriptionEntitlementRepository, ent service.SubscriptionEntitlement, groupIDs []int64) int64 {
	t.Helper()
	if ent.ExpiresAt.IsZero() {
		ent.ExpiresAt = time.Now().Add(24 * time.Hour)
	}
	require.NoError(t, repo.Create(context.Background(), &ent, groupIDs))
	return ent.ID
}

func decodeEntitlementSlice(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var envelope struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	return envelope.Data
}

func decodeEntitlementObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	return envelope.Data
}

func ptr[T any](v T) *T {
	return &v
}

type handlerAdvanceEntitlementRepo struct {
	ent *service.SubscriptionEntitlement

	lockedUserID int64
	updateCalls  []service.SubscriptionEntitlementMonthlyCycleUpdate
	resetLogs    []service.SubscriptionEntitlementCycleResetLog
}

func (r *handlerAdvanceEntitlementRepo) WithUserEntitlementMutationTx(ctx context.Context, _ int64, fn func(context.Context) error) error {
	return fn(ctx)
}

func (r *handlerAdvanceEntitlementRepo) Create(context.Context, *service.SubscriptionEntitlement, []int64) error {
	return service.ErrSubscriptionEntitlementNotFound
}

func (r *handlerAdvanceEntitlementRepo) CreateTx(context.Context, *service.SubscriptionEntitlement, []int64) error {
	return service.ErrSubscriptionEntitlementNotFound
}

func (r *handlerAdvanceEntitlementRepo) CreateWithFulfillment(context.Context, *service.SubscriptionEntitlement, []int64, *service.SubscriptionEntitlementFulfillment) error {
	return service.ErrSubscriptionEntitlementNotFound
}

func (r *handlerAdvanceEntitlementRepo) GetByID(_ context.Context, id int64) (*service.SubscriptionEntitlement, error) {
	if r.ent == nil || r.ent.ID != id {
		return nil, service.ErrSubscriptionEntitlementNotFound
	}
	return cloneHandlerEntitlement(r.ent), nil
}

func (r *handlerAdvanceEntitlementRepo) GetByIDForUpdate(ctx context.Context, id int64) (*service.SubscriptionEntitlement, error) {
	return r.GetByID(ctx, id)
}

func (r *handlerAdvanceEntitlementRepo) GetBySourceID(context.Context, string, int64) (*service.SubscriptionEntitlement, error) {
	return nil, service.ErrSubscriptionEntitlementNotFound
}

func (r *handlerAdvanceEntitlementRepo) GetBySourceExternalID(context.Context, string, string) (*service.SubscriptionEntitlement, error) {
	return nil, service.ErrSubscriptionEntitlementNotFound
}

func (r *handlerAdvanceEntitlementRepo) GetBySourceRedeemCodeID(context.Context, int64) (*service.SubscriptionEntitlement, error) {
	return nil, service.ErrSubscriptionEntitlementNotFound
}

func (r *handlerAdvanceEntitlementRepo) GetFulfillmentBySourceID(context.Context, string, int64) (*service.SubscriptionEntitlementFulfillment, error) {
	return nil, service.ErrSubscriptionEntitlementNotFound
}

func (r *handlerAdvanceEntitlementRepo) GetFulfillmentBySourceExternalID(context.Context, string, string) (*service.SubscriptionEntitlementFulfillment, error) {
	return nil, service.ErrSubscriptionEntitlementNotFound
}

func (r *handlerAdvanceEntitlementRepo) GetFulfillmentBySourceRedeemCodeID(context.Context, int64) (*service.SubscriptionEntitlementFulfillment, error) {
	return nil, service.ErrSubscriptionEntitlementNotFound
}

func (r *handlerAdvanceEntitlementRepo) GetActiveCoveringGroup(context.Context, int64, int64) ([]service.SubscriptionEntitlement, error) {
	return nil, nil
}

func (r *handlerAdvanceEntitlementRepo) ListByUserID(context.Context, int64) ([]service.SubscriptionEntitlement, error) {
	return nil, nil
}

func (r *handlerAdvanceEntitlementRepo) ListByUserPlanID(context.Context, int64, int64) ([]service.SubscriptionEntitlement, error) {
	return nil, nil
}

func (r *handlerAdvanceEntitlementRepo) ListByUserPlanIDForUpdate(ctx context.Context, userID, planID int64) ([]service.SubscriptionEntitlement, error) {
	return r.ListByUserPlanID(ctx, userID, planID)
}

func (r *handlerAdvanceEntitlementRepo) ListActiveByUserID(context.Context, int64) ([]service.SubscriptionEntitlement, error) {
	return nil, nil
}

func (r *handlerAdvanceEntitlementRepo) ListActiveCoveringGroupForUser(context.Context, int64, int64) ([]service.SubscriptionEntitlement, error) {
	return nil, nil
}

func (r *handlerAdvanceEntitlementRepo) UpdateTerm(context.Context, int64, time.Time, time.Time, string, string) error {
	return service.ErrSubscriptionEntitlementNotFound
}

func (r *handlerAdvanceEntitlementRepo) UpdateTermAndSource(context.Context, int64, time.Time, time.Time, string, string, service.SubscriptionEntitlementSourceRef) error {
	return service.ErrSubscriptionEntitlementNotFound
}

func (r *handlerAdvanceEntitlementRepo) ExtendWithFulfillment(context.Context, int64, time.Time, time.Time, string, string, service.SubscriptionEntitlementSourceRef, *service.SubscriptionEntitlementFulfillment, bool, time.Time, time.Time) error {
	return service.ErrSubscriptionEntitlementNotFound
}

func (r *handlerAdvanceEntitlementRepo) ActivateWindows(context.Context, int64, time.Time, time.Time) error {
	return service.ErrSubscriptionEntitlementNotFound
}

func (r *handlerAdvanceEntitlementRepo) ResetUsage(context.Context, int64, bool, bool, bool, time.Time, time.Time) error {
	return service.ErrSubscriptionEntitlementNotFound
}

func (r *handlerAdvanceEntitlementRepo) ResetDailyUsage(context.Context, int64, *time.Time, time.Time) error {
	return service.ErrSubscriptionEntitlementNotFound
}

func (r *handlerAdvanceEntitlementRepo) ResetWeeklyUsage(context.Context, int64, *time.Time, time.Time) error {
	return service.ErrSubscriptionEntitlementNotFound
}

func (r *handlerAdvanceEntitlementRepo) ResetMonthlyUsage(context.Context, int64, *time.Time, time.Time) error {
	return service.ErrSubscriptionEntitlementNotFound
}

func (r *handlerAdvanceEntitlementRepo) ApplyEntitlementUsage(context.Context, int64, float64, time.Time) (*service.EntitlementUsageApplyResult, error) {
	return nil, service.ErrSubscriptionEntitlementNotFound
}

func (r *handlerAdvanceEntitlementRepo) ReplaceGroups(context.Context, int64, []int64) error {
	return service.ErrSubscriptionEntitlementNotFound
}

func (r *handlerAdvanceEntitlementRepo) LockEntitlementMonthlyCycle(_ context.Context, userID, entitlementID int64) (*service.SubscriptionEntitlementMonthlyCycleSnapshot, error) {
	r.lockedUserID = userID
	if r.ent == nil || r.ent.ID != entitlementID || r.ent.UserID != userID {
		return nil, service.ErrSubscriptionEntitlementNotFound
	}
	return &service.SubscriptionEntitlementMonthlyCycleSnapshot{
		ID:                 r.ent.ID,
		UserID:             r.ent.UserID,
		PlanID:             cloneHandlerInt64Ptr(r.ent.PlanID),
		Status:             r.ent.Status,
		StartsAt:           r.ent.StartsAt,
		ExpiresAt:          r.ent.ExpiresAt,
		MonthlyLimitUSD:    cloneHandlerFloat64Ptr(r.ent.MonthlyLimitUSD),
		MonthlyUsageUSD:    r.ent.MonthlyUsageUSD,
		MonthlyWindowStart: cloneHandlerTimePtr(r.ent.MonthlyWindowStart),
	}, nil
}

func (r *handlerAdvanceEntitlementRepo) UpdateEntitlementMonthlyCycle(_ context.Context, update service.SubscriptionEntitlementMonthlyCycleUpdate) error {
	if r.ent == nil || r.ent.ID != update.EntitlementID || r.ent.UserID != update.UserID {
		return service.ErrSubscriptionEntitlementNotFound
	}
	r.ent.MonthlyUsageUSD = update.NewMonthlyUsageUSD
	r.ent.MonthlyWindowStart = cloneHandlerTimeValue(update.NewMonthlyWindowStart)
	r.ent.ExpiresAt = update.NewExpiresAt
	r.ent.UpdatedAt = update.UpdatedAt
	r.updateCalls = append(r.updateCalls, update)
	return nil
}

func (r *handlerAdvanceEntitlementRepo) InsertEntitlementCycleResetLog(_ context.Context, log service.SubscriptionEntitlementCycleResetLog) error {
	r.resetLogs = append(r.resetLogs, log)
	return nil
}

func TestEntitlementHandler_AdvanceMonthlyCycleUsesSubjectOwnerAndPublicDTO(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	userID := int64(42)
	entitlementID := int64(22)
	planID := int64(5)
	monthlyLimit := 100.0
	startsAt := now.Add(-10 * 24 * time.Hour)
	expiresAt := startsAt.Add(120 * 24 * time.Hour)
	repo := &handlerAdvanceEntitlementRepo{ent: &service.SubscriptionEntitlement{
		ID:                 entitlementID,
		UserID:             userID,
		PlanID:             &planID,
		Name:               "Shared Pro",
		Status:             service.SubscriptionStatusActive,
		StartsAt:           startsAt,
		ExpiresAt:          expiresAt,
		MonthlyLimitUSD:    &monthlyLimit,
		MonthlyUsageUSD:    95,
		MonthlyWindowStart: &startsAt,
		Groups: []service.Group{{
			ID:               101,
			Name:             "OpenAI Pro",
			Platform:         service.PlatformOpenAI,
			RateMultiplier:   1,
			Status:           service.StatusActive,
			SubscriptionType: service.SubscriptionTypeSubscription,
		}},
	}}
	svc := service.NewSubscriptionEntitlementService(repo, nil)
	svc.SetNowFunc(func() time.Time { return now })
	h := NewEntitlementHandler(svc, apiKeyHandlerRuntimeStub{runtime: service.SubscriptionEntitlementsRuntime{Enabled: true}})
	h.SetNowFunc(func() time.Time { return now })
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: userID, Concurrency: 5})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleUser)
		c.Next()
	})
	router.POST("/entitlements/:id/advance-monthly-cycle", h.AdvanceMonthlyCycle)

	resp := performJSONRequest(router, http.MethodPost, "/entitlements/22/advance-monthly-cycle", nil)

	require.Equal(t, http.StatusOK, resp.Code)
	require.NotContains(t, resp.Body.String(), `"subscription"`)
	require.Equal(t, userID, repo.lockedUserID)
	require.Len(t, repo.updateCalls, 1)
	require.Len(t, repo.resetLogs, 1)
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &envelope))
	require.Contains(t, envelope.Data, "entitlement")
	require.Equal(t, float64(95), envelope.Data["previous_monthly_usage_usd"])
	entitlement, ok := envelope.Data["entitlement"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(entitlementID), entitlement["id"])
	require.Equal(t, float64(0), entitlement["monthly_usage_usd"])
	require.NotContains(t, entitlement, "source_id")
	require.NotContains(t, entitlement, "notes")
}

func TestEntitlementHandler_AdvanceMonthlyCycleCrossUserReturnsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	monthlyLimit := 100.0
	startsAt := now.Add(-10 * 24 * time.Hour)
	repo := &handlerAdvanceEntitlementRepo{ent: &service.SubscriptionEntitlement{
		ID:                 22,
		UserID:             99,
		Status:             service.SubscriptionStatusActive,
		StartsAt:           startsAt,
		ExpiresAt:          startsAt.Add(120 * 24 * time.Hour),
		MonthlyLimitUSD:    &monthlyLimit,
		MonthlyUsageUSD:    95,
		MonthlyWindowStart: &startsAt,
	}}
	svc := service.NewSubscriptionEntitlementService(repo, nil)
	svc.SetNowFunc(func() time.Time { return now })
	h := NewEntitlementHandler(svc, apiKeyHandlerRuntimeStub{runtime: service.SubscriptionEntitlementsRuntime{Enabled: true}})
	h.SetNowFunc(func() time.Time { return now })
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42, Concurrency: 5})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleUser)
		c.Next()
	})
	router.POST("/entitlements/:id/advance-monthly-cycle", h.AdvanceMonthlyCycle)

	resp := performJSONRequest(router, http.MethodPost, "/entitlements/22/advance-monthly-cycle", nil)

	require.Equal(t, http.StatusNotFound, resp.Code)
	require.Empty(t, repo.updateCalls)
	require.Empty(t, repo.resetLogs)
}

func cloneHandlerEntitlement(ent *service.SubscriptionEntitlement) *service.SubscriptionEntitlement {
	if ent == nil {
		return nil
	}
	cp := *ent
	cp.PlanID = cloneHandlerInt64Ptr(ent.PlanID)
	cp.LegacySubscriptionID = cloneHandlerInt64Ptr(ent.LegacySubscriptionID)
	cp.PrimaryGroupID = cloneHandlerInt64Ptr(ent.PrimaryGroupID)
	cp.DailyWindowStart = cloneHandlerTimePtr(ent.DailyWindowStart)
	cp.WeeklyWindowStart = cloneHandlerTimePtr(ent.WeeklyWindowStart)
	cp.MonthlyWindowStart = cloneHandlerTimePtr(ent.MonthlyWindowStart)
	cp.DailyLimitUSD = cloneHandlerFloat64Ptr(ent.DailyLimitUSD)
	cp.WeeklyLimitUSD = cloneHandlerFloat64Ptr(ent.WeeklyLimitUSD)
	cp.MonthlyLimitUSD = cloneHandlerFloat64Ptr(ent.MonthlyLimitUSD)
	cp.SourceID = cloneHandlerInt64Ptr(ent.SourceID)
	cp.SourceExternalID = cloneHandlerStringPtr(ent.SourceExternalID)
	cp.SourceRedeemCodeID = cloneHandlerInt64Ptr(ent.SourceRedeemCodeID)
	cp.AssignedBy = cloneHandlerInt64Ptr(ent.AssignedBy)
	cp.Groups = append([]service.Group(nil), ent.Groups...)
	return &cp
}

func cloneHandlerInt64Ptr(v *int64) *int64 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneHandlerFloat64Ptr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneHandlerStringPtr(v *string) *string {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneHandlerTimePtr(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneHandlerTimeValue(v time.Time) *time.Time {
	out := v
	return &out
}
