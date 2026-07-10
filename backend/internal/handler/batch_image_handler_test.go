//go:build unit

package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBatchImageOwnerFromContextCarriesResolvedBillingIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	groupID := int64(31)
	subscription := &service.UserSubscription{ID: 41}
	entitlement := &service.SubscriptionEntitlement{ID: 51}
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 21, UserID: 11, GroupID: &groupID})
	c.Set(string(middleware.ContextKeySubscription), subscription)
	c.Set(string(middleware.ContextKeySubscriptionEntitlement), entitlement)
	c.Set(string(middleware.ContextKeySubscriptionEntitlementBalanceFallback), true)

	owner, ok := batchImageOwnerFromContext(c)

	require.True(t, ok)
	require.Equal(t, int64(11), owner.UserID)
	require.Equal(t, int64(21), owner.APIKeyID)
	require.Equal(t, &groupID, owner.GroupID)
	require.Equal(t, &subscription.ID, owner.SubscriptionID)
	require.Equal(t, &entitlement.ID, owner.EntitlementID)
	require.True(t, owner.EntitlementBalanceFallback)
}
