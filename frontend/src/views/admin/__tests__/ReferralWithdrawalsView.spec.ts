import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ReferralWithdrawalsView from '../ReferralWithdrawalsView.vue'

const {
  listWithdrawals,
  getWithdrawalItems,
  approveWithdrawal,
  rejectWithdrawal,
  markWithdrawalPaid,
  showError,
  showSuccess
} = vi.hoisted(() => ({
  listWithdrawals: vi.fn(),
  getWithdrawalItems: vi.fn(),
  approveWithdrawal: vi.fn(),
  rejectWithdrawal: vi.fn(),
  markWithdrawalPaid: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin/referral', () => ({
  default: {
    listWithdrawals,
    getWithdrawalItems,
    approveWithdrawal,
    rejectWithdrawal,
    markWithdrawalPaid
  },
  listWithdrawals,
  getWithdrawalItems,
  approveWithdrawal,
  rejectWithdrawal,
  markWithdrawalPaid
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
      t: (key: string) => key
    })
  }
})

describe('admin ReferralWithdrawalsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listWithdrawals.mockResolvedValue({
      items: [
        {
          id: 1,
          user_id: 7,
          withdrawal_no: 'WD001',
          amount: 20,
          fee_amount: 1,
          net_amount: 19,
          currency: 'CNY',
        status: 'pending_review',
        payout_method: 'alipay',
        payout_account_snapshot_json: JSON.stringify({
          method: 'alipay',
          account_name: 'Alice',
          account_no_encrypted: 'alipay@example.com',
          qr_image_url: 'https://example.com/qr.png'
        }),
        user_email: 'user@example.com',
        username: 'user',
        item_count: 2,
          created_at: '2026-04-09T00:00:00Z',
          updated_at: '2026-04-09T00:00:00Z'
        }
      ],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    getWithdrawalItems.mockResolvedValue([
      {
        id: 1,
        withdrawal_id: 1,
        user_id: 7,
        reward_id: 10,
        recharge_order_id: 20,
        allocated_amount: 12,
        fee_allocated_amount: 1,
        net_allocated_amount: 11,
        currency: 'CNY',
        status: 'frozen',
        created_at: '2026-04-09T00:00:00Z',
        updated_at: '2026-04-09T00:00:00Z'
      }
    ])
    approveWithdrawal.mockResolvedValue({})
    rejectWithdrawal.mockResolvedValue({})
    markWithdrawalPaid.mockResolvedValue({})
  })

  it('opens withdrawal details drawer', async () => {
    const wrapper = mount(ReferralWithdrawalsView, {
      attachTo: document.body,
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          RouterLink: { template: '<a><slot /></a>' }
        }
      }
    })
    await flushPromises()

    // The view details button is the one with t('admin.referral.viewDetails') text
    const link = wrapper.findAll('button').find((btn) => btn.text().includes('admin.referral.viewDetails'))
    expect(link).toBeTruthy()
    await link!.trigger('click')
    await flushPromises()

    expect(getWithdrawalItems).toHaveBeenCalledWith(1)
    expect(document.body.textContent).toContain('admin.referral.withdrawalItemsTitle')
    expect(document.body.textContent).toContain('12.00')
    expect(document.body.textContent).toContain('alipay@example.com')
  })
})
