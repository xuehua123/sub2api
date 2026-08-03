import type { ConnectivityGrade, ConnectivityRunResult } from './types'

const cacheKey = 'sub2api_connectivity_results'
const cacheTTL = 30 * 60 * 1000
const grades = new Set<ConnectivityGrade>(['excellent', 'good', 'fair', 'not_recommended'])

export interface CachedConnectivityGrade {
  url: string
  grade: ConnectivityGrade
  tested_at: number
  grading_version: string
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
  return typeof record.url === 'string'
    && record.url.length > 0
    && typeof record.grade === 'string'
    && grades.has(record.grade as ConnectivityGrade)
    && typeof record.tested_at === 'number'
    && Number.isFinite(record.tested_at)
    && typeof record.grading_version === 'string'
    && record.grading_version.length > 0
}
