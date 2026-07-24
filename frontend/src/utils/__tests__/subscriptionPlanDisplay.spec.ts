import { describe, expect, it } from 'vitest'
import type { UserEntitlement, UserSubscription } from '@/types'
import { activeSubscriptionDisplayRecords, subscriptionDisplayName } from '../subscriptionPlanDisplay'

function subscription(overrides: Partial<UserSubscription> = {}): UserSubscription {
  return {
    id: 10,
    user_id: 20,
    group_id: 33,
    status: 'active',
    starts_at: '2026-06-01T00:00:00Z',
    expires_at: '2026-07-01T00:00:00Z',
    daily_usage_usd: 0,
    weekly_usage_usd: 0,
    monthly_usage_usd: 0,
    daily_window_start: null,
    weekly_window_start: null,
    monthly_window_start: null,
    created_at: '2026-06-01T00:00:00Z',
    updated_at: '2026-06-01T00:00:00Z',
    group: {
      id: 33,
      name: 'plus pro group',
      description: '',
      platform: 'openai',
      rate_multiplier: 1,
      is_exclusive: false,
      status: 'active',
      subscription_type: 'subscription',
      daily_limit_usd: null,
      weekly_limit_usd: null,
      monthly_limit_usd: null,
      allow_image_generation: false,
      image_rate_independent: false,
      image_rate_multiplier: 1,
      image_price_1k: null,
      image_price_2k: null,
      image_price_4k: null,
      claude_code_only: false,
      fallback_group_id: null,
      fallback_group_id_on_invalid_request: null,
      require_oauth_only: false,
      require_privacy_set: false,
      created_at: '2026-06-01T00:00:00Z',
      updated_at: '2026-06-01T00:00:00Z',
    },
    ...overrides,
  }
}

function entitlement(overrides: Partial<UserEntitlement> = {}): UserEntitlement {
  return {
    id: 44,
    plan_id: 9,
    plan_name: '多分组月卡',
    name: '多分组月卡',
    status: 'active',
    starts_at: '2026-06-01T00:00:00Z',
    expires_at: '2099-07-01T00:00:00Z',
    groups: [
      { id: 33, name: 'OpenAI', platform: 'openai', rate_multiplier: 1 },
      { id: 34, name: 'Claude', platform: 'anthropic', rate_multiplier: 1.2 },
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
    ...overrides,
  }
}

describe('subscriptionDisplayName', () => {
  it('prefers subscription plan_name over group fallback', () => {
    const sub = subscription({ plan_id: 9, plan_name: '尊享月卡' })

    expect(subscriptionDisplayName(sub, [])).toBe('尊享月卡')
  })

  it('falls back to the plan list before group name', () => {
    const sub = subscription({ plan_id: 9 })

    expect(subscriptionDisplayName(sub, [{
      id: 9,
      group_id: 33,
      group_ids: [33],
      groups: [],
      name: 'Plan fallback',
      product_name: '畅享月卡',
      price: 168,
      original_price: null,
      validity_days: 30,
      validity_unit: 'day',
      quota_usd: 2400,
      quota_period: 'monthly',
      daily_limit_usd: null,
      weekly_limit_usd: null,
      monthly_limit_usd: 2400,
      unit_cost_per_usd: null,
      overage_policy: 'block',
      access_scope: 'explicit',
      allowed_platforms: [],
      features: [],
      for_sale: true,
      status: 'active',
      created_at: '2026-06-01T00:00:00Z',
      updated_at: '2026-06-01T00:00:00Z',
    }])).toBe('畅享月卡')
  })
})

describe('activeSubscriptionDisplayRecords', () => {
  it('shows a pure V2 multi-group entitlement as one display record', () => {
    const records = activeSubscriptionDisplayRecords([], [entitlement()])

    expect(records).toHaveLength(1)
    expect(records[0]).toMatchObject({
      id: -44,
      group_id: 33,
      entitlement_only: true,
      entitlement_id: 44,
      plan_id: 9,
      plan_name: '多分组月卡',
      monthly_limit_usd: 600,
      monthly_usage_usd: 120,
    })
    expect(records[0].groups?.map((group) => group.id)).toEqual([33, 34])
  })

  it('does not duplicate an entitlement already represented by a legacy alias', () => {
    const records = activeSubscriptionDisplayRecords(
      [subscription({ id: 11, entitlement_id: 44, plan_id: 9 })],
      [entitlement({ legacy_subscription_id: 11 })],
    )

    expect(records).toHaveLength(1)
    expect(records[0].id).toBe(11)
  })

  it('does not display an entitlement before its start time', () => {
    const records = activeSubscriptionDisplayRecords(
      [],
      [entitlement({ starts_at: '2026-08-01T00:00:00Z' })],
      new Date('2026-07-24T00:00:00Z'),
    )

    expect(records).toHaveLength(0)
  })
})
