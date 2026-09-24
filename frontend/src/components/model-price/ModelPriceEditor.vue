<template>
  <BaseDialog
    :show="true"
    :title="'编辑展示价 · ' + model.name"
    width="normal"
    :close-on-escape="!busy"
    :show-close-button="!busy"
    @close="!busy && $emit('close')"
  >
    <form id="model-price-edit" class="space-y-4" @submit.prevent="review">
      <p class="text-sm text-amber-700 dark:text-amber-300">
        只修改管理员价格管理中的展示配置，不修改官方原价或网关实际计费。未填写的价格保持未知，不按 0
        计算。
      </p>
      <p class="text-sm">{{ groupName }} · #{{ groupID }}</p>
      <label class="block text-sm"
        >计费方式<select
          v-model="mode"
          :disabled="busy || reviewing || clearReview"
          class="input mt-1"
          data-testid="price-editor-mode"
        >
          <option value="token">Token</option>
          <option value="per_request">每次</option>
          <option value="image">每张图片</option>
          <option value="video">视频每秒</option>
        </select></label
      >
      <div class="grid gap-3 sm:grid-cols-2">
        <label v-for="f in visibleFields" :key="f.key" class="text-sm"
          >{{ f.label
          }}<input
            v-model="values[f.key]"
            :disabled="busy || reviewing || clearReview"
            type="number"
            min="0"
            step="any"
            class="input mt-1"
            :data-testid="'price-editor-' + f.key"
            placeholder="未定价"
        /></label>
      </div>
      <p v-if="validation" role="alert" class="text-sm text-red-600">
        {{ validation }}
      </p>
      <div
        v-if="reviewing"
        class="border-t border-gray-200 pt-3 dark:border-dark-700"
        data-testid="price-editor-review"
      >
        <h4 class="text-sm font-medium">确认变更</h4>
        <dl
          v-for="f in visibleFields"
          :key="f.key"
          class="mt-2 flex flex-wrap justify-between gap-2 text-xs"
        >
          <dt>{{ f.label }}</dt>
          <dd>
            {{ money(model.actual[f.key], 'USD') }} →
            {{ money(payload?.[f.key], 'USD') }}
          </dd>
        </dl>
      </div>
      <p
        v-if="clearReview"
        role="status"
        class="border-t pt-3 text-sm text-amber-700"
      >
        确认清除自定义展示价？将恢复系统计算的参考价，不改变网关计费。
      </p>
    </form>
    <template #footer
      ><div class="flex flex-wrap justify-between gap-2">
        <button
          v-if="model.custom_price && !clearReview"
          type="button"
          class="btn btn-secondary"
          :disabled="busy"
          @click="startClear"
        >
          清除覆盖
        </button>
        <div class="ml-auto flex gap-2">
          <button
            class="btn btn-secondary"
            :disabled="busy"
            @click="
              clearReview
                ? (clearReview = false)
                : reviewing
                  ? (reviewing = false)
                  : $emit('close')
            "
          >
            {{ reviewing ? '返回修改' : '取消' }}</button
          ><button
            v-if="!reviewing && !clearReview"
            form="model-price-edit"
            type="submit"
            class="btn btn-primary"
            :disabled="busy"
          >
            预览变更</button
          ><button
            v-else-if="reviewing"
            class="btn btn-primary"
            :disabled="busy"
            @click="payload && $emit('save', payload)"
          >
            {{ busy ? '保存中…' : '确认保存展示价' }}
          </button>
          <button
            v-else
            class="btn btn-danger"
            :disabled="busy"
            @click="
              $emit('save', {
                group_id: groupID,
                model: model.name,
                clear: true,
              })
            "
          >
            确认清除覆盖
          </button>
        </div>
      </div></template
    >
  </BaseDialog>
</template>
<script setup lang="ts">
import { ref, computed, reactive } from 'vue'
import type {
  ModelPriceModel,
  UpdateModelPriceCustomPriceRequest,
} from '@/api/modelPrices'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { kind, money, validPrice } from './pricing'
const props = defineProps<{
  model: ModelPriceModel
  groupID: number
  groupName: string
  busy: boolean
}>()
defineEmits<{
  save: [request: UpdateModelPriceCustomPriceRequest]
  close: []
}>()
const mode = ref(kind(props.model) === 'unknown' ? 'token' : kind(props.model))
const fields = [
  { key: 'input_usd_per_m', label: '输入 USD / 百万 tokens' },
  { key: 'output_usd_per_m', label: '输出 USD / 百万 tokens' },
  { key: 'cache_write_usd_per_m', label: '缓存写入 USD / 百万 tokens' },
  { key: 'cache_read_usd_per_m', label: '缓存读取 USD / 百万 tokens' },
  { key: 'image_output_usd_per_m', label: '图片输出 USD / 百万 tokens' },
  { key: 'per_request_usd', label: '固定单价 USD / 计费单位' },
] as const
const values = reactive(
  Object.fromEntries(
    fields.map((f) => {
      const v = props.model.custom_price
        ? props.model.custom_price[f.key]
        : props.model.actual[f.key]
      return [f.key, validPrice(v) ? String(v) : '']
    }),
  ) as Record<(typeof fields)[number]['key'], string>,
)
const visibleFields = computed(() =>
  fields.filter((f) =>
    mode.value === 'token'
      ? f.key !== 'per_request_usd'
      : f.key === 'per_request_usd',
  ),
)
const reviewing = ref(false),
  clearReview = ref(false),
  validation = ref('')
const payload = ref<UpdateModelPriceCustomPriceRequest>()
function startClear() { clearReview.value = true; reviewing.value = false }
function review() {
  if (props.busy || clearReview.value) return
  validation.value = ''
  const body: UpdateModelPriceCustomPriceRequest = {
    group_id: props.groupID,
    model: props.model.name,
    billing_mode: mode.value,
  }
  let configured = false
  for (const f of visibleFields.value) {
    const raw = String(values[f.key]).trim()
    if (!raw) {
      body[f.key] = null
      continue
    }
    const value = Number(raw)
    if (!validPrice(value)) {
      validation.value = '价格必须是非负有限数'
      return
    }
    body[f.key] = value
    configured = true
  }
  if (!configured) {
    validation.value = '至少填写一个价格'
    return
  }
  payload.value = body
  reviewing.value = true
}
</script>
