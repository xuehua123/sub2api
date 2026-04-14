import { describe, expect, it } from 'vitest'

import {
  buildLobeHubOIDCWebSessionPayload,
  buildLobeHubSelectKeyQuery,
  filterActiveLobeHubKeys,
  isDefaultChatAPIKeyRequired,
  parseLobeHubFlowQuery
} from '../lobehubFlow'
import type { ApiKey } from '@/types'

function apiKey(id: number, status: ApiKey['status']): ApiKey {
  return {
    id,
    user_id: 1,
    key: `sk-test-${id}`,
    name: `key-${id}`,
    group_id: 1,
    status,
    ip_whitelist: [],
    ip_blacklist: [],
    last_used_at: null,
    quota: 0,
    quota_used: 0,
    expires_at: null,
    created_at: '2026-04-14T00:00:00Z',
    updated_at: '2026-04-14T00:00:00Z',
    rate_limit_5h: 0,
    rate_limit_1d: 0,
    rate_limit_7d: 0,
    usage_5h: 0,
    usage_1d: 0,
    usage_7d: 0,
    window_5h_start: null,
    window_1d_start: null,
    window_7d_start: null,
    reset_5h_at: null,
    reset_1d_at: null,
    reset_7d_at: null
  }
}

describe('lobehubFlow', () => {
  it('builds OIDC continuation payload from query', () => {
    const query = {
      resume: ' resume-token ',
      return_url: ' https://chat.example.com/callback '
    }

    expect(parseLobeHubFlowQuery(query)).toEqual({
      resumeToken: 'resume-token',
      returnURL: 'https://chat.example.com/callback',
      mode: 'oidc'
    })
    expect(buildLobeHubOIDCWebSessionPayload(query, 42)).toEqual({
      return_url: 'https://chat.example.com/callback',
      resume_token: 'resume-token',
      api_key_id: 42
    })
  })

  it('builds refresh-target payload without resume token', () => {
    const query = {
      resume: 'ignored-resume',
      return_url: 'https://chat.example.com/refresh',
      mode: 'refresh-target'
    }

    expect(buildLobeHubOIDCWebSessionPayload(query, 7)).toEqual({
      return_url: 'https://chat.example.com/refresh',
      mode: 'refresh-target',
      api_key_id: 7
    })
  })

  it('preserves only LobeHub handoff query fields for key selection', () => {
    expect(buildLobeHubSelectKeyQuery({
      resume: ['resume-token'],
      return_url: 'https://chat.example.com/callback',
      mode: 'refresh-target',
      ignored: 'value'
    })).toEqual({
      resume: 'resume-token',
      return_url: 'https://chat.example.com/callback',
      mode: 'refresh-target'
    })
  })

  it('detects default key conflict and filters active keys', () => {
    expect(isDefaultChatAPIKeyRequired({ code: 'LOBEHUB_DEFAULT_CHAT_API_KEY_REQUIRED' })).toBe(true)
    expect(isDefaultChatAPIKeyRequired({ status: 409 })).toBe(true)
    expect(filterActiveLobeHubKeys([
      apiKey(1, 'active'),
      apiKey(2, 'inactive'),
      apiKey(3, 'quota_exhausted'),
      apiKey(4, 'expired')
    ])).toEqual([apiKey(1, 'active')])
  })
})
