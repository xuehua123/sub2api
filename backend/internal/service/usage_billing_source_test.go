package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveUsageBillingSourceFromApplyResultPrefersPersistedSource(t *testing.T) {
	entitlementID := int64(42)
	newBalance := 10.0

	require.Equal(t, BillingSourceEntitlementQuota, ResolveUsageBillingSourceFromApplyResult(
		BillingTypeSubscription,
		nil,
		&entitlementID,
		&UsageBillingApplyResult{
			BillingSource: BillingSourceEntitlementQuota,
			NewBalance:    &newBalance,
		},
	))
	require.Equal(t, BillingSourceEntitlementBalanceFallback, ResolveUsageBillingSourceFromApplyResult(
		BillingTypeSubscription,
		nil,
		&entitlementID,
		&UsageBillingApplyResult{BillingSource: BillingSourceEntitlementBalanceFallback},
	))
	require.Equal(t, BillingSourceEntitlementBalanceFallback, ResolveUsageBillingSourceFromApplyResult(
		BillingTypeSubscription,
		nil,
		&entitlementID,
		&UsageBillingApplyResult{Applied: true, BillingSource: "invalid", NewBalance: &newBalance},
	))
	require.Empty(t, ResolveUsageBillingSourceFromApplyResult(
		BillingTypeSubscription,
		nil,
		&entitlementID,
		&UsageBillingApplyResult{Applied: false},
	))
}
