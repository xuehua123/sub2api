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
  getTodayUsage,
  listAll,
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
})
