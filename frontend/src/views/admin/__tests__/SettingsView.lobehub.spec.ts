import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import SettingsView from '../SettingsView.vue'

const {
  getSettings,
  updateSettings,
  getAdminApiKey,
  getOverloadCooldownSettings,
  updateOverloadCooldownSettings,
  getStreamTimeoutSettings,
  updateStreamTimeoutSettings,
  getRectifierSettings,
  updateRectifierSettings,
  getBetaPolicySettings,
  updateBetaPolicySettings,
  getAllGroups,
  showError,
  showSuccess,
  fetchPublicSettings,
  adminSettingsFetch,
  copyToClipboard
} = vi.hoisted(() => ({
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
  getAdminApiKey: vi.fn(),
  updateOverloadCooldownSettings: vi.fn(),
  getOverloadCooldownSettings: vi.fn(),
  getStreamTimeoutSettings: vi.fn(),
  updateStreamTimeoutSettings: vi.fn(),
  getRectifierSettings: vi.fn(),
  updateRectifierSettings: vi.fn(),
  getBetaPolicySettings: vi.fn(),
  updateBetaPolicySettings: vi.fn(),
  getAllGroups: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  fetchPublicSettings: vi.fn(),
  adminSettingsFetch: vi.fn(),
  copyToClipboard: vi.fn()
}))

const messages: Record<string, string> = {
  'admin.settings.referral.title': 'Referral Commission',
  'admin.settings.referral.description': 'Configure referral bindings, commission rules, and withdrawal behavior.',
  'admin.settings.referral.enabled': 'Enable referral commission',
  'admin.settings.referral.level1Enabled': 'Enable level 1 commission',
  'admin.settings.referral.level1Rate': 'Level 1 rate',
  'admin.settings.referral.rewardMode': 'Reward mode',
  'admin.settings.referral.rewardModeFirstPaidOrder': 'First paid order only',
  'admin.settings.referral.rewardModeEveryPaidOrder': 'Every paid order',
  'admin.settings.referral.settlementDelayDays': 'Settlement delay',
  'admin.settings.referral.bindBeforeFirstPaidOnly': 'Lock binding before first paid order',
  'admin.settings.referral.allowManualInput': 'Allow manual referral code input',
  'admin.settings.referral.withdrawEnabled': 'Enable withdrawals',
  'admin.settings.referral.withdrawMinAmount': 'Minimum withdrawal amount',
  'admin.settings.referral.withdrawMaxAmount': 'Maximum withdrawal amount',
  'admin.settings.referral.withdrawDailyLimit': 'Daily withdrawal limit',
  'admin.settings.referral.withdrawFeeRate': 'Withdrawal fee rate',
  'admin.settings.referral.withdrawFixedFee': 'Fixed withdrawal fee',
  'admin.settings.referral.withdrawManualReviewRequired': 'Require manual review',
  'admin.settings.referral.refundReverseEnabled': 'Reverse commission on refund',
  'admin.settings.referral.negativeCarryEnabled': 'Allow negative carry after paid withdrawals',
  'admin.settings.referral.settlementCurrency': 'Settlement currency',
  'admin.settings.referral.withdrawMethods': 'Withdrawal methods',
  'admin.settings.referral.withdrawMethod.alipay': 'Alipay',
  'admin.settings.referral.withdrawMethod.wechat': 'WeChat',
  'admin.settings.referral.withdrawMethod.bank': 'Bank',
  'admin.settings.saveSettings': 'Save Settings',
  'admin.settings.saving': 'Saving...',
  'admin.settings.lobehub.title': 'LobeHub Integration',
  'admin.settings.lobehub.description': 'Configure official LobeHub SSO and import behavior.',
  'admin.settings.lobehub.enabled': 'Enable LobeHub integration',
  'admin.settings.lobehub.enabledHint': 'Show launch entry points and enable the OIDC provider.',
  'admin.settings.lobehub.chatUrl': 'Chat URL',
  'admin.settings.lobehub.chatUrlPlaceholder': 'https://chat.example.com',
  'admin.settings.lobehub.chatUrlHint': 'Public LobeHub chat domain.',
  'admin.settings.lobehub.oidcIssuer': 'OIDC Issuer',
  'admin.settings.lobehub.oidcIssuerPlaceholder': 'https://api.example.com',
  'admin.settings.lobehub.oidcIssuerHint': 'Issuer used by LobeHub generic OIDC.',
  'admin.settings.lobehub.clientId': 'OIDC Client ID',
  'admin.settings.lobehub.clientIdPlaceholder': 'sub2api-lobehub',
  'admin.settings.lobehub.clientIdHint': 'Must match the LobeHub env configuration.',
  'admin.settings.lobehub.clientSecret': 'OIDC Client Secret',
  'admin.settings.lobehub.clientSecretPlaceholder': 'Enter a client secret',
  'admin.settings.lobehub.clientSecretConfiguredPlaceholder':
    'Leave blank to keep current client secret',
  'admin.settings.lobehub.clientSecretHint': 'Used by the token endpoint.',
  'admin.settings.lobehub.clientSecretConfiguredHint':
    'Leave blank to keep the existing secret, fill to rotate it.',
  'admin.settings.lobehub.defaultProvider': 'Default Provider',
  'admin.settings.lobehub.defaultProviderHint': 'Provider name used in imported settings.',
  'admin.settings.lobehub.defaultModel': 'Default Model',
  'admin.settings.lobehub.defaultModelPlaceholder': 'gpt-4.1',
  'admin.settings.lobehub.defaultModelHint': 'Optional default model passed to LobeHub settings.',
  'admin.settings.lobehub.runtimeConfigVersion': 'Runtime Config Version',
  'admin.settings.lobehub.runtimeConfigVersionPlaceholder': '2026-04-07',
  'admin.settings.lobehub.runtimeConfigVersionHint':
    'Bump this whenever import behavior changes so chat bootstrap refreshes.',
  'admin.settings.lobehub.hideImportButton': 'Hide import button',
  'admin.settings.lobehub.hideImportButtonHint':
    'Hide the user-side "Import and Open LobeHub" action.',
  'common.loading': 'Loading'
}

vi.mock('@/api', () => ({
  adminAPI: {
    settings: {
      getSettings,
      updateSettings,
      getAdminApiKey,
      getOverloadCooldownSettings,
      updateOverloadCooldownSettings,
      getStreamTimeoutSettings,
      updateStreamTimeoutSettings,
      getRectifierSettings,
      updateRectifierSettings,
      getBetaPolicySettings,
      updateBetaPolicySettings
    },
    groups: {
      getAll: getAllGroups
    }
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    fetchPublicSettings
  })
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({
    fetch: adminSettingsFetch
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key
    })
  }
})

describe('admin SettingsView LobeHub section', () => {
  beforeEach(() => {
    vi.clearAllMocks()

    getSettings.mockResolvedValue({
      referral_enabled: true,
      referral_level1_enabled: true,
      referral_level1_rate: 0.18,
      referral_reward_mode: 'first_paid_order',
      referral_settlement_delay_days: 7,
      referral_bind_before_first_paid_only: true,
      referral_allow_manual_input: true,
      referral_withdraw_enabled: true,
      referral_withdraw_min_amount: 100,
      referral_withdraw_max_amount: 3000,
      referral_withdraw_daily_limit: 2,
      referral_withdraw_fee_rate: 0.01,
      referral_withdraw_fixed_fee: 0,
      referral_withdraw_manual_review_required: true,
      referral_refund_reverse_enabled: true,
      referral_negative_carry_enabled: true,
      referral_settlement_currency: 'CNY',
      referral_withdraw_methods_enabled: ['alipay', 'wechat'],
      lobehub_enabled: true,
      lobehub_chat_url: 'https://chat.example.com',
      lobehub_oidc_issuer: 'https://api.example.com',
      lobehub_oidc_client_id: 'sub2api-lobehub',
      lobehub_oidc_client_secret_configured: true,
      lobehub_default_provider: 'openai',
      lobehub_default_model: 'gpt-4.1',
      lobehub_runtime_config_version: 'runtime-v1',
      hide_lobehub_import_button: true,
      default_subscriptions: [],
      registration_email_suffix_whitelist: []
    } as any)
    getAllGroups.mockResolvedValue([])
    getAdminApiKey.mockResolvedValue({ exists: false, masked_key: '' })
    getOverloadCooldownSettings.mockResolvedValue({ enabled: true, cooldown_minutes: 10 })
    getStreamTimeoutSettings.mockResolvedValue({
      enabled: true,
      action: 'temp_unsched',
      temp_unsched_minutes: 5,
      threshold_count: 3,
      threshold_window_minutes: 10
    })
    getRectifierSettings.mockResolvedValue({
      enabled: true,
      thinking_signature_enabled: true,
      thinking_budget_enabled: true,
      apikey_signature_enabled: false,
      apikey_signature_patterns: []
    })
    getBetaPolicySettings.mockResolvedValue({ rules: [] })
    updateSettings.mockImplementation(async (payload: any) => payload)
  })

  it('renders LobeHub controls and preserves the configured-secret placeholder', async () => {
    const wrapper = mount(SettingsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
          Select: true,
          GroupBadge: true,
          GroupOptionItem: true,
          Toggle: {
            props: ['modelValue'],
            template: '<input type="checkbox" :checked="modelValue" />'
          },
          ImageUpload: true,
          BackupSettings: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('LobeHub Integration')
    expect(wrapper.find('input[placeholder="https://chat.example.com"]').exists()).toBe(true)
    expect(wrapper.find('input[placeholder="https://api.example.com"]').exists()).toBe(true)
    expect(
      wrapper.find('input[placeholder="Leave blank to keep current client secret"]').exists()
    ).toBe(true)
    expect(
      (
        wrapper.find(
          'input[placeholder="2026-04-07"]'
        ).element as HTMLInputElement
      ).value
    ).toBe('runtime-v1')
    expect(wrapper.text()).toContain('Hide import button')
  })

  it('loads referral settings and submits them back in the save payload', async () => {
    const wrapper = mount(SettingsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
          Select: true,
          GroupBadge: true,
          GroupOptionItem: true,
          Toggle: {
            props: ['modelValue'],
            template: '<input type="checkbox" :checked="modelValue" />'
          },
          ImageUpload: true,
          BackupSettings: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('Referral Commission')
    expect(wrapper.text()).toContain('Settlement currency')
    expect(wrapper.text()).toContain('Withdrawal methods')

    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(updateSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        referral_enabled: true,
        referral_level1_enabled: true,
        referral_level1_rate: 0.18,
        referral_reward_mode: 'first_paid_order',
        referral_settlement_delay_days: 7,
        referral_bind_before_first_paid_only: true,
        referral_allow_manual_input: true,
        referral_withdraw_enabled: true,
        referral_withdraw_min_amount: 100,
        referral_withdraw_max_amount: 3000,
        referral_withdraw_daily_limit: 2,
        referral_withdraw_fee_rate: 0.01,
        referral_withdraw_fixed_fee: 0,
        referral_withdraw_manual_review_required: true,
        referral_refund_reverse_enabled: true,
        referral_negative_carry_enabled: true,
        referral_settlement_currency: 'CNY',
        referral_withdraw_methods_enabled: ['alipay', 'wechat']
      })
    )
  })
})
