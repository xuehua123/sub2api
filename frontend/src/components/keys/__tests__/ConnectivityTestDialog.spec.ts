import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { fetchPublicSettings, runConnectivityTest, copyToClipboard } = vi.hoisted(() => ({
  fetchPublicSettings: vi.fn(),
  runConnectivityTest: vi.fn(),
  copyToClipboard: vi.fn().mockResolvedValue(true),
}))

const settings = {
  connectivity_test_enabled: true,
  connectivity_client_ip_enabled: false,
  connectivity_grade_thresholds: {
    grading_version: '1',
    minimum_success_rate: 0.8,
    max_consecutive_timeouts: 2,
    excellent: { min_success_rate: 1, max_p95_ms: 250, max_mad_ms: 50 },
    good: { min_success_rate: 0.9, max_p95_ms: 500, max_mad_ms: 120 },
  },
  connectivity_probe_samples: 5,
  connectivity_probe_warmup: 0,
  connectivity_probe_max_concurrency: 1,
  connectivity_probe_timeout_ms: 5000,
  connectivity_test_endpoints: [{
    name: '默认端点',
    api_url: 'https://api.example.com/v1',
    probe_url: 'https://api.example.com/.well-known/sub2api/edge-probe',
    is_default: true,
  }],
}

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: settings,
    fetchPublicSettings,
  }),
}))

vi.mock('@/features/connectivity/runner', () => ({ runConnectivityTest }))
vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard }),
}))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

import ConnectivityTestDialog from '../ConnectivityTestDialog.vue'

describe('ConnectivityTestDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    sessionStorage.clear()
    settings.connectivity_client_ip_enabled = false
    settings.connectivity_test_endpoints = [{
      name: '默认端点',
      api_url: 'https://api.example.com/v1',
      probe_url: 'https://api.example.com/.well-known/sub2api/edge-probe',
      is_default: true,
    }]
    fetchPublicSettings.mockResolvedValue(settings)
    runConnectivityTest.mockResolvedValue({
      status: 'complete',
      endpoints: [{ endpoint: settings.connectivity_test_endpoints[0], status: 'graded', grade: 'excellent' }],
      recommendedAPIURL: 'https://api.example.com/v1',
      testedAt: 1234,
      gradingVersion: '1',
    })
  })

  it('forces a public-settings refresh before checking and never opens another page', async () => {
    const open = vi.spyOn(window, 'open')
    const wrapper = mount(ConnectivityTestDialog, {
      props: {
        show: true,
        fallbackEndpoints: [{ name: '默认端点', apiURL: 'https://api.example.com/v1', isDefault: true }],
      },
      global: {
        stubs: {
          Teleport: true,
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
        },
      },
    })

    await wrapper.get('[data-test="start-connectivity-test"]').trigger('click')
    await flushPromises()

    expect(fetchPublicSettings).toHaveBeenCalledWith(true)
    expect(runConnectivityTest).toHaveBeenCalledTimes(1)
    expect(open).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('keys.connectivity.grades.excellent')

    const copyButton = wrapper.get('[data-testid="connectivity-copy-url"]')
    expect(copyButton.attributes('aria-label')).toBe('keys.endpoints.clickToCopy')
    expect(copyButton.classes()).toContain('h-9')
    expect(copyButton.classes()).toContain('w-9')
    await copyButton.trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith(
      'https://api.example.com/v1',
      'keys.endpoints.copied',
    )
  })

  it('keeps focus on one stable primary action while the asynchronous phase changes', async () => {
    const completedResult = {
      status: 'complete' as const,
      endpoints: [{
        endpoint: settings.connectivity_test_endpoints[0],
        status: 'graded' as const,
        grade: 'excellent' as const,
      }],
      recommendedAPIURL: 'https://api.example.com/v1',
      testedAt: 1234,
      gradingVersion: '1',
    }
    let resolveRun!: (value: typeof completedResult) => void
    runConnectivityTest.mockReturnValueOnce(new Promise((resolve) => {
      resolveRun = resolve
    }))
    const wrapper = mount(ConnectivityTestDialog, {
      attachTo: document.body,
      props: {
        show: true,
        fallbackEndpoints: [{ name: '默认端点', apiURL: 'https://api.example.com/v1', isDefault: true }],
      },
      global: {
        stubs: {
          Teleport: true,
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
        },
      },
    })

    try {
      const action = wrapper.get('[data-test="start-connectivity-test"]')
      const actionElement = action.element
      ;(actionElement as HTMLButtonElement).focus()
      await action.trigger('click')
      await flushPromises()

      const runningAction = wrapper.get('[data-test="start-connectivity-test"]')
      expect(runningAction.element).toBe(actionElement)
      expect(runningAction.text()).toContain('common.cancel')
      expect(document.activeElement).toBe(actionElement)

      resolveRun(completedResult)
      await flushPromises()

      const completedAction = wrapper.get('[data-test="start-connectivity-test"]')
      expect(completedAction.element).toBe(actionElement)
      expect(completedAction.text()).toContain('keys.connectivity.retry')
      expect(document.activeElement).toBe(actionElement)
    } finally {
      wrapper.unmount()
    }
  })

  it('localizes the backend default endpoint name', async () => {
    const wrapper = mount(ConnectivityTestDialog, {
      props: { show: true, fallbackEndpoints: [] },
      global: {
        stubs: {
          Teleport: true,
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
        },
      },
    })

    await wrapper.get('[data-test="start-connectivity-test"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('keys.endpoints.title')
    expect(wrapper.text()).not.toContain('默认端点')
  })

  it('does not start probes when the page becomes hidden while settings load', async () => {
    let resolveSettings!: (value: typeof settings) => void
    fetchPublicSettings.mockReturnValueOnce(new Promise((resolve) => {
      resolveSettings = resolve
    }))
    let hidden = false
    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockImplementation(() => hidden ? 'hidden' : 'visible')
    const wrapper = mount(ConnectivityTestDialog, {
      props: { show: true, fallbackEndpoints: [] },
      global: {
        stubs: {
          Teleport: true,
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
        },
      },
    })

    await wrapper.get('[data-test="start-connectivity-test"]').trigger('click')
    hidden = true
    resolveSettings(settings)
    await flushPromises()

    expect(runConnectivityTest).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('keys.connectivity.incompleteMessage')
    visibility.mockRestore()
  })

  it('shows and politely announces that the connectivity configuration is loading', async () => {
    let resolveSettings!: (value: typeof settings) => void
    fetchPublicSettings.mockReturnValueOnce(new Promise((resolve) => {
      resolveSettings = resolve
    }))
    const wrapper = mount(ConnectivityTestDialog, {
      props: { show: true, fallbackEndpoints: [] },
      global: {
        stubs: {
          Teleport: true,
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
        },
      },
    })

    await wrapper.get('[data-test="start-connectivity-test"]').trigger('click')

    expect(wrapper.get('[data-testid="connectivity-loading"]').text()).toContain(
      'keys.connectivity.loadingConfig',
    )
    const announcement = wrapper.get('[data-testid="connectivity-status-announcement"]')
    expect(announcement.attributes('role')).toBe('status')
    expect(announcement.attributes('aria-live')).toBe('polite')
    expect(announcement.text()).toContain('keys.connectivity.loadingConfig')

    resolveSettings(settings)
    await flushPromises()
  })

  it('restores a fresh cached grade when mounted already open', () => {
    sessionStorage.setItem('sub2api_connectivity_results', JSON.stringify([{
      url: 'https://api.example.com/v1',
      grade: 'good',
      tested_at: Date.now(),
      grading_version: '1',
    }]))

    const wrapper = mount(ConnectivityTestDialog, {
      props: {
        show: true,
        fallbackEndpoints: [{ name: '默认端点', apiURL: 'https://api.example.com/v1', isDefault: true }],
      },
      global: {
        stubs: {
          Teleport: true,
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
        },
      },
    })

    expect(wrapper.text()).toContain('keys.connectivity.grades.good')
    expect(wrapper.get('[data-test="start-connectivity-test"]').text()).toContain(
      'keys.connectivity.retry',
    )
  })

  it('matches cached grades against equivalent display URLs after backend normalization', () => {
    sessionStorage.setItem('sub2api_connectivity_results', JSON.stringify([{
      url: 'https://api.example.com/v1',
      grade: 'excellent',
      tested_at: Date.now(),
      grading_version: '1',
    }]))

    const wrapper = mount(ConnectivityTestDialog, {
      props: {
        show: true,
        fallbackEndpoints: [{ name: '默认端点', apiURL: 'https://API.EXAMPLE.com/v1/', isDefault: true }],
      },
      global: {
        stubs: {
          Teleport: true,
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
        },
      },
    })

    expect(wrapper.text()).toContain('keys.connectivity.grades.excellent')
  })

  it('aborts the active browser check when the dialog closes', async () => {
    let observedSignal: AbortSignal | undefined
    runConnectivityTest.mockImplementation((_config, signal: AbortSignal) => {
      observedSignal = signal
      return new Promise((resolve) => signal.addEventListener('abort', () => resolve({
        status: 'cancelled',
        endpoints: [],
        testedAt: Date.now(),
        gradingVersion: '1',
      }), { once: true }))
    })
    const wrapper = mount(ConnectivityTestDialog, {
      props: { show: true, fallbackEndpoints: [] },
      global: {
        stubs: {
          Teleport: true,
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
        },
      },
    })

    await wrapper.get('[data-test="start-connectivity-test"]').trigger('click')
    await flushPromises()
    await wrapper.setProps({ show: false })
    await flushPromises()

    expect(observedSignal?.aborted).toBe(true)
  })

  it('does not claim a common exit IP when any configured URL is incomplete', async () => {
    settings.connectivity_client_ip_enabled = true
    const secondEndpoint = {
      name: '备用端点',
      api_url: 'https://alt.example.com/v1',
      probe_url: 'https://alt.example.com/.well-known/sub2api/edge-probe',
      is_default: false,
    }
    settings.connectivity_test_endpoints = [
      settings.connectivity_test_endpoints[0],
      secondEndpoint,
    ]
    runConnectivityTest.mockResolvedValue({
      status: 'incomplete',
      endpoints: [
        {
          endpoint: settings.connectivity_test_endpoints[0],
          status: 'graded',
          grade: 'excellent',
          clientIP: '8.8.8.8',
        },
        { endpoint: secondEndpoint, status: 'incomplete' },
      ],
      testedAt: 1234,
      gradingVersion: '1',
    })

    const wrapper = mount(ConnectivityTestDialog, {
      props: { show: true, fallbackEndpoints: [] },
      global: {
        stubs: {
          Teleport: true,
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
        },
      },
    })

    await wrapper.get('[data-test="start-connectivity-test"]').trigger('click')
    await flushPromises()

    const summary = wrapper.get('[data-testid="connectivity-exit-summary"]')
    expect(summary.text()).toContain('keys.connectivity.unknownExit')
    expect(summary.text()).not.toContain('8.8.8.8')
  })
})
