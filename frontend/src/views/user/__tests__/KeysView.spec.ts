import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import KeysView from '../KeysView.vue'
import type { ApiKey, AvailableGroup } from '@/types'

const apiMocks = vi.hoisted(() => ({
  list: vi.fn(),
  createWithPayload: vi.fn(),
  update: vi.fn(),
  deleteKey: vi.fn(),
  toggleStatus: vi.fn(),
  getAvailable: vi.fn(),
  getUserGroupRates: vi.fn(),
  getDashboardApiKeysUsage: vi.fn(),
  getPublicSettings: vi.fn(),
  createLaunchTicket: vi.fn(),
}))

const storeMocks = vi.hoisted(() => ({
  showSuccess: vi.fn(),
  showError: vi.fn(),
  showInfo: vi.fn(),
  nextStep: vi.fn(),
  isCurrentStep: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/keys', query: {} }),
  useRouter: () => ({
    push: vi.fn(),
    replace: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (params && 'amount' in params && 'rate' in params) return `${key}:${String(params.rate)}:${String(params.amount)}`
        if (params && 'amount' in params) return `${key}:${String(params.amount)}`
        if (params && 'remaining' in params && 'total' in params) return `${key}:${String(params.remaining)}:${String(params.total)}`
        if (params && 'total' in params) return `${key}:${String(params.total)}`
        return key
      },
    }),
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess: storeMocks.showSuccess,
    showError: storeMocks.showError,
    showInfo: storeMocks.showInfo,
  }),
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({
    isCurrentStep: storeMocks.isCurrentStep,
    nextStep: storeMocks.nextStep,
  }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: vi.fn().mockResolvedValue(true),
  }),
}))

vi.mock('@/composables/useCurrencyResolver', () => ({
  useCurrencyResolver: () => ({
    convertUsdToCnyForLog: (val: number) => (val != null ? val * 7 : null),
    convertUsdToCny: (val: number) => (val != null ? val * 7 : null),
    formatCny: (val: number | null | undefined) => (val != null ? `¥${val.toFixed(2)}` : '-'),
  })
}))

vi.mock('@/api', () => ({
  keysAPI: {
    list: apiMocks.list,
    createWithPayload: apiMocks.createWithPayload,
    update: apiMocks.update,
    delete: apiMocks.deleteKey,
    toggleStatus: apiMocks.toggleStatus,
  },
  userGroupsAPI: {
    getAvailable: apiMocks.getAvailable,
    getUserGroupRates: apiMocks.getUserGroupRates,
  },
  usageAPI: {
    getDashboardApiKeysUsage: apiMocks.getDashboardApiKeysUsage,
  },
  authAPI: {
    getPublicSettings: apiMocks.getPublicSettings,
  },
  lobehubAPI: {
    createLaunchTicket: apiMocks.createLaunchTicket,
  },
}))

const LayoutStub = defineComponent({ template: '<div><slot /></div>' })
const TablePageLayoutStub = defineComponent({
  template: '<div><slot name="filters" /><slot name="actions" /><slot name="table" /><slot name="pagination" /></div>',
})
const BaseDialogStub = defineComponent({
  props: { show: Boolean },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})
const DataTableStub = defineComponent({
  name: 'DataTable',
  props: {
    columns: { type: Array, default: () => [] },
    data: { type: Array, default: () => [] },
  },
  emits: ['sort'],
  template: `
    <div>
      <div data-test="columns">{{ columns.map((col) => col.key).join(',') }}</div>
      <div data-test="columns-meta">{{ JSON.stringify(columns.map((col) => ({ key: col.key, sortable: !!col.sortable }))) }}</div>
      <button data-test="sort-current-concurrency" @click="$emit('sort', 'current_concurrency', 'asc')">
        Sort Current Concurrency
      </button>
      <div v-for="row in data" :key="row.id">
        <div
          v-if="columns.some((col) => col.key === 'id')"
          data-test="key-id"
        >
          <slot name="cell-id" :value="row.id" :row="row" />
        </div>
        <slot name="cell-name" :value="row.name" :row="row" />
        <div data-test="current-concurrency">
          <slot name="cell-current_concurrency" :value="row.current_concurrency" :row="row" />
        </div>
        <div v-if="columns.some((col) => col.key === 'last_used_ip')" data-test="last-used-ip">
          <slot name="cell-last_used_ip" :value="row.last_used_ip" :row="row" />
        </div>
      </div>
      <slot name="empty" />
    </div>
  `,
})
const SelectStub = defineComponent({
  name: 'AppSelect',
  props: {
    modelValue: [String, Number, Boolean, null],
    options: { type: Array, default: () => [] },
  },
  emits: ['update:modelValue', 'change'],
  template: '<select><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>',
})
const SearchInputStub = defineComponent({
  name: 'SearchInput',
  props: ['modelValue'],
  emits: ['update:modelValue', 'search'],
  template: `<input :value="modelValue" @input="$emit('update:modelValue', $event.target.value)" />`,
})
const PaginationStub = defineComponent({
  name: 'Pagination',
  props: ['page', 'total', 'pageSize'],
  emits: ['update:page', 'update:pageSize'],
  template: `<button data-test="page-size-50" @click="$emit('update:pageSize', 50)">50</button>`,
})

function baseGroup(overrides: Partial<AvailableGroup>): AvailableGroup {
  return {
    id: 1,
    name: 'Default',
    description: null,
    platform: 'anthropic',
    rate_multiplier: 1,
    is_exclusive: false,
    status: 'active',
    subscription_type: 'standard',
    balance_enabled: true,
    subscription_enabled: false,
    plan_auto_grant_enabled: false,
    daily_limit_usd: null,
    weekly_limit_usd: null,
    monthly_limit_usd: null,
    allow_image_generation: false,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    image_price_1k: null,
    image_price_2k: null,
    image_price_4k: null,
    claude_code_only: false,
    fallback_group_id: null,
    fallback_group_id_on_invalid_request: null,
    require_oauth_only: false,
    require_privacy_set: false,
    created_at: '2026-06-01T00:00:00Z',
    updated_at: '2026-06-01T00:00:00Z',
    access_sources: [
      { type: 'balance', label: 'Balance access', name: 'Balance access' },
    ],
    ...overrides,
  }
}

function subscriptionGroup(
  entitlementIDs: number[],
  overrides: Partial<AvailableGroup> = {}
): AvailableGroup {
  const groupID = overrides.id ?? 20
  return baseGroup({
    id: groupID,
    name: 'Subscription Claude',
    subscription_type: 'subscription',
    balance_enabled: false,
    subscription_enabled: true,
    plan_auto_grant_enabled: true,
    entitlements: entitlementIDs.map((id) => ({
      id,
      name: `Plan ${id}`,
      plan_id: id + 1000,
      primary_group_id: groupID,
      starts_at: '2026-06-01T00:00:00Z',
      expires_at: '2026-07-01T00:00:00Z',
      purchase_price: 29.9,
      purchase_currency: 'CNY',
      quota_usd: 10,
      quota_used_usd: 2.5,
      quota_period: 'monthly',
      unit_cost_per_usd: 2.99,
      overage_policy: 'block',
    })),
    access_sources: entitlementIDs.map((id) => ({
      type: 'entitlement',
      label: `Plan ${id}`,
      name: `Plan ${id}`,
      entitlement_id: id,
      plan_id: id + 1000,
      purchase_price: 29.9,
      purchase_currency: 'CNY',
      quota_usd: 10,
      quota_used_usd: 2.5,
      quota_period: 'monthly',
      unit_cost_per_usd: 2.99,
      overage_policy: 'block',
      expires_at: '2026-07-01T00:00:00Z',
    })),
    ...overrides,
  })
}

function keyFixture(overrides: Partial<ApiKey> = {}): ApiKey {
  return {
    id: 1,
    user_id: 1,
    key: 'sk-test-key',
    name: 'Existing key',
    group_id: 10,
    subscription_entitlement_id: null,
    access_source: 'balance',
    auto_switch_group_enabled: true,
    status: 'active',
    ip_whitelist: [],
    ip_blacklist: [],
    last_used_at: null,
    last_used_ip: null,
    quota: 0,
    quota_used: 0,
    expires_at: null,
    created_at: '2026-06-01T00:00:00Z',
    updated_at: '2026-06-01T00:00:00Z',
    current_concurrency: 3,
    rate_limit_5h: 0,
    rate_limit_1d: 0,
    rate_limit_7d: 0,
    usage_5h: 0,
    usage_1d: 0,
    usage_7d: 0,
    window_5h_start: null,
    window_1d_start: null,
    window_7d_start: null,
    reset_5h_at: null,
    reset_1d_at: null,
    reset_7d_at: null,
    ...overrides,
  }
}

async function mountView(
  groups: AvailableGroup[] = [baseGroup({ id: 10, name: 'Standard' })],
  keys: ApiKey[] = []
) {
  apiMocks.list.mockResolvedValue({ items: keys, total: keys.length, page: 1, page_size: 10, pages: keys.length > 0 ? 1 : 0 })
  apiMocks.createWithPayload.mockResolvedValue(keyFixture())
  apiMocks.update.mockResolvedValue(keyFixture())
  apiMocks.getAvailable.mockResolvedValue(groups)
  apiMocks.getUserGroupRates.mockResolvedValue({})
  apiMocks.getDashboardApiKeysUsage.mockResolvedValue({ stats: {} })
  apiMocks.getPublicSettings.mockResolvedValue({})

  const wrapper = mount(KeysView, {
    global: {
      stubs: {
        AppLayout: LayoutStub,
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        Pagination: PaginationStub,
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        EmptyState: true,
        Select: SelectStub,
        SearchInput: SearchInputStub,
        Icon: true,
        UseKeyModal: true,
        EndpointPopover: true,
        GroupBadge: true,
        GroupOptionItem: true,
        Teleport: true,
      },
    },
  })
  await flushPromises()
  await nextTick()
  return wrapper
}

function setupState(wrapper: ReturnType<typeof mount>) {
  return (wrapper.vm as any).$?.setupState as Record<string, any>
}

describe('KeysView entitlement group binding', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
    storeMocks.isCurrentStep.mockReturnValue(false)
  })

  it('splits the API key table into entitlement and current group columns', async () => {
    const wrapper = await mountView([subscriptionGroup([101])])
    const vm = setupState(wrapper)

    expect(vm.columns.map((column: { key: string; label: string }) => [column.key, column.label])).toEqual(expect.arrayContaining([
      ['entitlement', 'keys.accessSourceColumn'],
      ['group', 'keys.currentGroupLabel'],
      ['current_concurrency', 'keys.currentConcurrency'],
    ]))
  })

  it('formats current group actual RMB cost with the effective rate', async () => {
    const wrapper = await mountView([subscriptionGroup([101], { rate_multiplier: 3 })])
    const vm = setupState(wrapper)

    expect(vm.entitlementQuotaPeriodTextByID(101)).toBe('keys.quotaPeriod.monthly')
    expect(vm.entitlementActualCostTextByID(101, 3)).toBe('keys.actualRmbCostHint:3x:¥8.97')
    expect(vm.entitlementQuotaRemainingTextByID(101)).toBe('keys.entitlementQuotaTotal:$10.00')
  })

  it('uses entitlement quota metadata from access source fallback', async () => {
    const wrapper = await mountView([
      subscriptionGroup([101], {
        entitlements: [],
      }),
    ], [
      keyFixture({
        group_id: 20,
        access_source: 'entitlement',
        subscription_entitlement_id: 101,
      }),
    ])
    const vm = setupState(wrapper)

    expect(vm.entitlementQuotaRemainingTextByID(101)).toBe('keys.entitlementQuotaTotal:$10.00')
  })

  it('filters selectable groups by the selected entitlement and display order', async () => {
    const wrapper = await mountView([
      subscriptionGroup([201, 202], { id: 20, name: 'Claude 默认组' }),
      subscriptionGroup([202], { id: 30, name: 'Claude 长上下文组' }),
      subscriptionGroup([201], { id: 40, name: 'Claude 备用组' }),
    ])
    const vm = setupState(wrapper)

    vm.handleCreateAction()
    vm.selectAccessSource('entitlement')
    vm.formData.subscription_entitlement_id = 202
    await nextTick()

    expect(vm.formGroupOptions.map((option: { value: number }) => option.value)).toEqual([30, 20])

    vm.formData.group_id = 40
    await nextTick()

    expect(vm.formData.group_id).toBeNull()
    expect(vm.formData.subscription_entitlement_id).toBe(202)
  })

  it('filters quick-switch groups to the key current plan card and display order', async () => {
    const key = keyFixture({
      group_id: 20,
      access_source: 'entitlement',
      subscription_entitlement_id: 202,
    })
    const wrapper = await mountView([
      baseGroup({ id: 10, name: 'Standard' }),
      subscriptionGroup([201, 202], { id: 20, name: 'Claude 默认组' }),
      subscriptionGroup([202], { id: 30, name: 'Claude 长上下文组' }),
      subscriptionGroup([201], { id: 40, name: 'Claude 备用组' }),
    ], [key])
    const vm = setupState(wrapper)

    vm.openGroupSelector(key)
    await nextTick()

    expect(vm.filteredGroupOptions.map((option: { value: number }) => option.value)).toEqual([30, 20])
  })

  it('auto-selects a single entitlement and sends it when creating a key', async () => {
    const wrapper = await mountView([baseGroup({ id: 10, name: 'Standard' }), subscriptionGroup([101])])
    const vm = setupState(wrapper)

    vm.handleCreateAction()
    vm.selectAccessSource('entitlement')
    vm.formData.name = 'Subscription key'
    vm.formData.group_id = 20
    await nextTick()

    expect(vm.formData.subscription_entitlement_id).toBe(101)

    await vm.handleSubmit()

    expect(apiMocks.createWithPayload).toHaveBeenCalledWith(expect.objectContaining({
      name: 'Subscription key',
      group_id: 20,
      access_source: 'entitlement',
      subscription_entitlement_id: 101,
    }))
  })

  it('uses access_sources when legacy entitlements metadata is absent', async () => {
    const wrapper = await mountView([
      subscriptionGroup([301], {
        entitlements: undefined,
        access_sources: [
          {
            type: 'entitlement',
            label: 'Source-only Plan',
            name: 'Source-only Plan',
            entitlement_id: 301,
            plan_id: 1301,
            overage_policy: 'balance_fallback',
            expires_at: '2026-07-01T00:00:00Z',
          },
        ],
      }),
    ])
    const vm = setupState(wrapper)

    vm.handleCreateAction()
    vm.selectAccessSource('entitlement')
    vm.formData.name = 'Source-aware key'
    vm.formData.group_id = 20
    await nextTick()

    expect(vm.formData.subscription_entitlement_id).toBe(301)

    await vm.handleSubmit()
    expect(apiMocks.createWithPayload).toHaveBeenCalledWith(expect.objectContaining({
      group_id: 20,
      access_source: 'entitlement',
      subscription_entitlement_id: 301,
    }))
  })

  it('keeps entitlement groups selectable when access source metadata is incomplete', async () => {
    const wrapper = await mountView([
      subscriptionGroup([401], {
        access_sources: [
          { type: 'balance', label: 'Balance access', name: 'Balance access' },
        ],
      }),
    ])
    const vm = setupState(wrapper)

    vm.editKey(keyFixture({
      group_id: 20,
      access_source: 'entitlement',
      subscription_entitlement_id: 401,
    }))
    await nextTick()

    expect(vm.formGroupOptions.map((option: { value: number }) => option.value)).toEqual([20])
  })

  it('keeps explicitly disabled entitlement sources unavailable', async () => {
    const wrapper = await mountView([
      subscriptionGroup([402], {
        access_sources: [
          {
            type: 'entitlement',
            label: 'Disabled plan',
            name: 'Disabled plan',
            entitlement_id: 402,
            disabled: true,
          },
        ],
      }),
    ])
    const vm = setupState(wrapper)

    vm.editKey(keyFixture({
      group_id: 20,
      access_source: 'entitlement',
      subscription_entitlement_id: 402,
    }))
    await nextTick()

    expect(vm.formGroupOptions).toEqual([])
  })

  it('requires explicit selection when multiple entitlements cover the group', async () => {
    const wrapper = await mountView([subscriptionGroup([201, 202])])
    const vm = setupState(wrapper)

    vm.handleCreateAction()
    vm.selectAccessSource('entitlement')
    vm.formData.name = 'Ambiguous key'
    vm.formData.group_id = 20
    await nextTick()

    expect(wrapper.find('[data-testid="entitlement-selector"]').exists()).toBe(true)
    expect(vm.formData.subscription_entitlement_id).toBeNull()

    await vm.handleSubmit()
    expect(storeMocks.showError).toHaveBeenCalledWith('keys.entitlementRequired')
    expect(apiMocks.createWithPayload).not.toHaveBeenCalled()

    vm.formData.subscription_entitlement_id = 202
    await vm.handleSubmit()
    expect(apiMocks.createWithPayload).toHaveBeenCalledWith(expect.objectContaining({
      group_id: 20,
      access_source: 'entitlement',
      subscription_entitlement_id: 202,
    }))
  })

  it('does not send entitlement id for standard groups or legacy groups without metadata', async () => {
    const wrapper = await mountView([
      baseGroup({ id: 10, name: 'Standard' }),
      baseGroup({
        id: 20,
        name: 'Legacy Sub',
        subscription_type: 'subscription',
        balance_enabled: undefined,
        subscription_enabled: undefined,
        access_sources: undefined,
      }),
    ])
    const vm = setupState(wrapper)

    vm.handleCreateAction()
    vm.formData.name = 'Standard key'
    vm.formData.group_id = 10
    vm.formData.subscription_entitlement_id = 999
    await nextTick()
    await vm.handleSubmit()

    expect(apiMocks.createWithPayload.mock.calls[0][0]).toMatchObject({
      access_source: 'balance',
      subscription_entitlement_id: null,
    })

    apiMocks.createWithPayload.mockClear()
    vm.formData.name = 'Legacy key'
    vm.formData.group_id = 20
    await nextTick()
    await vm.handleSubmit()

    expect(apiMocks.createWithPayload.mock.calls[0][0]).toMatchObject({
      access_source: 'balance',
      subscription_entitlement_id: null,
    })
  })

  it('initializes entitlement id while editing an existing key', async () => {
    const wrapper = await mountView([subscriptionGroup([201, 202])])
    const vm = setupState(wrapper)

    vm.editKey(keyFixture({
      group_id: 20,
      access_source: 'entitlement',
      subscription_entitlement_id: 202,
    }))
    await nextTick()

    expect(vm.formData.group_id).toBe(20)
    expect(vm.formData.access_source).toBe('entitlement')
    expect(vm.formData.subscription_entitlement_id).toBe(202)
  })

  it('quick switch keeps balance keys on balance groups and entitlement keys on the same entitlement', async () => {
    const wrapper = await mountView([subscriptionGroup([101])])
    const vm = setupState(wrapper)

    await vm.changeGroup(keyFixture({ group_id: 10 }), 20)
    expect(apiMocks.update).not.toHaveBeenCalled()
    expect(vm.showEditModal).toBe(true)
    expect(storeMocks.showInfo).toHaveBeenCalledWith('keys.accessSourceSwitchRequired')

    apiMocks.update.mockClear()
    const ambiguousWrapper = await mountView([subscriptionGroup([201, 202])])
    const ambiguousVM = setupState(ambiguousWrapper)

    await ambiguousVM.changeGroup(keyFixture({ group_id: 10 }), 20)

    expect(apiMocks.update).not.toHaveBeenCalled()
    expect(ambiguousVM.showEditModal).toBe(true)
    expect(storeMocks.showInfo).toHaveBeenCalledWith('keys.accessSourceSwitchRequired')

    apiMocks.update.mockClear()
    const preserveWrapper = await mountView([subscriptionGroup([201, 202])])
    const preserveVM = setupState(preserveWrapper)

    await preserveVM.changeGroup(keyFixture({
      group_id: 10,
      access_source: 'entitlement',
      subscription_entitlement_id: 201,
    }), 20)

    expect(apiMocks.update).toHaveBeenCalledWith(1, {
      group_id: 20,
      access_source: 'entitlement',
      subscription_entitlement_id: 201,
    })
  })
})

function visibleColumnKeys(wrapper: VueWrapper): string[] {
  return wrapper.get('[data-test="columns"]').text().split(',').filter(Boolean)
}

function visibleColumnMeta(wrapper: VueWrapper): Array<{ key: string; sortable: boolean }> {
  return JSON.parse(wrapper.get('[data-test="columns-meta"]').text())
}

function getButtonByText(wrapper: VueWrapper, text: string) {
  const button = wrapper.findAll('button').find((item) => item.text().includes(text))
  if (!button) throw new Error(`Button not found: ${text}`)
  return button
}

describe('KeysView column settings', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
    storeMocks.isCurrentStep.mockReturnValue(false)
  })

  it('keeps entitlement columns visible while hiding low-frequency columns by default', async () => {
    const wrapper = await mountView()

    expect(visibleColumnKeys(wrapper)).toEqual([
      'name',
      'key',
      'entitlement',
      'group',
      'current_concurrency',
      'usage',
      'expires_at',
      'status',
      'created_at',
      'actions',
    ])
    expect(visibleColumnKeys(wrapper)).not.toContain('rate_limit')
    expect(visibleColumnKeys(wrapper)).not.toContain('last_used_at')
    expect(visibleColumnKeys(wrapper)).not.toContain('last_used_ip')
    expect(visibleColumnKeys(wrapper)).not.toContain('id')
  })

  it('shows a hidden column when toggled and persists the preference', async () => {
    const wrapper = await mountView()

    await wrapper.get('button[title="keys.columnSettings"]').trigger('click')
    await getButtonByText(wrapper, 'keys.rateLimitColumn').trigger('click')
    await nextTick()

    expect(visibleColumnKeys(wrapper)).toContain('rate_limit')
    expect(localStorage.getItem('api-key-hidden-columns')).toBe(
      JSON.stringify(['id', 'last_used_at', 'last_used_ip'])
    )
    expect(localStorage.getItem('api-key-column-settings-version')).toBe('3')
  })

  it('shows the API key ID column when toggled', async () => {
    const wrapper = await mountView([], [keyFixture()])

    await wrapper.get('button[title="keys.columnSettings"]').trigger('click')
    await getButtonByText(wrapper, 'keys.id').trigger('click')
    await nextTick()

    expect(visibleColumnKeys(wrapper)).toContain('id')
    expect(wrapper.get('[data-test="key-id"]').text()).toBe('#1')
    expect(visibleColumnMeta(wrapper).find((column) => column.key === 'id')?.sortable).toBe(true)
  })

  it('shows the last used IP column when toggled', async () => {
    const wrapper = await mountView([], [keyFixture({ last_used_ip: '203.0.113.10' })])

    await wrapper.get('button[title="keys.columnSettings"]').trigger('click')
    await getButtonByText(wrapper, 'keys.lastUsedIP').trigger('click')
    await nextTick()

    expect(visibleColumnKeys(wrapper)).toContain('last_used_ip')
    expect(wrapper.get('[data-test="last-used-ip"]').text()).toBe('203.0.113.10')
  })

  it('migrates saved preferences without hiding fork entitlement columns', async () => {
    localStorage.setItem('api-key-hidden-columns', JSON.stringify(['group', 'created_at']))
    localStorage.setItem('api-key-column-settings-version', '1')

    const wrapper = await mountView()

    expect(visibleColumnKeys(wrapper)).toEqual([
      'name',
      'key',
      'entitlement',
      'current_concurrency',
      'usage',
      'rate_limit',
      'expires_at',
      'status',
      'last_used_at',
      'actions',
    ])
    expect(localStorage.getItem('api-key-hidden-columns')).toBe(
      JSON.stringify(['group', 'created_at', 'last_used_ip', 'id'])
    )
    expect(localStorage.getItem('api-key-column-settings-version')).toBe('3')
  })

  it('does not include always-visible columns in the toggleable menu', async () => {
    const wrapper = await mountView()

    await wrapper.get('button[title="keys.columnSettings"]').trigger('click')
    await nextTick()

    const columnMenuText = wrapper.text()
    expect(columnMenuText).toContain('keys.apiKey')
    expect(columnMenuText).toContain('keys.accessSourceColumn')
    expect(columnMenuText).toContain('keys.id')
    expect(columnMenuText).toContain('keys.currentConcurrency')
    expect(columnMenuText).toContain('keys.rateLimitColumn')
    expect(columnMenuText).toContain('keys.lastUsedIP')
    expect(columnMenuText).not.toContain('common.name')
    expect(columnMenuText).not.toContain('common.actions')
  })

  it('renders current concurrency and marks it sortable', async () => {
    const wrapper = await mountView([], [keyFixture({ current_concurrency: 3 })])

    expect(wrapper.get('[data-test="current-concurrency"]').text()).toBe('3')
    const currentConcurrencyColumn = visibleColumnMeta(wrapper).find(
      (column) => column.key === 'current_concurrency',
    )
    expect(currentConcurrencyColumn?.sortable).toBe(true)
  })

  it('keeps filters and selected page size when sorting by current concurrency', async () => {
    const wrapper = await mountView([baseGroup({ id: 42, name: 'OpenAI' })], [keyFixture()])

    await wrapper.get('[data-test="page-size-50"]').trigger('click')
    await flushPromises()
    await wrapper.findComponent({ name: 'SearchInput' }).vm.$emit('update:modelValue', 'target')
    await wrapper.findComponent({ name: 'SearchInput' }).vm.$emit('search')
    await flushPromises()

    const selects = wrapper.findAllComponents(SelectStub)
    await selects[0].vm.$emit('update:modelValue', 42)
    await flushPromises()
    await selects[1].vm.$emit('update:modelValue', 'active')
    await flushPromises()
    apiMocks.list.mockClear()

    await wrapper.get('[data-test="sort-current-concurrency"]').trigger('click')
    await flushPromises()

    expect(apiMocks.list).toHaveBeenLastCalledWith(
      1,
      50,
      {
        search: 'target',
        status: 'active',
        group_id: 42,
        sort_by: 'current_concurrency',
        sort_order: 'asc',
      },
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
  })
})
