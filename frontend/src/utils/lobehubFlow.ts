import type { RouteLocationNormalizedLoaded } from 'vue-router'
import type { CreateLobeHubOIDCWebSessionRequest } from '@/api/lobehub'
import type { ApiKey } from '@/types'

type QueryValue = RouteLocationNormalizedLoaded['query'][string]

export interface LobeHubFlowQuery {
  resumeToken: string
  returnURL: string
  mode: 'oidc' | 'refresh-target'
}

function firstString(value: QueryValue): string {
  if (Array.isArray(value)) {
    return typeof value[0] === 'string' ? value[0].trim() : ''
  }
  return typeof value === 'string' ? value.trim() : ''
}

export function parseLobeHubFlowQuery(query: RouteLocationNormalizedLoaded['query']): LobeHubFlowQuery {
  const mode = firstString(query.mode) === 'refresh-target' ? 'refresh-target' : 'oidc'
  return {
    resumeToken: firstString(query.resume),
    returnURL: firstString(query.return_url),
    mode
  }
}

export function buildLobeHubOIDCWebSessionPayload(
  query: RouteLocationNormalizedLoaded['query'],
  apiKeyId?: number
): CreateLobeHubOIDCWebSessionRequest {
  const parsed = parseLobeHubFlowQuery(query)
  const payload: CreateLobeHubOIDCWebSessionRequest = {
    return_url: parsed.returnURL
  }

  if (parsed.mode === 'refresh-target') {
    payload.mode = 'refresh-target'
  } else if (parsed.resumeToken) {
    payload.resume_token = parsed.resumeToken
  }

  if (typeof apiKeyId === 'number' && apiKeyId > 0) {
    payload.api_key_id = apiKeyId
  }

  return payload
}

export function buildLobeHubSelectKeyQuery(query: RouteLocationNormalizedLoaded['query']) {
  const parsed = parseLobeHubFlowQuery(query)
  const nextQuery: Record<string, string> = {}
  if (parsed.resumeToken) nextQuery.resume = parsed.resumeToken
  if (parsed.returnURL) nextQuery.return_url = parsed.returnURL
  if (parsed.mode === 'refresh-target') nextQuery.mode = 'refresh-target'
  return nextQuery
}

export function isDefaultChatAPIKeyRequired(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false
  const err = error as { code?: unknown; status?: unknown }
  return err.code === 'LOBEHUB_DEFAULT_CHAT_API_KEY_REQUIRED' || err.status === 409
}

export function filterActiveLobeHubKeys(keys: ApiKey[]): ApiKey[] {
  return keys.filter((key) => key.status === 'active')
}
