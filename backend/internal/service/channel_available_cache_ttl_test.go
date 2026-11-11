//go:build unit

package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

// The fork builds separate effective price cards and Fast/long-context tiers.
// Every projection must retain the new OpenAI TTL rate, including explicit zero.
func TestCacheWriteTTLDisplayProjection(t *testing.T) {
	for _, longRate := range []float64{0, 5e-6} {
		t.Run(map[bool]string{true: "free", false: "paid"}[longRate == 0], func(t *testing.T) {
			pricing := &ModelPricing{
				InputPricePerToken: 2e-6, OutputPricePerToken: 10e-6,
				CacheCreationPricePerToken: 2.5e-6, CacheCreation12hPricePerToken: &longRate,
				LongContextInputThreshold: 100, LongContextInputMultiplier: 3,
				LongContextOutputMultiplier: 2,
			}
			card := effectiveTokenPricingForDisplay(pricing, false)
			require.NotNil(t, card.CacheWrite12hPrice)
			require.Equal(t, longRate, *card.CacheWrite12hPrice)
			tiers := groupTokenIntervalsForDisplay("gpt-6-luna", card, nil, pricing, true)
			require.Len(t, tiers, 4)
			tokens := UsageTokens{CacheCreationTokens: 1_000_000, CacheCreation12hTokens: 1_000_000}
			for i, tc := range []struct {
				tier string
				long bool
			}{{"", false}, {"", true}, {"priority", false}, {"priority", true}} {
				require.NotNil(t, tiers[i].CacheWrite12hPrice)
				cost := (&BillingService{}).computeTokenBreakdown("gpt-6-luna", pricing, tokens, 1, tc.tier, tc.long)
				require.InDelta(t, cost.CacheCreationCost, *tiers[i].CacheWrite12hPrice*1e6, 1e-12)
			}
			require.Equal(t, longRate, *pricing.CacheCreation12hPricePerToken, "display must not mutate runtime pricing")
		})
	}
}

func TestCacheWriteTTLDisplayPriorityCatalogRatio(t *testing.T) {
	longRate := 5e-6
	pricing := &ModelPricing{
		InputPricePerToken: 2e-6, InputPricePerTokenPriority: 6e-6,
		CacheCreationPricePerToken: 2.5e-6, CacheCreationPricePerTokenPriority: 10e-6,
		CacheCreation12hPricePerToken: &longRate,
	}
	card := effectiveTokenPricingForDisplay(pricing, false)
	base := PricingInterval{InputPrice: card.InputPrice, CacheWritePrice: card.CacheWritePrice, CacheWrite12hPrice: card.CacheWrite12hPrice}
	tier := fastDisplayTier("custom-model", base, pricing, "Fast", 1)
	require.NotNil(t, tier.CacheWrite12hPrice)
	cost := (&BillingService{}).computeTokenBreakdown("custom-model", pricing,
		UsageTokens{CacheCreationTokens: 1_000_000, CacheCreation12hTokens: 1_000_000}, 1, "priority", false)
	require.InDelta(t, cost.CacheCreationCost, *tier.CacheWrite12hPrice*1e6, 1e-12)
}

func TestCacheWriteTTLDisplayOverrides(t *testing.T) {
	short, long, free := 2e-6, 5e-6, 0.0
	base := &ChannelModelPricing{BillingMode: BillingModeToken, CacheWritePrice: &short, CacheWrite12hPrice: &long}
	for _, configured := range []*ChannelModelPricing{
		{CacheWritePrice: &short},
		{CacheWrite12hPrice: &free},
		{CacheWritePrice: &short, CacheWrite12hPrice: &long},
	} {
		effective := cloneChannelModelPricingForDisplay(base)
		overlayConfiguredPrices(effective, configured)
		runtime := groupRuntimeTokenPricingForDisplay(effective, configured, nil)
		fallback := overlayTokenPricingForDisplay(&ModelPricing{CacheCreation12hPricePerToken: &long}, configured)
		for _, got := range []*float64{effective.CacheWrite12hPrice, runtime.CacheCreation12hPricePerToken, fallback.CacheCreation12hPricePerToken} {
			if configured.CacheWrite12hPrice == nil {
				require.Nil(t, got, "legacy flat overrides must not inherit the catalog 12h premium")
			} else {
				require.NotNil(t, got)
				require.Equal(t, *configured.CacheWrite12hPrice, *got)
			}
		}
		projectEffectiveCacheBreakdownForDisplay(effective, runtime)
		require.Equal(t, runtime.CacheCreation12hPricePerToken, effective.CacheWrite12hPrice)
	}
	require.Equal(t, long, *base.CacheWrite12hPrice)
}
