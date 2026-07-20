<template>
  <button
    type="button"
    class="group flex min-w-[17rem] max-w-[22rem] flex-col gap-1 rounded-lg border border-gray-200 bg-white/70 px-2 py-1.5 text-left transition hover:border-sky-300 hover:bg-sky-50/70 dark:border-dark-700 dark:bg-dark-800/70 dark:hover:border-sky-800 dark:hover:bg-sky-950/20"
    :title="title"
    @click="$emit('open')"
  >
    <span v-if="loading" class="h-16 w-full animate-pulse rounded bg-gray-200 dark:bg-dark-700"></span>
    <template v-else-if="item">
      <span class="grid grid-cols-[2rem_2.4rem_3.4rem_3.2rem] items-center gap-x-1 text-[10px] font-medium uppercase tracking-wide text-gray-400 dark:text-dark-400">
        <span></span>
        <span class="text-right">请求</span>
        <span class="text-right">成功%</span>
        <span class="text-right">首Token</span>
      </span>
      <span
        v-for="window in windows"
        :key="window"
        class="grid grid-cols-[2rem_2.4rem_3.4rem_3.2rem] items-center gap-x-1 text-[11px] leading-4"
      >
        <span class="font-semibold text-gray-600 dark:text-gray-300">{{ window }}</span>
        <span class="text-right tabular-nums text-gray-700 dark:text-gray-200">{{ formatCount(stats(window)?.request_count) }}</span>
        <span class="flex items-center justify-end gap-0.5 tabular-nums font-medium" :class="successClass(stats(window)?.success_rate_percent)">
          <span>{{ successText(window) }}</span>
          <span
            v-if="window === '1m' && trendArrow(item.success_rate_trend_1m)"
            class="text-[10px] font-bold"
            :class="trendClass(item.success_rate_trend_1m)"
          >{{ trendArrow(item.success_rate_trend_1m) }}</span>
        </span>
        <span class="text-right tabular-nums text-gray-600 dark:text-gray-300">{{ firstTokenText(window) }}</span>
      </span>
      <span class="mt-0.5 flex items-center justify-between gap-2 border-t border-gray-100 pt-1 text-[10px] text-gray-500 dark:border-dark-700 dark:text-gray-400">
        <span class="truncate">{{ probeLine }}</span>
        <span class="shrink-0 font-medium text-sky-600 group-hover:text-sky-700 dark:text-sky-400">详情</span>
      </span>
    </template>
    <span v-else class="text-xs text-gray-400 dark:text-dark-400">暂无健康数据</span>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { OpsAccountHealthItem, OpsAccountHealthWindow } from '@/api/admin/ops'
import {
  ACCOUNT_HEALTH_PRIMARY_WINDOWS,
  firstTokenStats,
  formatCount,
  formatMs,
  formatPercent,
  probeStatusLabel,
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

const windows = ACCOUNT_HEALTH_PRIMARY_WINDOWS

const title = computed(() => {
  const item = props.item
  if (!item) return '暂无健康数据'
  const trend = item.success_rate_trend_1m
  const trendPart = trend?.direction && trend.direction !== 'unknown'
    ? ` | 1m趋势 ${trendArrow(trend)} ${formatPercent(trend.delta_percent, 1)}`
    : ''
  return `${item.account_name || item.account_id}${trendPart} | 点击查看详情`
})

const probeLine = computed(() => {
  const interval = props.probeIntervalMinutes
  const intervalText = Number.isFinite(interval as number) && (interval as number) > 0
    ? `每${interval}m`
    : '探测'
  return `${intervalText} · ${probeStatusLabel(props.item)}`
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
</script>
