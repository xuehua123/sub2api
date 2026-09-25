<template>
  <section
    class="border-y border-gray-200 py-5 dark:border-dark-700"
    data-testid="payment-usage-profit"
  >
    <div class="flex items-center justify-between gap-3">
      <h3 class="text-sm font-semibold">
        {{ t('upstreamWorkspace.usageProfit') }}
      </h3>
      <button
        class="btn btn-secondary"
        :disabled="loading"
        :title="t('common.refresh')"
        :aria-label="t('common.refresh')"
        @click="load"
      >
        <Icon name="refresh" size="sm" />
      </button>
    </div>
    <p class="mt-2 text-xs text-gray-500">
      {{ t('upstreamWorkspace.usageProfitScope') }} {{ result?.timezone }}
    </p>
    <p v-if="loading" role="status" class="py-12 text-center text-gray-500">
      {{ t('upstreamWorkspace.busy') }}
    </p>
    <p v-else-if="error" role="alert" class="py-6 text-red-600">
      {{ t('upstreamWorkspace.profitLoadError') }}
    </p>
    <template v-else-if="result">
      <div class="my-4 grid grid-cols-1 gap-3 sm:grid-cols-3">
        <div v-for="item in summaries" :key="item.label">
          <p class="text-xs text-gray-500">{{ item.label }}</p>
          <p
            class="mt-1 text-xl font-semibold tabular-nums"
            :style="{ color: item.color }"
          >
            {{ money(item.value) }}
          </p>
        </div>
      </div>
      <div class="relative h-64 w-full sm:h-80">
        <Line
          :data="chartData"
          :options="chartOptions"
          :aria-label="t('upstreamWorkspace.usageProfit')"
        />
      </div>
    </template>
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
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
import Icon from '@/components/icons/Icon.vue'
import { apiClient } from '@/api/client'
ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
)
interface ProfitTrend {
  daily: {
    date: string
    account_cost: number
    revenue: number
    gross_profit: number
  }[]
  total_cost: number
  total_revenue: number
  gross_profit: number
  timezone: string
  currency: string
}
const props = defineProps<{ days: number }>()
const { t } = useI18n()
const result = ref<ProfitTrend | null>(null),
  loading = ref(false),
  error = ref(false)
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
let generation = 0,
  controller: AbortController | undefined
async function load() {
  const current = ++generation
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  error.value = false
  result.value = null
  try {
    const { data } = await apiClient.get<ProfitTrend>(
      '/admin/payment/usage-profit',
      { params: { days: props.days }, signal: controller.signal },
    )
    if (current === generation) result.value = data
  } catch {
    if (current === generation) error.value = true
  } finally {
    if (current === generation) loading.value = false
  }
}
watch(() => props.days, load, { immediate: true })
onBeforeUnmount(() => {
  generation++
  controller?.abort()
  observer.disconnect()
})
const money = (v: number) =>
  new Intl.NumberFormat(undefined, {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits: 4,
  }).format(v)
const metrics = computed(() => [
  {
    key: 'revenue' as const,
    label: t('upstreamWorkspace.revenue'),
    color: '#0284c7',
  },
  {
    key: 'account_cost' as const,
    label: t('upstreamWorkspace.accountCost'),
    color: '#d97706',
  },
  {
    key: 'gross_profit' as const,
    label: t('upstreamWorkspace.grossProfit'),
    color: '#059669',
  },
])
const summaries = computed(() =>
  metrics.value.map((m) => ({
    ...m,
    value:
      m.key === 'revenue'
        ? (result.value?.total_revenue ?? 0)
        : m.key === 'account_cost'
          ? (result.value?.total_cost ?? 0)
          : (result.value?.gross_profit ?? 0),
  })),
)
const chartData = computed(() => ({
  labels: result.value?.daily.map((d) => d.date) ?? [],
  datasets: metrics.value.map((m) => ({
    label: m.label,
    data: result.value?.daily.map((d) => d[m.key]) ?? [],
    borderColor: m.color,
    backgroundColor: m.color,
    borderWidth: 2,
    pointRadius: props.days <= 30 ? 2 : 0,
    pointHitRadius: 10,
    tension: 0,
  })),
}))
const chartOptions = computed<ChartOptions<'line'>>(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: false,
  normalized: true,
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
        label: (ctx) => ctx.dataset.label + ': ' + money(Number(ctx.raw)),
      },
    },
  },
  scales: {
    x: {
      grid: { display: false },
      ticks: {
        color: dark.value ? '#9ca3af' : '#6b7280',
        maxTicksLimit: 8,
        maxRotation: 0,
      },
    },
    y: {
      beginAtZero: true,
      ticks: {
        color: dark.value ? '#9ca3af' : '#6b7280',
        callback: (v) => '$' + v,
      },
      grid: { color: dark.value ? '#374151' : '#e5e7eb' },
    },
  },
}))
</script>
