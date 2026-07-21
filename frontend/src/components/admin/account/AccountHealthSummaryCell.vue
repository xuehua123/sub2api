<template>
  <button
    type="button"
    class="group flex w-[15.5rem] min-w-[15.5rem] max-w-[15.5rem] flex-col gap-0.5 rounded-md border border-gray-200 bg-white/70 px-1.5 py-1 text-left transition hover:border-sky-300 hover:bg-sky-50/70 dark:border-dark-700 dark:bg-dark-800/70 dark:hover:border-sky-800 dark:hover:bg-sky-950/20"
    :title="title"
    @click="$emit('open')"
  >
    <span v-if="loading" class="h-10 w-full animate-pulse rounded bg-gray-200 dark:bg-dark-700"></span>
    <template v-else-if="item">
      <!-- Each time window keeps its label, traffic, rate, and latency together. -->
      <span class="grid grid-cols-2 gap-x-2 gap-y-1">
        <span
          v-for="window in windows"
          :key="window"
          class="min-w-0 border-l-2 border-gray-200 pl-1.5 dark:border-dark-600"
          :title="windowTitle(window)"
        >
          <span class="flex items-baseline justify-between gap-1 leading-4">
            <span class="shrink-0 text-[11px] font-bold text-gray-700 dark:text-gray-200">{{ windowLabel(window) }}</span>
            <span class="shrink-0 tabular-nums text-[10px] text-gray-500 dark:text-gray-400">{{ t('admin.accounts.accountHealth.requestCount', { count: formatCount(stats(window)?.request_count) }) }}</span>
          </span>
          <span class="flex items-baseline justify-between gap-1 leading-3 tabular-nums">
            <span class="min-w-0 truncate text-[11px] font-semibold" :class="successClass(stats(window)?.success_rate_percent)">{{ successText(window) }}</span>
            <span
              v-if="window === '1m' && trendArrow(item.success_rate_trend_1m)"
              class="ml-0.5 text-[9px] font-bold"
              :class="trendClass(item.success_rate_trend_1m)"
            >{{ trendArrow(item.success_rate_trend_1m) }}</span>
            <span class="ml-auto shrink-0 text-[10px] text-gray-400 dark:text-gray-500">{{ firstTokenText(window) }}</span>
          </span>
        </span>
      </span>

      <!-- Always at bottom: traffic + probe bars (closed accounts keep probe history) -->
      <span class="mt-0.5 border-t border-gray-100 pt-0.5 dark:border-dark-700">
        <span class="grid grid-cols-[repeat(24,minmax(0,1fr))] gap-px">
          <span
            v-for="(sample, idx) in timelineBars"
            :key="idx"
            class="h-1.5 rounded-[1px]"
            :class="sampleClass(sample)"
            :title="sampleTitle(sample)"
          />
        </span>
        <span class="mt-0.5 flex items-center justify-between gap-1 text-[9px] leading-3 text-gray-500 dark:text-gray-400">
          <span class="truncate">{{ probeLine }}</span>
          <span class="shrink-0 text-sky-600 group-hover:text-sky-700 dark:text-sky-400">详情</span>
        </span>
      </span>
    </template>
    <span v-else class="text-xs text-gray-400 dark:text-dark-400">暂无健康数据</span>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpsAccountHealthItem, OpsAccountHealthWindow } from '@/api/admin/ops'
import {
  ACCOUNT_HEALTH_PRIMARY_WINDOWS,
  firstTokenStats,
  formatCount,
  formatMs,
  formatPercent,
  paddedTimelineSamples,
  probeStatusLabel,
  sampleClass,
  sampleTitle,
  successClass,
  trendArrow,
  trendClass,
  windowStats
} from '@/utils/accountHealthDisplay'

const props = defineProps<{
  item?: OpsAccountHealthItem | null
  loading?: boolean
  probeIntervalMinutes?: number | null
}>()

defineEmits<{
  open: []
}>()

const { t } = useI18n()
const windows = ACCOUNT_HEALTH_PRIMARY_WINDOWS
const timelineBars = computed(() => paddedTimelineSamples(props.item, 24))

const title = computed(() => {
  const item = props.item
  if (!item) return '暂无健康数据'
  const trend = item.success_rate_trend_1m
  const trendPart =
    trend?.direction && trend.direction !== 'unknown'
      ? ` | 1m趋势 ${trendArrow(trend)} ${formatPercent(trend.delta_percent, 1)}`
      : ''
  return `${item.account_name || item.account_id}${trendPart} | 绿=请求 蓝=探测成功 紫=探测失败 | 点击详情`
})

const probeLine = computed(() => {
  const interval = props.probeIntervalMinutes
  const intervalText =
    Number.isFinite(interval as number) && (interval as number) > 0 ? `每${interval}m` : '探测'
  const auto = props.item?.probe_auto_disabled ? '停' : '开'
  return `${intervalText}·${auto}·${probeStatusLabel(props.item)}`
})

function stats(window: OpsAccountHealthWindow) {
  return windowStats(props.item, window)
}

function successText(window: OpsAccountHealthWindow): string {
  const stat = stats(window)
  if (!stat || stat.request_count <= 0) return '—'
  return formatPercent(stat.success_rate_percent)
}

function firstTokenText(window: OpsAccountHealthWindow): string {
  const ft = firstTokenStats(props.item, window)
  if (!ft || !ft.sample_count || ft.avg_ms == null) return '—'
  return formatMs(ft.avg_ms)
}

function windowLabel(window: OpsAccountHealthWindow): string {
  return t(`admin.accounts.accountHealth.windows.${window}`)
}

function windowTitle(window: OpsAccountHealthWindow): string {
  return t('admin.accounts.accountHealth.windowTitle', { window: windowLabel(window) })
}
</script>
