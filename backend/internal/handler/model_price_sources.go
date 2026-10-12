package handler

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func modelPricePlatformMatches(groupPlatform, platform string) bool {
	if groupPlatform == service.PlatformComposite {
		return domain.IsConcretePlatform(platform)
	}
	return groupPlatform == platform
}

func modelPriceAccountPlatform(account service.Account, groupPlatform string) string {
	if groupPlatform == service.PlatformComposite {
		return account.Platform
	}
	return groupPlatform
}

func modelPriceAggregateKey(groupPlatform, platform, name string) string {
	if groupPlatform == service.PlatformComposite {
		// Composite routing identifies a public ID before its provider is chosen.
		// Conflicting provider claims must share a row and fail closed.
		platform = service.PlatformComposite
	}
	return strings.ToLower(platform + "\x00" + strings.TrimSpace(name))
}

func seedModelPriceConfiguredModels(aggregates map[string]*modelAggregate, group modelPriceGroupDTO, internalTargets map[string]struct{}) {
	for _, pricing := range group.modelPricing {
		for _, raw := range pricing.Models {
			name := strings.TrimSpace(raw)
			if name == "" || strings.Contains(name, "*") {
				continue
			}
			if _, internal := internalTargets[strings.ToLower(name)]; internal {
				// Pricing may refer to an internal upstream alias. Existing public
				// rows already use its price; do not publish it as a new model.
				continue
			}
			platform := group.Platform
			if platform == service.PlatformComposite {
				platform = pricing.Platform
				if !domain.IsConcretePlatform(platform) {
					if detected, ok := service.DetectModelPlatform(name); ok {
						platform = detected
					} else {
						platform = service.PlatformComposite
					}
				}
			}
			key := modelPriceAggregateKey(group.Platform, platform, name)
			if _, exists := aggregates[key]; exists {
				continue
			}
			aggregates[key] = &modelAggregate{
				name: name, platform: platform, billingModel: name,
				billingMode:  string(normalizeDisplayBillingMode(pricing.BillingMode)),
				channelNames: map[string]struct{}{}, configuredOnly: true,
			}
		}
	}
}

func modelPriceAvailability(agg *modelAggregate) string {
	switch {
	case agg.billingModelAmbiguous:
		return "unknown"
	case agg.channelBacked:
		return "channel"
	case agg.accountOnly:
		return "account"
	case agg.configuredOnly:
		return "configured_only"
	case agg.catalogOnly:
		return "catalog_only"
	default:
		return ""
	}
}

func (h *ModelPriceHandler) loadModelPriceCompositeRoutes(ctx context.Context, groups []modelPriceGroupDTO, channels []service.AvailableChannel, accounts map[int64][]service.Account) error {
	if h.compositeResolver == nil {
		return nil
	}
	for i := range groups {
		if groups[i].Platform != service.PlatformComposite {
			continue
		}
		names := []string{}
		for _, agg := range collectModelPriceAggregates(channels, accounts[groups[i].ID], groups[i]) {
			names = append(names, agg.name)
		}
		decisions, err := h.compositeResolver.ConfiguredModelsForDisplay(ctx, groups[i].ID, names)
		if err != nil {
			return err
		}
		groups[i].routeDecisions = decisions
	}
	return nil
}

func applyModelPriceCompositeRoutes(aggregates map[string]*modelAggregate, group modelPriceGroupDTO, accounts []service.Account) {
	seen := map[string]service.CompositeRouteDecision{}
	for _, decision := range group.routeDecisions {
		key := modelPriceAggregateKey(group.Platform, decision.TargetPlatform, decision.PublicModel)
		agg, exists := aggregates[key]
		if !exists {
			agg = &modelAggregate{name: decision.PublicModel, platform: decision.TargetPlatform, billingModel: decision.UpstreamModel, billingMode: string(service.BillingModeToken), channelNames: map[string]struct{}{}, configuredOnly: true}
			aggregates[key] = agg
		}
		if previous, duplicate := seen[key]; duplicate {
			if previous.TargetPlatform != decision.TargetPlatform || !strings.EqualFold(previous.UpstreamModel, decision.UpstreamModel) {
				agg.billingModelAmbiguous = true
			}
			continue
		}
		seen[key] = decision
		changedTarget := !strings.EqualFold(agg.billingModel, decision.UpstreamModel) || agg.platform != decision.TargetPlatform
		if (agg.channelBacked && agg.billingModelAmbiguous) || ((agg.channelBacked || agg.accountOnly) && changedTarget) {
			// A channel projection belongs to its original target. A changed route
			// cannot safely reuse those already projected prices.
			agg.billingModelAmbiguous = true
			continue
		}
		verified := false
		for i := range accounts {
			account := accounts[i]
			if !modelPriceAccountUsableForGroup(account, decision.TargetPlatform) || account.Platform != decision.TargetPlatform || !account.IsModelSupported(decision.UpstreamModel) {
				continue
			}
			verified = true
			if !strings.EqualFold(account.GetMappedModel(decision.UpstreamModel), decision.UpstreamModel) {
				// A route is only the first mapping stage. Without a proven final
				// account projection, its upstream ID is not a billing price.
				verified = false
				break
			}
		}
		if !verified {
			agg.billingModelAmbiguous = true
			continue
		}
		agg.platform = decision.TargetPlatform
		agg.billingModel = decision.UpstreamModel
		agg.billingModelAmbiguous = false
	}
}

func modelPriceCatalogPlatform(groupPlatform, provider, name string) string {
	if groupPlatform != service.PlatformComposite {
		return groupPlatform
	}
	for _, spec := range domain.Platforms() {
		if spec.LiteLLMProvider == provider && spec.ID != service.PlatformAntigravity {
			return spec.ID
		}
	}
	if platform, ok := service.DetectModelPlatform(name); ok {
		return platform
	}
	return service.PlatformComposite
}

func modelPriceRegistryCatalogProviders(platform string) []string {
	if platform != service.PlatformComposite {
		if spec, ok := domain.LookupPlatform(platform); ok && spec.LiteLLMProvider != "" {
			return []string{spec.LiteLLMProvider}
		}
		return nil
	}
	providers := []string{}
	seen := map[string]struct{}{}
	for _, platformID := range domain.CompositePrecedencePlatformIDs() {
		for _, provider := range catalogProvidersForPlatform(platformID) {
			if _, exists := seen[provider]; !exists {
				seen[provider] = struct{}{}
				providers = append(providers, provider)
			}
		}
	}
	return providers
}

// Only shared, exact alias rules may identify an authoritative catalog card.
// Family-price guesses remain usable billing fallbacks, never catalog prices.
func modelPriceIdentifiedCatalogName(catalog modelPriceCatalog, platform, model string) string {
	if catalog.GetIdentifiedModelPricing(model) != nil {
		return model
	}
	bare := strings.ToLower(xai.StripGrokProviderPrefix(model))
	if bare == "grok" || bare == "grok-latest" {
		// Bare defaults vary by endpoint and runtime mapping. They do not
		// identify one authoritative catalog model without an actual card.
		return model
	}
	if platform == service.PlatformGrok && xai.IsGrokTextResponsesModelID(model) {
		canonical := xai.ResolveGrokTextResponsesModelID(model)
		if catalog.GetIdentifiedModelPricing(canonical) != nil {
			return canonical
		}
	}
	return model
}

// Group media cards use the same explicit configuration precedence as billing.
// Missing tiers stay absent; this display path never invents a global fallback.
func modelPriceConfiguredMedia(model modelPriceModelDTO, agg *modelAggregate, group modelPriceGroupDTO, usdCNYRate float64) modelPriceModelDTO {
	if agg.platform != service.PlatformGrok {
		return model
	}
	name := strings.ToLower(xai.StripGrokProviderPrefix(agg.billingModel))
	video := strings.HasPrefix(name, "grok-imagine-video") || strings.HasPrefix(name, "grok-video")
	image := xai.IsGrokImagineModel(name) && !video
	if !video && !image {
		return model
	}
	cardMode := service.BillingMode(agg.billingMode)
	if !agg.billingModelAmbiguous && cardMode == service.BillingModeToken && pricingHasValues(agg.pricing) &&
		(agg.pricingSource == service.PricingSourceGroup || agg.pricingSource == service.PricingSourceChannel) {
		// Explicit token cards take the gateway's token-billing branch even for
		// media responses. Only guessed text fallback prices are discarded.
		return model
	}
	mode := string(service.BillingModeImage)
	defaultTier := service.NormalizeImageBillingTierOrDefault("")
	if video {
		mode = string(service.BillingModeVideo)
		defaultTier = service.NormalizeVideoBillingResolutionOrDefault("")
	}
	// Media cannot reuse a guessed text-token price. Keep the media kind even
	// when no explicit per-image/per-second billing price is known.
	original := model
	model.BillingMode = mode
	model.Official = modelPriceValueDTO{}
	model.Actual = modelPriceActualDTO{}
	model.PriceTiers = []modelPriceTierDTO{}
	model.OfficialMissing = true
	model.PricingSource = "unknown"
	model.Multiplier = modelPriceMultiplier(group, mode, model.Official)
	model.CheaperFactor = cheaperFactor(model.Multiplier)
	if agg.billingModelAmbiguous {
		return model
	}
	groupPriority := agg.pricingSource == service.PricingSourceGroup &&
		((image && (cardMode == service.BillingModeImage || cardMode == service.BillingModePerRequest)) ||
			(video && cardMode == service.BillingModeVideo))
	if groupPriority {
		return modelPriceMediaProjection(original, model, mode, defaultTier, agg.pricing, usdCNYRate)
	}
	tiers := []modelPriceTierDTO{}
	groupMediaTier := false
	g := group.mediaGroup
	if g == nil {
		g = &service.Group{}
	}
	appendTier := func(label string, price *float64) {
		if value := catalogScaled(price, 1); value != nil {
			tiers = append(tiers, modelPriceTierDTO{Key: strings.ToLower(label), Label: label, Official: modelPriceValueDTO{PerRequestUSD: value}})
		}
	}
	if video {
		for _, tier := range []struct {
			label string
			price *float64
		}{{"480p", g.VideoPrice480P}, {"720p", g.VideoPrice720P}, {"1080p", g.VideoPrice1080P}} {
			price := service.LookupVideoModelPrice(g.VideoModelPrices, name, tier.label)
			if price == nil {
				price = tier.price
			}
			groupMediaTier = groupMediaTier || catalogScaled(price, 1) != nil
			if price == nil && agg.pricingSource == service.PricingSourceChannel && cardMode == service.BillingModeVideo {
				price = modelPriceMediaTierPrice(agg.pricing, tier.label)
			}
			appendTier(tier.label, price)
		}
	} else {
		for _, tier := range []struct {
			label string
			price *float64
		}{{"1K", g.ImagePrice1K}, {"2K", g.ImagePrice2K}, {"4K", g.ImagePrice4K}} {
			price := tier.price
			groupMediaTier = groupMediaTier || catalogScaled(price, 1) != nil
			if price == nil && agg.pricingSource == service.PricingSourceChannel && (cardMode == service.BillingModeImage || cardMode == service.BillingModePerRequest) {
				price = modelPriceMediaTierPrice(agg.pricing, tier.label)
			}
			appendTier(tier.label, price)
		}
	}
	if len(tiers) == 0 {
		if agg.pricingSource == service.PricingSourceChannel && (cardMode == service.BillingModeImage || cardMode == service.BillingModePerRequest || cardMode == service.BillingModeVideo) {
			if video && cardMode != service.BillingModeVideo {
				// Runtime channel per-request/image video cards bill per request,
				// not per second. The unit must retain that distinction.
				mode = string(service.BillingModePerRequest)
			}
			return modelPriceMediaProjection(original, model, mode, defaultTier, agg.pricing, usdCNYRate)
		}
		return model
	}
	model.BillingMode = mode
	model.PricingSource = service.PricingSourceGroup
	if !groupMediaTier {
		model.PricingSource = service.PricingSourceChannel
	}
	model.Official, tiers = modelPriceMediaDefaultTier(tiers, defaultTier)
	model.OfficialMissing = model.Official.PerRequestUSD == nil
	model.Multiplier = modelPriceMultiplier(group, mode, model.Official)
	model.PriceTiers = actualizePriceTiers(tiers, model.Multiplier, usdCNYRate)
	model.Actual = actualPriceValue(model.Official, model.Multiplier, usdCNYRate)
	model.CheaperFactor = cheaperFactor(model.Multiplier)
	return model
}

func modelPriceMediaProjection(original, model modelPriceModelDTO, mode, defaultTier string, pricing *service.ChannelModelPricing, usdCNYRate float64) modelPriceModelDTO {
	model.BillingMode = mode
	model.PricingSource = original.PricingSource
	tiers := []modelPriceTierDTO{}
	labels := []string{"1K", "2K", "4K"}
	if strings.EqualFold(defaultTier, service.NormalizeVideoBillingResolutionOrDefault("")) {
		labels = []string{"480p", "720p", "1080p"}
	}
	for _, label := range labels {
		if price := catalogScaled(modelPriceMediaTierPrice(pricing, label), 1); price != nil {
			tiers = append(tiers, modelPriceTierDTO{Key: strings.ToLower(label), Label: label, Official: modelPriceValueDTO{PerRequestUSD: price}})
		}
	}
	model.Official, tiers = modelPriceMediaDefaultTier(tiers, defaultTier)
	model.OfficialMissing = model.Official.PerRequestUSD == nil
	model.PriceTiers = actualizePriceTiers(tiers, model.Multiplier, usdCNYRate)
	model.Actual = actualPriceValue(model.Official, model.Multiplier, usdCNYRate)
	return model
}

func modelPriceMediaDefaultTier(tiers []modelPriceTierDTO, defaultTier string) (modelPriceValueDTO, []modelPriceTierDTO) {
	base := modelPriceTierDTO{Key: "base", Label: defaultTier + "（默认）"}
	other := make([]modelPriceTierDTO, 0, len(tiers))
	for _, tier := range tiers {
		if strings.EqualFold(tier.Label, defaultTier) {
			base.Official = tier.Official
		} else {
			other = append(other, tier)
		}
	}
	return base.Official, append([]modelPriceTierDTO{base}, other...)
}

func modelPriceMediaTierPrice(pricing *service.ChannelModelPricing, label string) *float64 {
	if pricing == nil {
		return nil
	}
	for _, interval := range pricing.Intervals {
		if strings.EqualFold(strings.TrimSpace(interval.TierLabel), label) && interval.PerRequestPrice != nil {
			return interval.PerRequestPrice
		}
	}
	if interval := service.FindMatchingInterval(pricing.Intervals, 0); interval != nil && interval.PerRequestPrice != nil {
		return interval.PerRequestPrice
	}
	return pricing.PerRequestPrice
}
