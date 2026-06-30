import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores/app'
import { useSubscriptionStore } from '@/stores/subscriptions'

export function useCurrencyResolver() {
  const appStore = useAppStore()
  const subStore = useSubscriptionStore()

  // Pre-fetch subscriptions and entitlements if they aren't loaded yet
  onMounted(() => {
    if (!subStore.entitlementsLoading && subStore.entitlements.length === 0) {
      subStore.fetchEntitlements().catch((err) =>
        console.warn('Failed to pre-fetch entitlements for currency conversion:', err)
      )
    }
    if (!subStore.loading && !subStore.hasActiveSubscriptions) {
      subStore.fetchActiveSubscriptions().catch((err) =>
        console.warn('Failed to pre-fetch active subscriptions for currency conversion:', err)
      )
    }
  })

  // Map entitlement ID -> unit_cost_per_usd
  const entitlementRateMap = computed(() => {
    const map = new Map<number, number>()
    for (const ent of subStore.entitlements) {
      if (ent.unit_cost_per_usd && ent.unit_cost_per_usd > 0) {
        map.set(ent.id, ent.unit_cost_per_usd)
      } else if (ent.purchase_price && ent.quota_usd && ent.quota_usd > 0) {
        map.set(ent.id, ent.purchase_price / ent.quota_usd)
      }
    }
    return map
  })

  // Map legacy subscription ID -> unit_cost_per_usd (via entitlements mapping)
  const subscriptionRateMap = computed(() => {
    const map = new Map<number, number>()
    for (const ent of subStore.entitlements) {
      if (ent.legacy_subscription_id) {
        const rate =
          ent.unit_cost_per_usd ||
          (ent.purchase_price && ent.quota_usd && ent.quota_usd > 0
            ? ent.purchase_price / ent.quota_usd
            : null)
        if (rate && rate > 0) {
          map.set(ent.legacy_subscription_id, rate)
        }
      }
    }
    return map
  })

  // 终极兜底汇率：取系统设置 model_price_cny_per_quota_usd，
  // 默认 0.068 = ¥680 套餐 / $10,000 额度。
  // 当用户订阅全部过期、或缓存无法匹配时，统一按此兜底。
  const globalFallbackRate = computed(() => {
    return appStore.cachedPublicSettings?.model_price_cny_per_quota_usd ?? 0.068
  })

  // Weighted average rate of all active entitlements/subscriptions
  const activeAverageRate = computed(() => {
    let totalCNY = 0
    let totalUSD = 0

    // Filter active/in_progress entitlements
    const activeEnts = subStore.entitlements.filter(
      (e) => e.status === 'active' || e.status === 'in_progress'
    )

    for (const ent of activeEnts) {
      const price = ent.purchase_price || 0
      const quota = ent.quota_usd || 0
      if (price > 0 && quota > 0) {
        totalCNY += price
        totalUSD += quota
      }
    }

    if (totalUSD > 0) {
      return totalCNY / totalUSD
    }

    // Fallback: check if there's any active entitlement rate
    for (const ent of activeEnts) {
      if (ent.unit_cost_per_usd && ent.unit_cost_per_usd > 0) {
        return ent.unit_cost_per_usd
      }
    }

    // Default to global fallback rate
    return globalFallbackRate.value
  })

  /**
   * Resolve rate for a specific request/usage log row
   */
  function resolveRateForLog(
    log?: { subscription_id?: number | null; entitlement_id?: number | null } | null
  ): number {
    if (log) {
      if (log.entitlement_id && entitlementRateMap.value.has(log.entitlement_id)) {
        return entitlementRateMap.value.get(log.entitlement_id)!
      }
      if (log.subscription_id && subscriptionRateMap.value.has(log.subscription_id)) {
        return subscriptionRateMap.value.get(log.subscription_id)!
      }
    }
    return activeAverageRate.value
  }

  /**
   * Convert USD amount to CNY based on specific log context
   */
  function convertUsdToCnyForLog(
    usdAmount: number | null | undefined,
    log?: { subscription_id?: number | null; entitlement_id?: number | null } | null
  ): number | null {
    if (usdAmount == null || !Number.isFinite(usdAmount)) return null
    return usdAmount * resolveRateForLog(log)
  }

  /**
   * Convert USD amount to CNY based on average active rate
   */
  function convertUsdToCny(usdAmount: number | null | undefined): number | null {
    if (usdAmount == null || !Number.isFinite(usdAmount)) return null
    return usdAmount * activeAverageRate.value
  }

  /**
   * Formats a CNY cost to 2 or 6 decimal places depending on magnitude, e.g. "¥0.0123"
   */
  function formatCny(amount: number | null | undefined, minDigits = 2, maxDigits = 4): string {
    if (amount == null || !Number.isFinite(amount)) return '-'
    const digits = amount > 0 && amount < 0.01 ? 6 : maxDigits
    return (
      '¥' +
      amount.toLocaleString('zh-CN', {
        minimumFractionDigits: minDigits,
        maximumFractionDigits: digits
      })
    )
  }

  return {
    entitlementRateMap,
    subscriptionRateMap,
    globalFallbackRate,
    activeAverageRate,
    resolveRateForLog,
    convertUsdToCnyForLog,
    convertUsdToCny,
    formatCny
  }
}
