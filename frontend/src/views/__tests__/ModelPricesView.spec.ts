import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ModelPricesView from '../ModelPricesView.vue'
import ModelPriceEditor from '@/components/model-price/ModelPriceEditor.vue'
import { model, response } from '@/components/model-price/__tests__/fixtures'
const { api, app, auth, settings } = vi.hoisted(() => ({
  api: {
    getModelPrices: vi.fn(),
    updateHiddenGroups: vi.fn(),
    updateHiddenModels: vi.fn(),
    updateCustomPrice: vi.fn(),
    syncCatalog: vi.fn(),
  },
  app: { showError: vi.fn(), showSuccess: vi.fn() },
  auth: { isAdmin: true },
  settings: vi.fn(),
}))
vi.mock('@/api/modelPrices', () => ({ default: api }))
vi.mock('@/api/admin', () => ({
  adminAPI: { settings: { updateSettings: settings } },
}))
vi.mock('@/stores', () => ({
  useAppStore: () => app,
  useAuthStore: () => auth,
}))
const mounts: ReturnType<typeof mount>[] = []
function mountView() {
  const w = mount(ModelPricesView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Icon: true,
        ConfirmDialog: true,
        RouterLink: { template: '<a><slot /></a>' },
        BaseDialog: {
          props: ['show'],
          template: '<div v-if="show"><slot/><slot name="footer"/></div>',
        },
      },
    },
  })
  mounts.push(w)
  return w
}
async function enter(w: ReturnType<typeof mount>, manage = false) {
  await flushPromises()
  await w.get('[data-testid="quick-group-46"]').trigger('click')
  await flushPromises()
  if (manage) await w.get('[data-testid="price-manage-tab"]').trigger('click')
  await flushPromises()
}
beforeEach(() => {
  vi.clearAllMocks()
  auth.isAdmin = true
  api.getModelPrices.mockImplementation(async (p) =>
    response({
      selected_group_id: p.group_id || null,
      models: p.group_id
        ? [model(), model({ name: 'second', pricing_source: 'fallback', catalog_price: null })]
        : [],
    }),
  )
  api.updateHiddenModels.mockResolvedValue({})
  api.updateHiddenGroups.mockResolvedValue({})
  api.updateCustomPrice.mockResolvedValue({})
})
afterEach(() => {
  mounts.splice(0).forEach((w) => w.unmount())
})
describe('model price workspaces', () => {
  it('shows official USD values rather than discounted CNY values', async () => {
    const w=mountView();await enter(w)
    const row=w.get('[data-model-name="test-model"] .price-main-values')
    expect(row.text()).toContain('US$3')
    expect(row.text()).toContain('US$12')
    expect(row.text()).not.toContain('4.2')
    expect(row.text()).not.toContain('16.8')
  })
  it('defaults to browsing and keeps admin controls in management', async () => {
    const w = mountView()
    await enter(w)
    expect(w.find('[data-testid="price-operations"]').exists()).toBe(false)
    expect(w.find('[data-testid="model-bulk-panel"]').exists()).toBe(false)
    expect(w.findAll('[data-testid="model-price-row"]')).toHaveLength(2)
  })
  it('requires target confirmation before hiding models', async () => {
    const w = mountView()
    await enter(w, true)
    await w.get('[data-testid="model-select-test-model"]').setValue(true)
    await w.get('[data-testid="model-bulk-hide"]').trigger('click')
    expect(api.updateHiddenModels).not.toHaveBeenCalled()
    await w.get('[data-testid="confirm-visibility"]').trigger('click')
    await flushPromises()
    expect(api.updateHiddenModels).toHaveBeenCalledWith(
      46,
      ['test-model'],
      true,
    )
  })
  it('keeps group hiding and restoring available', async () => {
    const w = mountView()
    await flushPromises()
    await w.get('[data-testid="price-manage-tab"]').trigger('click')
    await w.get('[data-testid="group-select-46"]').setValue(true)
    await w.get('[data-testid="group-bulk-hide"]').trigger('click')
    await w.get('[data-testid="confirm-visibility"]').trigger('click')
    await flushPromises()
    expect(api.updateHiddenGroups).toHaveBeenCalledWith([46])
  })
  it('restores hidden models without changing calling permissions', async () => {
    api.getModelPrices.mockImplementation(async (p) =>
      response({
        selected_group_id: p.group_id || null,
        models: p.group_id ? [model({ hidden: true })] : [],
      }),
    )
    const w = mountView()
    await enter(w, true)
    await w.get('[data-testid="model-hidden-only-filter"]').setValue(true)
    await flushPromises()
    expect(api.getModelPrices).toHaveBeenLastCalledWith(
      expect.objectContaining({ show_hidden_models: true }),
    )
    await w.get('[data-testid="model-select-test-model"]').setValue(true)
    await w.get('[data-testid="model-bulk-restore"]').trigger('click')
    await w.get('[data-testid="confirm-visibility"]').trigger('click')
    await flushPromises()
    expect(api.updateHiddenModels).toHaveBeenCalledWith(
      46,
      ['test-model'],
      false,
    )
  })
  it('filters source, search and unit without duplicate mobile rows', async () => {
    const w = mountView()
    await enter(w)
    await w.get('[data-testid="price-source-filter"]').setValue('unknown')
    expect(w.findAll('[data-testid="model-price-row"]')).toHaveLength(1)
    expect(w.text()).toContain('官方原价缺失')
    await w.get('[data-testid="model-search"]').setValue('absent')
    expect(w.findAll('[data-testid="model-price-row"]')).toHaveLength(0)
  })
  it('preserves explicit zero in the editor and previews before saving', async () => {
    const zero = model({ billing_mode: 'video' })
    zero.actual.per_request_usd = 0
    zero.actual.per_request_cny = 0
    api.getModelPrices.mockImplementation(async (p) =>
      response({
        selected_group_id: p.group_id || null,
        models: p.group_id ? [zero] : [],
      }),
    )
    const w = mountView()
    await enter(w, true)
    expect(w.text()).toContain('每秒')
    await w.get('[aria-label="编辑展示价"]').trigger('click')
    expect(
      w
        .getComponent(ModelPriceEditor)
        .get('[data-testid="price-editor-per_request_usd"]').element,
    ).toHaveProperty('value', '0')
    await w.get('#model-price-edit').trigger('submit')
    expect(api.updateCustomPrice).not.toHaveBeenCalled()
    await w
      .findAll('button')
      .find((b) => b.text() === '确认保存展示价')!
      .trigger('click')
    await flushPromises()
    expect(api.updateCustomPrice).toHaveBeenCalledWith(
      expect.objectContaining({
        group_id: 46,
        billing_mode: 'video',
        per_request_usd: 0,
      }),
    )
  })
  it('renders cache and tier prices including zero in expanded details', async () => {
    const m = model()
    m.price_tiers = [
      {
        key: 'fast',
        label: 'Fast',
        official: m.official,
        actual: { ...m.actual, cache_write_cny_per_m: 0 },
      },
    ]
    m.catalog_price!.tiers = m.price_tiers
    m.catalog_price!.tiers[0].official = {...m.official, cache_write_usd_per_m: 0}
    api.getModelPrices.mockImplementation(async (p) =>
      response({
        selected_group_id: p.group_id || null,
        models: p.group_id ? [m] : [],
      }),
    )
    const w = mountView()
    await enter(w)
    await w.findAll('input[type="checkbox"]')[0].setValue(true)
    expect(w.text()).toContain('Fast')
    expect(w.text()).toContain('缓存写入')
    expect(w.text()).toContain('US$0')
    expect(w.text()).toContain('官方原价')
  })
  it('clears stale platform and source criteria on group change', async () => {
    api.getModelPrices.mockImplementation(async p=>response({selected_group_id:p.group_id||null,models:p.group_id?[model({name:p.group_id===47?'claude':'gpt',platform:p.group_id===47?'anthropic':'openai'})]:[]}))
    const w=mountView();await enter(w)
    await w.get('[data-testid="platform-filter"]').setValue('openai')
    await w.get('[data-testid="quick-group-47"]').trigger('click');await flushPromises()
    expect(w.get('[data-model-name="claude"]').exists()).toBe(true)
    expect((w.get('[data-testid="platform-filter"]').element as HTMLSelectElement).value).toBe('')
  })
  it('keeps management prices and sources separate from official browsing', async () => {
    const custom=model({pricing_source:'custom',custom_price:{input_usd_per_m:99}})
    custom.actual={...custom.actual,input_usd_per_m:99,input_cny_per_m:693}
    api.getModelPrices.mockImplementation(async p=>response({selected_group_id:p.group_id||null,models:p.group_id?[custom]:[]}))
    const w=mountView();await enter(w)
    expect(w.get('.price-main-values').text()).toContain('US$3')
    await w.get('[data-testid="price-manage-tab"]').trigger('click');await flushPromises()
    expect(w.get('.price-main-values').text()).toContain('693')
    await w.get('[data-testid="price-source-filter"]').setValue('custom')
    expect(w.findAll('[data-testid="model-price-row"]')).toHaveLength(1)
    await w.findAll('[role="tab"]').find(tab=>tab.text()==='价格查询')!.trigger('click');await flushPromises()
    expect(w.get('.price-main-values').text()).toContain('US$3')
  })

  it('ignores stale group responses', async () => {
    const w = mountView()
    await flushPromises()
    let resolve!: (v: unknown) => void
    api.getModelPrices
      .mockImplementationOnce(
        () =>
          new Promise((r) => {
            resolve = r
          }),
      )
      .mockResolvedValueOnce(
        response({ selected_group_id: 47, models: [model({ name: 'new' })] }),
      )
    await w.get('[data-testid="quick-group-46"]').trigger('click')
    await w.get('[data-testid="quick-group-47"]').trigger('click')
    await flushPromises()
    resolve(
      response({ selected_group_id: 46, models: [model({ name: 'stale' })] }),
    )
    await flushPromises()
    expect(w.text()).toContain('new')
    expect(w.find('[data-model-name="stale"]').exists()).toBe(false)
  })
  it('never exposes management actions to regular users', async () => {
    auth.isAdmin = false
    const w = mountView()
    await enter(w)
    expect(w.find('[data-testid="price-manage-tab"]').exists()).toBe(false)
    expect(w.find('[aria-label="编辑展示价"]').exists()).toBe(false)
  })
  it('shows retry after load failure and clears stale rows while loading', async () => {
    const w = mountView()
    await enter(w)
    api.getModelPrices.mockRejectedValueOnce(new Error('offline'))
    await w.get('[aria-label="刷新价格"]').trigger('click')
    await flushPromises()
    expect(w.find('[role="alert"]').exists()).toBe(true)
    expect(w.find('[data-testid="model-price-row"]').exists()).toBe(false)
  })
})
