<template>
  <section>
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.upstreamConnections.usage.title') }}
        </h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.upstreamConnections.detail.observeOnly') }}
        </p>
      </div>
      <button
        v-if="error"
        type="button"
        class="rounded p-2 text-gray-500 hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700"
        :title="t('admin.upstreamConnections.usage.retry')"
        @click="emit('retry')"
      >
        <Icon name="refresh" size="sm" />
      </button>
    </div>

    <div v-if="loading" class="flex h-32 items-center justify-center">
      <LoadingSpinner />
    </div>

    <div v-else class="mt-3 space-y-5">
      <p
        v-if="error"
        class="border-y border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200"
      >
        {{ error }}
      </p>

      <template v-if="usage">
        <div class="grid grid-cols-3 divide-x divide-gray-200 border-y border-gray-200 dark:divide-dark-600 dark:border-dark-600">
          <div class="min-w-0 px-3 py-3">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.upstreamConnections.usage.accountCost') }}</p>
            <p data-testid="usage-summary-cost" class="mt-1 truncate text-base font-semibold tabular-nums text-emerald-700 dark:text-emerald-300">
              ${{ formatCost(usage.summary.account_cost) }}
            </p>
          </div>
          <div class="min-w-0 px-3 py-3">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.upstreamConnections.usage.requests') }}</p>
            <p class="mt-1 truncate text-base font-semibold tabular-nums text-gray-900 dark:text-white">
              {{ formatNumber(usage.summary.requests) }}
            </p>
          </div>
          <div class="min-w-0 px-3 py-3">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.upstreamConnections.usage.tokens') }}</p>
            <p class="mt-1 truncate text-base font-semibold tabular-nums text-gray-900 dark:text-white">
              {{ formatNumber(usage.summary.tokens) }}
            </p>
          </div>
        </div>

        <div v-if="usage.accounts.length > 0">
          <div class="mb-2 flex flex-wrap items-center justify-between gap-3">
            <h4 class="text-xs font-semibold text-gray-700 dark:text-gray-200">
              {{ t('admin.upstreamConnections.usage.hourlyTrend') }}
            </h4>
            <div class="inline-flex rounded-md border border-gray-200 bg-gray-50 p-0.5 dark:border-dark-600 dark:bg-dark-700">
              <button
                v-for="option in metricOptions"
                :key="option.value"
                type="button"
                class="rounded px-3 py-1 text-xs font-medium transition-colors"
                :class="metric === option.value
                  ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white'
                  : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-100'"
                @click="metric = option.value"
              >
                {{ option.label }}
              </button>
            </div>
          </div>
          <div class="h-64 w-full">
            <Line :data="chartData" :options="chartOptions" />
          </div>
        </div>
      </template>

      <div class="overflow-x-auto border-y border-gray-200 dark:border-dark-600">
        <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
          <thead class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-700">
            <tr>
              <th class="px-3 py-2">{{ t('admin.upstreamConnections.detail.accountId') }}</th>
              <th class="px-3 py-2">{{ t('admin.upstreamConnections.detail.token') }}</th>
              <th class="px-3 py-2">{{ t('admin.upstreamConnections.detail.groupName') }}</th>
              <th class="px-3 py-2 text-right">{{ t('admin.upstreamConnections.detail.multiplier') }}</th>
              <th class="px-3 py-2 text-right">{{ t('admin.upstreamConnections.usage.accountCost') }}</th>
              <th class="px-3 py-2 text-right">{{ t('admin.upstreamConnections.usage.requests') }}</th>
              <th class="px-3 py-2 text-right">{{ t('admin.upstreamConnections.usage.tokens') }}</th>
              <th class="px-3 py-2">{{ t('admin.upstreamConnections.detail.status') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="row in tableRows" :key="row.binding_id">
              <td class="px-3 py-2">
                <span class="block font-medium text-gray-800 dark:text-gray-100">{{ row.account_name || `#${row.account_id}` }}</span>
                <span v-if="row.account_name" class="text-xs text-gray-500">#{{ row.account_id }}</span>
              </td>
              <td class="max-w-[220px] truncate px-3 py-2" :title="remoteKeyLabel(row)">{{ remoteKeyLabel(row) }}</td>
              <td class="px-3 py-2">{{ row.remote_group_name || row.resolution_kind || '-' }}</td>
              <td class="px-3 py-2 text-right tabular-nums">{{ row.observed_multiplier === null ? '-' : `${row.observed_multiplier}x` }}</td>
              <td class="px-3 py-2 text-right tabular-nums">{{ row.stats ? `$${formatCost(row.stats.account_cost)}` : '-' }}</td>
              <td class="px-3 py-2 text-right tabular-nums">{{ row.stats ? formatNumber(row.stats.requests) : '-' }}</td>
              <td class="px-3 py-2 text-right tabular-nums">{{ row.stats ? formatNumber(row.stats.tokens) : '-' }}</td>
              <td class="px-3 py-2">
                <span class="badge" :class="row.status === 'ready' ? 'badge-success' : 'badge-warning'">
                  {{ statusLabel(row.status) }}
                </span>
              </td>
            </tr>
            <tr v-if="tableRows.length === 0">
              <td colspan="8" class="px-3 py-6 text-center text-gray-500">
                {{ t('admin.upstreamConnections.detail.noBindings') }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend
} from 'chart.js'
import { Line } from 'vue-chartjs'
import type {
  UpstreamAccountBinding,
  UpstreamConnectionAccountUsage,
  UpstreamConnectionTodayUsage
} from '@/api/admin/upstreamConnections'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend)

type UsageMetric = 'account_cost' | 'requests' | 'tokens'
type UsageTableRow = Omit<UpstreamConnectionAccountUsage, 'stats'> & {
  stats?: UpstreamConnectionAccountUsage['stats']
}

const props = defineProps<{
  usage: UpstreamConnectionTodayUsage | null
  bindings: UpstreamAccountBinding[]
  loading: boolean
  error: string
}>()

const emit = defineEmits<{
  (event: 'retry'): void
}>()

const { t } = useI18n()
const metric = ref<UsageMetric>('account_cost')
const colors = ['#0284c7', '#059669', '#d97706', '#e11d48', '#7c3aed', '#0891b2', '#4d7c0f', '#c2410c']

const metricOptions = computed<Array<{ value: UsageMetric; label: string }>>(() => [
  { value: 'account_cost', label: t('admin.upstreamConnections.usage.accountCost') },
  { value: 'requests', label: t('admin.upstreamConnections.usage.requests') },
  { value: 'tokens', label: t('admin.upstreamConnections.usage.tokens') }
])

const tableRows = computed<UsageTableRow[]>(() => {
  if (props.usage) return props.usage.accounts
  return props.bindings.map(binding => ({
    binding_id: binding.id,
    account_id: binding.account_id,
    account_name: '',
    remote_token_id: binding.remote_token_id,
    remote_token_name: binding.remote_token_name,
    remote_group_name: binding.remote_group_name,
    resolution_kind: binding.resolution_kind,
    observed_multiplier: binding.observed_multiplier,
    status: binding.status,
    trend: []
  }))
})

const chartData = computed(() => ({
  labels: props.usage?.trend.map(point => formatBucket(point.bucket)) ?? [],
  datasets: (props.usage?.accounts ?? []).map((account, index) => ({
    label: accountSeriesLabel(account),
    data: account.trend.map(point => point[metric.value]),
    borderColor: colors[index % colors.length],
    backgroundColor: colors[index % colors.length],
    borderWidth: 2,
    pointRadius: 1.5,
    pointHoverRadius: 4,
    tension: 0.25
  }))
}))

const chartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: { intersect: false, mode: 'index' as const },
  plugins: {
    legend: {
      position: 'top' as const,
      labels: {
        color: document.documentElement.classList.contains('dark') ? '#d1d5db' : '#374151',
        usePointStyle: true,
        pointStyle: 'circle' as const,
        boxWidth: 8,
        font: { size: 11 }
      }
    },
    tooltip: {
      callbacks: {
        label: (context: { dataset: { label?: string }; raw: unknown }) => {
          const label = context.dataset.label || ''
          const value = Number(context.raw || 0)
          return metric.value === 'account_cost'
            ? `${label}: $${formatCost(value)}`
            : `${label}: ${formatNumber(value)}`
        }
      }
    }
  },
  scales: {
    x: {
      grid: { color: document.documentElement.classList.contains('dark') ? '#374151' : '#e5e7eb' },
      ticks: { color: document.documentElement.classList.contains('dark') ? '#9ca3af' : '#6b7280', maxRotation: 0 }
    },
    y: {
      beginAtZero: true,
      grid: { color: document.documentElement.classList.contains('dark') ? '#374151' : '#e5e7eb' },
      ticks: {
        color: document.documentElement.classList.contains('dark') ? '#9ca3af' : '#6b7280',
        callback: (value: string | number) => metric.value === 'account_cost' ? `$${value}` : formatNumber(Number(value))
      }
    }
  }
}))

function accountSeriesLabel(account: UpstreamConnectionAccountUsage): string {
  const accountLabel = account.account_name || `#${account.account_id}`
  const keyLabel = account.remote_token_name || (account.remote_token_id ? `#${account.remote_token_id}` : '')
  return keyLabel ? `${accountLabel} - ${keyLabel}` : accountLabel
}

function remoteKeyLabel(row: Pick<UsageTableRow, 'remote_token_name' | 'remote_token_id'>): string {
  return row.remote_token_name || (row.remote_token_id ? `#${row.remote_token_id}` : '-')
}

function statusLabel(status: string): string {
  return t(`admin.upstreamConnections.statuses.${status}`, status)
}

function formatBucket(value: string): string {
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  try {
    return new Intl.DateTimeFormat(undefined, {
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
      timeZone: props.usage?.timezone || undefined
    }).format(parsed)
  } catch {
    return parsed.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false })
  }
}

function formatCost(value: number): string {
  return Number(value || 0).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 4 })
}

function formatNumber(value: number): string {
  return Number(value || 0).toLocaleString()
}
</script>
