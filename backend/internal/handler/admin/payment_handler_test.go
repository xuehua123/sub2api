//go:build unit

package admin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestSanitizeAdminPaymentOrderForResponseAddsCurrency(t *testing.T) {
	now := time.Now()
	order := &dbent.PaymentOrder{
		ID:          1,
		UserID:      2,
		Amount:      100,
		PayAmount:   108,
		FeeRate:     8,
		OutTradeNo:  "sub2_202606250001",
		PaymentType: "stripe",
		OrderType:   "subscription",
		Status:      "COMPLETED",
		ExpiresAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
		ProviderSnapshot: map[string]any{
			"schema_version": 2,
			"currency":       "USD",
		},
	}

	got := sanitizeAdminPaymentOrderForResponse(order)
	if got == nil {
		t.Fatal("expected sanitized order")
	}
	if got.Currency != "USD" {
		t.Fatalf("expected currency USD, got %q", got.Currency)
	}

	body, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal sanitized order: %v", err)
	}
	if strings.Contains(string(body), "provider_snapshot") {
		t.Fatalf("expected provider_snapshot to be omitted, got %s", string(body))
	}
}

func TestPaymentPlanAdminHandlerReturnsEntitlementV2Config(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	client := newAdminPaymentPlanTestClient(t)
	anthropic := createAdminPaymentPlanTestGroup(t, client, "anthropic-sub", service.PlatformAnthropic, 10)
	openai := createAdminPaymentPlanTestGroup(t, client, "openai-sub", service.PlatformOpenAI, 20)
	handler := NewPaymentHandler(nil, service.NewPaymentConfigService(client, nil, nil))
	router := gin.New()
	router.GET("/plans", handler.ListPlans)
	router.POST("/plans", handler.CreatePlan)
	router.PUT("/plans/:id", handler.UpdatePlan)

	createBody := []byte(fmt.Sprintf(`{
		"name":"Admin Plan",
		"description":"v2",
		"price":19.99,
		"validity_days":30,
		"validity_unit":"day",
		"access_scope":"explicit",
		"group_ids":[%d,%d],
		"daily_limit_usd":1.5,
		"overage_policy":"balance_fallback",
		"for_sale":true
	}`, anthropic.ID, openai.ID))
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, httptest.NewRequest(http.MethodPost, "/plans", bytes.NewReader(createBody)))
	require.Equal(t, http.StatusCreated, createRec.Code)
	created := decodeAdminPaymentPlanResponse(t, createRec.Body.Bytes())
	require.Equal(t, []int64{anthropic.ID, openai.ID}, created.GroupIDs)
	require.Equal(t, service.SubscriptionEntitlementOverageBalanceFallback, created.OveragePolicy)
	require.Len(t, created.Groups, 2)

	updateBody := []byte(`{
		"access_scope":"platform_subscription_groups",
		"group_ids":[],
		"allowed_platforms":["openai"],
		"weekly_limit_usd":8
	}`)
	updateRec := httptest.NewRecorder()
	router.ServeHTTP(updateRec, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/plans/%d", created.ID), bytes.NewReader(updateBody)))
	require.Equal(t, http.StatusOK, updateRec.Code)
	updated := decodeAdminPaymentPlanResponse(t, updateRec.Body.Bytes())
	require.Equal(t, service.PlanAccessScopePlatformSubscriptionGroups, updated.AccessScope)
	require.Equal(t, []string{service.PlatformOpenAI}, updated.AllowedPlatforms)
	require.Equal(t, []int64{openai.ID}, updated.GroupIDs)
	require.NotNil(t, updated.WeeklyLimitUSD)
	require.InDelta(t, 8, *updated.WeeklyLimitUSD, 0.000001)

	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/plans", nil))
	require.Equal(t, http.StatusOK, listRec.Code)
	var listEnvelope struct {
		Code int                                `json:"code"`
		Data []service.SubscriptionPlanResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listEnvelope))
	require.Equal(t, 0, listEnvelope.Code)
	require.Len(t, listEnvelope.Data, 1)
	require.Equal(t, service.PlanAccessScopePlatformSubscriptionGroups, listEnvelope.Data[0].AccessScope)
	require.Equal(t, []int64{openai.ID}, listEnvelope.Data[0].GroupIDs)

	rows, err := client.SubscriptionPlanGroup.Query().All(ctx)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func decodeAdminPaymentPlanResponse(t *testing.T, raw []byte) service.SubscriptionPlanResponse {
	t.Helper()
	var envelope struct {
		Code int                              `json:"code"`
		Data service.SubscriptionPlanResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &envelope))
	require.Equal(t, 0, envelope.Code)
	return envelope.Data
}

func newAdminPaymentPlanTestClient(t *testing.T) *dbent.Client {
	t.Helper()
	dbName := fmt.Sprintf(
		"file:%s?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()),
	)
	db, err := sql.Open("sqlite", dbName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func createAdminPaymentPlanTestGroup(t *testing.T, client *dbent.Client, name, platform string, sortOrder int) *dbent.Group {
	t.Helper()
	group, err := client.Group.Create().
		SetName(name).
		SetPlatform(platform).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeSubscription).
		SetSubscriptionEnabled(true).
		SetPlanAutoGrantEnabled(true).
		SetRateMultiplier(1).
		SetSortOrder(sortOrder).
		Save(context.Background())
	require.NoError(t, err)
	return group
}
