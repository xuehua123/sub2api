import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
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
      t: (key: string) => key,
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
const DataTableStub = defineComponent({ template: '<div><slot name="empty" /></div>' })
const SelectStub = defineComponent({
  props: {
    modelValue: [String, Number, Boolean, null],
    options: { type: Array, default: () => [] },
  },
  emits: ['update:modelValue', 'change'],
  template: '<select><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>',
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
    ...overrides,
  }
}

function subscriptionGroup(entitlementIDs: number[]): AvailableGroup {
  return baseGroup({
    id: 20,
    name: 'Subscription Claude',
    subscription_type: 'subscription',
    entitlements: entitlementIDs.map((id) => ({
      id,
      name: `Plan ${id}`,
      plan_id: id + 1000,
      primary_group_id: 20,
      starts_at: '2026-06-01T00:00:00Z',
      expires_at: '2026-07-01T00:00:00Z',
    })),
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
    auto_switch_group_enabled: true,
    status: 'active',
    ip_whitelist: [],
    ip_blacklist: [],
    last_used_at: null,
    quota: 0,
    quota_used: 0,
    expires_at: null,
    created_at: '2026-06-01T00:00:00Z',
    updated_at: '2026-06-01T00:00:00Z',
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

async function mountView(groups: AvailableGroup[] = [baseGroup({ id: 10, name: 'Standard' })]) {
  apiMocks.list.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 10, pages: 0 })
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
        Pagination: true,
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        EmptyState: true,
        Select: SelectStub,
        SearchInput: true,
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
    vi.clearAllMocks()
    storeMocks.isCurrentStep.mockReturnValue(false)
  })

  it('auto-selects a single entitlement and sends it when creating a key', async () => {
    const wrapper = await mountView([baseGroup({ id: 10, name: 'Standard' }), subscriptionGroup([101])])
    const vm = setupState(wrapper)

    vm.handleCreateAction()
    vm.formData.name = 'Subscription key'
    vm.formData.group_id = 20
    await nextTick()

    expect(vm.formData.subscription_entitlement_id).toBe(101)

    await vm.handleSubmit()

    expect(apiMocks.createWithPayload).toHaveBeenCalledWith(expect.objectContaining({
      name: 'Subscription key',
      group_id: 20,
      subscription_entitlement_id: 101,
    }))
  })

  it('requires explicit selection when multiple entitlements cover the group', async () => {
    const wrapper = await mountView([subscriptionGroup([201, 202])])
    const vm = setupState(wrapper)

    vm.handleCreateAction()
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
      subscription_entitlement_id: 202,
    }))
  })

  it('does not send entitlement id for standard groups or legacy groups without metadata', async () => {
    const wrapper = await mountView([
      baseGroup({ id: 10, name: 'Standard' }),
      baseGroup({ id: 20, name: 'Legacy Sub', subscription_type: 'subscription' }),
    ])
    const vm = setupState(wrapper)

    vm.handleCreateAction()
    vm.formData.name = 'Standard key'
    vm.formData.group_id = 10
    vm.formData.subscription_entitlement_id = 999
    await nextTick()
    await vm.handleSubmit()

    expect(apiMocks.createWithPayload.mock.calls[0][0]).not.toHaveProperty('subscription_entitlement_id')

    apiMocks.createWithPayload.mockClear()
    vm.formData.name = 'Legacy key'
    vm.formData.group_id = 20
    await nextTick()
    await vm.handleSubmit()

    expect(apiMocks.createWithPayload.mock.calls[0][0]).not.toHaveProperty('subscription_entitlement_id')
  })

  it('initializes entitlement id while editing an existing key', async () => {
    const wrapper = await mountView([subscriptionGroup([201, 202])])
    const vm = setupState(wrapper)

    vm.editKey(keyFixture({ group_id: 20, subscription_entitlement_id: 202 }))
    await nextTick()

    expect(vm.formData.group_id).toBe(20)
    expect(vm.formData.subscription_entitlement_id).toBe(202)
  })

  it('quick switch sends single entitlement but opens edit flow for ambiguous groups', async () => {
    const wrapper = await mountView([subscriptionGroup([101])])
    const vm = setupState(wrapper)

    await vm.changeGroup(keyFixture({ group_id: 10 }), 20)
    expect(apiMocks.update).toHaveBeenCalledWith(1, {
      group_id: 20,
      subscription_entitlement_id: 101,
    })

    apiMocks.update.mockClear()
    const ambiguousWrapper = await mountView([subscriptionGroup([201, 202])])
    const ambiguousVM = setupState(ambiguousWrapper)

    await ambiguousVM.changeGroup(keyFixture({ group_id: 10 }), 20)

    expect(apiMocks.update).not.toHaveBeenCalled()
    expect(ambiguousVM.showEditModal).toBe(true)
    expect(ambiguousVM.formData.group_id).toBe(20)
    expect(ambiguousVM.formData.subscription_entitlement_id).toBeNull()
    expect(storeMocks.showInfo).toHaveBeenCalledWith('keys.entitlementSelectionRequired')
  })
})
