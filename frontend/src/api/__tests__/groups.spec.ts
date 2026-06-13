import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({
  get: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    get,
  },
}))

import userGroupsAPI from '@/api/groups'

describe('user groups api', () => {
  beforeEach(() => {
    get.mockReset()
  })

  it('preserves entitlement metadata from available groups', async () => {
    const groups = [
      {
        id: 20,
        name: 'Claude Pro',
        description: null,
        platform: 'anthropic',
        rate_multiplier: 1,
        is_exclusive: false,
        status: 'active',
        subscription_type: 'subscription',
        balance_enabled: false,
        subscription_enabled: true,
        plan_auto_grant_enabled: true,
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
        entitlements: [
          {
            id: 101,
            name: 'Team Pro',
            plan_id: 7,
            primary_group_id: 20,
            starts_at: '2026-06-01T00:00:00Z',
            expires_at: '2026-07-01T00:00:00Z',
          },
        ],
      },
    ]
    get.mockResolvedValue({ data: groups })

    const result = await userGroupsAPI.getAvailable()

    expect(get).toHaveBeenCalledWith('/groups/available')
    expect(result[0].entitlements).toEqual(groups[0].entitlements)
    expect(result[0]).toMatchObject({
      balance_enabled: false,
      subscription_enabled: true,
      plan_auto_grant_enabled: true,
    })
  })
})
