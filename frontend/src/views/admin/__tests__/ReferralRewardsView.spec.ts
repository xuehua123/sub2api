import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ReferralRewardsView from '../ReferralRewardsView.vue'

const { listCommissionRewards, showError } = vi.hoisted(() => ({
  listCommissionRewards: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin/referral', () => ({
  default: {
    listCommissionRewards
  },
  listCommissionRewards
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showSuccess: vi.fn()
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

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ replace: vi.fn() })
}))

describe('admin ReferralRewardsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listCommissionRewards.mockResolvedValue({
      items: [
        {
          id: 11,
          user_id: 1,
          user_email: 'ref@example.com',
          username: 'ref',
          source_user_id: 2,
          source_user_email: 'invitee@example.com',
          source_username: 'invitee',
          recharge_order_id: 99,
          external_order_id: 'ORD-99',
          level: 1,
          base_amount_snapshot: 100,
          rate_snapshot: 0.1,
          reward_amount: 10,
          status: 'available',
          created_at: '2026-07-01T00:00:00Z'
        }
      ],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
  })

  it('loads and renders commission booking rows', async () => {
    const wrapper = mount(ReferralRewardsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="actions" /><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          Pagination: true,
          LoadingSpinner: true,
          'router-link': { template: '<a><slot /></a>' }
        }
      }
    })

    await flushPromises()

    expect(listCommissionRewards).toHaveBeenCalled()
    expect(wrapper.text()).toContain('ref@example.com')
    expect(wrapper.text()).toContain('invitee@example.com')
    expect(wrapper.text()).toContain('ORD-99')
    expect(wrapper.text()).toContain('10.00')
  })

  it('shows available_at for pending rewards', async () => {
    listCommissionRewards.mockResolvedValueOnce({
      items: [
        {
          id: 12,
          user_id: 1,
          user_email: 'ref@example.com',
          username: 'ref',
          source_user_id: 2,
          source_user_email: 'invitee@example.com',
          source_username: 'invitee',
          recharge_order_id: 100,
          external_order_id: 'ORD-100',
          level: 1,
          base_amount_snapshot: 50,
          rate_snapshot: 0.1,
          reward_amount: 5,
          status: 'pending',
          available_at: '2026-07-10T12:00:00Z',
          created_at: '2026-07-01T00:00:00Z'
        }
      ],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })

    const wrapper = mount(ReferralRewardsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="actions" /><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          Pagination: true,
          LoadingSpinner: true,
          'router-link': { template: '<a><slot /></a>' }
        }
      }
    })
    await flushPromises()

    expect(wrapper.text()).toContain('ORD-100')
    // locale string or raw date from toLocaleString
    expect(wrapper.html()).toMatch(/2026/)
  })
})
