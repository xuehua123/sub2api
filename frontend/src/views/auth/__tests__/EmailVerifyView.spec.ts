import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import EmailVerifyView from '@/views/auth/EmailVerifyView.vue'

const {
  pushMock,
  showSuccessMock,
  showErrorMock,
  registerMock,
  setTokenMock,
  setPendingAuthSessionMock,
  clearPendingAuthSessionMock,
  getPublicSettingsMock,
  sendVerifyCodeMock,
  sendPendingOAuthVerifyCodeMock,
  persistOAuthTokenContextMock,
  apiClientPostMock,
  authStoreState,
  createTurnstileResetMock,
  verifyActionMock,
} = vi.hoisted(() => ({
  pushMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showErrorMock: vi.fn(),
  registerMock: vi.fn(),
  setTokenMock: vi.fn(),
  setPendingAuthSessionMock: vi.fn(),
  clearPendingAuthSessionMock: vi.fn(),
  getPublicSettingsMock: vi.fn(),
  sendVerifyCodeMock: vi.fn(),
  sendPendingOAuthVerifyCodeMock: vi.fn(),
  persistOAuthTokenContextMock: vi.fn(),
  apiClientPostMock: vi.fn(),
  createTurnstileResetMock: vi.fn(),
  verifyActionMock: vi.fn(),
  authStoreState: {
    pendingAuthSession: null as null | {
      token: string
      token_field: 'pending_auth_token' | 'pending_oauth_token'
      provider: string
      redirect?: string
      adoption_required?: boolean
      suggested_display_name?: string
      suggested_avatar_url?: string
    }
  },
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: pushMock,
  }),
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: {
      t: (key: string) => key,
    },
  }),
  useI18n: () => ({
    t: (key: string, params?: Record<string, string | number>) => {
      if (key === 'auth.accountCreatedSuccess') {
        return `Account created for ${params?.siteName ?? 'Sub2API'}`
      }
      if (key === 'auth.emailDomainRegistrationLimit') {
        return '该邮箱域名无法注册新账户。请使用主流邮箱注册；如需使用企业邮箱，请联系客服添加域名白名单。'
      }
      return key
    },
    locale: { value: 'en' },
  }),
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    pendingAuthSession: authStoreState.pendingAuthSession,
    register: (...args: any[]) => registerMock(...args),
    setToken: (...args: any[]) => setTokenMock(...args),
    setPendingAuthSession: (...args: any[]) => setPendingAuthSessionMock(...args),
    clearPendingAuthSession: (...args: any[]) => clearPendingAuthSessionMock(...args),
  }),
  useAppStore: () => ({
    showSuccess: (...args: any[]) => showSuccessMock(...args),
    showError: (...args: any[]) => showErrorMock(...args),
  }),
}))

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    getPublicSettings: (...args: any[]) => getPublicSettingsMock(...args),
    sendVerifyCode: (...args: any[]) => sendVerifyCodeMock(...args),
    sendPendingOAuthVerifyCode: (...args: any[]) => sendPendingOAuthVerifyCodeMock(...args),
    persistOAuthTokenContext: (...args: any[]) => persistOAuthTokenContextMock(...args),
  }
})

vi.mock('@/api/client', () => ({
  apiClient: {
    post: (...args: any[]) => apiClientPostMock(...args),
  },
}))

describe('EmailVerifyView', () => {
  function configurePendingOAuthCaptcha(
    settings: Record<string, unknown>,
    initialProof: Record<string, string>,
  ): void {
    authStoreState.pendingAuthSession = {
      token: 'pending-token-captcha',
      token_field: 'pending_auth_token',
      provider: 'oidc',
      redirect: '/profile',
    }
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: [],
      ...settings,
    })
    sendPendingOAuthVerifyCodeMock.mockResolvedValue({ countdown: 0 })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
        ...initialProof,
      }),
    )
  }

  function mountTrackedCaptcha(clickBehavior: 'none' | 'verify' | 'error' = 'none') {
    let captchaMountCount = 0
    const CaptchaChallengeStub = defineComponent({
      props: {
        tencentRegion: String,
      },
      emits: ['verify', 'error'],
      setup(_, { emit, expose }) {
        const mountId = ++captchaMountCount
        expose({ verifyAction: verifyActionMock, reset: createTurnstileResetMock })
        return () =>
          h(
            clickBehavior === 'none' ? 'div' : 'button',
            {
              'data-captcha-mount-id': mountId,
              onClick:
                clickBehavior === 'verify'
                  ? () => emit('verify', `turnstile-proof-${mountId}`)
                  : clickBehavior === 'error'
                    ? () => emit('error')
                    : undefined,
            },
            clickBehavior,
          )
      },
    })
    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: CaptchaChallengeStub,
          transition: false,
        },
      },
    })

    return { CaptchaChallengeStub, wrapper }
  }

  beforeEach(() => {
    pushMock.mockReset()
    showSuccessMock.mockReset()
    showErrorMock.mockReset()
    registerMock.mockReset()
    setTokenMock.mockReset()
    setPendingAuthSessionMock.mockReset()
    clearPendingAuthSessionMock.mockReset()
    getPublicSettingsMock.mockReset()
    sendVerifyCodeMock.mockReset()
    sendPendingOAuthVerifyCodeMock.mockReset()
    persistOAuthTokenContextMock.mockReset()
    apiClientPostMock.mockReset()
    createTurnstileResetMock.mockReset()
    verifyActionMock.mockReset()
    authStoreState.pendingAuthSession = null
    sessionStorage.clear()
    localStorage.clear()

    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: [],
    })
    sendVerifyCodeMock.mockResolvedValue({ countdown: 60 })
    sendPendingOAuthVerifyCodeMock.mockResolvedValue({ countdown: 60 })
    setTokenMock.mockResolvedValue({})
  })

  it('temporarily swaps the pending oauth create captcha for the Tencent INTL resend captcha', async () => {
    configurePendingOAuthCaptcha(
      {
        tencent_captcha_enabled: true,
        tencent_captcha_app_id: 'tencent-app-id',
        tencent_captcha_region: 'intl',
      },
      {
        tencent_captcha_ticket: 'initial-ticket',
        tencent_captcha_randstr: '@initial-rand',
      },
    )
    let resolveResendProof!: (proof: { token: string; randstr: string } | null) => void
    verifyActionMock.mockReturnValue(
      new Promise((resolve) => {
        resolveResendProof = resolve
      }),
    )
    const { CaptchaChallengeStub, wrapper } = mountTrackedCaptcha()

    await flushPromises()
    expect(wrapper.findAllComponents(CaptchaChallengeStub)).toHaveLength(1)
    expect(wrapper.findComponent(CaptchaChallengeStub).props('tencentRegion')).toBe('intl')
    expect(wrapper.get('[data-captcha-mount-id="1"]').exists()).toBe(true)

    const resendButton = wrapper.findAll('button').find((button) =>
      button.text().includes('auth.clickToResend'),
    )!
    await resendButton.trigger('click')
    await flushPromises()

    expect(verifyActionMock).toHaveBeenCalledTimes(1)
    expect(wrapper.findAllComponents(CaptchaChallengeStub)).toHaveLength(1)
    expect(wrapper.get('[data-captcha-mount-id="2"]').exists()).toBe(true)

    resolveResendProof(null)
    await flushPromises()

    expect(wrapper.findAllComponents(CaptchaChallengeStub)).toHaveLength(1)
    expect(wrapper.get('[data-captcha-mount-id="3"]').exists()).toBe(true)
    expect(sendPendingOAuthVerifyCodeMock).toHaveBeenCalledTimes(1)
  })

  it.each([
    {
      captchaName: 'Tencent CN',
      settings: {
        tencent_captcha_enabled: true,
        tencent_captcha_app_id: 'tencent-app-id',
        tencent_captcha_region: 'cn',
      },
      initialProof: {
        tencent_captcha_ticket: 'initial-ticket',
        tencent_captcha_randstr: '@initial-rand',
      },
      resendProof: { token: 'resend-ticket', randstr: '@resend-rand' },
      expectedPayload: {
        tencent_captcha_ticket: 'resend-ticket',
        tencent_captcha_randstr: '@resend-rand',
      },
    },
    {
      captchaName: 'Aliyun',
      settings: {
        aliyun_captcha_enabled: true,
        aliyun_captcha_scene_id: 'aliyun-scene',
        aliyun_captcha_prefix: 'aliyun-prefix',
        aliyun_captcha_region: 'sgp',
      },
      initialProof: {
        turnstile_token: 'initial-aliyun-proof',
      },
      resendProof: { token: 'resend-aliyun-proof', randstr: '' },
      expectedPayload: {
        turnstile_token: 'resend-aliyun-proof',
      },
    },
  ])(
    'restores the pending oauth create captcha after a $captchaName resend succeeds',
    async ({ settings, initialProof, resendProof, expectedPayload }) => {
      configurePendingOAuthCaptcha(settings, initialProof)
      verifyActionMock.mockResolvedValue(resendProof)
      const { CaptchaChallengeStub, wrapper } = mountTrackedCaptcha()

      await flushPromises()
      expect(wrapper.findAllComponents(CaptchaChallengeStub)).toHaveLength(1)

      const resendButton = wrapper.findAll('button').find((button) =>
        button.text().includes('auth.clickToResend'),
      )!
      await resendButton.trigger('click')
      await flushPromises()

      expect(sendPendingOAuthVerifyCodeMock).toHaveBeenNthCalledWith(
        2,
        expect.objectContaining(expectedPayload),
      )
      expect(wrapper.findAllComponents(CaptchaChallengeStub)).toHaveLength(1)
      expect(wrapper.get('[data-captcha-mount-id="3"]').exists()).toBe(true)
    },
  )

  it('keeps the pending oauth Turnstile resend staged and restores the create captcha after send', async () => {
    configurePendingOAuthCaptcha(
      {
        turnstile_enabled: true,
        turnstile_site_key: 'turnstile-site-key',
      },
      { turnstile_token: 'initial-turnstile-proof' },
    )
    const { CaptchaChallengeStub, wrapper } = mountTrackedCaptcha('verify')

    await flushPromises()
    expect(wrapper.findAllComponents(CaptchaChallengeStub)).toHaveLength(1)
    expect(wrapper.get('[data-captcha-mount-id="1"]').exists()).toBe(true)

    const resendButton = wrapper.findAll('button').find((button) =>
      button.text().includes('auth.clickToResend'),
    )!
    await resendButton.trigger('click')
    await flushPromises()

    expect(sendPendingOAuthVerifyCodeMock).toHaveBeenCalledTimes(1)
    expect(wrapper.findAllComponents(CaptchaChallengeStub)).toHaveLength(1)
    expect(wrapper.get('[data-captcha-mount-id="2"]').exists()).toBe(true)

    await wrapper.get('[data-captcha-mount-id="2"]').trigger('click')
    await resendButton.trigger('click')
    await flushPromises()

    expect(sendPendingOAuthVerifyCodeMock).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({ turnstile_token: 'turnstile-proof-2' }),
    )
    expect(wrapper.findAllComponents(CaptchaChallengeStub)).toHaveLength(1)
    expect(wrapper.get('[data-captcha-mount-id="3"]').exists()).toBe(true)
  })

  it('restores the pending oauth create captcha when Turnstile resend proof fails', async () => {
    configurePendingOAuthCaptcha(
      {
        turnstile_enabled: true,
        turnstile_site_key: 'turnstile-site-key',
      },
      { turnstile_token: 'initial-turnstile-proof' },
    )
    const { CaptchaChallengeStub, wrapper } = mountTrackedCaptcha('error')

    await flushPromises()
    const resendButton = wrapper.findAll('button').find((button) =>
      button.text().includes('auth.clickToResend'),
    )!
    await resendButton.trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-captcha-mount-id="2"]').exists()).toBe(true)

    await wrapper.get('[data-captcha-mount-id="2"]').trigger('click')
    await flushPromises()

    expect(showErrorMock).toHaveBeenCalledWith('auth.turnstileFailed')
    expect(sendPendingOAuthVerifyCodeMock).toHaveBeenCalledTimes(1)
    expect(wrapper.findAllComponents(CaptchaChallengeStub)).toHaveLength(1)
    expect(wrapper.get('[data-captcha-mount-id="3"]').exists()).toBe(true)
    expect(resendButton.attributes('disabled')).toBeUndefined()
  })

  it('acquires a fresh Tencent proof for each resend action', async () => {
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      tencent_captcha_enabled: true,
      tencent_captcha_app_id: 'tencent-app-id',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: [],
    })
    sendVerifyCodeMock.mockResolvedValue({ countdown: 0 })
    verifyActionMock
      .mockResolvedValueOnce({ token: 'ticket-1', randstr: '@rand-1' })
      .mockResolvedValueOnce({ token: 'ticket-2', randstr: '@rand-2' })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
        tencent_captcha_ticket: 'initial-ticket',
        tencent_captcha_randstr: '@initial-rand',
      })
    )

    const CaptchaChallengeStub = defineComponent({
      setup(_, { expose }) {
        expose({ verifyAction: verifyActionMock, reset: createTurnstileResetMock })
        return () => h('div')
      },
    })
    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: CaptchaChallengeStub,
          transition: false,
        },
      },
    })

    await flushPromises()
    const resendButton = () => wrapper.findAll('button').find((button) =>
      button.text().includes('auth.clickToResend')
    )!

    await resendButton().trigger('click')
    await flushPromises()
    await resendButton().trigger('click')
    await flushPromises()

    expect(verifyActionMock).toHaveBeenCalledTimes(2)
    expect(sendVerifyCodeMock).toHaveBeenNthCalledWith(2, expect.objectContaining({
      tencent_captcha_ticket: 'ticket-1',
      tencent_captcha_randstr: '@rand-1',
    }))
    expect(sendVerifyCodeMock).toHaveBeenNthCalledWith(3, expect.objectContaining({
      tencent_captcha_ticket: 'ticket-2',
      tencent_captcha_randstr: '@rand-2',
    }))
  })

  it('uses the pending oauth verify-code endpoint when register data carries a pending auth session', async () => {
    authStoreState.pendingAuthSession = {
      token: 'pending-token-1',
      token_field: 'pending_auth_token',
      provider: 'wechat',
      redirect: '/profile',
    }
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
      })
    )

    mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()

    expect(sendPendingOAuthVerifyCodeMock).toHaveBeenCalledWith({
      email: 'fresh@example.com',
      pending_auth_token: 'pending-token-1',
    })
    expect(sendVerifyCodeMock).not.toHaveBeenCalled()
  })

  it('requires a fresh captcha proof after the initial send-code request fails', async () => {
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: true,
      turnstile_site_key: 'site-key',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: [],
    })
    sendVerifyCodeMock.mockRejectedValue(new Error('send failed'))
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
        turnstile_token: 'initial-proof',
      })
    )

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: {
            template: '<button data-testid="resend-captcha" @click="$emit(\'verify\', \'fresh-proof\')">verify</button>',
          },
          transition: false,
        },
      },
    })

    await flushPromises()

    expect(sendVerifyCodeMock).toHaveBeenCalledWith(expect.objectContaining({
      turnstile_token: 'initial-proof',
    }))
    expect(wrapper.find('[data-testid="resend-captcha"]').exists()).toBe(true)
  })

  it('skips the registration email suffix whitelist for pending oauth verification', async () => {
    authStoreState.pendingAuthSession = {
      token: 'pending-token-2',
      token_field: 'pending_auth_token',
      provider: 'oidc',
      redirect: '/profile',
    }
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
    })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
      })
    )

    mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()

    expect(sendPendingOAuthVerifyCodeMock).toHaveBeenCalledWith({
      email: 'fresh@example.com',
      pending_auth_token: 'pending-token-2',
    })
    expect(showErrorMock).not.toHaveBeenCalled()
  })

  it('sends a verification code for a non-whitelist email domain', async () => {
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
      registration_email_domain_quota_enabled: true,
    })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'first@custom.example',
        password: 'secret-123',
      })
    )

    mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()

    expect(sendVerifyCodeMock).toHaveBeenCalledWith(
      expect.objectContaining({ email: 'first@custom.example' })
    )
    expect(showErrorMock).not.toHaveBeenCalled()
  })

  it('shows the localized domain quota message when sending a verification code is rejected', async () => {
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
      registration_email_domain_quota_enabled: true,
    })
    sendVerifyCodeMock.mockRejectedValueOnce({
      reason: 'EMAIL_DOMAIN_REGISTRATION_LIMIT',
      message: 'raw backend message',
    })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'second@custom.example',
        password: 'secret-123',
      })
    )

    mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()

    expect(showErrorMock).toHaveBeenLastCalledWith(
      '该邮箱域名无法注册新账户。请使用主流邮箱注册；如需使用企业邮箱，请联系客服添加域名白名单。'
    )
  })

  it('shows the localized domain quota message when verified registration is rejected', async () => {
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
      registration_email_domain_quota_enabled: true,
    })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'second@custom.example',
        password: 'secret-123',
      })
    )
    registerMock.mockRejectedValueOnce({
      reason: 'EMAIL_DOMAIN_REGISTRATION_LIMIT',
      message: 'raw backend message',
    })

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()
    await wrapper.get('#code').setValue('123456')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).toHaveBeenCalled()
    expect(showErrorMock).toHaveBeenLastCalledWith(
      '该邮箱域名无法注册新账户。请使用主流邮箱注册；如需使用企业邮箱，请联系客服添加域名白名单。'
    )
  })

  // 域名限量注册开关默认关闭：恢复 PR5423 之前的客户端白名单预检，非白名单域名不发送验证码。
  it('blocks sending a verification code for a non-whitelist email domain when the quota switch is disabled', async () => {
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
    })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'first@custom.example',
        password: 'secret-123',
      })
    )

    mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()

    expect(sendVerifyCodeMock).not.toHaveBeenCalled()
    expect(showErrorMock).toHaveBeenCalledWith('auth.emailSuffixNotAllowedWithAllowed')
  })

  it('uses the pending oauth verify-code endpoint when auth store only carries the pending provider', async () => {
    authStoreState.pendingAuthSession = {
      token: '',
      token_field: 'pending_oauth_token',
      provider: 'oidc',
      redirect: '/profile',
    }
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
    })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
      })
    )

    mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()

    expect(sendPendingOAuthVerifyCodeMock).toHaveBeenCalledWith({
      email: 'fresh@example.com',
      pending_oauth_token: undefined,
    })
    expect(sendVerifyCodeMock).not.toHaveBeenCalled()
    expect(showErrorMock).not.toHaveBeenCalled()
  })

  it('returns to the oauth callback flow when pending send-code detects an existing account email', async () => {
    authStoreState.pendingAuthSession = {
      token: '',
      token_field: 'pending_oauth_token',
      provider: 'oidc',
      redirect: '/profile/security',
    }
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
    })
    sendPendingOAuthVerifyCodeMock.mockResolvedValue({
      auth_result: 'pending_session',
      provider: 'oidc',
      redirect: '/profile/security',
    })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
      })
    )

    mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()

    expect(setPendingAuthSessionMock).toHaveBeenCalledWith({
      token: '',
      token_field: 'pending_oauth_token',
      provider: 'oidc',
      redirect: '/profile/security',
    })
    expect(pushMock).toHaveBeenCalledWith('/auth/oidc/callback')
    expect(showErrorMock).not.toHaveBeenCalled()
  })

  it('submits pending auth account creation when session storage has no pending metadata but auth store does', async () => {
    authStoreState.pendingAuthSession = {
      token: 'pending-token-1',
      token_field: 'pending_auth_token',
      provider: 'wechat',
      redirect: '/profile',
    }
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
        aff_code: 'AFF123',
      })
    )
    apiClientPostMock.mockResolvedValue({
      data: {
        access_token: 'oauth-access-token',
        refresh_token: 'oauth-refresh-token',
        expires_in: 3600,
        token_type: 'Bearer',
      },
    })

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()
    await wrapper.get('#code').setValue('123456')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(apiClientPostMock).toHaveBeenCalledWith('/auth/oauth/pending/create-account', {
      email: 'fresh@example.com',
      password: 'secret-123',
      verify_code: '123456',
      aff_code: 'AFF123',
    })
    expect(persistOAuthTokenContextMock).toHaveBeenCalledWith({
      access_token: 'oauth-access-token',
      refresh_token: 'oauth-refresh-token',
      expires_in: 3600,
      token_type: 'Bearer',
    })
    expect(setTokenMock).toHaveBeenCalledWith('oauth-access-token')
    expect(clearPendingAuthSessionMock).toHaveBeenCalled()
    expect(pushMock).toHaveBeenCalledWith('/profile')
    expect(registerMock).not.toHaveBeenCalled()
  })

  it('requires and submits a fresh turnstile token for pending oauth account creation', async () => {
    authStoreState.pendingAuthSession = {
      token: 'pending-token-3',
      token_field: 'pending_auth_token',
      provider: 'oidc',
      redirect: '/profile',
    }
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: true,
      turnstile_site_key: 'site-key',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
    })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
        turnstile_token: 'send-code-token',
      })
    )
    apiClientPostMock.mockResolvedValue({
      data: {
        access_token: 'oauth-access-token',
        refresh_token: 'oauth-refresh-token',
        expires_in: 3600,
        token_type: 'Bearer',
      },
    })

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: {
            template: '<button data-testid="create-turnstile" @click="$emit(\'verify\', \'create-token\')">verify</button>',
            methods: {
              reset: createTurnstileResetMock,
            },
          },
          transition: false,
        },
      },
    })

    await flushPromises()

    expect(sendPendingOAuthVerifyCodeMock).toHaveBeenCalledWith({
      email: 'fresh@example.com',
      pending_auth_token: 'pending-token-3',
      turnstile_token: 'send-code-token',
    })

    await wrapper.get('#code').setValue('123456')
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-testid="create-turnstile"]').trigger('click')
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeUndefined()

    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(apiClientPostMock).toHaveBeenCalledWith('/auth/oauth/pending/create-account', {
      email: 'fresh@example.com',
      password: 'secret-123',
      verify_code: '123456',
      turnstile_token: 'create-token',
    })
    expect(setTokenMock).toHaveBeenCalledWith('oauth-access-token')
  })

  it('resets the pending oauth create-account turnstile after submit failure', async () => {
    authStoreState.pendingAuthSession = {
      token: 'pending-token-4',
      token_field: 'pending_auth_token',
      provider: 'oidc',
      redirect: '/profile',
    }
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: true,
      turnstile_site_key: 'site-key',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
    })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
        turnstile_token: 'send-code-token',
      })
    )
    apiClientPostMock.mockRejectedValue(new Error('invalid verify code'))

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: {
            template: '<button data-testid="create-turnstile" @click="$emit(\'verify\', \'create-token\')">verify</button>',
            methods: {
              reset: createTurnstileResetMock,
            },
          },
          transition: false,
        },
      },
    })

    await flushPromises()
    await wrapper.get('#code').setValue('123456')
    await wrapper.get('[data-testid="create-turnstile"]').trigger('click')
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeUndefined()

    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(apiClientPostMock).toHaveBeenCalledWith('/auth/oauth/pending/create-account', {
      email: 'fresh@example.com',
      password: 'secret-123',
      verify_code: '123456',
      turnstile_token: 'create-token',
    })
    expect(createTurnstileResetMock).toHaveBeenCalled()
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
  })

  it('returns to the oauth callback flow when pending account creation becomes bind-login', async () => {
    authStoreState.pendingAuthSession = {
      token: '',
      token_field: 'pending_oauth_token',
      provider: 'oidc',
      redirect: '/profile/security',
    }
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
    })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
      })
    )
    apiClientPostMock.mockResolvedValue({
      data: {
        auth_result: 'pending_session',
        provider: 'oidc',
        step: 'bind_login_required',
        redirect: '/profile/security',
        email: 'fresh@example.com',
      },
    })

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()
    await wrapper.get('#code').setValue('123456')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(apiClientPostMock).toHaveBeenCalledWith('/auth/oauth/pending/create-account', {
      email: 'fresh@example.com',
      password: 'secret-123',
      verify_code: '123456',
    })
    expect(setPendingAuthSessionMock).toHaveBeenCalledWith({
      token: '',
      token_field: 'pending_oauth_token',
      provider: 'oidc',
      redirect: '/profile/security',
    })
    expect(pushMock).toHaveBeenCalledWith('/auth/oidc/callback')
    expect(setTokenMock).not.toHaveBeenCalled()
    expect(persistOAuthTokenContextMock).not.toHaveBeenCalled()
    expect(clearPendingAuthSessionMock).not.toHaveBeenCalled()
    expect(showSuccessMock).not.toHaveBeenCalled()
  })

  it('keeps the normal email registration flow unchanged', async () => {
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'normal@example.com',
        password: 'secret-456',
        promo_code: 'PROMO',
        invitation_code: 'INVITE',
      })
    )
    registerMock.mockResolvedValue({})

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()
    await wrapper.get('#code').setValue('654321')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).toHaveBeenCalledWith({
      email: 'normal@example.com',
      password: 'secret-456',
      verify_code: '654321',
      turnstile_token: undefined,
      tencent_captcha_ticket: undefined,
      tencent_captcha_randstr: undefined,
      promo_code: 'PROMO',
      invitation_code: 'INVITE',
    })
    expect(apiClientPostMock).not.toHaveBeenCalled()
    expect(pushMock).toHaveBeenCalledWith('/dashboard')
  })

  it('preserves referral code through email verification registration', async () => {
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'invitee@example.com',
        password: 'secret-456',
        referral_code: 'REF12345',
      })
    )
    registerMock.mockResolvedValue({})

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()
    await wrapper.get('#code').setValue('654321')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).toHaveBeenCalledWith(expect.objectContaining({
      email: 'invitee@example.com',
      password: 'secret-456',
      verify_code: '654321',
      referral_code: 'REF12345',
    }))
  })

  it('does not require another Tencent proof for final email registration', async () => {
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      tencent_captcha_enabled: true,
      tencent_captcha_app_id: 'tencent-app-id',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: [],
    })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'normal@example.com',
        password: 'secret-456',
        tencent_captcha_ticket: 'send-code-ticket',
        tencent_captcha_randstr: '@send-code-rand',
      })
    )
    registerMock.mockResolvedValue({})

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: {
            template: '<span />',
            methods: {
              reset: createTurnstileResetMock,
            },
          },
          transition: false,
        },
      },
    })

    await flushPromises()
    expect(sendVerifyCodeMock).toHaveBeenCalledWith(expect.objectContaining({
      tencent_captcha_ticket: 'send-code-ticket',
      tencent_captcha_randstr: '@send-code-rand',
    }))
    expect(JSON.parse(sessionStorage.getItem('register_data') || '{}')).toEqual({
      email: 'normal@example.com',
      password: 'secret-456',
    })

    await wrapper.get('#code').setValue('654321')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).toHaveBeenCalledWith(expect.objectContaining({
      email: 'normal@example.com',
      verify_code: '654321',
      tencent_captcha_ticket: undefined,
      tencent_captcha_randstr: undefined,
    }))
  })
})
