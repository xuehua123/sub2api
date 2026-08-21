package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// AvailableGroupRef 渠道视图中关联分组的简要信息。
//
// 用户侧「可用渠道」页面据此展示：专属分组 vs 公开分组（IsExclusive）、
// 订阅 vs 标准（SubscriptionType）、默认倍率（RateMultiplier）与高峰倍率规则。
// 用户专属倍率不在这里暴露，前端自己通过 /groups/rates 拉取，和 API 密钥页面保持一致。
type AvailableGroupRef struct {
	ID                 int64
	Name               string
	Platform           string
	SubscriptionType   string
	RateMultiplier     float64
	SortOrder          int
	PeakRateEnabled    bool
	PeakStart          string
	PeakEnd            string
	PeakRateMultiplier float64
	IsExclusive        bool
}

// AvailableChannel 可用渠道视图：用于「可用渠道」页面展示渠道基础信息 +
// 关联的分组 + 推导出的支持模型列表（无通配符）。
type AvailableChannel struct {
	ID                 int64
	Name               string
	Description        string
	Status             string
	BillingModelSource string
	RestrictModels     bool
	Groups             []AvailableGroupRef
	SupportedModels    []SupportedModel
}

// ListAvailable 返回所有渠道的可用视图：每个渠道附带关联分组信息与支持模型列表。
//
// 支持模型通过 (*Channel).SupportedModels() 计算（mapping ∪ pricing 并联）。
// 对于渠道未配置定价的模型，进一步用 PricingService 的全局 LiteLLM 数据合成
// 一份展示用定价，让用户看到默认价格而非"未配置"。
//
// 关联分组信息通过 groupRepo.ListActive 查询后按 ID 映射；渠道 GroupIDs 中未在活跃列表中
// 的分组（已停用或删除）会被忽略。
//
// 前置条件：s.groupRepo 必须非 nil（由 wire DI 保证）。直接 nil-deref 用于 fail-fast，
// 避免静默掩盖注入缺失。
func (s *ChannelService) ListAvailable(ctx context.Context) ([]AvailableChannel, error) {
	channels, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}

	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}
	groupByID := make(map[int64]AvailableGroupRef, len(groups))
	groupEntityByID := make(map[int64]*Group, len(groups))
	for i := range groups {
		g := groups[i]
		groupEntityByID[g.ID] = &groups[i]
		groupByID[g.ID] = AvailableGroupRef{
			ID:                 g.ID,
			Name:               g.Name,
			Platform:           g.Platform,
			SubscriptionType:   g.SubscriptionType,
			RateMultiplier:     g.RateMultiplier,
			SortOrder:          g.SortOrder,
			PeakRateEnabled:    g.PeakRateEnabled,
			PeakStart:          g.PeakStart,
			PeakEnd:            g.PeakEnd,
			PeakRateMultiplier: g.PeakRateMultiplier,
			IsExclusive:        g.IsExclusive,
		}
	}

	out := make([]AvailableChannel, 0, len(channels))
	for i := range channels {
		ch := &channels[i]
		groups := make([]AvailableGroupRef, 0, len(ch.GroupIDs))
		for _, gid := range ch.GroupIDs {
			if ref, ok := groupByID[gid]; ok {
				groups = append(groups, ref)
			}
		}
		sort.SliceStable(groups, func(i, j int) bool {
			leftRate := availableGroupDisplayRate(groups[i])
			rightRate := availableGroupDisplayRate(groups[j])
			if leftRate != rightRate {
				return leftRate < rightRate
			}
			if groups[i].SortOrder != groups[j].SortOrder {
				return groups[i].SortOrder < groups[j].SortOrder
			}
			leftName := strings.ToLower(groups[i].Name)
			rightName := strings.ToLower(groups[j].Name)
			if leftName != rightName {
				return leftName < rightName
			}
			return groups[i].ID < groups[j].ID
		})

		ch.normalizeBillingModelSource()

		supported := ch.SupportedModels()
		rawPricing := make([]*ChannelModelPricing, len(supported))
		for modelIdx := range supported {
			rawPricing[modelIdx] = cloneChannelModelPricingForDisplay(supported[modelIdx].Pricing)
			if rawPricing[modelIdx] != nil {
				supported[modelIdx].PricingSource = PricingSourceChannel
			}
		}
		s.fillGlobalPricingFallback(supported)
		for modelIdx := range supported {
			billingModel := supported[modelIdx].BillingModel
			if billingModel == "" {
				billingModel = supported[modelIdx].Name
			}
			pricingByGroup := make(map[int64]*ChannelModelPricing, len(groups))
			pricingSourceByGroup := make(map[int64]string, len(groups))
			for _, groupRef := range groups {
				group := groupEntityByID[groupRef.ID]
				pricing, matched := s.PricingForGroupDisplay(group, billingModel, rawPricing[modelIdx])
				if pricing == nil {
					pricing = cloneChannelModelPricingForDisplay(supported[modelIdx].Pricing)
				}
				pricingByGroup[groupRef.ID] = pricing
				if matched {
					pricingSourceByGroup[groupRef.ID] = PricingSourceGroup
				} else {
					pricingSourceByGroup[groupRef.ID] = supported[modelIdx].PricingSource
				}
			}
			supported[modelIdx].PricingByGroup = pricingByGroup
			supported[modelIdx].PricingSourceByGroup = pricingSourceByGroup
		}

		out = append(out, AvailableChannel{
			ID:                 ch.ID,
			Name:               ch.Name,
			Description:        ch.Description,
			Status:             ch.Status,
			BillingModelSource: ch.BillingModelSource,
			RestrictModels:     ch.RestrictModels,
			Groups:             groups,
			SupportedModels:    supported,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// GroupModelPricingForDisplay resolves a group override using this service's
// current global pricing catalog.
func (s *ChannelService) GroupModelPricingForDisplay(group *Group, model string) (*ChannelModelPricing, bool) {
	var official *LiteLLMModelPricing
	if s != nil && s.pricingService != nil {
		official = s.pricingService.GetModelPricing(model)
	}
	configured := MatchGroupModelPricing(group, model)
	var effective *ModelPricing
	if configured != nil && s != nil && s.billingService != nil {
		effective, _ = s.billingService.GetModelPricingWithChannel(model, configured)
		effective = s.billingService.applyModelSpecificPricingPolicy(model, effective)
	}
	pricing, matched := resolveGroupModelPricingForDisplay(group, model, official, effective)
	markOpenAIAccountLongContextGate(pricing, displayRoutePlatform(group, configured))
	return pricing, matched
}

// PricingForGroupDisplay projects the exact effective pricing for one group.
// rawChannelPricing must be the unmodified channel card: passing an already
// projected card would incorrectly turn catalog fallback fields into explicit
// channel overrides.
func (s *ChannelService) PricingForGroupDisplay(group *Group, model string, rawChannelPricing *ChannelModelPricing) (*ChannelModelPricing, bool) {
	groupPricing := MatchGroupModelPricing(group, model)
	groupMatched := groupPricing != nil
	selected := rawChannelPricing
	if groupMatched {
		selected = groupPricing
		if selected.BillingMode == "" || selected.BillingMode == BillingModeToken {
			stripped := selected.Clone()
			stripped.Intervals = nil
			selected = &stripped
		}
	}
	mode := BillingModeToken
	if selected != nil && selected.BillingMode != "" {
		mode = selected.BillingMode
	}
	if mode != BillingModeToken {
		return cloneChannelModelPricingForDisplay(selected), groupMatched
	}
	if s == nil || s.billingService == nil {
		if groupMatched {
			pricing, _ := s.GroupModelPricingForDisplay(group, model)
			return pricing, true
		}
		return cloneChannelModelPricingForDisplay(rawChannelPricing), false
	}

	longEnabled := group == nil || group.LongContextPricingEnabled
	var official *LiteLLMModelPricing
	if s.pricingService != nil {
		official = s.pricingService.GetModelPricing(model)
	}
	if !groupMatched && rawChannelPricing != nil {
		valid := filterValidIntervals(rawChannelPricing.Intervals)
		if len(valid) > 0 {
			return s.projectChannelIntervalsForGroup(model, rawChannelPricing, valid, official, longEnabled), false
		}
	}

	runtimePricing, err := s.billingService.GetModelPricingWithChannel(model, selected)
	if err != nil || runtimePricing == nil {
		if selected == nil {
			return nil, groupMatched
		}
		runtimePricing = overlayTokenPricingForDisplay(&ModelPricing{}, selected)
	}
	runtimePricing = s.billingService.applyModelSpecificPricingPolicy(model, runtimePricing)
	projected := effectiveTokenPricingForDisplay(runtimePricing, selected != nil)
	if selected != nil {
		projected.Platform = selected.Platform
		projected.Models = append([]string(nil), selected.Models...)
	}
	projected.Intervals = groupTokenIntervalsForDisplay(model, projected, official, runtimePricing, longEnabled)
	markOpenAIAccountLongContextGate(projected, displayRoutePlatform(group, selected))
	return projected, groupMatched
}

func overlayTokenPricingForDisplay(base *ModelPricing, configured *ChannelModelPricing) *ModelPricing {
	pricing := &ModelPricing{}
	if base != nil {
		*pricing = *base
	}
	if configured.InputPrice != nil {
		pricing.InputPricePerToken = *configured.InputPrice
		pricing.InputPricePerTokenPriority = *configured.InputPrice
	}
	if configured.OutputPrice != nil {
		pricing.OutputPricePerToken = *configured.OutputPrice
		pricing.OutputPricePerTokenPriority = *configured.OutputPrice
	}
	if configured.CacheWritePrice != nil {
		pricing.CacheCreationPricePerToken = *configured.CacheWritePrice
		pricing.CacheCreationPricePerTokenPriority = *configured.CacheWritePrice
		pricing.CacheCreationPriceExplicit = true
		pricing.CacheCreation5mPrice = *configured.CacheWritePrice
		pricing.CacheCreation1hPrice = *configured.CacheWritePrice
	}
	if configured.CacheReadPrice != nil {
		pricing.CacheReadPricePerToken = *configured.CacheReadPrice
		pricing.CacheReadPricePerTokenPriority = *configured.CacheReadPrice
	}
	if configured.ImageOutputPrice != nil {
		pricing.ImageOutputPricePerToken = *configured.ImageOutputPrice
	}
	pricing.ImageOutputPriceExplicit = true
	applyChannelImageInputPrice(configured, pricing)
	return pricing
}

func displayRoutePlatform(group *Group, pricing *ChannelModelPricing) string {
	if pricing != nil && strings.TrimSpace(pricing.Platform) != "" {
		return pricing.Platform
	}
	if group != nil {
		return group.Platform
	}
	return ""
}

func markOpenAIAccountLongContextGate(pricing *ChannelModelPricing, platform string) {
	if pricing == nil {
		return
	}
	openAIRoute := strings.EqualFold(strings.TrimSpace(platform), PlatformOpenAI)
	for i := range pricing.Intervals {
		pricing.Intervals[i].RequiresAccountLongContext = openAIRoute &&
			strings.Contains(pricing.Intervals[i].TierLabel, "长上下文")
	}
}

func (s *ChannelService) projectChannelIntervalsForGroup(model string, raw *ChannelModelPricing, valid []PricingInterval, official *LiteLLMModelPricing, longEnabled bool) *ChannelModelPricing {
	baseRuntime, err := s.billingService.GetModelPricing(model)
	if err != nil || baseRuntime == nil {
		baseRuntime = &ModelPricing{}
	}
	baseRuntime = s.billingService.applyModelSpecificPricingPolicy(model, baseRuntime)
	fallbackRuntime := *baseRuntime
	applyChannelTokenPriceOverrides(&fallbackRuntime, raw)
	fallbackRuntime.FastMultiplier = raw.FastMultiplier
	fallbackRuntime.FlexMultiplier = raw.FlexMultiplier
	if raw.ImageOutputPrice != nil {
		fallbackRuntime.ImageOutputPricePerToken = *raw.ImageOutputPrice
	}
	fallbackRuntime.ImageOutputPriceExplicit = true
	applyChannelImageInputPrice(raw, &fallbackRuntime)
	if !longEnabled {
		first := valid[0]
		for _, interval := range valid[1:] {
			if interval.MinTokens < first.MinTokens {
				first = interval
			}
		}
		runtimePricing := intervalToModelPricingForModel(model, &first, &fallbackRuntime, raw)
		runtimePricing = s.billingService.applyModelSpecificPricingPolicy(model, runtimePricing)
		projected := effectiveTokenPricingForDisplay(runtimePricing, true)
		projected.Platform = raw.Platform
		projected.Models = append([]string(nil), raw.Models...)
		projected.Intervals = groupTokenIntervalsForDisplay(model, projected, official, runtimePricing, false)
		return projected
	}

	projected := effectiveTokenPricingForDisplay(&fallbackRuntime, true)
	projected.Platform = raw.Platform
	projected.Models = append([]string(nil), raw.Models...)
	tiers := make([]PricingInterval, 0, len(valid)*2)
	hasFast := groupDisplaySupportsFast(model, official)
	for i := range valid {
		iv := valid[i]
		runtimePricing := intervalToModelPricingForModel(model, &iv, &fallbackRuntime, raw)
		runtimePricing = s.billingService.applyModelSpecificPricingPolicy(model, runtimePricing)
		card := effectiveTokenPricingForDisplay(runtimePricing, true)
		tier := PricingInterval{
			MinTokens:         iv.MinTokens,
			MaxTokens:         iv.MaxTokens,
			TierLabel:         iv.TierLabel,
			InputPrice:        card.InputPrice,
			OutputPrice:       card.OutputPrice,
			CacheWritePrice:   card.CacheWritePrice,
			CacheWrite5mPrice: card.CacheWrite5mPrice,
			CacheWrite1hPrice: card.CacheWrite1hPrice,
			CacheReadPrice:    card.CacheReadPrice,
			SortOrder:         i * 2,
		}
		tiers = append(tiers, tier)
		if hasFast {
			fast := fastDisplayTier(model, tier, runtimePricing, fastIntervalLabel(tier.TierLabel), i*2+1)
			fast.MinTokens = tier.MinTokens
			fast.MaxTokens = tier.MaxTokens
			tiers = append(tiers, fast)
		}
	}
	projected.Intervals = tiers
	return projected
}

func fastIntervalLabel(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return "Fast"
	}
	return "Fast " + label
}

// ResolveGroupModelPricingForDisplay resolves a group override with the same matcher
// used by billing. Token overrides are merged over the global catalog because
// runtime group pricing bypasses channel pricing and only replaces explicitly
// configured fields. Pointer-valued zeroes are deliberately preserved. When
// the group enables long-context pricing, the returned intervals mirror the
// official multiplier policy applied at runtime to the group-overridden base.
func ResolveGroupModelPricingForDisplay(
	group *Group,
	model string,
	official *LiteLLMModelPricing,
) (*ChannelModelPricing, bool) {
	return resolveGroupModelPricingForDisplay(group, model, official, nil)
}

func resolveGroupModelPricingForDisplay(
	group *Group,
	model string,
	official *LiteLLMModelPricing,
	runtimePricing *ModelPricing,
) (*ChannelModelPricing, bool) {
	configured := MatchGroupModelPricing(group, model)
	if configured == nil {
		return nil, false
	}
	mode := configured.BillingMode
	if mode == "" {
		mode = BillingModeToken
	}
	if mode != BillingModeToken {
		configured.BillingMode = mode
		return configured, true
	}

	var effective *ChannelModelPricing
	if runtimePricing != nil {
		effective = effectiveTokenPricingForDisplay(runtimePricing, configured != nil)
	} else {
		if official != nil {
			effective = synthesizePricingFromLiteLLM(official, nil)
		}
		if effective == nil {
			effective = &ChannelModelPricing{BillingMode: BillingModeToken}
		} else {
			effective = cloneChannelModelPricingForDisplay(effective)
		}
		overlayConfiguredPrices(effective, configured)
		if configured.ImageInputPrice == nil || *configured.ImageInputPrice <= 0 {
			effective.ImageInputPrice = cloneDisplayPrice(effective.InputPrice)
		}
		// Runtime applyTokenOverrides treats a missing group image-output token price
		// as an explicit zero; do not leak the catalog image price into display.
		if configured.ImageOutputPrice == nil {
			zero := 0.0
			effective.ImageOutputPrice = &zero
		}
		runtimePricing = groupRuntimeTokenPricingForDisplay(effective, configured, official)
		runtimePricing = (&BillingService{}).applyModelSpecificPricingPolicy(model, runtimePricing)
	}
	effective.BillingMode = BillingModeToken
	effective.Platform = configured.Platform
	effective.Models = append([]string(nil), configured.Models...)
	effective.Intervals = groupTokenIntervalsForDisplay(
		model,
		effective,
		official,
		runtimePricing,
		group.LongContextPricingEnabled,
	)
	return effective, true
}

// effectiveTokenPricingForDisplay projects the effective runtime token card
// into the existing display DTO. It intentionally does not mutate or persist
// ChannelModelPricing. A zero image-input rate follows runtime semantics and
// falls back to the effective text-input rate.
func effectiveTokenPricingForDisplay(pricing *ModelPricing, channelSemantics bool) *ChannelModelPricing {
	if pricing == nil {
		return nil
	}
	result := &ChannelModelPricing{
		BillingMode:      BillingModeToken,
		InputPrice:       floatPointerForDisplay(pricing.InputPricePerToken),
		OutputPrice:      floatPointerForDisplay(pricing.OutputPricePerToken),
		CacheWritePrice:  floatPointerForDisplay(pricing.CacheCreationPricePerToken),
		CacheReadPrice:   floatPointerForDisplay(pricing.CacheReadPricePerToken),
		ImageInputPrice:  floatPointerForDisplay(pricing.ImageInputPricePerToken),
		ImageOutputPrice: nonZeroPtr(pricing.ImageOutputPricePerToken),
	}
	if pricing.ImageInputPricePerToken <= 0 {
		result.ImageInputPrice = floatPointerForDisplay(pricing.InputPricePerToken)
	}
	if channelSemantics {
		// Channel/group token cards make image-output zero explicit at runtime.
		result.ImageOutputPrice = floatPointerForDisplay(pricing.ImageOutputPricePerToken)
	}
	if pricing.SupportsCacheBreakdown && (pricing.CacheCreation5mPrice > 0 || pricing.CacheCreation1hPrice > 0) {
		result.CacheWrite5mPrice = floatPointerForDisplay(pricing.CacheCreation5mPrice)
		result.CacheWrite1hPrice = floatPointerForDisplay(pricing.CacheCreation1hPrice)
	}
	return result
}

// TokenPricingForDisplay resolves one token card through BillingService and
// returns a display-only projection. The optional configured card is treated
// with the same overlay semantics as runtime channel/group billing.
func (s *ChannelService) TokenPricingForDisplay(model string, configured *ChannelModelPricing) (*ChannelModelPricing, bool) {
	if s == nil || s.billingService == nil {
		return nil, false
	}
	var pricing *ModelPricing
	var err error
	if configured == nil {
		pricing, err = s.billingService.GetModelPricing(model)
	} else {
		pricing, err = s.billingService.GetModelPricingWithChannel(model, configured)
	}
	if err != nil || pricing == nil {
		return nil, false
	}
	pricing = s.billingService.applyModelSpecificPricingPolicy(model, pricing)
	projected := effectiveTokenPricingForDisplay(pricing, configured != nil)
	if configured != nil {
		projected.Platform = configured.Platform
		projected.Models = append([]string(nil), configured.Models...)
		projected.Intervals = append([]PricingInterval(nil), configured.Intervals...)
	}
	return projected, true
}

// groupTokenIntervalsForDisplay builds the same base/long/Fast/Fast-long rate
// matrix used by runtime billing. It intentionally returns display intervals
// only when there is more than one rate tier; a plain base price remains in the
// flat fields for backwards-compatible clients.
func groupTokenIntervalsForDisplay(
	model string,
	base *ChannelModelPricing,
	pricing *LiteLLMModelPricing,
	runtimePricing *ModelPricing,
	longContextEnabled bool,
) []PricingInterval {
	if base == nil {
		return nil
	}
	if runtimePricing == nil {
		return nil
	}

	hasLong := longContextEnabled && runtimePricing.LongContextInputThreshold > 0 &&
		(runtimePricing.LongContextInputMultiplier > 1 || runtimePricing.LongContextOutputMultiplier > 1)
	hasFast := groupDisplaySupportsFast(model, pricing)
	if !hasLong && !hasFast {
		return nil
	}

	baseTier := PricingInterval{
		TierLabel:         "基础",
		InputPrice:        cloneDisplayPrice(base.InputPrice),
		OutputPrice:       cloneDisplayPrice(base.OutputPrice),
		CacheWritePrice:   cloneDisplayPrice(base.CacheWritePrice),
		CacheWrite5mPrice: cloneDisplayPrice(base.CacheWrite5mPrice),
		CacheWrite1hPrice: cloneDisplayPrice(base.CacheWrite1hPrice),
		CacheReadPrice:    cloneDisplayPrice(base.CacheReadPrice),
		SortOrder:         0,
	}
	displayThreshold := runtimePricing.LongContextInputThreshold
	if hasLong && runtimePricing.LongContextThresholdInclusive && displayThreshold > 0 {
		// PricingInterval uses (min,max] integer ranges, while xAI applies its
		// long-context rate at >= threshold. Shift the display boundary by one
		// token so 200K is advertised in the same tier runtime bills.
		displayThreshold--
	}
	if hasLong {
		baseTier.MaxTokens = intPointerForDisplay(displayThreshold)
	}
	tiers := []PricingInterval{baseTier}

	var longTier PricingInterval
	if hasLong {
		longTier = longContextDisplayTier(baseTier, runtimePricing, displayThreshold, "长上下文", 1)
		tiers = append(tiers, longTier)
	}
	if !hasFast {
		return tiers
	}

	fastTier := fastDisplayTier(model, baseTier, runtimePricing, "Fast", 2)
	tiers = append(tiers, fastTier)
	if hasLong {
		var fastLong PricingInterval
		if isOpenAIGPT54Model(model) && !isOpenAIGPT55Model(model) {
			// GPT-5.4 priority requests above the threshold use the standard
			// long-context rates, not the priority floor rates.
			fastLong = longTier
			fastLong.TierLabel = "Fast 长上下文"
			fastLong.SortOrder = 3
		} else {
			fastLong = longContextDisplayTier(fastTier, runtimePricing, displayThreshold, "Fast 长上下文", 3)
		}
		tiers = append(tiers, fastLong)
	}
	return tiers
}

func groupDisplaySupportsFast(model string, pricing *LiteLLMModelPricing) bool {
	normalized := normalizeKnownOpenAICodexModel(model)
	if isOpenAIGPT56Model(normalized) || isOpenAIGPT54Model(normalized) {
		return true
	}
	if pricing == nil {
		return false
	}
	if pricing.SupportsServiceTier || pricing.InputCostPerTokenPriority > 0 ||
		pricing.OutputCostPerTokenPriority > 0 || pricing.CacheCreationInputTokenCostPriority > 0 ||
		pricing.CacheReadInputTokenCostPriority > 0 {
		return true
	}
	for _, tier := range pricing.ContextPriceTiers {
		if tier.Priority {
			return true
		}
	}
	return false
}

func groupRuntimeTokenPricingForDisplay(base, configured *ChannelModelPricing, pricing *LiteLLMModelPricing) *ModelPricing {
	runtimePricing := &ModelPricing{
		InputPricePerToken:         displayPriceValue(base.InputPrice),
		OutputPricePerToken:        displayPriceValue(base.OutputPrice),
		CacheCreationPricePerToken: displayPriceValue(base.CacheWritePrice),
		CacheReadPricePerToken:     displayPriceValue(base.CacheReadPrice),
	}
	if pricing != nil {
		runtimePricing.InputPricePerTokenPriority = pricing.InputCostPerTokenPriority
		runtimePricing.OutputPricePerTokenPriority = pricing.OutputCostPerTokenPriority
		runtimePricing.CacheCreationPricePerTokenPriority = pricing.CacheCreationInputTokenCostPriority
		runtimePricing.CacheReadPricePerTokenPriority = pricing.CacheReadInputTokenCostPriority
		runtimePricing.LongContextInputThreshold = pricing.LongContextInputTokenThreshold
		runtimePricing.LongContextInputMultiplier = pricing.LongContextInputCostMultiplier
		runtimePricing.LongContextOutputMultiplier = pricing.LongContextOutputCostMultiplier
		runtimePricing.LongContextCacheReadMultiplier = pricing.LongContextCacheReadCostMultiplier
	}
	// Runtime group overrides replace both standard and priority fields for each
	// explicitly configured text/cache rate.
	if configured != nil {
		if configured.InputPrice != nil {
			runtimePricing.InputPricePerTokenPriority = *configured.InputPrice
		}
		if configured.OutputPrice != nil {
			runtimePricing.OutputPricePerTokenPriority = *configured.OutputPrice
		}
		if configured.CacheWritePrice != nil {
			runtimePricing.CacheCreationPricePerTokenPriority = *configured.CacheWritePrice
			runtimePricing.CacheCreationPriceExplicit = true
		}
		if configured.CacheReadPrice != nil {
			runtimePricing.CacheReadPricePerTokenPriority = *configured.CacheReadPrice
		}
	}
	return runtimePricing
}

func longContextDisplayTier(base PricingInterval, pricing *ModelPricing, displayThreshold int, label string, sortOrder int) PricingInterval {
	cacheReadMultiplier := pricing.LongContextCacheReadMultiplier
	if cacheReadMultiplier <= 0 {
		cacheReadMultiplier = pricing.LongContextInputMultiplier
	}
	return PricingInterval{
		MinTokens:         displayThreshold,
		TierLabel:         label,
		InputPrice:        multiplyDisplayPrice(base.InputPrice, pricing.LongContextInputMultiplier),
		OutputPrice:       multiplyDisplayPrice(base.OutputPrice, pricing.LongContextOutputMultiplier),
		CacheWritePrice:   multiplyDisplayPrice(base.CacheWritePrice, pricing.LongContextInputMultiplier),
		CacheWrite5mPrice: multiplyDisplayPrice(base.CacheWrite5mPrice, pricing.LongContextInputMultiplier),
		CacheWrite1hPrice: multiplyDisplayPrice(base.CacheWrite1hPrice, pricing.LongContextInputMultiplier),
		CacheReadPrice:    multiplyDisplayPrice(base.CacheReadPrice, cacheReadMultiplier),
		SortOrder:         sortOrder,
	}
}

func fastDisplayTier(model string, base PricingInterval, pricing *ModelPricing, label string, sortOrder int) PricingInterval {
	tier := PricingInterval{TierLabel: label, SortOrder: sortOrder}
	if isOpenAIGPT55Model(model) {
		tier.InputPrice = multiplyDisplayPrice(base.InputPrice, openAIGPT55PriorityMultiplier)
		tier.OutputPrice = multiplyDisplayPrice(base.OutputPrice, openAIGPT55PriorityMultiplier)
		tier.CacheWritePrice = multiplyDisplayPrice(base.CacheWritePrice, openAIGPT55PriorityMultiplier)
		tier.CacheWrite5mPrice = multiplyDisplayPrice(base.CacheWrite5mPrice, openAIGPT55PriorityMultiplier)
		tier.CacheWrite1hPrice = multiplyDisplayPrice(base.CacheWrite1hPrice, openAIGPT55PriorityMultiplier)
		tier.CacheReadPrice = multiplyDisplayPrice(base.CacheReadPrice, openAIGPT55PriorityMultiplier)
		return tier
	}
	if usePriorityServiceTierPricing("priority", pricing) {
		tier.InputPrice = priorityOrStandardDisplayPrice(pricing.InputPricePerTokenPriority, base.InputPrice)
		tier.OutputPrice = priorityOrStandardDisplayPrice(pricing.OutputPricePerTokenPriority, base.OutputPrice)
		tier.CacheWritePrice = priorityOrStandardDisplayPrice(pricing.CacheCreationPricePerTokenPriority, base.CacheWritePrice)
		if pricing.SupportsCacheBreakdown {
			tier.CacheWrite5mPrice = cloneDisplayPrice(base.CacheWrite5mPrice)
			tier.CacheWrite1hPrice = cloneDisplayPrice(base.CacheWrite1hPrice)
		}
		tier.CacheReadPrice = priorityOrStandardDisplayPrice(pricing.CacheReadPricePerTokenPriority, base.CacheReadPrice)
		return tier
	}
	multiplier := serviceTierCostMultiplier("priority")
	tier.InputPrice = multiplyDisplayPrice(base.InputPrice, multiplier)
	tier.OutputPrice = multiplyDisplayPrice(base.OutputPrice, multiplier)
	tier.CacheWritePrice = multiplyDisplayPrice(base.CacheWritePrice, multiplier)
	tier.CacheWrite5mPrice = multiplyDisplayPrice(base.CacheWrite5mPrice, multiplier)
	tier.CacheWrite1hPrice = multiplyDisplayPrice(base.CacheWrite1hPrice, multiplier)
	tier.CacheReadPrice = multiplyDisplayPrice(base.CacheReadPrice, multiplier)
	return tier
}

func priorityOrStandardDisplayPrice(priority float64, standard *float64) *float64 {
	if priority > 0 {
		return cloneDisplayPrice(&priority)
	}
	return cloneDisplayPrice(standard)
}

func displayPriceValue(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func intPointerForDisplay(value int) *int {
	return &value
}

func floatPointerForDisplay(value float64) *float64 {
	return &value
}

func cloneDisplayPrice(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func multiplyDisplayPrice(value *float64, multiplier float64) *float64 {
	if value == nil {
		return nil
	}
	multiplied := *value * multiplier
	return &multiplied
}

func overlayConfiguredPrices(dst, configured *ChannelModelPricing) {
	if dst == nil || configured == nil {
		return
	}
	if configured.InputPrice != nil {
		dst.InputPrice = configured.InputPrice
	}
	if configured.OutputPrice != nil {
		dst.OutputPrice = configured.OutputPrice
	}
	if configured.CacheWritePrice != nil {
		dst.CacheWritePrice = configured.CacheWritePrice
	}
	if configured.CacheReadPrice != nil {
		dst.CacheReadPrice = configured.CacheReadPrice
	}
	if configured.ImageInputPrice != nil {
		dst.ImageInputPrice = configured.ImageInputPrice
	}
	if configured.ImageOutputPrice != nil {
		dst.ImageOutputPrice = configured.ImageOutputPrice
	}
	if configured.PerRequestPrice != nil {
		dst.PerRequestPrice = configured.PerRequestPrice
	}
}

func cloneChannelModelPricingForDisplay(pricing *ChannelModelPricing) *ChannelModelPricing {
	if pricing == nil {
		return nil
	}
	clone := pricing.Clone()
	return &clone
}

// fillGlobalPricingFallback 对未命中渠道定价的支持模型，从全局 LiteLLM 数据合成一份
// 展示用定价。仅用于「可用渠道」展示，不影响真实计费链路。
//
// 触发条件：
//  1. Pricing == nil（渠道完全没声明该模型的定价条目）
//  2. Pricing 非 nil 但所有价格字段为空（admin UI 建了条目但没填价格）
//
// 当 s.pricingService 为 nil（测试场景），跳过回落。
func (s *ChannelService) fillGlobalPricingFallback(models []SupportedModel) {
	if s.pricingService == nil && s.billingService == nil {
		return
	}
	for i := range models {
		existing := models[i].Pricing
		if existing != nil {
			models[i].PricingSource = PricingSourceChannel
		}
		mode := BillingModeToken
		if existing != nil && existing.BillingMode != "" {
			mode = existing.BillingMode
		}
		// Token billing resolves from the official card and overlays every
		// explicitly configured channel field. Enrich partial token cards too;
		// otherwise a lone image-input override would hide text/cache prices.
		if mode != BillingModeToken && !pricingNeedsFallback(existing) {
			continue
		}
		billingModel := strings.TrimSpace(models[i].BillingModel)
		if billingModel == "" {
			billingModel = models[i].Name
		}
		if mode == BillingModeToken && s.billingService != nil {
			if projected, ok := s.TokenPricingForDisplay(billingModel, existing); ok {
				models[i].Pricing = projected
				if existing == nil {
					models[i].PricingSource = s.catalogOrFallbackPricingSource(billingModel)
				}
				continue
			}
		}
		if s.pricingService == nil {
			continue
		}
		lp := s.pricingService.GetModelPricing(billingModel)
		if lp == nil {
			continue
		}
		models[i].Pricing = synthesizePricingFromLiteLLM(lp, existing)
		if existing == nil {
			models[i].PricingSource = PricingSourceOfficial
		}
	}
}

func (s *ChannelService) catalogOrFallbackPricingSource(model string) string {
	if s != nil && s.pricingService != nil {
		if pricing := s.pricingService.GetIdentifiedModelPricing(model); pricing != nil && !pricing.TokenPricingAbsent {
			return PricingSourceOfficial
		}
	}
	return PricingSourceFallback
}

// pricingNeedsFallback 判定一个 ChannelModelPricing 是否需要走全局回落。
// 价格全部缺失（无 flat 字段且无任何带价 interval）即视为未配置。
func pricingNeedsFallback(p *ChannelModelPricing) bool {
	if p == nil {
		return true
	}
	if p.InputPrice != nil || p.OutputPrice != nil ||
		p.CacheWritePrice != nil || p.CacheReadPrice != nil ||
		p.ImageInputPrice != nil || p.ImageOutputPrice != nil || p.PerRequestPrice != nil {
		return false
	}
	for _, iv := range p.Intervals {
		if iv.InputPrice != nil || iv.OutputPrice != nil ||
			iv.CacheWritePrice != nil || iv.CacheReadPrice != nil ||
			iv.PerRequestPrice != nil {
			return false
		}
	}
	return true
}

// synthesizePricingFromLiteLLM 把 LiteLLM 的定价数据转成 ChannelModelPricing 形态，
// 仅用于展示。
//
// 计费模式优先级：
//  1. 渠道已选 BillingMode（admin 在 UI 里选了 image / per_request 但没填价的场景，
//     按选定模式合成对应字段）
//  2. LiteLLM mode="image_generation" → image
//  3. 默认 token
//
// LiteLLM 中字段 0 视为未配置，不带入展示。
func synthesizePricingFromLiteLLM(lp *LiteLLMModelPricing, existing *ChannelModelPricing) *ChannelModelPricing {
	if lp == nil {
		return existing
	}

	mode := BillingModeToken
	switch {
	case existing != nil && existing.BillingMode != "":
		mode = existing.BillingMode
	case lp.Mode == "image_generation":
		mode = BillingModeImage
	}

	var synthesized *ChannelModelPricing
	if mode == BillingModeImage || mode == BillingModePerRequest || mode == BillingModeVideo {
		synthesized = &ChannelModelPricing{
			BillingMode:      mode,
			PerRequestPrice:  nonZeroPtr(lp.OutputCostPerImage),
			ImageInputPrice:  nonZeroPtr(lp.InputCostPerImageToken),
			ImageOutputPrice: nonZeroPtr(lp.OutputCostPerImageToken),
			InputPrice:       nonZeroPtr(lp.InputCostPerToken),
			OutputPrice:      nonZeroPtr(lp.OutputCostPerToken),
		}
	} else {
		synthesized = &ChannelModelPricing{
			BillingMode:      mode,
			InputPrice:       nonZeroPtr(lp.InputCostPerToken),
			OutputPrice:      nonZeroPtr(lp.OutputCostPerToken),
			CacheWritePrice:  nonZeroPtr(lp.CacheCreationInputTokenCost),
			CacheReadPrice:   nonZeroPtr(lp.CacheReadInputTokenCost),
			ImageInputPrice:  nonZeroPtr(lp.InputCostPerImageToken),
			ImageOutputPrice: nonZeroPtr(lp.OutputCostPerImageToken),
		}
	}
	if existing == nil {
		return synthesized
	}

	synthesized.Platform = existing.Platform
	synthesized.Models = append([]string(nil), existing.Models...)
	synthesized.Intervals = append([]PricingInterval(nil), existing.Intervals...)
	overlayConfiguredPrices(synthesized, existing)
	if mode == BillingModeToken {
		if existing.ImageInputPrice == nil {
			synthesized.ImageInputPrice = cloneDisplayPrice(synthesized.InputPrice)
		}
		if existing.ImageOutputPrice == nil {
			zero := 0.0
			synthesized.ImageOutputPrice = &zero
		}
	}
	return synthesized
}

func availableGroupDisplayRate(g AvailableGroupRef) float64 {
	if g.RateMultiplier > 0 {
		return g.RateMultiplier
	}
	return 1
}

func nonZeroPtr(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}
