export interface ConnectivityGradeThreshold {
  min_success_rate: number
  max_p95_ms: number
  max_mad_ms: number
}

export interface ConnectivityGradeThresholds {
  grading_version: string
  minimum_success_rate: number
  max_consecutive_timeouts: number
  excellent: ConnectivityGradeThreshold
  good: ConnectivityGradeThreshold
}

export interface ConnectivityTestEndpoint {
  name: string
  api_url: string
  probe_url: string
  is_default: boolean
}

export type ProbeFailureKind =
  | 'timeout'
  | 'http_error'
  | 'network_or_cors'
  | 'protocol_error'
  | 'cancelled'
  | 'rate_limited'

export type ProbeAttempt =
  | { kind: 'success'; durationMs: number; clientIP: string | null }
  | { kind: ProbeFailureKind }

export type ConnectivityGrade = 'excellent' | 'good' | 'fair' | 'not_recommended'
export type ConnectivityEvaluationStatus = 'graded' | 'incomplete' | 'cancelled' | 'rate_limited'

export interface ConnectivityMetrics {
  successRate: number
  p95Ms: number
  medianMs: number
  madMs: number
  maxConsecutiveTimeouts: number
}

export interface ConnectivityEvaluation {
  status: ConnectivityEvaluationStatus
  grade?: ConnectivityGrade
  metrics?: ConnectivityMetrics
  clientIP?: string | null
}

export interface ConnectivityProbeConfig {
  endpoints: ConnectivityTestEndpoint[]
  thresholds: ConnectivityGradeThresholds
  samples: number
  warmup: number
  maxConcurrency: number
  timeoutMs: number
  clientIPEnabled: boolean
}

export interface ConnectivityEndpointResult extends ConnectivityEvaluation {
  endpoint: ConnectivityTestEndpoint
}

export interface ConnectivityRunResult {
  status: 'complete' | 'incomplete' | 'cancelled' | 'rate_limited'
  endpoints: ConnectivityEndpointResult[]
  recommendedAPIURL?: string
  testedAt: number
  gradingVersion: string
}
