import type { PaymentOrder } from '@/types/payment'
export function orderState(order: PaymentOrder): {
  label: string
  hint: string
  tone: string
} {
  switch (order.status) {
    case 'PENDING':
      return {
        label: 'accountPages.statusPending',
        hint: 'accountPages.hintPending',
        tone: 'pending',
      }
    case 'PAID':
      return {
        label: 'accountPages.statusPaid',
        hint: 'accountPages.hintPaid',
        tone: 'processing',
      }
    case 'RECHARGING':
      return {
        label: 'accountPages.statusProcessing',
        hint: 'accountPages.hintProcessing',
        tone: 'processing',
      }
    case 'COMPLETED':
      return {
        label: 'accountPages.statusCompleted',
        hint:
          order.order_type === 'subscription'
            ? 'accountPages.hintPlanDone'
            : 'accountPages.hintBalanceDone',
        tone: 'completed',
      }
    case 'FAILED':
      return {
        label: 'payment.status.failed',
        hint: order.paid_at
          ? 'accountPages.hintFailedPaid'
          : 'accountPages.hintFailed',
        tone: 'failed',
      }
    case 'EXPIRED':
      return {
        label: 'payment.status.expired',
        hint: 'accountPages.hintExpired',
        tone: 'neutral',
      }
    case 'CANCELLED':
      return {
        label: 'payment.status.cancelled',
        hint: 'accountPages.hintCancelled',
        tone: 'neutral',
      }
    case 'REFUND_REQUESTED':
    case 'REFUNDING':
    case 'REFUND_PENDING':
    case 'REFUND_FAILED':
    case 'PARTIALLY_REFUNDED':
    case 'REFUNDED':
      return {
        label: 'payment.status.' + order.status.toLowerCase(),
        hint: 'accountPages.hintRefund',
        tone: order.status === 'REFUND_FAILED' ? 'failed' : 'neutral',
      }
    default:
      return {
        label: 'accountPages.unknown',
        hint: 'accountPages.unknown',
        tone: 'neutral',
      }
  }
}
