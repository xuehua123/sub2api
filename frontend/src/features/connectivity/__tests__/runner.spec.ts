import { describe, expect, it, vi } from 'vitest'
import { runConnectivityTest } from '../runner'
import type { ConnectivityProbeConfig, ProbeAttempt } from '../types'

function config(): ConnectivityProbeConfig {
  return {
    endpoints: [
      { name: '默认', api_url: 'https://a.example/v1', probe_url: 'https://a.example/.well-known/sub2api/edge-probe', is_default: true },
      { name: '同源备用', api_url: 'https://a.example/compatible', probe_url: 'https://a.example/.well-known/sub2api/edge-probe', is_default: false },
      { name: '备用', api_url: 'https://b.example/v1', probe_url: 'https://b.example/.well-known/sub2api/edge-probe', is_default: false },
    ],
    thresholds: {
      grading_version: '1',
      minimum_success_rate: 0.8,
      max_consecutive_timeouts: 2,
      excellent: { min_success_rate: 1, max_p95_ms: 250, max_mad_ms: 50 },
      good: { min_success_rate: 0.9, max_p95_ms: 500, max_mad_ms: 120 },
    },
    samples: 2,
    warmup: 1,
    maxConcurrency: 2,
    timeoutMs: 5000,
    clientIPEnabled: false,
  }
}

describe('runConnectivityTest', () => {
  it('samples unique origins in interleaved rounds and maps results to every display URL', async () => {
    const calls: string[] = []
    let active = 0
    let maxActive = 0
    const probe = vi.fn(async (url: string): Promise<ProbeAttempt> => {
      calls.push(new URL(url).origin)
      active++
      maxActive = Math.max(maxActive, active)
      await Promise.resolve()
      active--
      const durationMs = new URL(url).hostname === 'a.example' ? 100 : 180
      return { kind: 'success', durationMs, clientIP: null }
    })

    const result = await runConnectivityTest(config(), undefined, { probe, now: () => 1234 })

    expect(calls.filter((origin) => origin === 'https://a.example')).toHaveLength(3)
    expect(calls.filter((origin) => origin === 'https://b.example')).toHaveLength(3)
    expect(maxActive).toBeLessThanOrEqual(2)
    expect(result.status).toBe('complete')
    expect(result.endpoints).toHaveLength(3)
    expect(result.endpoints[0].grade).toBe('excellent')
    expect(result.endpoints[1].grade).toBe('excellent')
    expect(result.recommendedAPIURL).toBe('https://a.example/v1')
    expect(result.testedAt).toBe(1234)
  })

  it('does not turn panel-wide network/CORS failure, rate limiting, or cancellation into a bad grade', async () => {
    const networkResult = await runConnectivityTest(config(), undefined, {
      probe: vi.fn().mockResolvedValue({ kind: 'network_or_cors' }),
    })
    expect(networkResult.status).toBe('incomplete')
    expect(networkResult.endpoints.every((item) => item.status === 'incomplete' && !item.grade)).toBe(true)

    const rateLimitResult = await runConnectivityTest(config(), undefined, {
      probe: vi.fn().mockResolvedValue({ kind: 'rate_limited' }),
    })
    expect(rateLimitResult.status).toBe('rate_limited')
    expect(rateLimitResult.endpoints.every((item) => item.status === 'rate_limited')).toBe(true)

    const controller = new AbortController()
    controller.abort()
    const cancelledResult = await runConnectivityTest(config(), controller.signal, {
      probe: vi.fn().mockResolvedValue({ kind: 'cancelled' }),
    })
    expect(cancelledResult.status).toBe('cancelled')
  })

  it('recommends only excellent or good URLs using stability metrics before defaults', async () => {
    const cfg = config()
    cfg.warmup = 0
    const attempts = new Map<string, ProbeAttempt[]>([
      ['a.example', [
        { kind: 'success', durationMs: 200, clientIP: null },
        { kind: 'success', durationMs: 220, clientIP: null },
      ]],
      ['b.example', [
        { kind: 'success', durationMs: 100, clientIP: null },
        { kind: 'success', durationMs: 110, clientIP: null },
      ]],
    ])
    const probe = vi.fn(async (url: string) => attempts.get(new URL(url).hostname)!.shift()!)

    const result = await runConnectivityTest(cfg, undefined, { probe })

    expect(result.recommendedAPIURL).toBe('https://b.example/v1')
  })
})
