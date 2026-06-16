import { describe, expect, it } from 'vitest'
import type { UserSubscription } from '@/types'
import { subscriptionDisplayName } from '../subscriptionPlanDisplay'

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
