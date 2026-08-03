//go:build unit

package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type connectivityProbeRuntimeStub struct {
	snapshot *service.ConnectivityProbeSnapshot
	clientIP netip.Addr
	verified bool
}

func (s *connectivityProbeRuntimeStub) ConnectivityProbeSnapshot() *service.ConnectivityProbeSnapshot {
	clone := *s.snapshot
	return &clone
}

func (s *connectivityProbeRuntimeStub) ConnectivityProbeRateLimitIP(*http.Request) (netip.Addr, bool) {
	return s.clientIP, s.verified
}

func TestConnectivityProbeRateLimiterReturnsFixed429PerVerifiedClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runtime := &connectivityProbeRuntimeStub{
		snapshot: &service.ConnectivityProbeSnapshot{IPRPM: 1, Burst: 1},
		clientIP: netip.MustParseAddr("8.8.8.8"),
		verified: true,
	}
	middleware, err := NewConnectivityProbeRateLimiter(runtime, []byte("test-only-secret"))
	require.NoError(t, err)

	router := gin.New()
	router.GET("/probe", middleware, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true, "client_ip": nil})
	})

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/probe", nil))
	require.Equal(t, http.StatusOK, first.Code)

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/probe", nil))
	require.Equal(t, http.StatusTooManyRequests, second.Code)
	require.Equal(t, "application/json; charset=utf-8", second.Header().Get("Content-Type"))
	require.Equal(t, "no-store, max-age=0", second.Header().Get("Cache-Control"))
	require.LessOrEqual(t, second.Body.Len(), 128)
	require.JSONEq(t, `{"ok":false,"client_ip":null}`, second.Body.String())

	runtime.clientIP = netip.MustParseAddr("1.1.1.1")
	third := httptest.NewRecorder()
	router.ServeHTTP(third, httptest.NewRequest(http.MethodGet, "/probe", nil))
	require.Equal(t, http.StatusOK, third.Code)
}

func TestConnectivityProbeRateLimiterFallsBackToDirectPeerThenGlobal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runtime := &connectivityProbeRuntimeStub{
		snapshot: &service.ConnectivityProbeSnapshot{IPRPM: 1, Burst: 1},
	}
	middleware, err := NewConnectivityProbeRateLimiter(runtime, []byte("test-only-secret"))
	require.NoError(t, err)
	router := gin.New()
	router.GET("/probe", middleware, func(c *gin.Context) { c.Status(http.StatusOK) })

	request := func(remoteAddr string) int {
		req := httptest.NewRequest(http.MethodGet, "/probe", nil)
		req.RemoteAddr = remoteAddr
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		return recorder.Code
	}

	require.Equal(t, http.StatusOK, request("192.0.2.10:1234"))
	require.Equal(t, http.StatusTooManyRequests, request("192.0.2.10:5678"))
	require.Equal(t, http.StatusOK, request("192.0.2.11:1234"))
	require.Equal(t, http.StatusOK, request("invalid"))
	require.Equal(t, http.StatusTooManyRequests, request("still-invalid"))
}
