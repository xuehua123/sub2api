package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type accountBalanceProbeRequest struct {
	Method string `json:"method"`
	Force  *bool  `json:"force"`
}

// GetAccountBalanceMonitor returns upstream account balance snapshots and settings.
// GET /api/v1/admin/ops/account-balance
func (h *OpsHandler) GetAccountBalanceMonitor(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	page, pageSize := response.ParsePagination(c)
	filter := service.OpsAccountBalanceMonitorFilter{
		Page:            page,
		PageSize:        pageSize,
		Platform:        strings.TrimSpace(c.Query("platform")),
		Status:          strings.TrimSpace(c.Query("status")),
		ProbeStatus:     strings.TrimSpace(c.Query("probe_status")),
		Search:          strings.TrimSpace(c.Query("q")),
		Method:          strings.TrimSpace(c.Query("method")),
		OnlyDue:         parseOpsBoolQuery(c.Query("only_due")),
		OnlyLow:         parseOpsBoolQuery(c.Query("only_low")),
		OnlyFailed:      parseOpsBoolQuery(c.Query("only_failed")),
		OnlySchedulable: parseOpsBoolQuery(c.Query("only_schedulable")),
		SortBy:          strings.TrimSpace(c.Query("sort_by")),
		SortOrder:       strings.TrimSpace(c.Query("sort_order")),
	}
	result, err := h.opsService.ListAccountBalanceMonitor(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// UpdateAccountBalanceSettings updates upstream balance monitor settings only.
// PATCH /api/v1/admin/ops/account-balance/settings
func (h *OpsHandler) UpdateAccountBalanceSettings(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var settings service.OpsAccountBalanceSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		response.BadRequest(c, "Invalid account balance settings")
		return
	}
	updated, err := h.opsService.UpdateOpsAccountBalanceSettings(c.Request.Context(), settings)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, service.MaskOpsAccountBalanceSettingsForResponse(updated))
}

// TestAccountBalanceEnterpriseWeChat sends a sample balance notification.
// POST /api/v1/admin/ops/account-balance/test-enterprise-wechat
func (h *OpsHandler) TestAccountBalanceEnterpriseWeChat(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var settings *service.OpsAccountBalanceSettings
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		var req service.OpsAccountBalanceSettings
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "Invalid account balance settings")
			return
		}
		settings = &req
	}
	if err := h.opsService.TestOpsAccountBalanceEnterpriseWeChat(c.Request.Context(), settings); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, gin.H{"ok": true})
}

// UpdateAccountBalanceProbeConfig updates one account's balance probing method.
// PATCH /api/v1/admin/ops/account-balance/:id
func (h *OpsHandler) UpdateAccountBalanceProbeConfig(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	accountID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "Invalid account id")
		return
	}
	var req service.OpsAccountBalanceProbeConfigUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	state, err := h.opsService.UpdateAccountBalanceProbeConfig(c.Request.Context(), accountID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, state)
}

// RunAccountBalanceProbe runs an immediate upstream balance probe for one account.
// POST /api/v1/admin/ops/account-balance/:id/probe
func (h *OpsHandler) RunAccountBalanceProbe(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	accountID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "Invalid account id")
		return
	}
	var req accountBalanceProbeRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "Invalid request body")
			return
		}
	}
	force := true
	if req.Force != nil {
		force = *req.Force
	}
	result, err := h.opsService.ProbeAccountBalance(c.Request.Context(), accountID, force, req.Method)
	if err != nil {
		response.Error(c, http.StatusBadGateway, err.Error())
		return
	}
	response.Success(c, result)
}

func parseOpsBoolQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
