import { describe, expect, it } from 'vitest'
import { parseConnectivityProbeConfig } from '../config'

function settings() {
  return {
    connectivity_test_enabled: true,
    connectivity_client_ip_enabled: false,
    connectivity_grade_thresholds: {
      grading_version: '1',
      minimum_success_rate: 0.8,
      max_consecutive_timeouts: 2,
      excellent: { min_success_rate: 1, max_p95_ms: 250, max_mad_ms: 50 },
      good: { min_success_rate: 0.9, max_p95_ms: 500, max_mad_ms: 120 },
    },
    connectivity_probe_samples: 10,
    connectivity_probe_warmup: 1,
    connectivity_probe_max_concurrency: 3,
    connectivity_probe_timeout_ms: 10000,
    connectivity_test_endpoints: [{
      name: '默认端点',
      api_url: 'https://api.example.com/v1',
      probe_url: 'https://api.example.com/.well-known/sub2api/edge-probe',
      is_default: true,
    }],
  }
}

describe('parseConnectivityProbeConfig', () => {
  it('accepts the complete backend-approved configuration', () => {
    expect(parseConnectivityProbeConfig(settings())).toMatchObject({
      samples: 10,
      warmup: 1,
      maxConcurrency: 3,
      timeoutMs: 10000,
    })
  })

  it('accepts backend boundary values and rejects an equal minimum and good success rate', () => {
    const boundarySettings = settings()
    boundarySettings.connectivity_grade_thresholds.minimum_success_rate = 0
    boundarySettings.connectivity_grade_thresholds.excellent.max_mad_ms = 0
    expect(parseConnectivityProbeConfig(boundarySettings)).not.toBeNull()

    boundarySettings.connectivity_grade_thresholds.good.min_success_rate = 0
    expect(parseConnectivityProbeConfig(boundarySettings)).toBeNull()
  })

  it('fails closed for disabled, cross-origin, invalid thresholds, or over-budget settings', () => {
    expect(parseConnectivityProbeConfig({ ...settings(), connectivity_test_enabled: false })).toBeNull()
    expect(parseConnectivityProbeConfig({
      ...settings(),
      connectivity_test_endpoints: [{
        ...settings().connectivity_test_endpoints[0],
        probe_url: 'https://other.example.com/.well-known/sub2api/edge-probe',
      }],
    })).toBeNull()
    expect(parseConnectivityProbeConfig({
      ...settings(),
      connectivity_grade_thresholds: {
        ...settings().connectivity_grade_thresholds,
        excellent: { min_success_rate: 0.8, max_p95_ms: 600, max_mad_ms: 200 },
      },
    })).toBeNull()

    const endpoints = Array.from({ length: 12 }, (_, index) => ({
      name: `端点 ${index}`,
      api_url: `https://api${index}.example.com/v1`,
      probe_url: `https://api${index}.example.com/.well-known/sub2api/edge-probe`,
      is_default: index === 0,
    }))
    expect(parseConnectivityProbeConfig({
      ...settings(),
      connectivity_probe_samples: 20,
      connectivity_probe_warmup: 2,
      connectivity_test_endpoints: endpoints,
    })).toBeNull()
  })

  it('fails closed for ambiguous endpoint URL paths', () => {
    for (const apiURL of [
      'https://api.example.com/v1%2Fadmin',
      String.raw`https://api.example.com/v1\admin`,
      'https://api.example.com/v1/../admin',
      'https://api.example.com/v1/./models',
      'https://api.example.com/v 1',
      'https://api.example.com/模型',
    ]) {
      expect(parseConnectivityProbeConfig({
        ...settings(),
        connectivity_test_endpoints: [{
          ...settings().connectivity_test_endpoints[0],
          api_url: apiURL,
        }],
      })).toBeNull()
    }
  })
})
