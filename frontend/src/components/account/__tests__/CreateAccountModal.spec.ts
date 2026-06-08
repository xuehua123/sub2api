import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const { createAccountMock, checkMixedChannelRiskMock, getWebSearchEmulationConfigMock } = vi.hoisted(() => ({
  createAccountMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn(),
  getWebSearchEmulationConfigMock: vi.fn()
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
      checkMixedChannelRisk: checkMixedChannelRiskMock
    },
    settings: {
      getWebSearchEmulationConfig: getWebSearchEmulationConfigMock
    },
    tlsFingerprintProfiles: {
      list: vi.fn().mockResolvedValue([])
    }
  }
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
        ProxySelector: true,
        GroupSelector: true,
        ModelWhitelistSelector: true,
        QuotaLimitCard: true,
        OAuthAuthorizationFlow: true,
        ConfirmDialog: true
      }
    }
  })
}

describe('CreateAccountModal', () => {
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
