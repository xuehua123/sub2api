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

func TestModelsForGroup_AnthropicFastRequiresDirectNonBedrockAccount(t *testing.T) {
	h := &ModelPriceHandler{pricingService: stubModelPriceCatalog{prices: map[string]*service.LiteLLMModelPricing{
		"claude-opus-4-8": {
			LiteLLMProvider:                     "anthropic",
			InputCostPerToken:                   5e-6,
			OutputCostPerToken:                  25e-6,
			CacheCreationInputTokenCost:         6.25e-6,
			CacheCreationInputTokenCostAbove1hr: 10e-6,
			CacheReadInputTokenCost:             0.5e-6,
		},
	}}}
	group := modelPriceGroupDTO{ID: 46, Platform: service.PlatformAnthropic, EffectiveMultiplier: 1}
	account := func(accountType string) service.Account {
		return service.Account{
			ID: 10, Name: accountType, Platform: service.PlatformAnthropic, Type: accountType,
			Status: service.StatusActive, Schedulable: true,
			Credentials: map[string]any{"model_mapping": map[string]any{
				"claude-opus-4-8": "claude-opus-4-8",
			}},
		}
	}

	t.Run("direct Anthropic exposes effective cache breakdown and Fast", func(t *testing.T) {
		models := h.modelsForGroup(nil, []service.Account{account(service.AccountTypeAPIKey)}, group, 1, false, nil)
		require.Len(t, models, 1)
		got := models[0]
		require.NotNil(t, got.Official.CacheWrite5mUSDPerM)
		require.NotNil(t, got.Official.CacheWrite1hUSDPerM)
		require.InDelta(t, 6.25, *got.Official.CacheWrite5mUSDPerM, 1e-12)
		require.InDelta(t, 10.0, *got.Official.CacheWrite1hUSDPerM, 1e-12)
		require.Len(t, got.PriceTiers, 2)
		require.Equal(t, "Fast", got.PriceTiers[1].Label)
		require.InDelta(t, 10.0, *got.PriceTiers[1].Official.InputUSDPerM, 1e-12)
		require.InDelta(t, 12.5, *got.PriceTiers[1].Official.CacheWrite5mUSDPerM, 1e-12)
		require.InDelta(t, 20.0, *got.PriceTiers[1].Official.CacheWrite1hUSDPerM, 1e-12)
	})

	t.Run("Bedrock keeps cache breakdown but does not advertise Fast", func(t *testing.T) {
		models := h.modelsForGroup(nil, []service.Account{account(service.AccountTypeBedrock)}, group, 1, false, nil)
		require.Len(t, models, 1)
		got := models[0]
		require.NotNil(t, got.Official.CacheWrite5mUSDPerM)
		require.NotNil(t, got.Official.CacheWrite1hUSDPerM)
		require.Len(t, got.PriceTiers, 1)
		require.Equal(t, "基础", got.PriceTiers[0].Label)
	})

	t.Run("mixed direct and Bedrock candidates fail closed", func(t *testing.T) {
		models := h.modelsForGroup(nil, []service.Account{
			account(service.AccountTypeAPIKey),
			account(service.AccountTypeBedrock),
		}, group, 1, false, nil)
		require.Len(t, models, 1)
		require.Len(t, models[0].PriceTiers, 1)
		require.Equal(t, "基础", models[0].PriceTiers[0].Label)
	})

	t.Run("unschedulable direct candidate does not advertise Fast", func(t *testing.T) {
		direct := account(service.AccountTypeAPIKey)
		direct.Schedulable = false
		models := h.modelsForGroup(nil, []service.Account{direct}, group, 1, false, nil)
		require.Len(t, models, 1)
		require.Len(t, models[0].PriceTiers, 1)
		require.Equal(t, "基础", models[0].PriceTiers[0].Label)
	})
}

func TestModelsForGroup_OpaqueAnthropicChannelAliasUsesFinalRuntimeFastCapability(t *testing.T) {
	input, output := 5e-6, 25e-6
	cacheWrite5m, cacheWrite1h, cacheRead := 6.25e-6, 10e-6, 0.5e-6
	poisonedMultiplier := 3.0
	fastInput, fastOutput := input*poisonedMultiplier, output*poisonedMultiplier
	poisoned := service.ChannelModelPricing{
		Platform:          service.PlatformAnthropic,
		BillingMode:       service.BillingModeToken,
		InputPrice:        &input,
		OutputPrice:       &output,
		CacheWrite5mPrice: &cacheWrite5m,
		CacheWrite1hPrice: &cacheWrite1h,
		CacheReadPrice:    &cacheRead,
		FastMultiplier:    &poisonedMultiplier,
		Intervals: []service.PricingInterval{
			{TierLabel: "基础", InputPrice: &input, OutputPrice: &output, CacheWrite5mPrice: &cacheWrite5m, CacheWrite1hPrice: &cacheWrite1h, CacheReadPrice: &cacheRead},
			{TierLabel: "Fast stale", InputPrice: &fastInput, OutputPrice: &fastOutput},
		},
	}
	poisonedBefore := poisoned.Clone()
	h := &ModelPriceHandler{channelService: service.NewChannelService(nil, nil, nil, nil, nil)}
	group := modelPriceGroupDTO{ID: 46, Platform: service.PlatformAnthropic, EffectiveMultiplier: 1}

	run := func(t *testing.T, billingModel string, accounts []service.Account) modelPriceModelDTO {
		t.Helper()
		accountsBefore := append([]service.Account(nil), accounts...)
		models := h.modelsForGroup([]service.AvailableChannel{{
			ID: 1, Name: "opaque-alias", Status: service.StatusActive,
			Groups: []service.AvailableGroupRef{{ID: group.ID, Platform: service.PlatformAnthropic}},
			SupportedModels: []service.SupportedModel{{
				Name: "opaque-opus", Platform: service.PlatformAnthropic, BillingModel: billingModel,
				Pricing: &poisoned, PricingSource: service.PricingSourceChannel,
			}},
		}}, accounts, group, 1, false, nil)
		require.Equal(t, accountsBefore, accounts, "Fast capability probing must not write model-mapping caches into the input slice")
		require.Equal(t, poisonedBefore, poisoned, "Fast display projection must not mutate the channel card")
		for _, candidate := range models {
			if candidate.Name == "opaque-opus" {
				return candidate
			}
		}
		t.Fatalf("opaque-opus row missing from model price output: %+v", models)
		return modelPriceModelDTO{}
	}
	direct := func(mapping map[string]any) service.Account {
		return service.Account{
			ID: 1, Name: "direct", Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
			Status: service.StatusActive, Schedulable: true,
			Credentials: map[string]any{"model_mapping": mapping},
		}
	}

	t.Run("opaque intermediate alias resolves to direct Opus 4.8", func(t *testing.T) {
		got := run(t, "opaque-mid", []service.Account{direct(map[string]any{
			"opaque-opus": "opaque-mid",
			"opaque-mid":  "claude-opus-4-8",
		})})
		require.Len(t, got.PriceTiers, 2)
		require.Equal(t, "Fast", got.PriceTiers[1].Label)
		require.InDelta(t, input*poisonedMultiplier*1e6, *got.PriceTiers[1].Official.InputUSDPerM, 1e-12)
	})

	t.Run("canonical channel target remains direct Fast capable", func(t *testing.T) {
		got := run(t, "claude-opus-4-8", []service.Account{direct(map[string]any{
			"opaque-opus": "claude-opus-4-8",
		})})
		require.Len(t, got.PriceTiers, 2)
		require.Equal(t, "Fast", got.PriceTiers[1].Label)
	})

	t.Run("unsupported final direct model sanitizes poisoned Fast", func(t *testing.T) {
		got := run(t, "opaque-mid", []service.Account{direct(map[string]any{
			"opaque-opus": "opaque-mid",
			"opaque-mid":  "claude-opus-4-7",
		})})
		require.Len(t, got.PriceTiers, 1)
		require.Equal(t, "基础", got.PriceTiers[0].Label)
	})

	t.Run("mixed direct and Bedrock canonical target sanitizes poisoned Fast", func(t *testing.T) {
		bedrock := direct(map[string]any{"opaque-opus": "claude-opus-4-8"})
		bedrock.ID = 2
		bedrock.Name = "bedrock"
		bedrock.Type = service.AccountTypeBedrock
		got := run(t, "claude-opus-4-8", []service.Account{
			direct(map[string]any{"opaque-opus": "claude-opus-4-8"}),
			bedrock,
		})
		require.Len(t, got.PriceTiers, 1)
		require.Equal(t, "基础", got.PriceTiers[0].Label)
	})

	t.Run("mixed Antigravity candidate vetoes opaque public alias", func(t *testing.T) {
		ag := service.Account{
			ID: 3, Name: "antigravity", Platform: service.PlatformAntigravity, Type: service.AccountTypeAPIKey,
			Status: service.StatusActive, Schedulable: true,
			Extra: map[string]any{"mixed_scheduling": true},
			Credentials: map[string]any{"model_mapping": map[string]any{
				"opaque-opus": "claude-opus-4-8",
			}},
		}
		got := run(t, "claude-opus-4-8", []service.Account{
			direct(map[string]any{"opaque-opus": "claude-opus-4-8"}),
			ag,
		})
		require.Len(t, got.PriceTiers, 1)
		require.Equal(t, "基础", got.PriceTiers[0].Label)
	})
}

func TestModelPriceSupportsDirectAnthropicFastRequiresEveryRuntimeCandidate(t *testing.T) {
	direct := func(mapping map[string]any) service.Account {
		credentials := map[string]any{}
		if mapping != nil {
			credentials["model_mapping"] = mapping
		}
		return service.Account{
			Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
			Status: service.StatusActive, Schedulable: true, Credentials: credentials,
		}
	}
	mapped := direct(map[string]any{"public-opus": "claude-opus-4-8"})
	accounts := []service.Account{mapped}
	accountsBefore := append([]service.Account(nil), accounts...)
	require.True(t, modelPriceDirectAnthropicFastEnabled(
		accounts, service.PlatformAnthropic, "public-opus", "claude-opus-4-8", false,
	))
	require.Equal(t, accountsBefore, accounts, "capability checks must not populate caches on caller-owned accounts")

	opus5 := direct(map[string]any{"public-opus": "claude-opus-5"})
	require.True(t, modelPriceDirectAnthropicFastEnabled(
		[]service.Account{mapped, opus5}, service.PlatformAnthropic, "public-opus", "public-opus", false,
	), "different final models remain valid when every candidate supports Fast")

	unmapped := direct(nil)
	require.False(t, modelPriceDirectAnthropicFastEnabled(
		[]service.Account{mapped, unmapped}, service.PlatformAnthropic, "public-opus", "claude-opus-4-8", false,
	), "one account's mapping must not make another account look Fast-capable")

	mixedAntigravity := service.Account{
		Platform: service.PlatformAntigravity, Status: service.StatusActive, Schedulable: true,
		Extra:       map[string]any{"mixed_scheduling": true},
		Credentials: map[string]any{"model_mapping": map[string]any{"public-opus": "claude-opus-4-8"}},
	}
	require.False(t, modelPriceDirectAnthropicFastEnabled(
		[]service.Account{mapped, mixedAntigravity}, service.PlatformAnthropic, "public-opus", "claude-opus-4-8", false,
	))
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
		InputCostPerToken:                  2e-6,
		InputCostPerTokenPriority:          4e-6,
		OutputCostPerToken:                 10e-6,
		OutputCostPerTokenPriority:         20e-6,
		CacheCreationInputTokenCost:        2e-6,
		CacheReadInputTokenCost:            0.2e-6,
		CacheReadInputTokenCostPriority:    0.4e-6,
		LongContextInputTokenThreshold:     256000,
		LongContextInputCostMultiplier:     3,
		LongContextOutputCostMultiplier:    3,
		LongContextCacheReadCostMultiplier: 3,
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

	tiers := actualizePriceTiers(priceTiersFromLiteLLM("test-model", pricing, false, false), 0.2, 7)
	require.Len(t, tiers, 4)
	require.Equal(t, "基础", tiers[0].Label)
	require.Equal(t, "长上下文", tiers[1].Label)
	require.Equal(t, "Fast", tiers[2].Label)
	require.Equal(t, "Fast 长上下文", tiers[3].Label)
	require.Equal(t, 256000, *tiers[3].ThresholdTokens)
	require.NotNil(t, tiers[3].Official.InputUSDPerM)
	require.Equal(t, 12.0, *tiers[3].Official.InputUSDPerM)
	require.NotNil(t, tiers[3].Actual.InputUSDPerM)
	require.Equal(t, 2.4, *tiers[3].Actual.InputUSDPerM)
	require.NotNil(t, tiers[3].Actual.InputCNYPerM)
	require.Equal(t, 16.8, *tiers[3].Actual.InputCNYPerM)
}

func TestPriceTiersFromLiteLLMUsesPriorityCacheWritePrice(t *testing.T) {
	pricing := &service.LiteLLMModelPricing{
		InputCostPerToken:                   2e-6,
		InputCostPerTokenPriority:           3.6e-6,
		OutputCostPerToken:                  12e-6,
		OutputCostPerTokenPriority:          21.6e-6,
		CacheCreationInputTokenCost:         2e-6,
		CacheCreationInputTokenCostPriority: 3.6e-6,
		CacheReadInputTokenCost:             0.2e-6,
		CacheReadInputTokenCostPriority:     0.36e-6,
	}

	tiers := priceTiersFromLiteLLM("gemini-3-pro-preview", pricing, false, false)
	require.Len(t, tiers, 2)
	require.Equal(t, "Fast", tiers[1].Label)
	require.NotNil(t, tiers[1].Official.CacheWriteUSDPerM)
	require.InDelta(t, 3.6, *tiers[1].Official.CacheWriteUSDPerM, 1e-12)
}

func TestPriceTiersFromLiteLLMSynthesizesRuntimeFastLongContext(t *testing.T) {
	base := &service.LiteLLMModelPricing{
		InputCostPerToken:                   5e-6,
		InputCostPerTokenPriority:           10e-6,
		OutputCostPerToken:                  30e-6,
		OutputCostPerTokenPriority:          60e-6,
		CacheCreationInputTokenCost:         6.25e-6,
		CacheCreationInputTokenCostPriority: 12.5e-6,
		CacheReadInputTokenCost:             0.5e-6,
		CacheReadInputTokenCostPriority:     1e-6,
		LongContextInputTokenThreshold:      272000,
		LongContextInputCostMultiplier:      2,
		LongContextOutputCostMultiplier:     1.5,
	}

	t.Run("gpt-5.6 applies priority and long-context prices to cache too", func(t *testing.T) {
		tiers := priceTiersFromLiteLLM("gpt-5.6-sol", base, false, false)
		require.Len(t, tiers, 4)
		require.Equal(t, "Fast 长上下文", tiers[3].Label)
		require.InDelta(t, 20.0, *tiers[3].Official.InputUSDPerM, 1e-12)
		require.InDelta(t, 90.0, *tiers[3].Official.OutputUSDPerM, 1e-12)
		require.InDelta(t, 25.0, *tiers[3].Official.CacheWriteUSDPerM, 1e-12)
		require.InDelta(t, 2.0, *tiers[3].Official.CacheReadUSDPerM, 1e-12)
	})

	t.Run("gpt-5.4 fast-long keeps standard long-context rates", func(t *testing.T) {
		pricing := *base
		pricing.InputCostPerToken = 2.5e-6
		pricing.InputCostPerTokenPriority = 5e-6
		pricing.OutputCostPerToken = 15e-6
		pricing.OutputCostPerTokenPriority = 30e-6
		pricing.CacheCreationInputTokenCost = 0
		pricing.CacheCreationInputTokenCostPriority = 0
		pricing.CacheReadInputTokenCost = 0.25e-6
		pricing.CacheReadInputTokenCostPriority = 0.5e-6
		tiers := priceTiersFromLiteLLM("gpt-5.4", &pricing, false, false)
		require.Len(t, tiers, 4)
		require.InDelta(t, *tiers[1].Official.InputUSDPerM, *tiers[3].Official.InputUSDPerM, 1e-12)
		require.InDelta(t, *tiers[1].Official.OutputUSDPerM, *tiers[3].Official.OutputUSDPerM, 1e-12)
		require.InDelta(t, *tiers[1].Official.CacheReadUSDPerM, *tiers[3].Official.CacheReadUSDPerM, 1e-12)
	})
}

func TestPriceTiersFromLiteLLMUsesEffectiveModelPolicyAndCacheOnlyFastCapability(t *testing.T) {
	t.Run("legacy gpt-5-nano alias exposes all runtime priority floors", func(t *testing.T) {
		pricing := &service.LiteLLMModelPricing{
			InputCostPerToken:         0.05e-6,
			InputCostPerTokenPriority: 2.5e-6,
			OutputCostPerToken:        0.4e-6,
			CacheReadInputTokenCost:   0.005e-6,
		}
		tiers := priceTiersFromLiteLLM("openai/gpt-5-nano", pricing, false, false)
		require.Len(t, tiers, 2)
		require.Equal(t, "Fast", tiers[1].Label)
		require.InDelta(t, 5.0, *tiers[1].Official.InputUSDPerM, 1e-12)
		require.InDelta(t, 30.0, *tiers[1].Official.OutputUSDPerM, 1e-12)
		require.InDelta(t, 0.5, *tiers[1].Official.CacheReadUSDPerM, 1e-12)
	})

	t.Run("cache-write-only priority price creates fast tier", func(t *testing.T) {
		pricing := &service.LiteLLMModelPricing{
			InputCostPerToken:                   1e-6,
			OutputCostPerToken:                  2e-6,
			CacheCreationInputTokenCost:         1.25e-6,
			CacheCreationInputTokenCostPriority: 2e-6,
		}
		tiers := priceTiersFromLiteLLM("provider-cache-fast", pricing, false, false)
		require.Len(t, tiers, 2)
		require.Equal(t, "Fast", tiers[1].Label)
		require.InDelta(t, 2.0, *tiers[1].Official.CacheWriteUSDPerM, 1e-12)
	})
}
