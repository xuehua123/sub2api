import type {
  OpsAccountHealthFirstTokenStats,
  OpsAccountHealthItem,
  OpsAccountHealthSample,
  OpsAccountHealthTrend,
  OpsAccountHealthWindow,
  OpsAccountHealthWindowStats
} from '@/api/admin/ops'

export const ACCOUNT_HEALTH_PRIMARY_WINDOWS: OpsAccountHealthWindow[] = ['1m', '5m', '30m', '1h']
export const ACCOUNT_HEALTH_ALL_WINDOWS: OpsAccountHealthWindow[] = ['1m', '5m', '10m', '30m', '1h']

export type HealthTimelineSource = 'traffic' | 'probe'

export type HealthTimelineSample = OpsAccountHealthSample & {
  source: HealthTimelineSource
}

export function formatPercent(value: number | null | undefined, digits = 1): string {
  if (!Number.isFinite(value as number)) return '—'
  return `${Number(value).toFixed(digits)}%`
}

export function formatCount(value: number | null | undefined): string {
  if (!Number.isFinite(value as number)) return '0'
  return String(Math.max(0, Math.round(Number(value))))
}

export function formatMs(value: number | null | undefined): string {
  if (!Number.isFinite(value as number)) return '—'
  const n = Number(value)
  if (n >= 1000) return `${(n / 1000).toFixed(n >= 10000 ? 0 : 1)}s`
  return `${Math.round(n)}ms`
}

export function successClass(successRatePercent: number | null | undefined): string {
  if (!Number.isFinite(successRatePercent as number)) return 'text-gray-400 dark:text-dark-400'
  if ((successRatePercent as number) >= 98) return 'text-emerald-600 dark:text-emerald-300'
  if ((successRatePercent as number) >= 90) return 'text-amber-600 dark:text-amber-300'
  return 'text-red-600 dark:text-red-300'
}

export function windowStats(
  item: OpsAccountHealthItem | null | undefined,
  window: OpsAccountHealthWindow | string
): OpsAccountHealthWindowStats | null {
  return item?.windows?.[window] ?? null
}

export function firstTokenStats(
  item: OpsAccountHealthItem | null | undefined,
  window: OpsAccountHealthWindow | string
): OpsAccountHealthFirstTokenStats | null {
  return item?.first_token_windows?.[window] ?? (window === '5m' ? item?.first_token_5m ?? null : null)
}

export function trendArrow(trend?: OpsAccountHealthTrend | null): string {
  switch (trend?.direction) {
    case 'up':
      return '↑'
    case 'down':
      return '↓'
    case 'flat':
      return '→'
    default:
      return ''
  }
}

export function trendClass(trend?: OpsAccountHealthTrend | null): string {
  switch (trend?.direction) {
    case 'up':
      return 'text-emerald-600 dark:text-emerald-300'
    case 'down':
      return 'text-red-600 dark:text-red-300'
    case 'flat':
      return 'text-gray-500 dark:text-gray-400'
    default:
      return 'text-gray-400 dark:text-dark-400'
  }
}

export function probeStatusLabel(item?: OpsAccountHealthItem | null): string {
  if (!item) return '—'
  if (item.probe_auto_disabled) return '探测停'
  const probe = item.probe
  if (!probe?.checked_at) return '未探测'
  if (probe.status === 'success') {
    return probe.latency_ms != null ? `探OK ${formatMs(probe.latency_ms)}` : '探OK'
  }
  return '探失败'
}

export function sampleTimestamp(sample: OpsAccountHealthSample): number {
  const value = new Date(sample.created_at).getTime()
  return Number.isFinite(value) ? value : 0
}

/** Synthetic probe sample from the latest probe result when history is empty. */
export function syntheticProbeSamples(item?: OpsAccountHealthItem | null): OpsAccountHealthSample[] {
  const probe = item?.probe
  if (!probe?.checked_at) return []
  return [
    {
      kind: probe.status === 'success' ? 'success' : 'error',
      created_at: probe.checked_at,
      model: probe.model_id,
      duration_ms: probe.latency_ms != null ? Number(probe.latency_ms) : null,
      message: probe.error_message || '主动探测'
    }
  ]
}

/**
 * Merge real traffic samples with probe samples so closed / idle accounts still
 * show a useful recent-60 bar (probe results otherwise only live under probe.recent).
 */
export function mergedTimelineSamples(
  item?: OpsAccountHealthItem | null,
  limit = 60
): HealthTimelineSample[] {
  if (!item) return []

  const traffic: HealthTimelineSample[] = (item.recent ?? []).map((sample) => ({
    ...sample,
    source: 'traffic' as const
  }))

  const probeRaw =
    item.probe?.recent && item.probe.recent.length > 0
      ? item.probe.recent
      : syntheticProbeSamples(item)

  const probes: HealthTimelineSample[] = probeRaw.map((sample) => ({
    ...sample,
    source: 'probe' as const,
    message: sample.message || '主动探测'
  }))

  // Dedupe identical timestamp+kind+source from probe synthetic vs history.
  const seen = new Set<string>()
  const merged: HealthTimelineSample[] = []
  for (const sample of [...traffic, ...probes]) {
    const key = `${sample.source}|${sample.created_at}|${sample.kind}|${sample.status_code ?? ''}|${sample.message ?? ''}`
    if (seen.has(key)) continue
    seen.add(key)
    merged.push(sample)
  }

  merged.sort((a, b) => sampleTimestamp(a) - sampleTimestamp(b))
  if (merged.length <= limit) return merged
  return merged.slice(merged.length - limit)
}

export function paddedTimelineSamples(
  item?: OpsAccountHealthItem | null,
  limit = 60
): Array<HealthTimelineSample | null> {
  const samples: Array<HealthTimelineSample | null> = [...mergedTimelineSamples(item, limit)]
  while (samples.length < limit) samples.unshift(null)
  return samples
}

export function sampleClass(sample: HealthTimelineSample | OpsAccountHealthSample | null | undefined): string {
  if (!sample) return 'bg-gray-200 dark:bg-dark-700'
  // Request and probe share the same palette: success green, 429/529 violet, failure red.
  // Source is only used in tooltips / titles, not bar color.
  if (sample.kind === 'success') return 'bg-emerald-500'
  if (sample.status_code === 429 || sample.status_code === 529) return 'bg-violet-500'
  return 'bg-red-500'
}

export function sampleTitle(sample: HealthTimelineSample | OpsAccountHealthSample | null | undefined): string {
  if (!sample) return ''
  const source = (sample as HealthTimelineSample).source
  const parts = [
    source === 'probe' ? '探测' : '请求',
    sample.kind,
    sample.created_at,
    sample.model,
    sample.status_code != null ? `HTTP ${sample.status_code}` : '',
    sample.duration_ms != null ? formatMs(sample.duration_ms) : '',
    sample.message || ''
  ].filter(Boolean)
  return parts.join(' · ')
}

export function hasTraffic(item?: OpsAccountHealthItem | null): boolean {
  if (!item?.windows) return false
  return Object.values(item.windows).some((stat) => (stat?.request_count ?? 0) > 0)
}

export function actionLabel(action?: string): string {
  switch (action) {
    case 'keep_open':
      return '保持开启'
    case 'watch':
      return '观察'
    case 'close_now':
      return '建议关闭'
    case 'can_open':
      return '可打开'
    case 'needs_probe':
      return '需探测'
    case 'keep_closed':
      return '保持关闭'
    default:
      return '查看'
  }
}
