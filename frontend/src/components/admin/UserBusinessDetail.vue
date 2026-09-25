<template>
  <BaseDialog
    :show="Boolean(user)"
    :title="(user?.username || user?.email || '') + ' · #' + (user?.id ?? '')"
    width="extra-wide"
    @close="$emit('close')"
  >
    <div v-if="loading" role="status" class="py-16 text-center text-gray-500">
      {{ t('userBusiness.loading') }}
    </div>
    <p v-else-if="error" role="alert" class="text-red-600">
      {{ t('userBusiness.failed') }}
      <button class="underline" @click="load">
        {{ t('userBusiness.retry') }}
      </button>
    </p>
    <div v-else-if="detail" class="space-y-5">
      <p class="text-xs text-gray-500">
        {{ t('userBusiness.assetNote') }} · {{ formatDate(detail.as_of) }}
      </p>
      <div class="grid grid-cols-2 gap-4 border-y py-3 dark:border-dark-700">
        <div>
          <p class="text-xs text-gray-500">
            {{ t('userBusiness.balance') }} · {{ t('userBusiness.quota') }}
          </p>
          <p class="text-xl font-semibold tabular-nums">
            {{ quota(detail.balance) }}
          </p>
        </div>
        <div>
          <p class="text-xs text-gray-500">{{ t('userBusiness.cards') }}</p>
          <p class="text-xl font-semibold">{{ detail.cards.length }}</p>
        </div>
      </div>
      <p v-if="detail.cards.length" class="text-xs text-gray-500">
        {{ t('userBusiness.quotaPairs') }}
      </p>
      <div v-if="detail.cards.length" class="overflow-x-auto">
        <table class="min-w-[760px] w-full text-sm">
          <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-800">
            <tr>
              <th class="p-2 text-left">{{ t('userBusiness.cards') }}</th>
              <th
                class="p-2 text-right"
                :title="t('userBusiness.currentPaidHint')"
              >
                {{ t('userBusiness.paidCard') }}
              </th>
              <th class="p-2 text-left">{{ t('userBusiness.expiry') }}</th>
              <th class="p-2 text-right">{{ t('userBusiness.dayQuota') }}</th>
              <th class="p-2 text-right">{{ t('userBusiness.weekQuota') }}</th>
              <th class="p-2 text-right">{{ t('userBusiness.monthQuota') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="card in detail.cards"
              :key="card.id"
              data-testid="business-card"
              class="border-t dark:border-dark-700"
            >
              <td class="p-2">{{ card.name || card.source }}</td>
              <td
                class="p-2 text-right"
                :title="t('userBusiness.currentPaidHint')"
              >
                {{
                  card.paid_cny === null
                    ? t('userBusiness.unknownPayment')
                    : money(card.paid_cny)
                }}
              </td>
              <td class="p-2 whitespace-nowrap">
                {{ formatDate(card.expires_at) }}
              </td>
              <td class="p-2 text-right tabular-nums">
                {{ quotaLimit(card.daily_remaining, card.daily_limit) }}
              </td>
              <td class="p-2 text-right tabular-nums">
                {{ quotaLimit(card.weekly_remaining, card.weekly_limit) }}
              </td>
              <td class="p-2 text-right tabular-nums">
                {{ quotaLimit(card.monthly_remaining, card.monthly_limit) }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-else class="text-sm text-gray-500">
        {{ t('userBusiness.noCards') }}
      </p>
      <section>
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <h3 class="text-sm font-semibold">{{ t('userBusiness.trend') }}</h3>
          <div class="inline-flex rounded-md border p-0.5 dark:border-dark-600">
            <button
              v-for="item in ['money', 'quotaTrend'] as const"
              :key="item"
              class="rounded px-3 py-1 text-xs"
              :class="
                mode === item
                  ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
                  : 'text-gray-500'
              "
              :aria-pressed="mode === item"
              @click="mode = item"
            >
              {{ t('userBusiness.' + item) }}
            </button>
          </div>
        </div>
        <div class="h-64 w-full">
          <Line
            :data="chartData"
            :options="chartOptions"
            :aria-label="t('userBusiness.trend')"
          />
        </div>
      </section>
      <details>
        <summary class="cursor-pointer text-sm">
          {{ t('userBusiness.trend') }} · {{ t('userBusiness.details') }}
        </summary>
        <div class="max-h-64 overflow-auto">
          <table class="w-full min-w-[600px] text-sm">
            <thead class="text-xs text-gray-500">
              <tr>
                <th class="p-2">{{ t('userBusiness.date') }}</th>
                <th class="p-2">{{ t('userBusiness.paid') }}</th>
                <th class="p-2">{{ t('userBusiness.refund') }}</th>
                <th class="p-2">{{ t('userBusiness.consumption') }}</th>
                <th class="p-2">{{ t('userBusiness.cost') }}</th>
                <th class="p-2">{{ t('userBusiness.profit') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="day in detail.daily"
                :key="day.date"
                class="border-t dark:border-dark-700"
              >
                <td class="p-2">{{ day.date }}</td>
                <td class="p-2 text-right">{{ money(day.paid) }}</td>
                <td class="p-2 text-right">{{ money(day.refund) }}</td>
                <td class="p-2 text-right">{{ quota(day.consumption) }}</td>
                <td class="p-2 text-right">{{ money(day.cost) }}</td>
                <td class="p-2 text-right">{{ money(day.profit) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </details>
      <section>
        <h3 class="text-sm font-semibold">{{ t('userBusiness.events') }}</h3>
        <p class="mt-1 text-xs text-gray-500">
          {{ t('userBusiness.eventLimit', { count: detail.payments_total }) }}
        </p>
        <div class="mt-2 max-h-72 overflow-auto">
          <table class="w-full min-w-[600px] text-sm">
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-800">
              <tr>
                <th class="p-2">{{ t('userBusiness.date') }}</th>
                <th class="p-2">{{ t('userBusiness.order') }}</th>
                <th class="p-2">{{ t('userBusiness.type') }}</th>
                <th class="p-2 text-right">{{ t('userBusiness.paid') }}</th>
                <th class="p-2 text-right">{{ t('userBusiness.refund') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="(event, i) in detail.payments"
                :key="i"
                class="border-t dark:border-dark-700"
              >
                <td class="p-2">{{ formatDate(event.at) }}</td>
                <td class="p-2">{{ event.order_id ?? '—' }}</td>
                <td class="p-2">
                  {{
                    t(
                      event.order_type === 'subscription'
                        ? 'userBusiness.subscription'
                        : event.order_type === 'balance'
                          ? 'userBusiness.cash'
                          : 'userBusiness.review',
                    )
                  }}<span
                    v-if="event.uncertain"
                    class="ml-1 text-xs text-amber-600"
                    >{{ t('userBusiness.review') }}</span
                  >
                </td>
                <td class="p-2 text-right">{{ money(event.paid) }}</td>
                <td class="p-2 text-right">{{ money(event.refund) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>
  </BaseDialog>
</template>
<script setup lang="ts">
import { computed, ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  type ChartOptions,
} from 'chart.js'
import { Line } from 'vue-chartjs'
import BaseDialog from '@/components/common/BaseDialog.vue'
import {
  getBusinessDetail,
  type BusinessDetail,
  type BusinessRow,
  type BusinessFilters,
} from '@/api/admin/userBusiness'
ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
)
const props = defineProps<{
  user: BusinessRow | null
  filters: BusinessFilters
}>()
defineEmits<{ close: [] }>()
const { t } = useI18n()
const detail = ref<BusinessDetail | null>(null),
  loading = ref(false),
  error = ref(false),
  mode = ref<'money' | 'quotaTrend'>('money')
let generation = 0,
  controller: AbortController | undefined
const dark = ref(document.documentElement.classList.contains('dark'))
const observer = new MutationObserver(() => {
  dark.value = document.documentElement.classList.contains('dark')
})
onMounted(() =>
  observer.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class'],
  }),
)
async function load() {
  const current = ++generation
  controller?.abort()
  controller = new AbortController()
  detail.value = null
  error.value = false
  if (!props.user) {
    loading.value = false
    return
  }
  loading.value = true
  try {
    const data = await getBusinessDetail(
      props.user.id,
      props.filters,
      controller.signal,
    )
    if (current === generation) detail.value = data
  } catch {
    if (current === generation) error.value = true
  } finally {
    if (current === generation) loading.value = false
  }
}
watch(
  () => [props.user?.id, props.filters],
  () => void load(),
)
const money = (v: number | null) =>
  v === null
    ? t('userBusiness.review')
    : new Intl.NumberFormat('zh-CN', {
        style: 'currency',
        currency: 'CNY',
      }).format(v)
const quota = (v: number) =>
  new Intl.NumberFormat('en-US', { maximumFractionDigits: 4 }).format(v)
const quotaLimit = (remaining: number | null, limit: number | null) =>
  limit === null
    ? t('userBusiness.unlimited')
    : quota(remaining ?? 0) + ' / ' + quota(limit)
const formatDate = (value: string) =>
  new Intl.DateTimeFormat('zh-CN', {
    dateStyle: 'short',
    timeStyle: 'short',
    timeZone: detail.value?.timezone || 'Asia/Shanghai',
  }).format(new Date(value))
const chartData = computed(() => {
  const days = detail.value?.daily ?? []
  const metrics =
    mode.value === 'money'
      ? [
          { key: 'paid' as const, color: '#0284c7' },
          { key: 'cost' as const, color: '#d97706' },
          { key: 'profit' as const, color: '#059669' },
        ]
      : [{ key: 'consumption' as const, color: '#6366f1' }]
  return {
    labels: days.map((d) => d.date),
    datasets: metrics.map((m) => ({
      label: t('userBusiness.' + m.key),
      data: days.map((d) => d[m.key]),
      borderColor: m.color,
      backgroundColor: m.color,
      borderWidth: 2,
      pointRadius: days.length > 31 ? 0 : 2,
      pointHitRadius: 10,
      spanGaps: false,
      tension: 0,
    })),
  }
})
const chartOptions = computed<ChartOptions<'line'>>(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: false,
  interaction: { mode: 'index', intersect: false },
  plugins: {
    legend: {
      labels: {
        color: dark.value ? '#d1d5db' : '#374151',
        usePointStyle: true,
      },
    },
    tooltip: {
      callbacks: {
        label: (ctx) =>
          ctx.dataset.label +
          ': ' +
          (mode.value === 'money'
            ? money(ctx.raw === null ? null : Number(ctx.raw))
            : quota(Number(ctx.raw))),
      },
    },
  },
  scales: {
    x: {
      ticks: {
        maxTicksLimit: 7,
        maxRotation: 0,
        color: dark.value ? '#9ca3af' : '#6b7280',
      },
      grid: { display: false },
    },
    y: {
      beginAtZero: true,
      ticks: {
        color: dark.value ? '#9ca3af' : '#6b7280',
        callback: (value) => (mode.value === 'money' ? '¥' : '') + value,
      },
      grid: { color: dark.value ? '#374151' : '#e5e7eb' },
    },
  },
}))
onBeforeUnmount(() => {
  generation++
  controller?.abort()
  observer.disconnect()
})
</script>
