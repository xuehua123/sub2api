import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({
  post: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    post,
  },
}))

import keysAPI from '@/api/keys'

describe('keys api', () => {
  beforeEach(() => {
    post.mockReset()
    post.mockResolvedValue({ data: { id: 1 } })
  })

  it('creates API keys with object payload including entitlement binding', async () => {
    await keysAPI.createWithPayload({
      name: 'subscription key',
      group_id: 20,
      access_source: 'entitlement',
      subscription_entitlement_id: 101,
      auto_switch_group_enabled: true,
    })

    expect(post).toHaveBeenCalledWith('/keys', {
      name: 'subscription key',
      group_id: 20,
      access_source: 'entitlement',
      subscription_entitlement_id: 101,
      auto_switch_group_enabled: true,
    })
  })

  it('creates API keys with explicit balance access source', async () => {
    await keysAPI.createWithPayload({
      name: 'balance key',
      group_id: 10,
      access_source: 'balance',
      subscription_entitlement_id: null,
      auto_switch_group_enabled: true,
    })

    expect(post).toHaveBeenCalledWith('/keys', {
      name: 'balance key',
      group_id: 10,
      access_source: 'balance',
      subscription_entitlement_id: null,
      auto_switch_group_enabled: true,
    })
  })

  it('keeps legacy positional create helper compatible', async () => {
    await keysAPI.create('legacy key', 10, undefined, [], [], 0, undefined, undefined, false)

    expect(post).toHaveBeenCalledWith('/keys', {
      name: 'legacy key',
      group_id: 10,
      auto_switch_group_enabled: false,
    })
  })
})
