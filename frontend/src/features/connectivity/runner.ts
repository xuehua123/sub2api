import { calculateMedian, gradeConnectivityAttempts } from './grading'
import { probeEndpoint } from './probe'
import type {
  ConnectivityEndpointResult,
  ConnectivityEvaluation,
  ConnectivityProbeConfig,
  ConnectivityRunResult,
  ConnectivityTestEndpoint,
  ProbeAttempt,
} from './types'

interface ConnectivityRunnerDependencies {
  probe?: (url: string, timeoutMs: number, signal?: AbortSignal) => Promise<ProbeAttempt>
  now?: () => number
}

interface OriginTarget {
  origin: string
  probeURL: string
}

export async function runConnectivityTest(
  config: ConnectivityProbeConfig,
  signal?: AbortSignal,
  dependencies: ConnectivityRunnerDependencies = {},
): Promise<ConnectivityRunResult> {
  const runProbe = dependencies.probe ?? probeEndpoint
  const now = dependencies.now ?? Date.now
  const targets = uniqueOriginTargets(config.endpoints)
  const attempts = new Map(targets.map((target) => [target.origin, [] as ProbeAttempt[]]))

  if (signal?.aborted) {
    return specialRunResult(config, 'cancelled', now())
  }

  for (let round = 0; round < config.warmup; round++) {
    const results = await runRound(targets, config.maxConcurrency, (target) => (
      runProbe(target.probeURL, config.timeoutMs, signal)
    ))
    if (results.some((attempt) => attempt.kind === 'rate_limited')) {
      return specialRunResult(config, 'rate_limited', now())
    }
    if (signal?.aborted || results.some((attempt) => attempt.kind === 'cancelled')) {
      return specialRunResult(config, 'cancelled', now())
    }
  }

  for (let round = 0; round < config.samples; round++) {
    const results = await runRound(targets, config.maxConcurrency, (target) => (
      runProbe(target.probeURL, config.timeoutMs, signal)
    ))
    results.forEach((attempt, index) => attempts.get(targets[index].origin)!.push(attempt))
    if (results.some((attempt) => attempt.kind === 'rate_limited')) {
      return specialRunResult(config, 'rate_limited', now())
    }
    if (signal?.aborted || results.some((attempt) => attempt.kind === 'cancelled')) {
      return specialRunResult(config, 'cancelled', now())
    }
  }

  const allAttempts = [...attempts.values()].flat()
  if (allAttempts.length > 0 && allAttempts.every((attempt) => attempt.kind === 'network_or_cors')) {
    return incompleteNetworkRunResult(config, attempts, now())
  }

  const evaluations = new Map<string, ConnectivityEvaluation>()
  for (const target of targets) {
    evaluations.set(
      target.origin,
      gradeConnectivityAttempts(attempts.get(target.origin)!, config.samples, config.thresholds),
    )
  }
  const endpointResults = config.endpoints.map((endpoint) => ({
    endpoint,
    ...evaluations.get(new URL(endpoint.probe_url).origin)!,
  }))
  const status = endpointResults.every((item) => item.status === 'graded') ? 'complete' : 'incomplete'

  return {
    status,
    endpoints: endpointResults,
    recommendedAPIURL: chooseRecommendedAPIURL(endpointResults),
    testedAt: now(),
    gradingVersion: config.thresholds.grading_version,
  }
}

// A browser cannot distinguish a CORS rejection from some network failures,
// so this remains an incomplete result rather than a bad grade. Preserve the
// measured durations nevertheless: they tell the user how long this failed
// connectivity attempt took, without presenting it as usable API latency.
function incompleteNetworkRunResult(
  config: ConnectivityProbeConfig,
  attempts: Map<string, ProbeAttempt[]>,
  testedAt: number,
): ConnectivityRunResult {
  return {
    status: 'incomplete',
    endpoints: config.endpoints.map((endpoint) => {
      const endpointAttempts = attempts.get(new URL(endpoint.probe_url).origin) ?? []
      const failedDurations = endpointAttempts.map((attempt) => attempt.durationMs)
      return {
        endpoint,
        status: 'incomplete' as const,
        metrics: {
          successRate: 0,
          p95Ms: Number.NaN,
          medianMs: Number.NaN,
          failureMedianMs: failedDurations.length > 0 ? calculateMedian(failedDurations) : undefined,
          madMs: Number.NaN,
          maxConsecutiveTimeouts: 0,
        },
      }
    }),
    testedAt,
    gradingVersion: config.thresholds.grading_version,
  }
}

function uniqueOriginTargets(endpoints: ConnectivityTestEndpoint[]): OriginTarget[] {
  const seen = new Set<string>()
  const result: OriginTarget[] = []
  for (const endpoint of endpoints) {
    const origin = new URL(endpoint.probe_url).origin
    if (seen.has(origin)) continue
    seen.add(origin)
    result.push({ origin, probeURL: endpoint.probe_url })
  }
  return result
}

async function runRound<T, R>(
  items: T[],
  concurrency: number,
  worker: (item: T) => Promise<R>,
): Promise<R[]> {
  const results = new Array<R>(items.length)
  let nextIndex = 0
  const workers = Array.from({ length: Math.min(Math.max(1, concurrency), items.length) }, async () => {
    while (nextIndex < items.length) {
      const index = nextIndex++
      results[index] = await worker(items[index])
    }
  })
  await Promise.all(workers)
  return results
}

function specialRunResult(
  config: ConnectivityProbeConfig,
  status: 'incomplete' | 'cancelled' | 'rate_limited',
  testedAt: number,
): ConnectivityRunResult {
  return {
    status,
    endpoints: config.endpoints.map((endpoint) => ({ endpoint, status })),
    testedAt,
    gradingVersion: config.thresholds.grading_version,
  }
}

function chooseRecommendedAPIURL(results: ConnectivityEndpointResult[]): string | undefined {
  const candidates = results
    .map((result, index) => ({ result, index }))
    .filter(({ result }) => result.status === 'graded' && (result.grade === 'excellent' || result.grade === 'good'))

  candidates.sort((left, right) => {
    const gradeRank = { excellent: 0, good: 1 } as const
    const leftGrade = left.result.grade as keyof typeof gradeRank
    const rightGrade = right.result.grade as keyof typeof gradeRank
    if (gradeRank[leftGrade] !== gradeRank[rightGrade]) {
      return gradeRank[leftGrade] - gradeRank[rightGrade]
    }
    const leftMetrics = left.result.metrics!
    const rightMetrics = right.result.metrics!
    if (leftMetrics.successRate !== rightMetrics.successRate) {
      return rightMetrics.successRate - leftMetrics.successRate
    }
    if (leftMetrics.p95Ms !== rightMetrics.p95Ms) return leftMetrics.p95Ms - rightMetrics.p95Ms
    if (leftMetrics.madMs !== rightMetrics.madMs) return leftMetrics.madMs - rightMetrics.madMs
    if (leftMetrics.medianMs !== rightMetrics.medianMs) return leftMetrics.medianMs - rightMetrics.medianMs
    if (left.result.endpoint.is_default !== right.result.endpoint.is_default) {
      return left.result.endpoint.is_default ? -1 : 1
    }
    return left.index - right.index
  })
  return candidates[0]?.result.endpoint.api_url
}
