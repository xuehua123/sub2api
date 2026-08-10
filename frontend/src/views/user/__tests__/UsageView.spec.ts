import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'

import UsageView from '../UsageView.vue'

const { query, getStatsByDateRange, list, showError, showWarning, showSuccess, showInfo } = vi.hoisted(() => ({
  query: vi.fn(),
  getStatsByDateRange: vi.fn(),
  list: vi.fn(),
  showError: vi.fn(),
  showWarning: vi.fn(),
  showSuccess: vi.fn(),
  showInfo: vi.fn(),
}))

const messages: Record<string, string> = {
  'usage.costDetails': 'Cost Breakdown',
  'usage.actualCost': 'Actual Cost',
  'usage.inSelectedRange': 'In selected range',
  'admin.usage.inputCost': 'Input Cost',
  'admin.usage.outputCost': 'Output Cost',
  'admin.usage.cacheCreationCost': 'Cache Creation Cost',
  'admin.usage.cacheReadCost': 'Cache Read Cost',
  'usage.inputTokenPrice': 'Input price',
  'usage.outputTokenPrice': 'Output price',
  'usage.perMillionTokens': '/ 1M tokens',
  'usage.serviceTier': 'Service tier',
  'usage.serviceTierPriority': 'Fast',
  'usage.serviceTierFlex': 'Flex',
  'usage.serviceTierStandard': 'Standard',
  'usage.rate': 'Rate',
  'usage.original': 'Original',
  'usage.billed': 'Billed',
  'usage.userBilled': 'User billed',
  'usage.allApiKeys': 'All API Keys',
  'usage.apiKeyFilter': 'API Key',
  'usage.model': 'Model',
  'usage.reasoningEffort': 'Reasoning Effort',
  'usage.type': 'Type',
  'usage.tokens': 'Tokens',
  'usage.cost': 'Cost',
  'usage.firstResponse': 'First Response',
  'usage.upstreamFirstEvent': 'First Event',
  'usage.firstToken': 'First Token',
  'usage.duration': 'Duration',
  'usage.time': 'Time',
  'usage.userAgent': 'User Agent',
  'usage.ipAddress': 'IP',
  'usage.imageUnit': ' images',
  'usage.imageCount': 'Image count',
  'usage.imageBillingSize': 'Billing size',
  'usage.imageInputSize': 'Input size',
  'usage.imageOutputSize': 'Output size',
  'usage.imageSizeSource': 'Size source',
  'usage.imageSizeBreakdown': 'Size breakdown',
  'usage.imageSizeSourceOutput': 'Upstream output',
  'usage.imageSizeSourceInput': 'Request input',
  'usage.imageSizeSourceDefault': 'Default billing tier',
  'usage.imageSizeSourceLegacy': 'Legacy record',
  'usage.imageSizeSourceMissing': 'Not recorded',
  'usage.imageSizeNotRecorded': 'not recorded',
  'usage.imageSizeLegacyUnstandardized': 'legacy unstandardized',
  'usage.imageSizeUnknown': 'unknown',
  'usage.imageUnitPrice': 'Per-image price',
  'usage.imageTotalPrice': 'Image total price',
  'admin.usage.billingModeToken': 'Token',
  'admin.usage.billingModePerRequest': 'Per request',
  'admin.usage.billingModeImage': 'Image',
  'admin.usage.billingModeVideo': 'Video',
}

vi.mock('@/api', () => ({
  usageAPI: {
    query,
    getStatsByDateRange,
  },
  keysAPI: {
    list,
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showWarning, showSuccess, showInfo }),
}))

vi.mock('@/composables/useCurrencyResolver', () => ({
  useCurrencyResolver: () => ({
    convertUsdToCnyForLog: (val: number) => (val != null ? val * 7 : null),
    convertUsdToCny: (val: number) => (val != null ? val * 7 : null),
    formatCny: (val: number | null | undefined) => (val != null ? `¥${val.toFixed(2)}` : '-'),
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

const AppLayoutStub = { template: '<div><slot /></div>' }
const TablePageLayoutStub = {
  template: '<div><slot name="actions" /><slot name="filters" /><slot name="table" /><slot /></div>',
}
const DataTableStub = {
  props: ['data'],
  template: `
    <div>
      <div v-for="row in data" :key="row.request_id">
        <slot name="cell-billing_mode" :row="row" />
        <slot name="cell-tokens" :row="row" />
        <slot name="cell-cost" :row="row" />
      </div>
    </div>
  `,
}

describe('user UsageView tooltip', () => {
  beforeEach(() => {
    query.mockReset()
    getStatsByDateRange.mockReset()
    list.mockReset()
    showError.mockReset()
    showWarning.mockReset()
    showSuccess.mockReset()
    showInfo.mockReset()

    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      x: 0,
      y: 0,
      top: 20,
      left: 20,
      right: 120,
      bottom: 40,
      width: 100,
      height: 20,
      toJSON: () => ({}),
    } as DOMRect)

    ;(globalThis as any).ResizeObserver = class {
      observe() {}
      disconnect() {}
    }
  })

  it('shows cost breakdown without original cost in user cost tooltip', async () => {
    query.mockResolvedValue({
      items: [
        {
          request_id: 'req-user-1',
          actual_cost: 0.092883,
          total_cost: 0.092883,
          rate_multiplier: 1,
          service_tier: 'priority',
          input_cost: 0.020285,
          output_cost: 0.00303,
          cache_creation_cost: 0,
          cache_read_cost: 0.069568,
          input_tokens: 4057,
          output_tokens: 101,
          cache_creation_tokens: 0,
          cache_read_tokens: 278272,
          cache_creation_5m_tokens: 0,
          cache_creation_1h_tokens: 0,
          image_count: 0,
          image_size: null,
          first_token_ms: null,
          first_sse_event_ms: null,
          first_client_flush_ms: null,
          duration_ms: 1,
          created_at: '2026-03-08T00:00:00Z',
        },
      ],
      total: 1,
      pages: 1,
    })
    getStatsByDateRange.mockResolvedValue({
      total_requests: 1,
      total_tokens: 100,
      total_cost: 0.1,
      avg_duration_ms: 1,
    })
    list.mockResolvedValue({ items: [] })

    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          TablePageLayout: TablePageLayoutStub,
          Pagination: true,
          EmptyState: true,
          Select: true,
          DateRangePicker: true,
          DataTable: DataTableStub,
          Icon: true,
          Teleport: true,
        },
      },
    })

    await flushPromises()
    await nextTick()

    const setupState = (wrapper.vm as any).$?.setupState
    setupState.tooltipData = {
      request_id: 'req-user-1',
      actual_cost: 0.092883,
      total_cost: 0.092883,
      rate_multiplier: 1,
      service_tier: 'priority',
      input_cost: 0.020285,
      output_cost: 0.00303,
      cache_creation_cost: 0,
      cache_read_cost: 0.069568,
      input_tokens: 4057,
      output_tokens: 101,
    }
    setupState.tooltipVisible = true
    await nextTick()

    const text = wrapper.text()
    expect(text).toContain('Cost Breakdown')
    expect(text).toContain('Input Cost')
    expect(text).toContain('Output Cost')
    expect(text).toContain('Input price')
    expect(text).toContain('Output price')
    expect(text).toContain('/ 1M tokens')
    expect(text).toContain('Cache Read Cost')
    expect(text).toContain('Service tier')
    expect(text).toContain('Fast')
    expect(text).toContain('Rate')
    expect(text).toContain('1.00x')
    expect(text).toContain('User billed')
    expect(text).toContain('$0.092883')
    expect(text).not.toContain('Original')
  })

  it('exports csv with only billed cost', async () => {
    const exportedLogs = [
      {
        request_id: 'req-user-export',
        actual_cost: 0.092883,
        total_cost: 0.092883,
        rate_multiplier: 1,
        service_tier: 'priority',
        input_cost: 0.020285,
        output_cost: 0.00303,
        cache_creation_cost: 0.000001,
        cache_read_cost: 0.069568,
        input_tokens: 4057,
        output_tokens: 101,
        cache_creation_tokens: 4,
        cache_read_tokens: 278272,
        cache_creation_5m_tokens: 0,
        cache_creation_1h_tokens: 0,
        image_count: 0,
        image_size: null,
        first_token_ms: 12,
        first_sse_event_ms: 10,
        first_client_flush_ms: 11,
        duration_ms: 345,
        ip_address: '203.0.113.10',
        created_at: '2026-03-08T00:00:00Z',
        model: 'gpt-5.4',
        reasoning_effort: null,
        api_key: { name: 'demo-key' },
      },
    ]

    query.mockResolvedValue({
      items: exportedLogs,
      total: 1,
      pages: 1,
    })
    getStatsByDateRange.mockResolvedValue({
      total_requests: 1,
      total_tokens: 100,
      total_cost: 0.1,
      avg_duration_ms: 1,
    })
    list.mockResolvedValue({ items: [] })

    let exportedBlob: Blob | null = null
    const originalCreateObjectURL = window.URL.createObjectURL
    const originalRevokeObjectURL = window.URL.revokeObjectURL
    window.URL.createObjectURL = vi.fn((blob: Blob | MediaSource) => {
      exportedBlob = blob as Blob
      return 'blob:usage-export'
    }) as typeof window.URL.createObjectURL
    window.URL.revokeObjectURL = vi.fn(() => {}) as typeof window.URL.revokeObjectURL
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})

    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          TablePageLayout: TablePageLayoutStub,
          Pagination: true,
          EmptyState: true,
          Select: true,
          DateRangePicker: true,
          DataTable: DataTableStub,
          Icon: true,
          Teleport: true,
        },
      },
    })

    await flushPromises()

    const setupState = (wrapper.vm as any).$?.setupState
    await setupState.exportToCSV()

    expect(exportedBlob).not.toBeNull()
    const hasSortedExportQuery = query.mock.calls.some((call) => {
      const params = call[0] as Record<string, unknown> | undefined
      const config = call[1]
      return (
        params?.page_size === 100 &&
        params?.sort_by === 'created_at' &&
        params?.sort_order === 'desc' &&
        config === undefined
      )
    })
    expect(hasSortedExportQuery).toBe(true)
    expect(clickSpy).toHaveBeenCalled()
    expect(showSuccess).toHaveBeenCalled()
    expect(setupState.columns.some((column: { key: string }) => column.key === 'ip_address')).toBe(true)
    const csv = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => resolve(String(reader.result))
      reader.onerror = () => reject(reader.error)
      reader.readAsText(exportedBlob as Blob)
    })
    const csvBuffer = await new Promise<ArrayBuffer>((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => resolve(reader.result as ArrayBuffer)
      reader.onerror = () => reject(reader.error)
      reader.readAsArrayBuffer(exportedBlob as Blob)
    })
    const csvBytes = new Uint8Array(csvBuffer)
    expect(Array.from(csvBytes.slice(0, 3))).toEqual([0xef, 0xbb, 0xbf])
    const csvBody = csv.replace(/^\uFEFF/, '')
    expect(csvBody).toContain('Cost')
    expect(csvBody).toContain('0.09288300')
    expect(csvBody).not.toContain('Rate Multiplier')
    expect(csvBody).not.toContain('Original Cost')

    window.URL.createObjectURL = originalCreateObjectURL
    window.URL.revokeObjectURL = originalRevokeObjectURL
    clickSpy.mockRestore()
  })

  it('exports historical image rows with image billing mode derived from image_count', async () => {
    const exportedLogs = [
      {
        request_id: 'req-user-export-legacy-image',
        actual_cost: 0.2,
        total_cost: 0.2,
        rate_multiplier: 1,
        service_tier: null,
        input_cost: 0,
        output_cost: 0,
        cache_creation_cost: 0,
        cache_read_cost: 0,
        input_tokens: 0,
        output_tokens: 0,
        cache_creation_tokens: 0,
        cache_read_tokens: 0,
        cache_creation_5m_tokens: 0,
        cache_creation_1h_tokens: 0,
        image_count: 1,
        image_size: null,
        billing_mode: null,
        first_token_ms: null,
        duration_ms: 345,
        created_at: '2026-03-08T00:00:00Z',
        model: 'gpt-image-2',
        reasoning_effort: null,
        api_key: { name: 'demo-key' },
      },
    ]

    query.mockResolvedValue({
      items: exportedLogs,
      total: 1,
      pages: 1,
    })
    getStatsByDateRange.mockResolvedValue({
      total_requests: 1,
      total_tokens: 0,
      total_cost: 0.2,
      avg_duration_ms: 1,
    })
    list.mockResolvedValue({ items: [] })

    let exportedBlob: Blob | null = null
    const originalCreateObjectURL = window.URL.createObjectURL
    const originalRevokeObjectURL = window.URL.revokeObjectURL
    window.URL.createObjectURL = vi.fn((blob: Blob | MediaSource) => {
      exportedBlob = blob as Blob
      return 'blob:usage-export'
    }) as typeof window.URL.createObjectURL
    window.URL.revokeObjectURL = vi.fn(() => {}) as typeof window.URL.revokeObjectURL
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})

    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          TablePageLayout: TablePageLayoutStub,
          Pagination: true,
          EmptyState: true,
          Select: true,
          DateRangePicker: true,
          DataTable: DataTableStub,
          Icon: true,
          Teleport: true,
        },
      },
    })

    await flushPromises()

    const setupState = (wrapper.vm as any).$?.setupState
    await setupState.exportToCSV()

    expect(exportedBlob).not.toBeNull()
    const csv = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => resolve(String(reader.result))
      reader.onerror = () => reject(reader.error)
      reader.readAsText(exportedBlob as Blob)
    })
    expect(csv).toContain('Billing Mode')
    expect(csv).toContain('Image')
    expect(csv).not.toContain(',Token,0,0,0,0,')

    window.URL.createObjectURL = originalCreateObjectURL
    window.URL.revokeObjectURL = originalRevokeObjectURL
    clickSpy.mockRestore()
  })

  it('does not display a 2K fallback for historical image rows with missing size', async () => {
    query.mockResolvedValue({
      items: [
        {
          request_id: 'req-user-legacy-missing-image',
          actual_cost: 0.2,
          total_cost: 0.2,
          rate_multiplier: 1,
          service_tier: null,
          input_cost: 0,
          output_cost: 0,
          cache_creation_cost: 0,
          cache_read_cost: 0,
          input_tokens: 0,
          output_tokens: 0,
          cache_creation_tokens: 0,
          cache_read_tokens: 0,
          cache_creation_5m_tokens: 0,
          cache_creation_1h_tokens: 0,
          image_count: 1,
          image_size: null,
          image_input_size: null,
          image_output_size: null,
          image_size_source: null,
          image_size_breakdown: null,
          billing_mode: null,
          first_token_ms: null,
          duration_ms: 1,
          created_at: '2026-03-08T00:00:00Z',
          model: 'gpt-image-2',
        },
      ],
      total: 1,
      pages: 1,
    })
    getStatsByDateRange.mockResolvedValue({
      total_requests: 1,
      total_tokens: 0,
      total_cost: 0.2,
      avg_duration_ms: 1,
    })
    list.mockResolvedValue({ items: [] })

    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          TablePageLayout: TablePageLayoutStub,
          Pagination: true,
          EmptyState: true,
          Select: true,
          DateRangePicker: true,
          DataTable: DataTableStub,
          Icon: true,
          Teleport: true,
        },
      },
    })

    await flushPromises()
    await nextTick()

    const text = wrapper.text()
    expect(text).toContain('Image')
    expect(text).toContain('not recorded')
    expect(text).not.toContain('(2K)')
  })

  it('does not treat explicit token or video rows as image billing when image_count is positive', async () => {
    const tokenRow = {
      request_id: 'req-user-token-with-image-output',
      actual_cost: 0.1,
      total_cost: 0.1,
      rate_multiplier: 1,
      service_tier: null,
      input_cost: 0.06,
      output_cost: 0.04,
      cache_creation_cost: 0,
      cache_read_cost: 0,
      input_tokens: 120,
      output_tokens: 30,
      cache_creation_tokens: 0,
      cache_read_tokens: 0,
      billing_mode: 'token',
      image_count: 7,
      image_size: '2K',
      first_token_ms: null,
      duration_ms: 1,
      created_at: '2026-03-08T00:00:00Z',
      model: 'gpt-image-2',
    }
    const videoRow = {
      ...tokenRow,
      request_id: 'req-user-video-with-preview-count',
      billing_mode: 'video',
      image_count: 8,
      input_tokens: 240,
      output_tokens: 60,
      model: 'grok-imagine-video',
    }

    query.mockResolvedValue({
      items: [tokenRow, videoRow],
      total: 2,
      pages: 1,
    })
    getStatsByDateRange.mockResolvedValue({
      total_requests: 2,
      total_tokens: 450,
      total_cost: 0.2,
      avg_duration_ms: 1,
    })
    list.mockResolvedValue({ items: [] })

    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          TablePageLayout: TablePageLayoutStub,
          Pagination: true,
          EmptyState: true,
          Select: true,
          DateRangePicker: true,
          DataTable: DataTableStub,
          Icon: true,
          Teleport: true,
        },
      },
    })

    await flushPromises()
    await nextTick()

    expect(wrapper.text()).toContain('Token')
    expect(wrapper.text()).toContain('Video')
    expect(wrapper.text()).not.toContain('7 images')
    expect(wrapper.text()).not.toContain('8 images')

    const setupState = (wrapper.vm as any).$?.setupState
    setupState.tooltipData = tokenRow
    setupState.tooltipVisible = true
    await nextTick()

    expect(wrapper.text()).toContain('Input price')
    expect(wrapper.text()).not.toContain('Image count')
  })

  it('shows image billing metadata in the user cost tooltip', async () => {
    query.mockResolvedValue({
      items: [],
      total: 0,
      pages: 0,
    })
    getStatsByDateRange.mockResolvedValue({
      total_requests: 0,
      total_tokens: 0,
      total_cost: 0,
      avg_duration_ms: 0,
    })
    list.mockResolvedValue({ items: [] })

    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          TablePageLayout: TablePageLayoutStub,
          Pagination: true,
          EmptyState: true,
          Select: true,
          DateRangePicker: true,
          DataTable: DataTableStub,
          Icon: true,
          Teleport: true,
        },
      },
    })

    await flushPromises()

    const setupState = (wrapper.vm as any).$?.setupState
    setupState.tooltipData = {
      request_id: 'req-user-output-image',
      actual_cost: 0.8,
      total_cost: 0.8,
      rate_multiplier: 1,
      service_tier: null,
      input_cost: 0,
      output_cost: 0,
      cache_creation_cost: 0,
      cache_read_cost: 0,
      input_tokens: 0,
      output_tokens: 0,
      cache_creation_tokens: 0,
      cache_read_tokens: 0,
      billing_mode: null,
      image_count: 2,
      image_size: '4K',
      image_input_size: '1024x1024',
      image_output_size: '3840x2160',
      image_size_source: 'output',
      image_size_breakdown: { '4K': 2 },
    }
    setupState.tooltipVisible = true
    await nextTick()

    const text = wrapper.text()
    expect(text).toContain('Image count')
    expect(text).toContain('Billing size')
    expect(text).toContain('4K')
    expect(text).toContain('Size source')
    expect(text).toContain('Upstream output')
    expect(text).toContain('Input size')
    expect(text).toContain('1024x1024')
    expect(text).toContain('Output size')
    expect(text).toContain('3840x2160')
    expect(text).toContain('4K x 2')
  })
})
