//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetModelPriceCustomPrice_PreservesExplicitZero(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, nil)
	zero := 0.0

	prices, err := svc.SetModelPriceCustomPrice(context.Background(), 46, "free-model", &ModelPriceCustomPrice{
		BillingMode:   string(BillingModeVideo),
		PerRequestUSD: &zero,
	})

	require.NoError(t, err)
	saved, ok := prices[ModelPriceCustomPriceKey(46, "free-model")]
	require.True(t, ok)
	require.NotNil(t, saved.PerRequestUSD)
	require.Zero(t, *saved.PerRequestUSD)
	require.True(t, saved.HasPrice())

	reloaded := svc.GetModelPriceCustomPrices(context.Background())
	require.NotNil(t, reloaded[ModelPriceCustomPriceKey(46, "free-model")].PerRequestUSD)
	require.Zero(t, *reloaded[ModelPriceCustomPriceKey(46, "free-model")].PerRequestUSD)
}
