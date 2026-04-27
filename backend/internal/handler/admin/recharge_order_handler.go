package admin

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type RechargeOrderHandler struct {
	rewardService *service.ReferralRewardService
}

func NewRechargeOrderHandler(rewardService *service.ReferralRewardService) *RechargeOrderHandler {
	return &RechargeOrderHandler{rewardService: rewardService}
}

type CreditRechargeOrderRequest struct {
	ExternalOrderID       string  `json:"external_order_id" binding:"required"`
	Provider              string  `json:"provider" binding:"required"`
	Channel               string  `json:"channel"`
	Currency              string  `json:"currency" binding:"required"`
	UserID                int64   `json:"user_id" binding:"required,gt=0"`
	GrossAmount           float64 `json:"gross_amount"`
	DiscountAmount        float64 `json:"discount_amount"`
	PaidAmount            float64 `json:"paid_amount" binding:"required,gt=0"`
	GiftBalanceAmount     float64 `json:"gift_balance_amount"`
	CreditedBalanceAmount float64 `json:"credited_balance_amount" binding:"gte=0"`
	MetadataJSON          string  `json:"metadata_json"`
	Notes                 string  `json:"notes"`
}

func (h *RechargeOrderHandler) Credit(c *gin.Context) {
	if _, ok := middleware2.GetAuthSubjectFromContext(c); !ok {
		response.Unauthorized(c, "Admin not authenticated")
		return
	}

	if h.rewardService == nil {
		response.InternalError(c, "recharge reward service not configured")
		return
	}

	var req CreditRechargeOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	executeAdminIdempotentJSON(c, "admin.recharge_orders.credit", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.rewardService.CreditRechargeOrder(ctx, &service.RechargeCreditInput{
			UserID:                req.UserID,
			ExternalOrderID:       strings.TrimSpace(req.ExternalOrderID),
			Provider:              strings.TrimSpace(req.Provider),
			Channel:               req.Channel,
			Currency:              strings.TrimSpace(req.Currency),
			GrossAmount:           req.GrossAmount,
			DiscountAmount:        req.DiscountAmount,
			PaidAmount:            req.PaidAmount,
			GiftBalanceAmount:     req.GiftBalanceAmount,
			CreditedBalanceAmount: req.CreditedBalanceAmount,
			IdempotencyKey:        c.GetHeader("Idempotency-Key"),
			MetadataJSON:          req.MetadataJSON,
			Notes:                 req.Notes,
		})
	})
}
