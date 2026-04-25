import { apiClient } from './client'

export interface CreateLobeHubLaunchTicketResponse {
  ticket_id: string
  bridge_url: string
}

export interface CreateLobeHubOIDCWebSessionRequest {
  resume_token?: string
  return_url: string
  api_key_id?: number
  mode?: 'refresh-target'
}

export interface CreateLobeHubOIDCWebSessionResponse {
  continue_url: string
}

export async function createLaunchTicket(apiKeyId: number): Promise<CreateLobeHubLaunchTicketResponse> {
  const { data } = await apiClient.post<CreateLobeHubLaunchTicketResponse>('/lobehub/launch-ticket', {
    api_key_id: apiKeyId
  })
  return data
}

export async function createOIDCWebSession(
  payload: CreateLobeHubOIDCWebSessionRequest
): Promise<CreateLobeHubOIDCWebSessionResponse> {
  const { data } = await apiClient.post<CreateLobeHubOIDCWebSessionResponse>(
    '/lobehub/oidc-web-session',
    payload
  )
  return data
}

export const lobehubAPI = {
  createLaunchTicket,
  createOIDCWebSession
}

export default lobehubAPI
