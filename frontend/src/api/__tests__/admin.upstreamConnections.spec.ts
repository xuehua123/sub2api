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
  listAll,
  migrateLegacy,
  previewLegacyMigration,
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

  it('keeps legacy preview and apply as separate explicit operations', async () => {
    const preview = { dry_run: true, summary: { planned_accounts: 2 }, items: [], warnings: [] }
    const applied = { ...preview, dry_run: false, summary: { planned_accounts: 0, migrated_accounts: 2 } }
    post.mockResolvedValueOnce({ data: preview }).mockResolvedValueOnce({ data: applied })

    await expect(previewLegacyMigration()).resolves.toEqual(preview)
    await expect(migrateLegacy()).resolves.toEqual(applied)

    expect(post).toHaveBeenNthCalledWith(1, '/admin/upstream-connections/migrate-legacy/preview')
    expect(post).toHaveBeenNthCalledWith(2, '/admin/upstream-connections/migrate-legacy')
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
})
