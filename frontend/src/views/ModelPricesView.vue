<template>
  <AppLayout>
    <div class="model-market -m-4 min-h-[calc(100vh-4rem)] bg-[#0f1014] text-slate-100 md:-m-6 lg:-m-8">
      <div class="flex min-h-[calc(100vh-4rem)]">
        <aside class="hidden w-[360px] shrink-0 border-r border-white/[0.08] bg-[#101114] px-4 py-5 xl:block">
          <div class="mb-5 flex items-center justify-between">
            <div>
              <p class="text-lg font-semibold text-white">筛选</p>
              <p class="mt-1 text-xs text-slate-500">按供应商和分组快速查看</p>
            </div>
            <button class="rounded-md border border-white/10 px-2.5 py-1 text-xs text-slate-300 hover:bg-white/[0.06]" @click="resetFilters">
              重置
            </button>
          </div>

          <FilterSection v-if="isAdmin" title="供应商">
            <FilterPill
              v-for="item in providerFilters"
              :key="item.value"
              :active="selectedProvider === item.value"
              :label="item.label"
              :count="item.count"
              :tone="item.tone"
              @click="selectedProvider = item.value"
            />
          </FilterSection>

          <FilterSection title="可用令牌分组">
            <FilterPill
              v-for="group in groupFilters"
              :key="group.id"
              :active="selectedGroupID === group.id"
              :label="group.name"
              :count-label="group.best_plan ? saveFactorLabel(groupSaleMultiplier(group)) : '参考'"
              tone="cyan"
              @click="selectedGroupID = group.id"
            />
          </FilterSection>

          <FilterSection title="计费类型">
            <FilterPill
              v-for="item in billingFilters"
              :key="item.value"
              :active="selectedBilling === item.value"
              :label="item.label"
              :count="item.count"
              tone="amber"
              @click="selectedBilling = item.value"
            />
          </FilterSection>

          <FilterSection v-if="isAdmin" title="端点类型">
            <FilterPill
              v-for="item in platformFilters"
              :key="item.value"
              :active="selectedPlatform === item.value"
              :label="item.label"
              :count="item.count"
              tone="emerald"
              @click="selectedPlatform = item.value"
            />
          </FilterSection>
        </aside>

        <main class="min-w-0 flex-1 px-4 py-5 sm:px-5 lg:px-6">
          <section class="overflow-hidden rounded-lg border border-white/10 bg-[#14161b] shadow-2xl shadow-black/30">
            <div class="relative min-h-[98px] overflow-hidden bg-[radial-gradient(circle_at_15%_20%,rgba(34,211,238,.34),transparent_31%),linear-gradient(105deg,#1e40af,#2563eb_42%,#4338ca)] px-5 py-4">
              <div class="absolute inset-y-0 right-0 w-2/3 bg-[linear-gradient(120deg,transparent,rgba(255,255,255,.16))]" />
              <div class="relative flex h-full items-center justify-between gap-5">
                <div>
                  <div class="flex items-center gap-2">
                    <h1 class="text-xl font-bold text-white">模型价格</h1>
                    <span class="rounded-full bg-white px-2 py-0.5 text-xs font-semibold text-blue-700">
                      共 {{ filteredModels.length }} 个模型
                    </span>
                  </div>
                  <p class="mt-2 max-w-3xl text-sm text-blue-50/90">
                    按当前分组展示我们的到手价格，并对照官方参考价，让优惠力度一眼能看懂。
                  </p>
                </div>
                <div class="hidden shrink-0 grid-cols-3 gap-3 xl:grid">
                  <div v-if="isAdmin" class="hero-stat">
                    <span>官方汇率</span>
                    <strong>{{ formatNumber(response?.usd_cny_rate ?? 7, 4) }}</strong>
                  </div>
                  <div class="hero-stat">
                    <span>{{ isAdmin ? '套餐折算' : '我们的价格' }}</span>
                    <strong>{{ selectedGroup?.best_plan ? `¥${formatNumber(selectedGroup.best_plan.cny_per_quota_usd, 4)}` : '参考价' }}</strong>
                  </div>
                  <div class="hero-stat">
                    <span>优惠力度</span>
                    <strong>{{ selectedGroup?.best_plan ? saveFactorLabel(groupSaleMultiplier(selectedGroup)) : '-' }}</strong>
                  </div>
                </div>
              </div>
            </div>

            <div class="border-b border-white/10 bg-[#15171c] p-3">
              <div class="flex flex-col gap-3 xl:flex-row xl:items-center">
                <div class="relative min-w-0 flex-1">
                  <Icon name="search" size="sm" class="absolute left-3 top-1/2 -translate-y-1/2 text-slate-500" />
                  <input
                    v-model="searchQuery"
                    class="h-10 w-full rounded-md border border-white/10 bg-[#2a2c31] pl-9 pr-3 text-sm text-slate-100 outline-none placeholder:text-slate-500 focus:border-blue-400"
                    placeholder="搜索模型名称"
                  />
                </div>
                <div class="flex flex-wrap items-center gap-2 text-xs">
                  <button v-if="isAdmin" class="toolbar-button" :class="{ active: showOfficial }" @click="showOfficial = !showOfficial">
                    参考价显示
                  </button>
                  <button class="toolbar-button" :class="{ active: showMultiplier }" @click="showMultiplier = !showMultiplier">
                    优惠倍数
                  </button>
                  <button class="toolbar-button" @click="copyVisibleModels">
                    <Icon name="copy" size="xs" />
                    复制
                  </button>
                  <button class="toolbar-button" :disabled="loading" @click="loadPrices(selectedGroupID)">
                    <Icon name="refresh" size="xs" :class="loading ? 'animate-spin' : ''" />
                    刷新
                  </button>
                </div>
              </div>
            </div>
          </section>

          <section v-if="selectedGroup" class="mt-4 grid gap-3 md:grid-cols-3">
            <div class="metric-tile">
              <span>当前分组</span>
              <strong>{{ selectedGroup.name }}</strong>
            </div>
            <div class="metric-tile">
              <span>{{ isAdmin ? '套餐单价' : '我们的价格' }}</span>
              <strong v-if="selectedGroup.best_plan">
                ¥{{ formatNumber(selectedGroup.best_plan.cny_per_quota_usd, 4) }}/额度 USD<span v-if="isAdmin"> · 分组 {{ formatNumber(selectedGroup.effective_multiplier, 2) }}x</span> · {{ selectedGroup.best_plan.name }}
              </strong>
              <strong v-else>暂无套餐，显示参考价</strong>
            </div>
            <div class="metric-tile">
              <span>优惠力度</span>
              <strong class="text-emerald-300">{{ selectedGroup.best_plan ? saveFactorLabel(groupSaleMultiplier(selectedGroup)) : '按官方参考' }}</strong>
            </div>
          </section>

          <div v-if="loading" class="flex min-h-[420px] items-center justify-center text-sm text-slate-400">
            正在加载模型价格...
          </div>
          <div v-else-if="filteredModels.length === 0" class="flex min-h-[420px] items-center justify-center text-sm text-slate-400">
            暂无可展示模型
          </div>
          <template v-else>
          <section class="mt-4 hidden overflow-hidden rounded-lg border border-white/[0.08] bg-[#14161b] xl:block">
            <div class="price-table-header" :class="{ admin: isAdmin }">
              <span>模型</span>
              <span>我们的价格</span>
              <span>官方参考价</span>
              <span>阶梯价格</span>
              <span>优惠力度</span>
              <span v-if="isAdmin">来源</span>
            </div>
            <article
              v-for="model in filteredModels"
              :key="model.platform + ':' + model.name"
              class="price-table-row"
              :class="{ admin: isAdmin }"
            >
              <div class="flex min-w-0 items-center gap-3">
                <ProviderMark :provider="model.platform" />
                <div class="min-w-0">
                  <div class="flex min-w-0 items-center gap-2">
                    <h2 class="truncate text-sm font-bold text-slate-100" :title="model.name">{{ model.name }}</h2>
                    <button class="copy-button" title="复制模型名" @click="copyText(model.name)">
                      <Icon name="copy" size="xs" />
                    </button>
                  </div>
                  <div class="mt-1.5 flex flex-wrap items-center gap-1.5">
                    <span class="tag">{{ providerLabel(model.platform) }}</span>
                    <span class="tag tag-purple">{{ billingLabel(model.billing_mode) }}</span>
                    <span v-if="isAdmin && model.channel_names.length > 0" class="tag tag-blue">{{ model.channel_names.length }} 渠道</span>
                  </div>
                </div>
              </div>

              <div class="price-pair actual">
                <strong>输入 {{ formatCNYPerMillion(bestActual(model, 'input_cny_per_m')) }}</strong>
                <strong>输出 {{ formatCNYPerMillion(bestActual(model, 'output_cny_per_m')) }}</strong>
                <small v-if="selectedGroup?.best_plan">{{ selectedGroup.best_plan.name }}</small>
              </div>

              <div class="price-pair muted">
                <span>输入 {{ formatUSDPerMillion(model.official.input_usd_per_m) }}</span>
                <span>输出 {{ formatUSDPerMillion(model.official.output_usd_per_m) }}</span>
                <small v-if="isAdmin && showOfficial">缓存读 {{ formatUSD(model.official.cache_read_usd_per_m) }} / 写 {{ formatUSD(model.official.cache_write_usd_per_m) }}</small>
              </div>

              <div class="tier-list">
                <template v-if="tierBadges(model).length > 0">
                  <span
                    v-for="tier in tierBadges(model)"
                    :key="tier.key"
                    class="tier-chip"
                    :title="tier.detail"
                  >
                    {{ tier.label }}
                  </span>
                </template>
                <span v-else class="text-xs text-slate-600">基础价</span>
              </div>

              <div>
                <span v-if="showMultiplier && isActuallyCheaper(model.cheaper_factor)" class="save-badge">
                  便宜 {{ formatNumber(model.cheaper_factor, 1) }} 倍
                </span>
                <span v-else-if="selectedGroup?.best_plan" class="save-badge neutral">
                  套餐折算
                </span>
                <span v-else class="text-xs font-semibold text-slate-500">参考</span>
              </div>

              <div v-if="isAdmin" class="source-cell">
                <span>{{ sourceLabel(model.pricing_source) }}</span>
                <small v-if="model.official_missing">缺官方价</small>
              </div>
            </article>
          </section>

          <section class="mt-4 grid gap-3 lg:grid-cols-2 xl:hidden">
            <article
              v-for="model in filteredModels"
              :key="model.platform + ':' + model.name"
              class="model-card group"
            >
              <div class="flex items-start gap-3">
                <ProviderMark :provider="model.platform" />
                <div class="min-w-0 flex-1">
                  <div class="flex min-w-0 items-start justify-between gap-3">
                    <h2 class="truncate text-base font-bold text-slate-100" :title="model.name">{{ model.name }}</h2>
                    <button class="copy-button" title="复制模型名" @click="copyText(model.name)">
                      <Icon name="copy" size="xs" />
                    </button>
                  </div>
                  <div class="mt-1.5 flex flex-wrap items-center gap-1.5">
                    <span class="tag">{{ providerLabel(model.platform) }}</span>
                    <span class="tag tag-purple">{{ billingLabel(model.billing_mode) }}</span>
                    <span v-if="tierBadges(model).length > 0" class="tag tag-blue">阶梯价格</span>
                  </div>
                </div>
              </div>

              <div class="mt-4 rounded-lg border border-emerald-400/10 bg-emerald-400/[0.035] p-3">
                <div class="mb-2 flex items-center justify-between gap-2">
                  <span class="text-[11px] font-bold text-emerald-200">我们的价格</span>
                  <span v-if="selectedGroup?.best_plan" class="truncate text-[11px] text-slate-500" :title="selectedGroup.best_plan.name">
                    {{ selectedGroup.best_plan.name }}
                  </span>
                </div>
                <div class="space-y-2">
                  <PriceRow label="输入价格" :value="bestActual(model, 'input_cny_per_m')" suffix="/ 百万 tokens" highlight />
                  <PriceRow label="输出价格" :value="bestActual(model, 'output_cny_per_m')" suffix="/ 百万 tokens" highlight />
                  <PriceRow label="缓存读取" :value="bestActual(model, 'cache_read_cny_per_m')" suffix="/ 百万 tokens" />
                  <PriceRow label="缓存创建" :value="bestActual(model, 'cache_write_cny_per_m')" suffix="/ 百万 tokens" />
                </div>
              </div>

              <div v-if="tierBadges(model).length > 0" class="tier-strip">
                <div
                  v-for="tier in tierBadges(model)"
                  :key="tier.key"
                  class="tier-line"
                >
                  <span class="truncate" :title="tier.detail">{{ tier.label }}</span>
                  <strong>可选</strong>
                </div>
              </div>

              <div v-if="showOfficial" class="mt-3 rounded-md border border-white/[0.08] bg-black/[0.16] p-2.5">
                <div class="mb-1.5 text-[11px] font-semibold text-slate-500">官方参考价</div>
                <div class="grid grid-cols-2 gap-2 text-xs text-slate-400">
                  <span>输入 {{ formatUSDPerMillion(model.official.input_usd_per_m) }}</span>
                  <span>输出 {{ formatUSDPerMillion(model.official.output_usd_per_m) }}</span>
                  <span>缓存读 {{ formatUSDPerMillion(model.official.cache_read_usd_per_m) }}</span>
                  <span>缓存写 {{ formatUSDPerMillion(model.official.cache_write_usd_per_m) }}</span>
                </div>
              </div>

              <div class="mt-4 flex items-center justify-between gap-3">
                <span class="rounded-full bg-violet-500/[0.12] px-2.5 py-1 text-xs font-semibold text-violet-200">{{ billingLabel(model.billing_mode) }}</span>
                <span v-if="showMultiplier && isActuallyCheaper(model.cheaper_factor)" class="save-badge">
                  便宜 {{ formatNumber(model.cheaper_factor, 1) }} 倍
                </span>
                <span v-else-if="selectedGroup?.best_plan" class="save-badge neutral">
                  我们的价格
                </span>
              </div>
            </article>
          </section>
          </template>
        </main>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import modelPricesAPI, {
  type ModelPriceGroup,
  type ModelPriceModel,
  type ModelPriceResponse,
  type ModelPriceTier,
} from '@/api/modelPrices'
import { useAppStore, useAuthStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

type FilterTone = 'purple' | 'cyan' | 'amber' | 'emerald'
type ActualPriceKey = keyof ModelPriceModel['actual']
interface TierBadge {
  key: string
  label: string
  detail: string
}

const FilterSection = defineComponent({
  props: {
    title: { type: String, required: true },
  },
  setup(props, { slots }) {
    return () => h('section', { class: 'mb-6 border-t border-white/[0.08] pt-4' }, [
      h('h3', { class: 'mb-3 px-1 text-sm font-bold text-slate-100' }, props.title),
      h('div', { class: 'grid gap-2' }, slots.default?.()),
    ])
  },
})

const FilterPill = defineComponent({
  props: {
    active: { type: Boolean, default: false },
    label: { type: String, required: true },
    count: { type: Number, default: undefined },
    countLabel: { type: String, default: '' },
    tone: { type: String as () => FilterTone, default: 'purple' },
  },
  emits: ['click'],
  setup(props, { emit }) {
    return () => h('button', {
      class: ['filter-pill', `tone-${props.tone}`, props.active ? 'active' : ''],
      onClick: () => emit('click'),
    }, [
      h('span', { class: 'truncate' }, props.label),
      props.count !== undefined || props.countLabel
        ? h('span', { class: 'pill-count' }, props.countLabel || String(props.count))
        : null,
    ])
  },
})

const ProviderMark = defineComponent({
  props: {
    provider: { type: String, required: true },
    large: { type: Boolean, default: false },
  },
  setup(props) {
    return () => {
      const label = providerShort(props.provider)
      const isAnthropic = props.provider.toLowerCase().includes('anthropic')
      return h('div', {
        class: [
          'provider-mark',
          props.large ? 'large' : '',
          isAnthropic ? 'anthropic' : 'openai',
        ],
      }, label)
    }
  },
})

const PriceRow = defineComponent({
  props: {
    label: { type: String, required: true },
    value: { type: Number, default: null },
    suffix: { type: String, default: '' },
    highlight: { type: Boolean, default: false },
  },
  setup(props) {
    return () => h('div', { class: 'price-row' }, [
      h('span', { class: 'text-slate-400' }, props.label),
      h('strong', { class: props.highlight ? 'text-emerald-300' : 'text-slate-200' }, [
        props.value == null ? '暂未定价' : `¥${formatNumber(props.value, props.value < 1 ? 4 : 2)}`,
        props.value == null ? '' : h('small', { class: 'ml-1 text-[11px] font-medium text-slate-500' }, props.suffix),
      ]),
    ])
  },
})

const appStore = useAppStore()
const authStore = useAuthStore()
const response = ref<ModelPriceResponse | null>(null)
const selectedGroupID = ref<number | undefined>(undefined)
const selectedProvider = ref('all')
const selectedPlatform = ref('all')
const selectedBilling = ref('all')
const searchQuery = ref('')
const loading = ref(false)
const showOfficial = ref(false)
const showMultiplier = ref(true)

const isAdmin = computed(() => authStore.isAdmin)
const groups = computed(() => sortGroupsForSales(response.value?.groups ?? []))
const selectedGroup = computed<ModelPriceGroup | undefined>(() =>
  groups.value.find((group) => group.id === selectedGroupID.value),
)
const models = computed(() => response.value?.models ?? [])

const groupFilters = computed(() => groups.value)
const providerFilters = computed(() => {
  const counts = countBy(models.value, model => providerLabel(model.platform))
  return [
    { value: 'all', label: '全部供应商', count: models.value.length, tone: 'purple' as FilterTone },
    ...Object.entries(counts).map(([label, count]) => ({
      value: label,
      label,
      count,
      tone: label.toLowerCase().includes('anthropic') ? 'amber' as FilterTone : 'purple' as FilterTone,
    })),
  ]
})
const platformFilters = computed(() => {
  const counts = countBy(models.value, model => model.platform || 'unknown')
  return [
    { value: 'all', label: '全部端点', count: models.value.length },
    ...Object.entries(counts).map(([label, count]) => ({ value: label, label, count })),
  ]
})
const billingFilters = computed(() => {
  const counts = countBy(models.value, model => billingLabel(model.billing_mode))
  return [
    { value: 'all', label: '全部类型', count: models.value.length },
    ...Object.entries(counts).map(([label, count]) => ({ value: label, label, count })),
  ]
})

const filteredModels = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return models.value.filter((model) => {
    if (selectedProvider.value !== 'all' && providerLabel(model.platform) !== selectedProvider.value) return false
    if (selectedPlatform.value !== 'all' && model.platform !== selectedPlatform.value) return false
    if (selectedBilling.value !== 'all' && billingLabel(model.billing_mode) !== selectedBilling.value) return false
    if (!q) return true
    return model.name.toLowerCase().includes(q) ||
      model.platform.toLowerCase().includes(q) ||
      model.provider.toLowerCase().includes(q) ||
      model.channel_names.some(name => name.toLowerCase().includes(q))
  }).sort(compareModelsForSales)
})

watch(selectedGroupID, (id, oldID) => {
  if (id && oldID && id !== oldID) {
    loadPrices(id)
  }
})

async function loadPrices(groupID?: number) {
  loading.value = true
  try {
    response.value = await modelPricesAPI.getModelPrices({ group_id: groupID })
    const preferredGroupID = groupID ?? groupFilters.value[0]?.id ?? response.value.selected_group_id ?? response.value.groups[0]?.id
    selectedGroupID.value = preferredGroupID
    if (!groupID && preferredGroupID && response.value.selected_group_id !== preferredGroupID) {
      await loadPrices(preferredGroupID)
      return
    }
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '加载模型价格失败'))
  } finally {
    loading.value = false
  }
}

function resetFilters() {
  selectedProvider.value = 'all'
  selectedPlatform.value = 'all'
  selectedBilling.value = 'all'
  searchQuery.value = ''
}

function bestActual(model: ModelPriceModel, key: ActualPriceKey): number | undefined {
  const base = model.actual[key]
  if (typeof base === 'number' && Number.isFinite(base) && base > 0) return base
  for (const tier of model.price_tiers) {
    const value = tier.actual[key]
    if (typeof value === 'number' && Number.isFinite(value) && value > 0) return value
  }
  return undefined
}

function extraTiers(model: ModelPriceModel): ModelPriceTier[] {
  return model.price_tiers.filter((tier) => tier.key !== 'base')
}

function tierBadges(model: ModelPriceModel): TierBadge[] {
  const tiers = extraTiers(model)
  const hasLongContext = tiers.some(tier => Boolean(tier.threshold_tokens))
  const hasFast = tiers.some(tier => tier.label.toLowerCase().includes('fast'))
  const badges: TierBadge[] = []
  if (hasLongContext) {
    const thresholds = tiers
      .map(tier => tier.threshold_tokens)
      .filter((value): value is number => typeof value === 'number' && value > 0)
    const maxThreshold = thresholds.length > 0 ? Math.max(...thresholds) : undefined
    badges.push({
      key: 'long-context',
      label: maxThreshold ? `长上下文 >${formatCompactTokens(maxThreshold)}` : '长上下文',
      detail: tiers.filter(tier => tier.threshold_tokens).map(tier => `${tierTitle(tier)}：${tierPriceSummary(tier)}`).join('；'),
    })
  }
  if (hasFast) {
    badges.push({
      key: 'fast',
      label: 'Fast 加速价',
      detail: tiers.filter(tier => tier.label.toLowerCase().includes('fast')).map(tier => `${tierTitle(tier)}：${tierPriceSummary(tier)}`).join('；'),
    })
  }
  if (!hasLongContext && !hasFast && tiers.length > 0) {
    badges.push({
      key: 'tiered',
      label: `${tiers.length} 档阶梯价`,
      detail: tiers.map(tier => `${tierTitle(tier)}：${tierPriceSummary(tier)}`).join('；'),
    })
  }
  return badges
}

function tierTitle(tier: ModelPriceTier): string {
  if (!tier.threshold_tokens) return tier.label
  return `${tier.label} >${formatCompactTokens(tier.threshold_tokens)}`
}

function tierPriceSummary(tier: ModelPriceTier): string {
  const input = formatCNYPerMillion(tier.actual.input_cny_per_m)
  const output = formatCNYPerMillion(tier.actual.output_cny_per_m)
  if (input === '-' && output === '-') return '暂未定价'
  return `入 ${input} / 出 ${output}`
}

function formatCompactTokens(tokens: number): string {
  if (!Number.isFinite(tokens) || tokens <= 0) return '-'
  if (tokens >= 1000) return `${formatNumber(tokens / 1000, 0)}K`
  return formatNumber(tokens, 0)
}

function isActuallyCheaper(factor: number | null | undefined): factor is number {
  return factor != null && Number.isFinite(factor) && factor > 1
}

function countBy<T>(items: T[], getKey: (item: T) => string): Record<string, number> {
  return items.reduce<Record<string, number>>((acc, item) => {
    const key = getKey(item) || 'unknown'
    acc[key] = (acc[key] ?? 0) + 1
    return acc
  }, {})
}

function sortGroupsForSales(items: ModelPriceGroup[]): ModelPriceGroup[] {
  return [...items].sort((a, b) => {
    const factorDiff = groupSaveFactor(b) - groupSaveFactor(a)
    if (factorDiff !== 0) return factorDiff
    return a.name.localeCompare(b.name, 'zh-CN')
  })
}

function groupSaveFactor(group: ModelPriceGroup): number {
  const multiplier = groupSaleMultiplier(group)
  if (multiplier == null || !Number.isFinite(multiplier) || multiplier <= 0) return 0
  if (multiplier >= 1) return 1 / multiplier
  return 1 / multiplier
}

function compareModelsForSales(a: ModelPriceModel, b: ModelPriceModel): number {
  const factorDiff = numericOrZero(b.cheaper_factor) - numericOrZero(a.cheaper_factor)
  if (factorDiff !== 0) return factorDiff
  const rankDiff = modelVersionRank(b.name) - modelVersionRank(a.name)
  if (rankDiff !== 0) return rankDiff
  return a.name.localeCompare(b.name, 'zh-CN', { numeric: true })
}

function numericOrZero(value: number | null | undefined): number {
  return value != null && Number.isFinite(value) ? value : 0
}

function modelVersionRank(name: string): number {
  const matches = name.match(/\d+(?:\.\d+)?/g)
  if (!matches) return 0
  return Math.max(...matches.map(value => Number.parseFloat(value)).filter(Number.isFinite))
}

function providerLabel(platform: string): string {
  const lower = platform.toLowerCase()
  if (lower.includes('anthropic')) return 'Anthropic'
  if (lower.includes('openai')) return 'OpenAI'
  if (lower.includes('gemini')) return 'Gemini'
  if (lower.includes('antigravity')) return 'Antigravity'
  return platform || '未知供应商'
}

function providerShort(platform: string): string {
  const label = providerLabel(platform)
  if (label === 'Anthropic') return '✹'
  if (label === 'OpenAI') return '◎'
  if (label === 'Gemini') return 'G'
  if (label === 'Antigravity') return 'AG'
  return label.slice(0, 2).toUpperCase()
}

function billingLabel(mode: string): string {
  if (mode === 'request') return '按次计费'
  if (mode === 'image') return '图片计费'
  return '按量计费'
}

function sourceLabel(source: string): string {
  if (source === 'official') return '官方目录'
  if (source === 'channel') return '渠道配置'
  return '待补价'
}

function groupSaleMultiplier(group: ModelPriceGroup | undefined): number | undefined {
  if (!group?.best_plan) return undefined
  return normalizePositive(group.effective_multiplier) * group.best_plan.usd_multiplier
}

function normalizePositive(value: number | null | undefined): number {
  if (value == null || !Number.isFinite(value) || value <= 0) return 1
  return value
}

function saveFactorLabel(multiplier: number | null | undefined): string {
  if (multiplier == null || !Number.isFinite(multiplier) || multiplier <= 0) return '参考'
  if (multiplier >= 1) return '套餐价'
  return `便宜 ${formatNumber(1 / multiplier, 1)} 倍`
}

function formatUSD(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '-'
  return `$${formatNumber(value, value < 1 ? 4 : 2)}`
}

function formatUSDPerMillion(value: number | null | undefined): string {
  const formatted = formatUSD(value)
  return formatted === '-' ? '-' : `${formatted} / 百万 tokens`
}

function formatCNYCompact(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '-'
  return `¥${formatNumber(value, value < 1 ? 4 : 2)}`
}

function formatCNYPerMillion(value: number | null | undefined): string {
  const formatted = formatCNYCompact(value)
  return formatted === '-' ? '-' : `${formatted} / 百万 tokens`
}

function formatNumber(value: number | null | undefined, digits = 2): string {
  if (value == null || !Number.isFinite(value)) return '-'
  return new Intl.NumberFormat('zh-CN', {
    minimumFractionDigits: 0,
    maximumFractionDigits: digits,
  }).format(value)
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    appStore.showSuccess('已复制')
  } catch {
    appStore.showError('复制失败')
  }
}

async function copyVisibleModels() {
  const text = filteredModels.value.map(model => model.name).join('\n')
  if (!text) return
  await copyText(text)
}

onMounted(() => loadPrices())
</script>

<style scoped>
.model-market {
  color-scheme: dark;
}

.hero-stat {
  min-width: 110px;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.16);
  background: rgba(15, 23, 42, 0.24);
  padding: 9px 11px;
}

.hero-stat span {
  display: block;
  font-size: 11px;
  font-weight: 700;
  color: rgba(219, 234, 254, 0.72);
}

.hero-stat strong {
  margin-top: 3px;
  display: block;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 14px;
  color: white;
}

.filter-pill {
  display: flex;
  min-width: 0;
  min-height: 38px;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  border-radius: 7px;
  border: 1px solid rgba(255, 255, 255, 0.07);
  background: rgba(255, 255, 255, 0.025);
  padding: 7px 10px;
  color: rgb(203 213 225);
  font-size: 12px;
  font-weight: 700;
  text-align: left;
}

.filter-pill.active {
  background: rgba(255, 255, 255, 0.12);
  color: white;
}

.pill-count {
  flex: none;
  border-radius: 999px;
  padding: 1px 6px;
  max-width: 96px;
  white-space: normal;
  text-align: center;
  line-height: 1.15;
  font-size: 10px;
  background: rgba(255, 255, 255, 0.12);
}

.tone-purple.active .pill-count { color: #c4b5fd; }
.tone-cyan.active .pill-count { color: #67e8f9; }
.tone-amber.active .pill-count { color: #fcd34d; }
.tone-emerald.active .pill-count { color: #6ee7b7; }

.toolbar-button {
  display: inline-flex;
  height: 32px;
  align-items: center;
  gap: 6px;
  border-radius: 7px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(255, 255, 255, 0.035);
  padding: 0 10px;
  color: rgb(203 213 225);
  font-weight: 700;
}

.toolbar-button.active,
.toolbar-button:hover {
  background: rgba(255, 255, 255, 0.12);
  color: white;
}

.metric-tile {
  min-height: 72px;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: #15171c;
  padding: 14px;
}

.metric-tile span {
  display: block;
  font-size: 12px;
  color: rgb(100 116 139);
}

.metric-tile strong {
  margin-top: 6px;
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 15px;
  color: white;
}

.model-card {
  min-height: 186px;
  border-radius: 10px;
  border: 1px solid rgba(255, 255, 255, 0.07);
  background: #14161b;
  padding: 16px;
  box-shadow: inset 0 1px rgba(255, 255, 255, 0.03);
}

.model-card:hover {
  border-color: rgba(96, 165, 250, 0.45);
  background: #171a21;
}

.price-table-header {
  display: grid;
  min-height: 44px;
  grid-template-columns: minmax(280px, 1.35fr) minmax(220px, 1fr) minmax(200px, 0.9fr) minmax(180px, 0.9fr) 120px;
  align-items: center;
  gap: 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  background: #191c23;
  padding: 0 16px;
  font-size: 12px;
  font-weight: 800;
  color: rgb(100 116 139);
}

.price-table-header.admin {
  grid-template-columns: minmax(280px, 1.35fr) minmax(220px, 1fr) minmax(200px, 0.9fr) minmax(180px, 0.9fr) 120px 100px;
}

.price-table-row {
  display: grid;
  min-height: 94px;
  grid-template-columns: minmax(280px, 1.35fr) minmax(220px, 1fr) minmax(200px, 0.9fr) minmax(180px, 0.9fr) 120px;
  align-items: center;
  gap: 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  padding: 14px 16px;
}

.price-table-row.admin {
  grid-template-columns: minmax(280px, 1.35fr) minmax(220px, 1fr) minmax(200px, 0.9fr) minmax(180px, 0.9fr) 120px 100px;
}

.price-table-row:last-child {
  border-bottom: 0;
}

.price-table-row:hover {
  background: rgba(255, 255, 255, 0.025);
}

.price-pair {
  display: grid;
  gap: 5px;
  min-width: 0;
}

.price-pair strong,
.price-pair span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 13px;
}

.price-pair.actual strong {
  color: #86efac;
}

.price-pair.muted span {
  color: rgb(148 163 184);
}

.price-pair small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11px;
  color: rgb(100 116 139);
}

.tier-list {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  gap: 6px;
}

.tier-chip {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  border-radius: 999px;
  border: 1px solid rgba(125, 211, 252, 0.14);
  background: rgba(14, 165, 233, 0.08);
  padding: 3px 8px;
  font-size: 11px;
  font-weight: 700;
  color: #bae6fd;
}

.tier-chip.dim {
  border-color: rgba(255, 255, 255, 0.09);
  background: rgba(255, 255, 255, 0.04);
  color: rgb(148 163 184);
}

.source-cell {
  display: grid;
  gap: 5px;
  font-size: 12px;
  font-weight: 800;
  color: rgb(203 213 225);
}

.source-cell small {
  font-size: 11px;
  font-weight: 700;
  color: #fbbf24;
}

.provider-mark {
  display: flex;
  width: 36px;
  height: 36px;
  flex: none;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  background: #1d2027;
  font-size: 18px;
  font-weight: 900;
}

.provider-mark.large {
  width: 48px;
  height: 48px;
  font-size: 24px;
}

.provider-mark.openai {
  color: #dbeafe;
}

.provider-mark.anthropic {
  color: #fb923c;
}

.copy-button {
  display: inline-flex;
  width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  color: rgb(148 163 184);
}

.copy-button:hover {
  color: white;
  background: rgba(255, 255, 255, 0.08);
}

.tag {
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.07);
  padding: 2px 7px;
  font-size: 11px;
  font-weight: 700;
  color: rgb(203 213 225);
}

.tag-purple {
  color: #c4b5fd;
  background: rgba(139, 92, 246, 0.14);
}

.tag-blue {
  color: #93c5fd;
  background: rgba(59, 130, 246, 0.14);
}

.tier-strip {
  margin-top: 10px;
  display: grid;
  gap: 6px;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.07);
  background: rgba(255, 255, 255, 0.025);
  padding: 8px;
}

.tier-line {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  font-size: 11px;
  color: rgb(148 163 184);
}

.tier-line strong {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-weight: 800;
  color: rgb(203 213 225);
}

.price-row {
  display: flex;
  min-height: 21px;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  font-size: 12px;
}

.price-row strong {
  text-align: right;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 13px;
}

.save-badge {
  border-radius: 999px;
  background: rgba(16, 185, 129, 0.13);
  padding: 4px 9px;
  color: #6ee7b7;
  font-size: 12px;
  font-weight: 800;
}

.save-badge.neutral {
  background: rgba(148, 163, 184, 0.12);
  color: rgb(203 213 225);
}
</style>
