import type { PublicSettings } from '@/types'
import type {
  ConnectivityGradeThreshold,
  ConnectivityGradeThresholds,
  ConnectivityProbeConfig,
  ConnectivityTestEndpoint,
} from './types'
import { hasUnsafeConnectivityURLSyntax } from './url'

const probePath = '/.well-known/sub2api/edge-probe'

export function parseConnectivityProbeConfig(settings: Partial<PublicSettings>): ConnectivityProbeConfig | null {
  if (settings.connectivity_test_enabled !== true) return null
  const samples = integerInRange(settings.connectivity_probe_samples, 5, 20)
  const warmup = integerInRange(settings.connectivity_probe_warmup, 0, 2)
  const maxConcurrency = integerInRange(settings.connectivity_probe_max_concurrency, 1, 3)
  const timeoutMs = integerInRange(settings.connectivity_probe_timeout_ms, 2000, 15000)
  const thresholds = parseThresholds(settings.connectivity_grade_thresholds)
  const endpoints = parseEndpoints(settings.connectivity_test_endpoints)
  if (samples === null || warmup === null || maxConcurrency === null || timeoutMs === null || !thresholds || !endpoints) {
    return null
  }
  const origins = new Set(endpoints.map((endpoint) => new URL(endpoint.probe_url).origin))
  if (origins.size * (samples + warmup) > 250) return null

  return {
    endpoints,
    thresholds,
    samples,
    warmup,
    maxConcurrency,
    timeoutMs,
    clientIPEnabled: settings.connectivity_client_ip_enabled === true,
  }
}

function integerInRange(value: unknown, minimum: number, maximum: number): number | null {
  return typeof value === 'number' && Number.isInteger(value) && value >= minimum && value <= maximum
    ? value
    : null
}

function parseThresholds(value: unknown): ConnectivityGradeThresholds | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  const thresholds = value as Partial<ConnectivityGradeThresholds>
  if (typeof thresholds.grading_version !== 'string' || thresholds.grading_version.trim() === '') return null
  if (!rateIncludingZero(thresholds.minimum_success_rate) || !integerInRange(thresholds.max_consecutive_timeouts, 1, 20)) return null
  const excellent = parseGradeThreshold(thresholds.excellent)
  const good = parseGradeThreshold(thresholds.good)
  if (!excellent || !good) return null
  if (
    excellent.min_success_rate <= good.min_success_rate
    || good.min_success_rate <= thresholds.minimum_success_rate!
    || excellent.max_p95_ms >= good.max_p95_ms
    || excellent.max_mad_ms >= good.max_mad_ms
  ) return null
  return {
    grading_version: thresholds.grading_version,
    minimum_success_rate: thresholds.minimum_success_rate!,
    max_consecutive_timeouts: thresholds.max_consecutive_timeouts!,
    excellent,
    good,
  }
}

function parseGradeThreshold(value: unknown): ConnectivityGradeThreshold | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  const threshold = value as Partial<ConnectivityGradeThreshold>
  if (!rate(threshold.min_success_rate) || !positiveFinite(threshold.max_p95_ms) || !nonNegativeFinite(threshold.max_mad_ms)) {
    return null
  }
  return {
    min_success_rate: threshold.min_success_rate!,
    max_p95_ms: threshold.max_p95_ms!,
    max_mad_ms: threshold.max_mad_ms!,
  }
}

function rate(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 && value <= 1
}

function rateIncludingZero(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 && value <= 1
}

function positiveFinite(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
}

function nonNegativeFinite(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
}

function parseEndpoints(value: unknown): ConnectivityTestEndpoint[] | null {
  if (!Array.isArray(value) || value.length < 1 || value.length > 11) return null
  const endpoints: ConnectivityTestEndpoint[] = []
  const seenURLs = new Set<string>()
  for (const candidate of value) {
    if (!candidate || typeof candidate !== 'object' || Array.isArray(candidate)) return null
    const endpoint = candidate as Partial<ConnectivityTestEndpoint>
    if (
      typeof endpoint.name !== 'string' || endpoint.name.trim() === ''
      || typeof endpoint.api_url !== 'string'
      || typeof endpoint.probe_url !== 'string'
      || typeof endpoint.is_default !== 'boolean'
    ) return null
    let apiURL: URL
    let probeURL: URL
    try {
      if (
        hasUnsafeConnectivityURLSyntax(endpoint.api_url)
        || hasUnsafeConnectivityURLSyntax(endpoint.probe_url)
      ) return null
      apiURL = new URL(endpoint.api_url)
      probeURL = new URL(endpoint.probe_url)
    } catch {
      return null
    }
    if (
      apiURL.protocol !== 'https:' || probeURL.protocol !== 'https:'
      || apiURL.username !== '' || apiURL.password !== ''
      || probeURL.username !== '' || probeURL.password !== ''
      || apiURL.search !== '' || apiURL.hash !== ''
      || probeURL.search !== '' || probeURL.hash !== ''
      || apiURL.pathname.includes('%')
      || probeURL.pathname !== probePath
      || probeURL.origin !== apiURL.origin
      || seenURLs.has(apiURL.toString())
    ) return null
    seenURLs.add(apiURL.toString())
    endpoints.push({
      name: endpoint.name.trim(),
      api_url: endpoint.api_url,
      probe_url: endpoint.probe_url,
      is_default: endpoint.is_default,
    })
  }
  return endpoints
}
