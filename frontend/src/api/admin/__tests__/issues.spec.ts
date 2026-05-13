import { beforeEach, describe, expect, it, vi } from 'vitest'
import type {
  AdminSupportIssue,
  AdminSupportIssueAttachment,
  CreateSupportIssueRequest,
  PublicSupportIssue,
  PublicSupportIssueAttachment,
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

import { adminIssuesAPI } from '@/api/admin/issues'

type Assert<T extends true> = T
type HasKey<T, K extends PropertyKey> = K extends keyof T ? true : false
type Not<T extends boolean> = T extends true ? false : true

const adminIssueIncludesAccountEmail: Assert<HasKey<AdminSupportIssue, 'account_email'>> = true
const adminIssueIncludesAPIKeySuffix: Assert<HasKey<AdminSupportIssue, 'api_key_suffix'>> = true
const adminAttachmentIncludesFilePath: Assert<HasKey<AdminSupportIssueAttachment, 'file_path'>> = true
const publicIssueOmitsAccountEmail: Assert<Not<HasKey<PublicSupportIssue, 'account_email'>>> = true
const publicAttachmentOmitsFilePath: Assert<
  Not<HasKey<PublicSupportIssueAttachment, 'file_path'>>
> = true
const createRequestOmitsInlineAttachments: Assert<
  Not<HasKey<CreateSupportIssueRequest, 'attachments'>>
> = true

describe('admin issues api', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    patch.mockReset()
    get.mockResolvedValue({ data: {} })
    post.mockResolvedValue({ data: {} })
    patch.mockResolvedValue({ data: {} })
  })

  it('lists admin issues', async () => {
    const params: SupportIssueListParams = {
      q: 'key:ab12cd',
      status: 'open',
      category: 'payment',
      severity: 'blocked',
      has_image: true,
      page: 1,
      page_size: 20,
      sort_by: 'updated_at',
      sort_order: 'desc'
    }
    const signal = new AbortController().signal

    await adminIssuesAPI.list(params, { signal })

    expect(get).toHaveBeenCalledWith('/admin/issues', { params, signal })
  })

  it('gets an admin issue by id', async () => {
    await adminIssuesAPI.get(123)

    expect(get).toHaveBeenCalledWith('/admin/issues/123')
  })

  it('updates issue status', async () => {
    const payload = { status: 'resolved' as const, reason: 'fixed' }

    await adminIssuesAPI.updateStatus(123, payload)

    expect(patch).toHaveBeenCalledWith('/admin/issues/123/status', payload)
  })

  it('reopens an issue', async () => {
    const payload = { reason: 'needs another look' }

    await adminIssuesAPI.reopen(123, payload)

    expect(post).toHaveBeenCalledWith('/admin/issues/123/reopen', payload)
  })

  it('hides a comment', async () => {
    const payload = { reason: 'sensitive info' }

    await adminIssuesAPI.hideComment(123, 456, payload)

    expect(post).toHaveBeenCalledWith('/admin/issues/123/comments/456/hide', payload)
  })

  it('hides an attachment', async () => {
    const payload = { reason: 'contains token' }

    await adminIssuesAPI.hideAttachment(123, 789, payload)

    expect(post).toHaveBeenCalledWith('/admin/issues/123/attachments/789/hide', payload)
  })

  it('lists issue events', async () => {
    await adminIssuesAPI.events(123)

    expect(get).toHaveBeenCalledWith('/admin/issues/123/events')
  })

  it('keeps admin types able to carry diagnostic fields', () => {
    const issue: AdminSupportIssue = {
      id: 123,
      public_id: 'ISS-000123',
      title: 'Payment missing',
      description: 'Balance did not arrive.',
      account_email: 'user@example.com',
      account_email_normalized: 'user@example.com',
      account_email_masked: 'u***@example.com',
      occurred_at: '2026-05-13T08:00:00Z',
      screenshot_text: 'balance missing',
      screenshot_language: 'en',
      category: 'payment',
      severity: 'partial',
      status: 'open',
      api_key_suffix: 'ab12cd',
      created_by_user_id: 99,
      resolved_by_user_id: null,
      comment_count: 1,
      hidden_comment_count: 1,
      attachment_count: 1,
      created_at: '2026-05-13T08:00:00Z',
      updated_at: '2026-05-13T08:00:00Z',
      attachments: [
        {
          id: 5,
          issue_id: 123,
          file_path: 'data/support-issue-attachments/private.png',
          file_url: '/api/v1/issues/attachments/5/file',
          file_name: 'private.png',
          mime_type: 'image/png',
          size_bytes: 10,
          visibility: 'public',
          created_at: '2026-05-13T08:00:00Z'
        }
      ]
    }

    expect(issue.account_email).toBe('user@example.com')
    expect(issue.api_key_suffix).toBe('ab12cd')
    expect(issue.attachments?.[0]?.file_path).toBe('data/support-issue-attachments/private.png')
    expect(adminIssueIncludesAccountEmail).toBe(true)
    expect(adminIssueIncludesAPIKeySuffix).toBe(true)
    expect(adminAttachmentIncludesFilePath).toBe(true)
  })

  it('keeps public types and create request scoped to public-safe fields', () => {
    expect(publicIssueOmitsAccountEmail).toBe(true)
    expect(publicAttachmentOmitsFilePath).toBe(true)
    expect(createRequestOmitsInlineAttachments).toBe(true)
  })
})
