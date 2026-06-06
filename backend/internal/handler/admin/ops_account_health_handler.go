package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GetAccountHealth returns account-level health windows and smart action hints.
// GET /api/v1/admin/ops/account-health
func (h *OpsHandler) GetAccountHealth(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	filter := &service.OpsAccountHealthFilter{
		Platform: strings.TrimSpace(c.Query("platform")),
	}

	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}

	if v := strings.TrimSpace(c.Query("recent_limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			response.BadRequest(c, "Invalid recent_limit")
			return
		}
		filter.RecentLimit = n
	}

	health, err := h.opsService.GetAccountHealth(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, health)
}

// UpdateAccountHealthSettings updates account health notification/probe settings only.
// PATCH /api/v1/admin/ops/account-health/settings
func (h *OpsHandler) UpdateAccountHealthSettings(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	var settings service.OpsAccountHealthSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		response.BadRequest(c, "Invalid account health settings")
		return
	}

	updated, err := h.opsService.UpdateOpsAccountHealthSettings(c.Request.Context(), settings)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(c, service.MaskOpsAccountHealthSettingsForResponse(updated))
}
