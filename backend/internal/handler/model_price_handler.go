package handler

import (
	"context"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type ModelPriceHandler struct {
	channelService       *service.ChannelService
	apiKeyService        *service.APIKeyService
	groupService         *service.GroupService
	pricingService       *service.PricingService
	settingService       *service.SettingService
	paymentConfigService *service.PaymentConfigService
}

func NewModelPriceHandler(
	channelService *service.ChannelService,
	apiKeyService *service.APIKeyService,
	groupService *service.GroupService,
	pricingService *service.PricingService,
	settingService *service.SettingService,
	paymentConfigService *service.PaymentConfigService,
) *ModelPriceHandler {
	return &ModelPriceHandler{
		channelService:       channelService,
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
	Name            string              `json:"name"`
	Platform        string              `json:"platform"`
	Provider        string              `json:"provider"`
	BillingMode     string              `json:"billing_mode"`
	PricingSource   string              `json:"pricing_source"`
	Official        modelPriceValueDTO  `json:"official"`
	Actual          modelPriceActualDTO `json:"actual"`
	PriceTiers      []modelPriceTierDTO `json:"price_tiers"`
	Multiplier      float64             `json:"multiplier"`
	CheaperFactor   *float64            `json:"cheaper_factor"`
	ChannelNames    []string            `json:"channel_names"`
	OfficialMissing bool                `json:"official_missing"`
}

type modelPriceSummaryDTO struct {
	ModelCount           int      `json:"model_count"`
	PricedCount          int      `json:"priced_count"`
	AverageCheaperFactor *float64 `json:"average_cheaper_factor"`
}

type modelPriceResponseDTO struct {
	USDCNYRate      float64              `json:"usd_cny_rate"`
	Groups          []modelPriceGroupDTO `json:"groups"`
	SelectedGroupID *int64               `json:"selected_group_id"`
	Models          []modelPriceModelDTO `json:"models"`
	Summary         modelPriceSummaryDTO `json:"summary"`
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
	groups, err := h.visibleGroups(c, subject.UserID, isAdmin)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	groupDTOs := h.groupDTOs(c, groups, subject.UserID, isAdmin)
	usdCNYRate := h.settingService.GetModelPriceUSDCNYRate(c.Request.Context())
	h.applyPlanEconomics(c.Request.Context(), groupDTOs, usdCNYRate)
	groupDTOs = filterCurrentSaleModelPriceGroups(groupDTOs)
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
		channels, err := h.channelService.ListAvailable(c.Request.Context())
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		models = h.modelsForGroup(channels, *selected, usdCNYRate)
	}

	response.Success(c, modelPriceResponseDTO{
		USDCNYRate:      usdCNYRate,
		Groups:          groupDTOs,
		SelectedGroupID: selectedGroupID,
		Models:          models,
		Summary:         summarizeModelPrices(models),
	})
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

func filterCurrentSaleModelPriceGroups(groups []modelPriceGroupDTO) []modelPriceGroupDTO {
	hasSaleBackedGroup := false
	for i := range groups {
		if groups[i].BestPlan != nil {
			hasSaleBackedGroup = true
			break
		}
	}
	if !hasSaleBackedGroup {
		return groups
	}
	out := make([]modelPriceGroupDTO, 0, len(groups))
	for i := range groups {
		if groups[i].BestPlan == nil {
			continue
		}
		out = append(out, groups[i])
	}
	return out
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
	id := groups[0].ID
	return &id
}

type modelAggregate struct {
	name         string
	platform     string
	billingMode  string
	pricing      *service.ChannelModelPricing
	channelNames map[string]struct{}
}

func (h *ModelPriceHandler) modelsForGroup(channels []service.AvailableChannel, group modelPriceGroupDTO, usdCNYRate float64) []modelPriceModelDTO {
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

	models := make([]modelPriceModelDTO, 0, len(aggregates))
	for _, agg := range aggregates {
		models = append(models, h.toModelPriceDTO(agg, group, usdCNYRate))
	}
	sort.SliceStable(models, func(i, j int) bool {
		return strings.ToLower(models[i].Name) < strings.ToLower(models[j].Name)
	})
	return models
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

	multiplier := group.EffectiveMultiplier
	if group.BestPlan != nil && group.BestPlan.USDMultiplier > 0 {
		multiplier = group.BestPlan.USDMultiplier
	}
	if (agg.billingMode == string(service.BillingModeImage) || official.ImageOutputUSDPerM != nil) &&
		group.ImageRateIndependent && group.BestPlan == nil {
		multiplier = group.ImageRateMultiplier
	}
	multiplier = normalizeMultiplier(multiplier)
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

func channelHasGroup(ch service.AvailableChannel, groupID int64) bool {
	for _, g := range ch.Groups {
		if g.ID == groupID {
			return true
		}
	}
	return false
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
