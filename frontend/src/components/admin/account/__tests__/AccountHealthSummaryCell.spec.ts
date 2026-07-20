import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import AccountHealthSummaryCell from '../AccountHealthSummaryCell.vue'

function windowStats(window: string, requests: number, successRate: number) {
  const success = Math.round((requests * successRate) / 100)
  return {
    window,
    request_count: requests,
    success_count: success,
    error_count: Math.max(0, requests - success),
    upstream_error_count: 0,
    status_429_count: 0,
    status_529_count: 0,
    success_rate_percent: successRate,
    error_rate_percent: requests > 0 ? 100 - successRate : 0,
    upstream_error_rate_percent: 0
  }
}

describe('AccountHealthSummaryCell', () => {
  it('renders multi-window request/success/first-token matrix and 1m trend', () => {
    const wrapper = mount(AccountHealthSummaryCell, {
      props: {
        probeIntervalMinutes: 30,
        item: {
          account_id: 2190,
          account_name: 'grok-1',
          windows: {
            '1m': windowStats('1m', 12, 91.7),
            '5m': windowStats('5m', 58, 96.5),
            '10m': windowStats('10m', 100, 97),
            '30m': windowStats('30m', 310, 98.1),
            '1h': windowStats('1h', 620, 97.8)
          },
          first_token_windows: {
            '1m': { window: '1m', sample_count: 10, avg_ms: 420 },
            '5m': { window: '5m', sample_count: 50, avg_ms: 380 },
            '30m': { window: '30m', sample_count: 200, avg_ms: 350 },
            '1h': { window: '1h', sample_count: 400, avg_ms: 360 }
          },
          recent: [],
          success_rate_trend_1m: {
            direction: 'down',
            delta_percent: -5.2,
            current_success_rate_percent: 91.7,
            previous_success_rate_percent: 96.9,
            current_request_count: 12,
            previous_request_count: 10
          },
          probe: {
            status: 'success',
            checked_at: new Date().toISOString(),
            latency_ms: 180
          },
          recommendation: {
            action: 'watch',
            severity: 'P2',
            title: '观察',
            reason: '',
            notify_mode: 'none',
            immediate: false,
            recovery_ready: false
          }
        } as any
      }
    })

    const text = wrapper.text()
    expect(text).toContain('1m')
    expect(text).toContain('5m')
    expect(text).toContain('30m')
    expect(text).toContain('1h')
    expect(text).toContain('12')
    expect(text).toContain('91.7%')
    expect(text).toContain('↓')
    expect(text).toContain('420ms')
    expect(text).toContain('每30m')
    expect(text).toContain('探测OK')
  })

  it('shows em dash when a window has no traffic', () => {
    const wrapper = mount(AccountHealthSummaryCell, {
      props: {
        item: {
          account_id: 1,
          account_name: 'empty',
          windows: {
            '1m': windowStats('1m', 0, 0),
            '5m': windowStats('5m', 0, 0),
            '30m': windowStats('30m', 0, 0),
            '1h': windowStats('1h', 0, 0)
          },
          recent: [],
          recommendation: {
            action: 'needs_probe',
            severity: 'P3',
            title: '',
            reason: '',
            notify_mode: 'none',
            immediate: false,
            recovery_ready: false
          }
        } as any
      }
    })

    expect(wrapper.text()).toContain('—')
    expect(wrapper.text()).toContain('未探测')
  })
})
