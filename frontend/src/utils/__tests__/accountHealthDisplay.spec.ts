import { describe, expect, it } from 'vitest'

import { mergedTimelineSamples, sampleClass } from '../accountHealthDisplay'

describe('accountHealthDisplay sampleClass', () => {
  it('uses green for success, violet for 429/529, red for other failures', () => {
    expect(sampleClass({ kind: 'success', created_at: new Date().toISOString() })).toContain('emerald')
    expect(
      sampleClass({ kind: 'error', status_code: 429, created_at: new Date().toISOString() })
    ).toContain('violet')
    expect(
      sampleClass({ kind: 'error', status_code: 529, created_at: new Date().toISOString() })
    ).toContain('violet')
    expect(
      sampleClass({ kind: 'error', status_code: 500, created_at: new Date().toISOString() })
    ).toContain('red')
  })
})

describe('accountHealthDisplay timeline merge', () => {
  it('merges traffic and probe samples so closed accounts still have bars', () => {
    const now = Date.now()
    const samples = mergedTimelineSamples(
      {
        recent: [{ kind: 'success', created_at: new Date(now - 10_000).toISOString() }],
        probe: {
          status: 'success',
          checked_at: new Date(now - 5_000).toISOString(),
          recent: [{ kind: 'success', created_at: new Date(now - 5_000).toISOString(), message: '主动探测' }]
        }
      } as any,
      60
    )

    expect(samples).toHaveLength(2)
    expect(samples.some((s) => s.source === 'traffic')).toBe(true)
    expect(samples.some((s) => s.source === 'probe')).toBe(true)
    // Probe success uses the same green as traffic success.
    expect(sampleClass(samples.find((s) => s.source === 'probe')!)).toContain('emerald')
    expect(sampleClass(samples.find((s) => s.source === 'traffic')!)).toContain('emerald')
  })

  it('falls back to synthetic probe sample when history is empty', () => {
    const samples = mergedTimelineSamples(
      {
        recent: [],
        probe: {
          status: 'failed',
          checked_at: new Date().toISOString(),
          latency_ms: 1200,
          error_message: 'timeout'
        }
      } as any,
      60
    )
    expect(samples).toHaveLength(1)
    expect(samples[0].source).toBe('probe')
    expect(samples[0].kind).toBe('error')
    // Probe failure uses the same red as traffic failure.
    expect(sampleClass(samples[0])).toContain('red')
  })
})
