import { describe, expect, it } from 'vitest'
import {
  buildConnectivityOriginOptions,
  connectivityEligibleEndpointCount,
  connectivityRequestBudget,
  createDefaultConnectivityGradeThresholds,
  normalizeConnectivityAllowedOrigins,
  normalizeConnectivityGradeThresholds,
  validateConnectivitySettings,
} from '../admin'

describe('connectivity admin settings', () => {
  it('derives exact origins from public API URLs, deduplicates them, and keeps stale settings visible', () => {
    const options = buildConnectivityOriginOptions(
      'https://api.example.com/v1',
      [
        { name: '同源路径', endpoint: 'https://api.example.com/compatible', description: '' },
        { name: '备用', endpoint: 'https://alt.example.com/v1', description: '' },
      ],
      ['https://api.example.com/', 'https://stale.example.com'],
    )

    expect(options).toEqual([
      {
        origin: 'https://api.example.com',
        name: '',
        isDefault: true,
        available: true,
      },
      {
        origin: 'https://alt.example.com',
        name: '备用',
        isDefault: false,
        available: true,
      },
      {
        origin: 'https://stale.example.com',
        name: '',
        isDefault: false,
        available: false,
      },
    ])
    expect(normalizeConnectivityAllowedOrigins([
      ' https://api.example.com/ ',
      'https://api.example.com',
    ])).toEqual(['https://api.example.com'])
  })

  it('offers and validates HTTPS origins only, matching the backend contract', () => {
    expect(buildConnectivityOriginOptions(
      'http://api.example.com/v1',
      [{ name: '安全端点', endpoint: 'https://secure.example.com/v1', description: '' }],
      [],
    )).toEqual([{
      origin: 'https://secure.example.com',
      name: '安全端点',
      isDefault: false,
      available: true,
    }])

    expect(validateConnectivitySettings({
      enabled: false,
      thresholds: createDefaultConnectivityGradeThresholds(),
      samples: 10,
      warmup: 1,
      maxConcurrency: 3,
      timeoutMs: 10000,
      allowedOrigins: ['http://api.example.com'],
      ipRpm: 360,
      burst: 250,
      eligibleOriginCount: 0,
      eligibleEndpointCount: 0,
    })).toBe('allowedOriginInvalid')
  })

  it('fills missing threshold fields without sharing the default object', () => {
    const first = normalizeConnectivityGradeThresholds({
      grading_version: 'custom',
      excellent: { max_p95_ms: 200 },
    })
    const second = normalizeConnectivityGradeThresholds(undefined)

    expect(first).toEqual({
      ...createDefaultConnectivityGradeThresholds(),
      grading_version: 'custom',
      excellent: {
        ...createDefaultConnectivityGradeThresholds().excellent,
        max_p95_ms: 200,
      },
    })
    first.excellent.max_p95_ms = 999
    expect(second.excellent.max_p95_ms).toBe(250)
  })

  it('enforces the same ranges, monotonic thresholds, and request budget as the backend', () => {
    const valid = {
      enabled: true,
      thresholds: createDefaultConnectivityGradeThresholds(),
      samples: 10,
      warmup: 1,
      maxConcurrency: 3,
      timeoutMs: 10000,
      allowedOrigins: ['https://api.example.com'],
      ipRpm: 360,
      burst: 250,
      eligibleOriginCount: 1,
      eligibleEndpointCount: 1,
    }

    expect(validateConnectivitySettings(valid)).toBeNull()
    expect(validateConnectivitySettings({
      ...valid,
      thresholds: {
        ...valid.thresholds,
        excellent: { ...valid.thresholds.excellent, min_success_rate: 0.9 },
      },
    })).toBe('successRateOrder')
    expect(validateConnectivitySettings({
      ...valid,
      samples: 20,
      warmup: 2,
      eligibleOriginCount: 12,
    })).toBe('requestBudgetExceeded')
    expect(connectivityRequestBudget(12, 20, 2)).toBe(264)
    expect(validateConnectivitySettings({
      ...valid,
      eligibleOriginCount: 2,
      burst: 21,
    })).toBe('burstBelowRequestBudget')
  })

  it('counts only unique URLs on selected origins and rejects more than eleven', () => {
    const endpoints = Array.from({ length: 12 }, (_, index) => ({
      name: `端点 ${index + 1}`,
      endpoint: `https://api.example.com/v${index + 1}`,
      description: '',
    }))
    const eligibleEndpointCount = connectivityEligibleEndpointCount(
      '',
      endpoints,
      ['https://api.example.com'],
    )
    expect(eligibleEndpointCount).toBe(12)

    expect(validateConnectivitySettings({
      enabled: true,
      thresholds: createDefaultConnectivityGradeThresholds(),
      samples: 10,
      warmup: 1,
      maxConcurrency: 3,
      timeoutMs: 10000,
      allowedOrigins: ['https://api.example.com'],
      ipRpm: 360,
      burst: 250,
      eligibleOriginCount: 1,
      eligibleEndpointCount,
    })).toBe('endpointLimitExceeded')
  })

  it('does not count ambiguous paths that the backend rejects', () => {
    for (const endpoint of [
      'https://api.example.com/v1%2Fadmin',
      String.raw`https://api.example.com/v1\admin`,
      'https://api.example.com/v1/../admin',
      'https://api.example.com/v1/./models',
      'https://api.example.com/v 1',
      'https://api.example.com/模型',
    ]) {
      expect(connectivityEligibleEndpointCount(
        endpoint,
        [],
        ['https://api.example.com'],
      )).toBe(0)
    }

    expect(normalizeConnectivityAllowedOrigins(['https://api.example.com/./']))
      .toEqual(['https://api.example.com/./'])
  })
})
