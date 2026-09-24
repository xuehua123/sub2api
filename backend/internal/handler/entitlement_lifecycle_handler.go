package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"strconv"
)

func (h *EntitlementHandler) lifecycleSubject(c *gin.Context) (int64, int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return 0, 0, false
	}
	if !h.entitlementsEnabled(c) {
		response.NotFound(c, "Entitlements are unavailable")
		return 0, 0, false
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid entitlement ID")
		return 0, 0, false
	}
	return subject.UserID, id, true
}

func (h *EntitlementHandler) SetAutoAdvanceMonthly(c *gin.Context) {
	userID, id, ok := h.lifecycleSubject(c)
	if !ok {
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		response.BadRequest(c, "enabled is required")
		return
	}
	ent, err := h.entitlementService.SetAutoAdvanceMonthly(c.Request.Context(), userID, id, *req.Enabled)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.UserEntitlementFromService(ent, h.now()))
}

func (h *EntitlementHandler) Events(c *gin.Context) {
	userID, id, ok := h.lifecycleSubject(c)
	if !ok {
		return
	}
	var before int64
	if raw := c.Query("before_id"); raw != "" {
		var err error
		before, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || before <= 0 {
			response.BadRequest(c, "Invalid event cursor")
			return
		}
	}
	events, err := h.entitlementService.EntitlementEvents(c.Request.Context(), userID, id, before)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, events)
}

func (h *EntitlementHandler) AcknowledgeEvent(c *gin.Context) {
	userID, id, ok := h.lifecycleSubject(c)
	if !ok {
		return
	}
	var req struct {
		EventID int64 `json:"event_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.EventID <= 0 {
		response.BadRequest(c, "Invalid event ID")
		return
	}
	if err := h.entitlementService.AcknowledgeEntitlementEvent(c.Request.Context(), userID, id, req.EventID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}
