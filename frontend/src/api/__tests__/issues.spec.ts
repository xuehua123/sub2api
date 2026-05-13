import { beforeEach, describe, expect, it, vi } from 'vitest'
import type {
  CreateSupportIssueRequest,
  PublicSupportIssue,
  PublicSupportIssueAttachment,
  PublicSupportIssueComment,
  SupportIssueListParams
} from '@/types'

const { get, post, patch } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  patch: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    get,
    post,
    patch
  }
}))

import { issuesAPI } from '@/api/issues'

type Assert<T extends true> = T
type HasKey<T, K extends PropertyKey> = K extends keyof T ? true : false
type Not<T extends boolean> = T extends true ? false : true

const createRequestOmitsInlineAttachments: Assert<
  Not<HasKey<CreateSupportIssueRequest, 'attachments'>>
> = true
const publicIssueOmitsAccountEmail: Assert<Not<HasKey<PublicSupportIssue, 'account_email'>>> = true
const publicIssueOmitsAPIKeySuffix: Assert<Not<HasKey<PublicSupportIssue, 'api_key_suffix'>>> = true
const publicAttachmentOmitsFilePath: Assert<
  Not<HasKey<PublicSupportIssueAttachment, 'file_path'>>
> = true
const publicCommentOmitsHideReason: Assert<Not<HasKey<PublicSupportIssueComment, 'hide_reason'>>> = true

describe('issues api', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    patch.mockReset()
    get.mockResolvedValue({ data: {} })
    post.mockResolvedValue({ data: {} })
    patch.mockResolvedValue({ data: {} })
  })

  it('lists issues with filters and pagination params', async () => {
    const params: SupportIssueListParams = {
      q: 'status:open',
      status: 'open',
      category: 'payment',
      severity: 'blocked',
      has_image: true,
      page: 2,
      page_size: 10,
      sort_by: 'last_comment_at',
      sort_order: 'desc'
    }
    const signal = new AbortController().signal

    await issuesAPI.list(params, { signal })

    expect(get).toHaveBeenCalledWith('/issues', { params, signal })
  })

  it('lists trending issues through the 24h endpoint', async () => {
    const params: SupportIssueListParams = {
      window: '24h',
      sort_by: 'hot_24h',
      sort_order: 'desc'
    }

    await issuesAPI.trending(params)

    expect(get).toHaveBeenCalledWith('/issues/trending', { params, signal: undefined })
  })

  it('lists my issues through the authenticated endpoint', async () => {
    const params: SupportIssueListParams = {
      status: 'open',
      page: 1,
      page_size: 20,
      sort_by: 'created_at',
      sort_order: 'desc'
    }

    await issuesAPI.mine(params)

    expect(get).toHaveBeenCalledWith('/issues/mine', { params, signal: undefined })
  })

  it('gets an issue by id', async () => {
    await issuesAPI.get(123)

    expect(get).toHaveBeenCalledWith('/issues/123')
  })

  it('creates an issue with attachment_ids and no inline attachments contract', async () => {
    const payload: CreateSupportIssueRequest = {
      title: 'Claude API 429',
      description: 'Requests are rate limited.',
      account_email: 'user@example.com',
      occurred_at: '2026-05-13T08:00:00Z',
      screenshot_text: 'rate limit',
      screenshot_language: 'en',
      category: 'api_call',
      severity: 'blocked',
      attachment_ids: [10, 11]
    }

    await issuesAPI.create(payload)

    expect(post).toHaveBeenCalledWith('/issues', payload)
    expect('attachments' in payload).toBe(false)
    expect(createRequestOmitsInlineAttachments).toBe(true)
  })

  it('adds a comment', async () => {
    const payload = { content: 'same issue here' }

    await issuesAPI.addComment(123, payload)

    expect(post).toHaveBeenCalledWith('/issues/123/comments', payload)
  })

  it('resolves an issue', async () => {
    await issuesAPI.resolve(123)

    expect(patch).toHaveBeenCalledWith('/issues/123/resolve')
  })

  it('requests search suggestions', async () => {
    const payload: CreateSupportIssueRequest = {
      title: 'Payment missing',
      description: 'Balance did not arrive.',
      account_email: 'user@example.com',
      occurred_at: '2026-05-13T08:00:00Z',
      screenshot_text: 'balance missing',
      screenshot_language: 'en',
      category: 'payment',
      severity: 'partial'
    }

    await issuesAPI.searchSuggestions(payload)

    expect(post).toHaveBeenCalledWith('/issues/search-suggestions', payload)
  })

  it('uploads an attachment using FormData without a manual content type config', async () => {
    const file = new File(['image-bytes'], 'screenshot.png', { type: 'image/png' })

    await issuesAPI.uploadAttachment(file)

    expect(post).toHaveBeenCalledTimes(1)
    const [path, body, config] = post.mock.calls[0]
    expect(path).toBe('/issues/attachments')
    expect(body).toBeInstanceOf(FormData)
    expect((body as FormData).get('file')).toBe(file)
    expect(config).toBeUndefined()
  })

  it('builds the public attachment file url', () => {
    expect(issuesAPI.attachmentFileURL(99)).toBe('/api/v1/issues/attachments/99/file')
  })

  it('keeps public issue types free of private fields', () => {
    expect(publicIssueOmitsAccountEmail).toBe(true)
    expect(publicIssueOmitsAPIKeySuffix).toBe(true)
    expect(publicAttachmentOmitsFilePath).toBe(true)
    expect(publicCommentOmitsHideReason).toBe(true)
  })
})
