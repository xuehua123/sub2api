package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestInvalidVoiceRequestRejectedBeforeBillingAdmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct{ endpoint, body string }{{"tts", "{"}, {"tts", "{}"}, {"stt", ""}} {
		t.Run(tc.endpoint+tc.body, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+tc.endpoint, strings.NewReader(tc.body))
			c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{Group: &service.Group{Platform: service.PlatformGrok}})
			h := &OpenAIGatewayHandler{
				gatewayService: &service.OpenAIGatewayService{}, apiKeyService: &service.APIKeyService{},
				billingCacheService: &service.BillingCacheService{},
				concurrencyHelper:   &ConcurrencyHelper{concurrencyService: &service.ConcurrencyService{}},
			}
			h.GrokVoice(c, tc.endpoint)
			require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
		})
	}
}

func TestEntitlementAdmissionRejectsUnpreparedRequestAndReleasesSlot(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &service.APIKeyService{}
	c.Request = c.Request.WithContext(svc.WithEntitlementGenerationAdmission(c.Request.Context(), nil, 1))
	released := 0
	h := &OpenAIGatewayHandler{}
	admitted := h.admitEntitlementBeforeForward(c, &service.SubscriptionEntitlement{ID: 1}, func() { released++ }, false)
	require.False(t, admitted)
	require.Equal(t, 1, released)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
}
