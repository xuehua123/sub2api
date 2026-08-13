//go:build unit

package handler

import (
	"strings"
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

func (s stubModelPriceCatalog) GetIdentifiedModelPricing(model string) *service.LiteLLMModelPricing {
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

	models := h.modelsForGroup(nil, nil, modelPriceGroupDTO{
		ID:                  1,
		Platform:            service.PlatformAnthropic,
		EffectiveMultiplier: 0.1,
	}, 7, false, nil)

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

	models := h.modelsForGroup(nil, nil, modelPriceGroupDTO{
		ID:                  1,
		Platform:            service.PlatformAnthropic,
		EffectiveMultiplier: 0.1,
	}, 7, true, nil)

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

func TestApplyModelPriceHiddenModelsFiltersPerGroupUnlessIncluded(t *testing.T) {
	models := []modelPriceModelDTO{
		{Name: "gpt-5"},
		{Name: "gpt-5.5"},
	}
	hidden := map[string]struct{}{
		service.ModelPriceHiddenModelKey(46, "gpt-5.5"): {},
		service.ModelPriceHiddenModelKey(47, "gpt-5"):   {},
	}

	filtered := applyModelPriceHiddenModels(models, 46, hidden, false)
	require.Len(t, filtered, 1)
	require.Equal(t, "gpt-5", filtered[0].Name)

	included := applyModelPriceHiddenModels(models, 46, hidden, true)
	require.Len(t, included, 2)
	require.False(t, included[0].Hidden)
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

	applyModelPriceGroupUsage(groups, channels, nil, nil, false)

	require.Equal(t, 1, groups[0].ChannelCount)
	require.Equal(t, 1, groups[0].ModelCount)
	require.Equal(t, 1, groups[1].ChannelCount)
	require.Equal(t, 1, groups[1].ModelCount)
}

func TestApplyModelPriceGroupUsageFallsBackToAccountsWithoutChannels(t *testing.T) {
	groups := []modelPriceGroupDTO{
		{ID: 46, Platform: service.PlatformOpenAI},
	}
	accountsByGroup := map[int64][]service.Account{
		46: {
			{
				ID:       206,
				Name:     "pro",
				Platform: service.PlatformOpenAI,
				Status:   service.StatusActive,
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"gpt-5.5":     "gpt-5.5",
						"gpt-image-2": "gpt-image-2",
					},
				},
			},
			{
				ID:       207,
				Name:     "disabled",
				Platform: service.PlatformOpenAI,
				Status:   service.StatusDisabled,
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"gpt-5.4": "gpt-5.4",
					},
				},
			},
		},
	}

	applyModelPriceGroupUsage(groups, nil, accountsByGroup, nil, false)

	require.Equal(t, 1, groups[0].ChannelCount)
	require.Equal(t, 2, groups[0].ModelCount)
}

func TestModelsForGroupIncludesAccountModelMapping(t *testing.T) {
	h := &ModelPriceHandler{
		pricingService: stubModelPriceCatalog{
			prices: map[string]*service.LiteLLMModelPricing{
				"gpt-5.5": {
					LiteLLMProvider:   "openai",
					InputCostPerToken: 1e-6,
				},
			},
		},
	}
	accounts := []service.Account{
		{
			ID:       206,
			Name:     "pro",
			Platform: service.PlatformOpenAI,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"model_mapping": map[string]any{
					"gpt-5.5": "gpt-5.5",
					"gpt-*":   "gpt-5.5",
				},
			},
		},
	}

	models := h.modelsForGroup(nil, accounts, modelPriceGroupDTO{
		ID:                  46,
		Platform:            service.PlatformOpenAI,
		EffectiveMultiplier: 0.5,
	}, 7, false, nil)

	require.Len(t, models, 1)
	require.Equal(t, "gpt-5.5", models[0].Name)
	require.Equal(t, "official", models[0].PricingSource)
	require.Equal(t, []string{"pro"}, models[0].ChannelNames)
}

func TestModelsForGroup_DirectAccountMappingUsesTargetForGroupAndGlobalPricing(t *testing.T) {
	zero := 0.0
	globalTargetPrice := 4e-6
	account := service.Account{
		ID: 10, Name: "direct", Platform: service.PlatformOpenAI, Status: service.StatusActive,
		Credentials: map[string]any{"model_mapping": map[string]any{"public-alias": "provider-target"}},
	}
	group := modelPriceGroupDTO{
		ID: 46, Platform: service.PlatformOpenAI, EffectiveMultiplier: 1,
		modelPricing: []service.ChannelModelPricing{{
			Models: []string{"provider-target"}, BillingMode: service.BillingModeToken, InputPrice: &zero,
		}},
	}

	groupModels := (&ModelPriceHandler{}).modelsForGroup(nil, []service.Account{account}, group, 7, false, nil)
	require.Len(t, groupModels, 1)
	require.Equal(t, "public-alias", groupModels[0].Name)
	require.Equal(t, service.PricingSourceGroup, groupModels[0].PricingSource)
	require.NotNil(t, groupModels[0].Official.InputUSDPerM)
	require.Zero(t, *groupModels[0].Official.InputUSDPerM, "target group explicit zero must win")

	group.modelPricing = nil
	globalModels := (&ModelPriceHandler{pricingService: stubModelPriceCatalog{prices: map[string]*service.LiteLLMModelPricing{
		"provider-target": {InputCostPerToken: globalTargetPrice},
	}}}).modelsForGroup(nil, []service.Account{account}, group, 7, false, nil)
	require.Len(t, globalModels, 1)
	require.Equal(t, "official", globalModels[0].PricingSource)
	require.InDelta(t, 4.0, *globalModels[0].Official.InputUSDPerM, 1e-12)
}

func TestModelsForGroup_DirectAccountSelfMappingUsesRequestedModel(t *testing.T) {
	price := 3e-6
	h := &ModelPriceHandler{pricingService: stubModelPriceCatalog{prices: map[string]*service.LiteLLMModelPricing{
		"public-alias": {InputCostPerToken: price},
	}}}
	accounts := []service.Account{{
		ID: 10, Name: "direct", Platform: service.PlatformOpenAI, Status: service.StatusActive,
		Credentials: map[string]any{"model_mapping": map[string]any{"public-alias": "public-alias"}},
	}}

	models := h.modelsForGroup(nil, accounts, modelPriceGroupDTO{
		ID: 46, Platform: service.PlatformOpenAI, EffectiveMultiplier: 1,
	}, 7, false, nil)
	require.Len(t, models, 1)
	require.InDelta(t, 3.0, *models[0].Official.InputUSDPerM, 1e-12)
}

func TestCollectModelPriceAggregates_ChannelBillingModelIsNotOverwrittenByAccount(t *testing.T) {
	channelPrice := 2e-6
	accountPrice := 9e-6
	channels := []service.AvailableChannel{{
		ID: 1, Name: "channel", Status: service.StatusActive,
		Groups: []service.AvailableGroupRef{{ID: 46}},
		SupportedModels: []service.SupportedModel{{
			Name: "public-alias", Platform: service.PlatformOpenAI, BillingModel: "channel-target",
		}},
	}}
	accounts := []service.Account{{
		ID: 10, Name: "direct", Platform: service.PlatformOpenAI, Status: service.StatusActive,
		Credentials: map[string]any{"model_mapping": map[string]any{"public-alias": "account-target"}},
	}}
	models := (&ModelPriceHandler{}).modelsForGroup(channels, accounts, modelPriceGroupDTO{
		ID: 46, Platform: service.PlatformOpenAI, EffectiveMultiplier: 1,
		modelPricing: []service.ChannelModelPricing{
			{Models: []string{"channel-target"}, InputPrice: &channelPrice},
			{Models: []string{"account-target"}, InputPrice: &accountPrice},
		},
	}, 7, false, nil)

	require.Len(t, models, 1)
	require.InDelta(t, 2.0, *models[0].Official.InputUSDPerM, 1e-12)
	require.ElementsMatch(t, []string{"channel", "direct"}, models[0].ChannelNames)
}

func TestCollectModelPriceAggregates_ConflictingAccountTargetsFailClosedDeterministically(t *testing.T) {
	accounts := []service.Account{
		{ID: 20, Name: "later", Platform: service.PlatformOpenAI, Status: service.StatusActive,
			Credentials: map[string]any{"model_mapping": map[string]any{"public-alias": "target-b"}}},
		{ID: 10, Name: "first", Platform: service.PlatformOpenAI, Status: service.StatusActive,
			Credentials: map[string]any{"model_mapping": map[string]any{"public-alias": "target-a"}}},
	}
	group := modelPriceGroupDTO{ID: 46, Platform: service.PlatformOpenAI, EffectiveMultiplier: 1}
	aggregates := collectModelPriceAggregates(nil, accounts, group)
	agg := aggregates[strings.ToLower(service.PlatformOpenAI+"\x00public-alias")]
	require.NotNil(t, agg)
	require.Equal(t, "target-a", agg.billingModel, "lowest account ID is the deterministic first candidate")
	require.True(t, agg.billingModelAmbiguous)

	models := (&ModelPriceHandler{pricingService: stubModelPriceCatalog{prices: map[string]*service.LiteLLMModelPricing{
		"target-a": {InputCostPerToken: 1e-6},
		"target-b": {InputCostPerToken: 9e-6},
	}}}).modelsForGroup(nil, accounts, group, 7, false, nil)
	require.Len(t, models, 1)
	require.Equal(t, "unknown", models[0].PricingSource)
	require.True(t, models[0].OfficialMissing)
	require.Nil(t, models[0].Official.InputUSDPerM, "ambiguous target must not advertise either account price")
}

func TestCollectModelPriceAggregates_ConflictingChannelProjectionFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*service.SupportedModel)
	}{
		{
			name: "billing model",
			mutate: func(model *service.SupportedModel) {
				model.BillingModel = "target-b"
			},
		},
		{
			name: "billing mode",
			mutate: func(model *service.SupportedModel) {
				model.Pricing.BillingMode = service.BillingModePerRequest
			},
		},
		{
			name: "effective pricing",
			mutate: func(model *service.SupportedModel) {
				otherPrice := 9e-6
				model.Pricing.InputPrice = &otherPrice
			},
		},
		{
			name: "pricing source",
			mutate: func(model *service.SupportedModel) {
				model.PricingSource = service.PricingSourceOfficial
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			firstPrice := 2e-6
			secondPrice := 2e-6
			first := service.SupportedModel{
				Name: "public-alias", Platform: service.PlatformOpenAI, BillingModel: "target-a",
				Pricing:       &service.ChannelModelPricing{BillingMode: service.BillingModeToken, InputPrice: &firstPrice},
				PricingSource: service.PricingSourceChannel,
			}
			second := service.SupportedModel{
				Name: "public-alias", Platform: service.PlatformOpenAI, BillingModel: "target-a",
				Pricing:       &service.ChannelModelPricing{BillingMode: service.BillingModeToken, InputPrice: &secondPrice},
				PricingSource: service.PricingSourceChannel,
			}
			tt.mutate(&second)
			channels := []service.AvailableChannel{
				{ID: 1, Name: "first", Status: service.StatusActive, Groups: []service.AvailableGroupRef{{ID: 46}}, SupportedModels: []service.SupportedModel{first}},
				{ID: 2, Name: "second", Status: service.StatusActive, Groups: []service.AvailableGroupRef{{ID: 46}}, SupportedModels: []service.SupportedModel{second}},
			}
			group := modelPriceGroupDTO{ID: 46, Platform: service.PlatformOpenAI, EffectiveMultiplier: 1}

			agg := collectModelPriceAggregates(channels, nil, group)[strings.ToLower(service.PlatformOpenAI+"\x00public-alias")]
			require.NotNil(t, agg)
			require.True(t, agg.billingModelAmbiguous)

			models := (&ModelPriceHandler{channelService: &service.ChannelService{}}).modelsForGroup(channels, nil, group, 7, false, nil)
			require.Len(t, models, 1)
			require.Equal(t, "unknown", models[0].PricingSource)
			require.True(t, models[0].OfficialMissing)
			require.Nil(t, models[0].Official.InputUSDPerM)
			require.ElementsMatch(t, []string{"first", "second"}, models[0].ChannelNames)
		})
	}
}

func TestCollectModelPriceAggregates_EquivalentChannelProjectionRemainsUsable(t *testing.T) {
	firstPrice := 2e-6
	secondPrice := 2e-6
	channels := []service.AvailableChannel{
		{ID: 1, Name: "first", Status: service.StatusActive, Groups: []service.AvailableGroupRef{{ID: 46}}, SupportedModels: []service.SupportedModel{{
			Name: "public-alias", Platform: service.PlatformOpenAI, BillingModel: "Target-A",
			Pricing: &service.ChannelModelPricing{BillingMode: service.BillingModeToken, InputPrice: &firstPrice}, PricingSource: service.PricingSourceChannel,
		}}},
		{ID: 2, Name: "second", Status: service.StatusActive, Groups: []service.AvailableGroupRef{{ID: 46}}, SupportedModels: []service.SupportedModel{{
			Name: "public-alias", Platform: service.PlatformOpenAI, BillingModel: "target-a",
			Pricing: &service.ChannelModelPricing{InputPrice: &secondPrice}, PricingSource: service.PricingSourceChannel,
		}}},
	}
	agg := collectModelPriceAggregates(channels, nil, modelPriceGroupDTO{ID: 46, Platform: service.PlatformOpenAI})[strings.ToLower(service.PlatformOpenAI+"\x00public-alias")]
	require.NotNil(t, agg)
	require.False(t, agg.billingModelAmbiguous)
}

func TestModelsForGroup_ChannelBackedGroupMissPreservesProjectedSource(t *testing.T) {
	globalPrice := 1e-6
	groupPrice := 2e-6
	channels := []service.AvailableChannel{{
		ID: 1, Name: "channel", Status: service.StatusActive,
		Groups: []service.AvailableGroupRef{{ID: 46}},
		SupportedModels: []service.SupportedModel{{
			Name: "public-alias", Platform: service.PlatformOpenAI, BillingModel: "provider-model",
			Pricing:       &service.ChannelModelPricing{InputPrice: &globalPrice},
			PricingSource: service.PricingSourceOfficial,
			PricingByGroup: map[int64]*service.ChannelModelPricing{
				46: {InputPrice: &groupPrice},
			},
			PricingSourceByGroup: map[int64]string{46: service.PricingSourceChannel},
		}},
	}}
	h := &ModelPriceHandler{
		channelService: &service.ChannelService{},
		pricingService: stubModelPriceCatalog{prices: map[string]*service.LiteLLMModelPricing{
			"provider-model": {InputCostPerToken: globalPrice},
		}},
	}

	models := h.modelsForGroup(channels, nil, modelPriceGroupDTO{
		ID: 46, Platform: service.PlatformOpenAI, EffectiveMultiplier: 1,
	}, 7, false, nil)

	require.Len(t, models, 1)
	require.Equal(t, service.PricingSourceChannel, models[0].PricingSource)
	require.NotNil(t, models[0].Official.InputUSDPerM)
	require.InDelta(t, 2.0, *models[0].Official.InputUSDPerM, 1e-12)
}

func TestAccountModelPriceEntries_CaseDuplicateTargetsFailClosed(t *testing.T) {
	account := service.Account{
		ID: 10, Platform: service.PlatformOpenAI, Status: service.StatusActive,
		Credentials: map[string]any{"model_mapping": map[string]any{
			"Alias": "target-a",
			"alias": "target-b",
		}},
	}
	entries := accountModelPriceEntries(account, service.PlatformOpenAI)
	require.Len(t, entries, 1)
	require.True(t, entries[0].ambiguous)
	require.Equal(t, "Alias", entries[0].displayName, "sorted raw key is deterministic")
}

func TestModelsForGroupUsesMappedGroupVideoPriceAndPreservesZero(t *testing.T) {
	zero := 0.0
	h := &ModelPriceHandler{}
	channels := []service.AvailableChannel{{
		ID: 1, Name: "video", Status: service.StatusActive,
		Groups: []service.AvailableGroupRef{{ID: 46}},
		SupportedModels: []service.SupportedModel{{
			Name: "video-public", Platform: service.PlatformOpenAI,
			BillingModel: "video-billing-model",
		}},
	}}

	models := h.modelsForGroup(channels, nil, modelPriceGroupDTO{
		ID:                   46,
		Platform:             service.PlatformOpenAI,
		EffectiveMultiplier:  0.5,
		VideoRateIndependent: true,
		VideoRateMultiplier:  0.25,
		modelPricing: []service.ChannelModelPricing{{
			Models: []string{"video-billing-*"}, BillingMode: service.BillingModeVideo,
			PerRequestPrice: &zero,
		}},
	}, 7, false, nil)

	require.Len(t, models, 1)
	require.Equal(t, "video", models[0].BillingMode)
	require.Equal(t, service.PricingSourceGroup, models[0].PricingSource)
	require.False(t, models[0].OfficialMissing)
	require.NotNil(t, models[0].Official.PerRequestUSD)
	require.Zero(t, *models[0].Official.PerRequestUSD)
	require.NotNil(t, models[0].Actual.PerRequestUSD)
	require.Zero(t, *models[0].Actual.PerRequestUSD)
	require.Equal(t, 0.25, models[0].Multiplier)
}

func TestModelsForGroupUsesRequestedBillingModelForGroupPrice(t *testing.T) {
	requestedPrice := 1e-6
	mappedPrice := 8e-6
	h := &ModelPriceHandler{}
	channels := []service.AvailableChannel{{
		ID: 1, Name: "requested", Status: service.StatusActive,
		Groups: []service.AvailableGroupRef{{ID: 46}},
		SupportedModels: []service.SupportedModel{{
			Name: "public-alias", Platform: service.PlatformOpenAI,
			BillingModel: "public-alias",
		}},
	}}

	models := h.modelsForGroup(channels, nil, modelPriceGroupDTO{
		ID: 46, Platform: service.PlatformOpenAI, EffectiveMultiplier: 1,
		modelPricing: []service.ChannelModelPricing{
			{Models: []string{"public-alias"}, InputPrice: &requestedPrice},
			{Models: []string{"provider-model"}, InputPrice: &mappedPrice},
		},
	}, 7, false, nil)

	require.Len(t, models, 1)
	require.Equal(t, service.PricingSourceGroup, models[0].PricingSource)
	require.NotNil(t, models[0].Official.InputUSDPerM)
	require.InDelta(t, 1.0, *models[0].Official.InputUSDPerM, 1e-12)
}

func TestModelsForGroup_MappedAliasUsesBillingModelForOfficialPrice(t *testing.T) {
	globalPrice := 4e-6
	globalImageInputPrice := 7e-6
	h := &ModelPriceHandler{pricingService: stubModelPriceCatalog{
		prices: map[string]*service.LiteLLMModelPricing{
			"provider-model": {
				LiteLLMProvider: "openai", InputCostPerToken: globalPrice,
				InputCostPerImageToken: globalImageInputPrice,
			},
		},
	}}
	channels := []service.AvailableChannel{{
		ID: 1, Name: "mapped", Status: service.StatusActive,
		Groups: []service.AvailableGroupRef{{ID: 46}},
		SupportedModels: []service.SupportedModel{{
			Name: "public-alias", Platform: service.PlatformOpenAI,
			BillingModel: "provider-model",
		}},
	}}

	models := h.modelsForGroup(channels, nil, modelPriceGroupDTO{
		ID: 46, Platform: service.PlatformOpenAI, EffectiveMultiplier: 0.5,
	}, 7, false, nil)

	require.Len(t, models, 1)
	require.Equal(t, "public-alias", models[0].Name)
	require.Equal(t, "official", models[0].PricingSource)
	require.False(t, models[0].OfficialMissing)
	require.NotNil(t, models[0].Official.InputUSDPerM)
	require.InDelta(t, 4.0, *models[0].Official.InputUSDPerM, 1e-12)
	require.InDelta(t, 2.0, *models[0].Actual.InputUSDPerM, 1e-12)
	require.InDelta(t, 7.0, *models[0].Official.ImageInputUSDPerM, 1e-12)
	require.InDelta(t, 3.5, *models[0].Actual.ImageInputUSDPerM, 1e-12)
}

func TestModelsForGroup_GroupLongContextSwitchControlsDisplayedTiers(t *testing.T) {
	baseInput := 2e-6
	baseOutput := 10e-6
	h := &ModelPriceHandler{pricingService: stubModelPriceCatalog{
		prices: map[string]*service.LiteLLMModelPricing{
			"gpt-5.6-sol": {
				InputCostPerToken:               3e-6,
				OutputCostPerToken:              15e-6,
				LongContextInputTokenThreshold:  272000,
				LongContextInputCostMultiplier:  2,
				LongContextOutputCostMultiplier: 1.5,
			},
		},
	}}
	channels := []service.AvailableChannel{{
		ID: 1, Name: "openai", Status: service.StatusActive,
		Groups: []service.AvailableGroupRef{{ID: 46}},
		SupportedModels: []service.SupportedModel{{
			Name: "gpt-5.6-sol", Platform: service.PlatformOpenAI,
			BillingModel: "gpt-5.6-sol",
		}},
	}}
	group := modelPriceGroupDTO{
		ID: 46, Platform: service.PlatformOpenAI, EffectiveMultiplier: 0.5,
		modelPricing: []service.ChannelModelPricing{{
			Models: []string{"gpt-5.6-sol"}, BillingMode: service.BillingModeToken,
			InputPrice: &baseInput, OutputPrice: &baseOutput,
		}},
		longContextPricing: true,
	}

	enabled := h.modelsForGroup(channels, nil, group, 7, false, nil)
	require.Len(t, enabled, 1)
	require.Len(t, enabled[0].PriceTiers, 4)
	require.Equal(t, "channel_interval_0", enabled[0].PriceTiers[0].Key)
	require.Equal(t, "channel_interval_1", enabled[0].PriceTiers[1].Key)
	require.Nil(t, enabled[0].PriceTiers[0].ThresholdTokens)
	require.NotNil(t, enabled[0].PriceTiers[1].ThresholdTokens)
	require.Equal(t, 272000, *enabled[0].PriceTiers[1].ThresholdTokens)
	require.InDelta(t, 4.0, *enabled[0].PriceTiers[1].Official.InputUSDPerM, 1e-12)
	require.InDelta(t, 2.0, *enabled[0].PriceTiers[1].Actual.InputUSDPerM, 1e-12)

	group.longContextPricing = false
	disabled := h.modelsForGroup(channels, nil, group, 7, false, nil)
	require.Len(t, disabled, 1)
	require.Len(t, disabled[0].PriceTiers, 2)
	require.Equal(t, "channel_interval_0", disabled[0].PriceTiers[0].Key)
	require.Equal(t, "channel_interval_2", disabled[0].PriceTiers[1].Key)
	require.InDelta(t, 2.0, *disabled[0].PriceTiers[0].Official.InputUSDPerM, 1e-12)
}

func TestModelPriceMultiplier_PreservesZeroIndependentMediaRates(t *testing.T) {
	imagePrice := 0.02
	require.Zero(t, modelPriceMultiplier(modelPriceGroupDTO{
		EffectiveMultiplier:  0.5,
		ImageRateIndependent: true,
		ImageRateMultiplier:  0,
	}, string(service.BillingModeImage), modelPriceValueDTO{PerRequestUSD: &imagePrice}))

	require.Zero(t, modelPriceMultiplier(modelPriceGroupDTO{
		EffectiveMultiplier:  0.5,
		VideoRateIndependent: true,
		VideoRateMultiplier:  0,
	}, string(service.BillingModeVideo), modelPriceValueDTO{PerRequestUSD: &imagePrice}))
}

func TestModelPriceMultiplier_TokenImageOutputDoesNotSelectImageMultiplier(t *testing.T) {
	zero := 0.0
	positive := 9.0
	group := modelPriceGroupDTO{
		EffectiveMultiplier:  0.5,
		ImageRateIndependent: true,
		ImageRateMultiplier:  3,
	}
	for _, imageOutput := range []*float64{&zero, &positive} {
		require.InDelta(t, 0.5, modelPriceMultiplier(group, string(service.BillingModeToken), modelPriceValueDTO{
			ImageOutputUSDPerM: imageOutput,
		}), 1e-12)
	}
}

func TestPricingHasValues_IncludesImageInputExplicitZero(t *testing.T) {
	zero := 0.0
	require.True(t, pricingHasValues(&service.ChannelModelPricing{ImageInputPrice: &zero}))
}

func TestModelsForGroupAppliesCustomUSDPrice(t *testing.T) {
	h := &ModelPriceHandler{
		pricingService: stubModelPriceCatalog{
			prices: map[string]*service.LiteLLMModelPricing{
				"deepseek-v4-pro": {
					LiteLLMProvider:   "deepseek",
					InputCostPerToken: 0.435e-6,
				},
			},
		},
	}
	customInput := 0.8
	accounts := []service.Account{
		{
			ID:       301,
			Name:     "domestic",
			Platform: service.PlatformOpenAI,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"model_mapping": map[string]any{
					"deepseek-v4-pro": "deepseek-v4-pro",
				},
			},
		},
	}

	models := h.modelsForGroup(nil, accounts, modelPriceGroupDTO{
		ID:                  70,
		Platform:            service.PlatformOpenAI,
		EffectiveMultiplier: 0.5,
	}, 7, false, map[string]service.ModelPriceCustomPrice{
		service.ModelPriceCustomPriceKey(70, "deepseek-v4-pro"): {
			BillingMode:  string(service.BillingModeToken),
			InputUSDPerM: &customInput,
		},
	})

	require.Len(t, models, 1)
	require.Equal(t, "custom", models[0].PricingSource)
	require.NotNil(t, models[0].CustomPrice)
	require.NotNil(t, models[0].Actual.InputCNYPerM)
	require.InDelta(t, 5.6, *models[0].Actual.InputCNYPerM, 0.000001)
	require.NotNil(t, models[0].Actual.InputUSDPerM)
	require.Equal(t, 0.8, *models[0].Actual.InputUSDPerM)
}

func TestModelPriceDTOUsesChannelIntervalsWhenOfficialCatalogMissing(t *testing.T) {
	inputBase := 2.5e-6
	outputBase := 10e-6
	inputLong := 4e-6
	outputLong := 16e-6
	maxBase := 32768
	h := &ModelPriceHandler{}

	model := h.toModelPriceDTO(&modelAggregate{
		name:        "qwen3-max",
		platform:    service.PlatformOpenAI,
		billingMode: string(service.BillingModeToken),
		pricing: &service.ChannelModelPricing{
			InputPrice:  &inputBase,
			OutputPrice: &outputBase,
			Intervals: []service.PricingInterval{
				{
					TierLabel:   "<=32K",
					MaxTokens:   &maxBase,
					InputPrice:  &inputBase,
					OutputPrice: &outputBase,
					SortOrder:   0,
				},
				{
					TierLabel:   "32K-128K",
					MinTokens:   maxBase,
					InputPrice:  &inputLong,
					OutputPrice: &outputLong,
					SortOrder:   1,
				},
			},
		},
	}, modelPriceGroupDTO{
		ID:                  51,
		Platform:            service.PlatformOpenAI,
		EffectiveMultiplier: 0.4,
	}, 7)

	require.Equal(t, "channel", model.PricingSource)
	require.False(t, model.OfficialMissing)
	require.Len(t, model.PriceTiers, 2)
	require.Equal(t, "<=32K", model.PriceTiers[0].Label)
	require.Nil(t, model.PriceTiers[0].ThresholdTokens)
	require.InDelta(t, 2.5, *model.PriceTiers[0].Official.InputUSDPerM, 0.000001)
	require.InDelta(t, 1.0, *model.PriceTiers[0].Actual.InputUSDPerM, 0.000001)
	require.Equal(t, "32K-128K", model.PriceTiers[1].Label)
	require.Equal(t, maxBase, *model.PriceTiers[1].ThresholdTokens)
	require.InDelta(t, 4.0, *model.PriceTiers[1].Official.InputUSDPerM, 0.000001)
	require.InDelta(t, 6.4, *model.PriceTiers[1].Actual.OutputUSDPerM, 0.000001)
}

func TestModelPriceDTOKeepsEffectiveFallbackSource(t *testing.T) {
	input := 2e-6
	dto := (&ModelPriceHandler{}).toModelPriceDTO(&modelAggregate{
		name:          "gpt-5.5",
		platform:      service.PlatformOpenAI,
		billingMode:   string(service.BillingModeToken),
		pricingSource: service.PricingSourceFallback,
		pricing:       &service.ChannelModelPricing{InputPrice: &input},
		channelNames:  map[string]struct{}{},
	}, modelPriceGroupDTO{EffectiveMultiplier: 1}, 7)

	require.Equal(t, service.PricingSourceFallback, dto.PricingSource)
	require.InDelta(t, 2, *dto.Official.InputUSDPerM, 1e-12)
}

func TestSanitizeModelPriceModelsForUserHidesCustomPriceMetadata(t *testing.T) {
	customInput := 0.8
	models := sanitizeModelPriceModelsForUser([]modelPriceModelDTO{{
		Name:          "deepseek-v4-pro",
		PricingSource: "custom",
		CustomPrice: &service.ModelPriceCustomPrice{
			InputUSDPerM: &customInput,
		},
		Actual: modelPriceActualDTO{
			InputUSDPerM: &customInput,
		},
	}})

	require.Len(t, models, 1)
	require.Nil(t, models[0].CustomPrice)
	require.Equal(t, "official", models[0].PricingSource)
	require.NotNil(t, models[0].Actual.InputUSDPerM)
	require.Equal(t, 0.8, *models[0].Actual.InputUSDPerM)
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

	overview := buildModelPriceGroupOverview(groups, channels, nil, nil, false)
	openai := overview[1]

	require.Equal(t, "openai", openai.Category)
	require.Equal(t, 2, openai.GroupCount)
	require.Equal(t, 1, openai.ModelCount)
	require.Equal(t, 1, openai.ChannelCount)
}

func TestModelPriceGroupUsageExcludesHiddenModelsFromCounts(t *testing.T) {
	groups := []modelPriceGroupDTO{
		{ID: 46, Platform: service.PlatformOpenAI},
	}
	channels := []service.AvailableChannel{{
		ID:     10,
		Name:   "primary",
		Status: service.StatusActive,
		Groups: []service.AvailableGroupRef{{ID: 46}},
		SupportedModels: []service.SupportedModel{
			{Name: "gpt-5", Platform: service.PlatformOpenAI},
			{Name: "gpt-5.5", Platform: service.PlatformOpenAI},
		},
	}}
	hidden := map[string]struct{}{
		service.ModelPriceHiddenModelKey(46, "gpt-5.5"): {},
	}

	applyModelPriceGroupUsage(groups, channels, nil, hidden, false)
	require.Equal(t, 1, groups[0].ModelCount)

	applyModelPriceGroupUsage(groups, channels, nil, hidden, true)
	require.Equal(t, 2, groups[0].ModelCount)
}

func TestBuildModelPriceGroupOverviewExcludesHiddenModels(t *testing.T) {
	groups := []modelPriceGroupDTO{
		{ID: 1, Platform: service.PlatformOpenAI},
	}
	channels := []service.AvailableChannel{{
		ID:     10,
		Name:   "primary",
		Status: service.StatusActive,
		Groups: []service.AvailableGroupRef{{ID: 1}},
		SupportedModels: []service.SupportedModel{
			{Name: "gpt-5", Platform: service.PlatformOpenAI},
			{Name: "gpt-5.5", Platform: service.PlatformOpenAI},
		},
	}}
	hidden := map[string]struct{}{
		service.ModelPriceHiddenModelKey(1, "gpt-5.5"): {},
	}

	overview := buildModelPriceGroupOverview(groups, channels, nil, hidden, false)
	require.Equal(t, 1, overview[1].ModelCount)

	overview = buildModelPriceGroupOverview(groups, channels, nil, hidden, true)
	require.Equal(t, 2, overview[1].ModelCount)
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
