import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SupportIssueDetailView from '../SupportIssueDetailView.vue'
import type { AdminSupportIssue, SupportIssueEvent } from '@/types'

const {
  get,
  events,
  updateStatus,
  reopen,
  hideComment,
  hideAttachment,
  hideIssue,
  restoreIssue,
  pin,
  unpin,
  markSolution,
  clearSolution,
  setRelatedIssue,
  clearRelatedIssue,
  addComment,
  route,
  router,
} = vi.hoisted(() => ({
  get: vi.fn(),
  events: vi.fn(),
  updateStatus: vi.fn(),
  reopen: vi.fn(),
  hideComment: vi.fn(),
  hideAttachment: vi.fn(),
  hideIssue: vi.fn(),
  restoreIssue: vi.fn(),
  pin: vi.fn(),
  unpin: vi.fn(),
  markSolution: vi.fn(),
  clearSolution: vi.fn(),
  setRelatedIssue: vi.fn(),
  clearRelatedIssue: vi.fn(),
  addComment: vi.fn(),
  route: {
    params: { id: '123' },
    fullPath: '/admin/issues/123',
  },
  router: {
    push: vi.fn(),
  },
}))

vi.mock('@/api/admin/issues', () => ({
  adminIssuesAPI: {
    get,
    events,
    updateStatus,
    reopen,
    hideComment,
    hideAttachment,
    hideIssue,
    restoreIssue,
    pin,
    unpin,
    markSolution,
    clearSolution,
    setRelatedIssue,
    clearRelatedIssue,
  },
  default: {
    get,
    events,
    updateStatus,
    reopen,
    hideComment,
    hideAttachment,
    hideIssue,
    restoreIssue,
    pin,
    unpin,
    markSolution,
    clearSolution,
    setRelatedIssue,
    clearRelatedIssue,
  },
}))

vi.mock('@/api/issues', () => ({
  issuesAPI: {
    addComment,
    attachmentFileURL: (id: number) => `/api/v1/issues/attachments/${id}/file`,
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
      t: (key: string) => key,
    }),
  }
})

const event: SupportIssueEvent = {
  id: 1,
  issue_id: 123,
  actor_user_id: 9,
  event_type: 'created',
  created_at: '2026-05-13T08:00:00Z',
}

function makeIssue(overrides: Partial<AdminSupportIssue> = {}): AdminSupportIssue {
  return {
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
    resolved_by_user_id: 11,
    hidden_comment_count: 1,
    comment_count: 1,
    attachment_count: 1,
    view_count: 5,
    created_at: '2026-05-13T08:00:00Z',
    updated_at: '2026-05-13T08:00:00Z',
    comments: [
      {
        id: 501,
        issue_id: 123,
        author_user_id: 10,
        author_role: 'user',
        content: 'same here',
        hide_reason: 'contains token',
        created_at: '2026-05-13T08:10:00Z',
        updated_at: '2026-05-13T08:10:00Z',
      },
    ],
    attachments: [
      {
        id: 77,
        issue_id: 123,
        uploaded_by_user_id: 10,
        file_path: 'E:\\private\\support\\screen.png',
        file_url: '/api/v1/issues/attachments/77/file',
        file_name: 'screen.png',
        mime_type: 'image/png',
        size_bytes: 12,
        visibility: 'public',
        created_at: '2026-05-13T08:00:00Z',
      },
    ],
    events: [event],
    ...overrides,
  }
}

function mountView() {
  return mount(SupportIssueDetailView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        LoadingSpinner: { template: '<div>loading</div>' },
        RouterLink: { template: '<a><slot /></a>' },
      },
    },
  })
}

describe('admin SupportIssueDetailView', () => {
  beforeEach(() => {
    get.mockReset()
    events.mockReset()
    updateStatus.mockReset()
    reopen.mockReset()
    hideComment.mockReset()
    hideAttachment.mockReset()
    hideIssue.mockReset()
    restoreIssue.mockReset()
    pin.mockReset()
    unpin.mockReset()
    markSolution.mockReset()
    clearSolution.mockReset()
    setRelatedIssue.mockReset()
    clearRelatedIssue.mockReset()
    addComment.mockReset()
    router.push.mockReset()
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    get.mockResolvedValue(makeIssue())
    events.mockResolvedValue([event])
    updateStatus.mockResolvedValue(makeIssue())
    reopen.mockResolvedValue(makeIssue())
    hideComment.mockResolvedValue({ message: 'ok' })
    hideAttachment.mockResolvedValue({ message: 'ok' })
    hideIssue.mockResolvedValue(makeIssue())
    restoreIssue.mockResolvedValue(makeIssue())
    pin.mockResolvedValue(makeIssue({ pinned_at: '2026-05-13T09:00:00Z' }))
    unpin.mockResolvedValue(makeIssue())
    markSolution.mockResolvedValue(makeIssue({ solution_comment_id: 501 }))
    clearSolution.mockResolvedValue(makeIssue())
    setRelatedIssue.mockResolvedValue(makeIssue({ related_issue_id: 999 }))
    clearRelatedIssue.mockResolvedValue(makeIssue())
    addComment.mockResolvedValue({
      id: 502,
      issue_id: 123,
      author_role: 'admin',
      content: 'admin note',
      created_at: '2026-05-13T09:00:00Z',
      updated_at: '2026-05-13T09:00:00Z',
    })
  })

  it('loads detail and events, shows diagnostics, and never uses file_path as image src', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(get).toHaveBeenCalledWith(123)
    expect(events).toHaveBeenCalledWith(123)
    expect(wrapper.text()).toContain('user@example.com')
    expect(wrapper.text()).toContain('ab12cd')
    expect(wrapper.text()).toContain('E:\\private\\support\\screen.png')

    const image = wrapper.get('[data-testid="admin-attachment-preview"]')
    expect(image.attributes('src')).toBe('/api/v1/issues/attachments/77/file')
    expect(image.attributes('src')).not.toContain('E:\\private')
  })

  it('calls admin mutation APIs with reason and comments through the public issue API', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="admin-status-select"]').setValue('resolved')
    await wrapper.get('[data-testid="admin-status-reason"]').setValue('fixed upstream')
    await wrapper.get('[data-testid="admin-update-status-form"]').trigger('submit')
    await flushPromises()

    expect(updateStatus).toHaveBeenCalledWith(123, {
      status: 'resolved',
      reason: 'fixed upstream',
    })

    await wrapper.get('[data-testid="admin-reopen-reason"]').setValue('user replied')
    await wrapper.get('[data-testid="admin-reopen-form"]').trigger('submit')
    await flushPromises()

    expect(reopen).toHaveBeenCalledWith(123, { reason: 'user replied' })

    await wrapper.get('[data-testid="admin-visibility-reason"]').setValue('private data')
    await wrapper.get('[data-testid="admin-visibility-form"]').trigger('submit')
    await flushPromises()

    expect(hideIssue).toHaveBeenCalledWith(123, { reason: 'private data' })

    await wrapper.get('[data-testid="admin-hide-comment-reason"]').setValue('contains private data')
    await wrapper.get('[data-testid="admin-hide-comment-form"]').trigger('submit')
    await flushPromises()

    expect(hideComment).toHaveBeenCalledWith(123, 501, { reason: 'contains private data' })

    await wrapper.get('[data-testid="admin-hide-attachment-reason"]').setValue('contains private data')
    await wrapper.get('[data-testid="admin-hide-attachment-form"]').trigger('submit')
    await flushPromises()

    expect(hideAttachment).toHaveBeenCalledWith(123, 77, { reason: 'contains private data' })

    await wrapper.get('[data-testid="admin-comment-input"]').setValue('admin note')
    await wrapper.get('[data-testid="admin-comment-form"]').trigger('submit')
    await flushPromises()

    expect(addComment).toHaveBeenCalledWith(123, { content: 'admin note' })
  })

  it('supports pinning, solution marking, and related solved issue linking', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="admin-pin-reason"]').setValue('frequent duplicate')
    await wrapper.get('[data-testid="admin-pin-form"]').trigger('submit')
    await flushPromises()

    expect(pin).toHaveBeenCalledWith(123, { reason: 'frequent duplicate' })

    await wrapper.get('[data-testid="admin-solution-comment-select"]').setValue('501')
    await wrapper.get('[data-testid="admin-solution-form"]').trigger('submit')
    await flushPromises()

    expect(markSolution).toHaveBeenCalledWith(123, { comment_id: 501 })

    await wrapper.get('[data-testid="admin-related-issue-id"]').setValue(999)
    await wrapper.get('[data-testid="admin-related-issue-reason"]').setValue('same resolved issue')
    await wrapper.get('[data-testid="admin-related-form"]').trigger('submit')
    await flushPromises()

    expect(setRelatedIssue).toHaveBeenCalledWith(123, {
      related_issue_id: 999,
      reason: 'same resolved issue',
    })
  })
})
