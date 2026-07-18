import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { listAll, getAccountBinding, bindAccount, unbindAccount, showError } = vi.hoisted(() => ({
  listAll: vi.fn(),
  getAccountBinding: vi.fn(),
  bindAccount: vi.fn(),
  unbindAccount: vi.fn(),
  showError: vi.fn()
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string, fallback?: string) => fallback || key })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    upstreamConnections: { listAll, getAccountBinding, bindAccount, unbindAccount }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError })
}))

import UpstreamConnectionBindingField from '@/components/account/UpstreamConnectionBindingField.vue'

const connection = {
  id: 7,
  name: 'Primary upstream',
  provider: 'newapi',
  status: 'ready'
}

const binding = {
  id: 3,
  account_id: 12,
  connection_id: 7,
  status: 'ready',
  remote_group_name: 'vip',
  observed_multiplier: 0.5,
  last_error: ''
}

describe('UpstreamConnectionBindingField', () => {
  beforeEach(() => {
    listAll.mockReset().mockResolvedValue([connection])
    getAccountBinding.mockReset().mockResolvedValue(binding)
    bindAccount.mockReset().mockResolvedValue(binding)
    unbindAccount.mockReset().mockResolvedValue(undefined)
    showError.mockReset()
  })

  it('loads and exposes the existing account binding', async () => {
    const wrapper = mount(UpstreamConnectionBindingField, {
      props: { modelValue: null, accountId: 12 },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
    })
    await flushPromises()

    expect(listAll).toHaveBeenCalledOnce()
    expect(getAccountBinding).toHaveBeenCalledWith(12)
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([7])
    expect(wrapper.text()).toContain('vip')
    expect(wrapper.text()).toContain('0.5x')
  })

  it('treats the API client normalized 404 as an unbound account', async () => {
    getAccountBinding.mockRejectedValueOnce({ status: 404, code: 'UPSTREAM_ACCOUNT_BINDING_NOT_FOUND' })
    const wrapper = mount(UpstreamConnectionBindingField, {
      props: { modelValue: null, accountId: 12 },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
    })
    await flushPromises()

    expect(showError).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="upstream-connection-binding-select"]').setValue('7')
    await (wrapper.vm as unknown as { apply: (accountId: number) => Promise<unknown> }).apply(12)
    expect(bindAccount).toHaveBeenCalledWith(7, 12)
  })

  it('rebinds without deleting the old row first', async () => {
    const wrapper = mount(UpstreamConnectionBindingField, {
      props: { modelValue: null, accountId: 12 },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
    })
    await flushPromises()
    await wrapper.setProps({ modelValue: 8 })

    await (wrapper.vm as unknown as { apply: (accountId: number) => Promise<unknown> }).apply(12)

    expect(bindAccount).toHaveBeenCalledWith(8, 12)
    expect(unbindAccount).not.toHaveBeenCalled()
  })

  it('unbinds only when the explicit selection is empty', async () => {
    const wrapper = mount(UpstreamConnectionBindingField, {
      props: { modelValue: null, accountId: 12 },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
    })
    await flushPromises()
    await wrapper.get('[data-testid="upstream-connection-binding-select"]').setValue('')

    await (wrapper.vm as unknown as { apply: (accountId: number) => Promise<unknown> }).apply(12)

    expect(unbindAccount).toHaveBeenCalledWith(7, 12)
    expect(bindAccount).not.toHaveBeenCalled()
  })

  it('binds every newly created account in a batch', async () => {
    getAccountBinding.mockRejectedValue({ response: { status: 404 } })
    const wrapper = mount(UpstreamConnectionBindingField, {
      props: { modelValue: null, accountId: null },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
    })
    await flushPromises()
    await wrapper.setProps({ modelValue: 7 })

    const exposed = wrapper.vm as unknown as { apply: (accountId: number) => Promise<unknown> }
    await exposed.apply(20)
    await exposed.apply(21)

    expect(bindAccount).toHaveBeenNthCalledWith(1, 7, 20)
    expect(bindAccount).toHaveBeenNthCalledWith(2, 7, 21)
  })

  it('never applies the previous account binding while the next account is loading', async () => {
    const wrapper = mount(UpstreamConnectionBindingField, {
      props: { modelValue: null, accountId: 12 },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
    })
    await flushPromises()
    await wrapper.setProps({ modelValue: 7 })

    let resolveNextList: ((value: typeof connection[]) => void) | undefined
    listAll.mockImplementationOnce(() => new Promise((resolve) => { resolveNextList = resolve }))
    await wrapper.setProps({ accountId: 13 })

    const exposed = wrapper.vm as unknown as { apply: (accountId: number) => Promise<unknown> }
    const applyPromise = exposed.apply(13)
    await Promise.resolve()
    expect(bindAccount).not.toHaveBeenCalled()
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([null])

    getAccountBinding.mockRejectedValueOnce({ status: 404 })
    resolveNextList?.([connection])
    await applyPromise
    await flushPromises()
    expect(bindAccount).not.toHaveBeenCalled()
  })

  it('ignores a stale 404 after the next account binding has loaded', async () => {
    let rejectPreviousBinding: ((reason: unknown) => void) | undefined
    getAccountBinding.mockImplementationOnce(() => new Promise((_, reject) => {
      rejectPreviousBinding = reject
    }))
    getAccountBinding.mockResolvedValueOnce({
      ...binding,
      account_id: 13,
      connection_id: 8
    })

    const wrapper = mount(UpstreamConnectionBindingField, {
      props: { modelValue: null, accountId: 12 },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
    })
    await flushPromises()
    await wrapper.setProps({ accountId: 13 })
    await flushPromises()
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([8])

    rejectPreviousBinding?.({ status: 404 })
    await flushPromises()

    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([8])
    expect(showError).not.toHaveBeenCalled()
  })
})
