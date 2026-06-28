package handler

import (
	"context"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type ModelPriceHandler struct {
	channelService       *service.ChannelService
	accountService       *service.AccountService
	apiKeyService        *service.APIKeyService
	groupService         *service.GroupService
	pricingService       modelPriceCatalog
	settingService       *service.SettingService
	paymentConfigService *service.PaymentConfigService
}

type modelPriceCatalog interface {
	GetModelPricing(model string) *service.LiteLLMModelPricing
	ListModelNamesByProvider(provider string) []string
}

type modelPriceCatalogManager interface {
	GetStatus() map[string]any
	ForceUpdate() error
}

func NewModelPriceHandler(
	channelService *service.ChannelService,
	accountService *service.AccountService,
	apiKeyService *service.APIKeyService,
	groupService *service.GroupService,
	pricingService *service.PricingService,
	settingService *service.SettingService,
	paymentConfigService *service.PaymentConfigService,
) *ModelPriceHandler {
	return &ModelPriceHandler{
		channelService:       channelService,
		accountService:       accountService,
		apiKeyService:        apiKeyService,
		groupService:         groupService,
		pricingService:       pricingService,
		settingService:       settingService,
		paymentConfigService: paymentConfigService,
	}
}

type modelPricePlanDTO struct {
	ID             int64   `json:"id"`
	Name           string  `json:"name"`
	PriceCNY       float64 `json:"price_cny"`
	QuotaUSD       float64 `json:"quota_usd"`
	CNYPerQuotaUSD float64 `json:"cny_per_quota_usd"`
	USDMultiplier  float64 `json:"usd_multiplier"`
}

type modelPriceGroupDTO struct {
	ID                   int64              `json:"id"`
	Name                 string             `json:"name"`
	Platform             string             `json:"platform"`
	SubscriptionType     string             `json:"subscription_type"`
	RateMultiplier       float64            `json:"rate_multiplier"`
	EffectiveMultiplier  float64            `json:"effective_multiplier"`
	UserRateMultiplier   *float64           `json:"user_rate_multiplier,omitempty"`
	ImageRateIndependent bool               `json:"image_rate_independent"`
	ImageRateMultiplier  float64            `json:"image_rate_multiplier"`
	IsExclusive          bool               `json:"is_exclusive"`
	Hidden               bool               `json:"hidden"`
	ModelCount           int                `json:"model_count"`
	ChannelCount         int                `json:"channel_count"`
	BestPlan             *modelPricePlanDTO `json:"best_plan,omitempty"`
}

type modelPriceValueDTO struct {
	InputUSDPerM       *float64 `json:"input_usd_per_m"`
	OutputUSDPerM      *float64 `json:"output_usd_per_m"`
	CacheWriteUSDPerM  *float64 `json:"cache_write_usd_per_m"`
	CacheReadUSDPerM   *float64 `json:"cache_read_usd_per_m"`
	ImageOutputUSDPerM *float64 `json:"image_output_usd_per_m"`
	PerRequestUSD      *float64 `json:"per_request_usd"`
}

type modelPriceActualDTO struct {
	InputUSDPerM       *float64 `json:"input_usd_per_m"`
	InputCNYPerM       *float64 `json:"input_cny_per_m"`
	OutputUSDPerM      *float64 `json:"output_usd_per_m"`
	OutputCNYPerM      *float64 `json:"output_cny_per_m"`
	CacheWriteUSDPerM  *float64 `json:"cache_write_usd_per_m"`
	CacheWriteCNYPerM  *float64 `json:"cache_write_cny_per_m"`
	CacheReadUSDPerM   *float64 `json:"cache_read_usd_per_m"`
	CacheReadCNYPerM   *float64 `json:"cache_read_cny_per_m"`
	ImageOutputUSDPerM *float64 `json:"image_output_usd_per_m"`
	ImageOutputCNYPerM *float64 `json:"image_output_cny_per_m"`
	PerRequestUSD      *float64 `json:"per_request_usd"`
	PerRequestCNY      *float64 `json:"per_request_cny"`
}

type modelPriceTierDTO struct {
	Key             string              `json:"key"`
	Label           string              `json:"label"`
	ThresholdTokens *int                `json:"threshold_tokens,omitempty"`
	Official        modelPriceValueDTO  `json:"official"`
	Actual          modelPriceActualDTO `json:"actual"`
}

type modelPriceModelDTO struct {
	Name            string                         `json:"name"`
	Platform        string                         `json:"platform"`
	Provider        string                         `json:"provider"`
	BillingMode     string                         `json:"billing_mode"`
	PricingSource   string                         `json:"pricing_source"`
	Official        modelPriceValueDTO             `json:"official"`
	Actual          modelPriceActualDTO            `json:"actual"`
	PriceTiers      []modelPriceTierDTO            `json:"price_tiers"`
	Multiplier      float64                        `json:"multiplier"`
	CheaperFactor   *float64                       `json:"cheaper_factor"`
	ChannelNames    []string                       `json:"channel_names"`
	OfficialMissing bool                           `json:"official_missing"`
	CustomPrice     *service.ModelPriceCustomPrice `json:"custom_price,omitempty"`
	Hidden          bool                           `json:"hidden"`
}

type modelPriceSummaryDTO struct {
	ModelCount           int      `json:"model_count"`
	PricedCount          int      `json:"priced_count"`
	AverageCheaperFactor *float64 `json:"average_cheaper_factor"`
}

type modelPriceGroupOverviewDTO struct {
	Category     string `json:"category"`
	GroupCount   int    `json:"group_count"`
	ModelCount   int    `json:"model_count"`
	ChannelCount int    `json:"channel_count"`
}

type modelPriceCatalogStatusDTO struct {
	ModelCount  int    `json:"model_count"`
	LastUpdated string `json:"last_updated,omitempty"`
	LocalHash   string `json:"local_hash,omitempty"`
}

type modelPriceResponseDTO struct {
	USDCNYRate          float64                      `json:"usd_cny_rate"`
	Groups              []modelPriceGroupDTO         `json:"groups"`
	GroupOverview       []modelPriceGroupOverviewDTO `json:"group_overview"`
	SelectedGroupID     *int64                       `json:"selected_group_id"`
	Models              []modelPriceModelDTO         `json:"models"`
	Summary             modelPriceSummaryDTO         `json:"summary"`
	CatalogStatus       *modelPriceCatalogStatusDTO  `json:"catalog_status,omitempty"`
	IncludeCatalog      bool                         `json:"include_catalog"`
	ShowHiddenGroups    bool                         `json:"show_hidden_groups"`
	ShowHiddenModels    bool                         `json:"show_hidden_models"`
	HiddenGroupIDs      []int64                      `json:"hidden_group_ids"`
	HiddenModelKeys     []string                     `json:"hidden_model_keys"`
	SelectedGroupHidden bool                         `json:"selected_group_hidden"`
}

type updateModelPriceHiddenGroupsRequest struct {
	HiddenGroupIDs []int64 `json:"hidden_group_ids"`
}

type updateModelPriceHiddenGroupsResponse struct {
	HiddenGroupIDs []int64 `json:"hidden_group_ids"`
}

type updateModelPriceHiddenModelRequest struct {
	GroupID int64    `json:"group_id"`
	Model   string   `json:"model"`
	Models  []string `json:"models"`
	Hidden  bool     `json:"hidden"`
}

type updateModelPriceHiddenModelResponse struct {
	HiddenModelKeys []string `json:"hidden_model_keys"`
}

type updateModelPriceCustomPriceRequest struct {
	GroupID            int64    `json:"group_id"`
	Model              string   `json:"model"`
	BillingMode        string   `json:"billing_mode"`
	InputUSDPerM       *float64 `json:"input_usd_per_m"`
	OutputUSDPerM      *float64 `json:"output_usd_per_m"`
	CacheWriteUSDPerM  *float64 `json:"cache_write_usd_per_m"`
	CacheReadUSDPerM   *float64 `json:"cache_read_usd_per_m"`
	ImageOutputUSDPerM *float64 `json:"image_output_usd_per_m"`
	PerRequestUSD      *float64 `json:"per_request_usd"`
	Clear              bool     `json:"clear"`
}

type updateModelPriceCustomPriceResponse struct {
	CustomPrices map[string]service.ModelPriceCustomPrice `json:"custom_prices"`
}

// List returns a group-centric model price view for admins and regular users.
// GET /api/v1/model-prices?group_id=123
func (h *ModelPriceHandler) List(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	role, _ := middleware.GetUserRoleFromContext(c)
	isAdmin := role == service.RoleAdmin
	if !isAdmin && !h.settingService.IsModelPricesUserVisible(c.Request.Context()) {
		response.Forbidden(c, "Model prices page is disabled")
		return
	}
	showHiddenGroups := isAdmin && parseModelPriceBoolQuery(c.Query("show_hidden_groups"))
	showHiddenModels := isAdmin && parseModelPriceBoolQuery(c.Query("show_hidden_models"))
	includeCatalog := isAdmin && parseModelPriceBoolQuery(c.Query("include_catalog"))
	groups, err := h.visibleGroups(c, subject.UserID, isAdmin)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	groupDTOs := h.groupDTOs(c, groups, subject.UserID, isAdmin)
	channels, err := h.channelService.ListAvailable(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	accountsByGroup, err := h.modelPriceAccountsByGroup(c.Request.Context(), groupDTOs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	hiddenModelKeys := h.settingService.GetModelPriceHiddenModelKeys(c.Request.Context())
	applyModelPriceGroupUsage(groupDTOs, channels, accountsByGroup, hiddenModelKeys, showHiddenModels)
	usdCNYRate := h.settingService.GetModelPriceUSDCNYRate(c.Request.Context())
	h.applyPlanEconomics(c.Request.Context(), groupDTOs, usdCNYRate)
	hiddenGroupIDs := h.settingService.GetModelPriceHiddenGroupIDs(c.Request.Context())
	groupDTOs = applyModelPriceHiddenGroups(groupDTOs, hiddenGroupIDs, showHiddenGroups)
	selectedGroupID := selectModelPriceGroupID(c.Query("group_id"), groupDTOs)
	var selected *modelPriceGroupDTO
	if selectedGroupID != nil {
		for i := range groupDTOs {
			if groupDTOs[i].ID == *selectedGroupID {
				selected = &groupDTOs[i]
				break
			}
		}
	}

	models := []modelPriceModelDTO{}
	if selected != nil {
		customPrices := h.settingService.GetModelPriceCustomPrices(c.Request.Context())
		models = h.modelsForGroup(channels, accountsByGroup[selected.ID], *selected, usdCNYRate, includeCatalog, customPrices)
		models = applyModelPriceHiddenModels(models, selected.ID, hiddenModelKeys, showHiddenModels)
		if !isAdmin {
			models = sanitizeModelPriceModelsForUser(models)
		}
	}
	responseHiddenModelKeys := map[string]struct{}{}
	if isAdmin {
		responseHiddenModelKeys = hiddenModelKeys
	}

	response.Success(c, modelPriceResponseDTO{
		USDCNYRate:          usdCNYRate,
		Groups:              groupDTOs,
		GroupOverview:       buildModelPriceGroupOverview(groupDTOs, channels, accountsByGroup, hiddenModelKeys, showHiddenModels),
		SelectedGroupID:     selectedGroupID,
		Models:              models,
		Summary:             summarizeModelPrices(models),
		CatalogStatus:       h.catalogStatus(isAdmin),
		IncludeCatalog:      includeCatalog,
		ShowHiddenGroups:    showHiddenGroups,
		ShowHiddenModels:    showHiddenModels,
		HiddenGroupIDs:      sortedInt64SetKeys(hiddenGroupIDs),
		HiddenModelKeys:     sortedModelPriceHandlerStringSetKeys(responseHiddenModelKeys),
		SelectedGroupHidden: selected != nil && selected.Hidden,
	})
}

func sanitizeModelPriceModelsForUser(models []modelPriceModelDTO) []modelPriceModelDTO {
	for i := range models {
		models[i].CustomPrice = nil
		if models[i].PricingSource == "custom" {
			models[i].PricingSource = "official"
		}
	}
	return models
}

func (h *ModelPriceHandler) UpdateHiddenGroups(c *gin.Context) {
	var req updateModelPriceHiddenGroupsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	ids, err := h.settingService.SetModelPriceHiddenGroupIDs(c.Request.Context(), req.HiddenGroupIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updateModelPriceHiddenGroupsResponse{HiddenGroupIDs: ids})
}

func (h *ModelPriceHandler) UpdateHiddenModel(c *gin.Context) {
	var req updateModelPriceHiddenModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	models := req.Models
	if strings.TrimSpace(req.Model) != "" {
		models = append(models, req.Model)
	}
	keys, err := h.settingService.SetModelPriceHiddenModels(c.Request.Context(), req.GroupID, models, req.Hidden)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updateModelPriceHiddenModelResponse{HiddenModelKeys: keys})
}

func (h *ModelPriceHandler) UpdateCustomPrice(c *gin.Context) {
	var req updateModelPriceCustomPriceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	var price *service.ModelPriceCustomPrice
	if !req.Clear {
		price = &service.ModelPriceCustomPrice{
			BillingMode:        normalizeModelPriceBillingMode(req.BillingMode),
			InputUSDPerM:       req.InputUSDPerM,
			OutputUSDPerM:      req.OutputUSDPerM,
			CacheWriteUSDPerM:  req.CacheWriteUSDPerM,
			CacheReadUSDPerM:   req.CacheReadUSDPerM,
			ImageOutputUSDPerM: req.ImageOutputUSDPerM,
			PerRequestUSD:      req.PerRequestUSD,
		}
	}
	customPrices, err := h.settingService.SetModelPriceCustomPrice(c.Request.Context(), req.GroupID, req.Model, price)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updateModelPriceCustomPriceResponse{CustomPrices: customPrices})
}

func (h *ModelPriceHandler) SyncCatalog(c *gin.Context) {
	manager, ok := h.pricingService.(modelPriceCatalogManager)
	if !ok || manager == nil {
		response.BadRequest(c, "Pricing catalog sync is unavailable")
		return
	}
	if err := manager.ForceUpdate(); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, modelPriceCatalogStatusFromMap(manager.GetStatus()))
}

func (h *ModelPriceHandler) catalogStatus(isAdmin bool) *modelPriceCatalogStatusDTO {
	if !isAdmin {
		return nil
	}
	manager, ok := h.pricingService.(modelPriceCatalogManager)
	if !ok || manager == nil {
		return nil
	}
	return modelPriceCatalogStatusFromMap(manager.GetStatus())
}

func modelPriceCatalogStatusFromMap(status map[string]any) *modelPriceCatalogStatusDTO {
	out := &modelPriceCatalogStatusDTO{}
	switch v := status["model_count"].(type) {
	case int:
		out.ModelCount = v
	case int64:
		out.ModelCount = int(v)
	case float64:
		out.ModelCount = int(v)
	}
	switch v := status["last_updated"].(type) {
	case time.Time:
		if !v.IsZero() {
			out.LastUpdated = v.Format(time.RFC3339)
		}
	case string:
		out.LastUpdated = strings.TrimSpace(v)
	}
	if hash, ok := status["local_hash"].(string); ok {
		out.LocalHash = strings.TrimSpace(hash)
	}
	return out
}

func (h *ModelPriceHandler) visibleGroups(c *gin.Context, userID int64, isAdmin bool) ([]service.Group, error) {
	if isAdmin {
		return h.groupService.ListActive(c.Request.Context())
	}
	available, err := h.apiKeyService.GetAvailableGroupsWithEntitlements(c.Request.Context(), userID)
	if err != nil {
		return nil, err
	}
	groups := make([]service.Group, 0, len(available))
	for i := range available {
		if !available[i].Group.IsActive() {
			continue
		}
		groups = append(groups, available[i].Group)
	}
	return groups, nil
}

func (h *ModelPriceHandler) groupDTOs(c *gin.Context, groups []service.Group, userID int64, isAdmin bool) []modelPriceGroupDTO {
	userRates := map[int64]float64{}
	if !isAdmin {
		rates, err := h.apiKeyService.GetUserGroupRates(c.Request.Context(), userID)
		if err == nil && rates != nil {
			userRates = rates
		}
	}

	out := make([]modelPriceGroupDTO, 0, len(groups))
	for i := range groups {
		g := groups[i]
		if !g.IsActive() {
			continue
		}
		effective := g.RateMultiplier
		var userRate *float64
		if rate, ok := userRates[g.ID]; ok {
			effective = rate
			userRate = &rate
		}
		out = append(out, modelPriceGroupDTO{
			ID:                   g.ID,
			Name:                 g.Name,
			Platform:             g.Platform,
			SubscriptionType:     g.SubscriptionType,
			RateMultiplier:       g.RateMultiplier,
			EffectiveMultiplier:  normalizeMultiplier(effective),
			UserRateMultiplier:   userRate,
			ImageRateIndependent: g.ImageRateIndependent,
			ImageRateMultiplier:  normalizeMultiplier(g.ImageRateMultiplier),
			IsExclusive:          g.IsExclusive,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Platform == out[j].Platform {
			return out[i].Name < out[j].Name
		}
		return out[i].Platform < out[j].Platform
	})
	return out
}

func applyModelPriceHiddenGroups(groups []modelPriceGroupDTO, hidden map[int64]struct{}, includeHidden bool) []modelPriceGroupDTO {
	if len(groups) == 0 || len(hidden) == 0 {
		return groups
	}
	out := make([]modelPriceGroupDTO, 0, len(groups))
	for i := range groups {
		_, isHidden := hidden[groups[i].ID]
		groups[i].Hidden = isHidden
		if isHidden && !includeHidden {
			continue
		}
		out = append(out, groups[i])
	}
	return out
}

func applyModelPriceHiddenModels(models []modelPriceModelDTO, groupID int64, hidden map[string]struct{}, includeHidden bool) []modelPriceModelDTO {
	if len(models) == 0 || len(hidden) == 0 {
		return models
	}
	out := make([]modelPriceModelDTO, 0, len(models))
	for i := range models {
		_, isHidden := hidden[service.ModelPriceHiddenModelKey(groupID, models[i].Name)]
		models[i].Hidden = isHidden
		if isHidden && !includeHidden {
			continue
		}
		out = append(out, models[i])
	}
	return out
}

func (h *ModelPriceHandler) modelPriceAccountsByGroup(ctx context.Context, groups []modelPriceGroupDTO) (map[int64][]service.Account, error) {
	out := make(map[int64][]service.Account, len(groups))
	if h.accountService == nil || len(groups) == 0 {
		return out, nil
	}
	for i := range groups {
		accounts, err := h.accountService.ListByGroup(ctx, groups[i].ID)
		if err != nil {
			return nil, err
		}
		out[groups[i].ID] = accounts
	}
	return out, nil
}

func applyModelPriceGroupUsage(groups []modelPriceGroupDTO, channels []service.AvailableChannel, accountsByGroup map[int64][]service.Account, hiddenModels map[string]struct{}, includeHiddenModels bool) {
	if len(groups) == 0 {
		return
	}
	platformByGroup := make(map[int64]string, len(groups))
	channelCounts := make(map[int64]int, len(groups))
	accountCounts := make(map[int64]int, len(groups))
	modelNamesByGroup := make(map[int64]map[string]struct{}, len(groups))
	for i := range groups {
		platformByGroup[groups[i].ID] = groups[i].Platform
		modelNamesByGroup[groups[i].ID] = make(map[string]struct{})
	}
	for _, ch := range channels {
		if ch.Status != service.StatusActive {
			continue
		}
		for _, group := range ch.Groups {
			platform, ok := platformByGroup[group.ID]
			if !ok {
				continue
			}
			channelCounts[group.ID]++
			for _, model := range ch.SupportedModels {
				if model.Platform != platform || strings.TrimSpace(model.Name) == "" {
					continue
				}
				if modelPriceModelHidden(group.ID, strings.TrimSpace(model.Name), hiddenModels, includeHiddenModels) {
					continue
				}
				key := strings.ToLower(model.Platform + "\x00" + strings.TrimSpace(model.Name))
				modelNamesByGroup[group.ID][key] = struct{}{}
			}
		}
	}
	for groupID, accounts := range accountsByGroup {
		platform, ok := platformByGroup[groupID]
		if !ok {
			continue
		}
		for i := range accounts {
			account := accounts[i]
			if !modelPriceAccountUsableForGroup(account, platform) {
				continue
			}
			accountCounts[groupID]++
			for _, model := range accountModelPriceNames(account, platform) {
				if modelPriceModelHidden(groupID, model, hiddenModels, includeHiddenModels) {
					continue
				}
				key := strings.ToLower(platform + "\x00" + model)
				modelNamesByGroup[groupID][key] = struct{}{}
			}
		}
	}
	for i := range groups {
		groups[i].ChannelCount = channelCounts[groups[i].ID]
		if groups[i].ChannelCount == 0 {
			groups[i].ChannelCount = accountCounts[groups[i].ID]
		}
		groups[i].ModelCount = len(modelNamesByGroup[groups[i].ID])
	}
}

func buildModelPriceGroupOverview(groups []modelPriceGroupDTO, channels []service.AvailableChannel, accountsByGroup map[int64][]service.Account, hiddenModels map[string]struct{}, includeHiddenModels bool) []modelPriceGroupOverviewDTO {
	type aggregate struct {
		groups   int
		models   map[string]struct{}
		channels map[string]struct{}
	}
	categories := []string{"claude", "openai", "gemini", "domestic"}
	byCategory := make(map[string]*aggregate, len(categories))
	for _, category := range categories {
		byCategory[category] = &aggregate{
			models:   make(map[string]struct{}),
			channels: make(map[string]struct{}),
		}
	}
	platformByGroup := make(map[int64]string, len(groups))
	categoryByGroup := make(map[int64]string, len(groups))
	for _, group := range groups {
		category := modelPriceGroupCategory(group.Platform, group.Name)
		byCategory[category].groups++
		platformByGroup[group.ID] = group.Platform
		categoryByGroup[group.ID] = category
	}
	channelCountsByGroup := make(map[int64]int, len(groups))
	for _, ch := range channels {
		if ch.Status != service.StatusActive {
			continue
		}
		for _, group := range ch.Groups {
			category, ok := categoryByGroup[group.ID]
			if !ok {
				continue
			}
			byCategory[category].channels[modelPriceChannelKey(ch)] = struct{}{}
			channelCountsByGroup[group.ID]++
			platform := platformByGroup[group.ID]
			for _, model := range ch.SupportedModels {
				if model.Platform != platform || strings.TrimSpace(model.Name) == "" {
					continue
				}
				if modelPriceModelHidden(group.ID, strings.TrimSpace(model.Name), hiddenModels, includeHiddenModels) {
					continue
				}
				key := strings.ToLower(model.Platform + "\x00" + strings.TrimSpace(model.Name))
				byCategory[category].models[key] = struct{}{}
			}
		}
	}
	for groupID, accounts := range accountsByGroup {
		category, ok := categoryByGroup[groupID]
		if !ok || channelCountsByGroup[groupID] > 0 {
			continue
		}
		platform := platformByGroup[groupID]
		for i := range accounts {
			account := accounts[i]
			if !modelPriceAccountUsableForGroup(account, platform) {
				continue
			}
			byCategory[category].channels["account:"+strconv.FormatInt(account.ID, 10)] = struct{}{}
			for _, model := range accountModelPriceNames(account, platform) {
				if modelPriceModelHidden(groupID, model, hiddenModels, includeHiddenModels) {
					continue
				}
				key := strings.ToLower(platform + "\x00" + model)
				byCategory[category].models[key] = struct{}{}
			}
		}
	}
	out := make([]modelPriceGroupOverviewDTO, 0, len(categories))
	for _, category := range categories {
		agg := byCategory[category]
		out = append(out, modelPriceGroupOverviewDTO{
			Category:     category,
			GroupCount:   agg.groups,
			ModelCount:   len(agg.models),
			ChannelCount: len(agg.channels),
		})
	}
	return out
}

func modelPriceModelHidden(groupID int64, model string, hiddenModels map[string]struct{}, includeHiddenModels bool) bool {
	if includeHiddenModels || len(hiddenModels) == 0 {
		return false
	}
	_, ok := hiddenModels[service.ModelPriceHiddenModelKey(groupID, model)]
	return ok
}

func modelPriceChannelKey(ch service.AvailableChannel) string {
	if ch.ID > 0 {
		return strconv.FormatInt(ch.ID, 10)
	}
	return strings.ToLower(strings.TrimSpace(ch.Name))
}

func modelPriceGroupCategory(platform, name string) string {
	text := strings.ToLower(strings.TrimSpace(platform + " " + name))
	if strings.Contains(name, "国模") ||
		strings.Contains(text, "deepseek") ||
		strings.Contains(text, "qwen") ||
		strings.Contains(text, "kimi") ||
		strings.Contains(text, "glm") ||
		strings.Contains(text, "minimax") ||
		strings.Contains(text, "moonshot") ||
		strings.Contains(text, "doubao") {
		return "domestic"
	}
	lower := strings.ToLower(strings.TrimSpace(platform))
	switch {
	case strings.Contains(lower, "anthropic") || strings.Contains(lower, "claude"):
		return "claude"
	case strings.Contains(lower, "openai"):
		return "openai"
	case strings.Contains(lower, "gemini") || strings.Contains(lower, "google") || strings.Contains(lower, "vertex"):
		return "gemini"
	default:
		return "domestic"
	}
}

func (h *ModelPriceHandler) applyPlanEconomics(ctx context.Context, groups []modelPriceGroupDTO, usdCNYRate float64) {
	if h.paymentConfigService == nil || usdCNYRate <= 0 {
		return
	}
	plans, err := h.paymentConfigService.ListPlanResponsesForSale(ctx)
	if err != nil {
		return
	}
	bestByGroup := make(map[int64]modelPricePlanDTO)
	for _, plan := range plans {
		quotaUSD := planPackageQuotaUSD(plan)
		if plan.ID <= 0 || plan.Price <= 0 || quotaUSD <= 0 {
			continue
		}
		cnyPerQuotaUSD := plan.Price / quotaUSD
		if cnyPerQuotaUSD <= 0 || math.IsNaN(cnyPerQuotaUSD) || math.IsInf(cnyPerQuotaUSD, 0) {
			continue
		}
		candidate := modelPricePlanDTO{
			ID:             plan.ID,
			Name:           plan.Name,
			PriceCNY:       roundPrice(plan.Price),
			QuotaUSD:       roundPrice(quotaUSD),
			CNYPerQuotaUSD: roundPrice(cnyPerQuotaUSD),
			USDMultiplier:  roundPrice(cnyPerQuotaUSD / usdCNYRate),
		}
		for _, groupID := range planModelPriceGroupIDs(plan) {
			existing, ok := bestByGroup[groupID]
			if !ok || candidate.CNYPerQuotaUSD < existing.CNYPerQuotaUSD {
				bestByGroup[groupID] = candidate
			}
		}
	}
	for i := range groups {
		if best, ok := bestByGroup[groups[i].ID]; ok {
			groups[i].BestPlan = &best
		}
	}
}

func planPackageQuotaUSD(plan service.SubscriptionPlanResponse) float64 {
	validityDays := planValidityDays(plan.ValidityDays, plan.ValidityUnit)
	if validityDays <= 0 {
		validityDays = 30
	}
	if plan.MonthlyLimitUSD != nil && *plan.MonthlyLimitUSD > 0 {
		return *plan.MonthlyLimitUSD * float64(validityDays) / 30
	}
	if plan.WeeklyLimitUSD != nil && *plan.WeeklyLimitUSD > 0 {
		return *plan.WeeklyLimitUSD * float64(validityDays) / 7
	}
	if plan.DailyLimitUSD != nil && *plan.DailyLimitUSD > 0 {
		return *plan.DailyLimitUSD * float64(validityDays)
	}
	return 0
}

func planValidityDays(days int, unit string) int {
	if days <= 0 {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "year", "years":
		return days * 365
	case "month", "months":
		return days * 30
	case "week", "weeks":
		return days * 7
	case "day", "days", "":
		return days
	default:
		return days
	}
}

func planModelPriceGroupIDs(plan service.SubscriptionPlanResponse) []int64 {
	seen := map[int64]struct{}{}
	out := make([]int64, 0, len(plan.GroupIDs)+1)
	add := func(id int64) {
		if id <= 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	add(plan.GroupID)
	for _, id := range plan.GroupIDs {
		add(id)
	}
	for _, group := range plan.Groups {
		add(group.ID)
	}
	return out
}

func selectModelPriceGroupID(raw string, groups []modelPriceGroupDTO) *int64 {
	if len(groups) == 0 {
		return nil
	}
	if strings.TrimSpace(raw) != "" {
		if id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil {
			for i := range groups {
				if groups[i].ID == id {
					return &id
				}
			}
		}
	}
	return nil
}

type modelAggregate struct {
	name         string
	platform     string
	billingMode  string
	pricing      *service.ChannelModelPricing
	channelNames map[string]struct{}
}

func (h *ModelPriceHandler) modelsForGroup(channels []service.AvailableChannel, accounts []service.Account, group modelPriceGroupDTO, usdCNYRate float64, includeCatalog bool, customPrices map[string]service.ModelPriceCustomPrice) []modelPriceModelDTO {
	aggregates := collectModelPriceAggregates(channels, accounts, group)
	if includeCatalog {
		h.seedCatalogModelsForGroup(aggregates, group)
	}

	models := make([]modelPriceModelDTO, 0, len(aggregates))
	for _, agg := range aggregates {
		model := h.toModelPriceDTO(agg, group, usdCNYRate)
		if custom, ok := customPrices[service.ModelPriceCustomPriceKey(group.ID, agg.name)]; ok && custom.HasPrice() {
			model = applyModelPriceCustomPrice(model, custom, usdCNYRate)
		}
		models = append(models, model)
	}
	sort.SliceStable(models, func(i, j int) bool {
		return strings.ToLower(models[i].Name) < strings.ToLower(models[j].Name)
	})
	return models
}

func collectModelPriceAggregates(channels []service.AvailableChannel, accounts []service.Account, group modelPriceGroupDTO) map[string]*modelAggregate {
	aggregates := make(map[string]*modelAggregate)
	for _, ch := range channels {
		if ch.Status != service.StatusActive || !channelHasGroup(ch, group.ID) {
			continue
		}
		for _, m := range ch.SupportedModels {
			if m.Platform != group.Platform || strings.TrimSpace(m.Name) == "" {
				continue
			}
			key := strings.ToLower(m.Platform + "\x00" + m.Name)
			agg, ok := aggregates[key]
			if !ok {
				agg = &modelAggregate{
					name:         m.Name,
					platform:     m.Platform,
					billingMode:  string(service.BillingModeToken),
					pricing:      m.Pricing,
					channelNames: make(map[string]struct{}),
				}
				aggregates[key] = agg
			}
			if pricingHasValues(m.Pricing) && !pricingHasValues(agg.pricing) {
				agg.pricing = m.Pricing
			}
			if m.Pricing != nil && m.Pricing.BillingMode != "" {
				agg.billingMode = string(m.Pricing.BillingMode)
			}
			agg.channelNames[ch.Name] = struct{}{}
		}
	}
	for i := range accounts {
		account := accounts[i]
		if !modelPriceAccountUsableForGroup(account, group.Platform) {
			continue
		}
		for _, model := range accountModelPriceNames(account, group.Platform) {
			key := strings.ToLower(group.Platform + "\x00" + model)
			agg, ok := aggregates[key]
			if !ok {
				agg = &modelAggregate{
					name:         model,
					platform:     group.Platform,
					billingMode:  string(service.BillingModeToken),
					channelNames: make(map[string]struct{}),
				}
				aggregates[key] = agg
			}
			agg.channelNames[account.Name] = struct{}{}
		}
	}
	return aggregates
}

func (h *ModelPriceHandler) seedCatalogModelsForGroup(aggregates map[string]*modelAggregate, group modelPriceGroupDTO) {
	if h.pricingService == nil {
		return
	}
	for _, provider := range catalogProvidersForPlatform(group.Platform) {
		for _, name := range h.pricingService.ListModelNamesByProvider(provider) {
			name = strings.TrimSpace(name)
			if name == "" || !catalogModelMatchesGroupPlatform(name, group.Platform) {
				continue
			}
			key := strings.ToLower(group.Platform + "\x00" + name)
			if _, ok := aggregates[key]; ok {
				continue
			}
			billingMode := string(service.BillingModeToken)
			if p := h.pricingService.GetModelPricing(name); p != nil && p.Mode == "image_generation" {
				billingMode = string(service.BillingModeImage)
			}
			aggregates[key] = &modelAggregate{
				name:         name,
				platform:     group.Platform,
				billingMode:  billingMode,
				channelNames: make(map[string]struct{}),
			}
		}
	}
}

func catalogProvidersForPlatform(platform string) []string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case service.PlatformOpenAI:
		return []string{"openai", "text-completion-openai"}
	case service.PlatformAnthropic:
		return []string{"anthropic"}
	case service.PlatformGemini:
		return []string{"google", "gemini", "vertex_ai-language-models", "vertex_ai-embedding-models"}
	case service.PlatformAntigravity:
		return []string{"anthropic", "google", "gemini", "vertex_ai-language-models", "vertex_ai-embedding-models"}
	default:
		return nil
	}
}

func catalogModelMatchesGroupPlatform(modelName, platform string) bool {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case service.PlatformAnthropic:
		return strings.HasPrefix(modelName, "claude-")
	case service.PlatformGemini:
		return strings.Contains(modelName, "gemini")
	case service.PlatformAntigravity:
		return strings.HasPrefix(modelName, "claude-") || strings.Contains(modelName, "gemini")
	default:
		return true
	}
}

func (h *ModelPriceHandler) toModelPriceDTO(agg *modelAggregate, group modelPriceGroupDTO, usdCNYRate float64) modelPriceModelDTO {
	official := modelPriceValueDTO{}
	priceTiers := []modelPriceTierDTO{}
	provider := agg.platform
	source := "unknown"
	officialMissing := true
	if h.pricingService != nil {
		if p := h.pricingService.GetModelPricing(agg.name); p != nil {
			official = priceValueFromLiteLLM(p)
			priceTiers = priceTiersFromLiteLLM(p)
			if strings.TrimSpace(p.LiteLLMProvider) != "" {
				provider = p.LiteLLMProvider
			}
			source = "official"
			officialMissing = false
		}
	}
	if officialMissing && pricingHasValues(agg.pricing) {
		official = priceValueFromChannel(agg.pricing)
		priceTiers = basePriceTier(official)
		source = "channel"
		officialMissing = false
	}

	multiplier := modelPriceMultiplier(group, agg.billingMode, official)
	priceTiers = actualizePriceTiers(priceTiers, multiplier, usdCNYRate)

	return modelPriceModelDTO{
		Name:            agg.name,
		Platform:        agg.platform,
		Provider:        provider,
		BillingMode:     agg.billingMode,
		PricingSource:   source,
		Official:        official,
		Actual:          actualPriceValue(official, multiplier, usdCNYRate),
		PriceTiers:      priceTiers,
		Multiplier:      multiplier,
		CheaperFactor:   cheaperFactor(multiplier),
		ChannelNames:    sortedSetKeys(agg.channelNames),
		OfficialMissing: officialMissing,
	}
}

func applyModelPriceCustomPrice(model modelPriceModelDTO, custom service.ModelPriceCustomPrice, usdCNYRate float64) modelPriceModelDTO {
	actual := model.actualFromCustom(custom, usdCNYRate)
	model.Actual = actual
	model.PriceTiers = []modelPriceTierDTO{}
	model.PricingSource = "custom"
	if custom.BillingMode != "" {
		model.BillingMode = normalizeModelPriceBillingMode(custom.BillingMode)
	}
	model.Multiplier = 1
	model.CheaperFactor = cheaperFactorFromActual(model.Official, actual)
	model.CustomPrice = &custom
	return model
}

func (m modelPriceModelDTO) actualFromCustom(custom service.ModelPriceCustomPrice, usdCNYRate float64) modelPriceActualDTO {
	return modelPriceActualDTO{
		InputUSDPerM:       custom.InputUSDPerM,
		InputCNYPerM:       usdToCNYPtr(custom.InputUSDPerM, usdCNYRate),
		OutputUSDPerM:      custom.OutputUSDPerM,
		OutputCNYPerM:      usdToCNYPtr(custom.OutputUSDPerM, usdCNYRate),
		CacheWriteUSDPerM:  custom.CacheWriteUSDPerM,
		CacheWriteCNYPerM:  usdToCNYPtr(custom.CacheWriteUSDPerM, usdCNYRate),
		CacheReadUSDPerM:   custom.CacheReadUSDPerM,
		CacheReadCNYPerM:   usdToCNYPtr(custom.CacheReadUSDPerM, usdCNYRate),
		ImageOutputUSDPerM: custom.ImageOutputUSDPerM,
		ImageOutputCNYPerM: usdToCNYPtr(custom.ImageOutputUSDPerM, usdCNYRate),
		PerRequestUSD:      custom.PerRequestUSD,
		PerRequestCNY:      usdToCNYPtr(custom.PerRequestUSD, usdCNYRate),
	}
}

func usdToCNYPtr(value *float64, usdCNYRate float64) *float64 {
	if value == nil || usdCNYRate <= 0 {
		return nil
	}
	v := *value * usdCNYRate
	return &v
}

func cheaperFactorFromActual(official modelPriceValueDTO, actual modelPriceActualDTO) *float64 {
	candidates := []struct {
		official *float64
		actual   *float64
	}{
		{official.InputUSDPerM, actual.InputUSDPerM},
		{official.OutputUSDPerM, actual.OutputUSDPerM},
		{official.CacheWriteUSDPerM, actual.CacheWriteUSDPerM},
		{official.CacheReadUSDPerM, actual.CacheReadUSDPerM},
		{official.ImageOutputUSDPerM, actual.ImageOutputUSDPerM},
		{official.PerRequestUSD, actual.PerRequestUSD},
	}
	best := 0.0
	for _, candidate := range candidates {
		if candidate.official == nil || candidate.actual == nil || *candidate.official <= 0 || *candidate.actual <= 0 {
			continue
		}
		factor := *candidate.official / *candidate.actual
		if factor > best {
			best = factor
		}
	}
	if best <= 0 {
		return nil
	}
	return &best
}

func normalizeModelPriceBillingMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case string(service.BillingModeImage):
		return string(service.BillingModeImage)
	case string(service.BillingModePerRequest), "request":
		return string(service.BillingModePerRequest)
	default:
		return string(service.BillingModeToken)
	}
}

func modelPriceMultiplier(group modelPriceGroupDTO, billingMode string, official modelPriceValueDTO) float64 {
	base := group.EffectiveMultiplier
	if (billingMode == string(service.BillingModeImage) || official.ImageOutputUSDPerM != nil) && group.ImageRateIndependent {
		base = group.ImageRateMultiplier
	}
	base = normalizeMultiplier(base)
	if group.BestPlan != nil && group.BestPlan.USDMultiplier > 0 {
		return normalizeMultiplier(base * group.BestPlan.USDMultiplier)
	}
	return base
}

func channelHasGroup(ch service.AvailableChannel, groupID int64) bool {
	for _, g := range ch.Groups {
		if g.ID == groupID {
			return true
		}
	}
	return false
}

func modelPriceAccountUsableForGroup(account service.Account, groupPlatform string) bool {
	if account.Status != service.StatusActive {
		return false
	}
	accountPlatform := strings.ToLower(strings.TrimSpace(account.Platform))
	groupPlatform = strings.ToLower(strings.TrimSpace(groupPlatform))
	if accountPlatform == groupPlatform {
		return true
	}
	return accountPlatform == service.PlatformAntigravity &&
		(groupPlatform == service.PlatformAnthropic || groupPlatform == service.PlatformGemini)
}

func accountModelPriceNames(account service.Account, groupPlatform string) []string {
	mapping := (&account).GetModelMapping()
	if len(mapping) == 0 {
		return nil
	}
	seen := make(map[string]string, len(mapping))
	for raw := range mapping {
		name := strings.TrimSpace(raw)
		if name == "" || strings.Contains(name, "*") || !catalogModelMatchesGroupPlatform(name, groupPlatform) {
			continue
		}
		lower := strings.ToLower(name)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = name
	}
	out := make([]string, 0, len(seen))
	for _, name := range seen {
		out = append(out, name)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func priceValueFromLiteLLM(p *service.LiteLLMModelPricing) modelPriceValueDTO {
	return modelPriceValueDTO{
		InputUSDPerM:       perMillionPtr(p.InputCostPerToken),
		OutputUSDPerM:      perMillionPtr(p.OutputCostPerToken),
		CacheWriteUSDPerM:  perMillionPtr(p.CacheCreationInputTokenCost),
		CacheReadUSDPerM:   perMillionPtr(p.CacheReadInputTokenCost),
		ImageOutputUSDPerM: perMillionPtr(p.OutputCostPerImageToken),
		PerRequestUSD:      nonZeroFloatPtr(p.OutputCostPerImage),
	}
}

func priceTiersFromLiteLLM(p *service.LiteLLMModelPricing) []modelPriceTierDTO {
	if p == nil {
		return nil
	}
	tiers := basePriceTier(priceValueFromLiteLLM(p))

	seenContextTiers := map[string]struct{}{}
	for _, contextTier := range p.ContextPriceTiers {
		if contextTier.Priority {
			continue
		}
		key := contextTierKey(contextTier.ThresholdTokens, contextTier.Priority)
		seenContextTiers[key] = struct{}{}
		tiers = appendTier(tiers, key, "长上下文", contextTier.ThresholdTokens, priceValueFromContextTier(contextTier))
	}
	if _, ok := seenContextTiers[contextTierKey(272000, false)]; !ok {
		tiers = appendTier(tiers, "long_context_272k", "长上下文", 272000, longContext272KValue(p))
	}

	if hasPriorityPricing(p) {
		tiers = appendTier(tiers, "priority", "Fast", 0, modelPriceValueDTO{
			InputUSDPerM:      perMillionPtr(p.InputCostPerTokenPriority),
			OutputUSDPerM:     perMillionPtr(p.OutputCostPerTokenPriority),
			CacheWriteUSDPerM: perMillionPtr(p.CacheCreationInputTokenCost),
			CacheReadUSDPerM:  perMillionPtr(p.CacheReadInputTokenCostPriority),
		})
	}
	for _, contextTier := range p.ContextPriceTiers {
		if !contextTier.Priority {
			continue
		}
		key := contextTierKey(contextTier.ThresholdTokens, contextTier.Priority)
		seenContextTiers[key] = struct{}{}
		tiers = appendTier(tiers, key, "Fast 长上下文", contextTier.ThresholdTokens, priceValueFromContextTier(contextTier))
	}

	return tiers
}

func contextTierKey(threshold int, priority bool) string {
	if priority {
		return "priority_long_context_" + strconv.Itoa(threshold)
	}
	return "long_context_" + strconv.Itoa(threshold)
}

func priceValueFromContextTier(tier service.LiteLLMContextPriceTier) modelPriceValueDTO {
	return modelPriceValueDTO{
		InputUSDPerM:      perMillionPtr(tier.InputCostPerToken),
		OutputUSDPerM:     perMillionPtr(tier.OutputCostPerToken),
		CacheWriteUSDPerM: perMillionPtr(tier.CacheCreationInputTokenCost),
		CacheReadUSDPerM:  perMillionPtr(tier.CacheReadInputTokenCost),
	}
}

func basePriceTier(v modelPriceValueDTO) []modelPriceTierDTO {
	if !priceValueHasValues(v) {
		return nil
	}
	return []modelPriceTierDTO{{
		Key:      "base",
		Label:    "基础",
		Official: v,
	}}
}

func appendTier(tiers []modelPriceTierDTO, key, label string, threshold int, value modelPriceValueDTO) []modelPriceTierDTO {
	if !priceValueHasValues(value) {
		return tiers
	}
	tier := modelPriceTierDTO{
		Key:      key,
		Label:    label,
		Official: value,
	}
	if threshold > 0 {
		tier.ThresholdTokens = intPtr(threshold)
	}
	return append(tiers, tier)
}

func longContext272KValue(p *service.LiteLLMModelPricing) modelPriceValueDTO {
	value := modelPriceValueDTO{
		InputUSDPerM:     perMillionPtr(p.InputCostPerTokenAbove272K),
		OutputUSDPerM:    perMillionPtr(p.OutputCostPerTokenAbove272K),
		CacheReadUSDPerM: perMillionPtr(p.CacheReadInputTokenCostAbove272K),
	}
	if p.LongContextInputTokenThreshold == 272000 {
		if value.InputUSDPerM == nil && p.InputCostPerToken > 0 && p.LongContextInputCostMultiplier > 0 {
			value.InputUSDPerM = perMillionPtr(p.InputCostPerToken * p.LongContextInputCostMultiplier)
		}
		if value.OutputUSDPerM == nil && p.OutputCostPerToken > 0 && p.LongContextOutputCostMultiplier > 0 {
			value.OutputUSDPerM = perMillionPtr(p.OutputCostPerToken * p.LongContextOutputCostMultiplier)
		}
		if value.CacheReadUSDPerM == nil && p.CacheReadInputTokenCost > 0 && p.LongContextCacheReadCostMultiplier > 0 {
			value.CacheReadUSDPerM = perMillionPtr(p.CacheReadInputTokenCost * p.LongContextCacheReadCostMultiplier)
		}
	}
	return value
}

func hasPriorityPricing(p *service.LiteLLMModelPricing) bool {
	return p != nil && (p.InputCostPerTokenPriority > 0 || p.OutputCostPerTokenPriority > 0 || p.CacheReadInputTokenCostPriority > 0)
}

func actualizePriceTiers(tiers []modelPriceTierDTO, multiplier, usdCNYRate float64) []modelPriceTierDTO {
	out := make([]modelPriceTierDTO, 0, len(tiers))
	for _, tier := range tiers {
		tier.Actual = actualPriceValue(tier.Official, multiplier, usdCNYRate)
		out = append(out, tier)
	}
	return out
}

func priceValueHasValues(v modelPriceValueDTO) bool {
	return v.InputUSDPerM != nil || v.OutputUSDPerM != nil || v.CacheWriteUSDPerM != nil ||
		v.CacheReadUSDPerM != nil || v.ImageOutputUSDPerM != nil || v.PerRequestUSD != nil
}

func priceValueFromChannel(p *service.ChannelModelPricing) modelPriceValueDTO {
	if p == nil {
		return modelPriceValueDTO{}
	}
	return modelPriceValueDTO{
		InputUSDPerM:       perMillionFromPtr(p.InputPrice),
		OutputUSDPerM:      perMillionFromPtr(p.OutputPrice),
		CacheWriteUSDPerM:  perMillionFromPtr(p.CacheWritePrice),
		CacheReadUSDPerM:   perMillionFromPtr(p.CacheReadPrice),
		ImageOutputUSDPerM: perMillionFromPtr(p.ImageOutputPrice),
		PerRequestUSD:      clonePositivePtr(p.PerRequestPrice),
	}
}

func actualPriceValue(v modelPriceValueDTO, multiplier, usdCNYRate float64) modelPriceActualDTO {
	return modelPriceActualDTO{
		InputUSDPerM:       multiplyPtr(v.InputUSDPerM, multiplier),
		InputCNYPerM:       multiplyPtr(v.InputUSDPerM, multiplier*usdCNYRate),
		OutputUSDPerM:      multiplyPtr(v.OutputUSDPerM, multiplier),
		OutputCNYPerM:      multiplyPtr(v.OutputUSDPerM, multiplier*usdCNYRate),
		CacheWriteUSDPerM:  multiplyPtr(v.CacheWriteUSDPerM, multiplier),
		CacheWriteCNYPerM:  multiplyPtr(v.CacheWriteUSDPerM, multiplier*usdCNYRate),
		CacheReadUSDPerM:   multiplyPtr(v.CacheReadUSDPerM, multiplier),
		CacheReadCNYPerM:   multiplyPtr(v.CacheReadUSDPerM, multiplier*usdCNYRate),
		ImageOutputUSDPerM: multiplyPtr(v.ImageOutputUSDPerM, multiplier),
		ImageOutputCNYPerM: multiplyPtr(v.ImageOutputUSDPerM, multiplier*usdCNYRate),
		PerRequestUSD:      multiplyPtr(v.PerRequestUSD, multiplier),
		PerRequestCNY:      multiplyPtr(v.PerRequestUSD, multiplier*usdCNYRate),
	}
}

func summarizeModelPrices(models []modelPriceModelDTO) modelPriceSummaryDTO {
	sum := 0.0
	count := 0
	priced := 0
	for _, m := range models {
		if !m.OfficialMissing {
			priced++
		}
		if m.CheaperFactor != nil && *m.CheaperFactor > 0 {
			sum += *m.CheaperFactor
			count++
		}
	}
	var avg *float64
	if count > 0 {
		v := roundPrice(sum / float64(count))
		avg = &v
	}
	return modelPriceSummaryDTO{
		ModelCount:           len(models),
		PricedCount:          priced,
		AverageCheaperFactor: avg,
	}
}

func pricingHasValues(p *service.ChannelModelPricing) bool {
	if p == nil {
		return false
	}
	if p.InputPrice != nil || p.OutputPrice != nil || p.CacheWritePrice != nil ||
		p.CacheReadPrice != nil || p.ImageOutputPrice != nil || p.PerRequestPrice != nil {
		return true
	}
	for _, iv := range p.Intervals {
		if iv.InputPrice != nil || iv.OutputPrice != nil || iv.CacheWritePrice != nil ||
			iv.CacheReadPrice != nil || iv.PerRequestPrice != nil {
			return true
		}
	}
	return false
}

func normalizeMultiplier(v float64) float64 {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 1
	}
	return v
}

func cheaperFactor(multiplier float64) *float64 {
	multiplier = normalizeMultiplier(multiplier)
	if multiplier <= 0 {
		return nil
	}
	v := roundPrice(1 / multiplier)
	return &v
}

func perMillionPtr(v float64) *float64 {
	if v <= 0 {
		return nil
	}
	out := roundPrice(v * 1_000_000)
	return &out
}

func perMillionFromPtr(v *float64) *float64 {
	if v == nil || *v <= 0 {
		return nil
	}
	out := roundPrice(*v * 1_000_000)
	return &out
}

func nonZeroFloatPtr(v float64) *float64 {
	if v <= 0 {
		return nil
	}
	out := roundPrice(v)
	return &out
}

func clonePositivePtr(v *float64) *float64 {
	if v == nil || *v <= 0 {
		return nil
	}
	out := roundPrice(*v)
	return &out
}

func intPtr(v int) *int {
	return &v
}

func multiplyPtr(v *float64, multiplier float64) *float64 {
	if v == nil {
		return nil
	}
	out := roundPrice(*v * multiplier)
	return &out
}

func roundPrice(v float64) float64 {
	return math.Round(v*1_000_000) / 1_000_000
}

func sortedSetKeys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for v := range values {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func sortedInt64SetKeys(values map[int64]struct{}) []int64 {
	out := make([]int64, 0, len(values))
	for v := range values {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sortedModelPriceHandlerStringSetKeys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for v := range values {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func parseModelPriceBoolQuery(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
