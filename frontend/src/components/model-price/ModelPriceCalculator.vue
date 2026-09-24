<template>
  <div data-testid="model-price-calculator" class="space-y-5">
    <div class="grid gap-3 sm:grid-cols-2">
      <label class="text-sm"
        >模型<select
          v-model="selectedKey"
          class="input mt-1"
          data-testid="calculator-model"
        >
          <option
            v-for="item in models"
            :key="modelKey(item)"
            :value="modelKey(item)"
          >
            {{ item.name }}
          </option>
        </select></label
      >
      <label class="text-sm"
        >计价档位<select
          v-model="tierKey"
          class="input mt-1"
          data-testid="calculator-tier"
        >
          <option value="base">基础价</option>
          <option
            v-for="tier in model?.price_tiers.filter((t) => t.key !== 'base') ||
            []"
            :key="tier.key"
            :value="tier.key"
          >
            {{ tier.label
            }}{{
              tier.threshold_tokens
                ? ' · 输入 > ' + tier.threshold_tokens + ' tokens'
                : ''
            }}
          </option>
        </select></label
      >
    </div>
    <p
      v-if="selectedTier?.requires_account_long_context"
      role="status"
      class="text-sm text-amber-700 dark:text-amber-300"
    >
      此档位仅在所用账号开启长上下文计费时适用。
    </p>
    <p class="text-xs text-gray-500">
      {{basis==='display'?'管理模式展示价试算':'官方原价试算（仅按汇率换算人民币）'}}，不发起请求、不扣费。Token
      输入按未缓存、缓存读取、缓存写入分别填写，不能重复计入。
    </p>
    <div class="grid gap-3 sm:grid-cols-2">
      <label v-for="field in fields" :key="field.key" class="text-sm"
        >{{
          field.key === 'quantity'
            ? '数量（' + unit(model!) + '）'
            : field.label + ' tokens'
        }}
        <input
          v-model.number="usage[field.key]"
          type="number"
          min="0"
          :step="
            field.key === 'quantity' && model && kind(model) === 'video'
              ? 'any'
              : '1'
          "
          class="input mt-1"
          :data-testid="'calculator-' + field.key"
        />
      </label>
      <label class="text-sm"
        >请求次数<input
          v-model.number="usage.requests"
          type="number"
          min="0"
          step="1"
          class="input mt-1"
          data-testid="calculator-requests"
      /></label>
    </div>
    <div
      class="border-t border-gray-200 pt-4 dark:border-dark-700"
      aria-live="polite"
    >
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="text-sm">{{basis==='display'?'展示价预计费用':'官方原价折合人民币'}}</span
        ><strong
          class="text-xl tabular-nums text-emerald-700 dark:text-emerald-300"
          data-testid="calculator-total"
          >{{
            result.total === null ? '暂不可估算' : money(result.total)
          }}</strong
        >
      </div>
      <p
        v-if="result.reason"
        class="mt-2 text-sm text-amber-700 dark:text-amber-300"
      >
        {{ result.reason }}
      </p>
      <dl
        v-for="line in result.lines"
        :key="line.label"
        class="mt-2 flex justify-between text-xs text-gray-500"
      >
        <dt>{{ line.label }}</dt>
        <dd>{{ money(line.amount) }}</dd>
      </dl>
      <p
        v-if="model && isDisplayOverride(model)"
        class="mt-3 text-sm text-amber-700 dark:text-amber-300"
      >
        使用自定义展示价估算，不代表网关实际结算价格。
      </p>
    </div>
  </div>
</template>
<script setup lang="ts">
import { computed, ref, reactive, watch } from 'vue'
import type { ModelPriceModel } from '@/api/modelPrices'
import {
  modelKey,
  tariff,
  tariffFields,
  estimate,
  kind,
  unit,
  money,
  isDisplayOverride,
  type Usage,
} from './pricing'
const props = defineProps<{
  models: ModelPriceModel[]
  initialModel?: string
  basis?: 'catalog' | 'display'
}>()
const selectedKey = ref(props.initialModel || '')
const tierKey = ref('base')
const usage = reactive<Usage>({
  input: 1000,
  output: 500,
  cache_read: 0,
  cache_write: 0,
  cache_write_5m: 0,
  cache_write_1h: 0,
  image_input: 0,
  image_output: 0,
  quantity: 1,
  requests: 1,
})
watch(
  () => props.models,
  (items) => {
    if (!items.some((m) => modelKey(m) === selectedKey.value))
      selectedKey.value = items[0] ? modelKey(items[0]) : ''
  },
  { immediate: true },
)
watch(
  () => props.initialModel,
  (k) => {
    if (k) selectedKey.value = k
  },
)
const model = computed(() => props.models.find(m => modelKey(m) === selectedKey.value))
watch(selectedKey, () => {
  tierKey.value = 'base'
  const price = model.value?.actual
  const imageInput = price?.input_cny_per_m == null && price?.image_input_cny_per_m != null
  const imageOutput = price?.output_cny_per_m == null && price?.image_output_cny_per_m != null
  Object.assign(usage, {input:imageInput?0:1000,output:imageOutput?0:500,cache_read:0,cache_write:0,cache_write_5m:0,cache_write_1h:0,image_input:imageInput?1000:0,image_output:imageOutput?500:0,quantity:1,requests:1})
}, {immediate:true})
const selectedTier = computed(() =>
  model.value?.price_tiers.find((t) => t.key === tierKey.value),
)
const fields = computed(() =>
  model.value
    ? tariffFields(model.value).filter(
        (f) =>
          ['input', 'output', 'quantity'].includes(f.key) ||
          tariff(model.value!, tierKey.value)?.actual[f.cny] != null ||
          usage[f.key] !== 0,
      )
    : [],
)
const result = computed(() =>
  model.value
    ? estimate(model.value, tierKey.value, usage)
    : { total: null, reason: '没有可估算的模型', lines: [] },
)
</script>
