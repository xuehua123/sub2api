//go:build unit

package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/stretchr/testify/require"
)

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

func TestSelectModelPriceGroupIDFallsBackToFirstVisibleGroup(t *testing.T) {
	groups := []modelPriceGroupDTO{
		{ID: 2, Name: "B"},
		{ID: 3, Name: "C"},
	}

	require.Equal(t, int64(3), *selectModelPriceGroupID("3", groups))
	require.Equal(t, int64(2), *selectModelPriceGroupID("999", groups))
	require.Nil(t, selectModelPriceGroupID("", nil))
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

func TestModelPriceDTOPrefersPackageMultiplier(t *testing.T) {
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

	require.Equal(t, 0.1, dto.Multiplier)
	require.NotNil(t, dto.Actual.InputCNYPerM)
	require.Equal(t, 7.0, *dto.Actual.InputCNYPerM)
	require.NotNil(t, dto.CheaperFactor)
	require.Equal(t, 10.0, *dto.CheaperFactor)
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
