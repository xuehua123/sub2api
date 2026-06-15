package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSubmitUsageRecordTaskCopiesRequestContext(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.ClientRequestID, "client-request-123")
	parent = context.WithValue(parent, ctxkey.RequestID, "request-456")

	var gotClientRequestID string
	var gotRequestID string
	h := &GatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotClientRequestID, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		gotRequestID, _ = ctx.Value(ctxkey.RequestID).(string)
	})

	require.Equal(t, "client-request-123", gotClientRequestID)
	require.Equal(t, "request-456", gotRequestID)
}

func TestOpenAISubmitUsageRecordTaskCopiesRequestContext(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.ClientRequestID, "openai-client-request-123")
	parent = context.WithValue(parent, ctxkey.RequestID, "openai-request-456")

	var gotClientRequestID string
	var gotRequestID string
	h := &OpenAIGatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotClientRequestID, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		gotRequestID, _ = ctx.Value(ctxkey.RequestID).(string)
	})

	require.Equal(t, "openai-client-request-123", gotClientRequestID)
	require.Equal(t, "openai-request-456", gotRequestID)
}

func TestSubscriptionEntitlementUsageContextReadsMiddlewareResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	entitlement := &service.SubscriptionEntitlement{ID: 123}
	c.Set(string(middleware2.ContextKeySubscriptionEntitlement), entitlement)
	c.Set(string(middleware2.ContextKeySubscriptionEntitlementBalanceFallback), true)

	got, fallback := subscriptionEntitlementUsageContext(c)

	require.Same(t, entitlement, got)
	require.True(t, fallback)
}
