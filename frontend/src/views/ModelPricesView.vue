<template>
  <AppLayout>
    <div class="model-market -m-4 min-h-[calc(100vh-4rem)] bg-[#0f1014] text-slate-100 md:-m-6 lg:-m-8">
      <div class="flex min-h-[calc(100vh-4rem)]">
        <aside class="hidden w-[300px] shrink-0 border-r border-white/[0.08] bg-[#101114] px-4 py-5 xl:block">
          <div class="mb-5 flex items-center justify-between">
            <div>
              <p class="text-lg font-semibold text-white">筛选</p>
              <p class="mt-1 text-xs text-slate-500">按供应商和分组快速查看</p>
            </div>
            <button class="rounded-md border border-white/10 px-2.5 py-1 text-xs text-slate-300 hover:bg-white/[0.06]" @click="resetFilters">
              重置
            </button>
          </div>

          <FilterSection title="供应商">
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
              :count-label="group.best_plan ? saveFactorLabel(group.best_plan.usd_multiplier) : '参考'"
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

          <FilterSection title="端点类型">
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
                    <h1 class="text-xl font-bold text-white">模型价格广场</h1>
                    <span class="rounded-full bg-white px-2 py-0.5 text-xs font-semibold text-blue-700">
                      共 {{ filteredModels.length }} 个模型
                    </span>
                  </div>
                  <p class="mt-2 max-w-3xl text-sm text-blue-50/90">
                    价格按当前分组可售套餐折算，优先展示客户实际感知的到手人民币价格；官方价只做参考。
                  </p>
                </div>
                <div class="hidden shrink-0 grid-cols-3 gap-3 xl:grid">
                  <div class="hero-stat">
                    <span>官方汇率</span>
                    <strong>{{ formatNumber(response?.usd_cny_rate ?? 7, 4) }}</strong>
                  </div>
                  <div class="hero-stat">
                    <span>套餐折算</span>
                    <strong>{{ selectedGroup?.best_plan ? `¥${formatNumber(selectedGroup.best_plan.cny_per_quota_usd, 4)}` : '参考价' }}</strong>
                  </div>
                  <div class="hero-stat">
                    <span>优惠感知</span>
                    <strong>{{ selectedGroup?.best_plan ? saveFactorLabel(selectedGroup.best_plan.usd_multiplier) : '-' }}</strong>
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
                    placeholder="模糊搜索模型名称"
                  />
                </div>
                <div class="flex flex-wrap items-center gap-2 text-xs">
                  <button class="toolbar-button" :class="{ active: showOfficial }" @click="showOfficial = !showOfficial">
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
              <span>套餐单价</span>
              <strong v-if="selectedGroup.best_plan">
                ¥{{ formatNumber(selectedGroup.best_plan.cny_per_quota_usd, 4) }}/额度 USD · {{ selectedGroup.best_plan.name }}
              </strong>
              <strong v-else>暂无套餐，显示参考价</strong>
            </div>
            <div class="metric-tile">
              <span>客户可感知</span>
              <strong class="text-emerald-300">{{ selectedGroup.best_plan ? saveFactorLabel(selectedGroup.best_plan.usd_multiplier) : '按官方参考' }}</strong>
            </div>
          </section>

          <div v-if="loading" class="flex min-h-[420px] items-center justify-center text-sm text-slate-400">
            正在加载模型价格...
          </div>
          <div v-else-if="filteredModels.length === 0" class="flex min-h-[420px] items-center justify-center text-sm text-slate-400">
            暂无可展示模型
          </div>
          <section v-else class="mt-4 grid gap-3 2xl:grid-cols-3 lg:grid-cols-2">
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
                    <span v-if="model.price_tiers.length > 0" class="tag tag-blue">多档价格</span>
                  </div>
                </div>
              </div>

              <div class="mt-4 rounded-lg border border-emerald-400/10 bg-emerald-400/[0.035] p-3">
                <div class="mb-2 flex items-center justify-between gap-2">
                  <span class="text-[11px] font-bold text-emerald-200">套餐折算价</span>
                  <span v-if="selectedGroup?.best_plan" class="truncate text-[11px] text-slate-500" :title="selectedGroup.best_plan.name">
                    {{ selectedGroup.best_plan.name }}
                  </span>
                </div>
                <div class="space-y-2">
                  <PriceRow label="输入价格" :value="bestActual(model, 'input_cny_per_m')" suffix="/ 1M Tokens" highlight />
                  <PriceRow label="补全价格" :value="bestActual(model, 'output_cny_per_m')" suffix="/ 1M Tokens" highlight />
                  <PriceRow label="缓存读取" :value="bestActual(model, 'cache_read_cny_per_m')" suffix="/ 1M Tokens" />
                  <PriceRow label="缓存创建" :value="bestActual(model, 'cache_write_cny_per_m')" suffix="/ 1M Tokens" />
                </div>
              </div>

              <div v-if="extraTiers(model).length > 0" class="tier-strip">
                <div
                  v-for="tier in extraTiers(model)"
                  :key="tier.key"
                  class="tier-line"
                >
                  <span class="truncate" :title="tierTitle(tier)">{{ tierTitle(tier) }}</span>
                  <strong>{{ tierPriceSummary(tier) }}</strong>
                </div>
              </div>

              <div v-if="showOfficial" class="mt-3 rounded-md border border-white/[0.08] bg-black/[0.16] p-2.5">
                <div class="mb-1.5 text-[11px] font-semibold text-slate-500">官方参考价</div>
                <div class="grid grid-cols-2 gap-2 text-xs text-slate-400">
                  <span>输入 {{ formatUSD(model.official.input_usd_per_m) }}</span>
                  <span>输出 {{ formatUSD(model.official.output_usd_per_m) }}</span>
                  <span>缓存读 {{ formatUSD(model.official.cache_read_usd_per_m) }}</span>
                  <span>缓存写 {{ formatUSD(model.official.cache_write_usd_per_m) }}</span>
                </div>
              </div>

              <div class="mt-4 flex items-center justify-between gap-3">
                <span class="rounded-full bg-violet-500/[0.12] px-2.5 py-1 text-xs font-semibold text-violet-200">{{ billingLabel(model.billing_mode) }}</span>
                <span v-if="showMultiplier && isActuallyCheaper(model.cheaper_factor)" class="save-badge">
                  便宜 {{ formatNumber(model.cheaper_factor, 1) }} 倍
                </span>
                <span v-else-if="selectedGroup?.best_plan" class="save-badge">
                  套餐折算价
                </span>
              </div>
            </article>
          </section>
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
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

type FilterTone = 'purple' | 'cyan' | 'amber' | 'emerald'
type ActualPriceKey = keyof ModelPriceModel['actual']

const FilterSection = defineComponent({
  props: {
    title: { type: String, required: true },
  },
  setup(props, { slots }) {
    return () => h('section', { class: 'mb-6 border-t border-white/[0.08] pt-4' }, [
      h('h3', { class: 'mb-3 px-1 text-sm font-bold text-slate-100' }, props.title),
      h('div', { class: 'grid grid-cols-2 gap-2' }, slots.default?.()),
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
const response = ref<ModelPriceResponse | null>(null)
const selectedGroupID = ref<number | undefined>(undefined)
const selectedProvider = ref('all')
const selectedPlatform = ref('all')
const selectedBilling = ref('all')
const searchQuery = ref('')
const loading = ref(false)
const showOfficial = ref(false)
const showMultiplier = ref(true)

const groups = computed(() => response.value?.groups ?? [])
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
  })
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
    selectedGroupID.value = response.value.selected_group_id ?? response.value.groups[0]?.id
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

function tierTitle(tier: ModelPriceTier): string {
  if (!tier.threshold_tokens) return tier.label
  return `${tier.label} >${formatCompactTokens(tier.threshold_tokens)}`
}

function tierPriceSummary(tier: ModelPriceTier): string {
  const input = formatCNYCompact(tier.actual.input_cny_per_m)
  const output = formatCNYCompact(tier.actual.output_cny_per_m)
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

function saveFactorLabel(multiplier: number | null | undefined): string {
  if (multiplier == null || !Number.isFinite(multiplier) || multiplier <= 0) return '参考'
  if (multiplier >= 1) return '套餐价'
  return `便宜 ${formatNumber(1 / multiplier, 1)} 倍`
}

function formatUSD(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '-'
  return `$${formatNumber(value, value < 1 ? 4 : 2)}`
}

function formatCNYCompact(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '-'
  return `¥${formatNumber(value, value < 1 ? 4 : 2)}`
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
  height: 34px;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  border-radius: 7px;
  border: 1px solid rgba(255, 255, 255, 0.07);
  background: rgba(255, 255, 255, 0.025);
  padding: 0 10px;
  color: rgb(203 213 225);
  font-size: 12px;
  font-weight: 700;
}

.filter-pill.active {
  background: rgba(255, 255, 255, 0.12);
  color: white;
}

.pill-count {
  flex: none;
  border-radius: 999px;
  padding: 1px 6px;
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
</style>
