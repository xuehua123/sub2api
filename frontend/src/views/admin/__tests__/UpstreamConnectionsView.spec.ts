import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const {
  createConnectionMock,
  updateConnectionMock,
  listAllConnectionsMock,
  getConnectionMock,
  getTodayUsageMock,
  getRuntimeOverviewMock,
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
  getRuntimeOverviewMock: vi.fn(),
  probeConnectionMock: vi.fn(),
  getBatchTodayStatsMock: vi.fn(),
  getProxiesMock: vi.fn(),
  showErrorMock: vi.fn()
}))

const routerPushMock = vi.hoisted(() => vi.fn())

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: routerPushMock })
}))

vi.mock('vue-i18n', async importOriginal => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => params
        ? `${key}:${Object.values(params).join(',')}`
        : key
    })
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
      getRuntimeOverview: getRuntimeOverviewMock,
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
  props: {
    data: { type: Array, default: () => [] },
    columns: { type: Array, default: () => [] }
  },
  emits: ['sort'],
  template: `
    <div>
      <button
        v-for="column in columns.filter(column => column.sortable)"
        :key="column.key"
        :data-testid="'sort-' + column.key"
        @click="$emit('sort', column.key, 'asc')"
      >{{ column.label }}</button>
      <div v-for="row in data" :key="row.id" class="data-row">
        <slot name="cell-name" :row="row"><button class="row-name"><span class="row-name-label">{{ row.name }}</span></button></slot>
        <slot name="cell-wallet" :row="row" />
        <slot name="cell-today_requests" :row="row" />
        <slot name="cell-runtime" :row="row" />
        <slot name="cell-status" :row="row" />
        <slot name="cell-actions" :row="row" />
      </div>
    </div>
  `
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
    localStorage.clear()
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
    getRuntimeOverviewMock.mockResolvedValue({ accounts: [] })
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
    expect(wrapper.text()).toContain('admin.upstreamConnections.walletHighlightHint')
  })

  it('renders a readable runtime summary and opens its account group', async () => {
    listAllConnectionsMock.mockResolvedValue([{
      id: 22, name: 'Runtime upstream', provider: 'newapi', auth_mode: 'password',
      management_base_url: 'https://console.example.com', forwarding_base_url: '', remote_user_id: '',
      proxy_id: null, sync_enabled: true, sync_interval_seconds: 300, version: 1,
      wallet_amount: 100, wallet_currency: 'USD', wallet_usd: 100, wallet_unlimited: false,
      wallet_reliability: 'exact', bound_account_ids: [8], binding_count: 1, group_count: 1
    }])
    getRuntimeOverviewMock.mockResolvedValue({ accounts: [{
      account_id: 8, account_name: 'Primary', current_concurrency: 4, waiting_count: 1,
      groups: [{
        group_id: 7, group_name: 'VIP', today: { requests: 12, tokens: 300, account_cost: 0, standard_cost: 0, user_cost: 0 },
        five_minute_requests: 4, five_minute_success_count: 3, five_minute_error_count: 1, five_minute_success_rate: 75
      }]
    }] })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('admin.upstreamConnections.runtime.accountsUnit')
    expect(wrapper.text()).toContain('admin.upstreamConnections.runtime.compactConcurrency:4')
    // 5m volume must be visible in the list (not only the detail dialog).
    expect(wrapper.find('[data-testid="runtime-group-5m-count"]').text()).toContain(
      'admin.upstreamConnections.runtime.fiveMinuteRequestsCompact:4'
    )
    expect(wrapper.text()).toContain('75.0%')
    expect(wrapper.text()).not.toContain('admin.upstreamConnections.runtime.unavailable')
    const groupButton = wrapper.findAll('button').find(button => button.text().includes('VIP'))
    expect(groupButton).toBeDefined()
    await groupButton!.trigger('click')
    expect(routerPushMock).toHaveBeenCalledWith({
      name: 'AdminAccounts',
      query: { group: '7', upstream_connection_id: '22', runtime_traffic: '1' }
    })
  })

  it('does not map log ungrouped traffic to membership ungrouped filter', async () => {
    listAllConnectionsMock.mockResolvedValue([{
      id: 25, name: 'Ungrouped traffic', provider: 'newapi', auth_mode: 'password',
      management_base_url: 'https://console.example.com', forwarding_base_url: '', remote_user_id: '',
      proxy_id: null, sync_enabled: true, sync_interval_seconds: 300, version: 1,
      wallet_amount: 100, wallet_currency: 'USD', wallet_usd: 100, wallet_unlimited: false,
      wallet_reliability: 'exact', bound_account_ids: [8], binding_count: 1, group_count: 0
    }])
    getRuntimeOverviewMock.mockResolvedValue({ accounts: [{
      account_id: 8, account_name: 'Primary', current_concurrency: 1, waiting_count: 0,
      groups: [{
        group_id: 0, group_name: '', today: { requests: 3, tokens: 0, account_cost: 1, standard_cost: 0, user_cost: 0 },
        five_minute_requests: 2, five_minute_success_count: 2, five_minute_error_count: 0, five_minute_success_rate: 100
      }]
    }] })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('admin.upstreamConnections.runtime.ungroupedTraffic')
    const groupButton = wrapper.findAll('button').find(button => button.text().includes('runtime.ungroupedTraffic'))
    expect(groupButton).toBeDefined()
    await groupButton!.trigger('click')
    expect(routerPushMock).toHaveBeenCalledWith({
      name: 'AdminAccounts',
      query: { upstream_connection_id: '25', runtime_traffic: '1' }
    })
  })

  it('retries runtime for only the unavailable row connection', async () => {
    listAllConnectionsMock.mockResolvedValue([
      {
        id: 31, name: 'Broken runtime', provider: 'newapi', auth_mode: 'password',
        management_base_url: 'https://console.example.com', forwarding_base_url: '', remote_user_id: '',
        proxy_id: null, sync_enabled: true, sync_interval_seconds: 300, version: 1,
        wallet_amount: 100, wallet_currency: 'USD', wallet_usd: 100, wallet_unlimited: false,
        wallet_reliability: 'exact', bound_account_ids: [8, 9], binding_count: 2, group_count: 0
      },
      {
        id: 32, name: 'Healthy runtime', provider: 'newapi', auth_mode: 'password',
        management_base_url: 'https://other.example.com', forwarding_base_url: '', remote_user_id: '',
        proxy_id: null, sync_enabled: true, sync_interval_seconds: 300, version: 1,
        wallet_amount: 50, wallet_currency: 'USD', wallet_usd: 50, wallet_unlimited: false,
        wallet_reliability: 'exact', bound_account_ids: [99], binding_count: 1, group_count: 0
      }
    ])
    // Initial overview fails for both paths that load runtime with all ids.
    getRuntimeOverviewMock.mockRejectedValueOnce(new Error('runtime down'))

    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('admin.upstreamConnections.runtime.unavailable')

    getRuntimeOverviewMock.mockClear()
    getRuntimeOverviewMock.mockResolvedValueOnce({ accounts: [
      { account_id: 8, account_name: 'A', current_concurrency: 1, waiting_count: 0, groups: [] },
      { account_id: 9, account_name: 'B', current_concurrency: 0, waiting_count: 0, groups: [] }
    ] })

    // Higher id sorts first; locate the broken row's retry by sibling text.
    const brokenRow = wrapper.findAll('.data-row').find(row => row.text().includes('Broken runtime'))
    expect(brokenRow).toBeDefined()
    await brokenRow!.get('[data-testid="runtime-row-retry"]').trigger('click')
    await flushPromises()

    // Only the broken connection's bound accounts — not account 99 from the other row.
    expect(getRuntimeOverviewMock).toHaveBeenCalledTimes(1)
    expect(getRuntimeOverviewMock).toHaveBeenCalledWith([8, 9])
    expect(wrapper.text()).toContain('admin.upstreamConnections.runtime.compactConcurrency:1')
  })

  it('keeps the connection list usable when a runtime account omits groups', async () => {
    listAllConnectionsMock.mockResolvedValue([{
      id: 221, name: 'Missing runtime groups', provider: 'newapi', auth_mode: 'password',
      management_base_url: 'https://console.example.com', forwarding_base_url: '', remote_user_id: '',
      proxy_id: null, sync_enabled: true, sync_interval_seconds: 300, version: 1,
      wallet_amount: 100, wallet_currency: 'USD', wallet_usd: 100, wallet_unlimited: false,
      wallet_reliability: 'exact', bound_account_ids: [8], binding_count: 1, group_count: 0
    }])
    getRuntimeOverviewMock.mockResolvedValue({ accounts: [{
      account_id: 8, account_name: 'Primary', current_concurrency: 2, waiting_count: 0
    }] })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('Missing runtime groups')
    expect(wrapper.text()).toContain('admin.upstreamConnections.runtime.compactConcurrency:2')
  })

  it('shows the two busiest groups in the table and every group in runtime details', async () => {
    listAllConnectionsMock.mockResolvedValue([{
      id: 23, name: 'Dense runtime', provider: 'newapi', auth_mode: 'password',
      management_base_url: 'https://console.example.com', forwarding_base_url: '', remote_user_id: '',
      proxy_id: null, sync_enabled: true, sync_interval_seconds: 300, version: 1,
      wallet_amount: 100, wallet_currency: 'USD', wallet_usd: 100, wallet_unlimited: false,
      wallet_reliability: 'exact', bound_account_ids: [1, 2, 3, 4], binding_count: 4, group_count: 1
    }])
    getRuntimeOverviewMock.mockResolvedValue({ accounts: [
      { account_id: 1, account_name: 'Lowest', current_concurrency: 0, waiting_count: 0, groups: [{ group_id: 1, group_name: 'A', today: { requests: 100, tokens: 0, account_cost: 1, standard_cost: 0, user_cost: 0 }, five_minute_requests: 0, five_minute_success_count: 0, five_minute_error_count: 0, five_minute_success_rate: null }] },
      { account_id: 2, account_name: 'Highest', current_concurrency: 3, waiting_count: 0, groups: [{ group_id: 2, group_name: 'B', today: { requests: 1, tokens: 0, account_cost: 8, standard_cost: 0, user_cost: 0 }, five_minute_requests: 1, five_minute_success_count: 1, five_minute_error_count: 0, five_minute_success_rate: 100 }] },
      { account_id: 3, account_name: 'Middle', current_concurrency: 1, waiting_count: 0, groups: [{ group_id: 3, group_name: 'C', today: { requests: 2, tokens: 0, account_cost: 5, standard_cost: 0, user_cost: 0 }, five_minute_requests: 1, five_minute_success_count: 1, five_minute_error_count: 0, five_minute_success_rate: 100 }] },
      { account_id: 4, account_name: 'Lower', current_concurrency: 0, waiting_count: 0, groups: [{ group_id: 4, group_name: 'D', today: { requests: 3, tokens: 0, account_cost: 2, standard_cost: 0, user_cost: 0 }, five_minute_requests: 1, five_minute_success_count: 0, five_minute_error_count: 1, five_minute_success_rate: 0 }] }
    ] })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('B')
    expect(wrapper.text()).toContain('C')
    // Compact cell shows top-2 by cost (B, C). Account "Lower" / group D appear only after details open.
    expect(wrapper.text()).not.toContain('Lower')
    const moreGroups = wrapper.findAll('button').find(button => button.text().includes('runtime.moreGroups:2'))
    expect(moreGroups).toBeDefined()
    await moreGroups!.trigger('click')
    expect(wrapper.text()).toContain('Lower')
    expect(wrapper.text()).toContain('admin.upstreamConnections.runtime.successFailure')
  })

  it('refreshes only the runtime snapshot without reloading connections', async () => {
    const connection = {
      id: 24, name: 'Runtime refresh', provider: 'newapi', auth_mode: 'password',
      management_base_url: 'https://console.example.com', forwarding_base_url: '', remote_user_id: '',
      proxy_id: null, sync_enabled: true, sync_interval_seconds: 300, version: 1,
      wallet_amount: 100, wallet_currency: 'USD', wallet_usd: 100, wallet_unlimited: false,
      wallet_reliability: 'exact', bound_account_ids: [8], binding_count: 1, group_count: 1
    }
    listAllConnectionsMock.mockResolvedValue([connection])
    getRuntimeOverviewMock.mockResolvedValueOnce({ accounts: [{
      account_id: 8, account_name: 'Primary', current_concurrency: 1, waiting_count: 0, groups: []
    }] })

    const wrapper = mountView()
    await flushPromises()
    listAllConnectionsMock.mockClear()
    getRuntimeOverviewMock.mockResolvedValueOnce({ accounts: [{
      account_id: 8, account_name: 'Primary', current_concurrency: 2, waiting_count: 0, groups: []
    }] })

    await wrapper.get('button[title="admin.upstreamConnections.runtime.refreshTitle"]').trigger('click')
    await flushPromises()

    expect(getRuntimeOverviewMock).toHaveBeenLastCalledWith([8])
    expect(listAllConnectionsMock).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.upstreamConnections.runtime.compactConcurrency:2')
  })

  it('refreshes runtime from the open detail dialog without wiping groups/bindings or other connections', async () => {
    listAllConnectionsMock.mockResolvedValue([
      {
        id: 26, name: 'Dialog refresh', provider: 'newapi', auth_mode: 'password',
        management_base_url: 'https://console.example.com', forwarding_base_url: '', remote_user_id: '',
        proxy_id: null, sync_enabled: true, sync_interval_seconds: 300, version: 1,
        wallet_amount: 100, wallet_currency: 'USD', wallet_usd: 100, wallet_unlimited: false,
        wallet_reliability: 'exact', bound_account_ids: [8], binding_count: 1, group_count: 1,
        groups: [], bindings: []
      },
      {
        id: 27, name: 'Other connection', provider: 'newapi', auth_mode: 'password',
        management_base_url: 'https://other.example.com', forwarding_base_url: '', remote_user_id: '',
        proxy_id: null, sync_enabled: true, sync_interval_seconds: 300, version: 1,
        wallet_amount: 50, wallet_currency: 'USD', wallet_usd: 50, wallet_unlimited: false,
        wallet_reliability: 'exact', bound_account_ids: [99], binding_count: 1, group_count: 0,
        groups: [], bindings: []
      }
    ])
    getRuntimeOverviewMock.mockResolvedValueOnce({ accounts: [
      {
        account_id: 8, account_name: 'Primary', current_concurrency: 1, waiting_count: 0,
        groups: [{
          group_id: 7, group_name: 'VIP', today: { requests: 2, tokens: 0, account_cost: 1, standard_cost: 0, user_cost: 0 },
          five_minute_requests: 1, five_minute_success_count: 1, five_minute_error_count: 0, five_minute_success_rate: 100
        }]
      },
      {
        account_id: 99, account_name: 'Other', current_concurrency: 0, waiting_count: 0, groups: []
      }
    ] })
    getConnectionMock.mockResolvedValue({
      id: 26, name: 'Dialog refresh', provider: 'newapi', auth_mode: 'password',
      management_base_url: 'https://console.example.com', forwarding_base_url: '', remote_user_id: '',
      proxy_id: null, sync_enabled: true, sync_interval_seconds: 300, version: 1,
      wallet_amount: 100, wallet_currency: 'USD', wallet_usd: 100, wallet_unlimited: false,
      wallet_reliability: 'exact', bound_account_ids: [8], binding_count: 1, group_count: 1,
      groups: [{ id: 1, remote_id: 'g1', name: 'VIP', rate_multiplier: 1.2, source: 'probe', confidence: 'exact', metadata: {}, observed_at: null, fresh_until: null }],
      bindings: [{ id: 1, account_id: 8, connection_id: 26, remote_token_id: 't1', remote_token_name: 'key', resolution_kind: 'fixed', remote_group_id: 'g1', remote_group_name: 'VIP', fallback_groups: [], observed_multiplier: 1.2, confidence: 'exact', source: 'probe', apply_policy: 'observe_only', status: 'ready', sync_failures: 0, last_error: '', resolution_details: {}, observed_at: null, fresh_until: null }]
    })
    getTodayUsageMock.mockResolvedValue({
      timezone: 'Asia/Shanghai',
      start_at: '2026-07-21T00:00:00Z',
      end_at: '2026-07-21T12:00:00Z',
      summary: { requests: 2, tokens: 0, account_cost: 1, standard_cost: 0, user_cost: 0 },
      trend: [],
      accounts: []
    })

    const wrapper = mountView()
    await flushPromises()
    // Default sort puts higher id first; open the connection under test by label.
    const nameButton = wrapper.findAll('.row-name').find(button => button.text().includes('Dialog refresh'))
    expect(nameButton).toBeDefined()
    await nameButton!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('1.2x')
    expect(wrapper.text()).toContain('VIP')

    getRuntimeOverviewMock.mockClear()
    getRuntimeOverviewMock.mockResolvedValueOnce({ accounts: [{
      account_id: 8, account_name: 'Primary', current_concurrency: 5, waiting_count: 2,
      groups: [{
        group_id: 7, group_name: 'VIP', today: { requests: 9, tokens: 0, account_cost: 3, standard_cost: 0, user_cost: 0 },
        five_minute_requests: 7, five_minute_success_count: 6, five_minute_error_count: 1, five_minute_success_rate: 85.7
      }]
    }] })

    await wrapper.get('[data-testid="runtime-detail-refresh"]').trigger('click')
    await flushPromises()

    // Scoped to the open connection only — not account 99 from the other connection.
    expect(getRuntimeOverviewMock).toHaveBeenCalledTimes(1)
    expect(getRuntimeOverviewMock).toHaveBeenCalledWith([8])

    // Runtime metrics update in place.
    expect(wrapper.text()).toContain('admin.upstreamConnections.runtime.compactConcurrency:5')
    expect(wrapper.find('[data-testid="runtime-group-5m-count"]').text()).toContain(
      'admin.upstreamConnections.runtime.fiveMinuteRequestsCompact:7'
    )
    // Full detail groups/bindings must survive runtime-only merge.
    expect(wrapper.text()).toContain('1.2x')
    expect(wrapper.text()).not.toContain('admin.upstreamConnections.detail.noGroups')
  })

  it('keeps a mid-flight detail refresh when the open GET returns later', async () => {
    let resolveGet: ((value: unknown) => void) | undefined
    listAllConnectionsMock.mockResolvedValue([{
      id: 28, name: 'Race connection', provider: 'newapi', auth_mode: 'password',
      management_base_url: 'https://console.example.com', forwarding_base_url: '', remote_user_id: '',
      proxy_id: null, sync_enabled: true, sync_interval_seconds: 300, version: 1,
      wallet_amount: 100, wallet_currency: 'USD', wallet_usd: 100, wallet_unlimited: false,
      wallet_reliability: 'exact', bound_account_ids: [8], binding_count: 1, group_count: 1,
      groups: [], bindings: []
    }])
    getRuntimeOverviewMock.mockResolvedValueOnce({ accounts: [{
      account_id: 8, account_name: 'Primary', current_concurrency: 1, waiting_count: 0,
      groups: [{
        group_id: 7, group_name: 'VIP', today: { requests: 2, tokens: 0, account_cost: 1, standard_cost: 0, user_cost: 0 },
        five_minute_requests: 1, five_minute_success_count: 1, five_minute_error_count: 0, five_minute_success_rate: 100
      }]
    }] })
    getConnectionMock.mockImplementation(() => new Promise(resolve => {
      resolveGet = resolve
    }))
    getTodayUsageMock.mockResolvedValue({
      timezone: 'Asia/Shanghai',
      start_at: '2026-07-21T00:00:00Z',
      end_at: '2026-07-21T12:00:00Z',
      summary: { requests: 2, tokens: 0, account_cost: 1, standard_cost: 0, user_cost: 0 },
      trend: [],
      accounts: []
    })

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('.row-name').trigger('click')
    await flushPromises()

    getRuntimeOverviewMock.mockResolvedValueOnce({ accounts: [{
      account_id: 8, account_name: 'Primary', current_concurrency: 9, waiting_count: 0,
      groups: [{
        group_id: 7, group_name: 'VIP', today: { requests: 20, tokens: 0, account_cost: 5, standard_cost: 0, user_cost: 0 },
        five_minute_requests: 15, five_minute_success_count: 15, five_minute_error_count: 0, five_minute_success_rate: 100
      }]
    }] })
    await wrapper.get('[data-testid="runtime-detail-refresh"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="runtime-group-5m-count"]').text()).toContain(
      'admin.upstreamConnections.runtime.fiveMinuteRequestsCompact:15'
    )

    resolveGet?.({
      id: 28, name: 'Race connection', provider: 'newapi', auth_mode: 'password',
      management_base_url: 'https://console.example.com', forwarding_base_url: '', remote_user_id: '',
      proxy_id: null, sync_enabled: true, sync_interval_seconds: 300, version: 1,
      wallet_amount: 100, wallet_currency: 'USD', wallet_usd: 100, wallet_unlimited: false,
      wallet_reliability: 'exact', bound_account_ids: [8], binding_count: 1, group_count: 1,
      groups: [{ id: 1, remote_id: 'g1', name: 'VIP', rate_multiplier: 1.5, source: 'probe', confidence: 'exact', metadata: {}, observed_at: null, fresh_until: null }],
      bindings: []
    })
    await flushPromises()

    // Late GET must not roll runtime back to the open-time snapshot (5m 1 / concurrency 1).
    expect(wrapper.find('[data-testid="runtime-group-5m-count"]').text()).toContain(
      'admin.upstreamConnections.runtime.fiveMinuteRequestsCompact:15'
    )
    expect(wrapper.text()).toContain('admin.upstreamConnections.runtime.compactConcurrency:9')
    expect(wrapper.text()).toContain('1.5x')
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
    await wrapper.get('.row-name').trigger('click')
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

    expect(wrapper.findAll('.row-name-label').map(node => node.text())).toEqual(['Higher traffic', 'Lower traffic'])
  })

  it('sorts by a clicked wallet column and keeps unknown balances last', async () => {
    const base = {
      provider: 'newapi', auth_mode: 'password', management_base_url: 'https://console.example.com',
      forwarding_base_url: '', remote_user_id: '', proxy_id: null, sync_enabled: true,
      sync_interval_seconds: 300, version: 1, wallet_currency: 'USD', wallet_unlimited: false,
      wallet_reliability: 'exact', binding_count: 0, bound_account_ids: []
    }
    listAllConnectionsMock.mockResolvedValue([
      { ...base, id: 61, name: 'Unknown', wallet_amount: null, wallet_usd: null, group_count: 0 },
      { ...base, id: 62, name: 'High', wallet_amount: 300, wallet_usd: 300, group_count: 0 },
      { ...base, id: 63, name: 'Low', wallet_amount: 20, wallet_usd: 20, group_count: 0 }
    ])

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="sort-wallet"]').trigger('click')

    expect(wrapper.findAll('.row-name-label').map(node => node.text())).toEqual(['Low', 'High', 'Unknown'])
  })

  it('uses the configurable wallet threshold only for display highlighting', async () => {
    listAllConnectionsMock.mockResolvedValue([{
      id: 71, name: 'Threshold wallet', provider: 'newapi', auth_mode: 'password',
      management_base_url: 'https://console.example.com', forwarding_base_url: '', remote_user_id: '',
      proxy_id: null, sync_enabled: true, sync_interval_seconds: 300, version: 1,
      wallet_amount: 49, wallet_currency: 'USD', wallet_usd: 49, wallet_unlimited: false,
      wallet_reliability: 'exact', group_count: 0, binding_count: 0, bound_account_ids: []
    }])

    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.find('.border-l-2').exists()).toBe(true)

    await wrapper.get('input[aria-label="admin.upstreamConnections.walletHighlightThreshold"]').setValue('40')
    await nextTick()
    expect(wrapper.find('.border-l-2').exists()).toBe(false)
  })

  it('updates wallet and status in-place after a single-row probe succeeds', async () => {
    const connection = {
      id: 31,
      name: 'Probe target',
      provider: 'newapi',
      auth_mode: 'password',
      management_base_url: 'https://console.example.com',
      forwarding_base_url: '',
      remote_user_id: '',
      proxy_id: null,
      sync_enabled: true,
      sync_interval_seconds: 300,
      version: 1,
      status: 'pending',
      last_error: '',
      wallet_amount: 10,
      wallet_currency: 'USD',
      wallet_usd: 10,
      wallet_unlimited: false,
      wallet_reliability: 'exact',
      last_synced_at: '2026-01-01T00:00:00Z',
      group_count: 1,
      bound_account_ids: [9],
      binding_count: 1
    }
    listAllConnectionsMock.mockResolvedValue([connection])
    getBatchTodayStatsMock.mockResolvedValue({ stats: { '9': { requests: 5, tokens: 50, cost: 1.5 } } })
    probeConnectionMock.mockResolvedValue({
      ...connection,
      version: 2,
      status: 'ready',
      wallet_amount: 88.5,
      wallet_usd: 88.5,
      last_synced_at: '2026-07-21T12:00:00Z',
      group_count: 2
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('10')
    expect(wrapper.text()).toContain('admin.upstreamConnections.statuses.pending')

    listAllConnectionsMock.mockClear()
    await wrapper.get('button[title="admin.upstreamConnections.probe"]').trigger('click')
    await flushPromises()

    expect(probeConnectionMock).toHaveBeenCalledWith(31)
    const row = wrapper.get('.data-row')
    // Immediate local patch — no full-table reload required for the new wallet value.
    expect(row.text()).toContain('88.5')
    expect(row.text()).toContain('admin.upstreamConnections.statuses.ready')
    // Today usage joined client-side must survive the probe merge.
    expect(wrapper.get('[data-testid="today-requests-summary"]').text()).toBe('5')
    // Must not re-list after probe (avoids wiping fresh probe data with a stale list).
    expect(listAllConnectionsMock).not.toHaveBeenCalled()
  })

  it('ignores a second probe click while the first probe is still in flight', async () => {
    const connection = {
      id: 32,
      name: 'Busy probe target',
      provider: 'newapi',
      auth_mode: 'password',
      management_base_url: 'https://console.example.com',
      forwarding_base_url: '',
      remote_user_id: '',
      proxy_id: null,
      sync_enabled: true,
      sync_interval_seconds: 300,
      version: 1,
      status: 'ready',
      wallet_amount: 10,
      wallet_currency: 'USD',
      wallet_usd: 10,
      wallet_unlimited: false,
      wallet_reliability: 'exact',
      bound_account_ids: [],
      binding_count: 0
    }
    listAllConnectionsMock.mockResolvedValue([connection])
    const pendingProbe = deferred<typeof connection>()
    probeConnectionMock.mockReturnValueOnce(pendingProbe.promise)

    const wrapper = mountView()
    await flushPromises()

    const probeButton = wrapper.get('button[title="admin.upstreamConnections.probe"]')
    await probeButton.trigger('click')
    await probeButton.trigger('click')
    await flushPromises()

    expect(probeConnectionMock).toHaveBeenCalledTimes(1)

    pendingProbe.resolve({
      ...connection,
      version: 2,
      wallet_amount: 42,
      wallet_usd: 42
    })
    await flushPromises()
    expect(wrapper.get('.data-row').text()).toContain('42')
  })

  it('keeps a fresher probe snapshot when a stale in-flight list finishes afterward', async () => {
    const connection = {
      id: 33,
      name: 'Race target',
      provider: 'newapi',
      auth_mode: 'password',
      management_base_url: 'https://console.example.com',
      forwarding_base_url: '',
      remote_user_id: '',
      proxy_id: null,
      sync_enabled: true,
      sync_interval_seconds: 300,
      version: 1,
      status: 'pending',
      wallet_amount: 10,
      wallet_currency: 'USD',
      wallet_usd: 10,
      wallet_unlimited: false,
      wallet_reliability: 'exact',
      last_synced_at: '2026-01-01T00:00:00Z',
      bound_account_ids: [9],
      binding_count: 1
    }
    const staleList = deferred<typeof connection[]>()
    listAllConnectionsMock
      .mockResolvedValueOnce([connection])
      .mockReturnValueOnce(staleList.promise)
    getBatchTodayStatsMock.mockResolvedValue({ stats: { '9': { requests: 5, tokens: 50, cost: 1.5 } } })
    probeConnectionMock.mockResolvedValue({
      ...connection,
      version: 2,
      status: 'ready',
      wallet_amount: 99,
      wallet_usd: 99,
      last_synced_at: '2026-07-21T12:00:00Z'
    })

    const wrapper = mountView()
    await flushPromises()

    // Start a full-table refresh that will resolve after probe with the old version.
    await wrapper.get('button[title="common.refresh"]').trigger('click')
    await wrapper.get('button[title="admin.upstreamConnections.probe"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('.data-row').text()).toContain('99')
    expect(wrapper.get('.data-row').text()).toContain('admin.upstreamConnections.statuses.ready')

    staleList.resolve([{ ...connection, version: 1, wallet_amount: 10, wallet_usd: 10, status: 'pending' }])
    await flushPromises()

    // Local probe version 2 must win over the stale list version 1.
    expect(wrapper.get('.data-row').text()).toContain('99')
    expect(wrapper.get('.data-row').text()).toContain('admin.upstreamConnections.statuses.ready')
    expect(wrapper.get('[data-testid="today-requests-summary"]').text()).toBe('5')
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
