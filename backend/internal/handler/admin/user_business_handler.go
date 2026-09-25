package admin

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type UserBusinessHandler struct {
	service    *service.UserBusinessService
	cache      *snapshotCache
	gate       chan struct{}
	mu         sync.Mutex
	fxRevision atomic.Uint64
}

func NewUserBusinessHandler(s *service.UserBusinessService) *UserBusinessHandler {
	return &UserBusinessHandler{service: s, cache: newSnapshotCache(30 * time.Second), gate: make(chan struct{}, 2)}
}
func (h *UserBusinessHandler) query(c *gin.Context) (service.UserBusinessQuery, bool) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		response.BadRequest(c, "Invalid page")
		return service.UserBusinessQuery{}, false
	}
	size, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil {
		response.BadRequest(c, "Invalid page size")
		return service.UserBusinessQuery{}, false
	}
	rate := 0.0
	if raw := c.Query("usd_cny"); raw != "" {
		rate, err = strconv.ParseFloat(raw, 64)
		if err != nil || rate <= 0 {
			response.BadRequest(c, "Invalid rate")
			return service.UserBusinessQuery{}, false
		}
	}
	q, err := h.service.Normalize(c.Request.Context(), service.UserBusinessParams{StartDate: c.Query("start_date"), EndDate: c.Query("end_date"), Search: c.Query("search"), Filter: c.DefaultQuery("filter", "used"), Sort: c.Query("sort"), Order: c.Query("order"), Page: page, PageSize: size, USDCNY: rate, CostMode: c.Query("cost_mode")})
	if err != nil {
		response.ErrorFrom(c, err)
		return q, false
	}
	return q, true
}
func (h *UserBusinessHandler) respond(c *gin.Context, q service.UserBusinessQuery) {
	// Bound retained snapshots; cache misses share work, and at most two report scans run concurrently.
	h.mu.Lock()
	h.cache.mu.Lock()
	if len(h.cache.items) >= 32 {
		h.cache.items = make(map[string]snapshotCacheEntry)
	}
	h.cache.mu.Unlock()
	h.mu.Unlock()
	entry, hit, err := h.cache.GetOrLoad(q.CacheKey()+"|fx:"+strconv.FormatUint(h.fxRevision.Load(), 10), func() (any, error) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()
		select {
		case h.gate <- struct{}{}:
			defer func() { <-h.gate }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		var data json.RawMessage
		var err error
		if q.UserID > 0 {
			data, err = h.service.Detail(ctx, q, q.UserID)
		} else {
			data, err = h.service.Report(ctx, q)
		}
		return data, err
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("X-Snapshot-Cache", cacheStatusValue(hit))
	response.Success(c, entry.Payload)
}
func (h *UserBusinessHandler) List(c *gin.Context) {
	q, ok := h.query(c)
	if ok {
		h.respond(c, q)
	}
}
func (h *UserBusinessHandler) Detail(c *gin.Context) {
	q, ok := h.query(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid user")
		return
	}
	q.UserID = id
	h.respond(c, q)
}

func (h *UserBusinessHandler) ListFX(c *gin.Context) {
	data, err := h.service.ListFX(c.Request.Context(), service.UserBusinessParams{StartDate: c.Query("start_date"), EndDate: c.Query("end_date")})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, data)
}
func (h *UserBusinessHandler) InsertFX(c *gin.Context) {
	var input service.UserBusinessFX
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid FX input")
		return
	}
	if err := h.service.InsertFX(c.Request.Context(), input); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.fxRevision.Add(1)
	response.Success(c, gin.H{"saved": true})
}
