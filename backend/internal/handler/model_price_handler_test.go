//go:build unit

package handler

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/stretchr/testify/require"
)

type stubModelPriceCatalog struct {
	prices map[string]*service.LiteLLMModelPricing
	byProv map[string][]string
}

func (s stubModelPriceCatalog) GetModelPricing(model string) *service.LiteLLMModelPricing {
	return s.prices[model]
}

func (s stubModelPriceCatalog) ListModelNamesByProvider(provider string) []string {
	return append([]string(nil), s.byProv[provider]...)
}

func TestModelPriceCatalogStatusFromMapFormatsLastUpdated(t *testing.T) {
	status := modelPriceCatalogStatusFromMap(map[string]any{
		"model_count":  123,
		"last_updated": time.Date(2026, 6, 28, 12, 30, 0, 0, time.UTC),
		"local_hash":   "abcdef12",
	})

	require.Equal(t, 123, status.ModelCount)
	require.Equal(t, "2026-06-28T12:30:00Z", status.LastUpdated)
	require.Equal(t, "abcdef12", status.LocalHash)
}

func TestModelPriceDTOAppliesMultiplierAndExchangeRate(t *testing.T) {
	h := &ModelPriceHandler{}
	input := 3.0 / 1_000_000
	output := 15.0 / 1_000_000

	dto := h.toModelPriceDTO(&modelAggregate{
		name:        "claude-sonnet-test",
		platform:    "anthropic",
		billingMode: string(service.BillingModeToken),
		pricing: &service.ChannelModelPricing{
			BillingMode: service.BillingModeToken,
			InputPrice:  &input,
			OutputPrice: &output,
		},
		channelNames: map[string]struct{}{"primary": {}},
	}, modelPriceGroupDTO{
		ID:                  1,
		Name:                "高速",
		Platform:            "anthropic",
		EffectiveMultiplier: 0.2,
	}, 7)

	require.Equal(t, "channel", dto.PricingSource)
	require.False(t, dto.OfficialMissing)
	require.NotNil(t, dto.Official.InputUSDPerM)
	require.Equal(t, 3.0, *dto.Official.InputUSDPerM)
	require.NotNil(t, dto.Actual.InputUSDPerM)
	require.Equal(t, 0.6, *dto.Actual.InputUSDPerM)
	require.NotNil(t, dto.Actual.InputCNYPerM)
	require.Equal(t, 4.2, *dto.Actual.InputCNYPerM)
	require.NotNil(t, dto.CheaperFactor)
	require.Equal(t, 5.0, *dto.CheaperFactor)
	require.Equal(t, []string{"primary"}, dto.ChannelNames)
}

func TestModelsForGroupUsesChannelModelsByDefault(t *testing.T) {
	h := &ModelPriceHandler{
		pricingService: stubModelPriceCatalog{
			prices: map[string]*service.LiteLLMModelPricing{
				"claude-sonnet-4-5": {
					LiteLLMProvider:    "anthropic",
					InputCostPerToken:  3e-6,
					OutputCostPerToken: 15e-6,
				},
			},
			byProv: map[string][]string{
				"anthropic": {"claude-sonnet-4-5"},
			},
		},
	}

	models := h.modelsForGroup(nil, modelPriceGroupDTO{
		ID:                  1,
		Platform:            service.PlatformAnthropic,
		EffectiveMultiplier: 0.1,
	}, 7, false)

	require.Empty(t, models)
}

func TestModelsForGroupIncludesOfficialCatalogModelsWhenRequested(t *testing.T) {
	h := &ModelPriceHandler{
		pricingService: stubModelPriceCatalog{
			prices: map[string]*service.LiteLLMModelPricing{
				"claude-sonnet-4-5": {
					LiteLLMProvider:    "anthropic",
					InputCostPerToken:  3e-6,
					OutputCostPerToken: 15e-6,
				},
			},
			byProv: map[string][]string{
				"anthropic": {"claude-sonnet-4-5"},
			},
		},
	}

	models := h.modelsForGroup(nil, modelPriceGroupDTO{
		ID:                  1,
		Platform:            service.PlatformAnthropic,
		EffectiveMultiplier: 0.1,
	}, 7, true)

	require.Len(t, models, 1)
	require.Equal(t, "claude-sonnet-4-5", models[0].Name)
	require.Equal(t, "official", models[0].PricingSource)
	require.NotNil(t, models[0].Official.InputUSDPerM)
	require.Equal(t, 3.0, *models[0].Official.InputUSDPerM)
	require.NotNil(t, models[0].Actual.InputCNYPerM)
	require.Equal(t, 2.1, *models[0].Actual.InputCNYPerM)
}

func TestCatalogProvidersForPlatformIncludesRealGeminiProviders(t *testing.T) {
	providers := catalogProvidersForPlatform(service.PlatformGemini)

	require.Contains(t, providers, "google")
	require.Contains(t, providers, "gemini")
	require.Contains(t, providers, "vertex_ai-language-models")
	require.Contains(t, providers, "vertex_ai-embedding-models")
}

func TestSelectModelPriceGroupIDDoesNotDefaultWithoutExplicitGroup(t *testing.T) {
	groups := []modelPriceGroupDTO{
		{ID: 2, Name: "B"},
		{ID: 3, Name: "C"},
	}

	require.Equal(t, int64(3), *selectModelPriceGroupID("3", groups))
	require.Nil(t, selectModelPriceGroupID("999", groups))
	require.Nil(t, selectModelPriceGroupID("", nil))
	require.Nil(t, selectModelPriceGroupID("", groups))
}

func TestApplyModelPriceHiddenGroupsFiltersUnlessIncluded(t *testing.T) {
	groups := []modelPriceGroupDTO{
		{ID: 1, Name: "visible"},
		{ID: 2, Name: "hidden"},
	}
	hidden := map[int64]struct{}{2: {}}

	filtered := applyModelPriceHiddenGroups(groups, hidden, false)
	require.Len(t, filtered, 1)
	require.Equal(t, int64(1), filtered[0].ID)

	included := applyModelPriceHiddenGroups(groups, hidden, true)
	require.Len(t, included, 2)
	require.True(t, included[1].Hidden)
}

func TestApplyModelPriceGroupUsageCountsActiveChannelsAndModels(t *testing.T) {
	groups := []modelPriceGroupDTO{
		{ID: 1, Platform: service.PlatformAnthropic},
		{ID: 2, Platform: service.PlatformOpenAI},
	}
	channels := []service.AvailableChannel{
		{
			ID:     10,
			Name:   "primary",
			Status: service.StatusActive,
			Groups: []service.AvailableGroupRef{
				{ID: 1},
				{ID: 2},
			},
			SupportedModels: []service.SupportedModel{
				{Name: "claude-sonnet-4-5", Platform: service.PlatformAnthropic},
				{Name: "claude-sonnet-4-5", Platform: service.PlatformAnthropic},
				{Name: "gpt-5", Platform: service.PlatformOpenAI},
			},
		},
		{
			ID:     11,
			Name:   "disabled",
			Status: "disabled",
			Groups: []service.AvailableGroupRef{
				{ID: 1},
			},
			SupportedModels: []service.SupportedModel{
				{Name: "claude-opus-4-5", Platform: service.PlatformAnthropic},
			},
		},
	}

	applyModelPriceGroupUsage(groups, channels)

	require.Equal(t, 1, groups[0].ChannelCount)
	require.Equal(t, 1, groups[0].ModelCount)
	require.Equal(t, 1, groups[1].ChannelCount)
	require.Equal(t, 1, groups[1].ModelCount)
}

func TestBuildModelPriceGroupOverviewDeduplicatesAcrossGroups(t *testing.T) {
	groups := []modelPriceGroupDTO{
		{ID: 1, Platform: service.PlatformOpenAI},
		{ID: 2, Platform: service.PlatformOpenAI},
	}
	channels := []service.AvailableChannel{
		{
			ID:     10,
			Name:   "shared",
			Status: service.StatusActive,
			Groups: []service.AvailableGroupRef{
				{ID: 1},
				{ID: 2},
			},
			SupportedModels: []service.SupportedModel{
				{Name: "gpt-5", Platform: service.PlatformOpenAI},
			},
		},
	}

	overview := buildModelPriceGroupOverview(groups, channels)
	openai := overview[1]

	require.Equal(t, "openai", openai.Category)
	require.Equal(t, 2, openai.GroupCount)
	require.Equal(t, 1, openai.ModelCount)
	require.Equal(t, 1, openai.ChannelCount)
}

func TestPlanPackageQuotaUSDUsesFullPackageValidity(t *testing.T) {
	monthly := 100.0
	weekly := 70.0
	daily := 10.0

	require.InDelta(t, 1216.666666, planPackageQuotaUSD(service.SubscriptionPlanResponse{
		ValidityDays:    1,
		ValidityUnit:    "year",
		MonthlyLimitUSD: &monthly,
	}), 0.000001)
	require.Equal(t, 140.0, planPackageQuotaUSD(service.SubscriptionPlanResponse{
		ValidityDays:   2,
		ValidityUnit:   "week",
		WeeklyLimitUSD: &weekly,
	}))
	require.Equal(t, 300.0, planPackageQuotaUSD(service.SubscriptionPlanResponse{
		ValidityDays:  30,
		ValidityUnit:  "day",
		DailyLimitUSD: &daily,
	}))
}

func TestModelPriceDTOCombinesGroupAndPackageMultiplier(t *testing.T) {
	h := &ModelPriceHandler{}
	input := 10.0 / 1_000_000

	dto := h.toModelPriceDTO(&modelAggregate{
		name:        "gpt-test",
		platform:    "openai",
		billingMode: string(service.BillingModeToken),
		pricing: &service.ChannelModelPricing{
			BillingMode: service.BillingModeToken,
			InputPrice:  &input,
		},
		channelNames: map[string]struct{}{"primary": {}},
	}, modelPriceGroupDTO{
		ID:                  1,
		Name:                "套餐分组",
		Platform:            "openai",
		EffectiveMultiplier: 3.6,
		BestPlan: &modelPricePlanDTO{
			USDMultiplier: 0.1,
		},
	}, 7)

	require.InDelta(t, 0.36, dto.Multiplier, 0.000001)
	require.NotNil(t, dto.Actual.InputCNYPerM)
	require.InDelta(t, 25.2, *dto.Actual.InputCNYPerM, 0.000001)
	require.NotNil(t, dto.CheaperFactor)
	require.InDelta(t, 2.777778, *dto.CheaperFactor, 0.000001)
}

func TestPriceTiersFromLiteLLMIncludesLongContextAndFastTiers(t *testing.T) {
	pricing := &service.LiteLLMModelPricing{
		InputCostPerToken:               2e-6,
		InputCostPerTokenPriority:       4e-6,
		OutputCostPerToken:              10e-6,
		OutputCostPerTokenPriority:      20e-6,
		CacheCreationInputTokenCost:     2e-6,
		CacheReadInputTokenCost:         0.2e-6,
		CacheReadInputTokenCostPriority: 0.4e-6,
		ContextPriceTiers: []service.LiteLLMContextPriceTier{
			{
				ThresholdTokens:             256000,
				InputCostPerToken:           6e-6,
				OutputCostPerToken:          30e-6,
				CacheCreationInputTokenCost: 7e-6,
				CacheReadInputTokenCost:     0.6e-6,
			},
			{
				ThresholdTokens:         256000,
				Priority:                true,
				InputCostPerToken:       9e-6,
				OutputCostPerToken:      45e-6,
				CacheReadInputTokenCost: 0.9e-6,
			},
		},
	}

	tiers := actualizePriceTiers(priceTiersFromLiteLLM(pricing), 0.2, 7)
	require.Len(t, tiers, 4)
	require.Equal(t, "基础", tiers[0].Label)
	require.Equal(t, "长上下文", tiers[1].Label)
	require.Equal(t, "Fast", tiers[2].Label)
	require.Equal(t, "Fast 长上下文", tiers[3].Label)
	require.Equal(t, 256000, *tiers[3].ThresholdTokens)
	require.NotNil(t, tiers[3].Official.InputUSDPerM)
	require.Equal(t, 9.0, *tiers[3].Official.InputUSDPerM)
	require.NotNil(t, tiers[3].Actual.InputUSDPerM)
	require.Equal(t, 1.8, *tiers[3].Actual.InputUSDPerM)
	require.NotNil(t, tiers[3].Actual.InputCNYPerM)
	require.Equal(t, 12.6, *tiers[3].Actual.InputCNYPerM)
}
