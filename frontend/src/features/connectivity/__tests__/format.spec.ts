import { describe, expect, it } from 'vitest'
import {
  formatClientLocation,
  formatTypicalLatency,
  summarizeNetworkExit,
} from '../format'
import type { ConnectivityEndpointResult } from '../types'

function endpointResult(overrides: Partial<ConnectivityEndpointResult> = {}): ConnectivityEndpointResult {
  return {
    endpoint: {
      name: '默认',
      api_url: 'https://a.example/v1',
      probe_url: 'https://a.example/.well-known/sub2api/edge-probe',
      is_default: true,
    },
    status: 'graded',
    grade: 'excellent',
    clientIP: '8.8.8.8',
    clientLocation: { country_code: 'CN', country: '中国', region: '广东', city: '深圳' },
    ...overrides,
  }
}

describe('formatTypicalLatency', () => {
  it('rounds the median to an integer', () => {
    expect(formatTypicalLatency(85.6)).toBe(86)
    expect(formatTypicalLatency(0)).toBe(0)
  })

  it('returns null when there is no usable latency', () => {
    expect(formatTypicalLatency(null)).toBeNull()
    expect(formatTypicalLatency(undefined)).toBeNull()
    expect(formatTypicalLatency(Number.NaN)).toBeNull()
    expect(formatTypicalLatency(-1)).toBeNull()
    expect(formatTypicalLatency(Number.POSITIVE_INFINITY)).toBeNull()
  })
})

describe('formatClientLocation', () => {
  it('joins non-empty country, region, and city parts', () => {
    expect(formatClientLocation({ country_code: 'CN', country: '中国', region: '广东', city: '深圳' }))
      .toBe('中国 · 广东 · 深圳')
  })

  it('returns an empty string when every part is empty', () => {
    expect(formatClientLocation({ country_code: '', country: '', region: '', city: '' })).toBe('')
    expect(formatClientLocation(null)).toBe('')
  })

  it('skips empty parts', () => {
    expect(formatClientLocation({ country_code: 'CN', country: '中国', region: '', city: '' })).toBe('中国')
  })

  it('falls back to the ISO country code when the localized country is missing', () => {
    expect(formatClientLocation({ country_code: 'CN', country: '', region: '', city: '' })).toBe('CN')
  })
})

describe('summarizeNetworkExit', () => {
  it('is none when IP display is disabled', () => {
    expect(summarizeNetworkExit(false, [endpointResult()])).toEqual({ mode: 'none' })
  })

  it('is none with no results', () => {
    expect(summarizeNetworkExit(true, [])).toEqual({ mode: 'none' })
  })

  it('summarizes a single common egress with a consistent location', () => {
    const summary = summarizeNetworkExit(true, [endpointResult()])
    expect(summary).toEqual({
      mode: 'common',
      ip: '8.8.8.8',
      location: { country_code: 'CN', country: '中国', region: '广东', city: '深圳' },
    })
  })

  it('hides the region while keeping the IP when locations disagree', () => {
    const first = endpointResult()
    const second = endpointResult({
      endpoint: { name: '备用', api_url: 'https://b.example/v1', probe_url: 'https://b.example/.well-known/sub2api/edge-probe', is_default: false },
      clientLocation: { country_code: 'CN', country: '中国', region: '广东', city: '广州' },
    })
    const summary = summarizeNetworkExit(true, [first, second])
    expect(summary.mode).toBe('common')
    if (summary.mode === 'common') {
      expect(summary.ip).toBe('8.8.8.8')
      expect(summary.location).toBeNull()
    }
  })

  it('reports split when two distinct IPs are observed', () => {
    const first = endpointResult()
    const second = endpointResult({
      endpoint: { name: '备用', api_url: 'https://b.example/v1', probe_url: 'https://b.example/.well-known/sub2api/edge-probe', is_default: false },
      clientIP: '45.77.18.20',
      clientLocation: { country_code: 'HK', country: '中国香港', region: '', city: '' },
    })
    expect(summarizeNetworkExit(true, [first, second])).toEqual({ mode: 'split' })
  })

  it('renders no egress banner at all when the run is incomplete', () => {
    const first = endpointResult()
    const second = endpointResult({ status: 'incomplete', clientIP: null, clientLocation: null })
    expect(summarizeNetworkExit(true, [first, second])).toEqual({ mode: 'none' })
  })

  it('shows "egress unknown" when every graded IP is missing', () => {
    const first = endpointResult({ clientIP: null, clientLocation: null })
    const second = endpointResult({
      endpoint: { name: '备用', api_url: 'https://b.example/v1', probe_url: 'https://b.example/.well-known/sub2api/edge-probe', is_default: false },
      clientIP: null,
      clientLocation: null,
    })
    expect(summarizeNetworkExit(true, [first, second])).toEqual({ mode: 'unknown' })
  })

  it('shows "egress unknown" when some graded IP is missing', () => {
    const first = endpointResult()
    const second = endpointResult({
      endpoint: { name: '备用', api_url: 'https://b.example/v1', probe_url: 'https://b.example/.well-known/sub2api/edge-probe', is_default: false },
      clientIP: null,
      clientLocation: null,
    })
    expect(summarizeNetworkExit(true, [first, second])).toEqual({ mode: 'unknown' })
  })
})
