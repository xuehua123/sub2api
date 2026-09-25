package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/gin-gonic/gin"
	"strconv"
	"time"
)

// Four range keys per local day; concurrent requests share one aggregation.
var paymentUsageProfitCache = newSnapshotCache(30 * time.Second)

func (h *UpstreamConnectionHandler) GetPaymentUsageProfit(c *gin.Context) {
	days, err := strconv.Atoi(c.DefaultQuery("days", "30"))
	if err != nil || (days != 1 && days != 7 && days != 30 && days != 90) {
		response.BadRequest(c, "days must be 1, 7, 30 or 90")
		return
	}
	now := timezone.Now()
	key := now.Format("2006-01-02") + ":" + strconv.Itoa(days)
	paymentUsageProfitCache.mu.Lock()
	for k, entry := range paymentUsageProfitCache.items {
		if !entry.ExpiresAt.After(now) {
			delete(paymentUsageProfitCache.items, k)
		}
	}
	paymentUsageProfitCache.mu.Unlock()
	entry, hit, err := paymentUsageProfitCache.GetOrLoad(key, func() (any, error) { return h.service.GetPaymentUsageProfit(c.Request.Context(), days) })
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("X-Snapshot-Cache", cacheStatusValue(hit))
	response.Success(c, entry.Payload)
}
