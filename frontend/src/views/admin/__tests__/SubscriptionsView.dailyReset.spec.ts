import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import type { UserSubscription } from '@/types'
import SubscriptionsView from '../SubscriptionsView.vue'

const { getPlans, listSubscriptions } = vi.hoisted(() => ({
  getPlans: vi.fn(),
  listSubscriptions: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    payment: {
      getPlans,
    },
    subscriptions: {
      list: listSubscriptions,
    },
    usage: {
      searchUsers: vi.fn(),
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (key === 'admin.subscriptions.resetInHoursMinutes') {
          return `${params?.hours}h ${params?.minutes}m`
        }
        return key
      },
    }),
  }
})

const DataTableStub = {
  props: ['columns', 'data'],
  template: `
    <div>
      <div v-for="row in data" :key="row.id" data-test="usage-cell">
        <slot name="cell-usage" :row="row" />
      </div>
    </div>
  `,
}

const subscriptionFixture = (overrides: Partial<UserSubscription> = {}): UserSubscription => ({
  id: 1,
  user_id: 7,
  group_id: 3,
  status: 'active',
  starts_at: '2026-08-01T06:00:00Z',
  expires_at: '2026-09-01T06:00:00Z',
  daily_usage_usd: 1,
  weekly_usage_usd: 0,
  monthly_usage_usd: 0,
  daily_limit_usd: 10,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  daily_window_start: '2026-08-08T06:00:00Z',
  weekly_window_start: null,
  monthly_window_start: null,
  daily_resets_at: '2026-08-08T18:30:00Z',
  created_at: '2026-08-01T06:00:00Z',
  updated_at: '2026-08-08T06:00:00Z',
  ...overrides,
})

describe('admin SubscriptionsView daily reset', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-08T12:00:00Z'))
    localStorage.clear()
    getPlans.mockReset().mockResolvedValue({ data: [] })
    listSubscriptions.mockReset().mockResolvedValue({
      items: [subscriptionFixture()],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('uses the backend daily_resets_at timestamp instead of a fixed 24-hour projection', async () => {
    const wrapper = mount(SubscriptionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>',
          },
          DataTable: DataTableStub,
          Pagination: true,
          BaseDialog: true,
          ConfirmDialog: true,
          EmptyState: true,
          Select: true,
          Icon: true,
          RouterLink: true,
          Teleport: true,
        },
      },
    })

    await flushPromises()

    expect(wrapper.get('[data-test="usage-cell"]').text()).toContain('6h 30m')
  })
})
