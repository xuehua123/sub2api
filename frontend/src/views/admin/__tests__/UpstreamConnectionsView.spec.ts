import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const {
  createConnectionMock,
  updateConnectionMock,
  listAllConnectionsMock,
  getConnectionMock,
  getTodayUsageMock,
  probeConnectionMock,
  getBatchTodayStatsMock,
  getProxiesMock,
  showErrorMock
} = vi.hoisted(() => ({
  createConnectionMock: vi.fn(),
  updateConnectionMock: vi.fn(),
  listAllConnectionsMock: vi.fn(),
  getConnectionMock: vi.fn(),
  getTodayUsageMock: vi.fn(),
  probeConnectionMock: vi.fn(),
  getBatchTodayStatsMock: vi.fn(),
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
      listAll: listAllConnectionsMock,
      get: getConnectionMock,
      getTodayUsage: getTodayUsageMock,
      create: createConnectionMock,
      update: updateConnectionMock,
      probe: probeConnectionMock
    },
    accounts: { getBatchTodayStats: getBatchTodayStatsMock },
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
  template: '<div><div v-for="row in data" :key="row.id"><span class="row-name">{{ row.name }}</span><slot name="cell-wallet" :row="row" /><slot name="cell-today_requests" :row="row" /><slot name="cell-actions" :row="row" /></div></div>'
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
        Icon: true,
        UpstreamConnectionUsagePanel: {
          props: ['usage'],
          template: '<div data-testid="usage-panel">{{ usage?.summary?.account_cost ?? "-" }}</div>'
        }
      }
    }
  })
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

describe('UpstreamConnectionsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listAllConnectionsMock.mockResolvedValue([])
    getBatchTodayStatsMock.mockResolvedValue({ stats: {} })
    getProxiesMock.mockResolvedValue([])
    createConnectionMock.mockResolvedValue({ id: 12 })
    updateConnectionMock.mockResolvedValue({ id: 12 })
    probeConnectionMock.mockResolvedValue({ id: 12 })
    getConnectionMock.mockResolvedValue({ id: 12, bindings: [] })
    getTodayUsageMock.mockResolvedValue({
      connection_id: 12,
      summary: { requests: 0, tokens: 0, account_cost: 0, standard_cost: 0, user_cost: 0 },
      trend: [],
      accounts: []
    })
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

  it('sends the explicit Sub2API location confirmation without putting it in secret fields', async () => {
    const wrapper = mountView()
    await flushPromises()

    const createButton = wrapper.findAll('button').find(button =>
      button.text().includes('admin.upstreamConnections.create')
    )
    await createButton!.trigger('click')
    await wrapper.get('[data-testid="upstream-provider-select"]').setValue('sub2api')
    await wrapper.get('[data-testid="upstream-name"]').setValue('Sub2API upstream')
    await wrapper.get('[data-testid="upstream-management-url"]').setValue('https://console.example.com')
    await wrapper.get('input[autocomplete="username"]').setValue('admin@example.com')
    await wrapper.get('input[autocomplete="new-password"]').setValue('secret')
    await wrapper.get('[data-testid="upstream-not-in-cn-confirmed"]').setValue(true)
    await wrapper.get('form#upstream-connection-form').trigger('submit.prevent')
    await flushPromises()

    expect(createConnectionMock).toHaveBeenCalledWith(expect.objectContaining({
      not_in_cn_confirmed: true,
      credential: { username: 'admin@example.com', password: 'secret' }
    }))
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
      version: 17,
      wallet_amount: null,
      wallet_currency: '',
      wallet_usd: null,
      wallet_unlimited: false,
      wallet_reliability: 'unknown',
      bound_account_ids: [],
      binding_count: 0
    }
    listAllConnectionsMock.mockResolvedValue([connection])
    updateConnectionMock.mockResolvedValue(connection)

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('button[title="common.edit"]').trigger('click')
    await wrapper.get('form#upstream-connection-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateConnectionMock).toHaveBeenCalledWith(12, expect.objectContaining({ expected_version: 17 }))
    expect(updateConnectionMock.mock.calls[0]?.[1]).not.toHaveProperty('not_in_cn_confirmed')
  })

  it('shows the upstream homepage, today requests, and low-balance state', async () => {
    listAllConnectionsMock.mockResolvedValue([{
      id: 21,
      name: 'Low wallet',
      provider: 'newapi',
      auth_mode: 'password',
      management_base_url: 'https://console.example.com/api',
      forwarding_base_url: '',
      remote_user_id: '',
      proxy_id: null,
      sync_enabled: true,
      sync_interval_seconds: 300,
      version: 1,
      wallet_amount: 49,
      wallet_currency: 'USD',
      wallet_usd: 49,
      wallet_unlimited: false,
      wallet_reliability: 'exact',
      bound_account_ids: [8],
      binding_count: 1
    }])
    getBatchTodayStatsMock.mockResolvedValue({ stats: { '8': { requests: 123, tokens: 456, cost: 7.5 } } })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('a[aria-label="admin.upstreamConnections.openHomepage"]').attributes('href')).toBe('https://console.example.com')
    expect(wrapper.text()).toContain('123')
    expect(wrapper.get('[data-testid="today-cost-summary"]').text()).toContain('7.50')
    expect(wrapper.text()).toContain('admin.upstreamConnections.lowBalanceHint')
  })

  it('loads the connection usage snapshot when details open', async () => {
    const connection = {
      id: 51,
      name: 'Usage upstream',
      provider: 'sub2api',
      auth_mode: 'password',
      management_base_url: 'https://console.example.com',
      forwarding_base_url: '',
      remote_user_id: '',
      proxy_id: null,
      sync_enabled: true,
      sync_interval_seconds: 300,
      version: 1,
      wallet_amount: 100,
      wallet_currency: 'USD',
      wallet_usd: 100,
      wallet_unlimited: false,
      wallet_reliability: 'exact',
      bound_account_ids: [8],
      binding_count: 1,
      groups: [],
      bindings: []
    }
    listAllConnectionsMock.mockResolvedValue([connection])
    getConnectionMock.mockResolvedValue(connection)
    getTodayUsageMock.mockResolvedValue({
      connection_id: 51,
      summary: { requests: 3, tokens: 100, account_cost: 4.25, standard_cost: 4, user_cost: 5 },
      trend: [],
      accounts: []
    })

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('button[title="common.view"]').trigger('click')
    await flushPromises()

    expect(getTodayUsageMock).toHaveBeenCalledWith(51)
    expect(wrapper.get('[data-testid="usage-panel"]').text()).toBe('4.25')
  })

  it('sorts the full connection set by today request count by default', async () => {
    const base = {
      provider: 'newapi',
      auth_mode: 'password',
      management_base_url: 'https://console.example.com',
      forwarding_base_url: '',
      remote_user_id: '',
      proxy_id: null,
      sync_enabled: true,
      sync_interval_seconds: 300,
      version: 1,
      wallet_amount: 100,
      wallet_currency: 'USD',
      wallet_usd: 100,
      wallet_unlimited: false,
      wallet_reliability: 'exact',
      binding_count: 1
    }
    listAllConnectionsMock.mockResolvedValue([
      { ...base, id: 31, name: 'Lower traffic', bound_account_ids: [31] },
      { ...base, id: 32, name: 'Higher traffic', bound_account_ids: [32] }
    ])
    getBatchTodayStatsMock.mockResolvedValue({ stats: {
      '31': { requests: 4 },
      '32': { requests: 40 }
    } })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findAll('.row-name').map(node => node.text())).toEqual(['Higher traffic', 'Lower traffic'])
  })

  it('ignores a stale today-stats failure after a newer refresh succeeds', async () => {
    const connection = {
      id: 41,
      name: 'Current upstream',
      provider: 'newapi',
      auth_mode: 'password',
      management_base_url: 'https://console.example.com',
      forwarding_base_url: '',
      remote_user_id: '',
      proxy_id: null,
      sync_enabled: true,
      sync_interval_seconds: 300,
      version: 1,
      wallet_amount: 100,
      wallet_currency: 'USD',
      wallet_usd: 100,
      wallet_unlimited: false,
      wallet_reliability: 'exact',
      bound_account_ids: [41],
      binding_count: 1
    }
    const staleStats = deferred<{ stats: Record<string, { requests: number }> }>()
    listAllConnectionsMock.mockResolvedValue([connection])
    getBatchTodayStatsMock
      .mockReturnValueOnce(staleStats.promise)
      .mockResolvedValueOnce({ stats: { '41': { requests: 20 } } })

    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('select')[0].setValue('newapi')
    await flushPromises()

    expect(wrapper.get('[data-testid="today-requests-summary"]').text()).toBe('20')

    staleStats.reject(new Error('stale request failed'))
    await flushPromises()

    expect(wrapper.get('[data-testid="today-requests-summary"]').text()).toBe('20')
  })
})
