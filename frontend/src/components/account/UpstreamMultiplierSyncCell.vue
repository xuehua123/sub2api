<template>
  <div
    class="flex min-w-[6.25rem] flex-col gap-0.5"
    :title="syncMeta.title"
    data-testid="upstream-multiplier-sync"
  >
    <span class="text-sm font-mono text-gray-700 dark:text-gray-300">
      {{ formattedAccountMultiplier }}x
    </span>
    <span
      class="inline-flex items-center gap-1 text-[11px] font-medium leading-4"
      :class="syncMeta.className"
      data-testid="upstream-multiplier-sync-status"
    >
      <Icon :name="syncMeta.icon" size="xs" />
      {{ syncMeta.label }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime } from '@/utils/format'
import type { UpstreamAccountBinding } from '@/api/admin/upstreamConnections'

const props = withDefaults(defineProps<{
  accountMultiplier?: number | null
  binding?: UpstreamAccountBinding | null
  connectionName?: string | null
  loading?: boolean
  loadFailed?: boolean
}>(), {
  accountMultiplier: null,
  binding: null,
  connectionName: null,
  loading: false,
  loadFailed: false
})

const { t } = useI18n()

type SyncIcon = 'checkCircle' | 'clock' | 'exclamationCircle' | 'exclamationTriangle' | 'link' | 'refresh'

type SyncMeta = {
  label: string
  title: string
  className: string
  icon: SyncIcon
}

const localMultiplier = computed(() => props.accountMultiplier ?? 1)
const formattedAccountMultiplier = computed(() => localMultiplier.value.toFixed(2))

const formatMultiplier = (value: number) => value.toFixed(6).replace(/\.?0+$/, '')

const isFresh = computed(() => {
  const freshUntil = props.binding?.fresh_until
  if (!freshUntil) return false
  const timestamp = new Date(freshUntil).getTime()
  return Number.isFinite(timestamp) && timestamp > Date.now()
})

const hasMatchingMultiplier = computed(() => {
  const observed = props.binding?.observed_multiplier
  if (typeof observed !== 'number' || !Number.isFinite(observed)) return false
  const baseline = Math.max(Math.abs(observed), Math.abs(localMultiplier.value), 1)
  return Math.abs(observed - localMultiplier.value) <= baseline * 1e-6
})

const rateConfidence = computed(() => {
  const details = props.binding?.resolution_details
  if (!details || typeof details !== 'object') return 'unknown'
  const value = details.rate_confidence
  return typeof value === 'string' && value.trim() ? value.trim() : 'unknown'
})

const isReportedRate = computed(() => rateConfidence.value === 'reported')

const observedAt = computed(() => {
  const value = props.binding?.observed_at
  return value ? formatDateTime(value) || t('common.time.never') : t('common.time.never')
})

const upstreamDetails = computed(() => ({
  connection: props.connectionName || '-',
  group: props.binding?.remote_group_name || '-',
  upstream: props.binding?.observed_multiplier == null
    ? '-'
    : formatMultiplier(props.binding.observed_multiplier),
  account: formatMultiplier(localMultiplier.value),
  observedAt: observedAt.value,
  error: props.binding?.last_error || '-',
  rateConfidence: rateConfidence.value
}))

const syncMeta = computed<SyncMeta>(() => {
  const key = 'admin.accounts.upstreamMultiplierSync'
  const binding = props.binding

  if (props.loading) {
    return {
      label: t(`${key}.loading`),
      title: t(`${key}.loadingTitle`),
      className: 'text-gray-500 dark:text-dark-400',
      icon: 'refresh'
    }
  }
  if (props.loadFailed) {
    return {
      label: t(`${key}.loadFailed`),
      title: t(`${key}.loadFailedTitle`),
      className: 'text-red-600 dark:text-red-400',
      icon: 'exclamationCircle'
    }
  }
  if (!binding) {
    return {
      label: t(`${key}.unbound`),
      title: t(`${key}.unboundTitle`),
      className: 'text-gray-400 dark:text-dark-500',
      icon: 'link'
    }
  }
  if (binding.status === 'error' || binding.last_error) {
    return {
      label: t(`${key}.failed`),
      title: t(`${key}.failedTitle`, upstreamDetails.value),
      className: 'text-red-600 dark:text-red-400',
      icon: 'exclamationCircle'
    }
  }
  if (binding.status === 'pending') {
    return {
      label: t(`${key}.pending`),
      title: t(`${key}.pendingTitle`, upstreamDetails.value),
      className: 'text-sky-600 dark:text-sky-400',
      icon: 'clock'
    }
  }
  if (binding.status === 'unresolved') {
    return {
      label: t(`${key}.unresolved`),
      title: t(`${key}.unresolvedTitle`, upstreamDetails.value),
      className: 'text-amber-600 dark:text-amber-300',
      icon: 'exclamationTriangle'
    }
  }
  if (binding.status !== 'ready') {
    return {
      label: t(`${key}.pending`),
      title: t(`${key}.pendingTitle`, upstreamDetails.value),
      className: 'text-sky-600 dark:text-sky-400',
      icon: 'clock'
    }
  }
  if (binding.observed_multiplier == null) {
    return {
      label: t(`${key}.multiplierUnavailable`),
      title: t(`${key}.multiplierUnavailableTitle`, upstreamDetails.value),
      className: 'text-gray-500 dark:text-dark-400',
      icon: 'exclamationCircle'
    }
  }
  if (!isFresh.value) {
    return {
      label: t(`${key}.stale`),
      title: t(`${key}.staleTitle`, upstreamDetails.value),
      className: 'text-amber-600 dark:text-amber-300',
      icon: 'clock'
    }
  }
  // Fallback/unknown rates are display-only and never authorize auto account
  // billing updates. Do not treat numeric equality as "already synchronized".
  if (!isReportedRate.value) {
    return {
      label: t(`${key}.observeOnly`),
      title: t(`${key}.observeOnlyTitle`, upstreamDetails.value),
      className: 'text-amber-600 dark:text-amber-300',
      icon: 'exclamationTriangle'
    }
  }
  if (!hasMatchingMultiplier.value) {
    return {
      label: t(`${key}.mismatch`),
      title: t(`${key}.mismatchTitle`, upstreamDetails.value),
      className: 'text-amber-600 dark:text-amber-300',
      icon: 'exclamationTriangle'
    }
  }
  return {
    label: t(`${key}.synchronized`),
    title: t(`${key}.synchronizedTitle`, upstreamDetails.value),
    className: 'text-emerald-600 dark:text-emerald-300',
    icon: 'checkCircle'
  }
})
</script>
