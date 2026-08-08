import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { post }
}))

const payload = {
  start_date: '2026-08-01',
  end_date: '2026-08-02',
  entitlement_id: 42,
  timezone: 'Asia/Shanghai'
}

describe('admin usage cleanup API', () => {
  beforeEach(() => {
    vi.resetModules()
    localStorage.clear()
    sessionStorage.clear()
    localStorage.setItem('auth_user', JSON.stringify({ id: 7 }))
    post.mockReset().mockResolvedValue({ data: { id: 9, status: 'pending' } })
    vi.spyOn(globalThis.crypto, 'randomUUID').mockReturnValue('11111111-1111-4111-8111-111111111111')
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('always sends an idempotency key and clears it after success', async () => {
    const { createCleanupTask } = await import('@/api/admin/usage')
    await createCleanupTask(payload)

    expect(post).toHaveBeenCalledWith('/admin/usage/cleanup-tasks', payload, {
      headers: {
        'Idempotency-Key': 'usage-cleanup-7-11111111-1111-4111-8111-111111111111'
      }
    })
    expect(sessionStorage.length).toBe(0)
  })

  it('reuses the same key after an ambiguous failure and page reload', async () => {
    const { createCleanupTask } = await import('@/api/admin/usage')
    post.mockRejectedValueOnce(new Error('network timeout'))
    await expect(createCleanupTask(payload)).rejects.toThrow('network timeout')
    const firstHeaders = post.mock.calls[0][2].headers

    vi.resetModules()
    post.mockResolvedValueOnce({ data: { id: 9, status: 'pending' } })
    const { createCleanupTask: createAfterReload } = await import('@/api/admin/usage')
    await createAfterReload({ ...payload })

    expect(post.mock.calls[1][2].headers).toEqual(firstHeaders)
    expect(sessionStorage.length).toBe(0)
  })

  it('does not reuse a pending key when the cleanup scope changes', async () => {
    const { createCleanupTask } = await import('@/api/admin/usage')
    post.mockRejectedValueOnce(new Error('network timeout'))
    await expect(createCleanupTask(payload)).rejects.toThrow('network timeout')
    const firstHeaders = post.mock.calls[0][2].headers
    vi.mocked(globalThis.crypto.randomUUID).mockReturnValueOnce('22222222-2222-4222-8222-222222222222')
    post.mockResolvedValueOnce({ data: { id: 10, status: 'pending' } })

    await createCleanupTask({ ...payload, entitlement_id: 43 })

    expect(post.mock.calls[1][2].headers).not.toEqual(firstHeaders)
  })
})
