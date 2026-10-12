<template>
  <article
    class="price-item"
    :class="{ 'opacity-70': model.hidden }"
    data-testid="model-price-row"
    :data-model-name="model.name"
  >
    <div class="price-item-main">
      <div class="flex min-w-0 items-start gap-2">
        <input
          v-if="manage"
          type="checkbox"
          class="mt-2 h-4 w-4 shrink-0"
          :checked="selected"
          :disabled="busy"
          :aria-label="'选择 ' + model.name"
          :data-testid="'model-select-' + model.name"
          @change="$emit('select')"
        />
        <div class="min-w-0 flex-1">
          <div class="flex items-start gap-2">
            <strong class="break-all text-sm">{{ model.name }}</strong
            ><button
              type="button"
              class="price-icon"
              :title="'复制 ' + model.name"
              :aria-label="'复制 ' + model.name"
              @click="$emit('copy', model.name)"
            >
              <Icon name="copy" size="sm" />
            </button>
          </div>
          <p class="mt-1 text-xs text-gray-500">
            {{ model.provider || model.platform }} · {{ modeLabel(model) }}
          </p>
          <p
            v-if="model.hidden"
            class="mt-1 text-xs text-amber-700 dark:text-amber-300"
          >
            已隐藏
          </p>
          <p
            v-if="isDisplayOverride(model)"
            class="mt-1 text-xs text-amber-700 dark:text-amber-300"
          >
            展示价覆盖 · 非结算承诺
          </p>
        </div>
      </div>
      <dl v-if="kind(model) === 'token'" class="price-main-values">
        <div>
          <dt>{{primaryInput.image ? '图片输入' : '输入'}} / 百万 tokens</dt>
          <dd>{{money(primaryInput.value, primaryInput.currency)}}</dd>
        </div>
        <div>
          <dt>{{primaryOutput.image ? '图片输出' : '输出'}} / 百万 tokens</dt>
          <dd>{{money(primaryOutput.value, primaryOutput.currency)}}</dd>
        </div>
      </dl>
      <dl v-else class="price-main-values">
        <div>
          <dt>每{{ unit(model) }}</dt>
          <dd>
            {{
              kind(model) === 'unknown'
                ? '单位待确认'
                : money(primaryFixed.value, primaryFixed.currency)
            }}
          </dd>
        </div>
      </dl>
      <div class="flex flex-wrap items-center gap-2 text-xs">
        <span>{{ sourceLabel(model.pricing_source) }}</span
        ><span
          v-if="
            showDiscount && model.cheaper_factor && model.cheaper_factor > 1
          "
          >基准价的 {{ (100 / model.cheaper_factor).toFixed(1) }}%</span
        ><span
          v-for="issue in issues(model, basis)"
          :key="issue"
          class="text-amber-700 dark:text-amber-300"
          >{{ issueLabels[issue] }}</span
        >
      </div>
      <div class="flex items-center justify-end gap-2">
        <button
          class="btn btn-secondary btn-sm"
          @click="$emit('estimate', model)"
        >
          试算</button
        ><button
          v-if="manage"
          class="price-icon"
          title="编辑展示价"
          aria-label="编辑展示价"
          @click="$emit('edit', model)"
        >
          <Icon name="edit" size="sm" />
        </button>
      </div>
    </div>
    <details
      :open="showBaseline"
      class="price-details"
      @toggle="expanded = ($event.target as HTMLDetailsElement).open"
    >
      <summary>
        价格明细<span class="ml-2 text-gray-500"
          >{{basis==='display'?'计费基准价':'官方原价'}} · 缓存{{
            model.price_tiers.length ? ' · 阶梯价' : ''
          }}</span
        >
      </summary>
      <div v-if="expanded || showBaseline" class="space-y-4 py-3">
        <p class="text-xs text-gray-500"><template v-if="basis==='display'">当前计费价按 CNY 显示，可能包含分组倍率或自定义展示价。</template><template v-else>官方目录价按 USD 显示，不叠加分组倍率或折扣。</template><span v-if="model.catalog_price?.model && model.catalog_price.model !== model.name">目录模型：{{ model.catalog_price.model }}</span></p>
        <div v-for="tier in tariffs" :key="tier.key" class="overflow-x-auto">
          <h3 class="mb-2 text-sm font-medium">
            {{ tier.label
            }}<span v-if="tier.threshold_tokens" class="ml-2 text-xs"
              >输入 > {{ tier.threshold_tokens }} tokens</span
            >
          </h3>
          <p
            v-if="tier.requires_account_long_context"
            class="mb-2 text-xs text-amber-700"
          >
            需账号开启长上下文计费
          </p>
          <table class="w-full text-left text-xs">
            <thead>
              <tr>
                <th class="py-2">项目 / {{ unit(model) }}</th>
                <th>{{ basis === 'display' ? '当前计费展示价 / CNY' : '官方目录原价 / USD' }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="f in tariffFields(model)"
                :key="f.key"
                class="border-t border-gray-100 dark:border-dark-700"
              >
                <td class="py-2">{{ f.label }}</td>
                <td>{{ money(detailValue(tier, f), detailCurrency) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-if="manage" class="break-words text-xs text-gray-500">
          渠道来源：{{
            model.channel_names.join('、') || '未提供，不能据此保证可调用'
          }}
        </p>
      </div>
    </details>
  </article>
</template>
<script setup lang="ts">
import { computed, ref } from 'vue'
import type { ModelPriceModel, ModelPriceTier } from '@/api/modelPrices'
import Icon from '@/components/icons/Icon.vue'
import {
  money,
  kind,
  unit,
  modeLabel,
  sourceLabel,
  tariffFields,
  priceFields,
  issues,
  issueLabels,
  isDisplayOverride,
  type PriceBasis,
} from './pricing'
const props = withDefaults(defineProps<{
  model: ModelPriceModel
  basis?: PriceBasis
  manage: boolean
  selected: boolean
  busy: boolean
  showBaseline: boolean
  showDiscount: boolean
  exchangeRate?: number
}>(), { basis: 'display' })
defineEmits<{
  select: []
  copy: [text: string]
  edit: [model: ModelPriceModel]
  estimate: [model: ModelPriceModel]
}>()
const primaryInput=computed(()=>{
 const text=props.basis==='display'?props.model.actual.input_cny_per_m:props.model.official.input_usd_per_m
 const image=props.basis==='display'?props.model.actual.image_input_cny_per_m:props.model.official.image_input_usd_per_m
 return {value:text ?? image,image:text==null && image!=null,currency:props.basis==='display'?'CNY':'USD'}
})
const primaryOutput=computed(()=>{
 const text=props.basis==='display'?props.model.actual.output_cny_per_m:props.model.official.output_usd_per_m
 const image=props.basis==='display'?props.model.actual.image_output_cny_per_m:props.model.official.image_output_usd_per_m
 return {value:text ?? image,image:text==null && image!=null,currency:props.basis==='display'?'CNY':'USD'}
})
const primaryFixed=computed(()=>({value:props.basis==='display'?props.model.actual.per_request_cny:props.model.official.per_request_usd,currency:props.basis==='display'?'CNY':'USD'}))
const detailCurrency = computed(() => (props.basis === 'display' ? 'CNY' : 'USD'))
function detailValue(
  tier: ModelPriceTier | { actual: ModelPriceTier['actual']; official: ModelPriceTier['official'] },
  field: (typeof priceFields)[number],
) {
  return props.basis === 'display'
    ? tier.actual[field.cny]
    : tier.official[field.usd]
}
const expanded = ref(false)
const tariffs = computed(() => [
  {
    key: 'base',
    label: '基础价',
    actual: props.model.actual,
    official: props.model.official,
    threshold_tokens: undefined as number | undefined,
    requires_account_long_context: false,
  },
  ...props.model.price_tiers.filter((t) => t.key !== 'base'),
])
</script>
