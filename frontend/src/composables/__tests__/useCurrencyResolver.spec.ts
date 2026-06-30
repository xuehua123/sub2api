import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ref } from 'vue'
import { setActivePinia, createPinia } from 'pinia'

// ── Mock stores ──────────────────────────────────────────────────────

const mockEntitlements = ref<any[]>([])
const mockEntitlementsLoading = ref(false)
const mockLoading = ref(false)
const mockHasActiveSubscriptions = ref(false)
const mockFetchEntitlements = vi.fn().mockResolvedValue([])
const mockFetchActiveSubscriptions = vi.fn().mockResolvedValue([])
const mockCachedPublicSettings = ref<any>(null)

vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({
    get entitlements() { return mockEntitlements.value },
    get entitlementsLoading() { return mockEntitlementsLoading.value },
    get loading() { return mockLoading.value },
    get hasActiveSubscriptions() { return mockHasActiveSubscriptions.value },
    fetchEntitlements: mockFetchEntitlements,
    fetchActiveSubscriptions: mockFetchActiveSubscriptions,
  }),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    get cachedPublicSettings() { return mockCachedPublicSettings.value },
  }),
}))

// Import AFTER mocks are set up
import { useCurrencyResolver } from '../useCurrencyResolver'

// Helper: call composable outside setup context (skip onMounted)
function createResolver() {
  // We call it inside a fake app context so computed/ref work, but
  // onMounted won't fire in this test helper — that's fine, we control
  // the store data directly.
  return useCurrencyResolver()
}

describe('useCurrencyResolver', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    mockEntitlements.value = []
    mockEntitlementsLoading.value = false
    mockLoading.value = false
    mockHasActiveSubscriptions.value = false
    mockCachedPublicSettings.value = null
  })

  // ── globalFallbackRate ──────────────────────────────────────────

  describe('globalFallbackRate', () => {
    it('defaults to 0.068 when no public settings cached', () => {
      const { globalFallbackRate } = createResolver()
      expect(globalFallbackRate.value).toBe(0.068)
    })

    it('reads model_price_cny_per_quota_usd from public settings', () => {
      mockCachedPublicSettings.value = { model_price_cny_per_quota_usd: 0.1 }
      const { globalFallbackRate } = createResolver()
      expect(globalFallbackRate.value).toBe(0.1)
    })
  })

  // ── entitlementRateMap ──────────────────────────────────────────

  describe('entitlementRateMap', () => {
    it('maps entitlement id to unit_cost_per_usd when available', () => {
      mockEntitlements.value = [
        { id: 1, unit_cost_per_usd: 0.05, status: 'active' },
        { id: 2, unit_cost_per_usd: 0.08, status: 'expired' },
      ]
      const { entitlementRateMap } = createResolver()
      expect(entitlementRateMap.value.get(1)).toBe(0.05)
      expect(entitlementRateMap.value.get(2)).toBe(0.08)
    })

    it('derives rate from purchase_price / quota_usd when unit_cost_per_usd is missing', () => {
      mockEntitlements.value = [
        { id: 10, unit_cost_per_usd: null, purchase_price: 680, quota_usd: 10000, status: 'active' },
      ]
      const { entitlementRateMap } = createResolver()
      expect(entitlementRateMap.value.get(10)).toBeCloseTo(0.068, 6)
    })

    it('skips entitlements with zero or null rate data', () => {
      mockEntitlements.value = [
        { id: 20, unit_cost_per_usd: 0, purchase_price: null, quota_usd: null, status: 'active' },
        { id: 21, unit_cost_per_usd: null, purchase_price: 0, quota_usd: 0, status: 'active' },
      ]
      const { entitlementRateMap } = createResolver()
      expect(entitlementRateMap.value.size).toBe(0)
    })
  })

  // ── subscriptionRateMap ────────────────────────────────────────

  describe('subscriptionRateMap', () => {
    it('maps legacy_subscription_id to entitlement rate', () => {
      mockEntitlements.value = [
        { id: 1, legacy_subscription_id: 100, unit_cost_per_usd: 0.07, status: 'active' },
      ]
      const { subscriptionRateMap } = createResolver()
      expect(subscriptionRateMap.value.get(100)).toBe(0.07)
    })

    it('skips entitlements without legacy_subscription_id', () => {
      mockEntitlements.value = [
        { id: 1, legacy_subscription_id: null, unit_cost_per_usd: 0.07, status: 'active' },
      ]
      const { subscriptionRateMap } = createResolver()
      expect(subscriptionRateMap.value.size).toBe(0)
    })
  })

  // ── activeAverageRate ──────────────────────────────────────────

  describe('activeAverageRate', () => {
    it('computes weighted average from active entitlements', () => {
      mockEntitlements.value = [
        { id: 1, status: 'active', purchase_price: 680, quota_usd: 10000, unit_cost_per_usd: 0.068 },
        { id: 2, status: 'active', purchase_price: 200, quota_usd: 2000, unit_cost_per_usd: 0.1 },
      ]
      const { activeAverageRate } = createResolver()
      // (680 + 200) / (10000 + 2000) = 880 / 12000 ≈ 0.07333
      expect(activeAverageRate.value).toBeCloseTo(880 / 12000, 6)
    })

    it('excludes expired entitlements from average', () => {
      mockEntitlements.value = [
        { id: 1, status: 'expired', purchase_price: 680, quota_usd: 10000, unit_cost_per_usd: 0.068 },
        { id: 2, status: 'active', purchase_price: 200, quota_usd: 2000, unit_cost_per_usd: 0.1 },
      ]
      const { activeAverageRate } = createResolver()
      expect(activeAverageRate.value).toBeCloseTo(0.1, 6)
    })

    it('falls back to unit_cost_per_usd when purchase_price/quota_usd are missing', () => {
      mockEntitlements.value = [
        { id: 1, status: 'active', purchase_price: null, quota_usd: null, unit_cost_per_usd: 0.05 },
      ]
      const { activeAverageRate } = createResolver()
      expect(activeAverageRate.value).toBe(0.05)
    })

    it('falls back to global rate (680 package) when all subscriptions expired', () => {
      mockEntitlements.value = [
        { id: 1, status: 'expired', purchase_price: 680, quota_usd: 10000, unit_cost_per_usd: 0.068 },
      ]
      const { activeAverageRate } = createResolver()
      // No active entitlements → globalFallbackRate → 0.068
      expect(activeAverageRate.value).toBe(0.068)
    })

    it('falls back to global rate when no entitlements at all', () => {
      mockEntitlements.value = []
      const { activeAverageRate } = createResolver()
      expect(activeAverageRate.value).toBe(0.068)
    })

    it('uses custom global rate when configured', () => {
      mockCachedPublicSettings.value = { model_price_cny_per_quota_usd: 0.05 }
      mockEntitlements.value = []
      const { activeAverageRate } = createResolver()
      expect(activeAverageRate.value).toBe(0.05)
    })
  })

  // ── resolveRateForLog ──────────────────────────────────────────

  describe('resolveRateForLog', () => {
    it('returns entitlement-specific rate when entitlement_id matches', () => {
      mockEntitlements.value = [
        { id: 5, status: 'active', unit_cost_per_usd: 0.05, purchase_price: 500, quota_usd: 10000 },
      ]
      const { resolveRateForLog } = createResolver()
      expect(resolveRateForLog({ entitlement_id: 5, subscription_id: null })).toBe(0.05)
    })

    it('returns subscription-specific rate when subscription_id matches', () => {
      mockEntitlements.value = [
        { id: 1, legacy_subscription_id: 99, status: 'active', unit_cost_per_usd: 0.07 },
      ]
      const { resolveRateForLog } = createResolver()
      expect(resolveRateForLog({ entitlement_id: null, subscription_id: 99 })).toBe(0.07)
    })

    it('prefers entitlement_id over subscription_id', () => {
      mockEntitlements.value = [
        { id: 1, legacy_subscription_id: 99, status: 'active', unit_cost_per_usd: 0.07 },
        { id: 2, status: 'active', unit_cost_per_usd: 0.03 },
      ]
      const { resolveRateForLog } = createResolver()
      expect(resolveRateForLog({ entitlement_id: 2, subscription_id: 99 })).toBe(0.03)
    })

    it('falls back to activeAverageRate when no match', () => {
      mockEntitlements.value = [
        { id: 1, status: 'active', purchase_price: 680, quota_usd: 10000, unit_cost_per_usd: 0.068 },
      ]
      const { resolveRateForLog } = createResolver()
      // entitlement_id 999 doesn't exist → fallback to average
      expect(resolveRateForLog({ entitlement_id: 999, subscription_id: null })).toBe(0.068)
    })

    it('falls back to 680 package rate when no data at all', () => {
      mockEntitlements.value = []
      const { resolveRateForLog } = createResolver()
      expect(resolveRateForLog(null)).toBe(0.068)
      expect(resolveRateForLog(undefined)).toBe(0.068)
      expect(resolveRateForLog({ entitlement_id: null, subscription_id: null })).toBe(0.068)
    })
  })

  // ── convertUsdToCnyForLog ──────────────────────────────────────

  describe('convertUsdToCnyForLog', () => {
    it('converts using log-specific rate', () => {
      mockEntitlements.value = [
        { id: 1, status: 'active', unit_cost_per_usd: 0.05 },
      ]
      const { convertUsdToCnyForLog } = createResolver()
      expect(convertUsdToCnyForLog(10, { entitlement_id: 1 })).toBeCloseTo(0.5, 6)
    })

    it('returns null for null/undefined/NaN amounts', () => {
      const { convertUsdToCnyForLog } = createResolver()
      expect(convertUsdToCnyForLog(null)).toBeNull()
      expect(convertUsdToCnyForLog(undefined)).toBeNull()
      expect(convertUsdToCnyForLog(NaN)).toBeNull()
    })

    it('handles zero amount', () => {
      const { convertUsdToCnyForLog } = createResolver()
      expect(convertUsdToCnyForLog(0)).toBe(0)
    })
  })

  // ── convertUsdToCny ────────────────────────────────────────────

  describe('convertUsdToCny', () => {
    it('converts using activeAverageRate', () => {
      mockEntitlements.value = [
        { id: 1, status: 'active', purchase_price: 680, quota_usd: 10000 },
      ]
      const { convertUsdToCny } = createResolver()
      expect(convertUsdToCny(100)).toBeCloseTo(6.8, 4)
    })

    it('returns null for invalid amounts', () => {
      const { convertUsdToCny } = createResolver()
      expect(convertUsdToCny(null)).toBeNull()
      expect(convertUsdToCny(undefined)).toBeNull()
      expect(convertUsdToCny(Infinity)).toBeNull()
    })
  })

  // ── formatCny ──────────────────────────────────────────────────

  describe('formatCny', () => {
    it('formats normal amounts with ¥ prefix', () => {
      const { formatCny } = createResolver()
      expect(formatCny(1.5)).toBe('¥1.50')
    })

    it('uses up to 4 decimal places for small amounts', () => {
      const { formatCny } = createResolver()
      const result = formatCny(0.0123)
      expect(result).toMatch(/^¥0\.012/)
    })

    it('uses 6 decimal places for very small amounts', () => {
      const { formatCny } = createResolver()
      const result = formatCny(0.001234)
      expect(result).toMatch(/^¥0\.00123/)
    })

    it('returns "-" for null/undefined/NaN', () => {
      const { formatCny } = createResolver()
      expect(formatCny(null)).toBe('-')
      expect(formatCny(undefined)).toBe('-')
      expect(formatCny(NaN)).toBe('-')
    })

    it('formats zero correctly', () => {
      const { formatCny } = createResolver()
      expect(formatCny(0)).toBe('¥0.00')
    })
  })
})
