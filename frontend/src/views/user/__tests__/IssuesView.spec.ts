import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import IssuesView from '../IssuesView.vue'
import type { PublicSupportIssue } from '@/types'

const { list, trending, mine, route, router, authStore, issueNotificationStore } = vi.hoisted(() => ({
  list: vi.fn(),
  trending: vi.fn(),
  mine: vi.fn(),
  route: {
    path: '/issues',
    query: {} as Record<string, string>,
    params: {},
    fullPath: '/issues',
  },
  router: {
    push: vi.fn(),
    replace: vi.fn(),
  },
  authStore: {
    isAuthenticated: true,
  },
  issueNotificationStore: {
    unreadCount: 0,
    refresh: vi.fn(),
  },
}))

vi.mock('@/api/issues', () => ({
  issuesAPI: {
    list,
    trending,
    mine,
  },
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStore,
}))

vi.mock('@/stores/supportIssueNotifications', () => ({
  useSupportIssueNotificationStore: () => issueNotificationStore,
}))

vi.mock('vue-router', () => ({
  useRoute: () => route,
  useRouter: () => router,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params?.count != null ? `${key}:${params.count}` : key,
    }),
  }
})

const issue: PublicSupportIssue = {
  id: 123,
  public_id: 'ISS-000123',
  title: 'Rate limit on Claude',
  description: 'Requests fail with 429.',
  account_email_masked: 'u***@example.com',
  occurred_at: '2026-05-13T08:00:00Z',
  screenshot_text: 'rate limit',
  screenshot_language: 'en',
  category: 'api_call',
  severity: 'blocked',
  status: 'open',
  model_name: 'claude-sonnet',
  client_name: 'claude-code',
  http_status: 429,
  error_code: 'rate_limit',
  comment_count: 2,
  attachment_count: 1,
  view_count: 4,
  created_at: '2026-05-13T08:00:00Z',
  updated_at: '2026-05-13T08:00:00Z',
  last_comment_at: '2026-05-13T09:00:00Z',
}

function mountView() {
  return mount(IssuesView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        LoadingSpinner: { template: '<div>loading</div>' },
        Pagination: true,
      },
    },
  })
}

describe('IssuesView', () => {
  beforeEach(() => {
    list.mockReset()
    trending.mockReset()
    mine.mockReset()
    issueNotificationStore.refresh.mockReset()
    issueNotificationStore.refresh.mockResolvedValue(undefined)
    issueNotificationStore.unreadCount = 0
    router.push.mockReset()
    router.replace.mockReset()
    authStore.isAuthenticated = true
    route.query = {}
    route.fullPath = '/issues'
  })

  it('uses the trending endpoint for the 24 hour ranking tab', async () => {
    route.query = { tab: 'hot24' }
    trending.mockResolvedValue({
      items: [issue],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    mountView()
    await flushPromises()

    expect(trending).toHaveBeenCalledWith({
      page: 1,
      page_size: 20,
      sort_by: 'hot_24h',
      sort_order: 'desc',
      window: '24h',
    })
  })

  it('uses the authenticated mine endpoint for my issues tab', async () => {
    route.query = { tab: 'mine' }
    route.fullPath = '/issues?tab=mine'
    mine.mockResolvedValue({
      items: [issue],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = mountView()
    await flushPromises()

    expect(mine).toHaveBeenCalledWith({
      page: 1,
      page_size: 20,
      sort_by: 'created_at',
      sort_order: 'desc',
    })
    expect(wrapper.text()).toContain('Rate limit on Claude')
  })

  it('loads issues with q/status/category/severity/has_image from the URL and displays the list', async () => {
    route.query = {
      q: 'status:open code:429',
      status: 'open',
      category: 'api_call',
      severity: 'blocked',
      has_image: 'true',
      page: '2',
      page_size: '10',
    }
    list.mockResolvedValue({
      items: [issue],
      total: 1,
      page: 2,
      page_size: 10,
      pages: 1,
    })

    const wrapper = mountView()
    await flushPromises()

    expect(list).toHaveBeenCalledWith({
      q: 'status:open code:429',
      status: 'open',
      category: 'api_call',
      severity: 'blocked',
      has_image: true,
      page: 2,
      page_size: 10,
      sort_by: 'last_comment_at',
      sort_order: 'desc',
    })
    expect(wrapper.text()).toContain('Rate limit on Claude')
    expect(wrapper.text()).toContain('ISS-000123')
  })

  it('shows the empty state when no issues are returned', async () => {
    list.mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      page_size: 20,
      pages: 0,
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="issues-empty"]').exists()).toBe(true)
  })
})
