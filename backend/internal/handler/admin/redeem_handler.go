package admin

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RedeemHandler handles admin redeem code management
type RedeemHandler struct {
	adminService          service.AdminService
	redeemService         *service.RedeemService
	referralRewardService referralRewardCreditor
}

type referralRewardCreditor interface {
	CreditRechargeOrder(context.Context, *service.RechargeCreditInput) (*service.RechargeCreditResult, error)
}

// NewRedeemHandler creates a new admin redeem handler
func NewRedeemHandler(adminService service.AdminService, redeemService *service.RedeemService, referralRewardService *service.ReferralRewardService) *RedeemHandler {
	return &RedeemHandler{
		adminService:          adminService,
		redeemService:         redeemService,
		referralRewardService: referralRewardService,
	}
}

// GenerateRedeemCodesRequest represents generate redeem codes request
type GenerateRedeemCodesRequest struct {
	Count         int        `json:"count" binding:"required,min=1,max=100"`
	Type          string     `json:"type" binding:"required,oneof=balance concurrency subscription invitation"`
	Value         float64    `json:"value"`
	GroupID       *int64     `json:"group_id"`      // 订阅类型必填
	PlanID        *int64     `json:"plan_id"`       // entitlement v2 subscription plan
	ValidityDays  int        `json:"validity_days"` // 订阅类型使用，正数增加/负数退款扣减
	ExpiresAt     *time.Time `json:"expires_at"`
	ExpiresInDays *int       `json:"expires_in_days" binding:"omitempty,min=1,max=3650"`
}

// CreateAndRedeemCodeRequest represents creating a fixed code and redeeming it for a target user.
// Type 为 omitempty 而非 required 是为了向后兼容旧版调用方（不传 type 时默认 balance）。
type CreateAndRedeemCodeRequest struct {
	Code          string     `json:"code" binding:"required,min=3,max=128"`
	Type          string     `json:"type" binding:"omitempty,oneof=balance concurrency subscription invitation"` // 不传时默认 balance（向后兼容）
	Value         float64    `json:"value" binding:"required"`
	UserID        int64      `json:"user_id" binding:"required,gt=0"`
	GroupID       *int64     `json:"group_id"`      // subscription 类型必填
	PlanID        *int64     `json:"plan_id"`       // entitlement v2 subscription plan
	ValidityDays  int        `json:"validity_days"` // subscription 类型：正数增加，负数退款扣减
	Notes         string     `json:"notes"`
	ExpiresAt     *time.Time `json:"expires_at"`
	ExpiresInDays *int       `json:"expires_in_days" binding:"omitempty,min=1,max=3650"`
}

func resolveRedeemCodeExpiresAt(expiresAt *time.Time, expiresInDays *int) (*time.Time, error) {
	if expiresAt != nil && expiresInDays != nil {
		return nil, infraerrors.BadRequest("REDEEM_CODE_EXPIRY_CONFLICT", "expires_at and expires_in_days cannot both be set")
	}

	now := time.Now().UTC()
	if expiresInDays != nil {
		if *expiresInDays <= 0 {
			return nil, infraerrors.BadRequest("REDEEM_CODE_EXPIRES_IN_DAYS_INVALID", "expires_in_days must be greater than zero")
		}
		expires := now.AddDate(0, 0, *expiresInDays)
		return &expires, nil
	}
	if expiresAt == nil {
		return nil, nil
	}

	expires := expiresAt.UTC()
	if !expires.After(now) {
		return nil, infraerrors.BadRequest("REDEEM_CODE_EXPIRES_AT_INVALID", "expires_at must be in the future")
	}
	return &expires, nil
}

// List handles listing all redeem codes with pagination
// GET /api/v1/admin/redeem-codes
func (h *RedeemHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	codeType := c.Query("type")
	status := c.Query("status")
	search := c.Query("search")
	sortBy := c.DefaultQuery("sort_by", "id")
	sortOrder := c.DefaultQuery("sort_order", "desc")
	// 标准化和验证 search 参数
	search = strings.TrimSpace(search)
	if len(search) > 100 {
		search = search[:100]
	}

	codes, total, err := h.adminService.ListRedeemCodes(c.Request.Context(), page, pageSize, codeType, status, search, sortBy, sortOrder)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.AdminRedeemCode, 0, len(codes))
	for i := range codes {
		out = append(out, *dto.RedeemCodeFromServiceAdmin(&codes[i]))
	}
	response.Paginated(c, out, total, page, pageSize)
}

// GetByID handles getting a redeem code by ID
// GET /api/v1/admin/redeem-codes/:id
func (h *RedeemHandler) GetByID(c *gin.Context) {
	codeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid redeem code ID")
		return
	}

	code, err := h.adminService.GetRedeemCode(c.Request.Context(), codeID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.RedeemCodeFromServiceAdmin(code))
}

// Generate handles generating new redeem codes
// POST /api/v1/admin/redeem-codes/generate
func (h *RedeemHandler) Generate(c *gin.Context) {
	var req GenerateRedeemCodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	expiresAt, err := resolveRedeemCodeExpiresAt(req.ExpiresAt, req.ExpiresInDays)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	executeAdminIdempotentJSON(c, "admin.redeem_codes.generate", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		codes, execErr := h.adminService.GenerateRedeemCodes(ctx, &service.GenerateRedeemCodesInput{
			Count:        req.Count,
			Type:         req.Type,
			Value:        req.Value,
			GroupID:      req.GroupID,
			PlanID:       req.PlanID,
			ValidityDays: req.ValidityDays,
			ExpiresAt:    expiresAt,
		})
		if execErr != nil {
			return nil, execErr
		}

		out := make([]dto.AdminRedeemCode, 0, len(codes))
		for i := range codes {
			out = append(out, *dto.RedeemCodeFromServiceAdmin(&codes[i]))
		}
		return out, nil
	})
}

// CreateAndRedeem creates a fixed redeem code and redeems it for a target user in one step.
// POST /api/v1/admin/redeem-codes/create-and-redeem
func (h *RedeemHandler) CreateAndRedeem(c *gin.Context) {
	if h.redeemService == nil {
		response.InternalError(c, "redeem service not configured")
		return
	}

	var req CreateAndRedeemCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	// 向后兼容：旧版调用方（如 Sub2ApiPay）不传 type 字段，默认当作 balance 充值处理。
	// 请勿删除此默认值逻辑，否则会导致旧版调用方 400 报错。
	if req.Type == "" {
		req.Type = "balance"
	}

	sourceCtx := service.DetectRedeemSourceContext(service.RedeemSourceDetectionInput{
		IdempotencyKey: c.GetHeader("Idempotency-Key"),
		Code:           req.Code,
		Type:           req.Type,
		GroupID:        req.GroupID,
		ValidityDays:   req.ValidityDays,
		Value:          req.Value,
	})

	if err := validateCreateAndRedeemSubscriptionRequest(req, sourceCtx); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	expiresAt, err := resolveRedeemCodeExpiresAt(req.ExpiresAt, req.ExpiresInDays)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	executeAdminIdempotentJSON(c, "admin.redeem_codes.create_and_redeem", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		existing, err := h.redeemService.GetByCode(ctx, req.Code)
		if err == nil {
			return h.resolveCreateAndRedeemExisting(ctx, existing, req, sourceCtx)
		}
		if !errors.Is(err, service.ErrRedeemCodeNotFound) {
			return nil, err
		}

		createErr := h.redeemService.CreateCode(ctx, &service.RedeemCode{
			Code:         req.Code,
			Type:         req.Type,
			Value:        req.Value,
			Status:       service.StatusUnused,
			Notes:        req.Notes,
			GroupID:      req.GroupID,
			PlanID:       req.PlanID,
			ValidityDays: req.ValidityDays,
			ExpiresAt:    expiresAt,
		})
		if createErr != nil {
			// Unique code race: if code now exists, use idempotent semantics by used_by.
			existingAfterCreateErr, getErr := h.redeemService.GetByCode(ctx, req.Code)
			if getErr == nil {
				return h.resolveCreateAndRedeemExisting(ctx, existingAfterCreateErr, req, sourceCtx)
			}
			return nil, createErr
		}

		redeemed, redeemErr := h.redeemService.RedeemWithOptions(ctx, service.RedeemInput{
			UserID: req.UserID,
			Code:   req.Code,
			Source: sourceCtx,
		})
		if redeemErr != nil {
			return nil, redeemErr
		}
		if rewardErr := h.syncSub2ApiPayReferralReward(ctx, req, sourceCtx); rewardErr != nil {
			return nil, rewardErr
		}
		return gin.H{"redeem_code": dto.RedeemCodeFromServiceAdmin(redeemed)}, nil
	})
}

func (h *RedeemHandler) resolveCreateAndRedeemExisting(ctx context.Context, existing *service.RedeemCode, req CreateAndRedeemCodeRequest, sourceCtx service.RedeemSourceContext) (any, error) {
	if existing == nil {
		return nil, service.ErrRedeemCodeConflict
	}
	if err := validateCreateAndRedeemReplay(existing, req, sourceCtx); err != nil {
		return nil, err
	}

	// If previous run created the code but crashed before redeem, redeem it now.
	if existing.IsExpired() {
		return nil, service.ErrRedeemCodeExpired
	}
	if existing.CanUse() {
		redeemed, err := h.redeemService.RedeemWithOptions(ctx, service.RedeemInput{
			UserID: req.UserID,
			Code:   existing.Code,
			Source: sourceCtx,
		})
		if err == nil {
			if rewardErr := h.syncSub2ApiPayReferralReward(ctx, req, sourceCtx); rewardErr != nil {
				return nil, rewardErr
			}
			return gin.H{"redeem_code": dto.RedeemCodeFromServiceAdmin(redeemed)}, nil
		}
		if !errors.Is(err, service.ErrRedeemCodeUsed) {
			return nil, err
		}
		latest, getErr := h.redeemService.GetByCode(ctx, existing.Code)
		if getErr == nil {
			existing = latest
		}
	}

	if existing.UsedBy != nil && *existing.UsedBy == req.UserID {
		if rewardErr := h.syncSub2ApiPayReferralReward(ctx, req, sourceCtx); rewardErr != nil {
			return nil, rewardErr
		}
		return gin.H{"redeem_code": dto.RedeemCodeFromServiceAdmin(existing)}, nil
	}

	return nil, infraerrors.Conflict("REDEEM_CODE_CONFLICT", "redeem code already used by another user")
}

func validateCreateAndRedeemSubscriptionRequest(req CreateAndRedeemCodeRequest, sourceCtx service.RedeemSourceContext) error {
	if req.Type != service.RedeemTypeSubscription {
		return nil
	}
	if sourceCtx.HasSub2PaymentPageSuffixMismatch() {
		return service.ErrRedeemCodeConflict
	}
	if req.GroupID != nil && *req.GroupID <= 0 {
		return infraerrors.BadRequest("REDEEM_CODE_INVALID", "group_id must be greater than zero for subscription type")
	}
	if req.PlanID != nil && *req.PlanID <= 0 {
		return infraerrors.BadRequest("REDEEM_CODE_INVALID", "plan_id must be greater than zero for subscription type")
	}
	if req.PlanID != nil && req.GroupID != nil {
		return infraerrors.BadRequest("REDEEM_CODE_INVALID", "plan_id and group_id cannot both be set for subscription type")
	}
	if req.PlanID != nil && req.ValidityDays < 0 {
		return infraerrors.BadRequest("REDEEM_CODE_INVALID", "plan subscription redeem code cannot reduce validity days")
	}
	if req.PlanID == nil && req.GroupID == nil {
		return infraerrors.BadRequest("REDEEM_CODE_INVALID", "plan_id or group_id is required for subscription type")
	}
	if req.GroupID != nil && req.ValidityDays == 0 {
		return infraerrors.BadRequest("REDEEM_CODE_INVALID", "validity_days must not be zero for subscription group redeem")
	}
	if sourceCtx.IsSub2PaymentPageLegacy() && req.ValidityDays <= 0 {
		return infraerrors.BadRequest("REDEEM_CODE_INVALID", "validity_days must be positive for payment page subscription redeem")
	}
	return nil
}

func validateCreateAndRedeemReplay(existing *service.RedeemCode, req CreateAndRedeemCodeRequest, sourceCtx service.RedeemSourceContext) error {
	if existing == nil {
		return service.ErrRedeemCodeConflict
	}
	if sourceCtx.HasSub2PaymentPageSuffixMismatch() {
		return service.ErrRedeemCodeConflict
	}
	if strings.TrimSpace(existing.Type) != strings.TrimSpace(req.Type) {
		return service.ErrRedeemCodeConflict
	}
	if !redeemFloatEqual(existing.Value, req.Value) {
		return service.ErrRedeemCodeConflict
	}
	if !sameInt64Ptr(existing.GroupID, req.GroupID) {
		return service.ErrRedeemCodeConflict
	}
	if !sameCreateAndRedeemReplayPlanID(existing.PlanID, req.PlanID, sourceCtx) {
		return service.ErrRedeemCodeConflict
	}
	if existing.ValidityDays != req.ValidityDays {
		return service.ErrRedeemCodeConflict
	}
	return nil
}

func redeemFloatEqual(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9
}

func sameInt64Ptr(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameCreateAndRedeemReplayPlanID(existing, requested *int64, sourceCtx service.RedeemSourceContext) bool {
	if sameInt64Ptr(existing, requested) {
		return true
	}
	return sourceCtx.IsSub2PaymentPageLegacy() && requested == nil && existing != nil
}

func (h *RedeemHandler) syncSub2ApiPayReferralReward(ctx context.Context, req CreateAndRedeemCodeRequest, sourceCtx service.RedeemSourceContext) error {
	if h.referralRewardService == nil {
		return nil
	}
	input := buildSub2ApiPayReferralCreditInput(req, sourceCtx, time.Now())
	if input == nil {
		return nil
	}
	_, err := h.referralRewardService.CreditRechargeOrder(ctx, input)
	return err
}

func buildSub2ApiPayReferralCreditInput(req CreateAndRedeemCodeRequest, sourceCtx service.RedeemSourceContext, paidAt time.Time) *service.RechargeCreditInput {
	if req.Type != service.RedeemTypeBalance && req.Type != service.RedeemTypeSubscription {
		return nil
	}
	if req.Value <= 0 {
		return nil
	}
	if !sourceCtx.IsSub2PaymentPageLegacy() {
		return nil
	}

	metadata := map[string]any{
		"source":            "sub2apipay_create_and_redeem",
		"redeem_code":       req.Code,
		"redeem_type":       req.Type,
		"external_order_id": sourceCtx.ExternalOrderID,
	}
	if req.GroupID != nil {
		metadata["group_id"] = *req.GroupID
	}
	if req.ValidityDays != 0 {
		metadata["validity_days"] = req.ValidityDays
	}
	if strings.TrimSpace(req.Notes) != "" {
		metadata["notes"] = strings.TrimSpace(req.Notes)
	}
	metadataJSON, _ := json.Marshal(metadata)

	return &service.RechargeCreditInput{
		UserID:                req.UserID,
		ExternalOrderID:       sourceCtx.ExternalOrderID,
		Provider:              "sub2apipay",
		Channel:               req.Type,
		Currency:              "CNY",
		GrossAmount:           req.Value,
		PaidAmount:            req.Value,
		CreditedBalanceAmount: 0,
		SkipBalanceCredit:     true,
		IdempotencyKey:        "sub2apipay:" + sourceCtx.ExternalOrderID + ":referral",
		MetadataJSON:          string(metadataJSON),
		Notes:                 strings.TrimSpace(req.Notes),
		PaidAt:                &paidAt,
	}
}

// Delete handles deleting a redeem code
// DELETE /api/v1/admin/redeem-codes/:id
func (h *RedeemHandler) Delete(c *gin.Context) {
	codeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid redeem code ID")
		return
	}

	err = h.adminService.DeleteRedeemCode(c.Request.Context(), codeID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Redeem code deleted successfully"})
}

// BatchDelete handles batch deleting redeem codes
// POST /api/v1/admin/redeem-codes/batch-delete
func (h *RedeemHandler) BatchDelete(c *gin.Context) {
	var req struct {
		IDs []int64 `json:"ids" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	deleted, err := h.adminService.BatchDeleteRedeemCodes(c.Request.Context(), req.IDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"deleted": deleted,
		"message": "Redeem codes deleted successfully",
	})
}

// BatchUpdate handles batch updating redeem codes
// POST /api/v1/admin/redeem-codes/batch-update
func (h *RedeemHandler) BatchUpdate(c *gin.Context) {
	if h.redeemService == nil {
		response.InternalError(c, "redeem service not configured")
		return
	}

	var req dto.BatchUpdateRedeemCodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	result, err := h.redeemService.BatchUpdate(c.Request.Context(), &service.RedeemCodeBatchUpdateInput{
		IDs:    req.IDs,
		Fields: redeemBatchUpdateFieldsFromDTO(req.Fields),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"updated": result.Updated,
		"message": "Redeem codes updated successfully",
	})
}

func redeemBatchUpdateFieldsFromDTO(in dto.BatchUpdateRedeemCodeFields) service.RedeemCodeBatchUpdateFields {
	out := service.RedeemCodeBatchUpdateFields{
		Status: in.Status,
		Notes:  in.Notes,
		Type:   in.Type,
		Value:  in.Value,
	}
	if in.ExpiresAt.Set {
		out.ExpiresAt = service.NullableTimeUpdate{Set: true, Value: in.ExpiresAt.Value}
	}
	if in.GroupID.Set {
		out.GroupID = service.NullableInt64Update{Set: true, Value: in.GroupID.Value}
	}
	return out
}

// Expire handles expiring a redeem code
// POST /api/v1/admin/redeem-codes/:id/expire
func (h *RedeemHandler) Expire(c *gin.Context) {
	codeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid redeem code ID")
		return
	}

	code, err := h.adminService.ExpireRedeemCode(c.Request.Context(), codeID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.RedeemCodeFromServiceAdmin(code))
}

// GetStats handles getting redeem code statistics
// GET /api/v1/admin/redeem-codes/stats
func (h *RedeemHandler) GetStats(c *gin.Context) {
	// Return mock data for now
	response.Success(c, gin.H{
		"total_codes":             0,
		"active_codes":            0,
		"used_codes":              0,
		"expired_codes":           0,
		"total_value_distributed": 0.0,
		"by_type": gin.H{
			"balance":     0,
			"concurrency": 0,
			"trial":       0,
		},
	})
}

// Export handles exporting redeem codes to CSV
// GET /api/v1/admin/redeem-codes/export
func (h *RedeemHandler) Export(c *gin.Context) {
	codeType := c.Query("type")
	status := c.Query("status")
	search := strings.TrimSpace(c.Query("search"))
	sortBy := c.DefaultQuery("sort_by", "id")
	sortOrder := c.DefaultQuery("sort_order", "desc")
	if len(search) > 100 {
		search = search[:100]
	}

	// Get all codes without pagination (use large page size)
	codes, _, err := h.adminService.ListRedeemCodes(c.Request.Context(), 1, 10000, codeType, status, search, sortBy, sortOrder)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// Create CSV buffer
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write header
	if err := writer.Write([]string{"id", "code", "type", "value", "status", "used_by", "used_by_email", "used_at", "expires_at", "created_at"}); err != nil {
		response.InternalError(c, "Failed to export redeem codes: "+err.Error())
		return
	}

	// Write data rows
	for _, code := range codes {
		usedBy := ""
		if code.UsedBy != nil {
			usedBy = fmt.Sprintf("%d", *code.UsedBy)
		}
		usedByEmail := ""
		if code.User != nil {
			usedByEmail = code.User.Email
		}
		usedAt := ""
		if code.UsedAt != nil {
			usedAt = code.UsedAt.Format("2006-01-02 15:04:05")
		}
		expiresAt := ""
		if code.ExpiresAt != nil {
			expiresAt = code.ExpiresAt.Format("2006-01-02 15:04:05")
		}
		if err := writer.Write([]string{
			fmt.Sprintf("%d", code.ID),
			code.Code,
			code.Type,
			fmt.Sprintf("%.2f", code.Value),
			code.Status,
			usedBy,
			usedByEmail,
			usedAt,
			expiresAt,
			code.CreatedAt.Format("2006-01-02 15:04:05"),
		}); err != nil {
			response.InternalError(c, "Failed to export redeem codes: "+err.Error())
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		response.InternalError(c, "Failed to export redeem codes: "+err.Error())
		return
	}

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=redeem_codes.csv")
	c.Data(200, "text/csv", buf.Bytes())
}
