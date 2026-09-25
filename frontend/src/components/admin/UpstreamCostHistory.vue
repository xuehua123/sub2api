<template>
  <BaseDialog
    :show="show"
    :title="t('upstreamWorkspace.history')"
    width="wide"
    @close="$emit('close')"
  >
    <div class="space-y-4">
      <div class="flex flex-wrap items-center gap-2">
        <button
          v-for="preset in presets"
          :key="preset"
          class="btn btn-secondary btn-sm"
          @click="choosePreset(preset)"
        >
          {{ t('upstreamWorkspace.' + preset) }}
        </button>
      </div>
      <form class="flex flex-wrap items-end gap-3" @submit.prevent="load(true)">
        <label class="w-full min-w-0 text-xs text-gray-500 sm:w-auto sm:flex-1"
          >{{ t('upstreamWorkspace.start')
          }}<input
            v-model="start"
            type="date"
            class="input mt-1"
            required
            :max="end"
        /></label>
        <label class="w-full min-w-0 text-xs text-gray-500 sm:w-auto sm:flex-1"
          >{{ t('upstreamWorkspace.end')
          }}<input
            v-model="end"
            type="date"
            class="input mt-1"
            required
            :min="start"
            :max="today()"
        /></label>
        <button class="btn btn-primary" :disabled="loading">
          {{ t('upstreamWorkspace.query') }}
        </button>
      </form>
      <label class="block text-xs text-gray-500"
        >{{ t('upstreamWorkspace.upstream') }}
        <select v-model="connection" class="input mt-1" @change="load()">
          <option value="">{{ t('upstreamWorkspace.allCost') }}</option>
          <option
            v-for="item in connections"
            :key="item.id"
            :value="String(item.id)"
          >
            {{ item.name }}
          </option>
        </select>
      </label>
      <p class="text-xs text-gray-500">
        {{ t('upstreamWorkspace.costScope') }}
        <span v-if="result">{{ result.timezone }}</span>
      </p>
      <div v-if="loading" role="status" class="py-8 text-center text-gray-500">
        {{ t('upstreamWorkspace.busy') }}
      </div>
      <p v-else-if="error" role="alert" class="text-sm text-red-600">
        {{ error }}
      </p>
      <template v-else-if="result">
        <div
          class="grid grid-cols-2 divide-x border-y border-gray-200 py-4 dark:border-dark-600"
        >
          <div class="pr-4">
            <p class="text-xs text-gray-500">
              {{ t('upstreamWorkspace.totalCost') }}
            </p>
            <p
              data-testid="history-total"
              class="mt-1 text-2xl font-semibold tabular-nums text-emerald-700 dark:text-emerald-300"
            >
              {{ money(result.total_cost) }}
            </p>
          </div>
          <div class="pl-4">
            <p class="text-xs text-gray-500">
              {{ t('upstreamWorkspace.requests') }}
            </p>
            <p class="mt-1 text-2xl font-semibold tabular-nums">
              {{ result.total_requests.toLocaleString() }}
            </p>
          </div>
        </div>
        <section aria-labelledby="history-trend-heading" class="min-w-0">
          <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
            <h3 id="history-trend-heading" class="text-sm font-semibold">
              {{ t('upstreamWorkspace.costTrend') }}
            </h3>
            <div
              class="inline-flex rounded-md border border-gray-200 p-0.5 dark:border-dark-600"
            >
              <button
                v-for="mode in ['daily', 'cumulative'] as const"
                :key="mode"
                type="button"
                :aria-pressed="chartMode === mode"
                class="rounded px-3 py-1 text-xs"
                :class="
                  chartMode === mode
                    ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
                    : 'text-gray-500'
                "
                @click="chartMode = mode"
              >
                {{ t('upstreamWorkspace.' + mode) }}
              </button>
            </div>
          </div>
          <div class="relative h-56 w-full sm:h-64" data-testid="history-chart">
            <Line
              v-if="props.show"
              :data="chartData"
              :options="chartOptions"
              :aria-label="t('upstreamWorkspace.costTrend')"
            />
          </div>
          <details class="mt-3 text-sm">
            <summary
              class="cursor-pointer py-2 text-gray-600 dark:text-gray-300"
            >
              {{ t('upstreamWorkspace.dailyDetails') }}
            </summary>
            <div class="max-h-64 overflow-auto">
              <table class="w-full text-sm">
                <thead
                  class="sticky top-0 bg-gray-50 text-xs text-gray-500 dark:bg-dark-800"
                >
                  <tr>
                    <th class="p-2 text-left">
                      {{ t('upstreamWorkspace.date') }}
                    </th>
                    <th class="p-2 text-right">
                      {{ t('upstreamWorkspace.daily') }}
                    </th>
                    <th class="p-2 text-right">
                      {{ t('upstreamWorkspace.requests') }}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="day in result.daily"
                    :key="day.date"
                    data-testid="daily-cost-row"
                    class="border-b border-gray-100 dark:border-dark-700"
                  >
                    <td class="p-2">{{ day.date }}</td>
                    <td class="p-2 text-right tabular-nums">
                      {{ money(day.account_cost) }}
                    </td>
                    <td class="p-2 text-right tabular-nums">
                      {{ day.requests.toLocaleString() }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </details>
        </section>
        <div class="max-h-96 overflow-auto">
          <table class="w-full text-sm">
            <thead
              class="sticky top-0 bg-gray-50 text-xs text-gray-500 dark:bg-dark-800"
            >
              <tr>
                <th class="p-2 text-left">
                  {{ t('upstreamWorkspace.upstream') }}
                </th>
                <th class="p-2 text-right">
                  {{ t('upstreamWorkspace.totalCost') }}
                </th>
                <th class="p-2 text-right">
                  {{ t('upstreamWorkspace.requests') }}
                </th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="item in result.items"
                :key="item.connection_id"
                class="border-b border-gray-100 dark:border-dark-700"
              >
                <td class="p-2">{{ item.name }}</td>
                <td class="p-2 text-right tabular-nums">
                  {{ money(item.account_cost) }}
                </td>
                <td class="p-2 text-right tabular-nums">
                  {{ item.requests.toLocaleString() }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p
          v-if="result.total_requests === 0"
          class="text-center text-sm text-gray-500"
        >
          {{ t('upstreamWorkspace.noHistory') }}
        </p>
      </template>
    </div>
  </BaseDialog>
</template>
<script setup lang="ts">
import { computed, ref, watch, onMounted, onBeforeUnmount } from 'vue'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  type ChartOptions,
} from 'chart.js'
import { Line } from 'vue-chartjs'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { getCostHistory, type CostHistory } from '@/api/admin/upstreamCatalog'
ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip)
const props = defineProps<{
  show: boolean
  connections: { id: number; name: string }[]
}>()
defineEmits<{ close: [] }>()
const { t } = useI18n()
const presets = ['today', 'yesterday', 'week', 'month'] as const
const start = ref(''),
  end = ref(''),
  connection = ref(''),
  loading = ref(false),
  error = ref('')
const result = ref<CostHistory | null>(null)
const chartMode = ref<'daily' | 'cumulative'>('daily')
const dark = ref(document.documentElement.classList.contains('dark'))
const themeObserver = new MutationObserver(() => {
  dark.value = document.documentElement.classList.contains('dark')
})
onMounted(() =>
  themeObserver.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class'],
  }),
)
// Per-dialog bounded cache only: at most eight recent selections, never shared across users.
const cache = new Map<string, { at: number; data: CostHistory }>()
const selectionKey = () =>
  JSON.stringify([start.value, end.value, connection.value])
const chartData = computed(() => {
  const days = result.value?.daily ?? []
  let cumulative = 0
  return {
    labels: days.map((day) => day.date),
    datasets: [
      {
        label: t('upstreamWorkspace.' + chartMode.value),
        data: days.map((day) => {
          cumulative += day.account_cost
          return chartMode.value === 'daily' ? day.account_cost : cumulative
        }),
        borderColor: chartMode.value === 'daily' ? '#059669' : '#0284c7',
        backgroundColor: chartMode.value === 'daily' ? '#059669' : '#0284c7',
        borderWidth: 2,
        pointRadius: days.length <= 31 ? 2 : 0,
        pointHitRadius: 10,
        pointHoverRadius: 4,
        tension: 0,
      },
    ],
  }
})
const chartOptions = computed<ChartOptions<'line'>>(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: false,
  normalized: true,
  interaction: { intersect: false, mode: 'index' },
  plugins: {
    legend: { display: false },
    tooltip: {
      callbacks: {
        label: (context) =>
          context.dataset.label + ': ' + money(Number(context.raw)),
      },
    },
  },
  scales: {
    x: {
      grid: { display: false },
      ticks: {
        color: dark.value ? '#9ca3af' : '#6b7280',
        autoSkip: true,
        maxTicksLimit: 8,
        maxRotation: 0,
      },
    },
    y: {
      beginAtZero: true,
      grid: { color: dark.value ? '#374151' : '#e5e7eb' },
      ticks: {
        color: dark.value ? '#9ca3af' : '#6b7280',
        maxTicksLimit: 5,
        callback: (value) => '$' + value,
      },
    },
  },
}))
let controller: AbortController | undefined
let generation = 0
const zone = ref(Intl.DateTimeFormat().resolvedOptions().timeZone)
function today() {
  return new Intl.DateTimeFormat('en-CA', {
    timeZone: zone.value,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(new Date())
}
function shift(date: string, days: number) {
  const d = new Date(date + 'T12:00:00Z')
  d.setUTCDate(d.getUTCDate() + days)
  return d.toISOString().slice(0, 10)
}
function choosePreset(preset: (typeof presets)[number]) {
  end.value = preset === 'yesterday' ? shift(today(), -1) : today()
  start.value =
    preset === 'week'
      ? shift(end.value, -6)
      : preset === 'month'
        ? shift(end.value, -29)
        : end.value
  void load()
}
async function load(force = false) {
  const current = ++generation
  controller?.abort()
  controller = new AbortController()
  result.value = null
  error.value = ''
  loading.value = true
  const key = selectionKey()
  const cached = cache.get(key)
  if (!force && cached && Date.now() - cached.at < 30_000) {
    result.value = cached.data
    loading.value = false
    return
  }
  try {
    const data = await getCostHistory(
      start.value,
      end.value,
      connection.value ? [Number(connection.value)] : [],
      controller.signal,
    )
    if (current !== generation) return
    result.value = data
    zone.value = data.timezone
    if (!start.value) {
      start.value = today()
      end.value = today()
    }
    cache.delete(selectionKey())
    cache.set(selectionKey(), { at: Date.now(), data })
    while (cache.size > 8) cache.delete(cache.keys().next().value!)
  } catch {
    if (current === generation) error.value = t('upstreamWorkspace.costError')
  } finally {
    if (current === generation) loading.value = false
  }
}
const money = (v: number) =>
  new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits: 4,
  }).format(v)
watch(
  () => props.show,
  (value) => {
    if (value) void load()
    else {
      generation++
      controller?.abort()
      loading.value = false
    }
  },
)
onBeforeUnmount(() => {
  generation++
  controller?.abort()
  themeObserver.disconnect()
  cache.clear()
})
</script>
