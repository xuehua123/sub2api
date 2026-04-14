import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import RegisterView from '../RegisterView.vue'

const {
  push,
  showSuccess,
  showError,
  register,
  getPublicSettings,
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
  validatePromoCode: vi.fn(),
  validateInvitationCode: vi.fn(),
  validateReferralCode: vi.fn(),
  routeQuery: { value: {} as Record<string, unknown> }
}))

const t = (key: string, params?: Record<string, unknown>) => {
  if (key === 'auth.signUpToStart') {
    return `Sign up to start ${String(params?.siteName ?? '')}`
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
  validatePromoCode,
  validateInvitationCode,
  validateReferralCode
}))

vi.mock('@/utils/authError', () => ({
  buildAuthErrorMessage: () => 'registration failed'
}))

vi.mock('@/utils/registrationEmailPolicy', () => ({
  isRegistrationEmailSuffixAllowed: () => true,
  normalizeRegistrationEmailSuffixWhitelist: (value: string[]) => value
}))

const baseSettings = {
  registration_enabled: true,
  email_verify_enabled: false,
  registration_email_suffix_whitelist: [],
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

