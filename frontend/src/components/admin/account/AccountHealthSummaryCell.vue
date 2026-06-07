<template>
  <button
    type="button"
    class="group flex min-w-[12rem] max-w-[18rem] flex-col gap-1.5 rounded-lg border border-gray-200 bg-white/70 px-2.5 py-2 text-left transition hover:border-sky-300 hover:bg-sky-50/70 dark:border-dark-700 dark:bg-dark-800/70 dark:hover:border-sky-800 dark:hover:bg-sky-950/20"
    :class="severityBorderClass"
    :title="title"
    @click="$emit('open')"
  >
    <span v-if="loading" class="h-4 w-28 animate-pulse rounded bg-gray-200 dark:bg-dark-700"></span>
    <template v-else-if="item">
      <span class="flex items-center justify-between gap-2">
        <span class="truncate text-xs font-semibold" :class="successClass(primaryStat)">
          10m {{ statText(primaryStat) }}
        </span>
        <span class="shrink-0 rounded-md px-1.5 py-0.5 text-[10px] font-semibold" :class="severityClass">
          {{ item.recommendation?.severity || 'P3' }}
        </span>
      </span>
      <span class="grid grid-cols-[repeat(30,minmax(2px,1fr))] gap-0.5">
        <span
          v-for="(sample, idx) in recentForDisplay"
          :key="`${item.account_id}-${idx}-${sample?.created_at || 'empty'}`"
          class="h-3 rounded-sm"
          :class="sampleClass(sample)"
        ></span>
      </span>
      <span class="flex items-center justify-between gap-2 text-[11px] text-gray-500 dark:text-gray-400">
        <span class="truncate">1h {{ statText(hourStat) }}</span>
        <span class="truncate text-right font-medium group-hover:text-sky-700 dark:group-hover:text-sky-300">
          {{ actionLabel(item.recommendation?.action) }}
        </span>
      </span>
    </template>
    <span v-else class="text-xs text-gray-400 dark:text-dark-400">暂无健康数据</span>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { OpsAccountHealthItem, OpsAccountHealthSample, OpsAccountHealthWindowStats } from '@/api/admin/ops'

const props = defineProps<{
  item?: OpsAccountHealthItem | null
  loading?: boolean
}>()

defineEmits<{
  open: []
}>()

const primaryStat = computed(() => props.item?.windows?.['10m'] ?? null)
const hourStat = computed(() => props.item?.windows?.['1h'] ?? null)

const recentForDisplay = computed<Array<OpsAccountHealthSample | null>>(() => {
  const raw = props.item?.recent ?? []
  const samples: Array<OpsAccountHealthSample | null> = raw.slice(-30)
  while (samples.length < 30) {
    samples.unshift(null)
  }
  return samples
})

const severityBorderClass = computed(() => {
  const item = props.item
  if (!item) return ''
  if (item.recommendation?.action === 'close_now' || item.recommendation?.severity === 'P1') {
    return 'border-red-200 dark:border-red-900/60'
  }
  if (item.recommendation?.action === 'can_open' || item.recommendation?.recovery_ready) {
    return 'border-emerald-200 dark:border-emerald-900/60'
  }
  if (item.recommendation?.severity === 'P2') {
    return 'border-amber-200 dark:border-amber-900/60'
  }
  return ''
})

const severityClass = computed(() => {
  switch ((props.item?.recommendation?.severity || '').toUpperCase()) {
    case 'P1':
      return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    case 'P2':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    default:
      return 'bg-slate-100 text-slate-600 dark:bg-dark-700 dark:text-slate-300'
  }
})

const title = computed(() => {
  const item = props.item
  if (!item) return '暂无健康数据'
  return `${item.account_name || item.account_id} | ${item.recommendation?.title || '健康'} | ${item.recommendation?.reason || ''}`
})

function statText(stat?: OpsAccountHealthWindowStats | null): string {
  if (!stat || stat.request_count <= 0) return '暂无'
  return `${formatPercent(stat.success_rate_percent)} / ${stat.request_count}次`
}

function successClass(stat?: OpsAccountHealthWindowStats | null): string {
  if (!stat || stat.request_count <= 0) return 'text-gray-400 dark:text-dark-400'
  if (stat.success_rate_percent >= 98) return 'text-emerald-600 dark:text-emerald-300'
  if (stat.success_rate_percent >= 90) return 'text-amber-600 dark:text-amber-300'
  return 'text-red-600 dark:text-red-300'
}

function sampleClass(sample: OpsAccountHealthSample | null): string {
  if (!sample) return 'bg-gray-200 dark:bg-dark-700'
  if (sample.kind === 'success') return 'bg-emerald-500'
  if (sample.status_code === 429 || sample.status_code === 529) return 'bg-amber-500'
  return 'bg-red-500'
}

function actionLabel(action?: string): string {
  switch (action) {
    case 'keep_open': return '保持开启'
    case 'watch': return '观察'
    case 'close_now': return '建议关闭'
    case 'can_open': return '可打开'
    case 'needs_probe': return '需探测'
    case 'keep_closed': return '保持关闭'
    default: return '查看'
  }
}

function formatPercent(value: number | null | undefined): string {
  if (!Number.isFinite(value as number)) return '0.0%'
  return `${Number(value).toFixed(1)}%`
}
</script>
