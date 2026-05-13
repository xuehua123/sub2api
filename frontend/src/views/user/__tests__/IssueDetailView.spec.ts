import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import IssueDetailView from '../IssueDetailView.vue'
import type { PublicSupportIssue } from '@/types'

const { get, addComment, resolve, route, router, authStore } = vi.hoisted(() => ({
  get: vi.fn(),
  addComment: vi.fn(),
  resolve: vi.fn(),
  route: {
    params: { id: '123' },
    fullPath: '/issues/123',
  },
  router: {
    push: vi.fn(),
  },
  authStore: {
    isAuthenticated: true,
    isAdmin: false,
  },
}))

vi.mock('@/api/issues', () => ({
  issuesAPI: {
    get,
    addComment,
    resolve,
    attachmentFileURL: (id: number) => `/api/v1/issues/attachments/${id}/file`,
  },
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStore,
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
      t: (key: string) => key,
    }),
  }
})

function makeIssue(overrides: Partial<PublicSupportIssue> = {}): PublicSupportIssue {
  return {
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
    comment_count: 1,
    attachment_count: 1,
    created_at: '2026-05-13T08:00:00Z',
    updated_at: '2026-05-13T08:00:00Z',
    comments: [
      {
        id: 1,
        issue_id: 123,
        author_role: 'user',
        content: 'same here',
        created_at: '2026-05-13T08:10:00Z',
        updated_at: '2026-05-13T08:10:00Z',
      },
    ],
    attachments: [
      {
        id: 5,
        issue_id: 123,
        file_url: '/api/v1/issues/attachments/5/file',
        file_name: 'screen.png',
        mime_type: 'image/png',
        size_bytes: 10,
        visibility: 'public',
        created_at: '2026-05-13T08:00:00Z',
      },
    ],
    ...overrides,
  }
}

function mountView() {
  return mount(IssueDetailView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        LoadingSpinner: { template: '<div>loading</div>' },
      },
    },
  })
}

describe('IssueDetailView', () => {
  beforeEach(() => {
    get.mockReset()
    addComment.mockReset()
    resolve.mockReset()
    router.push.mockReset()
    authStore.isAuthenticated = true
    authStore.isAdmin = false
  })

  it.each([
    makeIssue({ status: 'resolved' }),
    makeIssue({ status: 'closed' }),
    makeIssue({ locked_at: '2026-05-13T09:00:00Z' }),
  ])('disables comments when the issue is resolved, closed, or locked', async (lockedIssue) => {
    get.mockResolvedValue(lockedIssue)

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="locked-comment-hint"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="comment-input"]').exists()).toBe(false)
  })

  it('submits comments and marks an open issue resolved', async () => {
    get.mockResolvedValue(makeIssue())
    addComment.mockResolvedValue({
      id: 2,
      issue_id: 123,
      author_role: 'user',
      content: 'new comment',
      created_at: '2026-05-13T09:00:00Z',
      updated_at: '2026-05-13T09:00:00Z',
    })
    resolve.mockResolvedValue(makeIssue({ status: 'resolved' }))

    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('[data-testid="comment-input"]').setValue('new comment')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(addComment).toHaveBeenCalledWith(123, { content: 'new comment' })

    await wrapper.find('[data-testid="resolve-issue-button"]').trigger('click')
    await flushPromises()

    expect(resolve).toHaveBeenCalledWith(123)
  })

  it('shows an admin management shortcut on the public detail page for admins', async () => {
    authStore.isAdmin = true
    get.mockResolvedValue(makeIssue())

    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('[data-testid="admin-manage-issue-button"]').trigger('click')

    expect(router.push).toHaveBeenCalledWith('/admin/issues/123')
  })
})
