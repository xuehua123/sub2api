package admin

import (
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type UpstreamConnectionHandler struct {
	service *service.UpstreamConnectionService
}

type upstreamConnectionRuntimeOverviewRequest struct {
	AccountIDs []int64 `json:"account_ids" binding:"required"`
}

type upstreamConnectionDeleteRequest struct {
	UnbindAccounts          bool     `json:"unbind_accounts"`
	ExpectedBoundAccountIDs *[]int64 `json:"expected_bound_account_ids"`
}

func NewUpstreamConnectionHandler(connectionService *service.UpstreamConnectionService) *UpstreamConnectionHandler {
	return &UpstreamConnectionHandler{service: connectionService}
}

type upstreamConnectionCredentialRequest struct {
	Username     string `json:"username" binding:"omitempty,max=320"`
	Password     string `json:"password" binding:"omitempty,max=4096"`
	AccessToken  string `json:"access_token" binding:"omitempty,max=8192"`
	RefreshToken string `json:"refresh_token" binding:"omitempty,max=8192"`
	UserAgent    string `json:"user_agent" binding:"omitempty,max=512"`
}

type upstreamConnectionCreateRequest struct {
	Name                string                              `json:"name" binding:"required,max=100"`
	Provider            string                              `json:"provider" binding:"omitempty,max=32"`
	AuthMode            string                              `json:"auth_mode" binding:"required,oneof=password access_token"`
	ManagementBaseURL   string                              `json:"management_base_url" binding:"required,max=500"`
	ForwardingBaseURL   string                              `json:"forwarding_base_url" binding:"omitempty,max=500"`
	Credential          upstreamConnectionCredentialRequest `json:"credential" binding:"required"`
	NotInCNConfirmed    bool                                `json:"not_in_cn_confirmed"`
	RemoteUserID        string                              `json:"remote_user_id" binding:"omitempty,max=128"`
	ProxyID             *int64                              `json:"proxy_id"`
	SyncEnabled         *bool                               `json:"sync_enabled"`
	SyncIntervalSeconds int                                 `json:"sync_interval_seconds" binding:"omitempty,min=30,max=86400"`
}

type upstreamConnectionUpdateRequest struct {
	ExpectedVersion     int64                                `json:"expected_version" binding:"required,min=1"`
	Name                *string                              `json:"name" binding:"omitempty,max=100"`
	Provider            *string                              `json:"provider" binding:"omitempty,max=32"`
	AuthMode            *string                              `json:"auth_mode" binding:"omitempty,oneof=password access_token"`
	ManagementBaseURL   *string                              `json:"management_base_url" binding:"omitempty,max=500"`
	ForwardingBaseURL   *string                              `json:"forwarding_base_url" binding:"omitempty,max=500"`
	Credential          *upstreamConnectionCredentialRequest `json:"credential"`
	NotInCNConfirmed    *bool                                `json:"not_in_cn_confirmed"`
	RemoteUserID        *string                              `json:"remote_user_id" binding:"omitempty,max=128"`
	ProxyID             *int64                               `json:"proxy_id"`
	ClearProxy          bool                                 `json:"clear_proxy"`
	SyncEnabled         *bool                                `json:"sync_enabled"`
	SyncIntervalSeconds *int                                 `json:"sync_interval_seconds" binding:"omitempty,min=30,max=86400"`
}

type upstreamGroupResponse struct {
	ID             int64          `json:"id"`
	RemoteID       string         `json:"remote_id"`
	Name           string         `json:"name"`
	RateMultiplier *float64       `json:"rate_multiplier"`
	Source         string         `json:"source"`
	Confidence     string         `json:"confidence"`
	Metadata       map[string]any `json:"metadata"`
	ObservedAt     *string        `json:"observed_at"`
	FreshUntil     *string        `json:"fresh_until"`
}

type upstreamAccountBindingResponse struct {
	ID                 int64          `json:"id"`
	AccountID          int64          `json:"account_id"`
	ConnectionID       int64          `json:"connection_id"`
	RemoteTokenID      string         `json:"remote_token_id"`
	RemoteTokenName    string         `json:"remote_token_name"`
	ResolutionKind     string         `json:"resolution_kind"`
	RemoteGroupID      string         `json:"remote_group_id"`
	RemoteGroupName    string         `json:"remote_group_name"`
	FallbackGroups     []string       `json:"fallback_groups"`
	ObservedMultiplier *float64       `json:"observed_multiplier"`
	Confidence         string         `json:"confidence"`
	Source             string         `json:"source"`
	ApplyPolicy        string         `json:"apply_policy"`
	Status             string         `json:"status"`
	SyncFailures       int            `json:"sync_failures"`
	LastError          string         `json:"last_error"`
	ResolutionDetails  map[string]any `json:"resolution_details"`
	ObservedAt         *string        `json:"observed_at"`
	FreshUntil         *string        `json:"fresh_until"`
}

type upstreamConnectionResponse struct {
	ID                   int64                            `json:"id"`
	Name                 string                           `json:"name"`
	Provider             string                           `json:"provider"`
	AuthMode             string                           `json:"auth_mode"`
	ManagementBaseURL    string                           `json:"management_base_url"`
	ForwardingBaseURL    string                           `json:"forwarding_base_url"`
	CredentialConfigured bool                             `json:"credential_configured"`
	CredentialHint       string                           `json:"credential_hint"`
	NotInCNConfirmed     bool                             `json:"not_in_cn_confirmed"`
	RemoteUserID         string                           `json:"remote_user_id"`
	ProxyID              *int64                           `json:"proxy_id"`
	Capabilities         map[string]any                   `json:"capabilities"`
	Status               string                           `json:"status"`
	LastError            string                           `json:"last_error"`
	SyncEnabled          bool                             `json:"sync_enabled"`
	SyncIntervalSeconds  int                              `json:"sync_interval_seconds"`
	SyncFailures         int                              `json:"sync_failures"`
	Version              int64                            `json:"version"`
	WalletAmount         *float64                         `json:"wallet_amount"`
	WalletCurrency       string                           `json:"wallet_currency"`
	WalletUSD            *float64                         `json:"wallet_usd"`
	WalletUnlimited      bool                             `json:"wallet_unlimited"`
	WalletSource         string                           `json:"wallet_source"`
	WalletReliability    string                           `json:"wallet_reliability"`
	WalletObservedAt     *string                          `json:"wallet_observed_at"`
	LastDiscoveredAt     *string                          `json:"last_discovered_at"`
	LastSyncedAt         *string                          `json:"last_synced_at"`
	NextSyncAt           *string                          `json:"next_sync_at"`
	CreatedAt            string                           `json:"created_at"`
	UpdatedAt            string                           `json:"updated_at"`
	GroupCount           int                              `json:"group_count"`
	BindingCount         int                              `json:"binding_count"`
	BoundAccountIDs      []int64                          `json:"bound_account_ids"`
	Groups               []upstreamGroupResponse          `json:"groups"`
	Bindings             []upstreamAccountBindingResponse `json:"bindings"`
}

func (h *UpstreamConnectionHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	if pageSize > 200 {
		pageSize = 200
	}
	items, total, err := h.service.List(c.Request.Context(), service.UpstreamConnectionListParams{
		Page: page, PageSize: pageSize, Provider: c.Query("provider"), Status: c.Query("status"), Search: c.Query("search"),
		IncludeBindings: c.Query("include_bindings") == "true",
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]upstreamConnectionResponse, 0, len(items))
	for _, item := range items {
		out = append(out, upstreamConnectionToResponse(item))
	}
	response.Paginated(c, out, total, page, pageSize)
}

func (h *UpstreamConnectionHandler) Get(c *gin.Context) {
	id, ok := parseUpstreamConnectionID(c)
	if !ok {
		return
	}
	connection, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, upstreamConnectionToResponse(connection))
}

func (h *UpstreamConnectionHandler) GetTodayUsage(c *gin.Context) {
	id, ok := parseUpstreamConnectionID(c)
	if !ok {
		return
	}
	usage, err := h.service.GetTodayUsage(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, usage)
}

// GetRuntimeOverview returns the compact, local runtime snapshot used by the
// upstream connection list. Account IDs use a JSON body so a large bound-account
// set does not exceed browser or proxy URL limits.
func (h *UpstreamConnectionHandler) GetRuntimeOverview(c *gin.Context) {
	var request upstreamConnectionRuntimeOverviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	accountIDs, ok := validateUpstreamRuntimeAccountIDs(c, request.AccountIDs)
	if !ok {
		return
	}
	overview, err := h.service.GetRuntimeOverview(c.Request.Context(), accountIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, overview)
}

func (h *UpstreamConnectionHandler) Create(c *gin.Context) {
	var request upstreamConnectionCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	provider := strings.TrimSpace(request.Provider)
	if provider == "" {
		provider = service.UpstreamConnectionProviderAuto
	}
	syncEnabled := true
	if request.SyncEnabled != nil {
		syncEnabled = *request.SyncEnabled
	}
	connection, err := h.service.Create(c.Request.Context(), service.UpstreamConnectionCreateParams{
		Name: request.Name, Provider: provider, AuthMode: request.AuthMode,
		ManagementBaseURL: request.ManagementBaseURL, ForwardingBaseURL: request.ForwardingBaseURL,
		Credential:       upstreamCredentialRequestToService(request.Credential, c.Request.UserAgent()),
		NotInCNConfirmed: request.NotInCNConfirmed, RemoteUserID: request.RemoteUserID,
		ProxyID: request.ProxyID, SyncEnabled: syncEnabled, SyncIntervalSeconds: request.SyncIntervalSeconds,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, upstreamConnectionToResponse(connection))
}

func (h *UpstreamConnectionHandler) Update(c *gin.Context) {
	id, ok := parseUpstreamConnectionID(c)
	if !ok {
		return
	}
	var request upstreamConnectionUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	params := service.UpstreamConnectionUpdateParams{
		ExpectedVersion: request.ExpectedVersion,
		Name:            request.Name, Provider: request.Provider, AuthMode: request.AuthMode,
		ManagementBaseURL: request.ManagementBaseURL, ForwardingBaseURL: request.ForwardingBaseURL,
		RemoteUserID: request.RemoteUserID, ProxyID: request.ProxyID, ClearProxy: request.ClearProxy,
		SyncEnabled: request.SyncEnabled, SyncIntervalSeconds: request.SyncIntervalSeconds,
		NotInCNConfirmed: request.NotInCNConfirmed,
	}
	if request.Credential != nil {
		credential := upstreamCredentialRequestToService(*request.Credential, c.Request.UserAgent())
		params.Credential = &credential
	}
	connection, err := h.service.Update(c.Request.Context(), id, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, upstreamConnectionToResponse(connection))
}

func (h *UpstreamConnectionHandler) Delete(c *gin.Context) {
	id, ok := parseUpstreamConnectionID(c)
	if !ok {
		return
	}
	var request upstreamConnectionDeleteRequest
	if c.Request.Body != nil {
		if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
			response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
			return
		}
	}
	// Keep accepting the old query flag, but active bindings still require the
	// confirmed account-ID snapshot from the JSON body.
	if c.Query("unbind_accounts") == "true" {
		request.UnbindAccounts = true
	}
	params := service.UpstreamConnectionDeleteParams{UnbindAccounts: request.UnbindAccounts}
	if request.ExpectedBoundAccountIDs != nil {
		params.HasExpectedBoundAccountIDs = true
		params.ExpectedBoundAccountIDs = append([]int64(nil), (*request.ExpectedBoundAccountIDs)...)
	}
	if err := h.service.Delete(c.Request.Context(), id, params); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}

func (h *UpstreamConnectionHandler) Probe(c *gin.Context) {
	id, ok := parseUpstreamConnectionID(c)
	if !ok {
		return
	}
	connection, err := h.service.Probe(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, upstreamConnectionToResponse(connection))
}

func (h *UpstreamConnectionHandler) BindAccount(c *gin.Context) {
	connectionID, ok := parseUpstreamConnectionID(c)
	if !ok {
		return
	}
	accountID, ok := parseUpstreamBindingAccountID(c)
	if !ok {
		return
	}
	binding, err := h.service.BindAccount(c.Request.Context(), connectionID, accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, upstreamBindingToResponse(*binding))
}

func (h *UpstreamConnectionHandler) GetAccountBinding(c *gin.Context) {
	accountID, ok := parseUpstreamBindingAccountID(c)
	if !ok {
		return
	}
	binding, err := h.service.GetAccountBinding(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, upstreamBindingToResponse(*binding))
}

func (h *UpstreamConnectionHandler) UnbindAccount(c *gin.Context) {
	connectionID, ok := parseUpstreamConnectionID(c)
	if !ok {
		return
	}
	accountID, ok := parseUpstreamBindingAccountID(c)
	if !ok {
		return
	}
	if err := h.service.UnbindAccount(c.Request.Context(), connectionID, accountID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}

func parseUpstreamConnectionID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_UPSTREAM_CONNECTION_ID", "invalid upstream connection id"))
		return 0, false
	}
	return id, true
}

func validateUpstreamRuntimeAccountIDs(c *gin.Context, accountIDs []int64) ([]int64, bool) {
	if len(accountIDs) > 5000 {
		response.ErrorFrom(c, infraerrors.BadRequest("TOO_MANY_UPSTREAM_RUNTIME_ACCOUNTS", "too many account ids"))
		return nil, false
	}
	seen := make(map[int64]struct{}, len(accountIDs))
	ids := make([]int64, 0, len(accountIDs))
	for _, id := range accountIDs {
		if id <= 0 {
			response.ErrorFrom(c, infraerrors.BadRequest("INVALID_UPSTREAM_RUNTIME_ACCOUNT_ID", "invalid account id"))
			return nil, false
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, true
}

func parseUpstreamBindingAccountID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("account_id"), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_UPSTREAM_BINDING_ACCOUNT_ID", "invalid account id"))
		return 0, false
	}
	return id, true
}

func upstreamCredentialRequestToService(request upstreamConnectionCredentialRequest, requestUserAgent string) service.UpstreamConnectionCredentialInput {
	userAgent := strings.TrimSpace(request.UserAgent)
	if userAgent == "" {
		userAgent = strings.TrimSpace(requestUserAgent)
	}
	return service.UpstreamConnectionCredentialInput{
		Username: request.Username, Password: request.Password,
		AccessToken: request.AccessToken, RefreshToken: request.RefreshToken,
		UserAgent: userAgent,
	}
}

func upstreamConnectionToResponse(connection *service.UpstreamConnection) upstreamConnectionResponse {
	groups := make([]upstreamGroupResponse, 0, len(connection.Groups))
	for _, group := range connection.Groups {
		groups = append(groups, upstreamGroupResponse{
			ID: group.ID, RemoteID: group.RemoteID, Name: group.Name, RateMultiplier: group.RateMultiplier,
			Source: group.Source, Confidence: group.Confidence, Metadata: nonNilHandlerMap(group.Metadata),
			ObservedAt: formatOptionalTime(group.ObservedAt), FreshUntil: formatOptionalTime(group.FreshUntil),
		})
	}
	bindings := make([]upstreamAccountBindingResponse, 0, len(connection.Bindings))
	for _, binding := range connection.Bindings {
		bindings = append(bindings, upstreamBindingToResponse(binding))
	}
	return upstreamConnectionResponse{
		ID: connection.ID, Name: connection.Name, Provider: connection.Provider, AuthMode: connection.AuthMode,
		ManagementBaseURL: connection.ManagementBaseURL, ForwardingBaseURL: connection.ForwardingBaseURL,
		CredentialConfigured: connection.CredentialHint != "", CredentialHint: connection.CredentialHint,
		NotInCNConfirmed: connection.NotInCNConfirmed,
		RemoteUserID:     connection.RemoteUserID, ProxyID: connection.ProxyID,
		Capabilities: nonNilHandlerMap(connection.Capabilities), Status: connection.Status, LastError: connection.LastError,
		SyncEnabled: connection.SyncEnabled, SyncIntervalSeconds: connection.SyncIntervalSeconds,
		SyncFailures: connection.SyncFailures, Version: connection.Version,
		WalletAmount: connection.WalletAmount, WalletCurrency: connection.WalletCurrency, WalletUSD: connection.WalletUSD,
		WalletUnlimited: connection.WalletUnlimited, WalletSource: connection.WalletSource,
		WalletReliability: connection.WalletReliability, WalletObservedAt: formatOptionalTime(connection.WalletObservedAt),
		LastDiscoveredAt: formatOptionalTime(connection.LastDiscoveredAt), LastSyncedAt: formatOptionalTime(connection.LastSyncedAt),
		NextSyncAt: formatOptionalTime(connection.NextSyncAt), CreatedAt: connection.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: connection.UpdatedAt.UTC().Format(time.RFC3339), GroupCount: connection.GroupCount,
		BindingCount: connection.BindingCount, BoundAccountIDs: nonNilHandlerInt64s(connection.BoundAccountIDs),
		Groups: groups, Bindings: bindings,
	}
}

func upstreamBindingToResponse(binding service.UpstreamAccountBinding) upstreamAccountBindingResponse {
	return upstreamAccountBindingResponse{
		ID: binding.ID, AccountID: binding.AccountID, ConnectionID: binding.ConnectionID, RemoteTokenID: binding.RemoteTokenID,
		RemoteTokenName: binding.RemoteTokenName, ResolutionKind: binding.ResolutionKind,
		RemoteGroupID: binding.RemoteGroupID, RemoteGroupName: binding.RemoteGroupName,
		FallbackGroups: nonNilHandlerStrings(binding.FallbackGroups), ObservedMultiplier: binding.ObservedMultiplier,
		Confidence: binding.Confidence, Source: binding.Source, ApplyPolicy: binding.ApplyPolicy,
		Status: binding.Status, SyncFailures: binding.SyncFailures, LastError: binding.LastError,
		ResolutionDetails: nonNilHandlerMap(binding.ResolutionDetails),
		ObservedAt:        formatOptionalTime(binding.ObservedAt), FreshUntil: formatOptionalTime(binding.FreshUntil),
	}
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339)
	return &formatted
}

func nonNilHandlerMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func nonNilHandlerStrings(value []string) []string {
	if value == nil {
		return []string{}
	}
	return value
}

func nonNilHandlerInt64s(value []int64) []int64 {
	if value == nil {
		return []int64{}
	}
	return value
}
