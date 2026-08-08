import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UsageCleanupDialog from '../UsageCleanupDialog.vue'

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
