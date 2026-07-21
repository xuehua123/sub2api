import { describe, expect, it } from 'vitest'
import {
  formatPayoutMethod,
  formatWithdrawalStatus,
  isCreditConversion
} from '../referralWithdrawalDisplay'

describe('referralWithdrawalDisplay', () => {
  it('detects credit conversion method', () => {
    expect(isCreditConversion('credit_conversion')).toBe(true)
    expect(isCreditConversion('alipay')).toBe(false)
  })

  it('labels paid cash vs paid conversion differently', () => {
    expect(formatWithdrawalStatus('paid', 'alipay')).toBe('已打款')
    expect(formatWithdrawalStatus('paid', 'credit_conversion')).toBe('已转余额')
    expect(formatWithdrawalStatus('approved', 'alipay')).toBe('已通过·待打款')
  })

  it('maps payout methods for operators', () => {
    expect(formatPayoutMethod('credit_conversion')).toBe('平台余额')
    expect(formatPayoutMethod('wechat')).toBe('微信')
  })
})
