import { apiClient } from './client'
import type {
  BasePaginationResponse,
  CommissionLedgerEntry,
  CommissionPayoutAccount,
  CommissionWithdrawal,
  CreateReferralWithdrawalRequest,
  ReferralCenterOverview,
  ReferralInvitee,
  ReferralWithdrawalResult,
  UpsertReferralPayoutAccountRequest,
  UserInviteeReward,
  ValidateReferralCodeResponse
} from '@/types'

export async function getOverview(): Promise<ReferralCenterOverview> {
  const { data } = await apiClient.get<ReferralCenterOverview>('/user/referral/overview')
  return data
}

export async function getLedger(
  page: number = 1,
  pageSize: number = 20
): Promise<BasePaginationResponse<CommissionLedgerEntry>> {
  const { data } = await apiClient.get<BasePaginationResponse<CommissionLedgerEntry>>(
    '/user/referral/ledger',
    { params: { page, page_size: pageSize } }
  )
  return data
}

export async function getInvitees(
  page: number = 1,
  pageSize: number = 20
): Promise<BasePaginationResponse<ReferralInvitee>> {
  const { data } = await apiClient.get<BasePaginationResponse<ReferralInvitee>>(
    '/user/referral/invitees',
    { params: { page, page_size: pageSize } }
  )
  return data
}

export async function convertToCredit(amount: number) {
  const { data } = await apiClient.post('/user/referral/convert-to-credit', { amount })
  return data
}

export async function validateCode(code: string): Promise<ValidateReferralCodeResponse> {
  const { data } = await apiClient.post<ValidateReferralCodeResponse>('/auth/validate-referral-code', { code })
  return data
}

export async function createWithdrawal(
  payload: CreateReferralWithdrawalRequest
): Promise<ReferralWithdrawalResult> {
  const { data } = await apiClient.post<ReferralWithdrawalResult>(
    '/user/referral/withdrawals',
    payload
  )
  return data
}

export async function getWithdrawals(
  page: number = 1,
  pageSize: number = 20
): Promise<BasePaginationResponse<CommissionWithdrawal>> {
  const { data } = await apiClient.get<BasePaginationResponse<CommissionWithdrawal>>(
    '/user/referral/withdrawals',
    { params: { page, page_size: pageSize } }
  )
  return data
}

export async function getPayoutAccounts(): Promise<CommissionPayoutAccount[]> {
  const { data } = await apiClient.get<CommissionPayoutAccount[]>('/user/referral/payout-accounts')
  return data
}

export async function createPayoutAccount(
  payload: UpsertReferralPayoutAccountRequest
): Promise<CommissionPayoutAccount> {
  const { data } = await apiClient.post<CommissionPayoutAccount>(
    '/user/referral/payout-accounts',
    payload
  )
  return data
}

export async function updatePayoutAccount(
  id: number,
  payload: UpsertReferralPayoutAccountRequest
): Promise<CommissionPayoutAccount> {
  const { data } = await apiClient.put<CommissionPayoutAccount>(
    `/user/referral/payout-accounts/${id}`,
    payload
  )
  return data
}

export async function getInviteeRewards(sourceUserID: number): Promise<UserInviteeReward[]> {
  const { data } = await apiClient.get<UserInviteeReward[]>(
    `/user/referral/invitees/${sourceUserID}/rewards`
  )
  return data
}

const referralAPI = {
  getOverview,
  getLedger,
  getInvitees,
  getInviteeRewards,
  convertToCredit,
  validateCode,
  createWithdrawal,
  getWithdrawals,
  getPayoutAccounts,
  createPayoutAccount,
  updatePayoutAccount
}

export default referralAPI
