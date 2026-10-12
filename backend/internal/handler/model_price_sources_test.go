//go:build unit

package handler

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func displayAccount(id int64, platform string, mapping map[string]any) service.Account {
	return service.Account{ID: id, Name: platform, Platform: platform, Status: service.StatusActive, Credentials: map[string]any{"model_mapping": mapping}}
}

func TestCompositeModelPricesRetainAccountPlatformsAndConfiguredOnlyCards(t *testing.T) {
	input := 0.000001
	group := modelPriceGroupDTO{ID: 80, Platform: service.PlatformComposite, EffectiveMultiplier: 0.5, modelPricing: []service.ChannelModelPricing{{Platform: service.PlatformComposite, Models: []string{"hy3"}, InputPrice: &input}, {Platform: service.PlatformComposite, Models: []string{"future-*"}, InputPrice: &input}}}
	accounts := []service.Account{
		displayAccount(1, service.PlatformOpenAI, map[string]any{"qwen3.8-max": "qwen3.8-max"}),
		displayAccount(2, service.PlatformDeepseek, map[string]any{"deepseek-v4.1-flash": "deepseek-v4.1-flash"}),
		displayAccount(3, service.PlatformAntigravity, map[string]any{"claude-sonnet-4-6": "claude-sonnet-4-6", "gemini-3-flash": "gemini-3-flash"}),
	}
	h := &ModelPriceHandler{}
	models := h.modelsForGroup(nil, accounts, group, 7, false, nil)
	byName := map[string]modelPriceModelDTO{}
	for _, model := range models {
		byName[model.Name] = model
	}
	require.Contains(t, byName, "qwen3.8-max")
	require.Equal(t, service.PlatformOpenAI, byName["qwen3.8-max"].Platform, "explicit account ownership wins over model-name detection")
	require.Equal(t, service.PlatformDeepseek, byName["deepseek-v4.1-flash"].Platform)
	require.Equal(t, service.PlatformAntigravity, byName["claude-sonnet-4-6"].Platform)
	require.Equal(t, "configured_only", byName["hy3"].Availability)
	require.Equal(t, service.PricingSourceGroup, byName["hy3"].PricingSource)
	require.Equal(t, 0.5, *byName["hy3"].Actual.InputUSDPerM)
	require.Nil(t, byName["hy3"].CatalogPrice)
	require.NotContains(t, byName, "future-*")
	groups := []modelPriceGroupDTO{group}
	applyModelPriceGroupUsage(groups, nil, map[int64][]service.Account{80: accounts}, map[string]struct{}{service.ModelPriceHiddenModelKey(80, "qwen3.8-max"): {}}, false)
	require.Equal(t, len(models)-1, groups[0].ModelCount)
}

func TestCompositeConflictingProviderClaimsHaveNoSingleAdvertisedPrice(t *testing.T) {
	accounts := []service.Account{displayAccount(1, service.PlatformOpenAI, map[string]any{"shared": "gpt-5.5"}), displayAccount(2, service.PlatformGrok, map[string]any{"shared": "grok-4.7"})}
	h := &ModelPriceHandler{pricingService: stubModelPriceCatalog{prices: map[string]*service.LiteLLMModelPricing{"gpt-5.5": {InputCostPerToken: 0.000005}, "grok-4.7": {InputCostPerToken: 0.000002}}}}
	models := h.modelsForGroup(nil, accounts, modelPriceGroupDTO{ID: 1, Platform: service.PlatformComposite, EffectiveMultiplier: 1}, 7, false, nil)
	require.Len(t, models, 1)
	require.Equal(t, "unknown", models[0].Availability)
	require.Nil(t, models[0].CatalogPrice)
	require.Nil(t, models[0].Official.InputUSDPerM)
}

func TestCompositeExplicitRouteRewritesDisplayAndEndpointConflictFailsClosed(t *testing.T) {
	group := modelPriceGroupDTO{ID: 80, Platform: service.PlatformComposite, EffectiveMultiplier: 1, routeDecisions: []service.CompositeRouteDecision{{PublicModel: "private-alias", TargetPlatform: service.PlatformGrok, UpstreamModel: "grok-4.7", Endpoint: service.CompositeRouteEndpointAny}}}
	h := &ModelPriceHandler{pricingService: stubModelPriceCatalog{prices: map[string]*service.LiteLLMModelPricing{"grok-4.7": {InputCostPerToken: 0.000002}}}}
	accounts := []service.Account{displayAccount(1, service.PlatformGrok, map[string]any{"grok-4.7": "grok-4.7"})}
	models := h.modelsForGroup(nil, accounts, group, 7, false, nil)
	require.Len(t, models, 2)
	require.Equal(t, service.PlatformGrok, models[1].Platform)
	require.Equal(t, "grok-4.7", models[1].CatalogPrice.Model)
	require.Equal(t, "configured_only", models[1].Availability)
	group.routeDecisions = append(group.routeDecisions, service.CompositeRouteDecision{PublicModel: "private-alias", TargetPlatform: service.PlatformOpenAI, UpstreamModel: "gpt-5.5", Endpoint: service.CompositeRouteEndpointResponses})
	models = h.modelsForGroup(nil, accounts, group, 7, false, nil)
	require.Nil(t, models[1].CatalogPrice)
	require.Nil(t, models[1].Official.InputUSDPerM)
}

func TestCompositeExplicitRouteWithAccountRemapDoesNotAdvertiseUpstreamPrice(t *testing.T) {
	group := modelPriceGroupDTO{ID: 80, Platform: service.PlatformComposite, EffectiveMultiplier: 1, routeDecisions: []service.CompositeRouteDecision{{PublicModel: "alias", TargetPlatform: service.PlatformOpenAI, UpstreamModel: "bridge", Endpoint: service.CompositeRouteEndpointAny}}}
	accounts := []service.Account{displayAccount(1, service.PlatformOpenAI, map[string]any{"alias": "gpt-5.5", "bridge": "gpt-6.1"})}
	h := &ModelPriceHandler{pricingService: stubModelPriceCatalog{prices: map[string]*service.LiteLLMModelPricing{"bridge": {InputCostPerToken: 0.000001}, "gpt-5.5": {InputCostPerToken: 0.000005}, "gpt-6.1": {InputCostPerToken: 0.000006}}}}
	models := h.modelsForGroup(nil, accounts, group, 7, false, nil)
	require.Equal(t, "alias", models[0].Name)
	require.Equal(t, "unknown", models[0].Availability)
	require.Nil(t, models[0].CatalogPrice)
	require.Nil(t, models[0].Actual.InputUSDPerM)
	models = h.modelsForGroup(nil, nil, group, 7, false, nil)
	require.Len(t, models, 1)
	require.Equal(t, "unknown", models[0].Availability)
	require.Nil(t, models[0].CatalogPrice, "a configured route alone cannot prove the final account billing projection")
}

func TestCatalogProviderRegistryAndKnownGrokAliases(t *testing.T) {
	for _, spec := range domain.Platforms() {
		if spec.LiteLLMProvider != "" {
			require.Contains(t, catalogProvidersForPlatform(spec.ID), spec.LiteLLMProvider)
		}
	}
	require.Contains(t, catalogProvidersForPlatform(service.PlatformComposite), "xai")
	h := &ModelPriceHandler{pricingService: stubModelPriceCatalog{prices: map[string]*service.LiteLLMModelPricing{"grok-4.6": {InputCostPerToken: 0.000002}, "grok-4.7": {InputCostPerToken: 0.000002}}}}
	require.Equal(t, "grok-4.7", h.catalogPriceForModel(&modelAggregate{name: "grok-4.7-latest", platform: service.PlatformGrok}).Model)
	require.Nil(t, h.catalogPriceForModel(&modelAggregate{name: "grok", platform: service.PlatformGrok}))
	require.Nil(t, h.catalogPriceForModel(&modelAggregate{name: "grok-latest", platform: service.PlatformGrok}))
	require.Nil(t, h.catalogPriceForModel(&modelAggregate{name: "grok-unknown-future", platform: service.PlatformGrok}))
	require.Nil(t, h.catalogPriceForModel(&modelAggregate{name: "gpt-reserve", platform: service.PlatformOpenAI}))
}

func TestGrokConfiguredMediaPricesKeepUnitsZeroAndTierPrecedence(t *testing.T) {
	zero, flat, override := 0.0, 0.2, 0.4
	group := modelPriceGroupDTO{EffectiveMultiplier: 0.5, ImageRateIndependent: true, ImageRateMultiplier: 0.2, VideoRateIndependent: true, VideoRateMultiplier: 0.3, mediaGroup: &service.Group{ImagePrice1K: &zero, ImagePrice2K: &flat, VideoPrice480P: &flat, VideoPrice720P: &flat, VideoModelPrices: map[string]map[string]float64{service.VideoPriceFamilyGrokImagineVideo15: {"720p": override}}}}
	image := modelPriceConfiguredMedia(modelPriceModelDTO{}, &modelAggregate{platform: service.PlatformGrok, billingModel: "grok-imagine-image"}, group, 7)
	require.Equal(t, "image", image.BillingMode)
	require.Nil(t, image.CatalogPrice)
	require.Len(t, image.PriceTiers, 2)
	require.Equal(t, "2K（默认）", image.PriceTiers[0].Label)
	require.Equal(t, "base", image.PriceTiers[0].Key)
	require.InDelta(t, 0.04, *image.PriceTiers[0].Actual.PerRequestUSD, 1e-9)
	require.Zero(t, *image.PriceTiers[1].Actual.PerRequestUSD)
	video := modelPriceConfiguredMedia(modelPriceModelDTO{}, &modelAggregate{platform: service.PlatformGrok, billingModel: "grok-imagine-video-1.5-preview"}, group, 7)
	require.Equal(t, "video", video.BillingMode)
	require.Len(t, video.PriceTiers, 2)
	require.InDelta(t, 0.12, *video.PriceTiers[1].Actual.PerRequestUSD, 1e-9)
	require.Equal(t, "720p", video.PriceTiers[1].Label)
	unknown := modelPriceConfiguredMedia(modelPriceModelDTO{}, &modelAggregate{platform: service.PlatformGrok, billingModel: "grok-imagine-image"}, modelPriceGroupDTO{mediaGroup: &service.Group{}}, 7)
	require.Nil(t, unknown.Official.PerRequestUSD)
	require.Equal(t, "image", unknown.BillingMode)
}

func TestModelsForGroupGrokMediaPreservesRuntimeGroupCardPriority(t *testing.T) {
	cardPrice, flatPrice, tokenPrice := 0.8, 0.2, 0.000001
	for _, tc := range []struct {
		name string
		mode service.BillingMode
	}{{"grok-imagine-image", service.BillingModeImage}, {"grok-imagine-image", service.BillingModePerRequest}, {"grok-imagine-video-1.5-preview", service.BillingModeVideo}} {
		t.Run(tc.name+string(tc.mode), func(t *testing.T) {
			group := modelPriceGroupDTO{ID: 1, Platform: service.PlatformGrok, EffectiveMultiplier: 0.5, ImageRateIndependent: true, ImageRateMultiplier: 0.2, VideoRateIndependent: true, VideoRateMultiplier: 0.3, mediaGroup: &service.Group{ImagePrice1K: &flatPrice, VideoPrice480P: &flatPrice}, modelPricing: []service.ChannelModelPricing{{Models: []string{tc.name}, BillingMode: tc.mode, PerRequestPrice: &cardPrice}}}
			h := &ModelPriceHandler{pricingService: stubModelPriceCatalog{prices: map[string]*service.LiteLLMModelPricing{tc.name: {InputCostPerToken: tokenPrice}}}}
			models := h.modelsForGroup(nil, []service.Account{displayAccount(1, service.PlatformGrok, map[string]any{tc.name: tc.name})}, group, 7, false, nil)
			require.Len(t, models, 1)
			model := models[0]
			require.Equal(t, service.PricingSourceGroup, model.PricingSource)
			require.Equal(t, cardPrice, *model.Official.PerRequestUSD)
			require.Nil(t, model.Official.InputUSDPerM)
			require.Nil(t, model.Actual.InputUSDPerM)
			if tc.mode == service.BillingModeVideo {
				require.Equal(t, "video", model.BillingMode)
				require.InDelta(t, 0.24, *model.Actual.PerRequestUSD, 1e-9)
			} else {
				require.Equal(t, "image", model.BillingMode)
				require.InDelta(t, 0.16, *model.Actual.PerRequestUSD, 1e-9)
			}
		})
	}
}

func TestModelsForGroupGrokMediaMissingCardClearsTextFallback(t *testing.T) {
	for _, name := range []string{"grok-imagine", "grok-imagine-1", "grok-imagine-image", "grok-video-1.5", "grok-imagine-video-1.5-preview"} {
		t.Run(name, func(t *testing.T) {
			h := &ModelPriceHandler{pricingService: stubModelPriceCatalog{prices: map[string]*service.LiteLLMModelPricing{name: {InputCostPerToken: 0.000001}}}}
			models := h.modelsForGroup(nil, []service.Account{displayAccount(1, service.PlatformGrok, map[string]any{name: name})}, modelPriceGroupDTO{ID: 1, Platform: service.PlatformGrok, EffectiveMultiplier: 1}, 7, false, nil)
			require.Len(t, models, 1)
			require.NotEqual(t, "token", models[0].BillingMode)
			require.True(t, models[0].OfficialMissing)
			require.Nil(t, models[0].Official.InputUSDPerM)
			require.Nil(t, models[0].Actual.InputUSDPerM)
			require.Empty(t, models[0].PriceTiers)
			encoded, err := json.Marshal(models[0])
			require.NoError(t, err)
			require.Contains(t, string(encoded), `"price_tiers":[]`)
		})
	}
}

func TestModelsForGroupGrokMediaChannelFillsOnlyUnconfiguredImageTiers(t *testing.T) {
	flatPrice, channelPrice := 0.2, 0.8
	channels := []service.AvailableChannel{{ID: 1, Name: "media", Status: service.StatusActive, Groups: []service.AvailableGroupRef{{ID: 1}}, SupportedModels: []service.SupportedModel{{Name: "grok-imagine-image", Platform: service.PlatformGrok, BillingModel: "grok-imagine-image", PricingSource: service.PricingSourceChannel, Pricing: &service.ChannelModelPricing{BillingMode: service.BillingModeImage, PerRequestPrice: &channelPrice}}}}}
	models := (&ModelPriceHandler{}).modelsForGroup(channels, nil, modelPriceGroupDTO{ID: 1, Platform: service.PlatformGrok, EffectiveMultiplier: 1, mediaGroup: &service.Group{ImagePrice1K: &flatPrice}}, 7, false, nil)
	require.Len(t, models, 1)
	require.Len(t, models[0].PriceTiers, 3)
	require.Equal(t, "2K（默认）", models[0].PriceTiers[0].Label)
	require.Equal(t, channelPrice, *models[0].PriceTiers[0].Actual.PerRequestUSD)
	require.Equal(t, flatPrice, *models[0].PriceTiers[1].Actual.PerRequestUSD)
	require.Equal(t, channelPrice, *models[0].PriceTiers[2].Actual.PerRequestUSD)
}

func TestGrokMediaBaseUsesOnlyRuntimeDefaultResolution(t *testing.T) {
	price := 0.2
	group := modelPriceGroupDTO{EffectiveMultiplier: 1, mediaGroup: &service.Group{ImagePrice1K: &price, VideoPrice720P: &price}}
	for _, tc := range []struct {
		model, label string
	}{{"grok-imagine-image", "2K（默认）"}, {"grok-imagine-video-1.5-preview", "480p（默认）"}} {
		model := modelPriceConfiguredMedia(modelPriceModelDTO{}, &modelAggregate{platform: service.PlatformGrok, billingModel: tc.model}, group, 7)
		require.Nil(t, model.Official.PerRequestUSD, "another resolution cannot stand in for the runtime default")
		require.Nil(t, model.Actual.PerRequestUSD)
		require.True(t, model.OfficialMissing)
		require.Equal(t, "base", model.PriceTiers[0].Key)
		require.Equal(t, tc.label, model.PriceTiers[0].Label)
		require.Len(t, model.PriceTiers, 2)
		require.Equal(t, price, *model.PriceTiers[1].Actual.PerRequestUSD)
	}
}

func TestCompositeDirectAnthropicFastUsesActualProviderCapability(t *testing.T) {
	model := "claude-opus-5"
	account := displayAccount(1, service.PlatformAnthropic, map[string]any{model: model})
	account.Schedulable = true
	account.Type = service.AccountTypeAPIKey
	h := &ModelPriceHandler{pricingService: stubModelPriceCatalog{prices: map[string]*service.LiteLLMModelPricing{model: {InputCostPerToken: 0.000005, OutputCostPerToken: 0.000025}}}}
	for _, platform := range []string{service.PlatformAnthropic, service.PlatformComposite} {
		models := h.modelsForGroup(nil, []service.Account{account}, modelPriceGroupDTO{ID: 1, Platform: platform, EffectiveMultiplier: 1}, 7, false, nil)
		require.Len(t, models, 1)
		require.Len(t, models[0].PriceTiers, 2)
		require.Equal(t, "Fast", models[0].PriceTiers[1].Label)
	}
	account.Platform = service.PlatformAntigravity
	models := h.modelsForGroup(nil, []service.Account{account}, modelPriceGroupDTO{ID: 1, Platform: service.PlatformComposite, EffectiveMultiplier: 1}, 7, false, nil)
	for _, row := range models {
		if row.Name == model {
			require.Len(t, row.PriceTiers, 1)
			return
		}
	}
	t.Fatal("Antigravity row was not retained")
}

func TestGrokMediaMissingTierUsesRuntimeContextBoundaryAndDefaultPrice(t *testing.T) {
	base, oneK := 0.6, 0.2
	pricing := &service.ChannelModelPricing{PerRequestPrice: &base, Intervals: []service.PricingInterval{{TierLabel: "1K", MinTokens: 0, PerRequestPrice: &oneK}}}
	require.Equal(t, oneK, *modelPriceMediaTierPrice(pricing, "1K"))
	require.Equal(t, base, *modelPriceMediaTierPrice(pricing, "2K"), "zero context does not match a tier beginning at zero")
	require.Equal(t, base, *modelPriceMediaTierPrice(pricing, "4K"))
}

func TestModelsForGroupGrokMediaExplicitTokenCardKeepsRuntimeBilling(t *testing.T) {
	input, output, flat := 0.000002, 0.000006, 0.2
	for _, name := range []string{"grok-imagine-image", "grok-imagine-video-1.5-preview"} {
		for _, source := range []string{service.PricingSourceGroup, service.PricingSourceChannel} {
			t.Run(name+source, func(t *testing.T) {
				card := service.ChannelModelPricing{Models: []string{name}, BillingMode: service.BillingModeToken, InputPrice: &input, OutputPrice: &output}
				group := modelPriceGroupDTO{ID: 1, Platform: service.PlatformGrok, EffectiveMultiplier: 0.5, mediaGroup: &service.Group{ImagePrice2K: &flat, VideoPrice480P: &flat}}
				var channels []service.AvailableChannel
				var accounts []service.Account
				if source == service.PricingSourceGroup {
					group.modelPricing = []service.ChannelModelPricing{card}
					accounts = []service.Account{displayAccount(1, service.PlatformGrok, map[string]any{name: name})}
				} else {
					channels = []service.AvailableChannel{{ID: 1, Name: "explicit-token", Status: service.StatusActive, Groups: []service.AvailableGroupRef{{ID: 1}}, SupportedModels: []service.SupportedModel{{Name: name, Platform: service.PlatformGrok, BillingModel: name, Pricing: &card, PricingSource: source}}}}
				}
				models := (&ModelPriceHandler{}).modelsForGroup(channels, accounts, group, 7, false, nil)
				require.Len(t, models, 1)
				require.Equal(t, "token", models[0].BillingMode)
				require.Equal(t, source, models[0].PricingSource)
				require.Equal(t, 1.0, *models[0].Actual.InputUSDPerM)
				require.Equal(t, 3.0, *models[0].Actual.OutputUSDPerM)
				require.Nil(t, models[0].Actual.PerRequestUSD)
			})
		}
	}
}
