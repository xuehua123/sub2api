import { apiClient } from './client'

export interface LobeHubLaunchTicketResponse {
  ticket_id: string
  bridge_url: string
}

export interface LobeHubOIDCWebSessionRequest {
  resume_token?: string
  return_url: string
  api_key_id?: number
  mode?: 'refresh-target'
}

export interface LobeHubOIDCWebSessionResponse {
  continue_url: string
}

export interface LobeHubBootstrapExchangeResponse {
  bootstrap_ticket_id: string
}

export interface LobeHubBootstrapConsumeResponse {
  redirect_url: string
}

export async function createLobeHubLaunchTicket(
  apiKeyId: number
): Promise<LobeHubLaunchTicketResponse> {
  const { data } = await apiClient.post<LobeHubLaunchTicketResponse>('/lobehub/launch-ticket', {
    api_key_id: apiKeyId
  })
  return data
}

export async function createLobeHubOIDCWebSession(
  payload: LobeHubOIDCWebSessionRequest
): Promise<LobeHubOIDCWebSessionResponse> {
  const { data } = await apiClient.post<LobeHubOIDCWebSessionResponse>(
    '/lobehub/oidc-web-session',
    payload
  )
  return data
}

export async function exchangeLobeHubBootstrap(
  returnURL: string
): Promise<LobeHubBootstrapExchangeResponse> {
  const { data } = await apiClient.post<LobeHubBootstrapExchangeResponse>(
    '/lobehub/bootstrap-exchange',
    { return_url: returnURL }
  )
  return data
}

export async function consumeLobeHubBootstrap(
  ticketID: string
): Promise<LobeHubBootstrapConsumeResponse> {
  const { data } = await apiClient.get<LobeHubBootstrapConsumeResponse>(
    '/lobehub/bootstrap/consume',
    {
      params: { ticket: ticketID }
    }
  )
  return data
}
