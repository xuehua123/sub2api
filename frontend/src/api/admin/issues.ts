import { apiClient } from '../client'
import type {
  AdminSupportIssue,
  BasePaginationResponse,
  FetchOptions,
  SupportIssueEvent,
  SupportIssueListParams,
  SupportIssueReasonRequest,
  UpdateSupportIssueStatusRequest
} from '@/types'

export async function list(
  params?: SupportIssueListParams,
  options?: FetchOptions
): Promise<BasePaginationResponse<AdminSupportIssue>> {
  const { data } = await apiClient.get<BasePaginationResponse<AdminSupportIssue>>('/admin/issues', {
    params,
    signal: options?.signal
  })
  return data
}

export async function get(id: number): Promise<AdminSupportIssue> {
  const { data } = await apiClient.get<AdminSupportIssue>(`/admin/issues/${id}`)
  return data
}

export async function updateStatus(
  id: number,
  request: UpdateSupportIssueStatusRequest
): Promise<AdminSupportIssue> {
  const { data } = await apiClient.patch<AdminSupportIssue>(`/admin/issues/${id}/status`, request)
  return data
}

export async function reopen(
  id: number,
  request?: SupportIssueReasonRequest
): Promise<AdminSupportIssue> {
  const { data } = await apiClient.post<AdminSupportIssue>(`/admin/issues/${id}/reopen`, request ?? {})
  return data
}

export async function hideComment(
  issueId: number,
  commentId: number,
  request?: SupportIssueReasonRequest
): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(
    `/admin/issues/${issueId}/comments/${commentId}/hide`,
    request ?? {}
  )
  return data
}

export async function hideAttachment(
  issueId: number,
  attachmentId: number,
  request?: SupportIssueReasonRequest
): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(
    `/admin/issues/${issueId}/attachments/${attachmentId}/hide`,
    request ?? {}
  )
  return data
}

export async function events(id: number): Promise<SupportIssueEvent[]> {
  const { data } = await apiClient.get<SupportIssueEvent[]>(`/admin/issues/${id}/events`)
  return data
}

export const adminIssuesAPI = {
  list,
  get,
  updateStatus,
  reopen,
  hideComment,
  hideAttachment,
  events
}

export default adminIssuesAPI
