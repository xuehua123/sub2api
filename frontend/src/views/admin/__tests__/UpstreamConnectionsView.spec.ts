import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const {
  createConnectionMock,
  updateConnectionMock,
  listConnectionsMock,
  probeConnectionMock,
  getProxiesMock,
  showErrorMock
} = vi.hoisted(() => ({
  createConnectionMock: vi.fn(),
  updateConnectionMock: vi.fn(),
  listConnectionsMock: vi.fn(),
  probeConnectionMock: vi.fn(),
  getProxiesMock: vi.fn(),
  showErrorMock: vi.fn()
}))

vi.mock('vue-i18n', async importOriginal => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: showErrorMock, showSuccess: vi.fn() })
}))

vi.mock('@/utils/format', () => ({
  formatDateTime: (value: string) => value
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    upstreamConnections: {
      list: listConnectionsMock,
      create: createConnectionMock,
      update: updateConnectionMock,
      probe: probeConnectionMock
    },
    proxies: { getAll: getProxiesMock }
  }
}))

import UpstreamConnectionsView from '../UpstreamConnectionsView.vue'

const SelectStub = defineComponent({
  inheritAttrs: false,
  props: {
    modelValue: { type: [String, Number], default: '' },
    options: { type: Array, default: () => [] }
  },
  emits: ['update:modelValue', 'change'],
  methods: {
    update(event: Event) {
      const value = (event.target as HTMLSelectElement).value
      this.$emit('update:modelValue', value)
      this.$emit('change', value)
    }
  },
  template: `
    <select v-bind="$attrs" :value="modelValue" @change="update">
      <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
    </select>
  `
})

const BaseDialogStub = defineComponent({
  props: { show: Boolean },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const DataTableStub = defineComponent({
  props: { data: { type: Array, default: () => [] } },
  template: '<div><div v-for="row in data" :key="row.id"><slot name="cell-actions" :row="row" /></div></div>'
})

function mountView() {
  return mount(UpstreamConnectionsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
        DataTable: DataTableStub,
        Pagination: true,
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        Select: SelectStub,
        Icon: true
      }
    }
  })
}

describe('UpstreamConnectionsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listConnectionsMock.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
    getProxiesMock.mockResolvedValue([])
    createConnectionMock.mockResolvedValue({ id: 12 })
    updateConnectionMock.mockResolvedValue({ id: 12 })
    probeConnectionMock.mockResolvedValue({ id: 12 })
  })

  it('accepts an optional remote user ID and refresh token in auto access-token mode', async () => {
    const wrapper = mountView()
    await flushPromises()

    const createButton = wrapper.findAll('button').find(button =>
      button.text().includes('admin.upstreamConnections.create')
    )
    expect(createButton).toBeDefined()
    await createButton!.trigger('click')

    await wrapper.get('[data-testid="upstream-auth-mode-select"]').setValue('access_token')
    expect(wrapper.get('[data-testid="upstream-remote-user-id"]').attributes('required')).toBeUndefined()

    await wrapper.get('[data-testid="upstream-name"]').setValue('Primary upstream')
    await wrapper.get('[data-testid="upstream-management-url"]').setValue('https://console.example.com')
    await wrapper.get('[data-testid="upstream-access-token"]').setValue('management-access-token')
    await wrapper.get('[data-testid="upstream-refresh-token"]').setValue('management-refresh-token')
    await wrapper.get('[data-testid="upstream-remote-user-id"]').setValue('42')
    await wrapper.get('form#upstream-connection-form').trigger('submit.prevent')
    await flushPromises()

    expect(showErrorMock).not.toHaveBeenCalled()
    expect(createConnectionMock).toHaveBeenCalledWith(expect.objectContaining({
      provider: 'auto',
      auth_mode: 'access_token',
      remote_user_id: '42',
      credential: {
        access_token: 'management-access-token',
        refresh_token: 'management-refresh-token'
      }
    }))
  })

  it('only requires a remote user ID for providers whose access-token API needs it', async () => {
    const wrapper = mountView()
    await flushPromises()

    const createButton = wrapper.findAll('button').find(button =>
      button.text().includes('admin.upstreamConnections.create')
    )
    await createButton!.trigger('click')
    await wrapper.get('[data-testid="upstream-auth-mode-select"]').setValue('access_token')

    await wrapper.get('[data-testid="upstream-provider-select"]').setValue('oneapi')
    expect(wrapper.get('[data-testid="upstream-remote-user-id"]').attributes('required')).toBeUndefined()

    await wrapper.get('[data-testid="upstream-provider-select"]').setValue('newapi')
    expect(wrapper.get('[data-testid="upstream-remote-user-id"]').attributes('required')).toBe('')
  })

  it('submits the version read by the edit form as expected_version', async () => {
    const connection = {
      id: 12,
      name: 'Primary upstream',
      provider: 'newapi',
      auth_mode: 'password',
      management_base_url: 'https://console.example.com',
      forwarding_base_url: '',
      remote_user_id: '7',
      proxy_id: null,
      sync_enabled: true,
      sync_interval_seconds: 300,
      version: 17
    }
    listConnectionsMock.mockResolvedValue({ items: [connection], total: 1, page: 1, page_size: 20 })
    updateConnectionMock.mockResolvedValue(connection)

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('button[title="common.edit"]').trigger('click')
    await wrapper.get('form#upstream-connection-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateConnectionMock).toHaveBeenCalledWith(12, expect.objectContaining({ expected_version: 17 }))
  })
})
