package handler

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type lobeHubLaunchService interface {
	CreateLaunchTicket(ctx context.Context, userID, apiKeyID int64) (*service.LobeHubLaunchTicketResult, error)
	BuildBridgePayload(ctx context.Context, ticketID string) (*service.LobeHubBridgePayload, error)
}

type lobeHubOIDCService interface {
	GetDiscoveryDocument(ctx context.Context) (*service.LobeHubOIDCDiscoveryDocument, error)
	GetJWKS(ctx context.Context) (*service.LobeHubJWKSet, error)
	Authorize(ctx context.Context, webSessionID string, req *service.LobeHubOIDCAuthorizeRequest) (*service.LobeHubOIDCAuthorizeResult, error)
	ExchangeAuthorizationCode(ctx context.Context, req *service.LobeHubOIDCTokenRequest) (*service.LobeHubOIDCTokenResponse, error)
	GetUserInfo(ctx context.Context, accessToken string) (*service.LobeHubOIDCUserInfo, error)
}

type lobeHubSSOService interface {
	PrepareOIDCContinuation(ctx context.Context, userID int64, req *service.LobeHubSSOContinuationRequest) (*service.LobeHubSSOContinuationResult, error)
	PrepareTargetRefresh(ctx context.Context, userID int64, req *service.LobeHubSSORefreshRequest) (*service.LobeHubSSOContinuationResult, error)
	ExchangeBootstrap(ctx context.Context, targetToken string, returnURL string) (*service.LobeHubBootstrapExchangeResult, error)
	ConsumeBootstrap(ctx context.Context, ticketID string) (*service.LobeHubBootstrapConsumeResult, error)
	CompareCurrentConfig(ctx context.Context, targetToken string, observed *service.LobeHubObservedSettings) (*service.LobeHubConfigProbeResult, error)
}

type LobeHubHandler struct {
	launchService lobeHubLaunchService
	oidcService   lobeHubOIDCService
	ssoService    lobeHubSSOService
}

type createLobeHubLaunchTicketRequest struct {
	APIKeyID int64 `json:"api_key_id" binding:"required"`
}

type createLobeHubOIDCWebSessionRequest struct {
	ResumeToken string `json:"resume_token"`
	ReturnURL   string `json:"return_url"`
	APIKeyID    *int64 `json:"api_key_id,omitempty"`
	Mode        string `json:"mode,omitempty"`
}

type lobeHubBootstrapExchangeRequest struct {
	ReturnURL string `json:"return_url" binding:"required"`
}

const (
	lobeHubOIDCSessionCookieName      = "lobehub_oidc_session"
	lobeHubOIDCSessionCookiePath      = "/api/v1/lobehub/oidc"
	lobeHubOIDCSessionCookieMaxAgeSec = 5 * 60
	lobeHubTargetCookieName           = "sub2api_lobehub_target"
	lobeHubTargetCookieMaxAgeSec      = int(service.LobeHubTargetTokenTTL / time.Second)
	lobeHubBootstrapCookieName        = "sub2api_lobehub_bootstrap"
	lobeHubBootstrapCookieMaxAgeSec   = int(service.LobeHubBootstrapTicketTTL / time.Second)
)

func NewLobeHubHandler(launchService lobeHubLaunchService, oidcService lobeHubOIDCService, ssoService lobeHubSSOService) *LobeHubHandler {
	return &LobeHubHandler{launchService: launchService, oidcService: oidcService, ssoService: ssoService}
}

func ProvideLobeHubHandler(launchService *service.LobeHubLaunchService, oidcService *service.LobeHubOIDCService, ssoService *service.LobeHubSSOService) *LobeHubHandler {
	return NewLobeHubHandler(launchService, oidcService, ssoService)
}

func (h *LobeHubHandler) CreateLaunchTicket(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req createLobeHubLaunchTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	result, err := h.launchService.CreateLaunchTicket(c.Request.Context(), subject.UserID, req.APIKeyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, result)
}

func (h *LobeHubHandler) Bridge(c *gin.Context) {
	ticketID := strings.TrimSpace(c.Query("ticket"))
	if ticketID == "" {
		response.BadRequest(c, "Missing ticket")
		return
	}

	payload, err := h.launchService.BuildBridgePayload(c.Request.Context(), ticketID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
	setLobeHubSharedCookie(c, lobeHubTargetCookieName, payload.TargetToken, payload.CookieDomain, lobeHubTargetCookieMaxAgeSec, true)
	setLobeHubSharedCookie(c, lobeHubBootstrapCookieName, payload.BootstrapTicketID, payload.CookieDomain, lobeHubBootstrapCookieMaxAgeSec, true)
	c.Redirect(http.StatusFound, payload.ContinueURL)
}

func (h *LobeHubHandler) Discovery(c *gin.Context) {
	document, err := h.oidcService.GetDiscoveryDocument(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, document)
}

func (h *LobeHubHandler) JWKS(c *gin.Context) {
	jwks, err := h.oidcService.GetJWKS(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, jwks)
}

func (h *LobeHubHandler) Authorize(c *gin.Context) {
	webSessionID := ""
	if cookie, err := c.Request.Cookie(lobeHubOIDCSessionCookieName); err == nil {
		webSessionID = strings.TrimSpace(cookie.Value)
	}

	req := &service.LobeHubOIDCAuthorizeRequest{
		ClientID:            strings.TrimSpace(c.Query("client_id")),
		RedirectURI:         strings.TrimSpace(c.Query("redirect_uri")),
		ResponseType:        strings.TrimSpace(c.Query("response_type")),
		Scope:               strings.TrimSpace(c.Query("scope")),
		State:               strings.TrimSpace(c.Query("state")),
		Nonce:               strings.TrimSpace(c.Query("nonce")),
		ReturnURL:           strings.TrimSpace(c.Query("return_url")),
		CodeChallenge:       strings.TrimSpace(c.Query("code_challenge")),
		CodeChallengeMethod: strings.TrimSpace(c.Query("code_challenge_method")),
	}

	result, err := h.oidcService.Authorize(c.Request.Context(), webSessionID, req)
	if err != nil {
		if protocolErr := oidcProtocolError(err); protocolErr != nil {
			redirectOIDCError(c, req.RedirectURI, req.State, protocolErr)
			return
		}
		response.ErrorFrom(c, err)
		return
	}

	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Redirect(http.StatusFound, result.RedirectURL)
}

func (h *LobeHubHandler) Token(c *gin.Context) {
	if err := c.Request.ParseForm(); err != nil {
		writeOIDCErrorJSON(c, &service.LobeHubOIDCProtocolError{
			Code:        service.ErrLobeHubOIDCInvalidRequest.Code,
			Description: "invalid form body",
			StatusCode:  service.ErrLobeHubOIDCInvalidRequest.StatusCode,
		})
		return
	}

	clientID, clientSecret, err := parseOIDCClientCredentials(c)
	if err != nil {
		writeOIDCErrorJSON(c, &service.LobeHubOIDCProtocolError{
			Code:        service.ErrLobeHubOIDCInvalidClient.Code,
			Description: err.Error(),
			StatusCode:  service.ErrLobeHubOIDCInvalidClient.StatusCode,
		})
		return
	}

	result, err := h.oidcService.ExchangeAuthorizationCode(c.Request.Context(), &service.LobeHubOIDCTokenRequest{
		GrantType:    strings.TrimSpace(c.PostForm("grant_type")),
		Code:         strings.TrimSpace(c.PostForm("code")),
		RedirectURI:  strings.TrimSpace(c.PostForm("redirect_uri")),
		ClientID:     clientID,
		ClientSecret: clientSecret,
		CodeVerifier: strings.TrimSpace(c.PostForm("code_verifier")),
	})
	if err != nil {
		if protocolErr := oidcProtocolError(err); protocolErr != nil {
			writeOIDCErrorJSON(c, protocolErr)
			return
		}
		response.ErrorFrom(c, err)
		return
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}

func (h *LobeHubHandler) UserInfo(c *gin.Context) {
	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	if !strings.HasPrefix(strings.ToLower(authorization), "bearer ") {
		writeOIDCErrorJSON(c, service.ErrLobeHubOIDCInvalidToken)
		return
	}
	accessToken := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer"))
	accessToken = strings.TrimSpace(strings.TrimPrefix(accessToken, "bearer"))
	if accessToken == "" {
		writeOIDCErrorJSON(c, service.ErrLobeHubOIDCInvalidToken)
		return
	}

	result, err := h.oidcService.GetUserInfo(c.Request.Context(), accessToken)
	if err != nil {
		if protocolErr := oidcProtocolError(err); protocolErr != nil {
			writeOIDCErrorJSON(c, protocolErr)
			return
		}
		response.ErrorFrom(c, err)
		return
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}

func (h *LobeHubHandler) CreateOIDCWebSession(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req createLobeHubOIDCWebSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	var (
		result *service.LobeHubSSOContinuationResult
		err    error
	)
	if strings.EqualFold(strings.TrimSpace(req.Mode), "refresh-target") {
		result, err = h.ssoService.PrepareTargetRefresh(c.Request.Context(), subject.UserID, &service.LobeHubSSORefreshRequest{
			ReturnURL: req.ReturnURL,
			APIKeyID:  req.APIKeyID,
		})
	} else {
		result, err = h.ssoService.PrepareOIDCContinuation(c.Request.Context(), subject.UserID, &service.LobeHubSSOContinuationRequest{
			ResumeToken: req.ResumeToken,
			ReturnURL:   req.ReturnURL,
			APIKeyID:    req.APIKeyID,
		})
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	if result.OIDCSessionID != "" {
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     lobeHubOIDCSessionCookieName,
			Value:    result.OIDCSessionID,
			Path:     lobeHubOIDCSessionCookiePath,
			MaxAge:   lobeHubOIDCSessionCookieMaxAgeSec,
			HttpOnly: true,
			Secure:   isRequestHTTPS(c),
			SameSite: http.SameSiteLaxMode,
		})
	}
	setLobeHubSharedCookie(c, lobeHubTargetCookieName, result.TargetToken, result.CookieDomain, lobeHubTargetCookieMaxAgeSec, true)
	setLobeHubSharedCookie(c, lobeHubBootstrapCookieName, result.BootstrapTicketID, result.CookieDomain, lobeHubBootstrapCookieMaxAgeSec, true)

	response.Success(c, gin.H{
		"continue_url": result.ContinueURL,
	})
}

func (h *LobeHubHandler) BootstrapExchange(c *gin.Context) {
	var req lobeHubBootstrapExchangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	targetCookie, err := c.Request.Cookie(lobeHubTargetCookieName)
	if err != nil || strings.TrimSpace(targetCookie.Value) == "" {
		response.ErrorFrom(c, service.ErrLobeHubInvalidTargetToken)
		return
	}

	result, err := h.ssoService.ExchangeBootstrap(c.Request.Context(), strings.TrimSpace(targetCookie.Value), req.ReturnURL)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *LobeHubHandler) ConsumeBootstrap(c *gin.Context) {
	ticketID := strings.TrimSpace(c.Query("ticket"))
	if ticketID == "" {
		response.BadRequest(c, "Missing ticket")
		return
	}

	result, err := h.ssoService.ConsumeBootstrap(c.Request.Context(), ticketID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *LobeHubHandler) CompareCurrentConfig(c *gin.Context) {
	var req service.LobeHubObservedSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	targetCookie, err := c.Request.Cookie(lobeHubTargetCookieName)
	if err != nil || strings.TrimSpace(targetCookie.Value) == "" {
		response.ErrorFrom(c, service.ErrLobeHubInvalidTargetToken)
		return
	}

	result, err := h.ssoService.CompareCurrentConfig(c.Request.Context(), strings.TrimSpace(targetCookie.Value), &req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func parseOIDCClientCredentials(c *gin.Context) (clientID string, clientSecret string, err error) {
	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	if strings.HasPrefix(strings.ToLower(authorization), "basic ") {
		payload := strings.TrimSpace(authorization[len("Basic "):])
		decoded, decodeErr := base64.StdEncoding.DecodeString(payload)
		if decodeErr != nil {
			return "", "", fmt.Errorf("invalid basic authorization header")
		}
		username, password, ok := strings.Cut(string(decoded), ":")
		if !ok {
			return "", "", fmt.Errorf("invalid basic authorization header")
		}
		return strings.TrimSpace(username), password, nil
	}

	clientID = strings.TrimSpace(c.PostForm("client_id"))
	clientSecret = c.PostForm("client_secret")
	if clientID == "" || clientSecret == "" {
		return "", "", fmt.Errorf("client credentials are required")
	}
	return clientID, clientSecret, nil
}

func oidcProtocolError(err error) *service.LobeHubOIDCProtocolError {
	var protocolErr *service.LobeHubOIDCProtocolError
	if errors.As(err, &protocolErr) {
		return protocolErr
	}
	return nil
}

func writeOIDCErrorJSON(c *gin.Context, err *service.LobeHubOIDCProtocolError) {
	if err == nil {
		err = service.ErrLobeHubOIDCInvalidRequest
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(err.StatusCode, gin.H{
		"error":             err.Code,
		"error_description": err.Description,
	})
}

func redirectOIDCError(c *gin.Context, redirectURI string, state string, err *service.LobeHubOIDCProtocolError) {
	if err == nil || strings.TrimSpace(redirectURI) == "" {
		writeOIDCErrorJSON(c, err)
		return
	}

	target, parseErr := url.Parse(strings.TrimSpace(redirectURI))
	if parseErr != nil {
		writeOIDCErrorJSON(c, err)
		return
	}
	query := target.Query()
	query.Set("error", err.Code)
	if err.Description != "" {
		query.Set("error_description", err.Description)
	}
	if strings.TrimSpace(state) != "" {
		query.Set("state", strings.TrimSpace(state))
	}
	target.RawQuery = query.Encode()
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Redirect(http.StatusFound, target.String())
}

func setLobeHubSharedCookie(c *gin.Context, name string, value string, domain string, maxAge int, httpOnly bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Domain:   strings.TrimSpace(domain),
		MaxAge:   maxAge,
		HttpOnly: httpOnly,
		Secure:   isRequestHTTPS(c),
		SameSite: http.SameSiteLaxMode,
	})
}
