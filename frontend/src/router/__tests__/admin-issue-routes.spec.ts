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

describe('admin-issue routes', () => {
  it('registers admin issue list and detail routes as admin-only', async () => {
    const { default: router } = await import('@/router')
    const routes = router.getRoutes()
    const list = routes.find((record) => record.name === 'AdminSupportIssues')
    const detail = routes.find((record) => record.name === 'AdminSupportIssueDetail')

    expect(list?.path).toBe('/admin/issues')
    expect(list?.meta.requiresAuth).toBe(true)
    expect(list?.meta.requiresAdmin).toBe(true)
    expect(detail?.path).toBe('/admin/issues/:id')
    expect(detail?.meta.requiresAuth).toBe(true)
    expect(detail?.meta.requiresAdmin).toBe(true)
  })
})
