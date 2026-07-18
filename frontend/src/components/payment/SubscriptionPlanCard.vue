<template>
  <div
    :class="[
      'group relative flex min-h-[500px] flex-col overflow-hidden rounded-xl border bg-white transition-all dark:bg-gradient-to-br dark:from-dark-800 dark:via-dark-800 dark:to-slate-950',
      'shadow-sm hover:-translate-y-0.5 hover:shadow-xl dark:shadow-black/20',
      borderClass,
    ]"
  >
    <div :class="['h-1', accentClass]" />

    <div class="flex flex-1 flex-col gap-3 p-4">
      <div class="flex items-start justify-between gap-3">
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <h3 class="text-base font-bold leading-tight text-gray-900 dark:text-white">
              {{ plan.name }}
            </h3>
            <span :class="['rounded-full px-2 py-0.5 text-[11px] font-semibold', badgeLightClass]">
              {{ pLabel }}
            </span>
          </div>
          <p v-if="plan.description" class="mt-1.5 whitespace-pre-wrap text-xs leading-5 text-gray-500 dark:text-dark-300">
            {{ plan.description }}
          </p>
        </div>
        <div class="shrink-0 rounded-2xl bg-gray-50/80 px-3 py-2 text-right shadow-sm ring-1 ring-gray-100 dark:bg-dark-900/60 dark:ring-white/5">
          <div class="flex items-baseline justify-end gap-1">
            <span class="text-sm font-semibold text-gray-400 dark:text-dark-400">{{ planCurrencySymbol }}</span>
            <span :class="['text-4xl font-black leading-none tracking-normal drop-shadow-sm', textClass]">{{ formatAmount(plan.price) }}</span>
            <span v-if="plan.currency" class="text-xs font-medium text-gray-400 dark:text-dark-500">{{ planCurrency }}</span>
          </div>
          <div class="mt-0.5 text-[11px] text-gray-400 dark:text-dark-400">/ {{ validitySuffix }}</div>
          <div v-if="plan.original_price" class="mt-1 flex items-center justify-end gap-1.5">
            <span class="text-xs text-gray-400 line-through dark:text-dark-500">
              {{ planCurrencySymbol }}{{ formatAmount(plan.original_price) }}<template v-if="plan.currency"> {{ planCurrency }}</template>
            </span>
            <span v-if="discountText" :class="['rounded px-1.5 py-0.5 text-[10px] font-semibold', discountClass]">
              {{ discountText }}
            </span>
          </div>
        </div>
      </div>

      <div class="grid grid-cols-3 gap-2">
        <div class="rounded-lg border border-gray-100 bg-white/80 p-2.5 shadow-sm dark:border-white/5 dark:bg-dark-700/70">
          <div class="text-[11px] font-medium text-gray-400 dark:text-dark-400">{{ t('payment.planCard.validity') }}</div>
          <div class="mt-1 truncate text-sm font-semibold text-gray-800 dark:text-gray-100">{{ validitySuffix }}</div>
        </div>
        <div class="rounded-lg border border-cyan-200 bg-cyan-50 p-2.5 shadow-sm dark:border-cyan-400/20 dark:bg-cyan-500/10">
          <div class="text-[11px] font-medium text-gray-400 dark:text-dark-400">{{ quotaMetricLabel }}</div>
          <div class="mt-1 truncate text-sm font-bold text-cyan-700 dark:text-cyan-200">{{ quotaMetricValue }}</div>
        </div>
        <div class="rounded-lg border border-emerald-200 bg-emerald-50 p-2.5 shadow-sm dark:border-emerald-400/25 dark:bg-emerald-500/10">
          <div class="text-[11px] font-medium text-gray-400 dark:text-dark-400">{{ t('payment.planCard.unitCost') }}</div>
          <div class="mt-1 truncate text-sm font-bold text-emerald-700 dark:text-emerald-200">{{ unitCostText }}</div>
        </div>
      </div>

      <div v-if="hasPeakRate" class="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-400/20 dark:bg-amber-500/10 dark:text-amber-200">
        <span class="font-semibold">{{ t('payment.planCard.peakRate') }}: </span>
        <span>{{ peakRateDisplay }}</span>
      </div>

      <div class="rounded-xl border border-gray-100 bg-gray-50/40 p-3 dark:border-dark-700/80 dark:bg-dark-900/30">
        <div class="mb-2 flex items-center justify-between gap-2">
          <span class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-dark-300">
            {{ t('payment.planCard.authorizedGroups') }}
          </span>
          <span class="text-xs text-gray-400 dark:text-dark-400">
            {{ t('payment.planCard.groupsCount', { count: includedGroups.length }) }}
          </span>
        </div>
        <div v-if="includedGroups.length > 0" class="max-h-56 space-y-1.5 overflow-y-auto pr-1">
          <div
            v-for="group in includedGroups"
            :key="group.id"
            class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-2 rounded-lg border border-gray-100 bg-white px-2.5 py-1.5 shadow-sm dark:border-white/5 dark:bg-dark-900/80"
          >
            <div class="min-w-0">
              <div class="truncate text-xs font-semibold text-gray-800 dark:text-gray-100">
                {{ group.name || `Group #${group.id}` }}
              </div>
              <div class="mt-0.5 truncate text-[10px] font-medium text-gray-500 dark:text-dark-400">
                {{ platformLabel(group.platform || '') }}
              </div>
            </div>
            <div class="flex shrink-0 items-center gap-1.5 text-right">
              <span class="rounded-md bg-emerald-100 px-1.5 py-0.5 text-[10px] font-bold text-emerald-700 dark:bg-emerald-400/15 dark:text-emerald-200">
                {{ rateText(group.rate_multiplier) }}
              </span>
              <span class="w-[82px] truncate text-[10px] font-bold text-emerald-700 dark:text-emerald-200">
                {{ groupUnitCostText(group.rate_multiplier) }}
              </span>
            </div>
          </div>
        </div>
        <div v-else class="rounded-lg bg-gray-50 px-3 py-3 text-xs text-gray-500 dark:bg-dark-900/60 dark:text-dark-300">
          {{ t('payment.planCard.noGroups') }}
        </div>
      </div>

      <div class="grid gap-2 text-[11px] text-gray-600 dark:text-gray-300 sm:grid-cols-2">
        <div class="rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-700/60">
          <span class="text-gray-400 dark:text-dark-400">{{ t('payment.planCard.overagePolicy') }}: </span>
          <span class="font-medium">{{ overagePolicyText }}</span>
        </div>
        <div v-if="modelScopeLabels.length > 0" class="rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-700/60">
          <span class="text-gray-400 dark:text-dark-400">{{ t('payment.planCard.models') }}: </span>
          <span class="font-medium">{{ modelScopeLabels.join(', ') }}</span>
        </div>
      </div>

      <div v-if="visibleFeatures.length > 0" class="grid gap-1.5 text-xs sm:grid-cols-2">
        <div v-for="feature in visibleFeatures" :key="feature" class="flex items-start gap-2">
          <svg :class="['mt-0.5 h-3.5 w-3.5 flex-shrink-0', iconClass]" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
          </svg>
          <span class="leading-5 text-gray-600 dark:text-gray-300">{{ feature }}</span>
        </div>
      </div>

      <div class="flex-1" />

      <button
        type="button"
        :class="['w-full rounded-xl py-2.5 text-sm font-bold transition-all active:scale-[0.98]', btnClass]"
        @click="emit('select', plan)"
      >
        {{ isRenewal ? t('payment.renewNow') : t('payment.subscribeNow') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SubscriptionPlan, SubscriptionPlanGroupInfo } from '@/types/payment'
import type { UserSubscription } from '@/types'
import { normalizePlanValidityUnit } from '@/utils/subscriptionTime'
import { sortGroupsForDisplay } from '@/utils/groupDisplayOrder'
import { useAppStore } from '@/stores/app'
import { hasPeakRate as groupHasPeakRate, formatPeakRateWindow, serverTimezoneLabel } from '@/utils/peak-rate'
import {
  platformAccentBarClass,
  platformBadgeLightClass,
  platformBorderClass,
  platformTextClass,
  platformIconClass,
  platformButtonClass,
  platformDiscountClass,
  platformLabel,
} from '@/utils/platformColors'

const props = defineProps<{ plan: SubscriptionPlan; activeSubscriptions?: UserSubscription[] }>()
const emit = defineEmits<{ select: [plan: SubscriptionPlan] }>()
const { t, locale } = useI18n()

const includedGroups = computed<SubscriptionPlanGroupInfo[]>(() => {
  if (props.plan.groups?.length) {
    return sortGroupsForDisplay(props.plan.groups)
  }
  if (props.plan.group_id) {
    return [{
      id: props.plan.group_id,
      name: props.plan.group_name || '',
      platform: props.plan.group_platform || '',
      rate_multiplier: props.plan.rate_multiplier ?? 1,
      peak_rate_enabled: props.plan.peak_rate_enabled,
      peak_start: props.plan.peak_start,
      peak_end: props.plan.peak_end,
      peak_rate_multiplier: props.plan.peak_rate_multiplier,
      daily_limit_usd: props.plan.daily_limit_usd,
      weekly_limit_usd: props.plan.weekly_limit_usd,
      monthly_limit_usd: props.plan.monthly_limit_usd,
      supported_model_scopes: props.plan.supported_model_scopes,
      sort_order: 0,
    }]
  }
  return []
})

const usesBroadGroupCoverage = computed(() =>
  props.plan.access_scope === 'all_subscription_groups' ||
  props.plan.access_scope === 'platform_subscription_groups' ||
  includedGroups.value.length > 1
)
const primaryGroup = computed(() => includedGroups.value[0])
const platform = computed(() => {
  if (usesBroadGroupCoverage.value) return ''
  return primaryGroup.value?.platform || props.plan.group_platform || ''
})
const planGroupIDs = computed(() => new Set(includedGroups.value.map(group => group.id)))
const isRenewal = computed(() =>
  props.activeSubscriptions?.some(sub => sub.status === 'active' && planGroupIDs.value.has(sub.group_id)) ?? false
)

const accentClass = computed(() => platformAccentBarClass(platform.value))
const borderClass = computed(() => platformBorderClass(platform.value))
const badgeLightClass = computed(() => platformBadgeLightClass(platform.value))
const textClass = computed(() => platformTextClass(platform.value))
const iconClass = computed(() => platformIconClass(platform.value))
const btnClass = computed(() => platformButtonClass(platform.value))
const discountClass = computed(() => platformDiscountClass(platform.value))
const pLabel = computed(() => (
  usesBroadGroupCoverage.value ? t('payment.planCard.allIncluded') : platformLabel(platform.value)
))

const planCurrency = computed(() => props.plan.currency?.trim().toUpperCase() || 'CNY')
const planCurrencySymbol = computed(() => {
  switch (planCurrency.value) {
    case 'CNY': return '¥'
    case 'USD': return '$'
    case 'EUR': return '€'
    case 'GBP': return '£'
    case 'JPY': return '¥'
    default: return planCurrency.value
  }
})

const discountText = computed(() => {
  if (!props.plan.original_price || props.plan.original_price <= 0) return ''
  const pct = Math.round((1 - props.plan.price / props.plan.original_price) * 100)
  return pct > 0 ? `-${pct}%` : ''
})

type QuotaMetric = { key: 'daily' | 'weekly' | 'monthly'; value: number } | null

const quotaMetric = computed<QuotaMetric>(() => {
  if (props.plan.monthly_limit_usd != null) return { key: 'monthly', value: props.plan.monthly_limit_usd }
  if (props.plan.weekly_limit_usd != null) return { key: 'weekly', value: props.plan.weekly_limit_usd }
  if (props.plan.daily_limit_usd != null) return { key: 'daily', value: props.plan.daily_limit_usd }
  return null
})

const quotaMetricLabel = computed(() => {
  if (!quotaMetric.value) return t('payment.planCard.quota')
  return t(`payment.planCard.${quotaMetric.value.key}Limit`)
})

const quotaMetricValue = computed(() => {
  if (!quotaMetric.value) return t('payment.planCard.unlimited')
  return `$${formatAmount(quotaMetric.value.value)}`
})

const unitCost = computed(() => {
  const quota = quotaMetric.value?.value
  if (!quota || quota <= 0 || props.plan.price <= 0) return null
  return props.plan.price / quota
})

const unitCostText = computed(() => {
  if (!unitCost.value) return t('payment.planCard.priceUnavailable')
  return t('payment.planCard.unitCostValue', { amount: formatPlanCurrency(unitCost.value) })
})

const overagePolicyText = computed(() => {
  if (props.plan.overage_policy === 'balance_fallback') {
    return t('payment.planCard.overageBalanceFallback')
  }
  return t('payment.planCard.overageBlock')
})

const visibleFeatures = computed(() => (
  props.plan.features || []
).map(feature => feature.trim()).filter(feature => feature && feature !== '[]'))

const appStore = useAppStore()

const hasPeakRate = computed(() => groupHasPeakRate(props.plan))

const peakRateDisplay = computed(() => {
  return formatPeakRateWindow(props.plan, serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset))
})

const MODEL_SCOPE_LABELS: Record<string, string> = {
  claude: 'Claude',
  gemini_text: 'Gemini',
  gemini_image: 'Imagen',
}

const shouldShowModelScopes = computed(() =>
  platform.value === 'antigravity' || includedGroups.value.some(group => group.platform === 'antigravity')
)

const modelScopeLabels = computed(() => {
  if (!shouldShowModelScopes.value) return []
  const scopes = props.plan.supported_model_scopes?.length
    ? props.plan.supported_model_scopes
    : includedGroups.value.flatMap(group => group.supported_model_scopes || [])
  return Array.from(new Set(scopes)).map(scope => MODEL_SCOPE_LABELS[scope] || scope)
})

const validitySuffix = computed(() => {
  const unit = normalizePlanValidityUnit(props.plan.validity_unit)
  if (unit === 'week') return `${props.plan.validity_days}${t('payment.admin.weeks')}`
  if (unit === 'month') return `${props.plan.validity_days}${t('payment.months')}`
  if (unit === 'year') return `${props.plan.validity_days}${t('payment.years')}`
  return `${props.plan.validity_days}${t('payment.days')}`
})

function formatAmount(value: number | null | undefined): string {
  const amount = value ?? 0
  return new Intl.NumberFormat(locale.value, {
    maximumFractionDigits: amount > 0 && amount < 1 ? 4 : 2,
    minimumFractionDigits: amount > 0 && amount < 1 ? 2 : 0,
  }).format(amount)
}

function formatPlanCurrency(value: number): string {
  return new Intl.NumberFormat(locale.value, {
    style: 'currency',
    currency: planCurrency.value,
    maximumFractionDigits: value > 0 && value < 1 ? 4 : 2,
    minimumFractionDigits: value > 0 && value < 1 ? 4 : 2,
  }).format(value)
}

function normalizedRate(rate: number | null | undefined): number {
  return rate && rate > 0 ? rate : 1
}

function rateText(rate: number | null | undefined): string {
  return `x${Number(normalizedRate(rate).toPrecision(10))}`
}

function groupUnitCostText(rate: number | null | undefined): string {
  if (!unitCost.value) return t('payment.planCard.priceUnavailable')
  return t('payment.planCard.unitCostValue', {
    amount: formatPlanCurrency(unitCost.value * normalizedRate(rate))
  })
}
</script>
