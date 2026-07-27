import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put, remove } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  remove: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put, delete: remove }
}))

import {
  bindAccount,
  getAccountBinding,
  getRuntimeOverview,
  getTodayUsage,
  listAll,
  remove as removeConnection,
  unbindAccount
} from '@/api/admin/upstreamConnections'

describe('admin upstream connections API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
    remove.mockReset()
  })

  it('uses the dedicated account binding routes', async () => {
    const binding = { id: 4, account_id: 12, connection_id: 7 }
    get.mockResolvedValueOnce({ data: binding })
    put.mockResolvedValueOnce({ data: binding })
    remove.mockResolvedValueOnce({})

    await expect(getAccountBinding(12)).resolves.toEqual(binding)
    await expect(bindAccount(7, 12)).resolves.toEqual(binding)
    await expect(unbindAccount(7, 12)).resolves.toBeUndefined()

    expect(get).toHaveBeenCalledWith('/admin/upstream-connections/bindings/by-account/12')
    expect(put).toHaveBeenCalledWith('/admin/upstream-connections/7/bindings/12')
    expect(remove).toHaveBeenCalledWith('/admin/upstream-connections/7/bindings/12')
  })

  it('sends the confirmed bound-account snapshot before deleting bound accounts', async () => {
    remove.mockResolvedValue({})

    await expect(removeConnection(7)).resolves.toBeUndefined()
    await expect(removeConnection(8, {
      unbindAccounts: true,
      expectedBoundAccountIds: [12, 24]
    })).resolves.toBeUndefined()

    expect(remove).toHaveBeenNthCalledWith(1, '/admin/upstream-connections/7')
    expect(remove).toHaveBeenNthCalledWith(2, '/admin/upstream-connections/8', {
      data: {
        unbind_accounts: true,
        expected_bound_account_ids: [12, 24]
      }
    })
  })

  it('loads every page for account binding selectors', async () => {
    const firstPage = Array.from({ length: 200 }, (_, index) => ({ id: index + 1 }))
    get
      .mockResolvedValueOnce({ data: { items: firstPage, total: 201, page: 1, page_size: 200 } })
      .mockResolvedValueOnce({ data: { items: [{ id: 201 }], total: 201, page: 2, page_size: 200 } })

    await expect(listAll()).resolves.toHaveLength(201)
    expect(get).toHaveBeenNthCalledWith(1, '/admin/upstream-connections', {
      params: { page: 1, page_size: 200 }
    })
    expect(get).toHaveBeenNthCalledWith(2, '/admin/upstream-connections', {
      params: { page: 2, page_size: 200 }
    })
  })

  it('uses the snake-case binding-details query parameter only', async () => {
    get.mockResolvedValueOnce({ data: { items: [], total: 0, page: 1, page_size: 200 } })

    await expect(listAll({ includeBindings: true })).resolves.toEqual([])

    expect(get).toHaveBeenCalledWith('/admin/upstream-connections', {
      params: { page: 1, page_size: 200, include_bindings: true }
    })
  })

  it('loads one connection today-usage snapshot', async () => {
    const usage = {
      connection_id: 9,
      summary: { requests: 2, tokens: 20, account_cost: 1.25, standard_cost: 1, user_cost: 1.5 },
      trend: [],
      accounts: []
    }
    get.mockResolvedValueOnce({ data: usage })

    await expect(getTodayUsage(9)).resolves.toEqual(usage)
    expect(get).toHaveBeenCalledWith('/admin/upstream-connections/9/usage/today')
  })

  it('loads the batched runtime overview for bound accounts', async () => {
    const overview = { accounts: [{ account_id: 8, account_name: 'Primary', groups: [] }] }
    post.mockResolvedValueOnce({ data: overview })

    await expect(getRuntimeOverview([8, 12])).resolves.toEqual(overview)
    expect(post).toHaveBeenCalledWith('/admin/upstream-connections/runtime-overview', { account_ids: [8, 12] })
  })

  it('splits a large runtime overview into bounded requests', async () => {
    const accountIds = Array.from({ length: 5001 }, (_, index) => index + 1)
    post
      .mockResolvedValueOnce({ data: { accounts: [{ account_id: 1, account_name: 'First', groups: [] }] } })
      .mockResolvedValueOnce({ data: { accounts: [{ account_id: 5001, account_name: 'Last', groups: [] }] } })

    await expect(getRuntimeOverview(accountIds)).resolves.toEqual({
      accounts: [{ account_id: 1, account_name: 'First', groups: [] }, { account_id: 5001, account_name: 'Last', groups: [] }]
    })
    expect(post).toHaveBeenCalledTimes(2)
    expect(post.mock.calls[0]?.[1].account_ids).toHaveLength(5000)
    expect(post.mock.calls[1]?.[1]).toEqual({ account_ids: [5001] })
  })
})
