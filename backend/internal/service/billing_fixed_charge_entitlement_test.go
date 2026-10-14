//go:build unit

package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type entitlementScopeCache struct {
	*memInflightCache
	scopeMu sync.Mutex
	scopes  map[string]map[string]float64
}

func newEntitlementScopeCache(balance float64) *entitlementScopeCache {
	return &entitlementScopeCache{memInflightCache: newMemInflightCache(balance), scopes: map[string]map[string]float64{}}
}

func (m *entitlementScopeCache) ReserveInflightScope(_ context.Context, scope, id string, amount float64, _ time.Duration) (float64, error) {
	m.scopeMu.Lock()
	defer m.scopeMu.Unlock()
	if m.scopes[scope] == nil {
		m.scopes[scope] = map[string]float64{}
	}
	pending := 0.0
	for _, cost := range m.scopes[scope] {
		pending += cost
	}
	m.scopes[scope][id] = amount
	return pending, nil
}

func (m *entitlementScopeCache) ReleaseInflightScope(_ context.Context, scope, id string) error {
	m.scopeMu.Lock()
	defer m.scopeMu.Unlock()
	delete(m.scopes[scope], id)
	return nil
}

func (m *entitlementScopeCache) scopeCount() int {
	m.scopeMu.Lock()
	defer m.scopeMu.Unlock()
	count := 0
	for _, requests := range m.scopes {
		count += len(requests)
	}
	return count
}

func fixedChargeEntitlement(limit, usage float64, policy string) *SubscriptionEntitlement {
	now := time.Now()
	return &SubscriptionEntitlement{
		ID: 91, UserID: 1, Status: SubscriptionStatusActive,
		StartsAt: now.Add(-time.Hour), ExpiresAt: now.Add(24 * time.Hour),
		MonthlyLimitUSD: &limit, MonthlyUsageUSD: usage, OveragePolicy: policy,
	}
}

func fixedChargeEntitlementLoader(ent *SubscriptionEntitlement) func(context.Context, int64) (*SubscriptionEntitlement, error) {
	return func(context.Context, int64) (*SubscriptionEntitlement, error) {
		copy := *ent
		return &copy, nil
	}
}

func TestReserveFixedChargeWithEntitlement_QuotaDoesNotRequireBalanceAndHoldsThroughBilling(t *testing.T) {
	cache := newEntitlementScopeCache(0)
	svc := newInflightSvc(t, cache, 60)
	ent := fixedChargeEntitlement(1, 0, SubscriptionEntitlementOverageBlock)
	res, err := svc.ReserveFixedChargeWithEntitlement(context.Background(), &User{ID: 1}, &APIKey{ID: 7, Quota: 2}, ent, 0.6, fixedChargeEntitlementLoader(ent))
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Zero(t, cache.count(), "entitlement quota must not reserve the balance")
	require.Equal(t, 2, cache.scopeCount())
	billingDone := res.Acquire()
	res.HandlerDone()
	require.Equal(t, 2, cache.scopeCount(), "pending billing retains entitlement and key admission")
	billingDone()
	billingDone()
	require.Zero(t, cache.scopeCount())
}

func TestReserveFixedChargeWithEntitlement_ConcurrentChargesCannotExceedQuota(t *testing.T) {
	cache := newEntitlementScopeCache(0)
	svc := newInflightSvc(t, cache, 60)
	ent := fixedChargeEntitlement(1, 0, SubscriptionEntitlementOverageBlock)
	loaded := make(chan struct{}, 2)
	continueLoad := make(chan struct{})
	loader := func(context.Context, int64) (*SubscriptionEntitlement, error) {
		loaded <- struct{}{}
		<-continueLoad
		copy := *ent
		return &copy, nil
	}
	type outcome struct {
		res *InflightReservation
		err error
	}
	results := make(chan outcome, 2)
	for range 2 {
		go func() {
			copy := *ent
			res, err := svc.ReserveFixedChargeWithEntitlement(context.Background(), &User{ID: 1}, nil, &copy, 0.6, loader)
			results <- outcome{res, err}
		}()
	}
	for range 2 {
		select {
		case <-loaded:
		case <-time.After(time.Second):
			t.Fatal("concurrent authoritative loads did not start")
		}
	}
	close(continueLoad)
	allowed, rejected := 0, 0
	var reservation *InflightReservation
	for range 2 {
		result := <-results
		if result.err == nil {
			allowed++
			reservation = result.res
		} else {
			rejected++
			require.ErrorIs(t, result.err, ErrSubscriptionEntitlementQuotaExceeded)
		}
	}
	require.Equal(t, 1, allowed)
	require.Equal(t, 1, rejected)
	require.Equal(t, 1, cache.scopeCount())
	reservation.HandlerDone()
	require.Zero(t, cache.scopeCount())
}

func TestReserveFixedChargeWithEntitlement_BalanceFallbackAndKeyLimitReleaseAllScopes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		balance float64
		quota   float64
		err     error
	}{
		{name: "fallback", balance: 1, quota: 2},
		{name: "insufficient balance", balance: 0.5, quota: 2, err: ErrInsufficientBalance},
		{name: "key still limits fallback", balance: 1, quota: 0.5, err: ErrAPIKeyQuotaExhausted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := newEntitlementScopeCache(tc.balance)
			svc := newInflightSvc(t, cache, 60)
			ent := fixedChargeEntitlement(1, 0.8, SubscriptionEntitlementOverageBalanceFallback)
			res, err := svc.ReserveFixedChargeWithEntitlement(context.Background(), &User{ID: 1}, &APIKey{ID: 7, Quota: tc.quota}, ent, 0.6, fixedChargeEntitlementLoader(ent))
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				require.Nil(t, res)
			} else {
				require.NoError(t, err)
				require.Equal(t, 1, cache.count(), "full fallback charge reserves balance")
				require.Equal(t, 2, cache.scopeCount())
				billingDone := res.Acquire()
				res.HandlerDone()
				require.Equal(t, 1, cache.count())
				require.Equal(t, 2, cache.scopeCount())
				billingDone()
				res.Release()
				require.Equal(t, int32(1), cache.releaseCt.Load())
			}
			require.Zero(t, cache.count())
			require.Zero(t, cache.scopeCount())
		})
	}
}

func TestReserveFixedChargeWithEntitlement_AuthoritativeFailuresReleaseScopes(t *testing.T) {
	for _, tc := range []struct {
		name string
		load func(context.Context, int64) (*SubscriptionEntitlement, error)
		err  error
	}{
		{name: "missing loader", err: ErrBillingServiceUnavailable},
		{name: "missing entitlement", load: func(context.Context, int64) (*SubscriptionEntitlement, error) { return nil, nil }, err: ErrSubscriptionEntitlementNotFound},
		{name: "revoked", load: func(context.Context, int64) (*SubscriptionEntitlement, error) {
			ent := fixedChargeEntitlement(1, 0, SubscriptionEntitlementOverageBlock)
			ent.Status = "cancelled"
			return ent, nil
		}, err: ErrSubscriptionEntitlementInactive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := newEntitlementScopeCache(0)
			svc := newInflightSvc(t, cache, 60)
			res, err := svc.ReserveFixedChargeWithEntitlement(context.Background(), &User{ID: 1}, nil, fixedChargeEntitlement(1, 0, SubscriptionEntitlementOverageBlock), 0.6, tc.load)
			require.ErrorIs(t, err, tc.err)
			require.Nil(t, res)
			require.Zero(t, cache.scopeCount())
		})
	}
}
