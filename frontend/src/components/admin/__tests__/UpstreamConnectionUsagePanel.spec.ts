import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string, fallback?: string) => fallback || key })
}))

vi.mock('vue-chartjs', () => ({
  Line: {
    props: ['data'],
    template: '<div data-testid="usage-chart">{{ data.datasets[0]?.label }}|{{ data.datasets[0]?.data.join(",") }}</div>'
  }
}))

import UpstreamConnectionUsagePanel from '../UpstreamConnectionUsagePanel.vue'

describe('UpstreamConnectionUsagePanel', () => {
  it('shows per-key account cost and switches the hourly series to requests', async () => {
    const wrapper = mount(UpstreamConnectionUsagePanel, {
      props: {
        loading: false,
        error: '',
        bindings: [],
        usage: {
          connection_id: 5,
          timezone: 'Asia/Shanghai',
          start_at: '2026-07-19T00:00:00+08:00',
          end_at: '2026-07-19T01:30:00+08:00',
          summary: { requests: 7, tokens: 300, account_cost: 3, standard_cost: 2.5, user_cost: 4 },
          trend: [
            { bucket: '2026-07-19T00:00:00+08:00', requests: 3, tokens: 100, account_cost: 1, standard_cost: 1, user_cost: 1.5 },
            { bucket: '2026-07-19T01:00:00+08:00', requests: 4, tokens: 200, account_cost: 2, standard_cost: 1.5, user_cost: 2.5 }
          ],
          accounts: [{
            binding_id: 1,
            account_id: 11,
            account_name: 'Primary account',
            remote_token_id: '101',
            remote_token_name: 'primary-key',
            remote_group_name: 'vip',
            resolution_kind: 'fixed',
            observed_multiplier: 0.5,
            status: 'ready',
            stats: { requests: 7, tokens: 300, account_cost: 3, standard_cost: 2.5, user_cost: 4 },
            trend: [
              { bucket: '2026-07-19T00:00:00+08:00', requests: 3, tokens: 100, account_cost: 1, standard_cost: 1, user_cost: 1.5 },
              { bucket: '2026-07-19T01:00:00+08:00', requests: 4, tokens: 200, account_cost: 2, standard_cost: 1.5, user_cost: 2.5 }
            ]
          }]
        }
      },
      global: {
        stubs: { Icon: true, LoadingSpinner: true }
      }
    })

    expect(wrapper.get('[data-testid="usage-summary-cost"]').text()).toContain('3.00')
    expect(wrapper.text()).toContain('Primary account')
    expect(wrapper.text()).toContain('primary-key')
    expect(wrapper.get('[data-testid="usage-chart"]').text()).toBe('Primary account - primary-key|1,2')

    const requestsButton = wrapper.findAll('button').find(button =>
      button.text().includes('admin.upstreamConnections.usage.requests')
    )
    await requestsButton!.trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="usage-chart"]').text()).toBe('Primary account - primary-key|3,4')
  })
})
