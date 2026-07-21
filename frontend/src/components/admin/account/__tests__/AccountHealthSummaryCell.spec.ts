import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import AccountHealthSummaryCell from '../AccountHealthSummaryCell.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, string | number>) => {
      const messages: Record<string, string> = {
        'admin.accounts.accountHealth.requestCount': '{count} 次',
        'admin.accounts.accountHealth.windowTitle': '{window}：请求数、成功率、首 Token 延迟',
        'admin.accounts.accountHealth.windows.1m': '近1分钟',
        'admin.accounts.accountHealth.windows.5m': '近5分钟',
        'admin.accounts.accountHealth.windows.10m': '近10分钟',
        'admin.accounts.accountHealth.windows.30m': '近30分钟',
        'admin.accounts.accountHealth.windows.1h': '近1小时'
      }
      return (messages[key] ?? key).replace(/\{(\w+)\}/g, (_, name: string) => String(params?.[name] ?? ''))
    }
  })
}))

function mountCell(props: Record<string, unknown>) {
  return mount(AccountHealthSummaryCell, {
    props
  })
}

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
  it('renders multi-window matrix, 1m trend, and bottom timeline', () => {
    const wrapper = mountCell({
      probeIntervalMinutes: 30,
      item: {
        account_id: 2190,
        account_name: 'grok-1',
        is_opened: true,
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
        recent: [
          { kind: 'success', created_at: new Date().toISOString() },
          { kind: 'error', created_at: new Date().toISOString(), status_code: 500 }
        ],
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
    })

    const text = wrapper.text()
    expect(text).toContain('近1分钟')
    expect(text).toContain('近5分钟')
    expect(text).toContain('近30分钟')
    expect(text).toContain('近1小时')
    expect(text).toContain('12')
    expect(text).toContain('91.7%')
    expect(text).toContain('↓')
    expect(text).toContain('420ms')
    expect(text).toContain('每30m')
    expect(text).toContain('探OK')
    // Fixed width keeps the column stable; compact 2x2 layout keeps height lower.
    expect(wrapper.classes().join(' ')).toContain('w-[15.5rem]')
    expect(wrapper.html()).toContain('grid-cols-2')
    // Bottom timeline always rendered
    expect(wrapper.html()).toContain('h-1.5')
  })

  it('shows probe timeline bars for closed accounts with no traffic', () => {
    const now = Date.now()
    const wrapper = mountCell({
      probeIntervalMinutes: 15,
      item: {
        account_id: 9,
        account_name: 'closed-idle',
        is_opened: false,
        probe_auto_disabled: false,
        windows: {
          '1m': windowStats('1m', 0, 0),
          '5m': windowStats('5m', 0, 0),
          '30m': windowStats('30m', 0, 0),
          '1h': windowStats('1h', 0, 0)
        },
        recent: [],
        probe: {
          status: 'success',
          checked_at: new Date(now - 60_000).toISOString(),
          latency_ms: 900,
          recent: [
            { kind: 'error', created_at: new Date(now - 90 * 60_000).toISOString(), message: '主动探测' },
            { kind: 'success', created_at: new Date(now - 30 * 60_000).toISOString(), message: '主动探测' },
            { kind: 'success', created_at: new Date(now - 60_000).toISOString(), message: '主动探测' }
          ]
        },
        recommendation: {
          action: 'can_open',
          severity: 'P2',
          title: '',
          reason: '',
          notify_mode: 'none',
          immediate: false,
          recovery_ready: true
        }
      } as any
    })

    expect(wrapper.text()).toContain('—')
    expect(wrapper.text()).toContain('探OK')
    // Probe success uses sky bars
    expect(wrapper.html()).toContain('bg-sky-500')
  })
})
