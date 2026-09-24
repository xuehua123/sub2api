import { describe, it, expect } from 'vitest'
import type { PaymentOrder } from '@/types/payment'
import { orderState } from '../orderState'
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
} as PaymentOrder
describe('order status presentation', () => {
  it('does not call paid or processing orders fulfilled', () => {
    for (const status of [
      'PENDING',
      'PAID',
      'RECHARGING',
      'FAILED',
      'REFUND_REQUESTED',
      'REFUND_PENDING',
      'REFUNDING',
      'PARTIALLY_REFUNDED',
      'REFUNDED',
      'REFUND_FAILED',
      'EXPIRED',
      'CANCELLED',
    ] as const) {
      expect(orderState({ ...order, status }).label).not.toBe(
        'accountPages.statusCompleted',
      )
    }
  })
  it('distinguishes confirmed plan and balance delivery', () => {
    expect(orderState({ ...order, status: 'COMPLETED' }).hint).toBe(
      'accountPages.hintPlanDone',
    )
    expect(
      orderState({ ...order, status: 'COMPLETED', order_type: 'balance' }).hint,
    ).toBe('accountPages.hintBalanceDone')
  })
  it('warns against paying again only when failed order has payment evidence', () => {
    expect(orderState({ ...order, status: 'FAILED' }).hint).toBe(
      'accountPages.hintFailed',
    )
    expect(
      orderState({ ...order, status: 'FAILED', paid_at: order.created_at })
        .hint,
    ).toBe('accountPages.hintFailedPaid')
  })
})
