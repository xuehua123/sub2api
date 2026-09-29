package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"strconv"
	"time"
)

// Concurrent requests for the same calendar range share one aggregation.
var paymentUsageProfitCache = newSnapshotCache(30 * time.Second)

func (h *UpstreamConnectionHandler) GetPaymentUsageProfit(c *gin.Context) {
	days, err := strconv.Atoi(c.DefaultQuery("days", "30"))
	if err != nil || (days != 1 && days != 7 && days != 30 && days != 90) {
		response.BadRequest(c, "days must be 1, 7, 30 or 90")
		return
	}
	now := timezone.Now()
	startDate, endDate := c.Query("start_date"), c.Query("end_date")
	rangeValue, err := service.ParsePaymentDateRange(startDate, endDate, days, now)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	key := rangeValue.Start.Format(time.DateOnly) + ":" + rangeValue.End.Format(time.DateOnly)
	paymentUsageProfitCache.mu.Lock()
	for k, entry := range paymentUsageProfitCache.items {
		if !entry.ExpiresAt.After(now) {
			delete(paymentUsageProfitCache.items, k)
		}
	}
	paymentUsageProfitCache.mu.Unlock()
	entry, hit, err := paymentUsageProfitCache.GetOrLoad(key, func() (any, error) {
		return h.service.GetPaymentUsageProfit(c.Request.Context(), days, startDate, endDate)
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("X-Snapshot-Cache", cacheStatusValue(hit))
	response.Success(c, entry.Payload)
}
