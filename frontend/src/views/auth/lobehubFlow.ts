import type { LocationQuery, LocationQueryRaw } from 'vue-router'

import type { ApiKey } from '@/types'

export type LobeHubContinuationMode = 'oidc' | 'refresh-target'

export interface LobeHubContinuationQuery {
  resumeToken: string
  returnURL: string
  mode: LobeHubContinuationMode
}

export interface LobeHubContinuationPayload {
  resume_token?: string
  return_url: string
  api_key_id?: number
  mode?: 'refresh-target'
}

function readQueryString(value: LocationQuery[string]): string {
  if (Array.isArray(value)) {
    return typeof value[0] === 'string' ? value[0].trim() : ''
  }
  return typeof value === 'string' ? value.trim() : ''
}

export function parseLobeHubContinuationQuery(query: LocationQuery): LobeHubContinuationQuery {
  const mode = readQueryString(query.mode) === 'refresh-target' ? 'refresh-target' : 'oidc'

  return {
    resumeToken: readQueryString(query.resume),
    returnURL: readQueryString(query.return_url),
    mode
  }
}

export function buildLobeHubContinuationPayload(
  query: LocationQuery,
  apiKeyID?: number
): LobeHubContinuationPayload {
  const parsed = parseLobeHubContinuationQuery(query)
  const payload: LobeHubContinuationPayload = {
    return_url: parsed.returnURL
  }

  if (parsed.mode === 'refresh-target') {
    payload.mode = 'refresh-target'
  } else if (parsed.resumeToken) {
    payload.resume_token = parsed.resumeToken
  }

  if (typeof apiKeyID === 'number' && apiKeyID > 0) {
    payload.api_key_id = apiKeyID
  }

  return payload
}

export function buildLobeHubSelectKeyRouteQuery(query: LocationQuery): LocationQueryRaw {
  const parsed = parseLobeHubContinuationQuery(query)
  const nextQuery: LocationQueryRaw = {}

  if (parsed.resumeToken) {
    nextQuery.resume = parsed.resumeToken
  }
  if (parsed.returnURL) {
    nextQuery.return_url = parsed.returnURL
  }
  if (parsed.mode === 'refresh-target') {
    nextQuery.mode = 'refresh-target'
  }

  return nextQuery
}

export function isLobeHubDefaultKeyRequiredError(error: unknown): boolean {
  if (!error || typeof error !== 'object') {
    return false
  }

  const candidate = error as { code?: string; status?: number }
  return (
    candidate.code === 'LOBEHUB_DEFAULT_CHAT_API_KEY_REQUIRED' ||
    candidate.status === 409
  )
}

export function filterSelectableLobeHubKeys(keys: ApiKey[]): ApiKey[] {
  return keys.filter((key) => key.status === 'active')
}
