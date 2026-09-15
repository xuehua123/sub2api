import { beforeEach, describe, expect, it, vi } from 'vitest'
import { subscriptionsAPI } from '../admin/subscriptions'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))
vi.mock('../client', () => ({ apiClient: { post } }))

describe('admin subscription batch APIs', () => {

  it('retries uncertain plan assignments with the same canonical payload and key', async () => {
    localStorage.setItem('auth_user', JSON.stringify({ id: 90101 }))
    const request = { user_ids: [12, 11, 12], plan_id: 9, group_id: 3, validity_days: 30 }
    post.mockRejectedValueOnce(new Error('network disconnected'))
    await expect(subscriptionsAPI.bulkAssign(request)).rejects.toThrow('network disconnected')
    const firstCall = post.mock.calls[0]
    expect(firstCall[1].user_ids).toEqual([11, 12])
    request.user_ids.reverse()
    post.mockRejectedValueOnce({ response: { status: 504 } })
    await expect(subscriptionsAPI.bulkAssign(request)).rejects.toBeDefined()
    expect(post.mock.calls[1]).toEqual(firstCall)
    const result = { success_count: 2, failed_count: 0, subscriptions: [], errors: [] }
    post.mockResolvedValue({ data: result })
    await subscriptionsAPI.bulkAssign(request)
    expect(post.mock.calls[2]).toEqual(firstCall)
    await subscriptionsAPI.bulkAssign(request)
    expect(post.mock.calls[3][2]).not.toEqual(firstCall[2])
  })

  it('isolates pending plan assignments by administrator and parameters', async () => {
    const request = { user_ids: [11], plan_id: 9, group_id: 3, validity_days: 30 }
    localStorage.setItem('auth_user', JSON.stringify({ id: 90102 }))
    post.mockRejectedValue(new Error('unknown outcome'))
    await expect(subscriptionsAPI.bulkAssign(request)).rejects.toThrow()
    await expect(subscriptionsAPI.bulkAssign({ ...request, plan_id: 10 })).rejects.toThrow()
    localStorage.setItem('auth_user', JSON.stringify({ id: 90103 }))
    await expect(subscriptionsAPI.bulkAssign(request)).rejects.toThrow()
    expect(new Set(post.mock.calls.map(call => call[2].headers['Idempotency-Key'])).size).toBe(3)
  })
  beforeEach(() => vi.clearAllMocks())

  it('sends the explicit operation key and returns partial results', async () => {
    const request = { subscription_ids: [3, 5], action: 'extend' as const, days: 7 }
    const result = { success_count: 1, failed_count: 1, results: [
      { subscription_id: 3, success: true }, { subscription_id: 5, success: false, error: 'Not found' }
    ] }
    post.mockResolvedValue({ data: result })
    expect(await subscriptionsAPI.bulkAction(request, 'test-key')).toEqual(result)
    expect(post).toHaveBeenCalledWith('/admin/subscriptions/bulk-action', request, {
      headers: { 'Idempotency-Key': 'test-key' }
    })
  })

  it('returns the assignment summary and per-user failures', async () => {
    const request = { user_ids: [11, 12], group_id: 3, validity_days: 30 }
    const result = { success_count: 1, failed_count: 1, subscriptions: [], errors: ['User 12: conflict'], statuses: { 11: 'created', 12: 'failed' } }
    post.mockResolvedValue({ data: result })
    expect(await subscriptionsAPI.bulkAssign(request)).toEqual(result)
    expect(post).toHaveBeenCalledWith('/admin/subscriptions/bulk-assign', request)
  })
})
