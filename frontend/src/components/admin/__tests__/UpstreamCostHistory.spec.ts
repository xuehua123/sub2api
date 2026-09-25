import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import History from '../UpstreamCostHistory.vue'
const { getCostHistory } = vi.hoisted(() => ({ getCostHistory: vi.fn() }))
vi.mock('vue-chartjs', () => ({
  Line: {
    name: 'Line',
    props: ['data', 'options'],
    template: '<canvas data-testid="line-chart" />',
  },
}))
vi.mock('@/api/admin/upstreamCatalog', () => ({ getCostHistory }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const result = {
  daily: [
    { date: '2026-09-24', account_cost: 1 },
    { date: '2026-09-25', account_cost: 1.5 },
  ].map((day) => ({ ...day, requests: 1 })),
  items: [{ connection_id: 1, name: 'One', requests: 3, account_cost: 2.5 }],
  total_cost: 2.5,
  total_requests: 3,
  timezone: 'UTC',
  start_at: '2026-09-24T00:00:00Z',
  end_at: '2026-09-25T00:00:00Z',
}
beforeEach(() => {
  vi.clearAllMocks()
  getCostHistory.mockResolvedValue(result)
})
describe('upstream cost history', () => {
  it('requests the server default day, then queries the selected dates', async () => {
    const w = mount(History, {
      props: { show: false, connections: [] },
      global: {
        stubs: {
          BaseDialog: {
            props: ['show'],
            template: '<div v-if="show"><slot/></div>',
          },
        },
      },
    })
    await w.setProps({ show: true })
    await flushPromises()
    expect(getCostHistory.mock.calls[0].slice(0, 3)).toEqual(['', '', []])
    expect(w.get('[data-testid="history-total"]').text()).toBe('$2.50')
    expect(
      w.findComponent({ name: 'Line' }).exists() ||
        w.find('[data-testid="line-chart"]').exists(),
    ).toBe(true)
    expect(w.findAll('[data-testid="daily-cost-row"]')).toHaveLength(2)
    const calls = getCostHistory.mock.calls.length
    const cumulative = w
      .findAll('button')
      .find((button) => button.text() === 'upstreamWorkspace.cumulative')!
    await cumulative.trigger('click')
    expect(getCostHistory).toHaveBeenCalledTimes(calls)
    const chart = w.findComponent({ name: 'Line' })
    expect(chart.props('data').datasets[0].data).toEqual([1, 2.5])
    await w.setProps({ show: false })
    await w.setProps({ show: true })
    await flushPromises()
    expect(getCostHistory).toHaveBeenCalledTimes(calls)
    const dates = w.findAll('input[type="date"]')
    await dates[0].setValue('2026-09-20')
    await dates[1].setValue('2026-09-24')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(getCostHistory.mock.calls.at(-1)?.slice(0, 3)).toEqual([
      '2026-09-20',
      '2026-09-24',
      [],
    ])
    w.unmount()
  })
})
