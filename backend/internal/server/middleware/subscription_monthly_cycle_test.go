package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestMonthlyCycleBillableRequest(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		want         bool
	}{
		{"POST", "/v1/responses", true}, {"POST", "/v1/messages", true}, {"POST", "/v1/chat/completions", true},
		{"POST", "/v1beta/models/gemini:generateContent", true}, {"POST", "/v1beta/models/gemini:streamGenerateContent", true},
		{"POST", "/v1/images/batches", true}, {"POST", "/v1/images/generations/async", true},
		{"GET", "/v1/usage", false}, {"GET", "/v1/sub2api/billing", false}, {"GET", "/v1/images/tasks/42", false},
		{"POST", "/v1/messages/count_tokens", false}, {"POST", "/v1/responses/input_tokens", false},
		{"GET", "/v1/models", false}, {"POST", "/v1/images/batches/42/cancel", false}, {"GET", "/v1/videos/42", false},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(tc.method, tc.path, nil)
			require.Equal(t, tc.want, isMonthlyCycleBillableRequest(c))
		})
	}
}

func TestRealtimeHandshakeDefersCycleUntilGeneration(t *testing.T) {
	for _, path := range []string{"/v1/realtime", "/realtime"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", path, nil)
		require.True(t, isMonthlyCycleWebSocketRequest(c))
		require.False(t, isMonthlyCycleBillableRequest(c))
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/v1/images/tasks/42", nil)
	require.False(t, isMonthlyCycleWebSocketRequest(c))
}
