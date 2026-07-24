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
	runtimeProvider    service.SubscriptionEntitlementsRuntimeProvider
	nowFunc            func() time.Time
}

type AdvanceEntitlementMonthlyCycleResponse struct {
	Entitlement           *dto.UserEntitlement `json:"entitlement"`
	PreviousExpiresAt     time.Time            `json:"previous_expires_at"`
	NewExpiresAt          time.Time            `json:"new_expires_at"`
	DeductedDays          int                  `json:"deducted_days"`
	DeductedSeconds       int64                `json:"deducted_seconds"`
	PreviousMonthlyUsage  float64              `json:"previous_monthly_usage_usd"`
	NewMonthlyWindowStart time.Time            `json:"new_monthly_window_start"`
}

func NewEntitlementHandler(
	entitlementService *service.SubscriptionEntitlementService,
	runtimeProvider service.SubscriptionEntitlementsRuntimeProvider,
) *EntitlementHandler {
	return &EntitlementHandler{
		entitlementService: entitlementService,
		runtimeProvider:    runtimeProvider,
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
	if !h.entitlementsEnabled(c) {
		response.Success(c, []dto.UserEntitlement{})
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
	if !h.entitlementsEnabled(c) {
		response.Success(c, []dto.UserEntitlement{})
		return
	}

	entitlements, err := h.entitlementService.ListActiveUserEntitlements(c.Request.Context(), subject.UserID, h.now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.UserEntitlementsFromService(entitlements, h.now()))
}

func (h *EntitlementHandler) entitlementsEnabled(c *gin.Context) bool {
	return h != nil &&
		h.runtimeProvider != nil &&
		h.runtimeProvider.GetSubscriptionEntitlementsRuntime(c.Request.Context()).Enabled
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

func (h *EntitlementHandler) Delete(c *gin.Context) {
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
	if err := h.entitlementService.RevokeUserEntitlement(c.Request.Context(), subject.UserID, entitlementID, h.now()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"success": true})
}

func (h *EntitlementHandler) AdvanceMonthlyCycle(c *gin.Context) {
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
	result, err := h.entitlementService.AdvanceMonthlyCycle(c.Request.Context(), subject.UserID, entitlementID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, advanceEntitlementMonthlyCycleResponseFromService(result, h.now()))
}

func advanceEntitlementMonthlyCycleResponseFromService(result *service.AdvanceEntitlementMonthlyCycleResult, now time.Time) *AdvanceEntitlementMonthlyCycleResponse {
	if result == nil {
		return nil
	}
	return &AdvanceEntitlementMonthlyCycleResponse{
		Entitlement:           dto.UserEntitlementFromService(result.Entitlement, now),
		PreviousExpiresAt:     result.PreviousExpiresAt,
		NewExpiresAt:          result.NewExpiresAt,
		DeductedDays:          result.DeductedDays,
		DeductedSeconds:       result.DeductedSeconds,
		PreviousMonthlyUsage:  result.PreviousMonthlyUsage,
		NewMonthlyWindowStart: result.NewMonthlyWindowStart,
	}
}

func (h *EntitlementHandler) now() time.Time {
	if h != nil && h.nowFunc != nil {
		return h.nowFunc()
	}
	return time.Now()
}
