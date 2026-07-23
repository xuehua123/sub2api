import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ReferralView from '../ReferralView.vue'

const {
  getOverview,
  getInvitees,
  getRewards,
  getLedger,
  getWithdrawals,
  getPayoutAccounts,
  bindReferralCode,
  createWithdrawal,
  createPayoutAccount,
  updatePayoutAccount,
  convertToCredit,
  validateReferralCode,
  showError,
  showSuccess,
  showInfo,
  storeState
} = vi.hoisted(() => ({
  getOverview: vi.fn(),
  getInvitees: vi.fn(),
  getRewards: vi.fn(),
  getLedger: vi.fn(),
  getWithdrawals: vi.fn(),
  getPayoutAccounts: vi.fn(),
  bindReferralCode: vi.fn(),
  createWithdrawal: vi.fn(),
  createPayoutAccount: vi.fn(),
  updatePayoutAccount: vi.fn(),
  convertToCredit: vi.fn(),
  validateReferralCode: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  showInfo: vi.fn(),
  storeState: {
    cachedPublicSettings: null as null | Record<string, unknown>
  }
}))

vi.mock('@/api/auth', () => ({
  validateReferralCode
}))

vi.mock('@/api/referral', () => ({
  default: {
    getOverview,
    getInvitees,
    getRewards,
    getLedger,
    getWithdrawals,
    getPayoutAccounts,
    bindReferralCode,
    createWithdrawal,
    createPayoutAccount,
    updatePayoutAccount,
    convertToCredit
  },
  getOverview,
  getInvitees,
  getRewards,
  getLedger,
  getWithdrawals,
  getPayoutAccounts,
  bindReferralCode,
  createWithdrawal,
  createPayoutAccount,
  updatePayoutAccount,
  convertToCredit
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showInfo,
    get cachedPublicSettings() {
      return storeState.cachedPublicSettings
    }
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, arg?: string | Record<string, unknown>) => {
        if (typeof arg === 'string') return arg
        if (arg && typeof arg === 'object') {
          return Object.entries(arg).reduce(
            (msg, [k, v]) => msg.replace(new RegExp(`\\{${k}\\}`, 'g'), String(v)),
            key
          )
        }
        return key
      }
    })
  }
})

describe('user ReferralView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    storeState.cachedPublicSettings = null
    getOverview.mockResolvedValue({
      referral_enabled: true,
      allow_manual_input: true,
      bind_before_first_paid_only: true,
      referral_withdraw_enabled: true,
      referral_credit_conversion_enabled: true,
      referral_credit_conversion_rate: 1,
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
      total_commission: 69,
      level1_enabled: true,
      level1_rate: 0.15,
      reward_mode: 'every_paid_order',
      settlement_delay_days: 7
    })
    getInvitees.mockResolvedValue({ items: [{ user_id: 10, email: 'invitee@example.com', username: 'invitee', bound_at: '2026-04-09T00:00:00Z', second_level_num: 1, total_recharge: 0 }], total: 1, page: 1, page_size: 20, pages: 1 })
    getRewards.mockResolvedValue({
      items: [{
        id: 9,
        source_user_email: 'invitee@example.com',
        external_order_id: 'ORD-1',
        order_paid_amount: 100,
        rate_snapshot: 0.1,
        reward_amount: 10,
        status: 'available',
        created_at: '2026-04-09T00:00:00Z'
      }],
      total: 1,
      page: 1,
      page_size: 15,
      pages: 1
    })
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
    expect(wrapper.text()).toContain('ORD-1')
    expect(wrapper.text()).toContain('10.00')
    // Rate banner must show how much users earn (every-paid mode copy)
    expect(wrapper.find('[data-test="referral-rate-pct"]').text()).toMatch(/15/)
    expect(wrapper.find('[data-test="referral-rate-banner"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="referral-rate-scope"]').text()).toMatch(
      /每笔|every|perRechargeEvery/i
    )
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
    expect(getRewards).toHaveBeenCalled()
    expect(getLedger).toHaveBeenCalled()
    expect(getWithdrawals).toHaveBeenCalled()
    expect(getPayoutAccounts).toHaveBeenCalled()
  })

  it('keeps invitees and withdrawals when getRewards fails', async () => {
    getRewards.mockRejectedValueOnce(new Error('rewards down'))

    const wrapper = mount(ReferralView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' }
        }
      }
    })
    await flushPromises()

    expect(wrapper.text()).toContain('invitee@example.com')
    expect(wrapper.text()).toContain('WD001')
    expect(wrapper.text()).not.toContain('ORD-1')
    expect(showError).toHaveBeenCalled()
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
      referral_credit_conversion_rate: 1,
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

    expect(wrapper.find('[data-test="open-convert-credit"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="credit-conversion-rate-hint"]').exists()).toBe(true)
    expect(wrapper.find('form[data-test="withdrawal-form"]').exists()).toBe(false)
  })

  it('shows conversion multiplier and expected credit for non-1 rate', async () => {
    // Use a rate whose product is NOT equal after toFixed(2), so 2-decimal formatting would fail.
    // 1 * 1.2345 = 1.2345 → toFixed(2) => "1.23", precise => "1.2345"
    getOverview.mockResolvedValueOnce({
      referral_enabled: true,
      allow_manual_input: true,
      bind_before_first_paid_only: true,
      referral_withdraw_enabled: false,
      referral_credit_conversion_enabled: true,
      referral_credit_conversion_rate: 1.2345,
      settlement_currency: 'CNY',
      default_code: { id: 1, user_id: 7, code: 'REF-007', status: 'active', is_default: true, created_at: '2026-04-09T00:00:00Z', updated_at: '2026-04-09T00:00:00Z' },
      relation: null,
      can_bind: true,
      has_paid_recharge: false,
      withdraw_methods_enabled: [],
      direct_invitees: 1,
      second_level_invitees: 0,
      pending_commission: 0,
      available_commission: 10,
      frozen_commission: 0,
      withdrawn_commission: 0,
      total_commission: 10,
      level1_enabled: true,
      level1_rate: 0.15,
      reward_mode: 'every_paid_order',
      settlement_delay_days: 7
    })
    convertToCredit.mockResolvedValueOnce({})

    const wrapper = mount(ReferralView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' }
        }
      }
    })
    await flushPromises()

    expect(wrapper.find('[data-test="credit-conversion-rate-hint"]').attributes('data-conversion-rate')).toMatch(
      /1\.2345/
    )
    await wrapper.find('[data-test="open-convert-credit"]').trigger('click')
    await flushPromises()

    const amountInput = wrapper.find('[data-test="convert-amount-input"]')
    expect(amountInput.exists()).toBe(true)
    await amountInput.setValue(1)
    await flushPromises()

    const expectedText = wrapper.find('[data-test="convert-expected-credit"]').text()
    expect(expectedText).toMatch(/1\.2345/)
    expect(expectedText).not.toMatch(/¥1\.23(?!45)/)
  })

  it('does not fall back to public-settings cache when overview level1_rate is explicitly 0', async () => {
    // Stale public cache still advertises 15% — must not win over explicit overview 0.
    storeState.cachedPublicSettings = {
      referral_level1_enabled: true,
      referral_level1_rate: 0.15,
      referral_reward_mode: 'every_paid_order',
      referral_settlement_delay_days: 7
    }
    getOverview.mockResolvedValueOnce({
      referral_enabled: true,
      allow_manual_input: true,
      bind_before_first_paid_only: true,
      referral_withdraw_enabled: true,
      referral_credit_conversion_enabled: false,
      referral_credit_conversion_rate: 1,
      settlement_currency: 'CNY',
      default_code: { id: 1, user_id: 7, code: 'REF-007', status: 'active', is_default: true, created_at: '2026-04-09T00:00:00Z', updated_at: '2026-04-09T00:00:00Z' },
      relation: null,
      can_bind: true,
      has_paid_recharge: false,
      withdraw_methods_enabled: ['alipay'],
      direct_invitees: 0,
      second_level_invitees: 0,
      pending_commission: 0,
      available_commission: 0,
      frozen_commission: 0,
      withdrawn_commission: 0,
      total_commission: 0,
      level1_enabled: true,
      level1_rate: 0,
      reward_mode: 'every_paid_order',
      settlement_delay_days: 7
    })

    const wrapper = mount(ReferralView, {
      global: {
        stubs: { AppLayout: { template: '<div><slot /></div>' } }
      }
    })
    await flushPromises()

    expect(wrapper.find('[data-test="referral-rate-examples"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="referral-rate-pct"]').text()).not.toMatch(/15/)
    expect(wrapper.find('[data-test="referral-rate-pct"]').text()).toMatch(/—|–|-/)
    // Must not reuse the "earn commission" marketing fallback subtitle either.
    expect(wrapper.text()).not.toMatch(/马上开赚/)
  })

  it('does not promise auto-credit when level1 is on but rate is zero', async () => {
    getOverview.mockResolvedValueOnce({
      referral_enabled: true,
      allow_manual_input: true,
      bind_before_first_paid_only: true,
      referral_withdraw_enabled: true,
      referral_credit_conversion_enabled: false,
      referral_credit_conversion_rate: 1,
      settlement_currency: 'CNY',
      default_code: { id: 1, user_id: 7, code: 'REF-007', status: 'active', is_default: true, created_at: '2026-04-09T00:00:00Z', updated_at: '2026-04-09T00:00:00Z' },
      relation: null,
      can_bind: true,
      has_paid_recharge: false,
      withdraw_methods_enabled: ['alipay'],
      direct_invitees: 0,
      second_level_invitees: 0,
      pending_commission: 0,
      available_commission: 0,
      frozen_commission: 0,
      withdrawn_commission: 0,
      total_commission: 0,
      level1_enabled: true,
      level1_rate: 0,
      reward_mode: 'first_paid_order',
      settlement_delay_days: 7
    })

    const wrapper = mount(ReferralView, {
      global: {
        stubs: { AppLayout: { template: '<div><slot /></div>' } }
      }
    })
    await flushPromises()

    const banner = wrapper.find('[data-test="referral-rate-banner"]')
    expect(banner.exists()).toBe(true)
    // Fallback marketing that implies earning must not appear.
    expect(banner.text()).not.toMatch(/你获得佣金|earn commission|titleEarn/)
    expect(banner.text()).not.toMatch(/首次成功充值记佣|每笔成功充值记佣|bulletAutoFirst|bulletAutoEvery/)
    expect(wrapper.find('[data-test="referral-share-card"]').text()).not.toMatch(/马上开赚/)
  })

  it('hides fake rate examples when level1_rate is missing', async () => {
    getOverview.mockResolvedValueOnce({
      referral_enabled: true,
      allow_manual_input: true,
      bind_before_first_paid_only: true,
      referral_withdraw_enabled: true,
      referral_credit_conversion_enabled: false,
      referral_credit_conversion_rate: 1,
      settlement_currency: 'CNY',
      default_code: { id: 1, user_id: 7, code: 'REF-007', status: 'active', is_default: true, created_at: '2026-04-09T00:00:00Z', updated_at: '2026-04-09T00:00:00Z' },
      relation: null,
      can_bind: true,
      has_paid_recharge: false,
      withdraw_methods_enabled: ['alipay'],
      direct_invitees: 0,
      second_level_invitees: 0,
      pending_commission: 0,
      available_commission: 0,
      frozen_commission: 0,
      withdrawn_commission: 0,
      total_commission: 0,
      level1_enabled: true,
      level1_rate: 0,
      reward_mode: 'first_paid_order'
    })

    const wrapper = mount(ReferralView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' }
        }
      }
    })
    await flushPromises()

    expect(wrapper.find('[data-test="referral-rate-examples"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="referral-rate-pending"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="referral-rate-pct"]').text()).toMatch(/—|–|-/)
  })

  it('hides commission promise when level1 is disabled even if rate is non-zero', async () => {
    getOverview.mockResolvedValueOnce({
      referral_enabled: true,
      allow_manual_input: true,
      bind_before_first_paid_only: true,
      referral_withdraw_enabled: true,
      referral_credit_conversion_enabled: false,
      referral_credit_conversion_rate: 1,
      settlement_currency: 'CNY',
      default_code: { id: 1, user_id: 7, code: 'REF-007', status: 'active', is_default: true, created_at: '2026-04-09T00:00:00Z', updated_at: '2026-04-09T00:00:00Z' },
      relation: null,
      can_bind: true,
      has_paid_recharge: false,
      withdraw_methods_enabled: ['alipay'],
      direct_invitees: 0,
      second_level_invitees: 0,
      pending_commission: 0,
      available_commission: 0,
      frozen_commission: 0,
      withdrawn_commission: 0,
      total_commission: 0,
      // Backend should zero rate when disabled; simulate client-side gate too.
      level1_enabled: false,
      level1_rate: 0,
      reward_mode: 'every_paid_order'
    })

    const wrapper = mount(ReferralView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' }
        }
      }
    })
    await flushPromises()

    expect(wrapper.find('[data-test="referral-rate-examples"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="referral-rate-pct"]').text()).toMatch(/—|–|-/)
    expect(wrapper.find('[data-test="referral-rate-scope"]').text()).toMatch(
      /未开启|disabled|perRechargeDisabled|rateDisabled/i
    )
  })

  it('opens total commission detail with all buckets not only available', async () => {
    getLedger.mockResolvedValue({
      items: [
        { id: 1, bucket: 'available', amount: 10, entry_type: 'reward_pending_to_available', created_at: '2026-04-09T00:00:00Z' },
        { id: 2, bucket: 'pending', amount: 5, entry_type: 'reward_pending_credit', created_at: '2026-04-09T00:00:00Z' },
        { id: 3, bucket: 'settled', amount: 8, entry_type: 'withdraw_paid', created_at: '2026-04-09T00:00:00Z' }
      ],
      total: 3,
      page: 1,
      page_size: 100,
      pages: 1
    })

    const wrapper = mount(ReferralView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' }
        }
      }
    })
    await flushPromises()

    // Third metric button is total commission → openBucket('all', ...)
    const metricButtons = wrapper.findAll('[data-test="referral-wallet-card"] button')
    const totalBtn = metricButtons.find((b) => b.text().includes('累计') || b.text().includes('totalCommission') || b.text().includes('Total'))
    expect(totalBtn).toBeTruthy()
    await totalBtn!.trigger('click')
    await flushPromises()

    expect(getLedger).toHaveBeenCalled()
    // Modal should include pending/settled amounts, not only available
    expect(wrapper.text()).toMatch(/5\.00/)
    expect(wrapper.text()).toMatch(/8\.00/)
  })

  it('renders disabled state instead of referral center when referral is disabled', async () => {
    getOverview.mockResolvedValueOnce({
      referral_enabled: false,
      allow_manual_input: false,
      bind_before_first_paid_only: true,
      referral_withdraw_enabled: false,
      referral_credit_conversion_enabled: false,
      referral_credit_conversion_rate: 1,
      settlement_currency: 'CNY',
      default_code: { id: 1, user_id: 7, code: 'REF-007', status: 'active', is_default: true, created_at: '2026-04-09T00:00:00Z', updated_at: '2026-04-09T00:00:00Z' },
      relation: null,
      can_bind: false,
      has_paid_recharge: false,
      withdraw_methods_enabled: [],
      direct_invitees: 0,
      second_level_invitees: 0,
      pending_commission: 0,
      available_commission: 0,
      frozen_commission: 0,
      withdrawn_commission: 0,
      total_commission: 0
    })

    const wrapper = mount(ReferralView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' }
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toMatch(/邀请功能未开启|referral\.disabledTitle/)
    expect(wrapper.text()).not.toContain('REF-007')
    expect(wrapper.find('form[data-test="withdrawal-form"]').exists()).toBe(false)
    expect(getInvitees).not.toHaveBeenCalled()
    expect(getLedger).not.toHaveBeenCalled()
    expect(getWithdrawals).not.toHaveBeenCalled()
    expect(getPayoutAccounts).not.toHaveBeenCalled()
  })
})
