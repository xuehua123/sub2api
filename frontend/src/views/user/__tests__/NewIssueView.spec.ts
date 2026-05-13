import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import NewIssueView from '../NewIssueView.vue'
import type { PublicSupportIssue } from '@/types'

const { uploadAttachment, create, searchSuggestions, queryUsage, refreshUser, router, authStore } = vi.hoisted(() => ({
  uploadAttachment: vi.fn(),
  create: vi.fn(),
  searchSuggestions: vi.fn(),
  queryUsage: vi.fn(),
  refreshUser: vi.fn(),
  authStore: {
    user: {
      id: 7,
      email: 'owner@example.com',
      role: 'user',
    },
    token: 'token',
    refreshUser: vi.fn(),
  },
  router: {
    push: vi.fn(),
  },
}))

vi.mock('@/api/issues', () => ({
  issuesAPI: {
    uploadAttachment,
    create,
    searchSuggestions,
  },
}))

vi.mock('@/api/usage', () => ({
  usageAPI: {
    query: queryUsage,
  },
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStore,
}))

vi.mock('vue-router', () => ({
  useRouter: () => router,
  RouterLink: {
    props: ['to'],
    template: '<a><slot /></a>',
  },
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

const duplicateIssue: PublicSupportIssue = {
  id: 321,
  public_id: 'ISS-000321',
  title: 'Existing rate limit issue',
  description: 'Same problem.',
  account_email_masked: 'u***@example.com',
  occurred_at: '2026-05-13T08:00:00Z',
  screenshot_text: 'rate limit',
  screenshot_language: 'en',
  category: 'api_call',
  severity: 'blocked',
  status: 'resolved',
  comment_count: 1,
  attachment_count: 0,
  created_at: '2026-05-13T08:00:00Z',
  updated_at: '2026-05-13T08:00:00Z',
}

function mountView() {
  return mount(NewIssueView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        RouterLink: { props: ['to'], template: '<a><slot /></a>' },
      },
    },
  })
}

async function fillRequiredFields(wrapper: ReturnType<typeof mount>) {
  await wrapper.find('[data-testid="new-issue-title"]').setValue('Rate limit on Claude')
  await wrapper.find('[data-testid="new-issue-error-summary"]').setValue('Requests fail with 429 after a few calls.')
  await wrapper.find('[data-testid="new-issue-description"]').setValue('I called Claude Code several times and expected a normal response, but every request failed after a short wait.')
  await wrapper.find('[data-testid="new-issue-screenshot-text"]').setValue('rate limit exceeded')
  await wrapper.find('[data-testid="new-issue-language"]').setValue('en')
  await wrapper.find('[data-testid="new-issue-category"]').setValue('api_call')
  await wrapper.find('[data-testid="new-issue-severity"]').setValue('blocked')
}

describe('NewIssueView', () => {
  beforeEach(() => {
    vi.useRealTimers()
    uploadAttachment.mockReset()
    create.mockReset()
    searchSuggestions.mockReset()
    queryUsage.mockReset()
    refreshUser.mockReset()
    authStore.refreshUser = refreshUser
    authStore.user = {
      id: 7,
      email: 'owner@example.com',
      role: 'user',
    }
    authStore.token = 'token'
    router.push.mockReset()
    vi.stubGlobal('URL', {
      ...URL,
      createObjectURL: vi.fn(() => 'blob:preview'),
      revokeObjectURL: vi.fn(),
    })
  })

  it('prefills account email from the signed-in user and defaults time to browser current minute', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-05-13T08:09:42'))

    const wrapper = mountView()
    await flushPromises()

    const emailInput = wrapper.find('[data-testid="new-issue-email"]').element as HTMLInputElement
    const occurredAtInput = wrapper.find('[data-testid="new-issue-occurred-at"]').element as HTMLInputElement

    expect(emailInput.value).toBe('owner@example.com')
    expect(emailInput.readOnly).toBe(true)
    expect(occurredAtInput.value).toBe('2026-05-13T08:09')
    expect(wrapper.find('[data-testid="new-issue-key-suffix"]').exists()).toBe(false)
  })

  it('uploads a screenshot and creates with attachment_ids only', async () => {
    uploadAttachment.mockResolvedValue({
      id: 77,
      file_url: '/api/v1/issues/attachments/77/file',
      file_name: 'screen.png',
      mime_type: 'image/png',
      size_bytes: 12,
      created_at: '2026-05-13T08:00:00Z',
    })
    searchSuggestions.mockResolvedValue([])
    create.mockResolvedValue({ id: 123 })

    const wrapper = mountView()
    await flushPromises()
    await fillRequiredFields(wrapper)

    const input = wrapper.find('[data-testid="issue-file-input"]').element as HTMLInputElement
    const file = new File(['png'], 'screen.png', { type: 'image/png' })
    Object.defineProperty(input, 'files', { value: [file] })
    await wrapper.find('[data-testid="issue-file-input"]').trigger('change')
    await flushPromises()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(uploadAttachment).toHaveBeenCalledWith(file)
    const payload = create.mock.calls[0][0] as Record<string, unknown>
    expect(payload.account_email).toBe('owner@example.com')
    expect(payload.attachment_ids).toEqual([77])
    expect(payload.title).toBe('Rate limit on Claude')
    expect(payload.description).toContain('Requests fail with 429')
    expect(payload.description).toContain('I called Claude Code')
    expect(payload.screenshot_text).toBe('rate limit exceeded')
    expect(payload).not.toHaveProperty('attachments')
  })

  it('uploads screenshots from drag and drop', async () => {
    uploadAttachment.mockResolvedValue({
      id: 88,
      file_url: '/api/v1/issues/attachments/88/file',
      file_name: 'drag.png',
      mime_type: 'image/png',
      size_bytes: 12,
      created_at: '2026-05-13T08:00:00Z',
    })
    searchSuggestions.mockResolvedValue([])
    create.mockResolvedValue({ id: 123 })

    const wrapper = mountView()
    await flushPromises()
    await fillRequiredFields(wrapper)

    const file = new File(['png'], 'drag.png', { type: 'image/png' })
    await wrapper.find('[data-testid="issue-dropzone"]').trigger('drop', {
      dataTransfer: {
        files: [file],
      },
    })
    await flushPromises()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(uploadAttachment).toHaveBeenCalledWith(file)
    expect((create.mock.calls[0][0] as Record<string, unknown>).attachment_ids).toEqual([88])
  })

  it('uploads screenshots pasted into the report field', async () => {
    uploadAttachment.mockResolvedValue({
      id: 99,
      file_url: '/api/v1/issues/attachments/99/file',
      file_name: 'paste.png',
      mime_type: 'image/png',
      size_bytes: 12,
      created_at: '2026-05-13T08:00:00Z',
    })
    searchSuggestions.mockResolvedValue([])
    create.mockResolvedValue({ id: 123 })

    const wrapper = mountView()
    await flushPromises()
    await fillRequiredFields(wrapper)

    const file = new File(['png'], 'paste.png', { type: 'image/png' })
    await wrapper.find('[data-testid="new-issue-description"]').trigger('paste', {
      clipboardData: {
        items: [
          {
            kind: 'file',
            type: 'image/png',
            getAsFile: () => file,
          },
        ],
        files: [file],
      },
    })
    await flushPromises()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(uploadAttachment).toHaveBeenCalledWith(file)
    expect((create.mock.calls[0][0] as Record<string, unknown>).attachment_ids).toEqual([99])
  })

  it('automatically searches for similar issues and displays possible duplicates', async () => {
    vi.useFakeTimers()
    searchSuggestions.mockResolvedValue([duplicateIssue])

    const wrapper = mountView()
    await flushPromises()
    await fillRequiredFields(wrapper)
    await vi.advanceTimersByTimeAsync(750)
    await flushPromises()

    expect(searchSuggestions).toHaveBeenCalled()
    expect(wrapper.find('[data-testid="similar-issues-panel"]').text()).toContain('issueCenter.new.similarFound')
    expect(wrapper.find('[data-testid="suggestions-list"]').text()).toContain('Existing rate limit issue')
    expect(wrapper.text()).toContain('ISS-000321')
  })

  it('loads a usage log time range and includes selected log diagnostics in the create request', async () => {
    queryUsage.mockResolvedValue({
      items: [
        {
          id: 456,
          user_id: 7,
          api_key_id: 12,
          account_id: null,
          request_id: 'req_456',
          model: 'claude-sonnet-4',
          inbound_endpoint: 'claude-code',
          upstream_endpoint: null,
          group_id: null,
          subscription_id: null,
          input_tokens: 100,
          output_tokens: 50,
          cache_creation_tokens: 0,
          cache_read_tokens: 0,
          cache_creation_5m_tokens: 0,
          cache_creation_1h_tokens: 0,
          input_cost: 0,
          output_cost: 0,
          cache_creation_cost: 0,
          cache_read_cost: 0,
          total_cost: 0,
          actual_cost: 0,
          rate_multiplier: 1,
          billing_type: 1,
          request_type: 'chat',
          stream: false,
          duration_ms: 1250,
          first_token_ms: null,
          first_sse_event_ms: null,
          first_client_flush_ms: null,
          image_count: 0,
          image_size: null,
          user_agent: 'Claude-Code/1.0',
          cache_ttl_overridden: false,
          billing_mode: null,
          created_at: '2026-05-13T08:07:30Z',
          api_key: { name: 'work key' },
          status_code: 429,
          error_code: 'rate_limit',
          error_message: 'Too many requests',
        },
        {
          id: 789,
          user_id: 7,
          api_key_id: 12,
          account_id: null,
          request_id: 'req_789',
          model: 'claude-sonnet-4',
          inbound_endpoint: 'claude-code',
          upstream_endpoint: null,
          group_id: null,
          subscription_id: null,
          input_tokens: 80,
          output_tokens: 20,
          cache_creation_tokens: 0,
          cache_read_tokens: 0,
          cache_creation_5m_tokens: 0,
          cache_creation_1h_tokens: 0,
          input_cost: 0,
          output_cost: 0,
          cache_creation_cost: 0,
          cache_read_cost: 0,
          total_cost: 0,
          actual_cost: 0,
          rate_multiplier: 1,
          billing_type: 1,
          request_type: 'chat',
          stream: false,
          duration_ms: 900,
          first_token_ms: null,
          first_sse_event_ms: null,
          first_client_flush_ms: null,
          image_count: 0,
          image_size: null,
          user_agent: 'Claude-Code/1.0',
          cache_ttl_overridden: false,
          billing_mode: null,
          created_at: '2026-05-13T08:05:30Z',
          api_key: { name: 'work key' },
          status_code: 429,
          error_code: 'rate_limit',
          error_message: 'Too many requests',
        },
      ],
      total: 2,
      page: 1,
      page_size: 50,
      pages: 1,
    })
    searchSuggestions.mockResolvedValue([])
    create.mockResolvedValue({ id: 123 })

    const wrapper = mountView()
    await flushPromises()
    await fillRequiredFields(wrapper)

    await wrapper.find('[data-testid="usage-log-range-start"]').setValue('2026-05-13T00:00')
    await wrapper.find('[data-testid="usage-log-range-end"]').setValue('2026-05-14T00:00')
    await wrapper.find('[data-testid="load-usage-logs"]').trigger('click')
    await flushPromises()

    const checkboxes = wrapper.findAll('[data-testid="usage-log-checkbox"]')
    expect(checkboxes).toHaveLength(2)
    expect((checkboxes[0].element as HTMLInputElement).checked).toBe(true)
    expect((checkboxes[1].element as HTMLInputElement).checked).toBe(true)

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const payload = create.mock.calls[0][0] as Record<string, unknown>
    expect(queryUsage).toHaveBeenCalledWith(expect.objectContaining({
      start_date: '2026-05-13',
      end_date: '2026-05-14',
      page_size: 50,
    }))
    expect(payload.model_name).toBe('claude-sonnet-4')
    expect(payload.client_name).toBe('claude-code')
    expect(payload.http_status).toBe(429)
    expect(payload.error_code).toBe('rate_limit')
    expect(payload.description).toContain('issueCenter.new.selectedLogPayloadTitle')
    expect(payload.description).toContain('456')
    expect(payload.description).toContain('789')
    expect(payload).not.toHaveProperty('attachments')
  })

  it('blocks low-quality reports without request logs', async () => {
    searchSuggestions.mockResolvedValue([])
    create.mockResolvedValue({ id: 123 })

    const wrapper = mountView()
    await flushPromises()
    await wrapper.find('[data-testid="new-issue-title"]').setValue('太慢了')
    await wrapper.find('[data-testid="new-issue-error-summary"]').setValue('太慢了')
    await wrapper.find('[data-testid="new-issue-description"]').setValue('看截图')
    await wrapper.find('[data-testid="new-issue-screenshot-text"]').setValue('报错')

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(create).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="new-issue-error"]').text()).toContain('issueCenter.new.lowQualityError')
  })

  it('requires evidence for slow or timeout issues unless request logs are selected', async () => {
    searchSuggestions.mockResolvedValue([])
    create.mockResolvedValue({ id: 123 })

    const wrapper = mountView()
    await flushPromises()
    await fillRequiredFields(wrapper)
    await wrapper.find('[data-testid="new-issue-scenario"]').setValue('slow_timeout')
    await wrapper.find('[data-testid="new-issue-model"]').setValue('')
    await wrapper.find('[data-testid="new-issue-client"]').setValue('')

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(create).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="new-issue-error"]').text()).toContain('issueCenter.new.slowEvidenceError')
  })
})
