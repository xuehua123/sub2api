<template>
  <section
    class="border-y border-gray-200 py-5 dark:border-dark-700"
    data-testid="price-operations"
  >
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h2 class="text-base font-semibold">价格运维</h2>
      <div class="flex gap-3 text-sm">
        <RouterLink
          class="text-primary-600 hover:underline"
          to="/admin/channels/pricing"
          >维护实际计费价格</RouterLink
        ><RouterLink
          class="text-primary-600 hover:underline"
          to="/admin/audit-logs"
          >操作审计</RouterLink
        >
      </div>
    </div>
    <div class="mt-4 grid gap-6 lg:grid-cols-2">
      <div>
        <div class="flex items-center justify-between">
          <h3 class="text-sm font-medium">参考价目录</h3>
          <button
            class="btn btn-secondary btn-sm"
            :disabled="busy"
            @click="$emit('sync')"
          >
            <Icon name="refresh" size="sm" />同步目录
          </button>
        </div>
        <p class="mt-2 text-xs text-gray-500">
          {{ response?.catalog_status?.model_count ?? 0 }} 个模型 · 最后同步
          {{
            response?.catalog_status?.last_updated
              ? new Date(response.catalog_status.last_updated).toLocaleString(
                  'zh-CN',
                )
              : '未知'
          }}
        </p>
        <p
          v-if="response?.catalog_status?.local_hash"
          class="mt-1 break-all text-xs text-gray-500"
        >
          版本 {{ response.catalog_status.local_hash }}
        </p>
      </div>
      <form class="space-y-3" @submit.prevent="review = true">
        <h3 class="text-sm font-medium">展示换算参数</h3>
        <div class="grid grid-cols-2 gap-3">
          <label class="text-xs"
            >USD/CNY<input
              v-model.number="rate"
              class="input mt-1"
              type="number"
              min="0.000001"
              step="any"
              required
              :disabled="busy" /></label
          ><label class="text-xs"
            >人民币 / 额度 USD<input
              v-model.number="quota"
              class="input mt-1"
              type="number"
              min="0.000001"
              step="any"
              required
              :disabled="busy"
          /></label>
        </div>
        <p class="text-xs text-gray-500">
          USD/CNY 用于官方原价的人民币等值试算；额度换算参数仅供销售展示配置使用，不改变本页官方原价。
        </p>
        <button class="btn btn-secondary btn-sm" :disabled="busy || !dirty">
          预览并保存
        </button>
      </form>
    </div>
    <p
      v-if="lastAction"
      role="status"
      class="mt-4 text-sm text-emerald-700 dark:text-emerald-300"
    >
      {{ lastAction }}
    </p>
    <BaseDialog
      :show="review"
      title="确认展示换算参数"
      width="narrow"
      :show-close-button="!busy"
      :close-on-escape="!busy"
      @close="!busy && (review = false)"
      ><dl class="space-y-3 text-sm">
        <div>
          <dt>USD/CNY</dt>
          <dd>{{ response?.usd_cny_rate }} → {{ rate }}</dd>
        </div>
        <div>
          <dt>人民币 / 额度 USD</dt>
          <dd>{{ response?.cny_per_quota_usd }} → {{ quota }}</dd>
        </div>
      </dl>
      <template #footer
        ><div class="flex justify-end gap-2">
          <button
            class="btn btn-secondary"
            :disabled="busy"
            @click="review = false"
          >
            取消</button
          ><button class="btn btn-primary" :disabled="busy" @click="save">
            {{ busy ? '保存中…' : '确认保存' }}
          </button>
        </div></template
      ></BaseDialog
    >
  </section>
</template>
<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import type { ModelPriceResponse } from '@/api/modelPrices'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
const props = defineProps<{
  response: ModelPriceResponse | null
  busy: boolean
  lastAction: string
  saveSettings: (rate: number, quota: number) => Promise<boolean>
}>()
defineEmits<{ sync: [] }>()
const rate = ref(props.response?.usd_cny_rate || 0),
  quota = ref(props.response?.cny_per_quota_usd || 0),
  review = ref(false)
const original = ref([rate.value, quota.value])
const dirty = computed(
  () => rate.value !== original.value[0] || quota.value !== original.value[1],
)
watch(
  () => props.response,
  (data) => {
    if (data && !dirty.value) {
      rate.value = data.usd_cny_rate
      quota.value = data.cny_per_quota_usd
      original.value = [rate.value, quota.value]
    }
  },
)
async function save() {
  if (await props.saveSettings(Number(rate.value), Number(quota.value))) {
    original.value = [rate.value, quota.value]
    review.value = false
  }
}
</script>
