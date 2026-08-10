import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import RegisterView from '../RegisterView.vue'

const {
  push,
  showSuccess,
  showError,
  register,
  getPublicSettings,
  isWeChatWebOAuthEnabled,
  validatePromoCode,
  validateInvitationCode,
  validateReferralCode,
  routeQuery
} = vi.hoisted(() => ({
  push: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
  register: vi.fn(),
  getPublicSettings: vi.fn(),
  isWeChatWebOAuthEnabled: vi.fn(),
  validatePromoCode: vi.fn(),
  validateInvitationCode: vi.fn(),
  validateReferralCode: vi.fn(),
  routeQuery: { value: {} as Record<string, unknown> }
}))

const t = (key: string, params?: Record<string, unknown>) => {
  if (key === 'auth.signUpToStart') {
    return `Sign up to start ${String(params?.siteName ?? '')}`
  }
  if (key === 'auth.emailDomainRegistrationLimit') {
    return '该邮箱域名无法注册新账户。请使用主流邮箱注册；如需使用企业邮箱，请联系客服添加域名白名单。'
  }
  return key
}

vi.mock('vue-router', () => ({
  useRouter: () => ({ push }),
  useRoute: () => ({ query: routeQuery.value })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t,
      locale: { value: 'en' }
    })
  }
})

vi.mock('@/stores', () => ({
  useAuthStore: () => ({ register }),
  useAppStore: () => ({ showSuccess, showError })
}))

vi.mock('@/api/auth', () => ({
  getPublicSettings,
  isWeChatWebOAuthEnabled,
  validatePromoCode,
  validateInvitationCode,
  validateReferralCode
}))

vi.mock('@/utils/authError', () => ({
  buildAuthErrorMessage: () => 'registration failed'
}))

const baseSettings = {
  registration_enabled: true,
  email_verify_enabled: false,
  registration_email_suffix_whitelist: [],
  registration_email_domain_quota_enabled: false,
  promo_code_enabled: false,
  password_reset_enabled: false,
  invitation_code_enabled: false,
  turnstile_enabled: false,
  turnstile_site_key: '',
  site_name: 'Sub2API',
  site_logo: '',
  site_subtitle: '',
  api_base_url: '',
  contact_info: '',
  doc_url: '',
  home_content: '',
  hide_ccs_import_button: false,
  payment_enabled: false,
  table_default_page_size: 20,
  table_page_size_options: [10, 20, 50, 100],
  custom_menu_items: [],
  custom_endpoints: [],
  linuxdo_oauth_enabled: false,
  oidc_oauth_enabled: false,
  oidc_oauth_provider_name: 'OIDC',
  backend_mode_enabled: false,
  version: '1.0.0',
  referral_enabled: true,
  referral_allow_manual_input: false,
  referral_bind_before_first_paid_only: true,
  referral_withdraw_enabled: false,
  referral_credit_conversion_enabled: false,
  referral_credit_conversion_rate: 1,
  referral_settlement_currency: 'CNY',
  referral_withdraw_methods_enabled: []
}

describe('RegisterView referral input visibility', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    routeQuery.value = {}
    getPublicSettings.mockResolvedValue({
      ...baseSettings
    })
    isWeChatWebOAuthEnabled.mockReturnValue(false)
    validatePromoCode.mockResolvedValue({ valid: true })
    validateInvitationCode.mockResolvedValue({ valid: true })
    validateReferralCode.mockResolvedValue({ valid: true })
    register.mockResolvedValue(undefined)
    push.mockResolvedValue(undefined)
  })

  it('hides referral input but still honors ref query when manual input is disabled', async () => {
    routeQuery.value = { ref: 'REF123' }

    const wrapper = mount(RegisterView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          LinuxDoOAuthSection: true,
          OidcOAuthSection: true,
          Icon: true,
          TurnstileWidget: true,
          'router-link': { template: '<a><slot /></a>' }
        }
      }
    })

    await flushPromises()

    expect(wrapper.find('#referral_code').exists()).toBe(false)
    expect(validateReferralCode).toHaveBeenCalledWith('REF123')

    await wrapper.find('#email').setValue('user@example.com')
    await wrapper.find('#password').setValue('password123')
    await wrapper.find('form').trigger('submit.prevent')

    expect(register).toHaveBeenCalledWith(
      expect.objectContaining({
        referral_code: 'REF123'
      })
    )
  })

  it('shows referral input and validates ref query when manual input is enabled', async () => {
    getPublicSettings.mockResolvedValueOnce({
      ...baseSettings,
      referral_allow_manual_input: true
    })
    routeQuery.value = { ref: 'REF123' }

    const wrapper = mount(RegisterView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          LinuxDoOAuthSection: true,
          OidcOAuthSection: true,
          Icon: true,
          TurnstileWidget: true,
          'router-link': { template: '<a><slot /></a>' }
        }
      }
    })

    await flushPromises()

    expect(wrapper.find('#referral_code').exists()).toBe(true)
    expect(validateReferralCode).toHaveBeenCalledWith('REF123')
  })

  it('shows referral input when manual input is enabled even if global referral is disabled', async () => {
    getPublicSettings.mockResolvedValueOnce({
      ...baseSettings,
      referral_enabled: false,
      referral_allow_manual_input: true
    })
    routeQuery.value = { ref: 'REF123' }

    const wrapper = mount(RegisterView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          LinuxDoOAuthSection: true,
          OidcOAuthSection: true,
          Icon: true,
          TurnstileWidget: true,
          'router-link': { template: '<a><slot /></a>' }
        }
      }
    })

    await flushPromises()

    expect(wrapper.find('#referral_code').exists()).toBe(true)
    expect(validateReferralCode).toHaveBeenCalledWith('REF123')
  })
})

function mountRegister() {
  return mount(RegisterView, {
    global: {
      stubs: {
        AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
        Icon: true,
        TurnstileWidget: { template: '<div data-testid="turnstile-widget" />' },
        LoginAgreementPrompt: true,
        EmailOAuthButtons: true,
        LinuxDoOAuthSection: true,
        WechatOAuthSection: true,
        OidcOAuthSection: true,
        RouterLink: true,
        transition: false
      }
    }
  })
}

describe('RegisterView invitation layout', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    routeQuery.value = {}
    getPublicSettings.mockResolvedValue({
      ...baseSettings,
      affiliate_enabled: true,
      turnstile_enabled: true,
      turnstile_site_key: 'site-key'
    })
    isWeChatWebOAuthEnabled.mockReturnValue(false)
    register.mockResolvedValue(undefined)
  })

  it('keeps the optional affiliate invitation field before Turnstile', async () => {
    const wrapper = mountRegister()
    await flushPromises()

    const invitationField = wrapper.get('[data-testid="affiliate-invitation-field"]')
    const turnstile = wrapper.get('[data-testid="registration-turnstile"]')

    expect(invitationField.get('input').attributes('id')).toBe('affiliate_code')
    expect(invitationField.text()).toContain('common.optional')
    expect(
      invitationField.element.compareDocumentPosition(turnstile.element) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })

  it('uses the mandatory invitation field without duplicating the affiliate field', async () => {
    getPublicSettings.mockResolvedValueOnce({
      ...baseSettings,
      affiliate_enabled: true,
      turnstile_enabled: true,
      turnstile_site_key: 'site-key',
      invitation_code_enabled: true
    })

    const wrapper = mountRegister()
    await flushPromises()

    expect(wrapper.find('[data-testid="affiliate-invitation-field"]').exists()).toBe(false)
    expect(wrapper.get('#invitation_code').exists()).toBe(true)
  })

  it('submits a non-whitelist email domain so the backend can enforce its registration quota', async () => {
    getPublicSettings.mockResolvedValueOnce({
      ...baseSettings,
      turnstile_enabled: false,
      registration_email_suffix_whitelist: ['allowed.com'],
      registration_email_domain_quota_enabled: true
    })

    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('first@custom.example')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(register).toHaveBeenCalledWith(
      expect.objectContaining({ email: 'first@custom.example' })
    )
    expect(showError).not.toHaveBeenCalled()
  })

  it('shows the localized registration domain quota message returned by the backend', async () => {
    getPublicSettings.mockResolvedValueOnce({
      ...baseSettings,
      turnstile_enabled: false,
      registration_email_suffix_whitelist: ['allowed.com'],
      registration_email_domain_quota_enabled: true
    })
    register.mockRejectedValueOnce({
      reason: 'EMAIL_DOMAIN_REGISTRATION_LIMIT',
      message: 'raw backend message'
    })

    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('second@custom.example')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith(
      '该邮箱域名无法注册新账户。请使用主流邮箱注册；如需使用企业邮箱，请联系客服添加域名白名单。'
    )
  })

  // 域名限量注册开关默认关闭：恢复 PR5423 之前的客户端白名单预检，非白名单域名不发起注册请求。
  it('rejects a non-whitelist email domain locally when the domain quota switch is disabled', async () => {
    getPublicSettings.mockResolvedValueOnce({
      ...baseSettings,
      turnstile_enabled: false,
      registration_email_suffix_whitelist: ['allowed.com']
    })

    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('first@custom.example')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(register).not.toHaveBeenCalled()
    // 校验失败通过 validationToastMessage watcher 弹 toast
    expect(showError).toHaveBeenCalledWith('auth.emailSuffixNotAllowedWithAllowed')
    expect(wrapper.get('#email').classes()).toContain('input-error')
  })

  it('still submits whitelisted email domains when the domain quota switch is disabled', async () => {
    getPublicSettings.mockResolvedValueOnce({
      ...baseSettings,
      turnstile_enabled: false,
      registration_email_suffix_whitelist: ['allowed.com']
    })

    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('user@allowed.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(register).toHaveBeenCalledWith(
      expect.objectContaining({ email: 'user@allowed.com' })
    )
    expect(showError).not.toHaveBeenCalled()
  })
})
