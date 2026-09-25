import { apiClient } from '../client'
export interface PaymentProfitDay {
  date: string
  account_cost: number
  revenue: number
  refund: number
  gross_profit: number | null
  uncertain_count: number
}
export interface PaymentProfitTrend {
  daily: PaymentProfitDay[]
  total_cost: number
  total_revenue: number
  total_refund: number
  gross_profit: number | null
  uncertain_count: number
  timezone: string
  currency: string
}
export async function getPaymentProfit(
  days: number,
  signal?: AbortSignal,
): Promise<PaymentProfitTrend> {
  return (
    await apiClient.get<PaymentProfitTrend>('/admin/payment/usage-profit', {
      params: { days },
      signal,
    })
  ).data
}
