import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it, vi } from 'vitest'

const authStore = vi.hoisted(() => ({
  checkAuth: vi.fn(),
  isAuthenticated: false,
  isAdmin: false,
  isSimpleMode: false,
  hasPendingAuthSession: false,
}))

const appStore = vi.hoisted(() => ({
  siteName: 'Sub2API',
  backendModeEnabled: false,
  cachedPublicSettings: null as null | Record<string, unknown>,
}))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => authStore }))
vi.mock('@/stores/app', () => ({ useAppStore: () => appStore }))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => ({ customMenuItems: [] }) }))
vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({ startNavigation: vi.fn(), endNavigation: vi.fn(), isLoading: { value: false } }),
}))
vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({ triggerPrefetch: vi.fn(), cancelPendingPrefetch: vi.fn(), resetPrefetchState: vi.fn() }),
}))

describe('issue center routes', () => {
  it('registers public list/detail routes and authenticated new issue route', async () => {
    const { default: router } = await import('@/router')
    const routes = router.getRoutes()
    const list = routes.find((record) => record.name === 'Issues')
    const detail = routes.find((record) => record.name === 'IssueDetail')
    const create = routes.find((record) => record.name === 'NewIssue')

    expect(list?.path).toBe('/issues')
    expect(list?.meta.requiresAuth).toBe(false)
    expect(detail?.path).toBe('/issues/:id')
    expect(detail?.meta.requiresAuth).toBe(false)
    expect(create?.path).toBe('/issues/new')
    expect(create?.meta.requiresAuth).toBe(true)
  })

  it('keeps /issues/new before /issues/:id and limits backend-mode public issue paths', () => {
    const routerPath = resolve(dirname(fileURLToPath(import.meta.url)), '../index.ts')
    const source = readFileSync(routerPath, 'utf8')

    expect(source.indexOf("path: '/issues/new'")).toBeLessThan(source.indexOf("path: '/issues/:id'"))
    expect(source).toContain("path === '/issues' || /^\\/issues\\/\\d+$/.test(path)")
  })
})
