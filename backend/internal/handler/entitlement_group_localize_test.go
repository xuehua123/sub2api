//go:build unit

package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEntitlementHandlerLocalizesGroupsAcrossUserResponses(t *testing.T) {
	fx := newEntitlementHandlerFixture(t)
	_, err := fx.client.Group.UpdateOneID(fx.groupA).SetI18n(service.GroupI18n{"en": {Name: "Standard"}}).Save(context.Background())
	require.NoError(t, err)
	_, err = fx.client.Group.UpdateOneID(fx.groupB).SetI18n(service.GroupI18n{"en": {Name: "Premium"}}).Save(context.Background())
	require.NoError(t, err)
	id := mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
		UserID: fx.userID, Name: "shared", Status: service.SubscriptionStatusActive,
		StartsAt: fx.now.Add(-time.Hour), ExpiresAt: fx.now.Add(90 * 24 * time.Hour),
		MonthlyLimitUSD: ptr(100.0),
	}, []int64{fx.groupA, fx.groupB})

	for _, path := range []string{"/entitlements", "/entitlements/active", "/entitlements/" + strconv.FormatInt(id, 10) + "/progress"} {
		for _, language := range []string{"en-US,en;q=0.9", "zh"} {
			resp := performLocalizedJSONRequest(fx.router, http.MethodGet, path, nil, language)
			require.Equal(t, http.StatusOK, resp.Code)
			var data map[string]any
			if path == "/entitlements" || path == "/entitlements/active" {
				rows := decodeEntitlementSlice(t, resp.Body.Bytes())
				require.Len(t, rows, 1)
				data = rows[0]
			} else {
				data = decodeEntitlementObject(t, resp.Body.Bytes())
			}
			groups := data["groups"].([]any)
			require.Len(t, groups, 2)
			if language == "zh" {
				require.Equal(t, "ent-group-a", groups[0].(map[string]any)["name"])
				require.Equal(t, "ent-group-b", groups[1].(map[string]any)["name"])
			} else {
				require.Equal(t, "Standard", groups[0].(map[string]any)["name"])
				require.Equal(t, "Premium", groups[1].(map[string]any)["name"])
			}
			require.NotContains(t, resp.Body.String(), `"i18n"`)
		}
	}
}

func TestSubscriptionHandlerLocalizesEntitlementAliases(t *testing.T) {
	fx := newSubscriptionAliasFixture(t, true)
	_, err := fx.client.Group.UpdateOneID(fx.groupA).SetI18n(service.GroupI18n{"en": {Name: "Standard"}}).Save(context.Background())
	require.NoError(t, err)
	_, err = fx.client.Group.UpdateOneID(fx.groupB).SetI18n(service.GroupI18n{"en": {Name: "Premium", Description: "Premium pool"}}).Save(context.Background())
	require.NoError(t, err)
	legacyID := mustCreateAliasLegacySubscription(t, fx.client, fx.userID, fx.groupA, fx.now, service.SubscriptionStatusActive)
	mustCreateEntitlementForHandler(t, fx.repo, service.SubscriptionEntitlement{
		UserID: fx.userID, LegacySubscriptionID: &legacyID, PrimaryGroupID: &fx.groupB,
		Name: "shared", Status: service.SubscriptionStatusActive,
		StartsAt: fx.now.Add(-time.Hour), ExpiresAt: fx.now.Add(24 * time.Hour),
	}, []int64{fx.groupA, fx.groupB})
	for _, path := range []string{"/subscriptions", "/subscriptions/active"} {
		resp := performLocalizedJSONRequest(fx.router, http.MethodGet, path, nil, "en")
		require.Equal(t, http.StatusOK, resp.Code)
		rows := decodeSubscriptionAliasSlice(t, resp.Body.Bytes())
		require.Len(t, rows, 1)
		primary := rows[0]["group"].(map[string]any)
		require.Equal(t, "Premium", primary["name"])
		require.Equal(t, "Premium pool", primary["description"])
		groups := rows[0]["groups"].([]any)
		require.Len(t, groups, 2)
		require.Equal(t, "Standard", groups[0].(map[string]any)["name"])
		require.Equal(t, "Premium", groups[1].(map[string]any)["name"])
		require.NotContains(t, resp.Body.String(), `"i18n"`)
	}
}

func performLocalizedJSONRequest(router *gin.Engine, method, path string, body []byte, language string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", language)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
