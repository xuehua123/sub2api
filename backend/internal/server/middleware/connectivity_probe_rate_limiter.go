package middleware

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/connectivity"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const connectivityProbeRateLimiterCapacity = 32768

type connectivityProbeRuntime interface {
	ConnectivityProbeSnapshot() *service.ConnectivityProbeSnapshot
	ConnectivityProbeRateLimitIP(*http.Request) (netip.Addr, bool)
}

func NewConnectivityProbeRateLimiter(runtime connectivityProbeRuntime, secret []byte) (gin.HandlerFunc, error) {
	if runtime == nil {
		return nil, errors.New("connectivity probe runtime is required")
	}
	limiter, err := connectivity.NewProbeRateLimiter(connectivity.ProbeRateLimiterOptions{
		Secret:   secret,
		Capacity: connectivityProbeRateLimiterCapacity,
	})
	if err != nil {
		return nil, err
	}

	return func(c *gin.Context) {
		snapshot := runtime.ConnectivityProbeSnapshot()
		identity := connectivityProbeRateLimitIdentity(runtime, c.Request)
		if snapshot == nil || !limiter.Allow(identity, snapshot.IPRPM, snapshot.Burst) {
			c.Header("Cache-Control", "no-store, max-age=0")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"ok":        false,
				"client_ip": nil,
			})
			return
		}
		c.Next()
	}, nil
}

func connectivityProbeRateLimitIdentity(runtime connectivityProbeRuntime, req *http.Request) string {
	if addr, ok := runtime.ConnectivityProbeRateLimitIP(req); ok && addr.IsValid() {
		return "verified:" + addr.Unmap().String()
	}
	if req != nil {
		host, _, err := net.SplitHostPort(strings.TrimSpace(req.RemoteAddr))
		if err == nil {
			if addr, parseErr := netip.ParseAddr(host); parseErr == nil {
				return "peer:" + addr.Unmap().String()
			}
		}
	}
	return "global"
}
