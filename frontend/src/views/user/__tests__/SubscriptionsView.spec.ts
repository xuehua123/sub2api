import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import SubscriptionsView from '../SubscriptionsView.vue'

const routerPush = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())
const showSuccess = vi.hoisted(() => vi.fn())
const getMySubscriptions = vi.hoisted(() => vi.fn())
const getGroupPreferences = vi.hoisted(() => vi.fn())
const getEntitlements = vi.hoisted(() => vi.fn())
const saveGroupPreferences = vi.hoisted(() => vi.fn())
const advanceMonthlyCycle = vi.hoisted(() => vi.fn())

const messages: Record<string, string> = {
  'userSubscriptions.entitlements.title': 'Plan Entitlements',
  'userSubscriptions.entitlements.sharedQuota': 'Shared quota',
  'userSubscriptions.entitlements.authorizedGroups': 'Authorized groups',
  'userSubscriptions.entitlements.validity': '{start} - {end}',
  'userSubscriptions.entitlements.legacyCompatible': 'Legacy subscription #{id}',
  'userSubscriptions.entitlements.id': 'Entitlement #{id}',
  'userSubscriptions.entitlements.dailyQuota': 'Daily quota',
  'userSubscriptions.entitlements.weeklyQuota': 'Weekly quota',
  'userSubscriptions.entitlements.monthlyQuota': 'Monthly quota',
  'userSubscriptions.entitlements.nextReset': 'Next reset',
  'userSubscriptions.entitlements.resetNow': 'Available now',
  'userSubscriptions.entitlements.unlimitedQuota': 'Unlimited',
  'userSubscriptions.entitlements.overage.block': 'Block at quota',
  'userSubscriptions.entitlements.overage.balanceFallback': 'Entitlement Overage Balance Fallback',
  'userSubscriptions.entitlements.overagePolicy': 'Overage policy',
  'userSubscriptions.entitlements.status.active': 'Active',
  'userSubscriptions.entitlements.status.future': 'Scheduled',
  'userSubscriptions.entitlements.status.expired': 'Expired',
  'userSubscriptions.entitlements.status.revoked': 'Revoked',
  'userSubscriptions.windowNotActive': 'Awaiting first use',
  'userSubscriptions.noActiveSubscriptions': 'No Active Subscriptions',
  'userSubscriptions.noActiveSubscriptionsDesc': 'No active subscriptions',
  'userSubscriptions.expires': 'Expires',
  'userSubscriptions.daily': 'Daily',
  'userSubscriptions.resetIn': 'Resets in {time}',
  'userSubscriptions.status.active': 'Active',
  'payment.renewNow': 'Renew now',
}

function t(key: string, params?: Record<string, unknown>) {
  let value = messages[key] || key
  for (const [param, raw] of Object.entries(params || {})) {
    value = value.replaceAll(`{${param}}`, String(raw))
  }
  return value
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t,
      locale: { value: 'en' },
    }),
  }
})

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRouter: () => ({ push: routerPush }),
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
  }),
}))

vi.mock('@/api/subscriptions', () => ({
  default: {
    getMySubscriptions,
    getGroupPreferences,
    getEntitlements,
    saveGroupPreferences,
    advanceMonthlyCycle,
  },
}))

const legacySubscription = {
  id: 1,
  user_id: 7,
  group_id: 101,
  status: 'active',
  starts_at: '2026-01-01T00:00:00Z',
  expires_at: '2027-01-01T00:00:00Z',
  daily_usage_usd: 1,
  weekly_usage_usd: 2,
  monthly_usage_usd: 3,
  daily_window_start: '2026-06-01T00:00:00Z',
  weekly_window_start: null,
  monthly_window_start: null,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  group: {
    id: 101,
    name: 'Legacy Group',
    description: '',
    platform: 'openai',
    rate_multiplier: 1,
    is_exclusive: false,
    status: 'active',
    subscription_type: 'subscription',
    daily_limit_usd: 10,
    weekly_limit_usd: null,
    monthly_limit_usd: null,
    allow_image_generation: false,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    image_price_1k: null,
    image_price_2k: null,
    image_price_4k: null,
    claude_code_only: false,
    fallback_group_id: null,
    fallback_group_id_on_invalid_request: null,
    require_oauth_only: false,
    require_privacy_set: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  },
}

const entitlement = {
  id: 22,
  plan_id: 5,
  plan_name: 'Shared Pro',
  name: 'Shared Pro',
  status: 'active',
  starts_at: '2026-01-01T00:00:00Z',
  expires_at: '2027-01-01T00:00:00Z',
  groups: [
    { id: 201, name: 'OpenAI Pro', platform: 'openai' },
    { id: 202, name: 'Gemini Pro', platform: 'gemini' },
  ],
  daily_limit_usd: 10,
  daily_usage_usd: 2.5,
  daily_window_start: '2026-06-01T00:00:00Z',
  daily_resets_at: '2026-06-02T00:00:00Z',
  daily_resets_in_seconds: 3600,
  weekly_limit_usd: null,
  weekly_usage_usd: 7,
  weekly_window_start: null,
  weekly_resets_at: null,
  weekly_resets_in_seconds: null,
  monthly_limit_usd: 100,
  monthly_usage_usd: 25,
  monthly_window_start: '2026-06-01T00:00:00Z',
  monthly_resets_at: '2026-07-01T00:00:00Z',
  monthly_resets_in_seconds: 7200,
  overage_policy: 'balance_fallback',
  legacy_subscription_id: 88,
}

async function mountView() {
  const wrapper = mount(SubscriptionsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Icon: true,
      },
    },
  })
  await flushPromises()
  await flushPromises()
  return wrapper
}

describe('SubscriptionsView entitlement v2 section', () => {
  beforeEach(() => {
    routerPush.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    getMySubscriptions.mockReset().mockResolvedValue([])
    getGroupPreferences.mockReset().mockResolvedValue([])
    getEntitlements.mockReset().mockResolvedValue([])
    saveGroupPreferences.mockReset().mockResolvedValue([])
    advanceMonthlyCycle.mockReset()
  })

  it('keeps legacy subscription display when no entitlements are returned', async () => {
    getMySubscriptions.mockResolvedValue([legacySubscription])

    const wrapper = await mountView()

    expect(wrapper.text()).toContain('Legacy Group')
    expect(wrapper.find('[data-testid="entitlement-section"]').exists()).toBe(false)
  })

  it('renders one entitlement card with multiple authorized groups and shared quota', async () => {
    getEntitlements.mockResolvedValue([entitlement])

    const wrapper = await mountView()
    const cards = wrapper.findAll('[data-testid="entitlement-card"]')

    expect(cards).toHaveLength(1)
    expect(wrapper.text()).toContain('Plan Entitlements')
    expect(wrapper.text()).toContain('Shared Pro')
    expect(wrapper.text()).toContain('OpenAI Pro')
    expect(wrapper.text()).toContain('Gemini Pro')
    expect(wrapper.text()).toContain('$2.50 / $10.00')
    expect(wrapper.text()).toContain('$7.00 / Unlimited')
  })

  it('shows entitlement balance fallback policy clearly', async () => {
    getEntitlements.mockResolvedValue([entitlement])

    const wrapper = await mountView()

    expect(wrapper.text()).toContain('Entitlement Overage Balance Fallback')
    expect(wrapper.text()).toContain('Legacy subscription #88')
  })

  it('keeps legacy subscriptions visible when entitlement loading fails', async () => {
    getMySubscriptions.mockResolvedValue([legacySubscription])
    getEntitlements.mockRejectedValue(new Error('entitlements unavailable'))

    const wrapper = await mountView()

    expect(wrapper.text()).toContain('Legacy Group')
    expect(showError).not.toHaveBeenCalled()
  })

  it('does not block legacy subscriptions while entitlements are still loading', async () => {
    getMySubscriptions.mockResolvedValue([legacySubscription])
    getEntitlements.mockImplementation(() => new Promise(() => {}))

    const wrapper = await mountView()

    expect(wrapper.text()).toContain('Legacy Group')
    expect(wrapper.find('[data-testid="entitlement-section"]').exists()).toBe(false)
  })
})
