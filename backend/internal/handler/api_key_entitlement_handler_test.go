//go:build unit

package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

type apiKeyHandlerRuntimeStub struct {
	runtime service.SubscriptionEntitlementsRuntime
}

func (s apiKeyHandlerRuntimeStub) GetSubscriptionEntitlementsRuntime(context.Context) service.SubscriptionEntitlementsRuntime {
	return s.runtime
}

type apiKeyHandlerEntitlementFixture struct {
	router         *gin.Engine
	client         *dbent.Client
	userID         int64
	standardID     int64
	subscriptionID int64
	entitlementID  int64
	now            time.Time
}

func TestAPIKeyHandler_CreateUpdateSubscriptionEntitlementID(t *testing.T) {
	service.SetDefaultIdempotencyCoordinator(nil)
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(nil) })

	fx := newAPIKeyHandlerEntitlementFixture(t, true)
	secondEntitlementID := mustCreateHandlerEntitlement(t, fx.client, fx.userID, fx.subscriptionID, fx.now, "handler entitlement 2")

	createBody := map[string]any{
		"name":                        "handler key",
		"group_id":                    fx.subscriptionID,
		"subscription_entitlement_id": fx.entitlementID,
		"custom_key":                  "sk-handler-entitlement-create",
	}
	createResp := performJSONRequest(fx.router, http.MethodPost, "/keys", createBody)
	require.Equal(t, http.StatusOK, createResp.Code)
	createData := decodeResponseData(t, createResp)
	require.Equal(t, float64(fx.entitlementID), createData["subscription_entitlement_id"])
	keyID := int64(createData["id"].(float64))

	updateBody := map[string]any{
		"subscription_entitlement_id": secondEntitlementID,
	}
	updateResp := performJSONRequest(fx.router, http.MethodPut, "/keys/"+strconv.FormatInt(keyID, 10), updateBody)
	require.Equal(t, http.StatusOK, updateResp.Code)
	updateData := decodeResponseData(t, updateResp)
	require.Equal(t, float64(secondEntitlementID), updateData["subscription_entitlement_id"])
}

func TestAPIKeyHandler_GetAvailableGroupsEntitlementAware(t *testing.T) {
	fx := newAPIKeyHandlerEntitlementFixture(t, true)

	resp := performJSONRequest(fx.router, http.MethodGet, "/groups/available", nil)
	require.Equal(t, http.StatusOK, resp.Code)
	data := decodeResponseDataSlice(t, resp)

	standard := findGroupResponse(t, data, fx.standardID)
	require.NotContains(t, standard, "entitlements")

	subscription := findGroupResponse(t, data, fx.subscriptionID)
	entitlements, ok := subscription["entitlements"].([]any)
	require.True(t, ok)
	require.Len(t, entitlements, 1)
	entitlement := entitlements[0].(map[string]any)
	require.Equal(t, float64(fx.entitlementID), entitlement["id"])
}

func TestAPIKeyHandler_GetAvailableGroupsV2OffKeepsLegacyShape(t *testing.T) {
	fx := newAPIKeyHandlerEntitlementFixture(t, false)
	_, err := fx.client.UserSubscription.Create().
		SetUserID(fx.userID).
		SetGroupID(fx.subscriptionID).
		SetStatus(service.SubscriptionStatusActive).
		SetStartsAt(fx.now.Add(-time.Hour)).
		SetExpiresAt(fx.now.Add(24 * time.Hour)).
		Save(context.Background())
	require.NoError(t, err)

	resp := performJSONRequest(fx.router, http.MethodGet, "/groups/available", nil)
	require.Equal(t, http.StatusOK, resp.Code)
	data := decodeResponseDataSlice(t, resp)

	subscription := findGroupResponse(t, data, fx.subscriptionID)
	require.NotContains(t, subscription, "entitlements")
}

func newAPIKeyHandlerEntitlementFixture(t *testing.T, v2Enabled bool) apiKeyHandlerEntitlementFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:api_key_handler_entitlement_"+strconv.FormatInt(time.Now().UnixNano(), 10)+"?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	user, err := client.User.Create().
		SetEmail("handler-entitlement@test.local").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	standard, err := client.Group.Create().
		SetName("handler-standard").
		SetPlatform(service.PlatformOpenAI).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeStandard).
		SetRateMultiplier(1).
		Save(ctx)
	require.NoError(t, err)

	subscription, err := client.Group.Create().
		SetName("handler-subscription").
		SetPlatform(service.PlatformOpenAI).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeSubscription).
		SetRateMultiplier(1).
		Save(ctx)
	require.NoError(t, err)

	apiKeyRepo := repository.NewAPIKeyRepository(client, db)
	userRepo := repository.NewUserRepository(client, db)
	groupRepo := repository.NewGroupRepository(client, db)
	userSubRepo := repository.NewUserSubscriptionRepository(client)
	entitlementRepo := repository.NewSubscriptionEntitlementRepository(client)
	entitlementSvc := service.NewSubscriptionEntitlementService(entitlementRepo, nil)
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	entitlementSvc.SetNowFunc(func() time.Time { return now })
	apiKeySvc := service.NewAPIKeyService(apiKeyRepo, userRepo, groupRepo, userSubRepo, nil, nil, &config.Config{})
	apiKeySvc.SetSubscriptionEntitlementDependencies(
		apiKeyHandlerRuntimeStub{runtime: service.SubscriptionEntitlementsRuntime{Enabled: v2Enabled}},
		entitlementSvc,
	)

	entitlementID := mustCreateHandlerEntitlement(t, client, user.ID, subscription.ID, now, "handler entitlement")

	h := NewAPIKeyHandler(apiKeySvc)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: user.ID, Concurrency: 5})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleUser)
		c.Next()
	})
	router.POST("/keys", h.Create)
	router.PUT("/keys/:id", h.Update)
	router.GET("/groups/available", h.GetAvailableGroups)

	return apiKeyHandlerEntitlementFixture{
		router:         router,
		client:         client,
		userID:         user.ID,
		standardID:     standard.ID,
		subscriptionID: subscription.ID,
		entitlementID:  entitlementID,
		now:            now,
	}
}

func mustCreateHandlerEntitlement(t *testing.T, client *dbent.Client, userID, groupID int64, now time.Time, name string) int64 {
	t.Helper()
	entitlementRepo := repository.NewSubscriptionEntitlementRepository(client)
	ent := &service.SubscriptionEntitlement{
		UserID:    userID,
		Name:      name,
		Status:    service.SubscriptionStatusActive,
		StartsAt:  now.Add(-48 * time.Hour),
		ExpiresAt: now.Add(48 * time.Hour),
	}
	require.NoError(t, entitlementRepo.Create(context.Background(), ent, []int64{groupID}))
	return ent.ID
}

func performJSONRequest(router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeResponseData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	return envelope.Data
}

func decodeResponseDataSlice(t *testing.T, rec *httptest.ResponseRecorder) []any {
	t.Helper()
	var envelope struct {
		Data []any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	return envelope.Data
}

func findGroupResponse(t *testing.T, groups []any, id int64) map[string]any {
	t.Helper()
	for _, item := range groups {
		group, ok := item.(map[string]any)
		require.True(t, ok)
		if group["id"] == float64(id) {
			return group
		}
	}
	t.Fatalf("group %d not found in response", id)
	return nil
}
