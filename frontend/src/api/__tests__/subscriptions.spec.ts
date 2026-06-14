import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    get,
    post,
  },
}))

import subscriptionsAPI from '@/api/subscriptions'

describe('subscriptions api', () => {
  beforeEach(() => {
    get.mockReset()
    get.mockResolvedValue({ data: [] })
    post.mockReset()
    post.mockResolvedValue({ data: {} })
  })

  it('requests user entitlement v2 list', async () => {
    await subscriptionsAPI.getEntitlements()

    expect(get).toHaveBeenCalledWith('/entitlements')
  })

  it('requests active user entitlement v2 list', async () => {
    await subscriptionsAPI.getActiveEntitlements()

    expect(get).toHaveBeenCalledWith('/entitlements/active')
  })

  it('requests user entitlement progress by id', async () => {
    get.mockResolvedValue({ data: { id: 12 } })

    await subscriptionsAPI.getEntitlementProgress(12)

    expect(get).toHaveBeenCalledWith('/entitlements/12/progress')
  })

  it('requests entitlement monthly cycle advance by id', async () => {
    await subscriptionsAPI.advanceEntitlementMonthlyCycle(22)

    expect(post).toHaveBeenCalledWith('/entitlements/22/advance-monthly-cycle')
  })
})
