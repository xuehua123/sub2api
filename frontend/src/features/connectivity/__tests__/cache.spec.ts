import { beforeEach, describe, expect, it } from 'vitest'
import {
  clearConnectivityCache,
  loadConnectivityCache,
  saveConnectivityCache,
} from '../cache'
import type { ConnectivityRunResult } from '../types'

describe('connectivity session cache', () => {
  beforeEach(() => sessionStorage.clear())

  it('stores only URL, grade, test time, and grading version for 30 minutes', () => {
    const result: ConnectivityRunResult = {
      status: 'complete',
      endpoints: [{
        endpoint: { name: '默认', api_url: 'https://api.example/v1', probe_url: 'https://api.example/probe', is_default: true },
        status: 'graded',
        grade: 'excellent',
        metrics: { successRate: 1, p95Ms: 100, medianMs: 90, madMs: 5, maxConsecutiveTimeouts: 0 },
        clientIP: '8.8.8.8',
      }],
      testedAt: 1_000_000,
      gradingVersion: '1',
      recommendedAPIURL: 'https://api.example/v1',
    }

    saveConnectivityCache(result)

    const stored = sessionStorage.getItem('sub2api_connectivity_results')!
    expect(stored).not.toContain('8.8.8.8')
    expect(stored).not.toContain('p95')
    expect(JSON.parse(stored)).toEqual([{
      url: 'https://api.example/v1',
      grade: 'excellent',
      tested_at: 1_000_000,
      grading_version: '1',
    }])
    expect(loadConnectivityCache('1', 1_000_000 + 30 * 60 * 1000 - 1)).toHaveLength(1)
    expect(loadConnectivityCache('1', 1_000_000 + 30 * 60 * 1000)).toEqual([])
    expect(loadConnectivityCache('2', 1_000_001)).toEqual([])
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
