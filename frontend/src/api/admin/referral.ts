import { apiClient } from '../client'
import type {
  AdminReferralAccountOption,
  AdminReferralOverview,
  AdminReferralTreeNode,
  AdminCommissionLedger,
  AdminCommissionReward,
  AdminCommissionWithdrawal,
  AdminReferralRelation,
  BasePaginationResponse,
  CommissionLedgerEntry,
  CommissionWithdrawalItem,
  ReferralRelationHistoryEntry,
  ReferralRelationInfo
} from '@/types'

export async function searchAccounts(
  q: string,
  limit: number = 10
): Promise<AdminReferralAccountOption[]> {
  const { data } = await apiClient.get<AdminReferralAccountOption[]>(
    '/admin/referral/accounts/search',
    { params: { q, limit } }
  )
  return data
}

export async function getOverview(): Promise<AdminReferralOverview> {
  const { data } = await apiClient.get<AdminReferralOverview>('/admin/referral/overview')
  return data
}

export async function getTree(userId: number): Promise<AdminReferralTreeNode> {
  const { data } = await apiClient.get<AdminReferralTreeNode>(`/admin/referral/tree/${userId}`)
  return data
}

export async function listRelations(
  page: number = 1,
  pageSize: number = 20,
  search: string = ''
): Promise<BasePaginationResponse<AdminReferralRelation>> {
  const { data } = await apiClient.get<BasePaginationResponse<AdminReferralRelation>>(
    '/admin/referral/relations',
    { params: { page, page_size: pageSize, search } }
  )
  return data
}

export async function listRelationHistories(
  page: number = 1,
  pageSize: number = 20,
  userId?: number
): Promise<BasePaginationResponse<ReferralRelationHistoryEntry>> {
  const { data } = await apiClient.get<BasePaginationResponse<ReferralRelationHistoryEntry>>(
    '/admin/referral/relation-histories',
    { params: { page, page_size: pageSize, user_id: userId } }
  )
  return data
}

export async function updateRelation(
  userId: number,
  payload: { code: string; reason?: string; notes?: string }
): Promise<ReferralRelationInfo> {
  const { data } = await apiClient.put<ReferralRelationInfo>(
    `/admin/referral/relations/${userId}`,
    payload
  )
  return data
}

export async function listCommissionRewards(
  page: number = 1,
  pageSize: number = 20,
  filters: Record<string, string | number | undefined> = {}
): Promise<BasePaginationResponse<AdminCommissionReward>> {
  const { data } = await apiClient.get<BasePaginationResponse<AdminCommissionReward>>(
    '/admin/referral/commission-rewards',
    { params: { page, page_size: pageSize, ...filters } }
  )
  return data
}

export async function listCommissionLedgers(
  page: number = 1,
  pageSize: number = 20,
  filters: Record<string, string | number | undefined> = {}
): Promise<BasePaginationResponse<AdminCommissionLedger>> {
  const { data } = await apiClient.get<BasePaginationResponse<AdminCommissionLedger>>(
    '/admin/referral/commission-ledgers',
    { params: { page, page_size: pageSize, ...filters } }
  )
  return data
}

export async function createCommissionAdjustment(payload: {
  reward_id: number
  amount: number
  remark?: string
}): Promise<CommissionLedgerEntry> {
  const { data } = await apiClient.post<CommissionLedgerEntry>(
    '/admin/referral/commission-adjustments',
    payload
  )
  return data
}

export async function listWithdrawals(
  page: number = 1,
  pageSize: number = 20,
  filters: Record<string, string | number | undefined> = {}
): Promise<BasePaginationResponse<AdminCommissionWithdrawal>> {
  const { data } = await apiClient.get<BasePaginationResponse<AdminCommissionWithdrawal>>(
    '/admin/referral/withdrawals',
    { params: { page, page_size: pageSize, ...filters } }
  )
  return data
}

export async function getWithdrawalItems(id: number): Promise<CommissionWithdrawalItem[]> {
  const { data } = await apiClient.get<CommissionWithdrawalItem[]>(
    `/admin/referral/withdrawals/${id}/items`
  )
  return data
}

export async function approveWithdrawal(id: number, remark?: string) {
  const { data } = await apiClient.post(`/admin/referral/withdrawals/${id}/approve`, { remark })
  return data
}

export async function rejectWithdrawal(id: number, reason: string) {
  const { data } = await apiClient.post(`/admin/referral/withdrawals/${id}/reject`, { reason })
  return data
}

export async function markWithdrawalPaid(id: number, remark?: string) {
  const { data } = await apiClient.post(`/admin/referral/withdrawals/${id}/mark-paid`, { remark })
  return data
}

const referralAPI = {
  searchAccounts,
  getOverview,
  getTree,
  listRelations,
  listRelationHistories,
  updateRelation,
  listCommissionRewards,
  listCommissionLedgers,
  createCommissionAdjustment,
  listWithdrawals,
  getWithdrawalItems,
  approveWithdrawal,
  rejectWithdrawal,
  markWithdrawalPaid
}

export default referralAPI
