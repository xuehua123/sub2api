package handler

import (
	"math"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (h *ModelPriceHandler) catalogPriceForModel(agg *modelAggregate) *modelPriceCatalogPriceDTO {
	if h.pricingService == nil || agg.billingModelAmbiguous {
		return nil
	}
	name := strings.TrimSpace(agg.billingModel)
	if name == "" {
		name = agg.name
	}
	return catalogPriceFromPricing(name, h.pricingService.GetIdentifiedModelPricing(name))
}

func catalogPriceFromPricing(name string, p *service.LiteLLMModelPricing) *modelPriceCatalogPriceDTO {
	if p == nil {
		return nil
	}
	fields := p.CatalogPrices
	if fields == nil {
		// Built-in/test price cards predate presence metadata. Positive optional
		// fields are known; only their declared token pair permits explicit zeros.
		fields = &service.CatalogPricingFields{
			ImageInput: catalogPositive(p.InputCostPerImageToken), ImageOutput: catalogPositive(p.OutputCostPerImageToken),
			CacheWrite: catalogPositive(p.CacheCreationInputTokenCost), CacheWrite1h: catalogPositive(p.CacheCreationInputTokenCostAbove1hr),
			CacheRead: catalogPositive(p.CacheReadInputTokenCost), PerImage: catalogPositive(p.OutputCostPerImage),
		}
		if !p.TokenPricingAbsent {
			fields.Input = &p.InputCostPerToken
			fields.Output = &p.OutputCostPerToken
		}
	}
	value := modelPriceValueDTO{
		InputUSDPerM: catalogScaled(fields.Input, 1e6), OutputUSDPerM: catalogScaled(fields.Output, 1e6),
		ImageInputUSDPerM: catalogScaled(fields.ImageInput, 1e6), ImageOutputUSDPerM: catalogScaled(fields.ImageOutput, 1e6),
		CacheWriteUSDPerM: catalogScaled(fields.CacheWrite, 1e6), CacheWrite5mUSDPerM: catalogScaled(fields.CacheWrite, 1e6),
		CacheWrite1hUSDPerM: catalogScaled(fields.CacheWrite1h, 1e6), CacheReadUSDPerM: catalogScaled(fields.CacheRead, 1e6),
		PerRequestUSD: catalogScaled(fields.PerImage, 1),
	}
	if !priceValueHasValues(value) {
		return nil
	}
	mode := string(service.BillingModeToken)
	// Image-token prices are token prices, not per-image prices. Do not drop them
	// merely because the catalog has no text-token prices.
	if value.PerRequestUSD != nil && value.ImageInputUSDPerM == nil && value.ImageOutputUSDPerM == nil && p.InputCostPerToken == 0 && p.OutputCostPerToken == 0 {
		mode = string(service.BillingModeImage)
	}
	tiers := []modelPriceTierDTO{}
	if mode == string(service.BillingModeToken) {
		tiers = priceTiersFromLiteLLM(name, p, false, false)
	}
	// The source base price must not be replaced by a channel or policy fallback.
	// Keep only genuinely distinct context/service tiers after the authoritative base.
	resultTiers := basePriceTier(value)
	for _, tier := range tiers {
		if tier.ThresholdTokens != nil || strings.Contains(strings.ToLower(tier.Label), "fast") || strings.Contains(strings.ToLower(tier.Label), "priority") {
			resultTiers = append(resultTiers, tier)
		}
	}
	return &modelPriceCatalogPriceDTO{Model: name, BillingMode: mode, Price: value, Tiers: resultTiers}
}

func catalogScaled(value *float64, scale float64) *float64 {
	if value == nil || *value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return nil
	}
	out := *value * scale
	if math.IsInf(out, 0) {
		return nil
	}
	return &out
}

func catalogPositive(value float64) *float64 {
	if value <= 0 {
		return nil
	}
	return &value
}
