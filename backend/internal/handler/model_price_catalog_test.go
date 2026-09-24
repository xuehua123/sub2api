//go:build unit

package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCatalogPriceIsIndependentOfChannelGroupAndCustomPrice(t *testing.T) {
	h := &ModelPriceHandler{pricingService: stubModelPriceCatalog{prices: map[string]*service.LiteLLMModelPricing{"known": {InputCostPerToken: 0.000003, OutputCostPerToken: 0.000012}}}}
	channel := 0.000999
	for _, source := range []string{service.PricingSourceChannel, service.PricingSourceGroup} {
		dto := h.toModelPriceDTO(&modelAggregate{name: "known", platform: "openai", billingMode: "token", pricingSource: source, pricing: &service.ChannelModelPricing{InputPrice: &channel}}, modelPriceGroupDTO{EffectiveMultiplier: 0.2}, 7)
		require.NotNil(t, dto.CatalogPrice)
		require.Equal(t, 3.0, *dto.CatalogPrice.Price.InputUSDPerM)
		custom := 0.01
		dto = applyModelPriceCustomPrice(dto, service.ModelPriceCustomPrice{InputUSDPerM: &custom}, 7)
		require.Equal(t, 3.0, *dto.CatalogPrice.Price.InputUSDPerM)
	}
	require.Nil(t, h.catalogPriceForModel(&modelAggregate{name: "missing"}))
	require.Nil(t, h.catalogPriceForModel(&modelAggregate{name: "known", billingModelAmbiguous: true}))
	require.Equal(t, "known", h.catalogPriceForModel(&modelAggregate{name: "alias", billingModel: "known"}).Model)
}
