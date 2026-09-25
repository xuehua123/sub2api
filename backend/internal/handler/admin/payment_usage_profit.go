package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"strconv"
	"time"
)

// Only three keys; concurrent cache misses are coalesced by snapshotCache.
var paymentUsageProfitCache = newSnapshotCache(30 * time.Second)

func (h *UpstreamConnectionHandler) GetPaymentUsageProfit(c *gin.Context) {
	days, err := strconv.Atoi(c.DefaultQuery("days", "30"))
	if err != nil || (days != 7 && days != 30 && days != 90) {
		response.BadRequest(c, "days must be 7, 30 or 90")
		return
	}
	entry, hit, err := paymentUsageProfitCache.GetOrLoad(strconv.Itoa(days), func() (any, error) { return h.service.GetPaymentUsageProfit(c.Request.Context(), days) })
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("X-Snapshot-Cache", cacheStatusValue(hit))
	response.Success(c, entry.Payload)
}
