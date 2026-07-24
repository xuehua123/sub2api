import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SubscriptionProgressMini from '../SubscriptionProgressMini.vue'

const subscriptions = vi.hoisted(() => ({
  activeSubscriptions: [] as any[],
  entitlements: [] as any[],
  fetchActiveSubscriptions: vi.fn(),
  fetchEntitlements: vi.fn(),
}))

vi.mock('@/stores', () => ({
  useSubscriptionStore: () => ({
    activeSubscriptions: subscriptions.activeSubscriptions,
    entitlements: subscriptions.entitlements,
    hasActiveSubscriptions: subscriptions.activeSubscriptions.length > 0,
    fetchActiveSubscriptions: subscriptions.fetchActiveSubscriptions,
    fetchEntitlements: subscriptions.fetchEntitlements,
  })
}))

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    getPlans: vi.fn().mockResolvedValue({ data: [] })
  }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => {
      if (key === 'subscriptionProgress.activeCount') return `${params?.count} active`
      if (key === 'subscriptionProgress.unlimited') return 'Unlimited'
      if (key === 'subscriptionProgress.monthly') return 'Monthly'
      if (key === 'subscriptionProgress.viewAll') return 'View all'
      if (key === 'subscriptionProgress.title') return 'My plans'
      if (key === 'subscriptionProgress.expired') return 'Expired'
      return key
    }
  })
}))

function activeAlias(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    user_id: 1,
    group_id: 10,
    status: 'active',
    starts_at: '2026-06-01T00:00:00Z',
    expires_at: '2026-07-01T00:00:00Z',
    daily_usage_usd: 0,
    weekly_usage_usd: 0,
    monthly_usage_usd: 120,
    daily_limit_usd: null,
    weekly_limit_usd: null,
    monthly_limit_usd: 600,
    daily_window_start: null,
    weekly_window_start: null,
    monthly_window_start: null,
    created_at: '2026-06-01T00:00:00Z',
    updated_at: '2026-06-01T00:00:00Z',
    entitlement_id: 100,
    plan_id: 20,
    group: {
      id: 10,
      name: '轻享月卡',
      daily_limit_usd: null,
      weekly_limit_usd: null,
      monthly_limit_usd: null
    },
    ...overrides
  }
}

describe('SubscriptionProgressMini', () => {
  beforeEach(() => {
    subscriptions.activeSubscriptions = []
    subscriptions.entitlements = []
    subscriptions.fetchActiveSubscriptions.mockReset()
    subscriptions.fetchEntitlements.mockReset()
    subscriptions.fetchActiveSubscriptions.mockResolvedValue(subscriptions.activeSubscriptions)
    subscriptions.fetchEntitlements.mockResolvedValue(subscriptions.entitlements)
  })

  it('uses alias quota limits before group limits', async () => {
    subscriptions.activeSubscriptions = [activeAlias()]
    subscriptions.fetchActiveSubscriptions.mockResolvedValue(subscriptions.activeSubscriptions)

    const wrapper = mount(SubscriptionProgressMini, {
      global: {
        stubs: {
          RouterLink: true
        }
      }
    })

    await wrapper.find('button').trigger('click')

    expect(wrapper.text()).toContain('轻享月卡')
    expect(wrapper.text()).toContain('$120.00/$600.00')
    expect(wrapper.text()).not.toContain('Unlimited')
  })

  it('does not fall back to group weekly limit for entitlement-backed subscriptions', async () => {
    subscriptions.activeSubscriptions = [
      activeAlias({
        weekly_usage_usd: 667.56,
        weekly_limit_usd: null,
        group: {
          id: 10,
          name: 'OpenAI Main',
          daily_limit_usd: null,
          weekly_limit_usd: 300,
          monthly_limit_usd: null
        }
      })
    ]
    subscriptions.fetchActiveSubscriptions.mockResolvedValue(subscriptions.activeSubscriptions)

    const wrapper = mount(SubscriptionProgressMini, {
      global: {
        stubs: {
          RouterLink: true
        }
      }
    })

    await wrapper.find('button').trigger('click')

    expect(wrapper.text()).toContain('$120.00/$600.00')
    expect(wrapper.text()).not.toContain('$667.56/$300.00')
  })

  it('shows a pure V2 entitlement that has no legacy subscription alias', async () => {
    subscriptions.entitlements = [{
      id: 100,
      plan_id: 20,
      plan_name: '全平台月卡',
      name: '全平台月卡',
      status: 'active',
      starts_at: '2026-06-01T00:00:00Z',
      expires_at: '2099-07-01T00:00:00Z',
      groups: [
        { id: 10, name: 'OpenAI', platform: 'openai', rate_multiplier: 1 },
        { id: 11, name: 'Claude', platform: 'anthropic', rate_multiplier: 1.2 },
      ],
      daily_limit_usd: null,
      daily_usage_usd: 0,
      daily_window_start: null,
      daily_resets_at: null,
      daily_resets_in_seconds: null,
      weekly_limit_usd: null,
      weekly_usage_usd: 0,
      weekly_window_start: null,
      weekly_resets_at: null,
      weekly_resets_in_seconds: null,
      monthly_limit_usd: 600,
      monthly_usage_usd: 120,
      monthly_window_start: null,
      monthly_resets_at: null,
      monthly_resets_in_seconds: null,
      overage_policy: 'block',
      legacy_subscription_id: null,
      purchase_price: 99,
      purchase_currency: 'CNY',
      quota_usd: 600,
      quota_period: 'monthly',
      unit_cost_per_usd: 0.165,
    }]
    subscriptions.fetchEntitlements.mockResolvedValue(subscriptions.entitlements)

    const wrapper = mount(SubscriptionProgressMini, {
      global: {
        stubs: {
          RouterLink: true
        }
      }
    })
    await flushPromises()

    expect(wrapper.find('button').exists()).toBe(true)
    await wrapper.find('button').trigger('click')
    expect(wrapper.text()).toContain('全平台月卡')
    expect(wrapper.text()).toContain('$120.00/$600.00')
  })
})
