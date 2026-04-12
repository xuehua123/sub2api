import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ReferralView from '../ReferralView.vue'
import AccountWorkbenchDrawer from '../referral-components/AccountWorkbenchDrawer.vue'

const {
  searchAccounts,
  getOverview,
  getTree,
  listRelations,
  listRelationHistories,
  listCommissionRewards,
  createCommissionAdjustment,
  updateRelation,
  listWithdrawals,
  getWithdrawalItems,
  showError,
  showSuccess
} = vi.hoisted(() => ({
  searchAccounts: vi.fn(),
  getOverview: vi.fn(),
  getTree: vi.fn(),
  listRelations: vi.fn(),
  listRelationHistories: vi.fn(),
  listCommissionRewards: vi.fn(),
  createCommissionAdjustment: vi.fn(),
  updateRelation: vi.fn(),
  listWithdrawals: vi.fn(),
  getWithdrawalItems: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin/referral', () => ({
  default: {
    searchAccounts,
    getOverview,
    getTree,
    listRelations,
    listRelationHistories,
    listCommissionRewards,
    createCommissionAdjustment,
    updateRelation,
    listWithdrawals,
    getWithdrawalItems
  },
  searchAccounts,
  getOverview,
  getTree,
  listRelations,
  listRelationHistories,
  listCommissionRewards,
  createCommissionAdjustment,
  updateRelation,
  listWithdrawals,
  getWithdrawalItems
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

vi.mock('vue-chartjs', () => ({
  Bar: {
    props: ['data'],
    template: '<div class="mock-bar-chart">{{ data?.labels?.join(",") }}</div>'
  }
}))

describe('admin ReferralView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    searchAccounts.mockResolvedValue([
      { user_id: 7, email: 'user@example.com', username: 'user', referral_code: 'REF-007' },
      { user_id: 99, email: 'parent@example.com', username: 'parent', referral_code: 'PARENT01' }
    ])
    getOverview.mockResolvedValue({
      total_accounts: 2,
      total_bound_users: 1,
      pending_commission: 12,
      available_commission: 34,
      frozen_commission: 5,
      withdrawn_commission: 18,
      pending_withdrawal_count: 1,
      pending_withdrawal_amount: 19,
      recent_trend: [
        { date: '2026-04-03', reward_amount: 0, withdrawal_amount: 0 },
        { date: '2026-04-04', reward_amount: 0, withdrawal_amount: 0 },
        { date: '2026-04-05', reward_amount: 3, withdrawal_amount: 0 },
        { date: '2026-04-06', reward_amount: 8, withdrawal_amount: 0 },
        { date: '2026-04-07', reward_amount: 5, withdrawal_amount: 2 },
        { date: '2026-04-08', reward_amount: 12, withdrawal_amount: 0 },
        { date: '2026-04-09', reward_amount: 18, withdrawal_amount: 4 }
      ],
      ranking: [
        {
          user_id: 7,
          email: 'user@example.com',
          username: 'user',
          referral_code: 'REF-007',
          direct_invitees: 2,
          second_level_invitees: 3,
          total_commission: 88,
          available_commission: 34,
          withdrawn_commission: 18
        }
      ]
    })
    getTree.mockResolvedValue({
      user_id: 7,
      email: 'user@example.com',
      username: 'user',
      referral_code: 'REF-007',
      level: 0,
      direct_invitees: 2,
      second_level_invitees: 3,
      total_commission: 88,
      available_commission: 34,
      children: [
        {
          user_id: 10,
          email: 'child@example.com',
          username: 'child',
          referral_code: 'CHILD001',
          level: 1,
          direct_invitees: 1,
          second_level_invitees: 0,
          total_commission: 10,
          available_commission: 5,
          children: []
        }
      ]
    })
    listRelations.mockResolvedValue({
      items: [{ user_id: 7, user_email: 'user@example.com', username: 'user', referrer_user_id: 99, referrer_email: 'parent@example.com', referrer_username: 'parent', bind_source: 'link', bind_code: 'REF-007', created_at: '2026-04-09T00:00:00Z', updated_at: '2026-04-09T00:00:00Z' }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    listRelationHistories.mockResolvedValue({
      items: [
        {
          id: 1,
          user_id: 7,
          old_referrer_user_id: 88,
          new_referrer_user_id: 99,
          old_bind_code: 'OLD001',
          new_bind_code: 'PARENT01',
          change_source: 'admin_override',
          reason: 'manual correction',
          created_at: '2026-04-09T01:00:00Z'
        }
      ],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    listCommissionRewards.mockResolvedValue({
      items: [{ id: 1, user_id: 99, source_user_id: 7, recharge_order_id: 11, level: 1, rate_snapshot: 0.1, base_amount_snapshot: 100, reward_amount: 10, currency: 'CNY', reward_mode_snapshot: 'every_paid_order', status: 'available', created_at: '2026-04-09T00:00:00Z', updated_at: '2026-04-09T00:00:00Z', user_email: 'parent@example.com', username: 'parent', source_user_email: 'user@example.com', source_username: 'user' }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    listWithdrawals.mockResolvedValue({
      items: [{ id: 1, user_id: 7, withdrawal_no: 'WD001', amount: 20, fee_amount: 1, net_amount: 19, currency: 'CNY', status: 'pending_review', payout_method: 'alipay', user_email: 'user@example.com', username: 'user', item_count: 1, created_at: '2026-04-09T00:00:00Z', updated_at: '2026-04-09T00:00:00Z' }],
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
        reward_id: 11,
        recharge_order_id: 23,
        allocated_amount: 20,
        fee_allocated_amount: 1,
        net_allocated_amount: 19,
        currency: 'CNY',
        status: 'frozen',
        created_at: '2026-04-09T00:00:00Z',
        updated_at: '2026-04-09T00:00:00Z'
      }
    ])
  })

  it('renders overview cards, trend, ranking and pending withdrawals', async () => {
    const wrapper = mount(ReferralView, {
      global: {
        stubs: {
          teleport: true,
          AppLayout: { template: '<div><slot /></div>' },
          RouterLink: { template: '<a><slot /></a>' }
        }
      }
    })
    await flushPromises()

    expect(wrapper.text()).toContain('88.00')
    expect(wrapper.text()).toContain('WD001')
    expect(wrapper.text()).toContain('user@example.com')
    expect(wrapper.text()).toContain('admin.referral.trendTitle')
    expect(wrapper.text()).toContain('admin.referral.rankingTitle')
  })

  it('opens tree drawer', async () => {
    const wrapper = mount(ReferralView, {
      attachTo: document.body,
      global: {
        stubs: {
          teleport: true,
          AppLayout: { template: '<div><slot /></div>' },
          RouterLink: { template: '<a><slot /></a>' }
        }
      }
    })
    await flushPromises()

    const button = wrapper.findAll('button').find((btn) => btn.text() === 'admin.referral.structureTree')
    expect(button).toBeTruthy()
    await button!.trigger('click')
    await flushPromises()

    expect(getTree).toHaveBeenCalledWith(7)
    expect(document.body.textContent).toContain('admin.referral.treeTitle')
    expect(document.body.textContent).toContain('child@example.com')
  })

  it('opens workspace drawer and uses fixed upstream code for relation updates', async () => {
    updateRelation.mockResolvedValue({})

    const wrapper = mount(ReferralView, {
      attachTo: document.body,
      global: {
        stubs: {
          teleport: true,
          AppLayout: { template: '<div><slot /></div>' },
          RouterLink: { template: '<a><slot /></a>' }
        }
      }
    })
    await flushPromises()

    const manageButton = wrapper.findAll('button').find((btn) => btn.text() === 'admin.referral.workspace')
    expect(manageButton).toBeTruthy()
    await manageButton!.trigger('click')
    await flushPromises()

    await wrapper.get('[data-test="workspace-upstream-input"]').setValue('parent@example.com')
    await flushPromises()
    const drawer = wrapper.findComponent(AccountWorkbenchDrawer)
    const drawerState = (drawer.vm as any).$.setupState
    drawerState.selectedUpstream = {
      user_id: 99,
      email: 'parent@example.com',
      username: 'parent',
      referral_code: 'PARENT01'
    }
    await flushPromises()
    await drawer.get('form[data-test="workspace-relation-form"]').trigger('submit')
    await flushPromises()
    expect(updateRelation).toHaveBeenCalledWith(7, expect.objectContaining({ code: 'PARENT01' }))
  })

  it('switches to adjustment and withdrawal tabs inside workspace', async () => {
    createCommissionAdjustment.mockResolvedValue({})

    const wrapper = mount(ReferralView, {
      attachTo: document.body,
      global: {
        stubs: {
          teleport: true,
          AppLayout: { template: '<div><slot /></div>' },
          RouterLink: { template: '<a><slot /></a>' }
        }
      }
    })
    await flushPromises()

    const manageButton = wrapper.findAll('button').find((btn) => btn.text() === 'admin.referral.workspace')
    expect(manageButton).toBeTruthy()
    await manageButton!.trigger('click')
    await flushPromises()

    const drawer = wrapper.findComponent(AccountWorkbenchDrawer)

    await drawer.get('[data-test="workspace-tab-adjustment"]').trigger('click')
    await drawer.get('[data-test="select-reward-1"]').trigger('click')
    await drawer.get('[data-test="workspace-adjustment-amount"]').setValue('-2')
    await drawer.get('form[data-test="workspace-adjustment-form"]').trigger('submit')
    await flushPromises()
    expect(createCommissionAdjustment).toHaveBeenCalled()

    await drawer.get('[data-test="workspace-tab-withdrawals"]').trigger('click')
    await flushPromises()
    expect(drawer.text()).toContain('WD001')
    expect(drawer.text()).toContain('19.00')
  })
})
