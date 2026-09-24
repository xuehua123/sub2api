import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import View from '../UserOrdersView.vue'
import Select from '@/components/common/Select.vue'
const { api, error } = vi.hoisted(() => ({
  api: {
    getMyOrders: vi.fn(),
    getRefundEligibleProviders: vi.fn(),
    cancelOrder: vi.fn(),
    requestRefund: vi.fn(),
  },
  error: vi.fn(),
}))
vi.mock('@/api/payment', () => ({ paymentAPI: api }))
vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError: error, showSuccess: vi.fn() }),
}))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key, locale: 'en' }),
}))
enableAutoUnmount(afterEach)
const order = {
  id: 1,
  user_id: 1,
  order_type: 'subscription',
  amount: 10,
  pay_amount: 70,
  currency: 'CNY',
  fee_rate: 0,
  payment_type: 'alipay',
  out_trade_no: 'ORDER-1',
  status: 'PENDING',
  created_at: '2026-09-01T00:00:00Z',
  expires_at: '2026-09-01T01:00:00Z',
  refund_amount: 0,
  provider_instance_id: 'provider',
}
function view() {
  return mount(View, {
    global: {
      stubs: {
        PricePortalLayout: { template: '<div><slot/></div>' },
        RouterLink: { template: '<a><slot/></a>' },
        Icon: true,
        Pagination: true,
        BaseDialog: {
          props: ['show'],
          template: '<div v-if="show"><slot/><slot name="footer"/></div>',
        },
      },
    },
  })
}
beforeEach(() => {
  vi.clearAllMocks()
  api.getMyOrders.mockResolvedValue({ data: { items: [order], total: 1 } })
  api.getRefundEligibleProviders.mockResolvedValue({
    data: { provider_instance_ids: [] },
  })
})
describe('user order interactions', () => {
  it('shows payment currency and never presents a plan amount as USD credited balance', async () => {
    const w = view()
    await flushPromises()
    expect(w.text()).toContain('CNY')
    expect(w.text()).not.toContain('accountPages.creditedAmount')
    expect(w.text()).toContain('accountPages.statusPending')
  })
  it('shows a retry state rather than an empty successful result on failures', async () => {
    api.getMyOrders.mockRejectedValueOnce(new Error('offline'))
    const w = view()
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toBeTruthy()
    expect(w.find('[data-testid="order-1"]').exists()).toBe(false)
  })
  it('keeps newer filter results when old requests finish late', async () => {
    const w = view()
    await flushPromises()
    let resolve!: (v: unknown) => void
    api.getMyOrders.mockImplementationOnce(
      () => new Promise((r) => (resolve = r)),
    )
    await w.get('[title="common.refresh"]').trigger('click')
    api.getMyOrders.mockResolvedValueOnce({
      data: {
        items: [{ ...order, id: 2, out_trade_no: 'NEW', status: 'PAID' }],
        total: 1,
      },
    })
    w.getComponent(Select).vm.$emit('update:modelValue', 'PAID')
    w.getComponent(Select).vm.$emit('change')
    await flushPromises()
    resolve({ data: { items: [order], total: 1 } })
    await flushPromises()
    expect(w.find('[data-testid="order-2"]').exists()).toBe(true)
    expect(w.find('[data-testid="order-1"]').exists()).toBe(false)
  })
  it('requires confirmation and blocks duplicate cancellations while saving', async () => {
    const w = view()
    await flushPromises()
    await w
      .findAll('button')
      .find((b) => b.text() === 'payment.orders.cancel')!
      .trigger('click')
    expect(api.cancelOrder).not.toHaveBeenCalled()
    let resolve!: () => void
    api.cancelOrder.mockReturnValueOnce(new Promise<void>((r) => (resolve = r)))
    const confirm = w.get('.btn-danger')
    await confirm.trigger('click')
    await confirm.trigger('click')
    expect(api.cancelOrder).toHaveBeenCalledTimes(1)
    resolve()
    await flushPromises()
  })
  it.each(['PAID','RECHARGING','COMPLETED','REFUNDED'])('never offers a refund action for %s orders, even if the provider allows refunds', async (status) => {
    api.getMyOrders.mockResolvedValue({data:{items:[{...order,status}],total:1}})
    api.getRefundEligibleProviders.mockResolvedValue({data:{provider_instance_ids:['provider']}})
    const w=view()
    await flushPromises()
    expect(w.text()).not.toContain('payment.orders.requestRefund')
    expect(w.find('textarea').exists()).toBe(false)
    expect(api.getRefundEligibleProviders).not.toHaveBeenCalled()
    expect(api.requestRefund).not.toHaveBeenCalled()
  })

})
