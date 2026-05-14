import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SupportIssuesView from '../SupportIssuesView.vue'
import type { AdminSupportIssue } from '@/types'

const { list, route, router } = vi.hoisted(() => ({
  list: vi.fn(),
  route: {
    path: '/admin/issues',
    query: {} as Record<string, string>,
    params: {},
    fullPath: '/admin/issues',
  },
  router: {
    push: vi.fn(),
    replace: vi.fn(),
  },
}))

vi.mock('@/api/admin/issues', () => ({
  adminIssuesAPI: {
    list,
  },
  default: {
    list,
  },
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

const issue: AdminSupportIssue = {
  id: 123,
  public_id: 'ISS-000123',
  title: 'Rate limit on Claude',
  description: 'Requests fail with 429.',
  account_email: 'user@example.com',
  account_email_normalized: 'user@example.com',
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
  api_key_suffix: 'ab12cd',
  created_by_user_id: 10,
  resolved_by_user_id: null,
  hidden_comment_count: 3,
  comment_count: 2,
  attachment_count: 1,
  created_at: '2026-05-13T08:00:00Z',
  updated_at: '2026-05-13T08:00:00Z',
  last_comment_at: '2026-05-13T09:00:00Z',
}

function mountView() {
  return mount(SupportIssuesView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        LoadingSpinner: { template: '<div>loading</div>' },
        Pagination: true,
      },
    },
  })
}

describe('admin SupportIssuesView', () => {
  beforeEach(() => {
    list.mockReset()
    router.push.mockReset()
    router.replace.mockReset()
    route.query = {}
    route.fullPath = '/admin/issues'
  })

  it('calls adminIssuesAPI.list with filters and displays diagnostic fields', async () => {
    route.query = {
      q: 'key:ab12cd email:user@example.com',
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
      q: 'key:ab12cd email:user@example.com',
      status: 'open',
      category: 'api_call',
      severity: 'blocked',
      has_image: true,
      page: 2,
      page_size: 10,
      sort_by: 'last_comment_at',
      sort_order: 'desc',
    })
    expect(wrapper.text()).toContain('user@example.com')
    expect(wrapper.text()).toContain('ab12cd')
    expect(wrapper.text()).toContain('3')
    expect(wrapper.text()).toContain('key:')
    expect(wrapper.text()).toContain('email:')
  })

  it('defaults to pending issues so sidebar badge points to visible work', async () => {
    list.mockResolvedValue({
      items: [issue],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    mountView()
    await flushPromises()

    expect(list).toHaveBeenCalledWith(expect.objectContaining({
      status: 'pending',
      page: 1,
      page_size: 20,
    }))
  })
})
