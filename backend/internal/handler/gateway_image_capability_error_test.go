package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayResponsesFailoverExhausted_ImageCapabilityUnavailableReturns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	h := &GatewayHandler{}
	h.handleResponsesFailoverExhausted(c, openAIImageCapabilityUnavailableFailoverErr(), false)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Contains(t, w.Body.String(), service.OpenAIImageGenerationUnavailableClientMessage())
	require.NotContains(t, w.Body.String(), "All available accounts exhausted")
}

func TestGatewayChatCompletionsFailoverExhausted_ImageCapabilityUnavailableReturns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	h := &GatewayHandler{}
	h.handleCCFailoverExhausted(c, openAIImageCapabilityUnavailableFailoverErr(), false)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Contains(t, w.Body.String(), service.OpenAIImageGenerationUnavailableClientMessage())
	require.NotContains(t, w.Body.String(), "All available accounts exhausted")
}

func openAIImageCapabilityUnavailableFailoverErr() *service.UpstreamFailoverError {
	return &service.UpstreamFailoverError{
		StatusCode: http.StatusForbidden,
		ResponseBody: []byte(`{
			"error": {
				"message": "Image generation is not enabled for this group"
			}
		}`),
	}
}
