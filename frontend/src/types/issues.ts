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

export interface SupportIssueReference {
  id: number
  public_id: string
  title: string
  status: SupportIssueStatus
  resolved_at?: string | null
}

export interface PublicSupportIssueComment {
  id: number
  issue_id: number
  author_role: string
  author_display_name?: string
  content: string
  related_issue_id?: number | null
  related_issue?: SupportIssueReference | null
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
  pinned_at?: string | null
  solution_comment_id?: number | null
  related_issue_id?: number | null
  related_issue?: SupportIssueReference | null
  last_comment_at?: string | null
  last_viewed_at?: string | null
  viewer_last_viewed_at?: string | null
  viewer_last_activity_at?: string | null
  has_unread_activity?: boolean
  attention_reason?: 'new_activity' | 'needs_info' | 'resolved' | 'solution' | 'related_solved' | string
  comment_count: number
  attachment_count: number
  view_count: number
  created_at: string
  updated_at: string
  comments?: PublicSupportIssueComment[]
  attachments?: PublicSupportIssueAttachment[]
  solution_comment?: PublicSupportIssueComment | null
}

export interface SupportIssueNotificationSummary {
  unread_count: number
  needs_info_count: number
  resolved_unread_count: number
  latest_activity_at?: string | null
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
  pinned_at?: string | null
  pinned_by_user_id?: number | null
  pinned_reason?: string
  solution_comment_id?: number | null
  related_issue_id?: number | null
  related_issue_reason?: string
  hidden_comment_count: number
  comments?: AdminSupportIssueComment[]
  attachments?: AdminSupportIssueAttachment[]
  events?: SupportIssueEvent[]
  solution_comment?: AdminSupportIssueComment | null
  related_issue?: SupportIssueReference | null
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
  related_issue_id?: number | null
}

export interface UpdateSupportIssueStatusRequest {
  status: SupportIssueStatus
  reason?: string
}

export interface SupportIssueReasonRequest {
  reason?: string
}

export interface PinSupportIssueRequest {
  reason?: string
}

export interface MarkSupportIssueSolutionRequest {
  comment_id: number
}

export interface SetRelatedSupportIssueRequest {
  related_issue_id: number
  reason?: string
}

export interface SupportIssueListFilters {
  status?: SupportIssueStatus | 'pending' | 'all'
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
