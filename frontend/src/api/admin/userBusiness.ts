import { apiClient } from '../client'
export interface BusinessFilters {
  start_date: string
  end_date: string
  search: string
  filter: string
  sort: string
  order: 'asc' | 'desc'
  page: number
  page_size: number
  cost_mode?: 'historical' | 'estimate'
  usd_cny?: number
}
export interface BusinessRow {
  id: number
  username: string
  email: string
  deleted: boolean
  rank: number
  balance: number
  consumption: number
  subscription_consumption: number
  balance_consumption: number
  cost: number | null
  requests: number
  paid: number
  balance_paid: number
  subscription_paid: number
  refund: number
  profit: number | null
  uncertain_count: number
  active_count: number
}
export interface BusinessReport {
  items: BusinessRow[]
  total: number
  summary: {
    paid: number
    refund: number
    cost: number | null
    consumption: number
    profit: number | null
    uncertain_count: number
  }
  start_at: string
  end_at: string
  as_of: string
  timezone: string
  cost_mode?: 'historical' | 'estimate'
  usd_cny: number
}
export interface BusinessCard {
  id: number
  name: string
  source: string
  expires_at: string
  paid_cny: number | null
  payments_count: number
  daily_limit: number | null
  weekly_limit: number | null
  monthly_limit: number | null
  daily_remaining: number | null
  weekly_remaining: number | null
  monthly_remaining: number | null
}
export interface BusinessDetail {
  daily: {
    date: string
    paid: number
    refund: number
    cost: number | null
    consumption: number
    profit: number | null
  }[]
  cards: BusinessCard[]
  balance: number
  as_of: string
  start_at: string
  end_at: string
  timezone: string
  cost_mode?: 'historical' | 'estimate'
  usd_cny: number
  payments: {
    order_id: number | null
    at: string
    order_type: string
    paid: number
    refund: number
    uncertain: boolean
  }[]
  payments_total: number
}
export async function getBusinessReport(
  params: BusinessFilters,
  signal?: AbortSignal,
) {
  return (
    await apiClient.get<BusinessReport>('/admin/user-business', {
      params,
      signal,
      timeout: 25000,
    })
  ).data
}
export async function getBusinessDetail(
  id: number,
  params: BusinessFilters,
  signal?: AbortSignal,
) {
  return (
    await apiClient.get<BusinessDetail>('/admin/user-business/' + id, {
      params,
      signal,
      timeout: 25000,
    })
  ).data
}

export interface BusinessFX {
  date: string
  usd_cny: number | null
  source: string | null
}
export async function getBusinessFX(
  start: string,
  end: string,
  signal?: AbortSignal,
) {
  return (
    await apiClient.get<BusinessFX[]>('/admin/user-business/fx-rates', {
      params: { start_date: start, end_date: end },
      signal,
    })
  ).data
}
export async function saveBusinessFX(data: {
  date: string
  usd_cny: number
  source: string
}) {
  await apiClient.put('/admin/user-business/fx-rates', data)
}
