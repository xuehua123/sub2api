import type { CustomEndpoint } from '@/types'
import type { ConnectivityGradeThresholds } from './types'
import { hasUnsafeConnectivityURLSyntax } from './url'

export interface ConnectivityOriginOption {
  origin: string
  name: string
  isDefault: boolean
  available: boolean
}

export type ConnectivityValidationError =
  | 'allowedOriginInvalid'
  | 'allowedOriginRequired'
  | 'samplesRange'
  | 'warmupRange'
  | 'concurrencyRange'
  | 'timeoutRange'
  | 'ipRpmRange'
  | 'burstRange'
  | 'gradingVersionInvalid'
  | 'successRateOrder'
  | 'p95Order'
  | 'madOrder'
  | 'consecutiveTimeoutsRange'
  | 'endpointLimitExceeded'
  | 'requestBudgetExceeded'
  | 'burstBelowRequestBudget'

export interface ConnectivityValidationInput {
  enabled: boolean
  thresholds: ConnectivityGradeThresholds
  samples: number
  warmup: number
  maxConcurrency: number
  timeoutMs: number
  allowedOrigins: string[]
  ipRpm: number
  burst: number
  eligibleOriginCount: number
  eligibleEndpointCount: number
}

export function createDefaultConnectivityGradeThresholds(): ConnectivityGradeThresholds {
  return {
    grading_version: '1',
    minimum_success_rate: 0.8,
    max_consecutive_timeouts: 2,
    excellent: {
      min_success_rate: 1,
      max_p95_ms: 250,
      max_mad_ms: 50,
    },
    good: {
      min_success_rate: 0.9,
      max_p95_ms: 500,
      max_mad_ms: 120,
    },
  }
}

export function normalizeConnectivityGradeThresholds(value: unknown): ConnectivityGradeThresholds {
  const fallback = createDefaultConnectivityGradeThresholds()
  if (!value || typeof value !== 'object' || Array.isArray(value)) return fallback

  const source = value as Record<string, unknown>
  const excellent = asRecord(source.excellent)
  const good = asRecord(source.good)
  return {
    grading_version: typeof source.grading_version === 'string'
      ? source.grading_version
      : fallback.grading_version,
    minimum_success_rate: finiteOr(source.minimum_success_rate, fallback.minimum_success_rate),
    max_consecutive_timeouts: finiteOr(
      source.max_consecutive_timeouts,
      fallback.max_consecutive_timeouts,
    ),
    excellent: {
      min_success_rate: finiteOr(
        excellent.min_success_rate,
        fallback.excellent.min_success_rate,
      ),
      max_p95_ms: finiteOr(excellent.max_p95_ms, fallback.excellent.max_p95_ms),
      max_mad_ms: finiteOr(excellent.max_mad_ms, fallback.excellent.max_mad_ms),
    },
    good: {
      min_success_rate: finiteOr(good.min_success_rate, fallback.good.min_success_rate),
      max_p95_ms: finiteOr(good.max_p95_ms, fallback.good.max_p95_ms),
      max_mad_ms: finiteOr(good.max_mad_ms, fallback.good.max_mad_ms),
    },
  }
}

export function normalizeConnectivityAllowedOrigins(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  const seen = new Set<string>()
  const normalized: string[] = []
  for (const entry of value) {
    if (typeof entry !== 'string') continue
    const trimmed = entry.trim()
    if (!trimmed) continue
    const origin = normalizeConnectivityOrigin(trimmed) ?? trimmed
    if (seen.has(origin)) continue
    seen.add(origin)
    normalized.push(origin)
  }
  return normalized
}

export function buildConnectivityOriginOptions(
  apiBaseURL: string,
  customEndpoints: CustomEndpoint[],
  configuredOrigins: string[],
): ConnectivityOriginOption[] {
  const options = new Map<string, ConnectivityOriginOption>()
  const addCandidate = (rawURL: string, name: string, isDefault: boolean) => {
    const origin = connectivityEndpointOrigin(rawURL)
    if (!origin) return
    const existing = options.get(origin)
    if (existing?.isDefault) return
    options.set(origin, {
      origin,
      name: name.trim(),
      isDefault,
      available: true,
    })
  }

  addCandidate(apiBaseURL, '', true)
  for (const endpoint of customEndpoints) {
    addCandidate(endpoint.endpoint, endpoint.name || '', false)
  }

  for (const rawOrigin of configuredOrigins) {
    const origin = normalizeConnectivityOrigin(rawOrigin) ?? rawOrigin
    if (options.has(origin)) continue
    options.set(origin, {
      origin,
      name: '',
      isDefault: false,
      available: false,
    })
  }
  return [...options.values()]
}

export function connectivityRequestBudget(
  eligibleOriginCount: number,
  samples: number,
  warmup: number,
): number {
  if (![eligibleOriginCount, samples, warmup].every(Number.isFinite)) return 0
  return Math.max(0, eligibleOriginCount) * Math.max(0, samples + warmup)
}

export function connectivityEligibleEndpointCount(
  apiBaseURL: string,
  customEndpoints: CustomEndpoint[],
  allowedOrigins: string[],
): number {
  const allowed = new Set(allowedOrigins)
  const uniqueURLs = new Set<string>()
  for (const rawURL of [apiBaseURL, ...customEndpoints.map((endpoint) => endpoint.endpoint)]) {
    const url = parseConnectivityEndpointURL(rawURL)
    if (!url || !allowed.has(url.origin)) continue
    uniqueURLs.add(`${url.origin}${url.pathname.replace(/\/+$/, '')}`)
  }
  return uniqueURLs.size
}

export function validateConnectivitySettings(
  value: ConnectivityValidationInput,
): ConnectivityValidationError | null {
  if (value.allowedOrigins.some((origin) => normalizeConnectivityOrigin(origin) !== origin)) {
    return 'allowedOriginInvalid'
  }
  if (value.enabled && value.eligibleOriginCount < 1) return 'allowedOriginRequired'
  if (!integerInRange(value.samples, 5, 20)) return 'samplesRange'
  if (!integerInRange(value.warmup, 0, 2)) return 'warmupRange'
  if (!integerInRange(value.maxConcurrency, 1, 3)) return 'concurrencyRange'
  if (!integerInRange(value.timeoutMs, 2000, 15000)) return 'timeoutRange'
  if (!integerInRange(value.ipRpm, 1, 10000)) return 'ipRpmRange'
  if (!integerInRange(value.burst, 1, 1000)) return 'burstRange'
  if (!integerInRange(value.eligibleEndpointCount, 0, 11)) return 'endpointLimitExceeded'

  const thresholds = value.thresholds
  if (
    !thresholds.grading_version.trim()
    || thresholds.grading_version.length > 32
  ) {
    return 'gradingVersionInvalid'
  }
  if (
    !finiteInRange(thresholds.minimum_success_rate, 0, 1)
    || !finiteInRange(thresholds.good.min_success_rate, 0, 1)
    || !finiteInRange(thresholds.excellent.min_success_rate, 0, 1)
    || thresholds.good.min_success_rate <= thresholds.minimum_success_rate
    || thresholds.excellent.min_success_rate <= thresholds.good.min_success_rate
  ) {
    return 'successRateOrder'
  }
  if (
    !Number.isFinite(thresholds.excellent.max_p95_ms)
    || !Number.isFinite(thresholds.good.max_p95_ms)
    || thresholds.excellent.max_p95_ms <= 0
    || thresholds.good.max_p95_ms <= thresholds.excellent.max_p95_ms
  ) {
    return 'p95Order'
  }
  if (
    !Number.isFinite(thresholds.excellent.max_mad_ms)
    || !Number.isFinite(thresholds.good.max_mad_ms)
    || thresholds.excellent.max_mad_ms < 0
    || thresholds.good.max_mad_ms <= thresholds.excellent.max_mad_ms
  ) {
    return 'madOrder'
  }
  if (!integerInRange(thresholds.max_consecutive_timeouts, 1, 20)) {
    return 'consecutiveTimeoutsRange'
  }
  const requestBudget = connectivityRequestBudget(value.eligibleOriginCount, value.samples, value.warmup)
  if (requestBudget > 250) {
    return 'requestBudgetExceeded'
  }
  if (value.enabled && value.burst < requestBudget) return 'burstBelowRequestBudget'
  return null
}

export function connectivityThresholdFingerprint(value: ConnectivityGradeThresholds): string {
  return JSON.stringify({
    minimum_success_rate: value.minimum_success_rate,
    max_consecutive_timeouts: value.max_consecutive_timeouts,
    excellent: value.excellent,
    good: value.good,
  })
}

export function nextConnectivityGradingVersion(current: string): string {
  const now = Date.now()
  return current === String(now) ? String(now + 1) : String(now)
}

function connectivityEndpointOrigin(rawURL: string): string | null {
  return parseConnectivityEndpointURL(rawURL)?.origin ?? null
}

function parseConnectivityEndpointURL(rawURL: string): URL | null {
  try {
    const trimmed = rawURL.trim()
    if (hasUnsafeConnectivityURLSyntax(trimmed)) return null
    const url = new URL(trimmed)
    if (
      url.protocol !== 'https:'
      || url.username
      || url.password
      || url.search
      || url.hash
      || url.pathname.includes('%')
    ) {
      return null
    }
    return url
  } catch {
    return null
  }
}

function normalizeConnectivityOrigin(rawOrigin: string): string | null {
  try {
    const trimmed = rawOrigin.trim()
    if (hasUnsafeConnectivityURLSyntax(trimmed)) return null
    const url = new URL(trimmed)
    if (
      url.protocol !== 'https:'
      || url.username
      || url.password
      || url.search
      || url.hash
      || (url.pathname !== '' && url.pathname !== '/')
    ) {
      return null
    }
    return url.origin
  } catch {
    return null
  }
}

function integerInRange(value: number, min: number, max: number): boolean {
  return Number.isInteger(value) && value >= min && value <= max
}

function finiteInRange(value: number, min: number, max: number): boolean {
  return Number.isFinite(value) && value >= min && value <= max
}

function finiteOr(value: unknown, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {}
}
