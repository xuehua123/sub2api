import { apiClient } from './client'
import type {
  AddSupportIssueCommentRequest,
  BasePaginationResponse,
  CreateSupportIssueRequest,
  FetchOptions,
  PublicSupportIssue,
  PublicSupportIssueComment,
  SupportIssueListParams,
  UploadedSupportIssueAttachment
} from '@/types'

export async function list(
  params?: SupportIssueListParams,
  options?: FetchOptions
): Promise<BasePaginationResponse<PublicSupportIssue>> {
  const { data } = await apiClient.get<BasePaginationResponse<PublicSupportIssue>>('/issues', {
    params,
    signal: options?.signal
  })
  return data
}

export async function trending(
  params?: SupportIssueListParams,
  options?: FetchOptions
): Promise<BasePaginationResponse<PublicSupportIssue>> {
  const { data } = await apiClient.get<BasePaginationResponse<PublicSupportIssue>>('/issues/trending', {
    params,
    signal: options?.signal
  })
  return data
}

export async function get(id: number): Promise<PublicSupportIssue> {
  const { data } = await apiClient.get<PublicSupportIssue>(`/issues/${id}`)
  return data
}

export async function create(request: CreateSupportIssueRequest): Promise<PublicSupportIssue> {
  const { data } = await apiClient.post<PublicSupportIssue>('/issues', request)
  return data
}

export async function addComment(
  id: number,
  request: AddSupportIssueCommentRequest
): Promise<PublicSupportIssueComment> {
  const { data } = await apiClient.post<PublicSupportIssueComment>(`/issues/${id}/comments`, request)
  return data
}

export async function resolve(id: number): Promise<PublicSupportIssue> {
  const { data } = await apiClient.patch<PublicSupportIssue>(`/issues/${id}/resolve`)
  return data
}

export async function searchSuggestions(request: CreateSupportIssueRequest): Promise<PublicSupportIssue[]> {
  const { data } = await apiClient.post<PublicSupportIssue[]>('/issues/search-suggestions', request)
  return data
}

export async function uploadAttachment(file: File): Promise<UploadedSupportIssueAttachment> {
  const formData = new FormData()
  formData.append('file', file)

  const { data } = await apiClient.post<UploadedSupportIssueAttachment>('/issues/attachments', formData)
  return data
}

export function attachmentFileURL(id: number): string {
  return `/api/v1/issues/attachments/${id}/file`
}

export const issuesAPI = {
  list,
  trending,
  get,
  create,
  addComment,
  resolve,
  searchSuggestions,
  uploadAttachment,
  attachmentFileURL
}

export default issuesAPI
