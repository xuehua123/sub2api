import type { ConnectivityGrade, ConnectivityRunResult } from './types'

const cacheKey = 'sub2api_connectivity_results'
const cacheTTL = 30 * 60 * 1000
const grades = new Set<ConnectivityGrade>(['excellent', 'good', 'fair', 'not_recommended'])
// The cache deliberately stores only URL / grade / median latency / timestamp /
// grading version. Client IPs, regions, raw samples, P95 and MAD are never
// persisted in the browser.
const maxCachedMedianMS = 60 * 60 * 1000

export interface CachedConnectivityGrade {
  url: string
  grade: ConnectivityGrade
  tested_at: number
  grading_version: string
  median_ms?: number
}

export function saveConnectivityCache(result: ConnectivityRunResult): void {
  const records: CachedConnectivityGrade[] = []
  const seen = new Set<string>()
  for (const item of result.endpoints) {
    if (item.status !== 'graded' || !item.grade || seen.has(item.endpoint.api_url)) continue
    seen.add(item.endpoint.api_url)
    records.push({
      url: item.endpoint.api_url,
      grade: item.grade,
      tested_at: result.testedAt,
      grading_version: result.gradingVersion,
      // Only persist a rounded median when at least one sample succeeded;
      // grading reports medianMs = 0 for a fully-failed run, which is not a
      // real latency and must not be cached.
      median_ms: item.metrics && item.metrics.successRate > 0 && Number.isFinite(item.metrics.medianMs)
        ? Math.round(item.metrics.medianMs)
        : undefined,
    })
  }
  try {
    if (records.length === 0) {
      sessionStorage.removeItem(cacheKey)
      return
    }
    sessionStorage.setItem(cacheKey, JSON.stringify(records))
  } catch {
    // Storage can be unavailable in private or restricted browser contexts.
  }
}

export function loadConnectivityCache(gradingVersion: string, now = Date.now()): CachedConnectivityGrade[] {
  try {
    const raw = sessionStorage.getItem(cacheKey)
    if (!raw) return []
    const value: unknown = JSON.parse(raw)
    if (!Array.isArray(value) || !value.every(isCachedConnectivityGrade)) {
      sessionStorage.removeItem(cacheKey)
      return []
    }
    const records = value as CachedConnectivityGrade[]
    if (records.some((record) => (
      record.grading_version !== gradingVersion
      || record.tested_at > now
      || now - record.tested_at >= cacheTTL
    ))) {
      sessionStorage.removeItem(cacheKey)
      return []
    }
    return records.map((record) => ({ ...record }))
  } catch {
    try {
      sessionStorage.removeItem(cacheKey)
    } catch {
      // Ignore unavailable storage.
    }
    return []
  }
}

export function clearConnectivityCache(): void {
  try {
    sessionStorage.removeItem(cacheKey)
  } catch {
    // Ignore unavailable storage.
  }
}

function isCachedConnectivityGrade(value: unknown): value is CachedConnectivityGrade {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const record = value as Record<string, unknown>
  if (
    typeof record.url !== 'string'
    || record.url.length === 0
    || typeof record.grade !== 'string'
    || !grades.has(record.grade as ConnectivityGrade)
    || typeof record.tested_at !== 'number'
    || !Number.isFinite(record.tested_at)
    || typeof record.grading_version !== 'string'
    || record.grading_version.length === 0
  ) return false
  // median_ms is optional for backward compatibility with pre-upgrade caches.
  if (record.median_ms === undefined || record.median_ms === null) return true
  return typeof record.median_ms === 'number'
    && Number.isFinite(record.median_ms)
    && record.median_ms >= 0
    && record.median_ms <= maxCachedMedianMS
}
