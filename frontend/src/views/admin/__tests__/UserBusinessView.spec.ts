import { beforeEach, describe, it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import View from '../UserBusinessView.vue'
const { getBusinessReport } = vi.hoisted(() => ({ getBusinessReport: vi.fn() }))
vi.mock('@/api/admin/userBusiness', () => ({ getBusinessReport }))
vi.mock('vue-i18n', async (original) => ({
  ...(await original<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))
const base = {
  items: [
    {
      id: 1,
      username: 'Alice',
      email: 'alice@example.invalid',
      rank: 1,
      balance: 30,
      consumption: 50,
      subscription_consumption: 40,
      balance_consumption: 10,
      cost: 70,
      requests: 3,
      paid: 100,
      balance_paid: 20,
      subscription_paid: 80,
      refund: 10,
      profit: 20,
      uncertain_count: 0,
      active_count: 2,
    },
  ],
  total: 1,
  summary: {
    paid: 100,
    refund: 10,
    cost: 70,
    consumption: 50,
    profit: 20,
    uncertain_count: 0,
  },
  start_at: '2026-09-25T00:00:00+08:00',
  end_at: '2026-09-25T12:00:00+08:00',
  as_of: '2026-09-25T12:00:00+08:00',
  timezone: 'Asia/Shanghai',
}
function view() {
  return mount(View, {
    global: {
      stubs: {
        AppLayout: { template: '<div data-testid="console"><slot/></div>' },
        Pagination: true,
        UserBusinessDetail: {
          props: ['user', 'filters'],
          template: '<div data-testid="user-detail">{{user?.username}}</div>',
        },
      },
    },
  })
}
beforeEach(() => {
  vi.clearAllMocks()
  getBusinessReport.mockResolvedValue(base)
})
describe('user business report', () => {
  it('keeps consumption as quota and does not convert CNY costs twice', async () => {
    const w = view()
    await flushPromises()
    const cells = w.get('[data-testid="business-row"]').findAll('td')
    expect(getBusinessReport.mock.calls[0][0]).toMatchObject({
      filter: 'used',
    })
    expect(w.find('input[type=number]').exists()).toBe(false)
    expect(w.text()).not.toContain('userBusiness.historicalNotice')
    expect(getBusinessReport.mock.calls[0][0]).not.toHaveProperty('usd_cny')
    expect(getBusinessReport.mock.calls[0][0]).not.toHaveProperty('cost_mode')
    expect(cells[2].text()).not.toContain('¥')
    expect(cells[3].text()).toContain('70.00')
    expect(cells[3].text()).not.toContain('490')
    expect(cells[4].text()).toContain('20.00')
    await cells[0].get('button').trigger('click')
    expect(w.get('[data-testid="user-detail"]').text()).toBe('Alice')
    w.unmount()
  })
  it('shows unknown profits as pending review, not zero', async () => {
    getBusinessReport.mockResolvedValue({
      ...base,
      items: [
        { ...base.items[0], cost: null, profit: null, uncertain_count: 1 },
      ],
      summary: {
        ...base.summary,
        cost: null,
        profit: null,
        uncertain_count: 1,
      },
    })
    const w = view()
    await flushPromises()
    expect(w.get('[data-testid="business-row"]').findAll('td')[4].text()).toBe(
      'userBusiness.review',
    )
    expect(w.get('[data-testid="business-row"]').findAll('td')[3].text()).toBe(
      'userBusiness.review',
    )
    expect(w.text()).toContain('userBusiness.unknownNote')
    w.unmount()
  })
  it('ignores stale results after switching dates', async () => {
    let done: (x: unknown) => void = () => {}
    getBusinessReport.mockReturnValueOnce(
      new Promise((r) => {
        done = r
      }),
    )
    const w = view()
    const button = w
      .findAll('button')
      .find((x) => x.text() === 'userBusiness.week')!
    await button.trigger('click')
    await flushPromises()
    done({ ...base, items: [{ ...base.items[0], username: 'Stale' }] })
    await flushPromises()
    expect(w.text()).not.toContain('Stale')
    expect(getBusinessReport.mock.calls[0][1].aborted).toBe(true)
    w.unmount()
  })
})
