import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import AccountHealthSummaryCell from '../AccountHealthSummaryCell.vue'

function zeroWindow(window: string) {
  return {
    window,
    request_count: 0,
    success_count: 0,
    error_count: 0,
    upstream_error_count: 0,
    status_429_count: 0,
    status_529_count: 0,
    success_rate_percent: 0,
    error_rate_percent: 0,
    upstream_error_rate_percent: 0
  }
}

describe('AccountHealthSummaryCell', () => {
  it('shows active probe health when request windows have no samples', () => {
    const now = Date.now()
    const wrapper = mount(AccountHealthSummaryCell, {
      props: {
        item: {
          account_id: 2190,
          account_name: 'https://www.findcg.com/',
          is_opened: false,
          windows: {
            '1m': zeroWindow('1m'),
            '5m': zeroWindow('5m'),
            '10m': zeroWindow('10m'),
            '30m': zeroWindow('30m'),
            '1h': zeroWindow('1h')
          },
          recent: [],
          probe: {
            status: 'success',
            checked_at: new Date(now - 60_000).toISOString(),
            latency_ms: 1200,
            recent: [
              { kind: 'error', created_at: new Date(now - 30 * 60_000).toISOString(), duration_ms: 1500 },
              { kind: 'success', created_at: new Date(now - 5 * 60_000).toISOString(), duration_ms: 1100 },
              { kind: 'success', created_at: new Date(now - 60_000).toISOString(), duration_ms: 1200 }
            ]
          },
          recommendation: {
            action: 'can_open',
            severity: 'P2',
            title: '账号探测已恢复，可尝试打开',
            reason: '',
            notify_mode: 'digest',
            immediate: false,
            recovery_ready: true
          }
        } as any
      }
    })

    expect(wrapper.text()).toContain('10m 探 100.0% / 2次')
    expect(wrapper.text()).toContain('1h 探 66.7% / 3次')
  })
})
