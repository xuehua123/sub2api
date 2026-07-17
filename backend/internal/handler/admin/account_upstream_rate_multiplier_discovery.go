package admin

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type upstreamRateMultiplierDiscoveryRequest struct {
	AccountID              *int64                               `json:"account_id,omitempty"`
	BaseURL                string                               `json:"base_url,omitempty"`
	UpstreamAPIKey         string                               `json:"upstream_api_key,omitempty"`
	ProxyID                *int64                               `json:"proxy_id,omitempty"`
	AuthMode               service.UpstreamManagementAuthMode   `json:"auth_mode"`
	RemoteUserID           *int64                               `json:"remote_user_id,omitempty"`
	UpstreamManagementAuth *service.UpstreamManagementAuthInput `json:"upstream_management_auth,omitempty"`
}

// DiscoverUpstreamRateMultiplierGroups detects the upstream management API and
// lists its visible groups. Plaintext credentials are only used for this request.
func (h *AccountHandler) DiscoverUpstreamRateMultiplierGroups(c *gin.Context) {
	if h == nil || h.upstreamRateSync == nil {
		response.InternalError(c, "upstream rate multiplier discovery is unavailable")
		return
	}

	var req upstreamRateMultiplierDiscoveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid upstream rate multiplier discovery request")
		return
	}

	account := &service.Account{Credentials: make(map[string]any)}
	if req.AccountID != nil {
		if *req.AccountID <= 0 {
			response.BadRequest(c, "invalid account id")
			return
		}
		existing, err := h.adminService.GetAccount(c.Request.Context(), *req.AccountID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		copied := *existing
		account = &copied
	}

	if baseURL := strings.TrimSpace(req.BaseURL); baseURL != "" {
		credentials := make(map[string]any, len(account.Credentials)+1)
		for key, value := range account.Credentials {
			credentials[key] = value
		}
		credentials["base_url"] = baseURL
		account.Credentials = credentials
	}
	if req.AccountID == nil && strings.TrimSpace(req.BaseURL) == "" {
		response.BadRequest(c, "base_url is required for a new account")
		return
	}

	if req.ProxyID != nil {
		if *req.ProxyID <= 0 {
			response.BadRequest(c, "invalid proxy id")
			return
		}
		proxy, err := h.adminService.GetProxy(c.Request.Context(), *req.ProxyID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		if proxy == nil || !proxy.IsActive() {
			response.BadRequest(c, "selected proxy is not active")
			return
		}
		account.ProxyID = req.ProxyID
		account.Proxy = proxy
	}

	remoteUserID := int64(0)
	if req.RemoteUserID != nil {
		remoteUserID = *req.RemoteUserID
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 24*time.Second)
	defer cancel()
	discovery, err := h.upstreamRateSync.DiscoverGroups(ctx, account, req.AuthMode, remoteUserID, req.UpstreamManagementAuth, req.UpstreamAPIKey)
	if err != nil {
		if errors.Is(err, service.ErrUpstreamAPIKeyGroupUnmapped) {
			response.BadRequest(c, "unable to map this account API key to an upstream group; verify the API key and management user")
			return
		}
		response.BadRequest(c, "unable to detect the upstream management API or load its groups; verify the address and management credentials")
		return
	}
	response.Success(c, discovery)
}
