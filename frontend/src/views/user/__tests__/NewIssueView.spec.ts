import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import NewIssueView from '../NewIssueView.vue'
import type { PublicSupportIssue } from '@/types'

const { uploadAttachment, create, searchSuggestions, router } = vi.hoisted(() => ({
  uploadAttachment: vi.fn(),
  create: vi.fn(),
  searchSuggestions: vi.fn(),
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
  await wrapper.find('[data-testid="new-issue-description"]').setValue('Requests fail with 429 after a few calls.')
  await wrapper.find('[data-testid="new-issue-email"]').setValue('user@example.com')
  await wrapper.find('[data-testid="new-issue-occurred-at"]').setValue('2026-05-13T08:00')
  await wrapper.find('[data-testid="new-issue-language"]').setValue('en')
  await wrapper.find('[data-testid="new-issue-category"]').setValue('api_call')
  await wrapper.find('[data-testid="new-issue-severity"]').setValue('blocked')
  await wrapper.find('[data-testid="new-issue-screenshot-text"]').setValue('rate limit')
}

describe('NewIssueView', () => {
  beforeEach(() => {
    uploadAttachment.mockReset()
    create.mockReset()
    searchSuggestions.mockReset()
    router.push.mockReset()
    vi.stubGlobal('URL', {
      ...URL,
      createObjectURL: vi.fn(() => 'blob:preview'),
      revokeObjectURL: vi.fn(),
    })
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
    expect(payload.attachment_ids).toEqual([77])
    expect(payload).not.toHaveProperty('attachments')
  })

  it('searches for similar issues and displays possible duplicates', async () => {
    searchSuggestions.mockResolvedValue([duplicateIssue])

    const wrapper = mountView()
    await fillRequiredFields(wrapper)
    await wrapper.find('[data-testid="suggestions-button"]').trigger('click')
    await flushPromises()

    expect(searchSuggestions).toHaveBeenCalled()
    expect(wrapper.find('[data-testid="suggestions-list"]').text()).toContain('Existing rate limit issue')
    expect(wrapper.text()).toContain('ISS-000321')
  })
})
