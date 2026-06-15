import type { UsageBillingSource } from '@/types'

export const getUsageBillingSourceLabel = (
  source: UsageBillingSource | null | undefined,
  t: (key: string) => string
): string => {
  if (!source) return '-'
  return t(`admin.usage.billingSource.${source}`)
}

export const getUsageBillingSourceBadgeClass = (
  source: UsageBillingSource | null | undefined
): string => {
  if (source === 'entitlement_quota') {
    return 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900 dark:text-emerald-200'
  }
  if (source === 'entitlement_balance_fallback') {
    return 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200'
  }
  if (source === 'legacy_subscription') {
    return 'bg-indigo-100 text-indigo-800 dark:bg-indigo-900 dark:text-indigo-200'
  }
  if (source === 'balance') {
    return 'bg-slate-100 text-slate-800 dark:bg-slate-700 dark:text-slate-200'
  }
  return 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
}
