import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ModelPricesView from '../ModelPricesView.vue'
import type { ModelPriceModel, ModelPriceResponse } from '@/api/modelPrices'

const { apiMock, appStoreMock, authStoreMock } = vi.hoisted(() => ({
  apiMock: {
    getModelPrices: vi.fn(),
    syncCatalog: vi.fn(),
    updateCustomPrice: vi.fn(),
    updateHiddenGroup: vi.fn(),
    updateHiddenGroups: vi.fn(),
    updateHiddenModel: vi.fn(),
    updateHiddenModels: vi.fn(),
  },
  appStoreMock: {
    showError: vi.fn(),
    showSuccess: vi.fn(),
  },
  authStoreMock: {
    isAdmin: true,
  },
}))

vi.mock('@/api/modelPrices', () => ({
  default: apiMock,
  modelPricesAPI: apiMock,
}))

vi.mock('@/stores', () => ({
  useAppStore: () => appStoreMock,
  useAuthStore: () => authStoreMock,
}))

function model(overrides: Partial<ModelPriceModel> = {}): ModelPriceModel {
  return {
    name: 'gpt-5.5',
    platform: 'openai',
    provider: 'openai',
    billing_mode: 'token',
    pricing_source: 'official',
    official: {
      input_usd_per_m: 3,
      output_usd_per_m: 12,
      cache_write_usd_per_m: null,
      cache_read_usd_per_m: null,
      image_output_usd_per_m: null,
      per_request_usd: null,
    },
    actual: {
      input_usd_per_m: 0.6,
      input_cny_per_m: 4.2,
      output_usd_per_m: 2.4,
      output_cny_per_m: 16.8,
      cache_write_usd_per_m: null,
      cache_write_cny_per_m: null,
      cache_read_usd_per_m: null,
      cache_read_cny_per_m: null,
      image_output_usd_per_m: null,
      image_output_cny_per_m: null,
      per_request_usd: null,
      per_request_cny: null,
    },
    price_tiers: [],
    multiplier: 0.2,
    cheaper_factor: 5,
    channel_names: ['openai-main'],
    official_missing: false,
    custom_price: null,
    hidden: false,
    ...overrides,
  }
}

function response(overrides: Partial<ModelPriceResponse> = {}): ModelPriceResponse {
  return {
    usd_cny_rate: 7,
    cny_per_quota_usd: 0.068,
    groups: [
      {
        id: 46,
        name: 'OpenAI 256',
        platform: 'openai',
        subscription_type: 'subscription',
        rate_multiplier: 1,
        effective_multiplier: 0.2,
        image_rate_independent: false,
        image_rate_multiplier: 1,
        is_exclusive: false,
        hidden: false,
        model_count: 2,
        channel_count: 1,
        best_plan: {
          id: 1,
          name: '旗舰套餐',
          price_cny: 700,
          quota_usd: 500,
          cny_per_quota_usd: 1.4,
          usd_multiplier: 0.2,
        },
      },
      {
        id: 47,
        name: 'Claude 200',
        platform: 'anthropic',
        subscription_type: 'subscription',
        rate_multiplier: 1,
        effective_multiplier: 0.25,
        image_rate_independent: false,
        image_rate_multiplier: 1,
        is_exclusive: false,
        hidden: false,
        model_count: 1,
        channel_count: 1,
      },
    ],
    group_overview: [
      { category: 'openai', group_count: 1, model_count: 2, channel_count: 1 },
      { category: 'claude', group_count: 1, model_count: 1, channel_count: 1 },
    ],
    selected_group_id: null,
    models: [],
    summary: {
      model_count: 0,
      priced_count: 0,
      average_cheaper_factor: null,
    },
    catalog_status: {
      model_count: 2,
      last_updated: '2026-06-28T00:00:00Z',
      local_hash: 'test',
    },
    include_catalog: false,
    show_hidden_groups: false,
    show_hidden_models: false,
    hidden_group_ids: [],
    hidden_model_keys: [],
    selected_group_hidden: false,
    ...overrides,
  }
}

function mountView() {
  return mount(ModelPricesView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Icon: true,
        RouterLink: { template: '<a><slot /></a>' },
      },
    },
  })
}

describe('ModelPricesView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    authStoreMock.isAdmin = true
    apiMock.updateHiddenGroups.mockResolvedValue({ hidden_group_ids: [46] })
    apiMock.updateHiddenModels.mockResolvedValue({ hidden_model_keys: ['46:gpt-5.5'] })
    apiMock.syncCatalog.mockResolvedValue({ model_count: 2 })
  })

  it('supports polished model batch selection and hide action', async () => {
    apiMock.getModelPrices
      .mockResolvedValueOnce(response())
      .mockResolvedValueOnce(response({
        selected_group_id: 46,
        models: [model(), model({ name: 'gpt-5' })],
        summary: { model_count: 2, priced_count: 2, average_cheaper_factor: 5 },
      }))
      .mockResolvedValueOnce(response({
        selected_group_id: 46,
        models: [model({ name: 'gpt-5' })],
        hidden_model_keys: ['46:gpt-5.5'],
        summary: { model_count: 1, priced_count: 1, average_cheaper_factor: 5 },
      }))

    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="group-pill-46"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="model-bulk-panel"]').text()).toContain('勾选模型后批量隐藏或恢复')

    await wrapper.get('[data-testid="model-select-gpt-5.5"]').setValue(true)

    expect(wrapper.get('[data-testid="model-bulk-panel"]').text()).toContain('已选 1 个模型')

    await wrapper.get('[data-testid="model-bulk-hide"]').trigger('click')
    await flushPromises()

    expect(apiMock.updateHiddenModels).toHaveBeenCalledWith(46, ['gpt-5.5'], true)
    expect(appStoreMock.showSuccess).toHaveBeenCalled()
  })

  it('can show hidden models and batch restore them', async () => {
    apiMock.getModelPrices
      .mockResolvedValueOnce(response())
      .mockResolvedValueOnce(response({
        selected_group_id: 46,
        models: [model({ hidden: true })],
        hidden_model_keys: ['46:gpt-5.5'],
        show_hidden_models: true,
        summary: { model_count: 1, priced_count: 1, average_cheaper_factor: 5 },
      }))
      .mockResolvedValueOnce(response({
        selected_group_id: 46,
        models: [model({ hidden: true })],
        hidden_model_keys: ['46:gpt-5.5'],
        show_hidden_models: true,
        summary: { model_count: 1, priced_count: 1, average_cheaper_factor: 5 },
      }))
      .mockResolvedValueOnce(response({
        selected_group_id: 46,
        models: [model()],
        hidden_model_keys: [],
        summary: { model_count: 1, priced_count: 1, average_cheaper_factor: 5 },
      }))

    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="group-pill-46"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="model-hidden-only-filter"]').trigger('click')
    await flushPromises()

    expect(apiMock.getModelPrices).toHaveBeenLastCalledWith(expect.objectContaining({
      group_id: 46,
      show_hidden_models: true,
    }))
    expect(wrapper.find('[data-model-name="gpt-5.5"]').exists()).toBe(true)

    await wrapper.get('[data-testid="model-select-gpt-5.5"]').setValue(true)
    await wrapper.get('[data-testid="model-bulk-restore"]').trigger('click')
    await flushPromises()

    expect(apiMock.updateHiddenModels).toHaveBeenCalledWith(46, ['gpt-5.5'], false)
  })

  it('supports group batch selection and hide action', async () => {
    apiMock.getModelPrices
      .mockResolvedValueOnce(response())
      .mockResolvedValueOnce(response({
        groups: [
          { ...response().groups[0], hidden: true },
          response().groups[1],
        ],
        hidden_group_ids: [46],
      }))

    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="group-select-46"]').setValue(true)

    expect(wrapper.get('[data-testid="group-bulk-panel"]').text()).toContain('已选 1 个')

    await wrapper.get('[data-testid="group-bulk-hide"]').trigger('click')
    await flushPromises()

    expect(apiMock.updateHiddenGroups).toHaveBeenCalledWith([46])
  })
})
