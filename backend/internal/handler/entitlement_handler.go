package handler

import (
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type EntitlementHandler struct {
	entitlementService *service.SubscriptionEntitlementService
	nowFunc            func() time.Time
}

func NewEntitlementHandler(entitlementService *service.SubscriptionEntitlementService) *EntitlementHandler {
	return &EntitlementHandler{
		entitlementService: entitlementService,
		nowFunc:            time.Now,
	}
}

func (h *EntitlementHandler) SetNowFunc(fn func() time.Time) {
	if fn != nil {
		h.nowFunc = fn
	}
}

func (h *EntitlementHandler) List(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	entitlements, err := h.entitlementService.ListUserEntitlements(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.UserEntitlementsFromService(entitlements, h.now()))
}

func (h *EntitlementHandler) GetActive(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	entitlements, err := h.entitlementService.ListActiveUserEntitlements(c.Request.Context(), subject.UserID, h.now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.UserEntitlementsFromService(entitlements, h.now()))
}

func (h *EntitlementHandler) GetProgress(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	entitlementID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || entitlementID <= 0 {
		response.BadRequest(c, "Invalid entitlement ID")
		return
	}
	entitlement, err := h.entitlementService.GetUserEntitlementByID(c.Request.Context(), subject.UserID, entitlementID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.UserEntitlementFromService(entitlement, h.now()))
}

func (h *EntitlementHandler) now() time.Time {
	if h != nil && h.nowFunc != nil {
		return h.nowFunc()
	}
	return time.Now()
}
