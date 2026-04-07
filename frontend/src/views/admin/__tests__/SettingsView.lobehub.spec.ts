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
})
