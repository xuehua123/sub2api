import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const {
  routeState,
  createOIDCWebSession,
  listKeys,
  showError,
  routerPush,
  routerReplace,
  assign,
  authStoreState
} = vi.hoisted(() => ({
  routeState: {
    query: {} as Record<string, string>,
    fullPath: ''
  },
  createOIDCWebSession: vi.fn(),
  listKeys: vi.fn(),
  showError: vi.fn(),
  routerPush: vi.fn(),
  routerReplace: vi.fn(),
  assign: vi.fn(),
  authStoreState: {
    user: {
      id: 1,
      username: 'alice',
      email: 'alice@example.com',
      role: 'user' as const,
      balance: 0,
      concurrency: 1,
      status: 'active' as const,
      default_chat_api_key_id: null,
      allowed_groups: null,
      created_at: '2026-04-07T00:00:00Z',
      updated_at: '2026-04-07T00:00:00Z'
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({
    push: routerPush,
    replace: routerReplace
  })
}))

vi.mock('@/api/lobehub', () => ({
  createLobeHubOIDCWebSession: createOIDCWebSession
}))

vi.mock('@/api', () => ({
  keysAPI: {
    list: listKeys
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError
  }),
  useAuthStore: () => authStoreState
}))

import LobeHubSSOView from '../LobeHubSSOView.vue'
import LobeHubSelectKeyView from '../LobeHubSelectKeyView.vue'

const authLayoutStub = {
  template: '<div><slot /><slot name="footer" /></div>'
}

describe('LobeHub auth views', () => {
  const originalLocation = window.location

  beforeEach(() => {
    vi.clearAllMocks()
    routeState.query = {}
    routeState.fullPath = ''
    authStoreState.user.default_chat_api_key_id = null

    Object.defineProperty(window, 'location', {
      configurable: true,
      value: {
        ...originalLocation,
        assign
      },
      writable: true
    })
  })

  afterEach(() => {
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: originalLocation,
      writable: true
    })
  })

  it('continues the OIDC resume flow and redirects to the returned continue_url', async () => {
    routeState.query = {
      resume: 'resume-1',
      return_url: 'https://chat.example.com/workspace'
    }
    routeState.fullPath = '/auth/lobehub-sso?resume=resume-1&return_url=https://chat.example.com/workspace'
    createOIDCWebSession.mockResolvedValue({
      continue_url: 'https://api.example.com/api/v1/lobehub/oidc/authorize?state=test'
    })

    mount(LobeHubSSOView, {
      global: {
        stubs: {
          AuthLayout: authLayoutStub,
          Icon: true
        }
      }
    })
    await flushPromises()

    expect(createOIDCWebSession).toHaveBeenCalledWith({
      resume_token: 'resume-1',
      return_url: 'https://chat.example.com/workspace'
    })
    expect(assign).toHaveBeenCalledWith(
      'https://api.example.com/api/v1/lobehub/oidc/authorize?state=test'
    )
  })

  it('routes to key selection when the backend requires a default chat key', async () => {
    routeState.query = {
      resume: 'resume-2',
      return_url: 'https://chat.example.com/chat'
    }
    routeState.fullPath = '/auth/lobehub-sso?resume=resume-2&return_url=https://chat.example.com/chat'
    createOIDCWebSession.mockRejectedValue({
      status: 409,
      code: 'LOBEHUB_DEFAULT_CHAT_API_KEY_REQUIRED'
    })

    mount(LobeHubSSOView, {
      global: {
        stubs: {
          AuthLayout: authLayoutStub,
          Icon: true
        }
      }
    })
    await flushPromises()

    expect(routerReplace).toHaveBeenCalledWith({
      name: 'LobeHubSelectKey',
      query: {
        resume: 'resume-2',
        return_url: 'https://chat.example.com/chat'
      }
    })
    expect(assign).not.toHaveBeenCalled()
  })

  it('loads selectable keys, persists the chosen default key locally, and resumes continuation', async () => {
    routeState.query = {
      resume: 'resume-3',
      return_url: 'https://chat.example.com/chat'
    }
    routeState.fullPath = '/auth/lobehub-select-key?resume=resume-3&return_url=https://chat.example.com/chat'
    listKeys.mockResolvedValue({
      items: [
        {
          id: 7,
          user_id: 1,
          key: 'sk-live',
          name: 'Main Key',
          group_id: null,
          status: 'active',
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
        },
        {
          id: 9,
          user_id: 1,
          key: 'sk-disabled',
          name: 'Disabled Key',
          group_id: null,
          status: 'inactive',
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
        }
      ],
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1
    })
    createOIDCWebSession.mockResolvedValue({
      continue_url: 'https://api.example.com/api/v1/lobehub/oidc/authorize?state=test-2'
    })

    const wrapper = mount(LobeHubSelectKeyView, {
      global: {
        stubs: {
          AuthLayout: authLayoutStub,
          Icon: true
        }
      }
    })
    await flushPromises()

    expect(wrapper.text()).toContain('Main Key')
    expect(wrapper.text()).not.toContain('Disabled Key')

    await wrapper.get('[data-testid="lobehub-key-option-7"]').trigger('click')
    await wrapper.get('[data-testid="lobehub-select-key-submit"]').trigger('click')
    await flushPromises()

    expect(createOIDCWebSession).toHaveBeenCalledWith({
      resume_token: 'resume-3',
      return_url: 'https://chat.example.com/chat',
      api_key_id: 7
    })
    expect(authStoreState.user.default_chat_api_key_id).toBe(7)
    expect(assign).toHaveBeenCalledWith(
      'https://api.example.com/api/v1/lobehub/oidc/authorize?state=test-2'
    )
  })

  it('sends users without active keys to the keys page with a continuation redirect', async () => {
    routeState.query = {
      resume: 'resume-4',
      return_url: 'https://chat.example.com/chat'
    }
    routeState.fullPath = '/auth/lobehub-select-key?resume=resume-4&return_url=https://chat.example.com/chat'
    listKeys.mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      page_size: 20,
      pages: 1
    })

    const wrapper = mount(LobeHubSelectKeyView, {
      global: {
        stubs: {
          AuthLayout: authLayoutStub,
          Icon: true
        }
      }
    })
    await flushPromises()

    await wrapper.get('button').trigger('click')

    expect(routerPush).toHaveBeenCalledWith({
      path: '/keys',
      query: {
        openCreate: '1',
        redirect: '/auth/lobehub-select-key?resume=resume-4&return_url=https://chat.example.com/chat'
      }
    })
  })
})
