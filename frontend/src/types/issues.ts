export type SupportIssueStatus = 'open' | 'needs_info' | 'in_progress' | 'resolved' | 'closed'

export type SupportIssueCategory =
  | 'login'
  | 'payment'
  | 'api_call'
  | 'model_unavailable'
  | 'api_key'
  | 'balance'
  | 'subscription'
  | 'channel'
  | 'other'

export type SupportIssueSeverity = 'blocked' | 'partial' | 'intermittent' | 'question'

export type SupportIssueScreenshotLanguage = 'zh' | 'en' | 'mixed' | 'unknown'

export interface PublicSupportIssueComment {
  id: number
  issue_id: number
  author_role: string
  content: string
  created_at: string
  updated_at: string
}

export interface PublicSupportIssueAttachment {
  id: number
  issue_id: number
  file_url: string
  file_name: string
  mime_type: string
  size_bytes: number
  ocr_text?: string
  visibility: string
  created_at: string
}

export interface UploadedSupportIssueAttachment {
  id: number
  file_url: string
  file_name: string
  mime_type: string
  size_bytes: number
  created_at: string
}

export interface PublicSupportIssue {
  id: number
  public_id: string
  title: string
  description: string
  account_email_masked: string
  occurred_at: string
  screenshot_text: string
  screenshot_language: SupportIssueScreenshotLanguage
  category: SupportIssueCategory
  severity: SupportIssueSeverity
  status: SupportIssueStatus
  model_name?: string
  client_name?: string
  http_status?: number | null
  error_code?: string
  resolved_at?: string | null
  locked_at?: string | null
  last_comment_at?: string | null
  last_viewed_at?: string | null
  comment_count: number
  attachment_count: number
  view_count: number
  created_at: string
  updated_at: string
  comments?: PublicSupportIssueComment[]
  attachments?: PublicSupportIssueAttachment[]
}

export interface AdminSupportIssueComment extends PublicSupportIssueComment {
  author_user_id?: number | null
  hidden_at?: string | null
  hidden_by_user_id?: number | null
  hide_reason?: string
}

export interface AdminSupportIssueAttachment extends PublicSupportIssueAttachment {
  uploaded_by_user_id?: number | null
  file_path?: string
  hidden_at?: string | null
  hidden_by_user_id?: number | null
}

export interface SupportIssueEvent {
  id: number
  issue_id: number
  actor_user_id?: number | null
  event_type: string
  from_status?: SupportIssueStatus | string | null
  to_status?: SupportIssueStatus | string | null
  metadata?: Record<string, unknown>
  created_at: string
}

export interface AdminSupportIssue
  extends Omit<PublicSupportIssue, 'comments' | 'attachments'> {
  account_email: string
  account_email_normalized: string
  api_key_suffix?: string
  created_by_user_id?: number | null
  resolved_by_user_id?: number | null
  hidden_at?: string | null
  hidden_by_user_id?: number | null
  hide_reason?: string
  hidden_comment_count: number
  comments?: AdminSupportIssueComment[]
  attachments?: AdminSupportIssueAttachment[]
  events?: SupportIssueEvent[]
}

export interface CreateSupportIssueRequest {
  title: string
  description: string
  account_email: string
  occurred_at: string
  screenshot_text: string
  screenshot_language: SupportIssueScreenshotLanguage
  category: SupportIssueCategory
  severity: SupportIssueSeverity
  model_name?: string
  client_name?: string
  http_status?: number | null
  error_code?: string
  api_key_suffix?: string
  attachment_ids?: number[]
}

export interface AddSupportIssueCommentRequest {
  content: string
}

export interface UpdateSupportIssueStatusRequest {
  status: SupportIssueStatus
  reason?: string
}

export interface SupportIssueReasonRequest {
  reason?: string
}

export interface SupportIssueListFilters {
  status?: SupportIssueStatus
  category?: SupportIssueCategory
  severity?: SupportIssueSeverity
  has_image?: boolean
  hidden?: boolean
}

export interface SupportIssueListParams extends SupportIssueListFilters {
  q?: string
  window?: '24h' | '7d' | string
  page?: number
  page_size?: number
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}
