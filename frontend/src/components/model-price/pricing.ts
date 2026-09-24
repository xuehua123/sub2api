import type {
  ModelPriceActual,
  ModelPriceModel,
  ModelPriceValue,
} from '@/api/modelPrices'

export type PriceKind = 'token' | 'per_request' | 'image' | 'video' | 'unknown'
export type PriceSort =
  | 'name'
  | 'input'
  | 'output'
  | 'fixed'
  | 'discount'
  | 'issues'
export const priceFields = [
  {
    key: 'input',
    label: '输入',
    usd: 'input_usd_per_m',
    cny: 'input_cny_per_m',
  },
  {
    key: 'output',
    label: '输出',
    usd: 'output_usd_per_m',
    cny: 'output_cny_per_m',
  },
  {
    key: 'cache_read',
    label: '缓存读取',
    usd: 'cache_read_usd_per_m',
    cny: 'cache_read_cny_per_m',
  },
  {
    key: 'cache_write',
    label: '缓存写入',
    usd: 'cache_write_usd_per_m',
    cny: 'cache_write_cny_per_m',
  },
  {
    key: 'cache_write_5m',
    label: '缓存写入 · 5 分钟',
    usd: 'cache_write_5m_usd_per_m',
    cny: 'cache_write_5m_cny_per_m',
  },
  {
    key: 'cache_write_1h',
    label: '缓存写入 · 1 小时',
    usd: 'cache_write_1h_usd_per_m',
    cny: 'cache_write_1h_cny_per_m',
  },
  {
    key: 'image_input',
    label: '图片输入',
    usd: 'image_input_usd_per_m',
    cny: 'image_input_cny_per_m',
  },
  {
    key: 'image_output',
    label: '图片输出',
    usd: 'image_output_usd_per_m',
    cny: 'image_output_cny_per_m',
  },
  {
    key: 'quantity',
    label: '固定单价',
    usd: 'per_request_usd',
    cny: 'per_request_cny',
  },
] as const satisfies ReadonlyArray<{
  key: string
  label: string
  usd: keyof ModelPriceValue
  cny: keyof ModelPriceActual
}>
export type UsageField = (typeof priceFields)[number]['key']
export type Usage = Record<UsageField | 'requests', number>
export function validPrice(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
}
export function money(
  value: number | null | undefined,
  currency = 'CNY',
): string {
  if (!validPrice(value)) return '待定价'
  return new Intl.NumberFormat('zh-CN', {
    style: 'currency',
    currency,
    maximumFractionDigits: 8,
    minimumFractionDigits: 0,
  }).format(value)
}
export function modelKey(m: ModelPriceModel) {
  return m.platform + ':' + m.name
}
export function kind(m: ModelPriceModel): PriceKind {
  if (['token', 'image', 'video'].includes(m.billing_mode))
    return m.billing_mode as PriceKind
  if (['request', 'per_request'].includes(m.billing_mode)) return 'per_request'
  return 'unknown'
}
export function unit(m: ModelPriceModel) {
  return {
    token: '百万 tokens',
    image: '张',
    video: '秒',
    per_request: '次',
    unknown: '单位未确认',
  }[kind(m)]
}
export function modeLabel(m: ModelPriceModel) {
  return {
    token: 'Token 计费',
    image: '图片按张计费',
    video: '视频按秒计费',
    per_request: '按次计费',
    unknown: '计费方式未确认',
  }[kind(m)]
}
export function sourceLabel(source: string) {
  return (
    (
      {
        official: '官方目录',
        channel: '渠道配置',
        group: '分组配置',
        fallback: '内置回退价',
        custom: '自定义展示价',
        display_override: '自定义展示价',
        unknown: '来源待确认',
      } as Record<string, string>
    )[source] || '来源待确认'
  )
}
export function isDisplayOverride(m: ModelPriceModel) {
  return (
    ['custom', 'display_override'].includes(m.pricing_source) ||
    !!m.custom_price
  )
}
export function issues(m: ModelPriceModel): string[] {
  const result: string[] = []
  if (kind(m) === 'unknown') result.push('unit')
  if (m.pricing_source === 'unknown') result.push('source')
  const required =
    kind(m) === 'token'
      ? [m.actual.input_cny_per_m ?? m.actual.image_input_cny_per_m, m.actual.output_cny_per_m ?? m.actual.image_output_cny_per_m]
      : [m.actual.per_request_cny]
  if (required.some((v) => !validPrice(v))) result.push('missing')
  if (m.official_missing) result.push('baseline')
  if (!m.channel_names.length) result.push('route')
  // A display-only zero is valid; never report it as a loss without cost data.
  if (required.some((v) => v === 0)) result.push('zero')
  return result
}
export const issueLabels: Record<string, string> = {
  unit: '计费单位待确认',
  source: '价格来源待确认',
  missing: '价格信息不完整',
  baseline: '官方原价缺失',
  route: '未提供渠道来源',
  zero: '含零价项目',
}
export function tariff(m: ModelPriceModel, key: string) {
  if (key === 'base')
    return { actual: m.actual, official: m.official, label: '基础价' }
  return m.price_tiers.find((t) => t.key === key)
}
export function tariffFields(m: ModelPriceModel) {
  return kind(m) === 'token'
    ? priceFields.filter((f) => f.key !== 'quantity')
    : priceFields.filter((f) => f.key === 'quantity')
}
export function estimate(
  m: ModelPriceModel,
  tierKey: string,
  usage: Usage,
): {
  total: number | null
  reason?: string
  lines: { label: string; amount: number }[]
} {
  const selected = tariff(m, tierKey)
  if (!selected || kind(m) === 'unknown')
    return { total: null, reason: '请选择已确认单位的计价档位', lines: [] }
  const values = Object.values(usage)
  if (
    values.some((v) => !Number.isFinite(v) || v < 0) ||
    !Number.isInteger(usage.requests)
  )
    return {
      total: null,
      reason: '用量必须是非负有限数，请求次数必须是整数',
      lines: [],
    }
  const context =
    usage.input +
    usage.cache_read +
    usage.cache_write +
    usage.cache_write_5m +
    usage.cache_write_1h +
    usage.image_input
  if (kind(m) === 'token') {
    if (tariffFields(m).some((f) => !Number.isInteger(usage[f.key])))
      return { total: null, reason: 'Token 数必须是整数', lines: [] }
    const crossed = m.price_tiers.filter(
      (t) => t.threshold_tokens != null && context > t.threshold_tokens,
    )
    if (tierKey === 'base' && crossed.length)
      return {
        total: null,
        reason: '输入超过阶梯阈值，请选择对应长上下文档位',
        lines: [],
      }
    const selectedTier = m.price_tiers.find((t) => t.key === tierKey)
    if (
      crossed.length &&
      (!selectedTier?.threshold_tokens ||
        crossed.some(
          (t) => (t.threshold_tokens || 0) > selectedTier.threshold_tokens!,
        ))
    ) {
      return {
        total: null,
        reason: '请选择适用于当前输入长度的阶梯档位',
        lines: [],
      }
    }
    if (
      selectedTier?.threshold_tokens != null &&
      context <= selectedTier.threshold_tokens
    )
      return { total: null, reason: '输入尚未达到所选档位的阈值', lines: [] }
  }
  if (
    ['image', 'per_request'].includes(kind(m)) &&
    !Number.isInteger(usage.quantity)
  ) {
    return { total: null, reason: '张数和调用数量必须是整数', lines: [] }
  }
  const lines: { label: string; amount: number }[] = []
  for (const f of tariffFields(m)) {
    const amount = usage[f.key]
    if (amount === 0) continue
    const rate = selected.actual[f.cny]
    if (!validPrice(rate))
      return { total: null, reason: f.label + '价格缺失，不能估算', lines: [] }
    lines.push({
      label: f.label,
      amount: (amount * rate) / (kind(m) === 'token' ? 1e6 : 1),
    })
  }
  const total = lines.reduce((s, x) => s + x.amount, 0) * usage.requests
  return Number.isFinite(total)
    ? {
        total,
        lines: lines.map((x) => ({ ...x, amount: x.amount * usage.requests })),
      }
    : { total: null, reason: '数值过大，无法估算', lines: [] }
}
export function comparePrices(
  a: ModelPriceModel,
  b: ModelPriceModel,
  sort: PriceSort,
) {
  const tie = () =>
    modelKey(a).localeCompare(modelKey(b), 'zh-CN', { numeric: true })
  if (sort === 'name') return tie()
  if (sort === 'issues') return issues(b).length - issues(a).length || tie()
  // Never rank a per-second price against a per-image price as if units matched.
  if (sort === 'fixed' && kind(a) !== kind(b))
    return kind(a).localeCompare(kind(b))
  const value = (m: ModelPriceModel) =>
    sort === 'discount'
      ? m.cheaper_factor
      : sort === 'fixed'
        ? kind(m) !== 'token'
          ? m.actual.per_request_cny
          : null
        : kind(m) === 'token'
          ? sort === 'input'
            ? m.actual.input_cny_per_m
            : m.actual.output_cny_per_m
          : null
  const x = value(a),
    y = value(b)
  if (!validPrice(x)) return !validPrice(y) ? tie() : 1
  if (!validPrice(y)) return -1
  return (sort === 'discount' ? y - x : x - y) || tie()
}
