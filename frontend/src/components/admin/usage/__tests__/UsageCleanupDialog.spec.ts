import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'

import UsageCleanupDialog from '../UsageCleanupDialog.vue'
import Pagination from '@/components/common/Pagination.vue'

enableAutoUnmount(afterEach)

const { createCleanupTask, listCleanupTasks, showError, showSuccess } = vi.hoisted(() => ({
  createCleanupTask: vi.fn(),
  listCleanupTasks: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      list: vi.fn().mockResolvedValue({ items: [] })
    }
  }
}))

vi.mock('@/api/admin/usage', () => {
  const api = {
    createCleanupTask,
    listCleanupTasks,
    cancelCleanupTask: vi.fn(),
    searchUsers: vi.fn(),
    searchApiKeys: vi.fn()
  }
  return { adminUsageAPI: api, default: api }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const BaseDialogStub = {
  props: ['show', 'title', 'width'],
  emits: ['close'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
}

const ConfirmDialogStub = {
  props: ['show'],
  emits: ['confirm', 'cancel'],
  template: '<button v-if="show" data-testid="confirm-cleanup" @click="$emit(\'confirm\')">confirm</button>'
}

describe('UsageCleanupDialog', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    createCleanupTask.mockReset().mockResolvedValue({})
    listCleanupTasks.mockReset().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 5 })
    showError.mockReset()
    showSuccess.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('keeps the entitlement filter when creating a cleanup task', async () => {
    const wrapper = mount(UsageCleanupDialog, {
      props: {
        show: false,
        filters: {
          entitlement_id: 42
        },
        startDate: '2026-08-01',
        endDate: '2026-08-02'
      },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          ConfirmDialog: ConfirmDialogStub,
          Pagination: true
        }
      }
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.find('[data-test="entitlement-id-filter"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('admin.usage.billingMode')
    expect(wrapper.text()).not.toContain('admin.usage.upstreamModelAudit')

    const submit = wrapper.findAll('button').find(button => button.text() === 'admin.usage.cleanup.submit')
    expect(submit).toBeDefined()
    await submit!.trigger('click')
    await wrapper.get('[data-testid="confirm-cleanup"]').trigger('click')
    await flushPromises()

    expect(createCleanupTask).toHaveBeenCalledWith(expect.objectContaining({
      start_date: '2026-08-01',
      end_date: '2026-08-02',
      entitlement_id: 42
    }))
  })

  it('does not broaden cleanup when the real entitlement filter contains invalid input', async () => {
    const wrapper = mount(UsageCleanupDialog, {
      props: {
        show: false,
        filters: {},
        startDate: '2026-08-01',
        endDate: '2026-08-02'
      },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          ConfirmDialog: ConfirmDialogStub,
          Pagination: true
        }
      }
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    await wrapper.get('[data-test="entitlement-id-filter"]').setValue('not-a-number')
    const submit = wrapper.findAll('button').find(button => button.text() === 'admin.usage.cleanup.submit')
    expect(submit).toBeDefined()
    await submit!.trigger('click')
    await wrapper.get('[data-testid="confirm-cleanup"]').trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.usage.cleanup.invalidEntitlementId')
    expect(createCleanupTask).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="confirm-cleanup"]').exists()).toBe(false)
  })
})

const page = (number: number) => ({ items: [{ id: number, status: 'succeeded', deleted_rows: number, filters: {} }], total: 15, page: number, page_size: 5 })
async function openDialog() {
  listCleanupTasks.mockResolvedValueOnce(page(1))
  const wrapper = mount(UsageCleanupDialog, {
    props: { show: false, filters: {}, startDate: '2026-09-01', endDate: '2026-09-02' },
    global: { stubs: { BaseDialog: { template: '<div><slot/><slot name="footer"/></div>' }, UsageFilters: true, ConfirmDialog: true, Pagination: true } },
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

describe('cleanup task pagination', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    listCleanupTasks.mockReset()
    showError.mockReset()
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it.each(['success', 'failure'])('ignores an older page request that completes with %s', async (outcome) => {
    const wrapper = await openDialog()
    let resolve!: (value: object) => void
    let reject!: (error: Error) => void
    listCleanupTasks.mockImplementationOnce(() => new Promise((res, rej) => { resolve = res; reject = rej }))
    wrapper.findComponent(Pagination).vm.$emit('update:page', 2)
    await flushPromises()
    listCleanupTasks.mockResolvedValueOnce(page(3))
    wrapper.findComponent(Pagination).vm.$emit('update:page', 3)
    await flushPromises()
    if (outcome === 'success') resolve(page(2))
    else reject(new Error('old failure'))
    await flushPromises()
    expect(wrapper.findComponent(Pagination).props('page')).toBe(3)
    expect(wrapper.text()).toContain('#3')
    expect(showError).not.toHaveBeenCalled()
  })

  it('ignores a pending result after the dialog is closed and reopened', async () => {
    const wrapper = await openDialog()
    let resolve!: (value: object) => void
    listCleanupTasks.mockImplementationOnce(() => new Promise(res => { resolve = res }))
    wrapper.findComponent(Pagination).vm.$emit('update:page', 2)
    await flushPromises()
    await wrapper.setProps({ show: false })
    listCleanupTasks.mockResolvedValueOnce(page(1))
    await wrapper.setProps({ show: true })
    await flushPromises()
    resolve(page(2))
    await flushPromises()
    expect(wrapper.findComponent(Pagination).props('page')).toBe(1)
    expect(wrapper.text()).toContain('#1')
  })
})
