import { describe, expect, it } from 'vitest'
import {
  calculateMAD,
  calculateMedian,
  calculateNearestRankP95,
  gradeConnectivityAttempts,
} from '../grading'
import type { ConnectivityGradeThresholds, ProbeAttempt } from '../types'

const thresholds: ConnectivityGradeThresholds = {
  grading_version: '1',
  minimum_success_rate: 0.8,
  max_consecutive_timeouts: 2,
  excellent: { min_success_rate: 1, max_p95_ms: 250, max_mad_ms: 50 },
  good: { min_success_rate: 0.9, max_p95_ms: 500, max_mad_ms: 120 },
}

function successes(values: number[]): ProbeAttempt[] {
  return values.map((durationMs) => ({ kind: 'success', durationMs, clientIP: null }))
}

describe('connectivity grading', () => {
  it('uses standard median, nearest-rank P95, and median absolute deviation', () => {
    expect(calculateMedian([1, 2, 9, 10])).toBe(5.5)
    expect(calculateNearestRankP95([10, 30, 20, 40])).toBe(40)
    expect(calculateMAD([1, 2, 9, 10])).toBe(4)
  })

  it('applies mutually exclusive grade boundaries in the required order', () => {
    expect(gradeConnectivityAttempts(successes([100, 120, 140, 160, 180]), 5, thresholds).grade).toBe('excellent')

    const good = successes([220, 250, 270, 300, 320, 340, 360, 380, 400])
    good.push({ kind: 'http_error' })
    expect(gradeConnectivityAttempts(good, 10, thresholds).grade).toBe('good')

    const fair = successes([300, 350, 400, 450, 520, 550, 600, 650])
    fair.push({ kind: 'http_error' }, { kind: 'http_error' })
    expect(gradeConnectivityAttempts(fair, 10, thresholds).grade).toBe('fair')

    const lowSuccess = successes([100, 100, 100, 100, 100, 100, 100])
    lowSuccess.push({ kind: 'http_error' }, { kind: 'http_error' }, { kind: 'http_error' })
    expect(gradeConnectivityAttempts(lowSuccess, 10, thresholds).grade).toBe('not_recommended')
  })

  it('treats protocol, cancellation, incomplete samples, and rate limiting as non-graded', () => {
    expect(gradeConnectivityAttempts([{ kind: 'protocol_error' }], 1, thresholds).status).toBe('incomplete')
    expect(gradeConnectivityAttempts([{ kind: 'cancelled' }], 1, thresholds).status).toBe('cancelled')
    expect(gradeConnectivityAttempts([{ kind: 'rate_limited' }], 1, thresholds).status).toBe('rate_limited')
    expect(gradeConnectivityAttempts(successes([100]), 2, thresholds).status).toBe('incomplete')
  })

  it('marks two consecutive timeouts as not recommended even at the minimum success rate', () => {
    const attempts = successes([100, 100, 100, 100, 100, 100, 100, 100])
    attempts.splice(3, 0, { kind: 'timeout' }, { kind: 'timeout' })
    expect(gradeConnectivityAttempts(attempts, 10, thresholds).grade).toBe('not_recommended')
  })

  it('exposes an exit IP only when every successful sample reports the same non-empty value', () => {
    const consistent: ProbeAttempt[] = [
      { kind: 'success', durationMs: 100, clientIP: '8.8.8.8' },
      { kind: 'success', durationMs: 110, clientIP: '8.8.8.8' },
    ]
    expect(gradeConnectivityAttempts(consistent, 2, thresholds).clientIP).toBe('8.8.8.8')

    const partiallyHidden: ProbeAttempt[] = [
      consistent[0],
      { kind: 'success', durationMs: 110, clientIP: null },
    ]
    expect(gradeConnectivityAttempts(partiallyHidden, 2, thresholds).clientIP).toBeNull()
  })

  it('never grades zero successful samples as fair when the configured minimum is zero', () => {
    const zeroMinimum = { ...thresholds, minimum_success_rate: 0 }
    const result = gradeConnectivityAttempts([
      { kind: 'http_error' },
      { kind: 'http_error' },
    ], 2, zeroMinimum)

    expect(result.grade).toBe('not_recommended')
    expect(Object.values(result.metrics ?? {}).every(Number.isFinite)).toBe(true)
  })
})
