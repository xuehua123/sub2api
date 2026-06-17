import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, del } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  del: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    get,
    post,
    delete: del,
  },
}))

import subscriptionsAPI from '@/api/admin/subscriptions'

describe('admin subscriptions api', () => {
  beforeEach(() => {
    get.mockReset()
    get.mockResolvedValue({ data: {} })
    post.mockReset()
    post.mockResolvedValue({ data: {} })
    del.mockReset()
    del.mockResolvedValue({ data: {} })
  })

  it('previews monthly cycle adjustments by subscription id', async () => {
    const request = {
      mode: 'advance_next_cycle' as const,
    }

    await subscriptionsAPI.previewMonthlyCycleAdjustment(912, request)

    expect(post).toHaveBeenCalledWith(
      '/admin/subscriptions/912/monthly-cycle-adjustments/preview',
      request
    )
  })

  it('applies monthly cycle adjustments by subscription id', async () => {
    const request = {
      mode: 'align_to_expiry' as const,
      cycle_count: 3,
      reason: 'support aligned cycle',
    }

    await subscriptionsAPI.applyMonthlyCycleAdjustment(912, request)

    expect(post).toHaveBeenCalledWith(
      '/admin/subscriptions/912/monthly-cycle-adjustments',
      request
    )
  })
})
