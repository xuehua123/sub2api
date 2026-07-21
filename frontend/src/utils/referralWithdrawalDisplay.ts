/** Shared display helpers for cash withdrawal vs platform credit conversion. */

export const CREDIT_CONVERSION_METHOD = 'credit_conversion'

export function isCreditConversion(payoutMethod?: string | null): boolean {
  return String(payoutMethod || '').trim() === CREDIT_CONVERSION_METHOD
}

export function formatPayoutMethod(method?: string | null): string {
  const value = String(method || '').trim()
  const map: Record<string, string> = {
    alipay: '支付宝',
    wechat: '微信',
    bank: '银行卡',
    [CREDIT_CONVERSION_METHOD]: '平台余额'
  }
  return map[value] || value || '-'
}

export function formatWithdrawalStatus(status?: string | null, payoutMethod?: string | null): string {
  const s = String(status || '').trim()
  if (s === 'paid' && isCreditConversion(payoutMethod)) return '已转余额'
  const map: Record<string, string> = {
    pending_review: '待审核',
    approved: '已通过·待打款',
    rejected: '已驳回',
    paid: '已打款',
    frozen: '冻结中'
  }
  return map[s] || s || '-'
}

export function withdrawalStatusBadgeClass(status?: string | null, payoutMethod?: string | null): string {
  if (status === 'paid' && isCreditConversion(payoutMethod)) {
    return 'bg-indigo-100 text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-300'
  }
  const map: Record<string, string> = {
    pending_review: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-500/10 dark:text-yellow-400',
    approved: 'bg-blue-100 text-blue-700 dark:bg-blue-500/10 dark:text-blue-400',
    paid: 'bg-green-100 text-green-700 dark:bg-green-500/10 dark:text-green-400',
    rejected: 'bg-red-100 text-red-700 dark:bg-red-500/10 dark:text-red-400',
    frozen: 'bg-gray-100 text-gray-700 dark:bg-gray-500/10 dark:text-gray-400'
  }
  return map[String(status || '')] || 'bg-gray-100 text-gray-700 dark:bg-gray-500/10 dark:text-gray-400'
}

export function withdrawalStatusDotClass(status?: string | null, payoutMethod?: string | null): string {
  if (status === 'paid' && isCreditConversion(payoutMethod)) return 'bg-indigo-500'
  const map: Record<string, string> = {
    pending_review: 'bg-yellow-500 animate-pulse',
    approved: 'bg-blue-500',
    paid: 'bg-green-500',
    rejected: 'bg-red-500',
    frozen: 'bg-gray-500'
  }
  return map[String(status || '')] || 'bg-gray-400'
}
