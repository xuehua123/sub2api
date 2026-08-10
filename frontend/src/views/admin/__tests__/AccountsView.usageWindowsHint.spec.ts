import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getAllProxies,
  getAllGroups,
  listUpstreamConnections,
  probeUpstreamConnection,
  pauseAutoRefresh,
  resumeAutoRefresh,
  intervalCallback
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  listUpstreamConnections: vi.fn(),
  probeUpstreamConnection: vi.fn(),
  pauseAutoRefresh: vi.fn(),
  resumeAutoRefresh: vi.fn(),
  intervalCallback: { current: null as null | (() => void | Promise<void>) }
}))

vi.mock('@vueuse/core', async () => {
  const actual = await vi.importActual<typeof import('@vueuse/core')>('@vueuse/core')
  return {
    ...actual,
    useIntervalFn: (callback: () => void | Promise<void>) => {
      intervalCallback.current = callback
      return {
        pause: pauseAutoRefresh,
        resume: resumeAutoRefresh
      }
    }
  }
})

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats,
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn()
    },
    proxies: {
      getAll: getAllProxies
    },
    groups: {
      getAll: getAllGroups
    },
    upstreamConnections: {
      listAll: listUpstreamConnections,
      probe: probeUpstreamConnection
    }
  }
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getAccountHealth: vi.fn().mockResolvedValue({ items: [] })
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    token: 'test-token'
  })
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({
    query: {}
  }),
  useRouter: () => ({
    push: vi.fn(() => Promise.resolve())
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

// Render the per-column header slots so we can assert the usage-window header hint.
const DataTableStub = {
  props: ['columns', 'data'],
  template: `
    <div data-test="data-table">
      <template v-for="column in columns" :key="column.key">
        <span :data-column-key="column.key">{{ column.label }}</span>
        <div v-if="column.key === 'usage'" data-test="usage-header">
          <slot :name="'header-' + column.key" :column="column" />
        </div>
        <div v-if="column.key === 'account_balance' && data.length" data-test="upstream-connection-balance-cell">
          <slot name="cell-account_balance" :row="data[0]" />
        </div>
        <div v-if="column.key === 'rate_multiplier' && data.length" data-test="upstream-multiplier-sync-cell">
          <slot name="cell-rate_multiplier" :row="data[0]" />
        </div>
      </template>
      <div v-for="row in data" :key="row.id" data-test="account-rate">
        <slot name="cell-rate_multiplier" :row="row" />
      </div>
    </div>
  `
}

// Expose the content passed to HelpTooltip without dealing with its <Teleport>.
const HelpTooltipStub = {
  props: ['content', 'widthClass'],
  template: '<span data-test="usage-windows-hint">{{ content }}</span>'
}

function mountView() {
  return mount(AccountsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
        },
        DataTable: DataTableStub,
        HelpTooltip: HelpTooltipStub,
        Pagination: true,
        ConfirmDialog: true,
        AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
        AccountTableFilters: { template: '<div></div>' },
        AccountBulkActionsBar: true,
        AccountActionMenu: true,
        ImportDataModal: true,
        ReAuthAccountModal: true,
        AccountTestModal: true,
        AccountStatsModal: true,
        ScheduledTestsPanel: true,
        SyncFromCrsModal: true,
        TempUnschedStatusModal: true,
        ErrorPassthroughRulesModal: true,
        TLSFingerprintProfilesModal: true,
        CreateAccountModal: true,
        EditAccountModal: true,
        BulkEditAccountModal: true,
        PlatformTypeBadge: true,
        AccountCapacityCell: true,
        AccountStatusIndicator: true,
        AccountTodayStatsCell: true,
        AccountGroupsCell: true,
        AccountUsageCell: true,
        UpstreamMultiplierSyncCell: {
          props: ['binding', 'connectionName'],
          template: '<div data-test="upstream-multiplier-sync-status">{{ binding?.status ?? "unbound" }}|{{ connectionName ?? "" }}</div>'
        },
        Icon: true
      }
    }
  })
}

describe('admin AccountsView usage windows hint', () => {
  beforeEach(() => {
    localStorage.clear()

    listAccounts.mockReset()
    listWithEtag.mockReset()
    getBatchTodayStats.mockReset()
    getAllProxies.mockReset()
    getAllGroups.mockReset()
    listUpstreamConnections.mockReset()
    probeUpstreamConnection.mockReset()
    pauseAutoRefresh.mockReset()
    resumeAutoRefresh.mockReset()
    intervalCallback.current = null

    listAccounts.mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      page_size: 20,
      pages: 0
    })
    listWithEtag.mockResolvedValue({
      notModified: true,
      etag: null,
      data: null
    })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    listUpstreamConnections.mockResolvedValue([])
  })

  it('registers viewport listeners before asynchronous account loading completes', async () => {
    listAccounts.mockReturnValue(new Promise(() => undefined))
    const addEventListener = vi.spyOn(window, 'addEventListener')
    const wrapper = mountView()

    try {
      await flushPromises()

      expect(addEventListener).toHaveBeenCalledWith('scroll', expect.any(Function), true)
      expect(addEventListener).toHaveBeenCalledWith('resize', expect.any(Function))
    } finally {
      wrapper.unmount()
      addEventListener.mockRestore()
    }
  })

  it('does not restart auto refresh when initial loading finishes after unmount', async () => {
    localStorage.setItem('account-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 5 }))
    const documentHidden = vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)

    try {
      let resolveAccounts!: (value: {
        items: never[]
        total: number
        page: number
        page_size: number
        pages: number
      }) => void
      listAccounts.mockReturnValueOnce(new Promise(resolve => {
        resolveAccounts = resolve
      }))

      const wrapper = mountView()
      await flushPromises()
      wrapper.unmount()

      resolveAccounts({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
      await flushPromises()
      await intervalCallback.current?.()

      expect(pauseAutoRefresh).toHaveBeenCalled()
      expect(resumeAutoRefresh).not.toHaveBeenCalled()
      expect(listWithEtag).not.toHaveBeenCalled()
      expect(getBatchTodayStats).not.toHaveBeenCalled()
      expect(listUpstreamConnections).not.toHaveBeenCalled()
      expect(getAllProxies).not.toHaveBeenCalled()
      expect(getAllGroups).not.toHaveBeenCalled()
    } finally {
      documentHidden.mockRestore()
    }
  })

  it('renders an explanatory tooltip next to the usage windows column header', async () => {
    const wrapper = mountView()
    await flushPromises()

    const header = wrapper.find('[data-test="usage-header"]')
    expect(header.exists()).toBe(true)
    // Column label is still shown alongside the help icon.
    expect(header.text()).toContain('admin.accounts.columns.usageWindows')

    const hint = wrapper.find('[data-test="usage-windows-hint"]')
    expect(hint.exists()).toBe(true)
    expect(hint.text()).toBe('admin.accounts.usageWindowsHint')
  })

  it('keeps Ollama Cloud in the single usage column and ignores legacy column preferences', async () => {
    localStorage.setItem('account-hidden-columns', JSON.stringify(['ollama_cloud_usage']))
    const wrapper = mountView()
    await flushPromises()

    const columns = wrapper.getComponent(DataTableStub).props('columns') as Array<{ key: string }>
    expect(columns.filter(column => column.key === 'usage')).toHaveLength(1)
    expect(columns.some(column => column.key === 'ollama_cloud_usage')).toBe(false)
  })

  it('does not expose the legacy declared-rate column', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-column-key="upstream_billing_rate"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('admin.accounts.columns.upstreamBillingRate')
  })

  it('maps one shared wallet to its bound account and refreshes by connection id', async () => {
    listAccounts.mockResolvedValue({
      items: [{ id: 17, name: 'bound account', platform: 'openai', type: 'apikey' }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    const sharedConnection = {
      id: 7,
      name: 'Primary upstream',
      status: 'ready',
      last_error: '',
      wallet_amount: 12.5,
      wallet_currency: 'USD',
      wallet_usd: 12.5,
      wallet_unlimited: false,
      bound_account_ids: [17],
      bindings: []
    }
    listUpstreamConnections.mockResolvedValue([sharedConnection])
    probeUpstreamConnection.mockResolvedValue({ ...sharedConnection, wallet_amount: 11.25, wallet_usd: 11.25 })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="upstream-connection-balance"]').text()).toBe('$12.50')
    await wrapper.get('[data-testid="upstream-connection-balance-refresh"]').trigger('click')
    await flushPromises()

    expect(probeUpstreamConnection).toHaveBeenCalledWith(7)
    expect(wrapper.get('[data-testid="upstream-connection-balance"]').text()).toBe('$11.25')
  })

  it('passes requested binding details to the multiplier sync cell', async () => {
    localStorage.setItem('account-hidden-columns', JSON.stringify([]))
    listAccounts.mockResolvedValue({
      items: [{ id: 17, name: 'bound account', platform: 'openai', type: 'apikey', rate_multiplier: 0.08 }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    listUpstreamConnections.mockResolvedValue([{
      id: 7,
      name: 'Primary upstream',
      status: 'ready',
      last_error: '',
      wallet_amount: 12.5,
      wallet_currency: 'USD',
      wallet_usd: 12.5,
      wallet_unlimited: false,
      bound_account_ids: [17],
      bindings: [{ account_id: 17, connection_id: 7, status: 'ready', observed_multiplier: 0.08, fresh_until: '2099-01-01T00:00:00Z' }]
    }])

    const wrapper = mountView()
    await flushPromises()

    expect(listUpstreamConnections).toHaveBeenCalledWith({ includeBindings: true })
    expect(wrapper.get('[data-test="upstream-multiplier-sync-status"]').text()).toBe('ready|Primary upstream')
  })

  it('distinguishes a shared-connection load failure from an unbound account', async () => {
    listAccounts.mockResolvedValue({
      items: [{ id: 17, name: 'unknown binding', platform: 'openai', type: 'apikey' }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    listUpstreamConnections.mockRejectedValue(new Error('network unavailable'))
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    try {
      const wrapper = mountView()
      await flushPromises()

      expect(wrapper.text()).toContain('admin.accounts.upstreamConnectionBalance.loadFailed')
    } finally {
      consoleError.mockRestore()
    }
  })
})
