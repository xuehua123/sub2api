package admin

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type accountHealthProbeRequest struct {
	ModelID string `json:"model_id"`
}

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

// RunAccountHealthProbe runs an immediate active probe for an account and persists
// the probe result into the account's observational Extra fields.
// POST /api/v1/admin/ops/account-health/:id/probe
func (h *OpsHandler) RunAccountHealthProbe(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if h.accountTestService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Account health probe service not available")
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

	var req accountHealthProbeRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "Invalid request body")
			return
		}
	}
	modelID := strings.TrimSpace(req.ModelID)

	timeout := 20 * time.Second
	if cfg, err := h.opsService.GetOpsAlertRuntimeSettings(c.Request.Context()); err == nil && cfg != nil {
		if modelID == "" {
			modelID = strings.TrimSpace(cfg.AccountHealth.Probe.ModelID)
		}
		if cfg.AccountHealth.Probe.TimeoutSeconds > 0 {
			timeout = time.Duration(cfg.AccountHealth.Probe.TimeoutSeconds) * time.Second
		}
	}
	if timeout < 5*time.Second {
		timeout = 5 * time.Second
	}
	if timeout > 120*time.Second {
		timeout = 120 * time.Second
	}

	probeCtx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	probe, err := h.accountTestService.RunAccountHealthProbe(probeCtx, accountID, modelID)
	if err != nil {
		response.Error(c, http.StatusBadGateway, err.Error())
		return
	}
	response.Success(c, probe)
}
