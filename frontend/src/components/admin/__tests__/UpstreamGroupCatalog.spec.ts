import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Catalog from '../UpstreamGroupCatalog.vue'
import type { CatalogGroup } from '@/api/admin/upstreamCatalog'
const { getGroupCatalog, annotateGroups } = vi.hoisted(() => ({
  getGroupCatalog: vi.fn(),
  annotateGroups: vi.fn(),
}))
vi.mock('@/api/admin/upstreamCatalog', () => ({
  getGroupCatalog,
  annotateGroups,
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
})
describe('upstream group catalog', () => {
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
