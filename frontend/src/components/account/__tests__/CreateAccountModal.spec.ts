import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const {
  createAccountMock,
  checkMixedChannelRiskMock,
  discoverUpstreamRateMultiplierGroupsMock,
  getWebSearchEmulationConfigMock,
  importCodexSessionMock,
  createOpenAICodexPATMock
} = vi.hoisted(() => ({
  createAccountMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn(),
  discoverUpstreamRateMultiplierGroupsMock: vi.fn(),
  getWebSearchEmulationConfigMock: vi.fn(),
  importCodexSessionMock: vi.fn(),
  createOpenAICodexPATMock: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
    showWarning: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    isSimpleMode: true
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      create: createAccountMock,
      checkMixedChannelRisk: checkMixedChannelRiskMock,
      discoverUpstreamRateMultiplierGroups: discoverUpstreamRateMultiplierGroupsMock,
      importCodexSession: importCodexSessionMock,
      createOpenAICodexPAT: createOpenAICodexPATMock
    },
    settings: {
      getWebSearchEmulationConfig: getWebSearchEmulationConfigMock,
      getSettings: vi.fn().mockResolvedValue({})
    },
    tlsFingerprintProfiles: {
      list: vi.fn().mockResolvedValue([])
    }
  }
}))

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn().mockResolvedValue([])
}))

vi.mock('@/composables/useModelWhitelist', () => ({
  claudeModels: ['claude-3-5-sonnet-latest'],
  getPresetMappingsByPlatform: vi.fn(() => []),
  getModelsByPlatform: vi.fn(() => []),
  commonErrorCodes: [],
  buildModelMappingObject: vi.fn(() => undefined),
  fetchAntigravityDefaultMappings: vi.fn().mockResolvedValue([]),
  isValidWildcardPattern: vi.fn(() => true)
}))

vi.mock('@/composables/useQuotaNotifyState', () => ({
  useQuotaNotifyState: () => ({
    globalEnabled: false,
    state: {
      daily: { enabled: false, threshold: null, thresholdType: 'percent' },
      weekly: { enabled: false, threshold: null, thresholdType: 'percent' },
      total: { enabled: false, threshold: null, thresholdType: 'percent' }
    },
    loadGlobalState: vi.fn(),
    writeToExtra: vi.fn()
  })
}))

vi.mock('@/composables/useAccountOAuth', async () => {
  const { ref } = await vi.importActual<typeof import('vue')>('vue')
  return {
    useAccountOAuth: () => ({
      authUrl: ref(''),
      sessionId: ref(''),
      loading: ref(false),
      error: ref(''),
      resetState: vi.fn(),
      generateAuthUrl: vi.fn(),
      buildExtraInfo: vi.fn(() => ({})),
      parseSessionKeys: vi.fn(() => [])
    })
  }
})

vi.mock('@/composables/useOpenAIOAuth', async () => {
  const { ref } = await vi.importActual<typeof import('vue')>('vue')
  return {
    useOpenAIOAuth: () => ({
      authUrl: ref(''),
      sessionId: ref(''),
      loading: ref(false),
      error: ref(''),
      oauthState: ref(''),
      resetState: vi.fn(),
      generateAuthUrl: vi.fn(),
      exchangeAuthCode: vi.fn(),
      validateRefreshToken: vi.fn(),
      buildCredentials: vi.fn(() => ({})),
      buildExtraInfo: vi.fn(() => ({}))
    })
  }
})

vi.mock('@/composables/useGeminiOAuth', async () => {
  const { ref } = await vi.importActual<typeof import('vue')>('vue')
  return {
    useGeminiOAuth: () => ({
      authUrl: ref(''),
      sessionId: ref(''),
      loading: ref(false),
      error: ref(''),
      state: ref(''),
      resetState: vi.fn(),
      generateAuthUrl: vi.fn(),
      getCapabilities: vi.fn().mockResolvedValue({ ai_studio_oauth_enabled: false }),
      exchangeAuthCode: vi.fn(),
      buildCredentials: vi.fn(() => ({})),
      buildExtraInfo: vi.fn(() => ({}))
    })
  }
})

vi.mock('@/composables/useAntigravityOAuth', async () => {
  const { ref } = await vi.importActual<typeof import('vue')>('vue')
  return {
    useAntigravityOAuth: () => ({
      authUrl: ref(''),
      sessionId: ref(''),
      loading: ref(false),
      error: ref(''),
      state: ref(''),
      resetState: vi.fn(),
      generateAuthUrl: vi.fn(),
      exchangeAuthCode: vi.fn(),
      validateRefreshToken: vi.fn(),
      buildCredentials: vi.fn(() => ({}))
    })
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

import CreateAccountModal from '../CreateAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: {
      type: Boolean,
      default: false
    }
  },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const OAuthAuthorizationFlowStub = defineComponent({
  name: 'OAuthAuthorizationFlow',
  emits: ['import-codex-session', 'import-codex-pat'],
  template: `
    <div>
      <button data-testid="import-codex-session" @click="$emit('import-codex-session', 'session-json')">session</button>
      <button data-testid="import-codex-pat" @click="$emit('import-codex-pat', 'pat-token')">pat</button>
    </div>
  `
})

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: {
    modelValue: {
      type: [String, Number, Boolean, null],
      default: ''
    },
    options: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue'],
  template: `
    <select
      v-bind="$attrs"
      :value="modelValue"
      @change="$emit('update:modelValue', $event.target.value)"
    >
      <option v-for="option in options" :key="option.value" :value="option.value">
        {{ option.label }}
      </option>
    </select>
  `
})

const clickButtonContaining = async (wrapper: ReturnType<typeof mount>, text: string) => {
  const button = wrapper.findAll('button').find(item => item.text().includes(text))
  expect(button, `button containing "${text}"`).toBeTruthy()
  await button!.trigger('click')
}

function mountModal() {
  return mount(CreateAccountModal, {
    props: {
      show: true,
      proxies: [],
      groups: []
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: SelectStub,
        Icon: true,
        PlatformIcon: true,
        ProxySelector: true,
        GroupSelector: true,
        ModelWhitelistSelector: true,
        QuotaLimitCard: true,
        OAuthAuthorizationFlow: OAuthAuthorizationFlowStub,
        ConfirmDialog: true
      }
    }
  })
}

async function submitApiKeyAccount(platform: 'openai' | 'anthropic', enableLongContextBilling = false) {
  const wrapper = mountModal()
  await clickButtonContaining(wrapper, platform === 'openai' ? 'OpenAI' : 'admin.accounts.claudeConsole')
  if (platform === 'openai') {
    await clickButtonContaining(wrapper, 'API Key')
  }
  await wrapper.get('form#create-account-form input[type="text"]').setValue(`${platform} account`)
  await wrapper.get('form#create-account-form input[type="password"]').setValue('test-api-key')
  if (enableLongContextBilling) {
    await wrapper.get('[data-testid="openai-long-context-billing-toggle"]').trigger('click')
  }
  await wrapper.get('form#create-account-form').trigger('submit.prevent')
  await flushPromises()
}

async function openCodexImportStep(toggleClicks = 0) {
  const wrapper = mountModal()
  await clickButtonContaining(wrapper, 'OpenAI')
  for (let click = 0; click < toggleClicks; click += 1) {
    await wrapper.get('[data-testid="openai-long-context-billing-toggle"]').trigger('click')
  }
  await wrapper.get('form#create-account-form input[type="text"]').setValue('Codex import')
  await wrapper.get('form#create-account-form').trigger('submit.prevent')
  return wrapper
}

describe('CreateAccountModal', () => {
  it('configures upstream management rate sync without submitting a manual multiplier', async () => {
    createAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset().mockResolvedValue({ has_risk: false })
    discoverUpstreamRateMultiplierGroupsMock.mockReset().mockResolvedValue({
      provider: 'sub2api',
      auth_mode: 'password',
      groups: [{ name: 'plus', rate_multiplier: 0.5 }]
    })
    getWebSearchEmulationConfigMock.mockReset().mockResolvedValue({ enabled: false, providers: [] })
    createAccountMock.mockResolvedValue({ id: 1 })

    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('[data-tour="account-form-name"]').setValue('Upstream synced account')
    await clickButtonContaining(wrapper, 'OpenAI')
    await clickButtonContaining(wrapper, 'API Key')
    await wrapper.get('input[placeholder="sk-proj-..."]').setValue('sk-proj-test')
    await wrapper.get('[data-testid="create-upstream-rate-sync-toggle"]').trigger('click')
    await wrapper.get('[data-testid="create-upstream-management-base-url"]').setValue('https://console.example.com')
    const groupSelectBeforeDetection = wrapper.get('[data-testid="create-upstream-rate-sync-group"]')
    expect(groupSelectBeforeDetection.element.tagName).toBe('SELECT')
    expect(groupSelectBeforeDetection.attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="create-upstream-rate-sync-username"]').setValue('manager@example.com')
    await wrapper.get('[data-testid="create-upstream-rate-sync-password"]').setValue('management-password')
    await wrapper.get('[data-testid="create-upstream-rate-sync-discover"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="create-upstream-rate-sync-group"]').setValue('plus')

    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    const payload = createAccountMock.mock.calls[0]?.[0]
    expect(payload.rate_multiplier).toBeUndefined()
    expect(payload.extra).toMatchObject({
      upstream_rate_multiplier_sync_enabled: true,
      upstream_rate_multiplier_sync_group: 'plus',
      upstream_rate_multiplier_sync_provider: 'sub2api',
      upstream_rate_multiplier_sync_auth_mode: 'password'
    })
    expect(discoverUpstreamRateMultiplierGroupsMock).toHaveBeenCalledWith(expect.objectContaining({
      base_url: 'https://api.openai.com',
      management_base_url: 'https://console.example.com',
      auth_mode: 'password'
    }))
    expect(payload.upstream_management_auth).toEqual({ username: 'manager@example.com', password: 'management-password' })
    expect(payload.upstream_management_base_url).toBe('https://console.example.com')
  })

  it('allows setting upstream gzip during account creation', async () => {
    createAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    getWebSearchEmulationConfigMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    getWebSearchEmulationConfigMock.mockResolvedValue({ enabled: false, providers: [] })
    createAccountMock.mockResolvedValue({ id: 1 })

    const wrapper = mountModal()
    await flushPromises()

    await wrapper.get('[data-tour="account-form-name"]').setValue('OpenAI Key')
    await clickButtonContaining(wrapper, 'OpenAI')
    await clickButtonContaining(wrapper, 'API Key')

    const toggle = wrapper.get('[data-testid="create-upstream-gzip-toggle"]')
    expect(toggle.attributes('aria-checked')).toBe('true')
    await toggle.trigger('click')
    expect(toggle.attributes('aria-checked')).toBe('false')

    await wrapper.get('input[placeholder="sk-proj-..."]').setValue('sk-proj-test')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.upstream_gzip_enabled).toBe(false)
  })

  it('allows setting OpenAI HTTP protocol during account creation', async () => {
    createAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    getWebSearchEmulationConfigMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    getWebSearchEmulationConfigMock.mockResolvedValue({ enabled: false, providers: [] })
    createAccountMock.mockResolvedValue({ id: 1 })

    const wrapper = mountModal()
    await flushPromises()

    await wrapper.get('[data-tour="account-form-name"]').setValue('OpenAI Key')
    await clickButtonContaining(wrapper, 'OpenAI')
    await clickButtonContaining(wrapper, 'API Key')
    await wrapper.get('[data-testid="create-openai-http-protocol-select"]').setValue('h1')
    await wrapper.get('input[placeholder="sk-proj-..."]').setValue('sk-proj-test')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.openai_http_protocol).toBe('h1')
  })
})

describe('CreateAccountModal OpenAI long-context billing', () => {
  beforeEach(() => {
    createAccountMock.mockReset().mockResolvedValue({})
    checkMixedChannelRiskMock.mockReset().mockResolvedValue({ has_risk: false })
    getWebSearchEmulationConfigMock.mockReset().mockResolvedValue({ enabled: false, providers: [] })
    importCodexSessionMock.mockReset().mockResolvedValue({
      created: 1,
      updated: 0,
      skipped: 0,
      failed: 0,
      errors: [],
      warnings: []
    })
    createOpenAICodexPATMock.mockReset().mockResolvedValue({})
  })

  it('sends false explicitly for normal OpenAI account creation by default', async () => {
    await submitApiKeyAccount('openai')

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBe(false)
  })

  it('sends true explicitly when OpenAI long-context billing is enabled', async () => {
    await submitApiKeyAccount('openai', true)

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBe(true)
  })

  it('omits the OpenAI setting for non-OpenAI account creation', async () => {
    await submitApiKeyAccount('anthropic')

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBeUndefined()
  })

  it('leaves Codex session import billing ownership to the backend', async () => {
    const wrapper = await openCodexImportStep()
    await wrapper.get('[data-testid="import-codex-session"]').trigger('click')
    await flushPromises()

    expect(importCodexSessionMock).toHaveBeenCalledTimes(1)
    expect(importCodexSessionMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBeUndefined()
  })

  it('leaves Codex PAT import billing ownership to the backend', async () => {
    const wrapper = await openCodexImportStep()
    await wrapper.get('[data-testid="import-codex-pat"]').trigger('click')
    await flushPromises()

    expect(createOpenAICodexPATMock).toHaveBeenCalledTimes(1)
    expect(createOpenAICodexPATMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBeUndefined()
  })

  it('sends explicit true for Codex session import after the toggle is enabled', async () => {
    const wrapper = await openCodexImportStep(1)
    await wrapper.get('[data-testid="import-codex-session"]').trigger('click')
    await flushPromises()

    expect(importCodexSessionMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBe(true)
  })

  it('sends explicit false for Codex session import after the toggle is changed back', async () => {
    const wrapper = await openCodexImportStep(2)
    await wrapper.get('[data-testid="import-codex-session"]').trigger('click')
    await flushPromises()

    expect(importCodexSessionMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBe(false)
  })

  it('sends explicit true for Codex PAT import after the toggle is enabled', async () => {
    const wrapper = await openCodexImportStep(1)
    await wrapper.get('[data-testid="import-codex-pat"]').trigger('click')
    await flushPromises()

    expect(createOpenAICodexPATMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBe(true)
  })

  it('sends explicit false for Codex PAT import after the toggle is changed back', async () => {
    const wrapper = await openCodexImportStep(2)
    await wrapper.get('[data-testid="import-codex-pat"]').trigger('click')
    await flushPromises()

    expect(createOpenAICodexPATMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBe(false)
  })
})
