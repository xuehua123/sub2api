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
        <span class="truncate text-xs font-semibold" :class="primaryDisplay.className">
          {{ primaryDisplay.label }} {{ primaryDisplay.text }}
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
        <span class="truncate">{{ hourDisplay.label }} {{ hourDisplay.text }}</span>
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
const probeWindowMinutes = {
  '10m': 10,
  '1h': 60
} as const

interface HealthDisplay {
  label: string
  text: string
  className: string
}

const primaryDisplay = computed<HealthDisplay>(() => metricDisplay('10m', primaryStat.value))
const hourDisplay = computed<HealthDisplay>(() => metricDisplay('1h', hourStat.value))

const recentForDisplay = computed<Array<OpsAccountHealthSample | null>>(() => {
  const item = props.item
  const raw = item && hasTraffic(item) ? item.recent ?? [] : probeSamples(item)
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

function metricDisplay(window: keyof typeof probeWindowMinutes, stat?: OpsAccountHealthWindowStats | null): HealthDisplay {
  if (stat && stat.request_count > 0) {
    return {
      label: window,
      text: statText(stat),
      className: successClass(stat.success_rate_percent)
    }
  }

  const summary = probeSummaryForWindow(props.item, window)
  if (summary.count > 0) {
    return {
      label: `${window} 探`,
      text: `${formatPercent(summary.successRatePercent)} / ${summary.count}次`,
      className: successClass(summary.successRatePercent)
    }
  }

  return {
    label: window,
    text: '暂无',
    className: 'text-gray-400 dark:text-dark-400'
  }
}

function successClass(successRatePercent: number): string {
  if (!Number.isFinite(successRatePercent)) return 'text-gray-400 dark:text-dark-400'
  if (successRatePercent >= 98) return 'text-emerald-600 dark:text-emerald-300'
  if (successRatePercent >= 90) return 'text-amber-600 dark:text-amber-300'
  return 'text-red-600 dark:text-red-300'
}

function sampleClass(sample: OpsAccountHealthSample | null): string {
  if (!sample) return 'bg-gray-200 dark:bg-dark-700'
  if (sample.kind === 'success') return 'bg-emerald-500'
  if (sample.status_code === 429 || sample.status_code === 529) return 'bg-amber-500'
  return 'bg-red-500'
}

function hasTraffic(item?: OpsAccountHealthItem | null): boolean {
  if (!item?.windows) return false
  return Object.values(item.windows).some(stat => (stat?.request_count ?? 0) > 0)
}

function probeSamples(item?: OpsAccountHealthItem | null): OpsAccountHealthSample[] {
  const raw = item?.probe?.recent?.length ? item.probe.recent : syntheticProbeSamples(item)
  return raw.slice().sort((left, right) => sampleTimestamp(left) - sampleTimestamp(right))
}

function syntheticProbeSamples(item?: OpsAccountHealthItem | null): OpsAccountHealthSample[] {
  const probe = item?.probe
  if (!probe?.checked_at) return []
  return [{
    kind: probe.status === 'success' ? 'success' : 'error',
    created_at: probe.checked_at,
    model: probe.model_id,
    duration_ms: probe.latency_ms ? Number(probe.latency_ms) : null,
    message: probe.error_message || '主动探测'
  }]
}

function probeSummaryForWindow(item: OpsAccountHealthItem | null | undefined, window: keyof typeof probeWindowMinutes) {
  const cutoff = Date.now() - probeWindowMinutes[window] * 60_000
  const samples = probeSamples(item).filter(sample => sampleTimestamp(sample) >= cutoff)
  const successCount = samples.filter(sample => sample.kind === 'success').length
  const count = samples.length

  return {
    count,
    successRatePercent: count > 0 ? (successCount / count) * 100 : 0
  }
}

function sampleTimestamp(sample: OpsAccountHealthSample): number {
  const value = new Date(sample.created_at).getTime()
  return Number.isFinite(value) ? value : 0
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
