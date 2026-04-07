import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { createLaunchTicket, showError, assign } = vi.hoisted(() => ({
  createLaunchTicket: vi.fn(),
  showError: vi.fn(),
  assign: vi.fn()
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

vi.mock('@/api/lobehub', () => ({
  createLobeHubLaunchTicket: createLaunchTicket
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
  })

  afterEach(() => {
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: originalLocation,
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

  it('creates a launch ticket and redirects to the bridge page', async () => {
    createLaunchTicket.mockResolvedValue({
      ticket_id: 'ticket-1',
      bridge_url: '/api/v1/lobehub/bridge?ticket=ticket-1'
    })

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
    expect(assign).toHaveBeenCalledWith('https://sub2api.example.com/api/v1/lobehub/bridge?ticket=ticket-1')
  })

  it('surfaces an error toast when launch fails', async () => {
    createLaunchTicket.mockRejectedValue(new Error('boom'))

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

    expect(showError).toHaveBeenCalledWith('keys.failedToOpenLobeHub')
    expect(assign).not.toHaveBeenCalled()
  })
})
