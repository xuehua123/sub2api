package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GetUpstreamConnectionBalanceMonitor returns shared connection wallet snapshots.
func (h *OpsHandler) GetUpstreamConnectionBalanceMonitor(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	page, pageSize := response.ParsePagination(c)
	result, err := h.opsService.ListUpstreamConnectionBalanceMonitor(c.Request.Context(), service.OpsUpstreamConnectionBalanceMonitorFilter{
		Page: page, PageSize: pageSize, Search: strings.TrimSpace(c.Query("q")), Status: strings.TrimSpace(c.Query("status")),
		OnlyLow: parseOpsBoolQuery(c.Query("only_low")), OnlyFailed: parseOpsBoolQuery(c.Query("only_failed")),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// UpdateUpstreamConnectionBalanceSettings updates global connection alert settings.
func (h *OpsHandler) UpdateUpstreamConnectionBalanceSettings(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var settings service.OpsUpstreamConnectionBalanceSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		response.BadRequest(c, "Invalid upstream connection balance settings")
		return
	}
	updated, err := h.opsService.UpdateOpsUpstreamConnectionBalanceSettings(c.Request.Context(), settings)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, service.MaskOpsUpstreamConnectionBalanceSettingsForResponse(updated))
}

// TestUpstreamConnectionBalanceEnterpriseWeChat sends a sample notification.
func (h *OpsHandler) TestUpstreamConnectionBalanceEnterpriseWeChat(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var settings *service.OpsUpstreamConnectionBalanceSettings
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		var request service.OpsUpstreamConnectionBalanceSettings
		if err := c.ShouldBindJSON(&request); err != nil {
			response.BadRequest(c, "Invalid upstream connection balance settings")
			return
		}
		settings = &request
	}
	if err := h.opsService.TestOpsUpstreamConnectionBalanceEnterpriseWeChat(c.Request.Context(), settings); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, gin.H{"ok": true})
}

// UpdateUpstreamConnectionBalanceAlert updates one connection's alert override.
func (h *OpsHandler) UpdateUpstreamConnectionBalanceAlert(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	connectionID, ok := parseOpsUpstreamConnectionID(c)
	if !ok {
		return
	}
	var request service.OpsUpstreamConnectionBalanceAlertUpdate
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	state, err := h.opsService.UpdateUpstreamConnectionBalanceAlert(c.Request.Context(), connectionID, request)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, state)
}

// ProbeUpstreamConnectionBalance refreshes the shared connection and returns its wallet state.
func (h *OpsHandler) ProbeUpstreamConnectionBalance(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	connectionID, ok := parseOpsUpstreamConnectionID(c)
	if !ok {
		return
	}
	item, err := h.opsService.ProbeUpstreamConnectionBalance(c.Request.Context(), connectionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func parseOpsUpstreamConnectionID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid upstream connection id")
		return 0, false
	}
	return id, true
}

func parseOpsBoolQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
