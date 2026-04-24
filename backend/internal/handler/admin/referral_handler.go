package admin

import (
	"errors"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ReferralHandler struct {
	adminService      *service.ReferralAdminService
	withdrawalService *service.ReferralWithdrawalService
}

func NewReferralHandler(adminService *service.ReferralAdminService, withdrawalService *service.ReferralWithdrawalService) *ReferralHandler {
	return &ReferralHandler{
		adminService:      adminService,
		withdrawalService: withdrawalService,
	}
}

type UpdateReferralRelationRequest struct {
	Code           string `json:"code"`
	ReferrerUserID int64  `json:"referrer_user_id"`
	Reason         string `json:"reason"`
	Notes          string `json:"notes"`
}

type CommissionAdjustmentRequest struct {
	RewardID int64   `json:"reward_id" binding:"required,gt=0"`
	Amount   float64 `json:"amount" binding:"required,ne=0"`
	Remark   string  `json:"remark"`
}

type ReviewWithdrawalRequest struct {
	Reason string `json:"reason"`
	Remark string `json:"remark"`
}

func (h *ReferralHandler) SearchAccounts(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	limit, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("limit", "10")))
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	items, err := h.adminService.SearchAccounts(c.Request.Context(), query, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *ReferralHandler) GetOverview(c *gin.Context) {
	overview, err := h.adminService.GetOverview(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, overview)
}

func (h *ReferralHandler) GetTree(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}
	tree, err := h.adminService.GetRelationTree(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tree)
}

func (h *ReferralHandler) ListRelations(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	items, paginationResult, err := h.adminService.ListRelations(c.Request.Context(), params, strings.TrimSpace(c.Query("search")))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, paginationResult.Total, page, pageSize)
}

func (h *ReferralHandler) GetRelation(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}
	relation, err := h.adminService.GetRelation(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, relation)
}

func (h *ReferralHandler) ListRelationHistories(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	userID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("user_id")), 10, 64)
	items, paginationResult, err := h.adminService.ListRelationHistories(c.Request.Context(), params, userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, paginationResult.Total, page, pageSize)
}

func (h *ReferralHandler) UpdateRelation(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "Admin not authenticated")
		return
	}
	var req UpdateReferralRelationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	relation, err := h.adminService.UpdateRelation(c.Request.Context(), &service.AdminUpdateReferralRelationInput{
		UserID:         userID,
		Code:           strings.TrimSpace(req.Code),
		ReferrerUserID: req.ReferrerUserID,
		ChangedBy:      subject.UserID,
		Reason:         strings.TrimSpace(req.Reason),
		Notes:          strings.TrimSpace(req.Notes),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, relation)
}

func (h *ReferralHandler) ListCommissionRewards(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	userID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("user_id")), 10, 64)
	sourceUserID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("source_user_id")), 10, 64)
	items, paginationResult, err := h.adminService.ListCommissionRewards(c.Request.Context(), params, service.AdminCommissionRewardFilter{
		UserID:       userID,
		SourceUserID: sourceUserID,
		Status:       strings.TrimSpace(c.Query("status")),
		Search:       strings.TrimSpace(c.Query("search")),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, paginationResult.Total, page, pageSize)
}

func (h *ReferralHandler) ListCommissionLedgers(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	userID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("user_id")), 10, 64)
	items, paginationResult, err := h.adminService.ListCommissionLedgers(c.Request.Context(), params, service.AdminCommissionLedgerFilter{
		UserID:    userID,
		EntryType: strings.TrimSpace(c.Query("entry_type")),
		Bucket:    strings.TrimSpace(c.Query("bucket")),
		Search:    strings.TrimSpace(c.Query("search")),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, paginationResult.Total, page, pageSize)
}

func (h *ReferralHandler) CreateCommissionAdjustment(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "Admin not authenticated")
		return
	}
	var req CommissionAdjustmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if math.Abs(req.Amount) > 1000000 {
		response.BadRequest(c, "Adjustment amount exceeds maximum allowed value")
		return
	}
	ledger, err := h.adminService.CreateCommissionAdjustment(c.Request.Context(), &service.AdminCommissionAdjustmentInput{
		RewardID:       req.RewardID,
		OperatorUserID: subject.UserID,
		Amount:         req.Amount,
		Remark:         strings.TrimSpace(req.Remark),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, ledger)
}

func (h *ReferralHandler) ListWithdrawals(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	userID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("user_id")), 10, 64)
	items, paginationResult, err := h.adminService.ListWithdrawals(c.Request.Context(), params, service.AdminCommissionWithdrawalFilter{
		UserID: userID,
		Status: strings.TrimSpace(c.Query("status")),
		Search: strings.TrimSpace(c.Query("search")),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, paginationResult.Total, page, pageSize)
}

func (h *ReferralHandler) GetWithdrawalItems(c *gin.Context) {
	withdrawalID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid withdrawal ID")
		return
	}
	items, err := h.adminService.ListWithdrawalItems(c.Request.Context(), withdrawalID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *ReferralHandler) ApproveWithdrawal(c *gin.Context) {
	withdrawalID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid withdrawal ID")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "Admin not authenticated")
		return
	}
	var req ReviewWithdrawalRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	withdrawal, err := h.withdrawalService.ApproveWithdrawal(c.Request.Context(), withdrawalID, subject.UserID, strings.TrimSpace(req.Remark))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, withdrawal)
}

func (h *ReferralHandler) RejectWithdrawal(c *gin.Context) {
	withdrawalID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid withdrawal ID")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "Admin not authenticated")
		return
	}
	var req ReviewWithdrawalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Reason) == "" && strings.TrimSpace(req.Remark) == "" {
		response.BadRequest(c, "Rejection reason is required")
		return
	}
	result, err := h.withdrawalService.RejectWithdrawal(c.Request.Context(), &service.ReviewReferralWithdrawalInput{
		WithdrawalID: withdrawalID,
		ReviewerID:   subject.UserID,
		Reason:       pickFirst(strings.TrimSpace(req.Reason), strings.TrimSpace(req.Remark)),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ReferralHandler) MarkWithdrawalPaid(c *gin.Context) {
	withdrawalID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid withdrawal ID")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "Admin not authenticated")
		return
	}
	var req ReviewWithdrawalRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.withdrawalService.MarkWithdrawalPaid(c.Request.Context(), &service.MarkReferralWithdrawalPaidInput{
		WithdrawalID: withdrawalID,
		PaidBy:       subject.UserID,
		Remark:       pickFirst(strings.TrimSpace(req.Remark), strings.TrimSpace(req.Reason)),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func pickFirst(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
