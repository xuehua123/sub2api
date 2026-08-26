import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

type NavigationGuard = (
  to: Record<string, any>,
  from: Record<string, any>,
  next: ReturnType<typeof vi.fn>
) => Promise<void>

const routerHarness = vi.hoisted(() => ({
  guard: null as NavigationGuard | null,
  routes: [] as Array<{ path: string; meta?: Record<string, unknown> }>,
}))

const authStore = vi.hoisted(() => ({
  checkAuth: vi.fn(),
  refreshUser: vi.fn(),
  isAuthenticated: true,
  isAdmin: false,
  isSimpleMode: false,
  hasPendingAuthSession: false,
  user: { payment_disabled: false },
}))

const appStore = vi.hoisted(() => ({
  siteName: 'Sub2API',
  backendModeEnabled: false,
  publicSettingsLoaded: false,
  cachedPublicSettings: null as null | {
    payment_enabled?: boolean
    risk_control_enabled?: boolean
    custom_menu_items?: []
  },
  fetchPublicSettings: vi.fn(),
}))

vi.mock('vue-router', () => ({
  createWebHistory: vi.fn(() => ({})),
  createRouter: vi.fn((config: { routes: Array<{ path: string; meta?: Record<string, unknown> }> }) => {
    routerHarness.routes = config.routes
    return {
      beforeEach: vi.fn((guard: NavigationGuard) => {
        routerHarness.guard = guard
      }),
      afterEach: vi.fn(),
      onError: vi.fn(),
    }
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStore,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({ customMenuItems: [] }),
}))

vi.mock('@/stores/adminCompliance', () => ({
  useAdminComplianceStore: () => ({
    initialized: true,
    fetchStatus: vi.fn(),
    requireAcknowledgement: vi.fn(),
  }),
}))

vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({
    startNavigation: vi.fn(),
    endNavigation: vi.fn(),
    isLoading: { value: false },
  }),
}))

vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({
    triggerPrefetch: vi.fn(),
    cancelPendingPrefetch: vi.fn(),
    resetPrefetchState: vi.fn(),
  }),
}))

function createDeferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

function runGuard(meta: Record<string, unknown>, path: string) {
  if (!routerHarness.guard) {
    throw new Error('router guard was not registered')
  }

  const next = vi.fn()
  const navigation = routerHarness.guard(
    {
      path,
      fullPath: path,
      name: 'FeatureRoute',
      params: {},
      meta: { requiresAuth: true, ...meta },
    },
    {},
    next
  )
  return { navigation, next }
}

describe('feature route guard', () => {
  beforeAll(async () => {
    await import('@/router')
  })

  beforeEach(() => {
    authStore.isAuthenticated = true
    authStore.isAdmin = false
    authStore.isSimpleMode = false
    authStore.user = { payment_disabled: false }
    authStore.refreshUser.mockReset()
    authStore.refreshUser.mockImplementation(async () => authStore.user)
    appStore.publicSettingsLoaded = false
    appStore.cachedPublicSettings = null
    appStore.backendModeEnabled = false
    appStore.fetchPublicSettings.mockReset()
  })

  it.each(['/payment/stripe', '/payment/airwallex', '/payment/stripe-popup'])(
    'protects direct checkout route %s with current user payment policy',
    (path) => {
      const route = routerHarness.routes.find((candidate) => candidate.path === path)
      expect(route?.meta?.requiresAuth).toBe(true)
      expect(route?.meta?.requiresPayment).toBe(true)
    }
  )

  it('waits for the first public-settings request before deciding payment access', async () => {
    const deferred = createDeferred<{ payment_enabled: boolean }>()
    appStore.fetchPublicSettings.mockImplementation(async () => {
      const settings = await deferred.promise
      appStore.cachedPublicSettings = settings
      appStore.publicSettingsLoaded = true
      return settings
    })

    const { navigation, next } = runGuard({ requiresPayment: true }, '/purchase')

    await vi.waitFor(() => expect(appStore.fetchPublicSettings).toHaveBeenCalledTimes(1))
    expect(next).not.toHaveBeenCalled()

    deferred.resolve({ payment_enabled: true })
    await navigation
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  })

  it.each([
    ['payment', { requiresPayment: true }, '/purchase'],
    ['risk control', { requiresRiskControl: true }, '/admin/risk-control'],
  ])('does not treat a failed %s settings load as explicitly disabled', async (_name, meta, path) => {
    authStore.isAdmin = meta.requiresRiskControl === true
    appStore.fetchPublicSettings.mockResolvedValue(null)

    const { navigation, next } = runGuard(meta, path)
    await navigation

    expect(appStore.publicSettingsLoaded).toBe(false)
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  })

  it.each([
    ['payment', { requiresPayment: true }, { payment_enabled: false }, '/dashboard'],
    [
      'risk control',
      { requiresRiskControl: true },
      { risk_control_enabled: false },
      '/admin/settings',
    ],
  ])('redirects when loaded settings explicitly disable %s', async (_name, meta, settings, target) => {
    authStore.isAdmin = meta.requiresRiskControl === true
    appStore.cachedPublicSettings = settings
    appStore.publicSettingsLoaded = true

    const { navigation, next } = runGuard(meta, '/feature')
    await navigation

    expect(appStore.fetchPublicSettings).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith(target)
  })

  it('redirects a payment-disabled user before loading global payment settings', async () => {
    authStore.user = { payment_disabled: true }

    const { navigation, next } = runGuard({ requiresPayment: true }, '/purchase')
    await navigation

    expect(appStore.fetchPublicSettings).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith('/dashboard')
  })

  it('refreshes stale user policy before allowing a payment route', async () => {
    authStore.user = { payment_disabled: false }
    authStore.refreshUser.mockImplementation(async () => {
      authStore.user = { payment_disabled: true }
      return authStore.user
    })

    const { navigation, next } = runGuard({ requiresPayment: true }, '/payment/stripe')
    await navigation

    expect(authStore.refreshUser).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith('/dashboard')
    expect(appStore.fetchPublicSettings).not.toHaveBeenCalled()
  })

  it('fails closed when current payment policy cannot be refreshed', async () => {
    authStore.refreshUser.mockRejectedValue(new Error('profile unavailable'))

    const { navigation, next } = runGuard({ requiresPayment: true }, '/purchase')
    await navigation

    expect(next).toHaveBeenCalledWith('/dashboard')
    expect(appStore.fetchPublicSettings).not.toHaveBeenCalled()
  })

  it('does not block admin payment management when the admin self-payment flag is disabled', async () => {
    authStore.isAdmin = true
    authStore.user = { payment_disabled: true }
    appStore.publicSettingsLoaded = true
    appStore.cachedPublicSettings = { payment_enabled: true }

    const { navigation, next } = runGuard({ requiresPayment: true }, '/admin/orders')
    await navigation

    expect(authStore.refreshUser).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  })

  it('allows authenticated users to complete Airwallex checkout in backend mode', async () => {
    appStore.backendModeEnabled = true
    appStore.publicSettingsLoaded = true
    appStore.cachedPublicSettings = { payment_enabled: true }

    const { navigation, next } = runGuard({ requiresPayment: true }, '/payment/airwallex')
    await navigation

    expect(authStore.refreshUser).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  })
})
