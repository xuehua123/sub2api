package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// channelMonitorProbeFailedAccounts seeds a new request's failover set with
// accounts that produced unusable responses in earlier monitor attempts.
func channelMonitorProbeFailedAccounts(c *gin.Context) map[int64]struct{} {
	if c == nil || c.Request == nil {
		return make(map[int64]struct{})
	}
	return service.ChannelMonitorProbeExcludedAccounts(c.Request.Context())
}

func seedChannelMonitorProbeFailedAccounts(c *gin.Context, failedAccountIDs map[int64]struct{}) {
	if c == nil || c.Request == nil || !service.IsChannelMonitorProbe(c.Request.Context()) {
		return
	}
	for accountID := range service.ChannelMonitorProbeExcludedAccounts(c.Request.Context()) {
		failedAccountIDs[accountID] = struct{}{}
	}
}

// recordChannelMonitorSelectedAccount exposes the chosen account only to a
// server-authenticated monitor probe. Normal API clients never receive it.
func recordChannelMonitorSelectedAccount(c *gin.Context, accountID int64) {
	if c == nil || c.Request == nil || accountID <= 0 || !service.IsChannelMonitorProbe(c.Request.Context()) {
		return
	}
	c.Header(service.ChannelMonitorProbeSelectedAccountHeaderName, strconv.FormatInt(accountID, 10))
}
