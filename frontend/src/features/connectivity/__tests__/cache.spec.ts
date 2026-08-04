import { beforeEach, describe, expect, it } from 'vitest'
import {
  clearConnectivityCache,
  loadConnectivityCache,
  saveConnectivityCache,
} from '../cache'
import type { ConnectivityRunResult } from '../types'

describe('connectivity session cache', () => {
  beforeEach(() => sessionStorage.clear())

  it('stores only URL, grade, median latency, test time, and grading version for 30 minutes', () => {
    const result: ConnectivityRunResult = {
      status: 'complete',
      endpoints: [{
        endpoint: { name: '默认', api_url: 'https://api.example/v1', probe_url: 'https://api.example/probe', is_default: true },
        status: 'graded',
        grade: 'excellent',
        metrics: { successRate: 1, p95Ms: 100, medianMs: 90, madMs: 5, maxConsecutiveTimeouts: 0 },
        clientIP: '8.8.8.8',
        clientLocation: { country_code: 'CN', country: '中国', region: '广东', city: '深圳' },
      }],
      testedAt: 1_000_000,
      gradingVersion: '1',
      recommendedAPIURL: 'https://api.example/v1',
    }

    saveConnectivityCache(result)

    const stored = sessionStorage.getItem('sub2api_connectivity_results')!
    expect(stored).not.toContain('8.8.8.8')
    expect(stored).not.toContain('中国')
    expect(stored).not.toContain('p95')
    expect(stored).not.toContain('mad')
    expect(JSON.parse(stored)).toEqual([{
      url: 'https://api.example/v1',
      grade: 'excellent',
      tested_at: 1_000_000,
      grading_version: '1',
      median_ms: 90,
    }])
    expect(loadConnectivityCache('1', 1_000_000 + 30 * 60 * 1000 - 1)).toHaveLength(1)
    expect(loadConnectivityCache('1', 1_000_000 + 30 * 60 * 1000)).toEqual([])
    expect(loadConnectivityCache('2', 1_000_001)).toEqual([])
  })

  it('rounds the median on save and omits it when no sample succeeded', () => {
    const saveWith = (successRate: number, medianMs: number) => {
      const result: ConnectivityRunResult = {
        status: 'complete',
        endpoints: [{
          endpoint: { name: '默认', api_url: 'https://api.example/v1', probe_url: 'https://api.example/probe', is_default: true },
          status: 'graded',
          grade: 'excellent',
          metrics: { successRate, p95Ms: 0, medianMs, madMs: 0, maxConsecutiveTimeouts: 0 },
        }],
        testedAt: 1_000_000,
        gradingVersion: '1',
      }
      saveConnectivityCache(result)
    }

    saveWith(1, 85.6)
    let stored = JSON.parse(sessionStorage.getItem('sub2api_connectivity_results')!) as Array<{ median_ms?: number }>
    expect(stored[0]?.median_ms).toBe(86)

    saveWith(0, 0)
    stored = JSON.parse(sessionStorage.getItem('sub2api_connectivity_results')!) as Array<{ median_ms?: number }>
    expect(stored[0]?.median_ms).toBeUndefined()
  })

  it('loads a pre-upgrade cache record without median_ms and rejects invalid median_ms', () => {
    sessionStorage.setItem('sub2api_connectivity_results', JSON.stringify([{
      url: 'https://api.example/v1',
      grade: 'good',
      tested_at: 1_000_000,
      grading_version: '1',
    }]))
    const loaded = loadConnectivityCache('1', 1_000_001)
    expect(loaded).toHaveLength(1)
    expect(loaded[0].median_ms).toBeUndefined()

    sessionStorage.setItem('sub2api_connectivity_results', JSON.stringify([{
      url: 'https://api.example/v1',
      grade: 'good',
      tested_at: 1_000_000,
      grading_version: '1',
      median_ms: -5,
    }]))
    expect(loadConnectivityCache('1', 1_000_001)).toEqual([])

    sessionStorage.setItem('sub2api_connectivity_results', JSON.stringify([{
      url: 'https://api.example/v1',
      grade: 'good',
      tested_at: 1_000_000,
      grading_version: '1',
      median_ms: 'fast',
    }]))
    expect(loadConnectivityCache('1', 1_000_001)).toEqual([])
  })

  it('clears stale or malformed data without throwing', () => {
    sessionStorage.setItem('sub2api_connectivity_results', '{bad json')
    expect(loadConnectivityCache('1', Date.now())).toEqual([])
    expect(sessionStorage.getItem('sub2api_connectivity_results')).toBeNull()

    sessionStorage.setItem('sub2api_connectivity_results', '[]')
    clearConnectivityCache()
    expect(sessionStorage.getItem('sub2api_connectivity_results')).toBeNull()
  })
})
