import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Catalog from '../UpstreamGroupCatalog.vue'
import type { CatalogGroup } from '@/api/admin/upstreamCatalog'
const { getGroupCatalog, annotateGroups, syncGroupModels } = vi.hoisted(() => ({
  getGroupCatalog: vi.fn(),
  annotateGroups: vi.fn(),
  syncGroupModels: vi.fn(),
}))
vi.mock('@/api/admin/upstreamCatalog', () => ({
  getGroupCatalog,
  annotateGroups,
  syncGroupModels,
}))
vi.mock('@/utils/format', () => ({ formatDateTime: (value: string) => value }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const row: CatalogGroup = {
  connection_id: 1,
  connection_name: 'Upstream',
  management_base_url: 'https://example.invalid',
  provider: 'sub2api',
  remote_key: 'id:g',
  remote_id: 'g',
  name: 'Zero group',
  rate_multiplier: 0,
  source: 'groups',
  confidence: 'default',
  observed_at: null,
  fresh_until: null,
  tags: [],
  favorite: false,
  account_ids: [],
  binding_count: 0,
  freshness: 'unknown',
  auto_tags: [], excluded_auto_tags: [], model_count: 0, model_preview: [], model_status: 'unknown', model_coverage: 'unknown', models_observed_at: null,
}
function view() {
  return mount(Catalog, {
    props: { connections: [{ id: 1, name: 'Upstream', provider: 'sub2api' }] },
    global: {
      stubs: {
        BaseDialog: {
          props: ['show'],
          template: '<div v-if="show"><slot/><slot name="footer"/></div>',
        },
        Pagination: true,
        UpstreamGroupModelsDialog: true,
      },
    },
  })
}
beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  getGroupCatalog.mockResolvedValue({
    items: [row],
    total: 1,
    page: 1,
    page_size: 20,
    tags: [],
  })
  annotateGroups.mockResolvedValue(undefined)
  syncGroupModels.mockResolvedValue({ status: 'ready' })
})
describe('upstream group catalog', () => {
  it('links to the upstream website without losing the connection detail action', async () => {
    const w = view()
    await flushPromises()
    const link = w.get('a[aria-label="upstreamWorkspace.website Upstream"]')
    expect(link.attributes('href')).toBe('https://example.invalid/')
    expect(link.attributes('target')).toBe('_blank')
    expect(link.attributes('rel')).toBe('noopener noreferrer')
    expect(w.get('[data-testid="catalog-row"]').text()).toContain('upstreamWorkspace.modelsNotFetched')
    expect(syncGroupModels).not.toHaveBeenCalled()
    w.unmount()
  })
  it('removes an automatic tag through the override API', async () => {
    getGroupCatalog.mockResolvedValue({ items: [{ ...row, tags: ['GPT'], auto_tags: ['GPT'] }], total: 1, tags: ['GPT'] })
    const w = view()
    await flushPromises()
    await w.get('[aria-label="upstreamWorkspace.removeTags GPT Zero group"]').trigger('click')
    await flushPromises()
    expect(annotateGroups).toHaveBeenCalledWith([{ connection_id: 1, remote_key: 'id:g' }], { remove_tags: ['GPT'] })
    w.unmount()
  })
  it('only refreshes the explicitly selected groups', async () => {
    const w = view()
    await flushPromises()
    await w.get('[aria-label="upstreamWorkspace.selectRow Zero group"]').setValue(true)
    const refresh = w.findAll('button').find(button => button.text() === 'upstreamWorkspace.refreshModels')!
    await refresh.trigger('click')
    await flushPromises()
    expect(syncGroupModels).toHaveBeenCalledTimes(1)
    expect(syncGroupModels.mock.calls[0][0].remote_key).toBe('id:g')
    expect(w.text()).toContain('upstreamWorkspace.modelsProgress')
    w.unmount()
  })
  it('shows unbound zero-rate groups and sends favorites by remote identity', async () => {
    const w = view()
    await flushPromises()
    expect(w.get('[data-testid="catalog-row"]').text()).toContain('0×')
    await w
      .get('[aria-label="upstreamWorkspace.favorite Zero group"]')
      .trigger('click')
    await flushPromises()
    expect(annotateGroups).toHaveBeenCalledWith(
      [{ connection_id: 1, remote_key: 'id:g' }],
      { favorite: true },
    )
    w.unmount()
  })
  it('adds custom tags without modifying billing values', async () => {
    const w = view()
    await flushPromises()
    await w
      .get('[aria-label="upstreamWorkspace.addTags Zero group"]')
      .trigger('click')
    await w
      .get('input[aria-label="upstreamWorkspace.tagInput"]')
      .setValue('主力, Claude')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(annotateGroups).toHaveBeenCalledWith(
      [{ connection_id: 1, remote_key: 'id:g' }],
      { add_tags: ['主力', 'Claude'] },
    )
    w.unmount()
  })
  it('does not let a stale response replace a newer search', async () => {
    vi.useFakeTimers()
    let resolveOld: (x: unknown) => void = () => {}
    getGroupCatalog.mockReturnValueOnce(
      new Promise((r) => {
        resolveOld = r
      }),
    )
    const w = view()
    await w.get('input[placeholder]').setValue('new')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    resolveOld({ items: [{ ...row, name: 'Old result' }], total: 1, tags: [] })
    await flushPromises()
    expect(w.text()).not.toContain('Old result')
    expect(getGroupCatalog.mock.calls[1][0].search).toBe('new')
    w.unmount()
    vi.useRealTimers()
  })
})
