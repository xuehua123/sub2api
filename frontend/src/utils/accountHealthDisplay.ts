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
  if (item.probe_auto_disabled) return '探测已停'
  const probe = item.probe
  if (!probe?.checked_at) return '未探测'
  if (probe.status === 'success') {
    return probe.latency_ms != null ? `探测OK ${formatMs(probe.latency_ms)}` : '探测OK'
  }
  return '探测失败'
}

export function sampleClass(sample: OpsAccountHealthSample | null | undefined): string {
  if (!sample) return 'bg-gray-200 dark:bg-dark-700'
  if (sample.kind === 'success') return 'bg-emerald-500'
  if (sample.status_code === 429 || sample.status_code === 529) return 'bg-amber-500'
  return 'bg-red-500'
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
