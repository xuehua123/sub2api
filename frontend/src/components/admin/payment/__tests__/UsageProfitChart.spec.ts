import { describe, it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import Profit from '../UsageProfitChart.vue'
const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('vue-chartjs', () => ({
  Line: { name: 'Line', props: ['data', 'options'], template: '<canvas/>' },
}))
it('plots revenue, cost and negative profit with monetary tooltip values', async () => {
  get.mockResolvedValue({
    data: {
      daily: [
        { date: '2026-09-25', account_cost: 5, revenue: 3, gross_profit: -2 },
      ],
      total_cost: 5,
      total_revenue: 3,
      gross_profit: -2,
      timezone: 'UTC',
      currency: 'USD',
    },
  })
  const w = mount(Profit, { props: { days: 7 } })
  await flushPromises()
  const chart = w.findComponent({ name: 'Line' })
  expect(
    chart.props('data').datasets.map((x: { data: number[] }) => x.data),
  ).toEqual([[3], [5], [-2]])
  expect(
    chart
      .props('options')
      .plugins.tooltip.callbacks.label({
        dataset: { label: 'Profit' },
        raw: -2,
      }),
  ).toMatch(/-.*\$2\.00/)
  expect(chart.props('options').interaction).toEqual({
    mode: 'index',
    intersect: false,
  })
  w.unmount()
})
describe('range request ordering', () => {
  it('ignores older responses and aborts the previous request', async () => {
    let resolveOld: (v: unknown) => void = () => {}
    get.mockReset()
    get.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveOld = resolve
      }),
    )
    get.mockResolvedValueOnce({
      data: {
        daily: [],
        total_cost: 2,
        total_revenue: 3,
        gross_profit: 1,
        timezone: 'UTC',
        currency: 'USD',
      },
    })
    const w = mount(Profit, { props: { days: 7 } })
    await w.setProps({ days: 30 })
    await flushPromises()
    expect(get.mock.calls[0][1].signal.aborted).toBe(true)
    resolveOld({
      data: {
        daily: [],
        total_cost: 999,
        total_revenue: 999,
        gross_profit: 0,
        timezone: 'UTC',
        currency: 'USD',
      },
    })
    await flushPromises()
    expect(w.text()).not.toContain('999')
    w.unmount()
  })
})
