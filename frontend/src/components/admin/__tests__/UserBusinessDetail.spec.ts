import { it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import Detail from '../UserBusinessDetail.vue'
const { getBusinessDetail } = vi.hoisted(() => ({ getBusinessDetail: vi.fn() }))
vi.mock('@/api/admin/userBusiness', () => ({ getBusinessDetail }))
vi.mock('vue-i18n', async (original) => ({
  ...(await original<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('vue-chartjs', () => ({
  Line: { name: 'Line', props: ['data', 'options'], template: '<canvas/>' },
}))
it('separates CNY money from quota chart and shows every active card', async () => {
  getBusinessDetail.mockResolvedValue({
    cards: [
      {
        id: 1,
        name: 'Card A',
        expires_at: '2027-01-01',
        paid_cny: 100,
        daily_limit: null,
        weekly_limit: null,
        monthly_limit: 100,
        monthly_remaining: 50,
      },
      {
        id: 2,
        name: 'Card B',
        expires_at: '2027-01-01',
        paid_cny: null,
        daily_limit: null,
        weekly_limit: null,
        monthly_limit: 100,
        monthly_remaining: 70,
      },
    ],
    balance: 30,
    as_of: '2026-09-25',
    timezone: 'UTC',
    daily: [
      {
        date: '2026-09-25',
        paid: 100,
        cost: 70,
        profit: 20,
        refund: 10,
        consumption: 50,
      },
    ],
    payments: [],
    payments_total: 0,
  })
  const w = mount(Detail, {
    props: {
      user: null,
      filters: {
        start_date: '2026-09-25',
        end_date: '2026-09-25',
        search: '',
        filter: '',
        sort: 'consumption',
        order: 'desc',
        page: 1,
        page_size: 20,
      },
    },
    global: {
      stubs: {
        BaseDialog: {
          props: ['show'],
          template: '<div v-if="show"><slot/></div>',
        },
      },
    },
  })
  await w.setProps({ user: { id: 1, username: 'Alice' } as never })
  await flushPromises()
  expect(w.findAll('[data-testid="business-card"]')).toHaveLength(2)
  expect(w.text()).toContain('userBusiness.unknownPayment')
  expect(
    w
      .findComponent({ name: 'Line' })
      .props('data')
      .datasets.map((d: { data: number[] }) => d.data),
  ).toEqual([[100], [70], [20]])
  await w
    .findAll('button')
    .find((x) => x.text() === 'userBusiness.quotaTrend')!
    .trigger('click')
  expect(
    w.findComponent({ name: 'Line' }).props('data').datasets[0].data,
  ).toEqual([50])
  expect(getBusinessDetail).toHaveBeenCalledTimes(1)
  w.unmount()
})
