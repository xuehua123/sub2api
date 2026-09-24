//go:build unit

package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCatalogPreservesImageTokenOnlyPrices(t *testing.T) {
	p := &service.LiteLLMModelPricing{Mode: "image_generation", TokenPricingAbsent: true, InputCostPerImageToken: 0.000008, OutputCostPerImageToken: 0.00003}
	got := catalogPriceFromPricing("image-only-tokens", p)
	require.NotNil(t, got)
	require.Equal(t, "token", got.BillingMode)
	require.Nil(t, got.Price.InputUSDPerM)
	require.InDelta(t, 8, *got.Price.ImageInputUSDPerM, 1e-9)
	require.InDelta(t, 30, *got.Price.ImageOutputUSDPerM, 1e-9)
}
func TestCatalogDistinguishesExplicitZeroFromAbsent(t *testing.T) {
	zero := 0.0
	p := &service.LiteLLMModelPricing{CatalogPrices: &service.CatalogPricingFields{Input: &zero, CacheRead: &zero}}
	got := catalogPriceFromPricing("explicit-zero", p)
	require.NotNil(t, got)
	require.NotNil(t, got.Price.InputUSDPerM)
	require.Zero(t, *got.Price.InputUSDPerM)
	require.NotNil(t, got.Price.CacheReadUSDPerM)
	require.Zero(t, *got.Price.CacheReadUSDPerM)
	require.Nil(t, got.Price.OutputUSDPerM)
	require.Nil(t, got.Price.CacheWriteUSDPerM)
	require.Zero(t, *got.Tiers[0].Official.InputUSDPerM)
}
func TestCatalogKeepsFreeImagePriceAndType(t *testing.T) {
	zero := 0.0
	got := catalogPriceFromPricing("free-image", &service.LiteLLMModelPricing{TokenPricingAbsent: true, CatalogPrices: &service.CatalogPricingFields{PerImage: &zero}})
	require.NotNil(t, got)
	require.Equal(t, "image", got.BillingMode)
	require.Zero(t, *got.Price.PerRequestUSD)
}
func TestPublicAndAuthenticatedCatalogUseSamePrice(t *testing.T) {
	input := 0.000003
	raw := &service.LiteLLMModelPricing{CatalogPrices: &service.CatalogPricingFields{Input: &input}}
	handler := &ModelPriceHandler{pricingService: stubModelPriceCatalog{prices: map[string]*service.LiteLLMModelPricing{"model": raw}}}
	private := handler.catalogPriceForModel(&modelAggregate{name: "model"})
	public := toModelPlazaGroupDTO(&service.PlazaGroup{Models: []service.PlazaModel{{Name: "model", CatalogModel: "model", CatalogPricing: raw}}}, nil)
	require.Equal(t, private, public.Models[0].CatalogPrice)
}
