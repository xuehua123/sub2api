//go:build unit

package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCatalogPresenceKeepsZerosWithoutChangingBillingValues(t *testing.T) {
	svc := &PricingService{}
	got, err := svc.parsePricingData([]byte(`{"zero":{"input_cost_per_token":0,"cache_read_input_token_cost":0},"image":{"output_cost_per_image_token":0.00003}}`))
	require.NoError(t, err)
	require.NotNil(t, got["zero"].CatalogPrices.Input)
	require.Zero(t, *got["zero"].CatalogPrices.Input)
	require.Nil(t, got["zero"].CatalogPrices.Output)
	require.NotNil(t, got["zero"].CatalogPrices.CacheRead)
	require.True(t, got["image"].TokenPricingAbsent)
	require.NotNil(t, got["image"].CatalogPrices.ImageOutput)
	require.InDelta(t, 0.00003, got["image"].OutputCostPerImageToken, 1e-12)
}
