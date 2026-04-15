import { describe, expect, it } from 'vitest'

import type { ApiKey } from '@/types'
import {
  buildLobeHubContinuationPayload,
  buildLobeHubSelectKeyRouteQuery,
  filterSelectableLobeHubKeys,
  isLobeHubDefaultKeyRequiredError,
  parseLobeHubContinuationQuery
} from '../lobehubFlow'

describe('lobehubFlow helpers', () => {
  it('parses resume and refresh-target query values', () => {
    expect(
      parseLobeHubContinuationQuery({
        resume: ['resume-1'],
        return_url: '/workspace',
        mode: 'refresh-target'
      })
    ).toEqual({
      resumeToken: 'resume-1',
      returnURL: '/workspace',
      mode: 'refresh-target'
    })

    expect(parseLobeHubContinuationQuery({})).toEqual({
      resumeToken: '',
      returnURL: '',
      mode: 'oidc'
    })
  })

  it('builds continuation payloads and preserves optional api key selection', () => {
    expect(
      buildLobeHubContinuationPayload(
        {
          resume: 'resume-1',
          return_url: 'https://chat.example.com/workspace'
        },
        88
      )
    ).toEqual({
      resume_token: 'resume-1',
      return_url: 'https://chat.example.com/workspace',
      api_key_id: 88
    })

    expect(
      buildLobeHubContinuationPayload({
        return_url: '/workspace',
        mode: 'refresh-target'
      })
    ).toEqual({
      return_url: '/workspace',
      mode: 'refresh-target'
    })
  })

  it('detects the default-key-required API error and keeps only active keys selectable', () => {
    expect(
      isLobeHubDefaultKeyRequiredError({
        code: 'LOBEHUB_DEFAULT_CHAT_API_KEY_REQUIRED'
      })
    ).toBe(true)
    expect(
      isLobeHubDefaultKeyRequiredError({
        message: 'fallback'
      })
    ).toBe(false)

    const baseKey = {
      id: 1,
      user_id: 1,
      key: 'sk-test',
      name: 'Test',
      group_id: null,
      ip_whitelist: [],
      ip_blacklist: [],
      last_used_at: null,
      quota: 0,
      quota_used: 0,
      expires_at: null,
      created_at: '2026-04-07T00:00:00Z',
      updated_at: '2026-04-07T00:00:00Z',
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
    } satisfies Omit<ApiKey, 'status'>

    expect(
      filterSelectableLobeHubKeys([
        { ...baseKey, status: 'active' },
        { ...baseKey, id: 2, status: 'inactive' },
        { ...baseKey, id: 3, status: 'quota_exhausted' }
      ])
    ).toEqual([{ ...baseKey, status: 'active' }])
  })

  it('rebuilds select-key query without losing continuation params', () => {
    expect(
      buildLobeHubSelectKeyRouteQuery({
        resume: 'resume-1',
        return_url: '/workspace',
        mode: 'refresh-target'
      })
    ).toEqual({
      resume: 'resume-1',
      return_url: '/workspace',
      mode: 'refresh-target'
    })
  })
})
