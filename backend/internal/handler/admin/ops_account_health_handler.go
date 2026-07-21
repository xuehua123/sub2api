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
	Mode    string `json:"mode"`
	Prompt  string `json:"prompt"`
}

type accountHealthProbeAutoRequest struct {
	Enabled bool `json:"enabled"`
}

type accountHealthProbeModelRequest struct {
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

	if v := strings.TrimSpace(c.Query("settings_only")); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes":
			filter.SettingsOnly = true
		case "0", "false", "no":
			filter.SettingsOnly = false
		default:
			response.BadRequest(c, "Invalid settings_only")
			return
		}
	}

	// Presence of account_ids (even empty) means the client wants a scoped response.
	if raw, ok := c.GetQuery("account_ids"); ok {
		filter.AccountIDsScoped = true
		ids := make([]int64, 0)
		seen := make(map[int64]struct{})
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			id, err := strconv.ParseInt(part, 10, 64)
			if err != nil || id <= 0 {
				response.BadRequest(c, "Invalid account_ids")
				return
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
			if len(ids) > 200 {
				response.BadRequest(c, "account_ids exceeds limit of 200")
				return
			}
		}
		filter.AccountIDs = ids
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

// TestAccountHealthEnterpriseWeChat sends a sample formatted account-health
// notification through the configured enterprise WeChat webhook.
// POST /api/v1/admin/ops/account-health/test-enterprise-wechat
func (h *OpsHandler) TestAccountHealthEnterpriseWeChat(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	var settings *service.OpsAccountHealthSettings
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		var req service.OpsAccountHealthSettings
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "Invalid account health settings")
			return
		}
		settings = &req
	}

	if err := h.opsService.TestOpsAccountHealthEnterpriseWeChat(c.Request.Context(), settings); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, gin.H{"ok": true})
}

// UpdateAccountHealthProbeAuto updates whether a single closed account may be
// picked by the background active-probe scheduler.
// PATCH /api/v1/admin/ops/account-health/:id/probe-auto
func (h *OpsHandler) UpdateAccountHealthProbeAuto(c *gin.Context) {
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

	var req accountHealthProbeAutoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	state, err := h.opsService.UpdateAccountHealthProbeAuto(c.Request.Context(), accountID, req.Enabled)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, state)
}

// UpdateAccountHealthProbeModel updates the per-account active probe model.
// Empty model_id means inheriting the global account-health probe model.
// PATCH /api/v1/admin/ops/account-health/:id/probe-model
func (h *OpsHandler) UpdateAccountHealthProbeModel(c *gin.Context) {
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

	var req accountHealthProbeModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	state, err := h.opsService.UpdateAccountHealthProbeModel(c.Request.Context(), accountID, req.ModelID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, state)
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
	modelID := h.opsService.ResolveAccountHealthProbeModelID(c.Request.Context(), accountID, req.ModelID)
	mode := strings.TrimSpace(req.Mode)
	prompt := strings.TrimSpace(req.Prompt)

	timeout := 20 * time.Second
	if cfg, err := h.opsService.GetOpsAlertRuntimeSettings(c.Request.Context()); err == nil && cfg != nil {
		if mode == "" {
			mode = strings.TrimSpace(cfg.AccountHealth.Probe.Mode)
		}
		if prompt == "" {
			prompt = strings.TrimSpace(cfg.AccountHealth.Probe.Prompt)
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

	probe, err := h.accountTestService.RunAccountHealthProbeWithOptions(probeCtx, accountID, modelID, prompt, mode)
	if err != nil {
		response.Error(c, http.StatusBadGateway, err.Error())
		return
	}
	response.Success(c, probe)
}
