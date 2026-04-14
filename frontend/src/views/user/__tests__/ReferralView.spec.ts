import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ReferralView from '../ReferralView.vue'

const {
  getOverview,
  getInvitees,
  getLedger,
  getWithdrawals,
  getPayoutAccounts,
  bindReferralCode,
  createWithdrawal,
  createPayoutAccount,
  updatePayoutAccount,
  validateReferralCode,
  showError,
  showSuccess
} = vi.hoisted(() => ({
  getOverview: vi.fn(),
  getInvitees: vi.fn(),
  getLedger: vi.fn(),
  getWithdrawals: vi.fn(),
  getPayoutAccounts: vi.fn(),
  bindReferralCode: vi.fn(),
  createWithdrawal: vi.fn(),
  createPayoutAccount: vi.fn(),
  updatePayoutAccount: vi.fn(),
  validateReferralCode: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/auth', () => ({
  validateReferralCode
}))

vi.mock('@/api/referral', () => ({
  default: {
    getOverview,
    getInvitees,
    getLedger,
    getWithdrawals,
    getPayoutAccounts,
    bindReferralCode,
    createWithdrawal,
    createPayoutAccount,
    updatePayoutAccount
  },
  getOverview,
  getInvitees,
  getLedger,
  getWithdrawals,
  getPayoutAccounts,
  bindReferralCode,
  createWithdrawal,
  createPayoutAccount,
  updatePayoutAccount
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, fallback?: string) => fallback || key
    })
  }
})

describe('user ReferralView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getOverview.mockResolvedValue({
      referral_enabled: true,
      allow_manual_input: true,
      bind_before_first_paid_only: true,
      referral_withdraw_enabled: true,
      referral_credit_conversion_enabled: true,
      settlement_currency: 'CNY',
      default_code: { id: 1, user_id: 7, code: 'REF-007', status: 'active', is_default: true, created_at: '2026-04-09T00:00:00Z', updated_at: '2026-04-09T00:00:00Z' },
      relation: null,
      can_bind: true,
      has_paid_recharge: false,
      withdraw_methods_enabled: ['alipay', 'wechat', 'bank'],
      direct_invitees: 3,
      second_level_invitees: 5,
      pending_commission: 12,
      available_commission: 34,
      frozen_commission: 5,
      withdrawn_commission: 18,
      total_commission: 69
    })
    getInvitees.mockResolvedValue({ items: [{ user_id: 10, email: 'invitee@example.com', username: 'invitee', bound_at: '2026-04-09T00:00:00Z', second_level_num: 1, total_recharge: 0 }], total: 1, page: 1, page_size: 20, pages: 1 })
    getLedger.mockResolvedValue({ items: [{ id: 1, user_id: 7, entry_type: 'reward_pending_credit', bucket: 'pending', amount: 12, currency: 'CNY', created_at: '2026-04-09T00:00:00Z' }], total: 1, page: 1, page_size: 20, pages: 1 })
    getWithdrawals.mockResolvedValue({ items: [{ id: 1, user_id: 7, withdrawal_no: 'WD001', amount: 20, fee_amount: 1, net_amount: 19, currency: 'CNY', status: 'paid', payout_method: 'alipay', created_at: '2026-04-09T00:00:00Z', updated_at: '2026-04-09T00:00:00Z' }], total: 1, page: 1, page_size: 20, pages: 1 })
    getPayoutAccounts.mockResolvedValue([{ id: 1, user_id: 7, method: 'alipay', account_name: 'Alice', account_no_masked: 'alice@example.com', is_default: true, status: 'active', created_at: '2026-04-01T00:00:00Z', updated_at: '2026-04-01T00:00:00Z' }])
    validateReferralCode.mockResolvedValue({ valid: true })
  })

  it('renders referral overview data', async () => {
    const wrapper = mount(ReferralView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' }
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('REF-007')
    expect(wrapper.text()).toContain('69.00')
    expect(wrapper.text()).toContain('invitee@example.com')
    expect(wrapper.text()).toContain('WD001')
  })

  it('calls API endpoints on mount', async () => {
    mount(ReferralView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' }
        }
      }
    })
    await flushPromises()

    expect(getOverview).toHaveBeenCalled()
    expect(getInvitees).toHaveBeenCalled()
    expect(getLedger).toHaveBeenCalled()
    expect(getWithdrawals).toHaveBeenCalled()
    expect(getPayoutAccounts).toHaveBeenCalled()
  })

  it('displays commission summary cards', async () => {
    const wrapper = mount(ReferralView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' }
        }
      }
    })
    await flushPromises()

    // Check that commission amounts are displayed
    expect(wrapper.text()).toContain('12.00') // pending
    expect(wrapper.text()).toContain('34.00') // available
    expect(wrapper.text()).toContain('17.00') // processing = pending + frozen
  })

  it('renders withdrawal form when accounts exist', async () => {
    const wrapper = mount(ReferralView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' }
        }
      }
    })
    await flushPromises()

    // The withdrawal form should be present
    const withdrawalForm = wrapper.find('form[data-test="withdrawal-form"]')
    expect(withdrawalForm.exists()).toBe(true)
  })

  it('renders payout account info', async () => {
    const wrapper = mount(ReferralView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' }
        }
      }
    })
    await flushPromises()

    // Payout account name should be visible
    expect(wrapper.text()).toContain('Alice')
  })

  it('allows credit conversion when withdrawals are disabled but conversion is enabled', async () => {
    getOverview.mockResolvedValueOnce({
      referral_enabled: true,
      allow_manual_input: true,
      bind_before_first_paid_only: true,
      referral_withdraw_enabled: false,
      referral_credit_conversion_enabled: true,
      settlement_currency: 'CNY',
      default_code: { id: 1, user_id: 7, code: 'REF-007', status: 'active', is_default: true, created_at: '2026-04-09T00:00:00Z', updated_at: '2026-04-09T00:00:00Z' },
      relation: null,
      can_bind: true,
      has_paid_recharge: false,
      withdraw_methods_enabled: ['alipay', 'wechat', 'bank'],
      direct_invitees: 3,
      second_level_invitees: 5,
      pending_commission: 12,
      available_commission: 34,
      frozen_commission: 5,
      withdrawn_commission: 18,
      total_commission: 69
    })

    const wrapper = mount(ReferralView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' }
        }
      }
    })
    await flushPromises()

    expect(wrapper.text()).toContain('将佣金转储为平台余额')
    expect(wrapper.find('form[data-test="withdrawal-form"]').exists()).toBe(false)
  })
})
