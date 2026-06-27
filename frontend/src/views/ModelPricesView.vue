<template>
  <AppLayout>
    <div class="min-h-full bg-gray-50/60 px-4 py-6 dark:bg-dark-950 sm:px-6 lg:px-8">
      <div class="mx-auto flex max-w-7xl flex-col gap-5">
        <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
            <div class="space-y-2">
              <p class="text-sm font-medium text-primary-600 dark:text-primary-400">模型价格</p>
              <h1 class="text-2xl font-semibold text-gray-950 dark:text-white">按分组查看官方价与倍率参考价</h1>
              <p class="max-w-3xl text-sm text-gray-500 dark:text-gray-400">
                官方价格来自系统模型价格表，倍率参考价按当前分组倍率和 USD/CNY 汇率计算，仅作价格参考。
              </p>
            </div>

            <div class="grid gap-3 sm:grid-cols-3 lg:w-[520px]">
              <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-800/70">
                <p class="text-xs text-gray-500 dark:text-gray-400">模型数</p>
                <p class="mt-1 text-xl font-semibold text-gray-950 dark:text-white">{{ response?.summary.model_count ?? 0 }}</p>
              </div>
              <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-800/70">
                <p class="text-xs text-gray-500 dark:text-gray-400">已匹配价格</p>
                <p class="mt-1 text-xl font-semibold text-gray-950 dark:text-white">{{ response?.summary.priced_count ?? 0 }}</p>
              </div>
              <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-800/70">
                <p class="text-xs text-gray-500 dark:text-gray-400">平均便宜</p>
                <p class="mt-1 text-xl font-semibold text-emerald-600 dark:text-emerald-400">
                  {{ formatFactor(response?.summary.average_cheaper_factor) }}
                </p>
              </div>
            </div>
          </div>
        </section>

        <section class="grid gap-5 lg:grid-cols-[320px_minmax(0,1fr)]">
          <aside class="space-y-4">
            <div class="rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-900">
              <label class="input-label mb-2 block">分组</label>
              <select v-model.number="selectedGroupID" class="input">
                <option v-for="group in groups" :key="group.id" :value="group.id">
                  {{ group.name }} · {{ group.platform }}
                </option>
              </select>

              <div v-if="selectedGroup" class="mt-4 space-y-3">
                <div class="flex items-center justify-between gap-3">
                  <span class="text-sm text-gray-500 dark:text-gray-400">计费倍率</span>
                  <span class="font-mono text-sm font-semibold text-gray-950 dark:text-white">
                    x{{ formatNumber(selectedGroup.effective_multiplier, 4) }}
                  </span>
                </div>
                <div class="flex items-center justify-between gap-3">
                  <span class="text-sm text-gray-500 dark:text-gray-400">相对官方</span>
                  <span class="text-sm font-semibold" :class="savingClass(selectedGroup.effective_multiplier)">
                    {{ savingText(selectedGroup.effective_multiplier) }}
                  </span>
                </div>
                <div class="flex items-center justify-between gap-3">
                  <span class="text-sm text-gray-500 dark:text-gray-400">类型</span>
                  <span class="rounded-full bg-gray-100 px-2 py-1 text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-200">
                    {{ selectedGroup.subscription_type === 'subscription' ? '订阅分组' : '余额分组' }}
                  </span>
                </div>
              </div>
            </div>

            <div class="rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-900">
              <div class="flex items-center justify-between gap-3">
                <div>
                  <p class="text-sm font-medium text-gray-950 dark:text-white">USD/CNY 汇率</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">用于人民币价格换算</p>
                </div>
                <span class="font-mono text-sm font-semibold text-gray-950 dark:text-white">{{ formatNumber(usdCnyRate, 4) }}</span>
              </div>
              <div v-if="authStore.isAdmin" class="mt-4 flex gap-2">
                <input
                  v-model="rateDraft"
                  type="number"
                  min="0.0001"
                  step="0.0001"
                  class="input"
                  placeholder="7"
                />
                <button class="btn btn-primary whitespace-nowrap" :disabled="savingRate" @click="saveRate">
                  保存
                </button>
              </div>
            </div>
          </aside>

          <main class="rounded-lg border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <div class="border-b border-gray-200 p-4 dark:border-dark-700">
              <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                <div class="relative w-full lg:max-w-md">
                  <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                  <input v-model="searchQuery" type="text" class="input pl-10" placeholder="搜索模型、平台、渠道" />
                </div>
                <button class="btn btn-secondary" :disabled="loading" @click="loadPrices(selectedGroupID)">
                  <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
                  <span>刷新</span>
                </button>
              </div>
            </div>

            <div v-if="loading" class="flex min-h-[360px] items-center justify-center text-sm text-gray-500">
              加载价格中...
            </div>
            <div v-else-if="filteredModels.length === 0" class="flex min-h-[360px] items-center justify-center text-sm text-gray-500">
              暂无可展示模型
            </div>
            <div v-else class="overflow-x-auto">
              <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
                <thead class="bg-gray-50 text-xs uppercase text-gray-500 dark:bg-dark-800 dark:text-gray-400">
                  <tr>
                    <th class="px-4 py-3 text-left font-medium">模型</th>
                    <th class="px-4 py-3 text-left font-medium">官方 USD</th>
                    <th class="px-4 py-3 text-left font-medium">参考 USD</th>
                    <th class="px-4 py-3 text-left font-medium">参考人民币</th>
                    <th class="px-4 py-3 text-left font-medium">倍率</th>
                    <th class="px-4 py-3 text-left font-medium">便宜</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                  <tr v-for="model in filteredModels" :key="model.platform + ':' + model.name" class="hover:bg-gray-50/80 dark:hover:bg-dark-800/60">
                    <td class="px-4 py-4 align-top">
                      <div class="min-w-[220px]">
                        <p class="font-medium text-gray-950 dark:text-white">{{ model.name }}</p>
                        <div class="mt-2 flex flex-wrap gap-1.5">
                          <span class="rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-800 dark:text-gray-300">{{ model.platform }}</span>
                          <span class="rounded-full px-2 py-0.5 text-xs" :class="sourceClass(model.pricing_source)">
                            {{ sourceText(model.pricing_source) }}
                          </span>
                        </div>
                        <p class="mt-2 max-w-xs truncate text-xs text-gray-400" :title="model.channel_names.join(', ')">
                          {{ model.channel_names.join(', ') }}
                        </p>
                      </div>
                    </td>
                    <td class="px-4 py-4 align-top">
                      <TierPriceColumn
                        :tiers="displayTiers(model)"
                        column="official"
                        empty="未匹配"
                      />
                    </td>
                    <td class="px-4 py-4 align-top">
                      <TierPriceColumn
                        :tiers="displayTiers(model)"
                        column="actual-usd"
                        empty="-"
                      />
                    </td>
                    <td class="px-4 py-4 align-top">
                      <TierPriceColumn
                        :tiers="displayTiers(model)"
                        column="actual-cny"
                        empty="-"
                      />
                    </td>
                    <td class="px-4 py-4 align-top">
                      <span class="font-mono font-semibold text-gray-950 dark:text-white">x{{ formatNumber(model.multiplier, 4) }}</span>
                    </td>
                    <td class="px-4 py-4 align-top">
                      <span class="font-semibold" :class="savingClass(model.multiplier)">
                        {{ savingText(model.multiplier) }}
                      </span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </main>
        </section>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import modelPricesAPI, { type ModelPriceGroup, type ModelPriceModel, type ModelPriceResponse, type ModelPriceTier } from '@/api/modelPrices'
import { updateSettings } from '@/api/admin/settings'
import { useAppStore, useAuthStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

type PriceLine = { label: string; value: number | null | undefined; currency: 'usd' | 'cny' }
type PriceColumn = 'official' | 'actual-usd' | 'actual-cny'

const PriceLines = defineComponent({
  props: {
    lines: { type: Array as () => PriceLine[], required: true },
    empty: { type: String, default: '-' }
  },
  setup(props) {
    return () => {
      const visible = props.lines.filter((line) => line.value != null)
      if (visible.length === 0) {
        return h('span', { class: 'text-gray-400' }, props.empty)
      }
      return h('div', { class: 'space-y-1.5' }, visible.map((line) =>
        h('div', { class: 'flex min-w-[160px] items-center justify-between gap-3' }, [
          h('span', { class: 'text-xs text-gray-500 dark:text-gray-400' }, line.label),
          h('span', { class: 'font-mono text-xs font-semibold text-gray-950 dark:text-white' }, formatMoney(line.value, line.currency)),
        ])
      ))
    }
  }
})

const TierPriceColumn = defineComponent({
  props: {
    tiers: { type: Array as () => ModelPriceTier[], required: true },
    column: { type: String as () => PriceColumn, required: true },
    empty: { type: String, default: '-' }
  },
  setup(props) {
    return () => {
      if (props.tiers.length === 0) {
        return h('span', { class: 'text-gray-400' }, props.empty)
      }
      return h('div', { class: 'min-w-[190px] space-y-3' }, props.tiers.map((tier) =>
        h('div', { class: 'border-l-2 border-gray-200 pl-3 dark:border-dark-700' }, [
          h('div', { class: 'mb-1.5 flex flex-wrap items-center gap-1.5' }, [
            h('span', { class: 'rounded bg-gray-100 px-1.5 py-0.5 text-[11px] font-medium text-gray-700 dark:bg-dark-800 dark:text-gray-200' }, tier.label),
            tier.threshold_tokens
              ? h('span', { class: 'text-[11px] text-gray-400' }, `>${formatCompactTokens(tier.threshold_tokens)}`)
              : null,
          ]),
          h(PriceLines, { lines: tierLines(tier, props.column), empty: props.empty }),
        ])
      ))
    }
  }
})

const appStore = useAppStore()
const authStore = useAuthStore()

const response = ref<ModelPriceResponse | null>(null)
const selectedGroupID = ref<number | undefined>(undefined)
const loading = ref(false)
const savingRate = ref(false)
const searchQuery = ref('')
const rateDraft = ref('7')

const groups = computed(() => response.value?.groups ?? [])
const usdCnyRate = computed(() => response.value?.usd_cny_rate ?? 7)
const selectedGroup = computed<ModelPriceGroup | undefined>(() => groups.value.find((group) => group.id === selectedGroupID.value))
const models = computed(() => response.value?.models ?? [])
const filteredModels = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return models.value
  return models.value.filter((model) =>
    model.name.toLowerCase().includes(q) ||
    model.platform.toLowerCase().includes(q) ||
    model.provider.toLowerCase().includes(q) ||
    model.channel_names.some((name) => name.toLowerCase().includes(q))
  )
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
    rateDraft.value = String(response.value.usd_cny_rate)
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '加载模型价格失败'))
  } finally {
    loading.value = false
  }
}

async function saveRate() {
  const nextRate = Number(rateDraft.value)
  if (!Number.isFinite(nextRate) || nextRate <= 0) {
    appStore.showError('请输入有效汇率')
    return
  }
  savingRate.value = true
  try {
    await updateSettings({ model_price_usd_cny_rate: nextRate })
    appStore.showSuccess('汇率已保存')
    await loadPrices(selectedGroupID.value)
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '保存汇率失败'))
  } finally {
    savingRate.value = false
  }
}

function displayTiers(model: ModelPriceModel): ModelPriceTier[] {
  if (Array.isArray(model.price_tiers) && model.price_tiers.length > 0) {
    return model.price_tiers
  }
  return [{
    key: 'base',
    label: '基础',
    official: model.official,
    actual: model.actual,
  }]
}

function tierLines(tier: ModelPriceTier, column: PriceColumn): PriceLine[] {
  if (column === 'official') return officialLines(tier)
  if (column === 'actual-usd') return actualUsdLines(tier)
  return actualCnyLines(tier)
}

function officialLines(tier: ModelPriceTier): PriceLine[] {
  return [
    { label: '输入 /1M', value: tier.official.input_usd_per_m, currency: 'usd' },
    { label: '输出 /1M', value: tier.official.output_usd_per_m, currency: 'usd' },
    { label: '缓存写 /1M', value: tier.official.cache_write_usd_per_m, currency: 'usd' },
    { label: '缓存读 /1M', value: tier.official.cache_read_usd_per_m, currency: 'usd' },
    { label: '图片 /1M', value: tier.official.image_output_usd_per_m, currency: 'usd' },
    { label: '每次', value: tier.official.per_request_usd, currency: 'usd' },
  ]
}

function actualUsdLines(tier: ModelPriceTier): PriceLine[] {
  return [
    { label: '输入 /1M', value: tier.actual.input_usd_per_m, currency: 'usd' },
    { label: '输出 /1M', value: tier.actual.output_usd_per_m, currency: 'usd' },
    { label: '缓存写 /1M', value: tier.actual.cache_write_usd_per_m, currency: 'usd' },
    { label: '缓存读 /1M', value: tier.actual.cache_read_usd_per_m, currency: 'usd' },
    { label: '图片 /1M', value: tier.actual.image_output_usd_per_m, currency: 'usd' },
    { label: '每次', value: tier.actual.per_request_usd, currency: 'usd' },
  ]
}

function actualCnyLines(tier: ModelPriceTier): PriceLine[] {
  return [
    { label: '输入 /1M', value: tier.actual.input_cny_per_m, currency: 'cny' },
    { label: '输出 /1M', value: tier.actual.output_cny_per_m, currency: 'cny' },
    { label: '缓存写 /1M', value: tier.actual.cache_write_cny_per_m, currency: 'cny' },
    { label: '缓存读 /1M', value: tier.actual.cache_read_cny_per_m, currency: 'cny' },
    { label: '图片 /1M', value: tier.actual.image_output_cny_per_m, currency: 'cny' },
    { label: '每次', value: tier.actual.per_request_cny, currency: 'cny' },
  ]
}

function formatCompactTokens(tokens: number) {
  if (!Number.isFinite(tokens) || tokens <= 0) return '-'
  if (tokens >= 1000) return `${formatNumber(tokens / 1000, 0)}K`
  return formatNumber(tokens, 0)
}

function formatMoney(value: number | null | undefined, currency: 'usd' | 'cny') {
  if (value == null || !Number.isFinite(value)) return '-'
  const prefix = currency === 'usd' ? '$' : '¥'
  return `${prefix}${formatNumber(value, value >= 1 ? 4 : 6)}`
}

function formatNumber(value: number | null | undefined, digits = 2) {
  if (value == null || !Number.isFinite(value)) return '-'
  return new Intl.NumberFormat('zh-CN', {
    minimumFractionDigits: 0,
    maximumFractionDigits: digits,
  }).format(value)
}

function formatFactor(value: number | null | undefined) {
  if (value == null || !Number.isFinite(value)) return '-'
  return value >= 1 ? `${formatNumber(value, 2)} 倍` : `高于官方 ${formatNumber(1 / value, 2)} 倍`
}

function savingText(multiplier: number) {
  if (!Number.isFinite(multiplier) || multiplier <= 0) return '-'
  if (multiplier < 1) return `便宜 ${formatNumber(1 / multiplier, 2)} 倍`
  if (multiplier === 1) return '与官方持平'
  return `高于官方 ${formatNumber(multiplier, 2)} 倍`
}

function savingClass(multiplier: number) {
  if (multiplier < 1) return 'text-emerald-600 dark:text-emerald-400'
  if (multiplier === 1) return 'text-gray-600 dark:text-gray-300'
  return 'text-amber-600 dark:text-amber-400'
}

function sourceText(source: string) {
  if (source === 'official') return '官方价'
  if (source === 'channel') return '渠道价'
  return '无价格'
}

function sourceClass(source: string) {
  if (source === 'official') return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300'
  if (source === 'channel') return 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300'
  return 'bg-gray-100 text-gray-600 dark:bg-dark-800 dark:text-gray-300'
}

onMounted(() => loadPrices())
</script>
