import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { createLaunchTicket, showError, assign } = vi.hoisted(() => ({
  createLaunchTicket: vi.fn(),
  showError: vi.fn(),
  assign: vi.fn()
}))

const { open, replace, close } = vi.hoisted(() => ({
  open: vi.fn(),
  replace: vi.fn(),
  close: vi.fn()
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

vi.mock('@/api/lobehub', () => ({
  createLaunchTicket
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError
  })
}))

import LobeHubLaunchButton from '../LobeHubLaunchButton.vue'

const publicSettings = {
  lobehub_enabled: true,
  hide_lobehub_import_button: false
}

describe('LobeHubLaunchButton', () => {
  const originalLocation = window.location
  const originalOpen = window.open

  beforeEach(() => {
    vi.clearAllMocks()
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: {
        ...originalLocation,
        origin: 'https://sub2api.example.com',
        assign
      },
      writable: true
    })
    Object.defineProperty(window, 'open', {
      configurable: true,
      value: open,
      writable: true
    })
  })

  afterEach(() => {
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: originalLocation,
      writable: true
    })
    Object.defineProperty(window, 'open', {
      configurable: true,
      value: originalOpen,
      writable: true
    })
  })

  it('does not render when lobehub is disabled or hidden', () => {
    const disabledWrapper = mount(LobeHubLaunchButton, {
      props: {
        apiKeyId: 9,
        publicSettings: { lobehub_enabled: false, hide_lobehub_import_button: false }
      },
      global: {
        stubs: { Icon: true }
      }
    })
    expect(disabledWrapper.find('button').exists()).toBe(false)

    const hiddenWrapper = mount(LobeHubLaunchButton, {
      props: {
        apiKeyId: 9,
        publicSettings: { lobehub_enabled: true, hide_lobehub_import_button: true }
      },
      global: {
        stubs: { Icon: true }
      }
    })
    expect(hiddenWrapper.find('button').exists()).toBe(false)
  })

  it('creates a launch ticket and opens the bridge page in a new tab', async () => {
    const popup = {
      opener: window,
      closed: false,
      close,
      location: {
        replace
      }
    }
    createLaunchTicket.mockResolvedValue({
      ticket_id: 'ticket-1',
      bridge_url: '/api/v1/lobehub/bridge?ticket=ticket-1'
    })
    open.mockReturnValue(popup)

    const wrapper = mount(LobeHubLaunchButton, {
      props: {
        apiKeyId: 9,
        publicSettings
      },
      global: {
        stubs: { Icon: true }
      }
    })

    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(createLaunchTicket).toHaveBeenCalledWith(9)
    expect(open).toHaveBeenCalledWith('', '_blank')
    expect(popup.opener).toBeNull()
    expect(replace).toHaveBeenCalledWith('https://sub2api.example.com/api/v1/lobehub/bridge?ticket=ticket-1')
    expect(assign).not.toHaveBeenCalled()
  })

  it('falls back to same-tab navigation when the new tab is blocked', async () => {
    createLaunchTicket.mockResolvedValue({
      ticket_id: 'ticket-1',
      bridge_url: '/api/v1/lobehub/bridge?ticket=ticket-1'
    })
    open.mockReturnValue(null)

    const wrapper = mount(LobeHubLaunchButton, {
      props: {
        apiKeyId: 9,
        publicSettings
      },
      global: {
        stubs: { Icon: true }
      }
    })

    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(assign).toHaveBeenCalledWith('https://sub2api.example.com/api/v1/lobehub/bridge?ticket=ticket-1')
  })

  it('surfaces an error toast when launch fails', async () => {
    const popup = {
      opener: window,
      closed: false,
      close,
      location: {
        replace
      }
    }
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    createLaunchTicket.mockRejectedValue(new Error('boom'))
    open.mockReturnValue(popup)

    const wrapper = mount(LobeHubLaunchButton, {
      props: {
        apiKeyId: 9,
        publicSettings
      },
      global: {
        stubs: { Icon: true }
      }
    })

    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(close).toHaveBeenCalled()
    expect(consoleError).toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('keys.failedToOpenLobeHub')
    expect(assign).not.toHaveBeenCalled()
  })
})
