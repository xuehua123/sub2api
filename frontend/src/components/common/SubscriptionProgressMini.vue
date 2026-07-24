<template>
  <div v-if="hasActiveSubscriptions" class="relative" ref="containerRef">
    <!-- Mini Progress Display -->
    <button
      @click="toggleTooltip"
      class="flex cursor-pointer items-center gap-2 rounded-xl bg-purple-50 px-3 py-1.5 transition-colors hover:bg-purple-100 dark:bg-purple-900/20 dark:hover:bg-purple-900/30"
      :title="t('subscriptionProgress.viewDetails')"
    >
      <Icon name="creditCard" size="sm" class="text-purple-600 dark:text-purple-400" />
      <div class="flex items-center gap-1.5">
        <!-- Combined progress indicator -->
        <div class="flex items-center gap-0.5">
          <div
            v-for="(sub, index) in displaySubscriptions.slice(0, 3)"
            :key="index"
            class="h-2 w-2 rounded-full"
            :class="getProgressDotClass(sub)"
          ></div>
        </div>
        <span class="text-xs font-medium text-purple-700 dark:text-purple-300">
          {{ displaySubscriptions.length }}
        </span>
      </div>
    </button>

    <!-- Hover/Click Tooltip -->
    <transition name="dropdown">
      <div
        v-if="tooltipOpen"
        class="absolute right-0 z-50 mt-2 w-[340px] overflow-hidden rounded-xl border border-gray-200 bg-white shadow-xl dark:border-dark-700 dark:bg-dark-800"
      >
        <div class="border-b border-gray-100 p-3 dark:border-dark-700">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('subscriptionProgress.title') }}
          </h3>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
            {{ t('subscriptionProgress.activeCount', { count: displaySubscriptions.length }) }}
          </p>
        </div>

        <div class="max-h-64 overflow-y-auto">
          <div
            v-for="subscription in displaySubscriptions"
            :key="subscription.id"
            class="border-b border-gray-50 p-3 last:border-b-0 dark:border-dark-700/50"
          >
            <div class="mb-2 flex items-center justify-between">
              <span class="text-sm font-medium text-gray-900 dark:text-white">
                {{ subscriptionDisplayName(subscription, subscriptionPlans) }}
              </span>
              <span
                v-if="subscription.expires_at"
                class="text-xs"
                :class="getDaysRemainingClass(subscription.expires_at)"
              >
                {{ formatDaysRemaining(subscription.expires_at) }}
              </span>
            </div>

            <!-- Progress bars or Unlimited badge -->
            <div class="space-y-1.5">
              <!-- Unlimited subscription badge -->
              <div
                v-if="isUnlimited(subscription)"
                class="flex items-center gap-2 rounded-lg bg-gradient-to-r from-emerald-50 to-teal-50 px-2.5 py-1.5 dark:from-emerald-900/20 dark:to-teal-900/20"
              >
                <span class="text-lg text-emerald-600 dark:text-emerald-400">∞</span>
                <span class="text-xs font-medium text-emerald-700 dark:text-emerald-300">
                  {{ t('subscriptionProgress.unlimited') }}
                </span>
              </div>

              <!-- Progress bars for limited subscriptions -->
              <template v-else>
                <div v-if="subscriptionDailyLimit(subscription)" class="flex items-center gap-2">
                  <span class="w-8 flex-shrink-0 text-[10px] text-gray-500">{{
                    t('subscriptionProgress.daily')
                  }}</span>
                  <div class="h-1.5 min-w-0 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="
                        getProgressBarClass(
                          subscription.daily_usage_usd,
                          subscriptionDailyLimit(subscription)
                        )
                      "
                      :style="{
                        width: getProgressWidth(
                          subscription.daily_usage_usd,
                          subscriptionDailyLimit(subscription)
                        )
                      }"
                    ></div>
                  </div>
                  <span class="w-24 flex-shrink-0 text-right text-[10px] text-gray-500">
                    {{
                      formatUsage(subscription.daily_usage_usd, subscriptionDailyLimit(subscription))
                    }}
                  </span>
                </div>

                <div v-if="subscriptionWeeklyLimit(subscription)" class="flex items-center gap-2">
                  <span class="w-8 flex-shrink-0 text-[10px] text-gray-500">{{
                    t('subscriptionProgress.weekly')
                  }}</span>
                  <div class="h-1.5 min-w-0 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="
                        getProgressBarClass(
                          subscription.weekly_usage_usd,
                          subscriptionWeeklyLimit(subscription)
                        )
                      "
                      :style="{
                        width: getProgressWidth(
                          subscription.weekly_usage_usd,
                          subscriptionWeeklyLimit(subscription)
                        )
                      }"
                    ></div>
                  </div>
                  <span class="w-24 flex-shrink-0 text-right text-[10px] text-gray-500">
                    {{
                      formatUsage(subscription.weekly_usage_usd, subscriptionWeeklyLimit(subscription))
                    }}
                  </span>
                </div>

                <div v-if="subscriptionMonthlyLimit(subscription)" class="flex items-center gap-2">
                  <span class="w-8 flex-shrink-0 text-[10px] text-gray-500">{{
                    t('subscriptionProgress.monthly')
                  }}</span>
                  <div class="h-1.5 min-w-0 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="
                        getProgressBarClass(
                          subscription.monthly_usage_usd,
                          subscriptionMonthlyLimit(subscription)
                        )
                      "
                      :style="{
                        width: getProgressWidth(
                          subscription.monthly_usage_usd,
                          subscriptionMonthlyLimit(subscription)
                        )
                      }"
                    ></div>
                  </div>
                  <span class="w-24 flex-shrink-0 text-right text-[10px] text-gray-500">
                    {{
                      formatUsage(
                        subscription.monthly_usage_usd,
                        subscriptionMonthlyLimit(subscription)
                      )
                    }}
                  </span>
                </div>
              </template>
            </div>
          </div>
        </div>

        <div class="border-t border-gray-100 p-2 dark:border-dark-700">
          <router-link
            to="/subscriptions"
            @click="closeTooltip"
            class="block w-full py-1 text-center text-xs text-primary-600 hover:underline dark:text-primary-400"
          >
            {{ t('subscriptionProgress.viewAll') }}
          </router-link>
        </div>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useSubscriptionStore } from '@/stores'
import { paymentAPI } from '@/api/payment'
import type { UserSubscription } from '@/types'
import type { SubscriptionPlan } from '@/types/payment'
import { formatRemainingDurationCompact, getRemainingHours } from '@/utils/subscriptionTime'
import {
  activeSubscriptionDisplayRecords,
  normalizeSubscriptionPlans,
  subscriptionDisplayName,
} from '@/utils/subscriptionPlanDisplay'

const { t } = useI18n()

const subscriptionStore = useSubscriptionStore()

const containerRef = ref<HTMLElement | null>(null)
const tooltipOpen = ref(false)
const subscriptionPlans = ref<SubscriptionPlan[]>([])

// Use store data instead of local state
const activeSubscriptions = computed(() => subscriptionStore.activeSubscriptions)

const displaySubscriptions = computed(() => {
  // Sort by most usage (highest percentage first)
  return activeSubscriptionDisplayRecords(
    activeSubscriptions.value,
    subscriptionStore.entitlements,
  ).sort((a, b) => {
    const aMax = getMaxUsagePercentage(a)
    const bMax = getMaxUsagePercentage(b)
    return bMax - aMax
  })
})
const hasActiveSubscriptions = computed(() => displaySubscriptions.value.length > 0)

function getMaxUsagePercentage(sub: UserSubscription): number {
  const percentages: number[] = []
  const dailyLimit = subscriptionDailyLimit(sub)
  const weeklyLimit = subscriptionWeeklyLimit(sub)
  const monthlyLimit = subscriptionMonthlyLimit(sub)
  if (dailyLimit) {
    percentages.push(((sub.daily_usage_usd || 0) / dailyLimit) * 100)
  }
  if (weeklyLimit) {
    percentages.push(((sub.weekly_usage_usd || 0) / weeklyLimit) * 100)
  }
  if (monthlyLimit) {
    percentages.push(((sub.monthly_usage_usd || 0) / monthlyLimit) * 100)
  }
  return percentages.length > 0 ? Math.max(...percentages) : 0
}

function isUnlimited(sub: UserSubscription): boolean {
  return (
    !subscriptionDailyLimit(sub) &&
    !subscriptionWeeklyLimit(sub) &&
    !subscriptionMonthlyLimit(sub)
  )
}

function positiveLimit(value: number | null | undefined): number | null {
  return typeof value === 'number' && value > 0 ? value : null
}

function isEntitlementBackedSubscription(sub: UserSubscription): boolean {
  return sub.entitlement_only === true || sub.entitlement_id != null || sub.plan_id != null
}

function subscriptionDailyLimit(sub: UserSubscription): number | null {
  const subscriptionLimit = positiveLimit(sub.daily_limit_usd)
  if (isEntitlementBackedSubscription(sub)) return subscriptionLimit
  return subscriptionLimit ?? positiveLimit(sub.group?.daily_limit_usd)
}

function subscriptionWeeklyLimit(sub: UserSubscription): number | null {
  const subscriptionLimit = positiveLimit(sub.weekly_limit_usd)
  if (isEntitlementBackedSubscription(sub)) return subscriptionLimit
  return subscriptionLimit ?? positiveLimit(sub.group?.weekly_limit_usd)
}

function subscriptionMonthlyLimit(sub: UserSubscription): number | null {
  const subscriptionLimit = positiveLimit(sub.monthly_limit_usd)
  if (isEntitlementBackedSubscription(sub)) return subscriptionLimit
  return subscriptionLimit ?? positiveLimit(sub.group?.monthly_limit_usd)
}

function getProgressDotClass(sub: UserSubscription): string {
  // Unlimited subscriptions get a special color
  if (isUnlimited(sub)) {
    return 'bg-emerald-500'
  }
  const maxPercentage = getMaxUsagePercentage(sub)
  if (maxPercentage >= 90) return 'bg-red-500'
  if (maxPercentage >= 70) return 'bg-orange-500'
  return 'bg-green-500'
}

function getProgressBarClass(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return 'bg-gray-400'
  const percentage = ((used || 0) / limit) * 100
  if (percentage >= 90) return 'bg-red-500'
  if (percentage >= 70) return 'bg-orange-500'
  return 'bg-green-500'
}

function getProgressWidth(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return '0%'
  const percentage = Math.min(((used || 0) / limit) * 100, 100)
  return `${percentage}%`
}

function formatUsage(used: number | undefined, limit: number | null | undefined): string {
  const usedValue = (used || 0).toFixed(2)
  const limitValue = limit?.toFixed(2) || '∞'
  return `$${usedValue}/$${limitValue}`
}

function formatDaysRemaining(expiresAt: string): string {
  return formatRemainingDurationCompact(expiresAt) || t('subscriptionProgress.expired')
}

function getDaysRemainingClass(expiresAt: string): string {
  const remainingHours = getRemainingHours(expiresAt)
  if (remainingHours === null || remainingHours <= 72) return 'text-red-600 dark:text-red-400'
  if (remainingHours <= 168) return 'text-orange-600 dark:text-orange-400'
  return 'text-gray-500 dark:text-dark-400'
}

function toggleTooltip() {
  tooltipOpen.value = !tooltipOpen.value
}

function closeTooltip() {
  tooltipOpen.value = false
}

async function loadSubscriptionPlansForSubscriptions(records: UserSubscription[]) {
  if (!records.some((subscription) => subscription.plan_id)) {
    subscriptionPlans.value = []
    return
  }

  try {
    const response = await paymentAPI.getPlans()
    subscriptionPlans.value = normalizeSubscriptionPlans(response.data || [])
  } catch (error) {
    console.error('Failed to load subscription plans in SubscriptionProgressMini:', error)
  }
}

function handleClickOutside(event: MouseEvent) {
  if (containerRef.value && !containerRef.value.contains(event.target as Node)) {
    closeTooltip()
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  void Promise.allSettled([
    subscriptionStore.fetchActiveSubscriptions(),
    subscriptionStore.fetchEntitlements(),
  ])
    .then(() => loadSubscriptionPlansForSubscriptions(displaySubscriptions.value))
    .catch((error) => {
      console.error('Failed to load subscriptions in SubscriptionProgressMini:', error)
    })
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.2s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: scale(0.95) translateY(-4px);
}
</style>
