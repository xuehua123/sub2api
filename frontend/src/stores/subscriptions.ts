/**
 * Subscription Store
 * Global state management for user subscriptions with caching and deduplication
 */

import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import subscriptionsAPI from '@/api/subscriptions'
import type { UserEntitlement, UserSubscription } from '@/types'

// Cache TTL: 60 seconds
const CACHE_TTL_MS = 60_000

// Request generation counter to invalidate stale in-flight responses
let requestGeneration = 0
let entitlementRequestGeneration = 0

export const useSubscriptionStore = defineStore('subscriptions', () => {
  // State
  const activeSubscriptions = ref<UserSubscription[]>([])
  const entitlements = ref<UserEntitlement[]>([])
  const loading = ref(false)
  const entitlementsLoading = ref(false)
  const loaded = ref(false)
  const entitlementsLoaded = ref(false)
  const lastFetchedAt = ref<number | null>(null)
  const entitlementsLastFetchedAt = ref<number | null>(null)

  // In-flight request deduplication
  let activePromise: Promise<UserSubscription[]> | null = null
  let entitlementPromise: Promise<UserEntitlement[]> | null = null

  // Auto-refresh interval
  let pollerInterval: ReturnType<typeof setInterval> | null = null

  // Computed
  const hasActiveSubscriptions = computed(() => activeSubscriptions.value.length > 0)
  const hasEntitlements = computed(() => entitlements.value.length > 0)

  /**
   * Fetch active subscriptions with caching and deduplication
   * @param force - Force refresh even if cache is valid
   */
  async function fetchActiveSubscriptions(force = false): Promise<UserSubscription[]> {
    const now = Date.now()

    // Return cached data if valid
    if (
      !force &&
      loaded.value &&
      lastFetchedAt.value &&
      now - lastFetchedAt.value < CACHE_TTL_MS
    ) {
      return activeSubscriptions.value
    }

    // Return in-flight request if exists (deduplication)
    if (activePromise && !force) {
      return activePromise
    }

    const currentGeneration = ++requestGeneration

    // Start new request
    loading.value = true
    const requestPromise = subscriptionsAPI
      .getActiveSubscriptions()
      .then((data) => {
        if (currentGeneration === requestGeneration) {
          activeSubscriptions.value = data
          loaded.value = true
          lastFetchedAt.value = Date.now()
        }
        return data
      })
      .catch((error) => {
        console.error('Failed to fetch active subscriptions:', error)
        throw error
      })
      .finally(() => {
        if (activePromise === requestPromise) {
          loading.value = false
          activePromise = null
        }
      })

    activePromise = requestPromise

    return activePromise
  }

  /**
   * Fetch entitlement v2 records without changing legacy subscription state.
   * @param force - Force refresh even if cache is valid
   */
  async function fetchEntitlements(force = false): Promise<UserEntitlement[]> {
    const now = Date.now()

    if (
      !force &&
      entitlementsLoaded.value &&
      entitlementsLastFetchedAt.value &&
      now - entitlementsLastFetchedAt.value < CACHE_TTL_MS
    ) {
      return entitlements.value
    }

    if (entitlementPromise && !force) {
      return entitlementPromise
    }

    const currentGeneration = ++entitlementRequestGeneration
    entitlementsLoading.value = true
    const requestPromise = subscriptionsAPI
      .getEntitlements()
      .then((data) => {
        if (currentGeneration === entitlementRequestGeneration) {
          entitlements.value = data
          entitlementsLoaded.value = true
          entitlementsLastFetchedAt.value = Date.now()
        }
        return data
      })
      .catch((error) => {
        console.error('Failed to fetch entitlements:', error)
        throw error
      })
      .finally(() => {
        if (entitlementPromise === requestPromise) {
          entitlementsLoading.value = false
          entitlementPromise = null
        }
      })

    entitlementPromise = requestPromise

    return entitlementPromise
  }

  /**
   * Start auto-refresh polling 
   */
  function startPolling() {
    if (pollerInterval) return

    pollerInterval = setInterval(() => {
      fetchActiveSubscriptions(true).catch((error) => {
        console.error('Subscription polling failed:', error)
      })
      fetchEntitlements(true).catch((error) => {
        console.error('Entitlement polling failed:', error)
      })
    }, 5 * 60 * 1000)
  }

  /**
   * Stop auto-refresh polling
   */
  function stopPolling() {
    if (pollerInterval) {
      clearInterval(pollerInterval)
      pollerInterval = null
    }
  }

  /**
   * Clear all subscription data and stop polling
   */
  function clear() {
    requestGeneration++
    entitlementRequestGeneration++
    activePromise = null
    entitlementPromise = null
    loading.value = false
    entitlementsLoading.value = false
    activeSubscriptions.value = []
    entitlements.value = []
    loaded.value = false
    entitlementsLoaded.value = false
    lastFetchedAt.value = null
    entitlementsLastFetchedAt.value = null
    stopPolling()
  }

  /**
   * Invalidate cache (force next fetch to reload)
   */
  function invalidateCache() {
    lastFetchedAt.value = null
    entitlementsLastFetchedAt.value = null
  }

  return {
    // State
    activeSubscriptions,
    entitlements,
    loading,
    entitlementsLoading,
    hasActiveSubscriptions,
    hasEntitlements,

    // Actions
    fetchActiveSubscriptions,
    fetchEntitlements,
    startPolling,
    stopPolling,
    clear,
    invalidateCache
  }
})
