package handler

import (
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type ReferralHandler struct {
	referralService   *service.ReferralService
	centerService     *service.ReferralCenterService
	withdrawalService *service.ReferralWithdrawalService
}

func NewReferralHandler(
	referralService *service.ReferralService,
	centerService *service.ReferralCenterService,
	withdrawalService *service.ReferralWithdrawalService,
) *ReferralHandler {
	return &ReferralHandler{
		referralService:   referralService,
		centerService:     centerService,
		withdrawalService: withdrawalService,
	}
}

type ConvertCommissionToCreditRequest struct {
	Amount float64 `json:"amount" binding:"required,gt=0"`
}

type ValidateReferralCodeRequest struct {
	Code string `json:"code" binding:"required"`
}

type ValidateReferralCodeResponse struct {
	Valid               bool   `json:"valid"`
	ErrorCode           string `json:"error_code,omitempty"`
	ReferrerUsername     string `json:"referrer_username,omitempty"`
	ReferrerEmailMasked string `json:"referrer_email_masked,omitempty"`
}

type CreateWithdrawalRequest struct {
	Amount          float64 `json:"amount" binding:"required,gt=0"`
	PayoutMethod    string  `json:"payout_method" binding:"required"`
	PayoutAccountID int64   `json:"payout_account_id"`
	Remark          string  `json:"remark"`
}

type UpsertPayoutAccountRequest struct {
	Method      string `json:"method" binding:"required"`
	AccountName string `json:"account_name" binding:"required"`
	AccountNo   string `json:"account_no"`
	BankName    string `json:"bank_name"`
	QRImageURL  string `json:"qr_image_url"`
	IsDefault   bool   `json:"is_default"`
}

func (h *ReferralHandler) GetOverview(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	overview, err := h.centerService.GetOverview(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, overview)
}

func (h *ReferralHandler) GetLedger(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	ledgers, paginationResult, err := h.centerService.ListLedger(c.Request.Context(), subject.UserID, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, ledgers, paginationResult.Total, page, pageSize)
}

func (h *ReferralHandler) GetInvitees(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	invitees, paginationResult, err := h.centerService.ListInvitees(c.Request.Context(), subject.UserID, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, invitees, paginationResult.Total, page, pageSize)
}

func (h *ReferralHandler) CreateWithdrawal(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req CreateWithdrawalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.withdrawalService.CreateWithdrawal(c.Request.Context(), &service.CreateReferralWithdrawalInput{
		UserID:          subject.UserID,
		Amount:          req.Amount,
		PayoutMethod:    strings.TrimSpace(req.PayoutMethod),
		PayoutAccountID: req.PayoutAccountID,
		Remark:          req.Remark,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ReferralHandler) GetWithdrawals(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	withdrawals, paginationResult, err := h.centerService.ListWithdrawals(c.Request.Context(), subject.UserID, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, withdrawals, paginationResult.Total, page, pageSize)
}

func (h *ReferralHandler) GetPayoutAccounts(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	accounts, err := h.centerService.ListPayoutAccounts(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, accounts)
}

func (h *ReferralHandler) CreatePayoutAccount(c *gin.Context) {
	h.upsertPayoutAccount(c, 0)
}

func (h *ReferralHandler) UpdatePayoutAccount(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid payout account ID")
		return
	}
	h.upsertPayoutAccount(c, accountID)
}

func (h *ReferralHandler) ConvertToCredit(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req ConvertCommissionToCreditRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	err := h.withdrawalService.ConvertCommissionToCredit(c.Request.Context(), subject.UserID, req.Amount)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Commission converted to credit successfully"})
}

func (h *ReferralHandler) GetInviteeRewards(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	sourceUserID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}
	rewards, err := h.centerService.ListInviteeRewards(c.Request.Context(), subject.UserID, sourceUserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rewards)
}

func (h *ReferralHandler) ValidateCode(c *gin.Context) {
	var req ValidateReferralCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	preview, err := h.referralService.PreviewReferralCode(c.Request.Context(), strings.TrimSpace(req.Code))
	if err == nil {
		resp := ValidateReferralCodeResponse{Valid: true}
		if preview != nil {
			resp.ReferrerUsername = preview.ReferrerUsername
			resp.ReferrerEmailMasked = preview.ReferrerEmailMasked
		}
		response.Success(c, resp)
		return
	}

	appErr := responseErrorCode(err)
	response.Success(c, ValidateReferralCodeResponse{
		Valid:     false,
		ErrorCode: appErr,
	})
}

func responseErrorCode(err error) string {
	if err == nil {
		return ""
	}
	return infraerrors.Reason(err)
}

func (h *ReferralHandler) upsertPayoutAccount(c *gin.Context, accountID int64) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req UpsertPayoutAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	account, err := h.withdrawalService.UpsertPayoutAccount(c.Request.Context(), subject.UserID, accountID, &service.UpsertReferralPayoutAccountInput{
		Method:      strings.TrimSpace(req.Method),
		AccountName: strings.TrimSpace(req.AccountName),
		AccountNo:   strings.TrimSpace(req.AccountNo),
		BankName:    strings.TrimSpace(req.BankName),
		QRImageURL:  strings.TrimSpace(req.QRImageURL),
		IsDefault:   req.IsDefault,
		Status:      "active",
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, account)
}
