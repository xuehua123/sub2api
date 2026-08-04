import type {
  ConnectivityClientLocation,
  ConnectivityEvaluation,
  ConnectivityGradeThresholds,
  ConnectivityMetrics,
  ProbeAttempt,
} from './types'

export function calculateMedian(values: number[]): number {
  if (values.length === 0) return Number.NaN
  const sorted = [...values].sort((a, b) => a - b)
  const middle = Math.floor(sorted.length / 2)
  return sorted.length % 2 === 0
    ? (sorted[middle - 1] + sorted[middle]) / 2
    : sorted[middle]
}

export function calculateNearestRankP95(values: number[]): number {
  if (values.length === 0) return Number.NaN
  const sorted = [...values].sort((a, b) => a - b)
  return sorted[Math.ceil(0.95 * sorted.length) - 1]
}

export function calculateMAD(values: number[]): number {
  if (values.length === 0) return Number.NaN
  const median = calculateMedian(values)
  return calculateMedian(values.map((value) => Math.abs(value - median)))
}

export function gradeConnectivityAttempts(
  attempts: ProbeAttempt[],
  plannedSamples: number,
  thresholds: ConnectivityGradeThresholds,
): ConnectivityEvaluation {
  if (attempts.some((attempt) => attempt.kind === 'rate_limited')) {
    return { status: 'rate_limited' }
  }
  if (attempts.some((attempt) => attempt.kind === 'cancelled')) {
    return { status: 'cancelled' }
  }
  if (attempts.length !== plannedSamples || attempts.some((attempt) => attempt.kind === 'protocol_error')) {
    return { status: 'incomplete' }
  }

  const durations = attempts.flatMap((attempt) => attempt.kind === 'success' ? [attempt.durationMs] : [])
  const successRate = durations.length / plannedSamples
  const hasSuccessfulSample = durations.length > 0
  // With no successful sample there is no median / P95 / MAD: NaN, never a
  // misleading 0 ms. Consumers must not render NaN as a latency.
  const metrics: ConnectivityMetrics = {
    successRate,
    p95Ms: hasSuccessfulSample ? calculateNearestRankP95(durations) : Number.NaN,
    medianMs: hasSuccessfulSample ? calculateMedian(durations) : Number.NaN,
    madMs: hasSuccessfulSample ? calculateMAD(durations) : Number.NaN,
    maxConsecutiveTimeouts: maxConsecutiveTimeouts(attempts),
  }
  const clientIP = resolveStableClientIP(attempts)
  const clientLocation = resolveStableClientLocation(attempts, clientIP)

  const base: ConnectivityEvaluation = {
    status: 'graded',
    metrics,
    clientIP,
    clientLocation,
  }

  if (
    !hasSuccessfulSample
    || successRate < thresholds.minimum_success_rate
    || metrics.maxConsecutiveTimeouts >= thresholds.max_consecutive_timeouts
  ) {
    return { ...base, grade: 'not_recommended' }
  }
  if (
    successRate >= thresholds.excellent.min_success_rate
    && metrics.p95Ms <= thresholds.excellent.max_p95_ms
    && metrics.madMs <= thresholds.excellent.max_mad_ms
  ) {
    return { ...base, grade: 'excellent' }
  }
  if (
    successRate >= thresholds.good.min_success_rate
    && metrics.p95Ms <= thresholds.good.max_p95_ms
    && metrics.madMs <= thresholds.good.max_mad_ms
  ) {
    return { ...base, grade: 'good' }
  }
  return { ...base, grade: 'fair' }
}

function maxConsecutiveTimeouts(attempts: ProbeAttempt[]): number {
  let current = 0
  let maximum = 0
  for (const attempt of attempts) {
    if (attempt.kind === 'timeout') {
      current++
      maximum = Math.max(maximum, current)
    } else {
      current = 0
    }
  }
  return maximum
}

function resolveStableClientIP(attempts: ProbeAttempt[]): string | null {
  const successful = attempts.filter((attempt) => attempt.kind === 'success')
  if (successful.length === 0 || successful.some((attempt) => !attempt.clientIP)) return null
  const values = new Set(successful.map((attempt) => attempt.clientIP))
  return values.size === 1 ? [...values][0] : null
}

// resolveStableClientLocation returns a location only when the resolved client
// IP is stable AND every successful attempt carrying that IP reports the same
// location. If any attempt lacks a location or the locations disagree, the
// region is hidden while the IP remains visible.
function resolveStableClientLocation(attempts: ProbeAttempt[], clientIP: string | null): ConnectivityClientLocation | null {
  if (!clientIP) return null
  const successful = attempts.filter(
    (attempt): attempt is Extract<ProbeAttempt, { kind: 'success' }> =>
      attempt.kind === 'success' && attempt.clientIP === clientIP,
  )
  if (successful.length === 0 || successful.some((attempt) => !attempt.clientLocation)) return null
  const first = successful[0].clientLocation!
  return successful.every((attempt) => sameClientLocation(attempt.clientLocation!, first)) ? first : null
}

function sameClientLocation(left: ConnectivityClientLocation, right: ConnectivityClientLocation): boolean {
  return left.country_code === right.country_code
    && left.country === right.country
    && left.region === right.region
    && left.city === right.city
}
