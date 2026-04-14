<template>
  <div class="space-y-6">
    <!-- Header Area -->
    <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between border-b border-gray-200 dark:border-dark-800 pb-5">
      <div>
        <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">
          {{ t('admin.referral.title', '返佣管理总览') }}
        </h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.referral.description', '查看全局分销账号、佣金趋势并处理提现。') }}
        </p>
      </div>

      <div class="flex flex-col sm:flex-row sm:items-center gap-3">
        <div class="w-full sm:w-[320px]">
          <ReferralAccountPicker
            :label="''"
            :placeholder="t('admin.referral.quickOpenAccountPlaceholder', '搜索账号邮箱直达工作台...')"
            :query="quickAccountQuery"
            :model-value="selectedQuickAccount"
            :options="quickAccountResults"
            :loading="quickAccountLoading"
            input-test-id="quick-account-input"
            @update:query="emit('updateQuickAccountQuery', $event)"
            @search="emit('searchQuickAccounts', $event)"
            @select="emit('openWorkspaceFromAccount', $event)"
            @clear="emit('clearQuickAccount')"
          />
        </div>
        <button class="btn btn-secondary shadow-sm" @click="emit('refresh')" :disabled="loading">
          {{ t('common.refresh', '刷新数据') }}
        </button>
      </div>
    </div>

    <!-- Menus Tabs -->
    <div class="flex">
      <nav class="flex space-x-4 border-b border-gray-200 dark:border-dark-800 w-full pb-px" aria-label="Tabs">
        <router-link to="/admin/referral" class="border-primary-500 text-primary-600 dark:text-primary-400 whitespace-nowrap border-b-2 py-4 px-1 text-sm font-medium">
          {{ t('admin.referral.menuCommission', '数据总览') }}
        </router-link>
        <router-link to="/admin/referral/withdrawals" class="border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300 whitespace-nowrap border-b-2 py-4 px-1 text-sm font-medium transition-colors">
          {{ t('admin.referral.menuWithdrawals', '批量提现审核') }}
        </router-link>
      </nav>
    </div>

    <div v-if="loading && !overview" class="flex flex-col items-center justify-center py-20">
      <LoadingSpinner class="h-6 w-6 text-primary-500 mb-2" />
      <span class="text-sm text-gray-500">{{ t('common.loading', '加载中...') }}</span>
    </div>

    <template v-else-if="overview">
      <!-- Row 1: Metrics (4 plain elegant cards) -->
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <!-- Card 1 -->
        <div class="bg-white dark:bg-dark-900 rounded-xl border border-gray-200 dark:border-dark-800 p-5 shadow-sm">
          <div class="text-sm font-medium text-gray-500 dark:text-gray-400 mb-2">{{ t('admin.referral.availableCommission', '当前可提现总量 (CNY)') }}</div>
          <div class="text-3xl font-bold text-gray-900 dark:text-white">￥{{ formatMoney(overview.available_commission) }}</div>
          <div class="mt-2 text-xs text-green-600 dark:text-green-400 font-medium">{{ t('admin.referral.fromActiveRelations', { count: overview.total_bound_users }) }}</div>
        </div>
        <!-- Card 2 -->
        <div class="bg-white dark:bg-dark-900 rounded-xl border border-gray-200 dark:border-dark-800 p-5 shadow-sm">
          <div class="text-sm font-medium text-gray-500 dark:text-gray-400 mb-2">{{ t('admin.referral.pendingCommission', '未出安全期总计') }}</div>
          <div class="text-3xl font-bold text-gray-900 dark:text-white">￥{{ formatMoney(overview.pending_commission) }}</div>
          <div class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.referral.pendingCommissionHint', '未结转的待生效佣金') }}</div>
        </div>
        <!-- Card 3 -->
        <div class="bg-white dark:bg-dark-900 rounded-xl border border-gray-200 dark:border-dark-800 p-5 shadow-sm">
          <div class="flex items-center gap-2 mb-2">
            <span class="text-sm font-medium text-gray-500 dark:text-gray-400">{{ t('admin.referral.frozenCommissionTitle', '涉案冻结资金') }}</span>
          </div>
          <div class="text-3xl font-bold text-gray-900 dark:text-white">￥{{ formatMoney(overview.frozen_commission) }}</div>
          <div class="mt-2 text-xs text-amber-600 dark:text-amber-500 font-medium">{{ t('admin.referral.pendingQueueCount', { count: overview.pending_withdrawal_count }) }}</div>
        </div>
        <!-- Card 4 -->
        <div class="bg-white dark:bg-dark-900 rounded-xl border border-gray-200 dark:border-dark-800 p-5 shadow-sm">
          <div class="text-sm font-medium text-gray-500 dark:text-gray-400 mb-2">{{ t('admin.referral.withdrawnCommission', '已清算提现金额') }}</div>
          <div class="text-3xl font-bold text-gray-900 dark:text-white">￥{{ formatMoney(overview.withdrawn_commission) }}</div>
           <div class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.referral.historicalPaidOut') }}</div>
        </div>
      </div>

      <!-- Row 2: Chart & Pending List -->
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <!-- Real Chart! -->
        <div class="lg:col-span-2 bg-white dark:bg-dark-900 rounded-xl border border-gray-200 dark:border-dark-800 p-5 shadow-sm flex flex-col">
          <div class="mb-4">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.referral.trendTitle', '近 7 天收支走势') }}</h2>
          </div>
          <div class="flex-1 min-h-[280px]">
            <Bar v-if="chartData" :data="chartData" :options="chartOptions" />
          </div>
        </div>

        <!-- Pending list -->
        <div class="bg-white dark:bg-dark-900 rounded-xl border border-gray-200 dark:border-dark-800 shadow-sm flex flex-col">
          <div class="p-5 border-b border-gray-200 dark:border-dark-800 flex justify-between items-center">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.referral.pendingWithdrawalsTitle', '快速审核通道') }}</h2>
            <router-link to="/admin/referral/withdrawals" class="text-sm text-primary-600 hover:underline">{{ t('admin.referral.viewAll') }} &rarr;</router-link>
          </div>
          <div class="p-0 overflow-y-auto max-h-[320px]">
            <div v-if="pendingWithdrawals?.items?.length" class="divide-y divide-gray-100 dark:divide-dark-800">
              <div v-for="item in pendingWithdrawals.items" :key="item.id" class="p-4 hover:bg-gray-50 dark:hover:bg-dark-800/50 transition-colors">
                <div class="flex justify-between items-center">
                  <div class="min-w-0 flex-1">
                    <p class="text-sm font-medium text-gray-900 dark:text-white truncate">{{ item.user_email }}</p>
                    <p class="text-xs text-gray-500 dark:text-gray-400 mt-1 font-mono">ID: {{ item.withdrawal_no }}</p>
                  </div>
                  <div class="text-right ml-4">
                    <p class="text-sm font-bold text-gray-900 dark:text-white">￥{{ formatMoney(item.net_amount) }}</p>
                    <button class="mt-2 text-xs font-medium text-primary-600 dark:text-primary-400 hover:text-primary-700 bg-primary-50 dark:bg-primary-900/30 px-2 py-1 rounded" @click="emit('openWorkspaceFromWithdrawal', item)">
                      {{ t('admin.referral.goToProcess') }}
                    </button>
                  </div>
                </div>
              </div>
            </div>
            <div v-else class="p-8 text-center text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.referral.noPendingRecords') }}
            </div>
          </div>
        </div>
      </div>

      <!-- Row 3: Standard Table -->
      <div class="bg-white dark:bg-dark-900 rounded-xl border border-gray-200 dark:border-dark-800 shadow-sm overflow-hidden">
        <div class="px-5 py-4 border-b border-gray-200 dark:border-dark-800">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.referral.rankingTitle', '返佣代理排行榜') }}</h2>
        </div>
        <div class="overflow-x-auto">
          <table class="min-w-full text-left text-sm align-middle">
            <thead class="bg-gray-50 dark:bg-dark-800/50">
              <tr>
                <th scope="col" class="px-5 py-3 font-medium text-gray-500 dark:text-gray-400 text-center w-16">{{ t('admin.referral.rankColumn') }}</th>
                <th scope="col" class="px-5 py-3 font-medium text-gray-500 dark:text-gray-400">{{ t('admin.referral.agentAccount') }}</th>
                <th scope="col" class="px-5 py-3 font-medium text-gray-500 dark:text-gray-400 text-center">{{ t('admin.referral.userScale') }}</th>
                <th scope="col" class="px-5 py-3 font-medium text-gray-500 dark:text-gray-400 text-right">{{ t('admin.referral.totalEarnings') }}</th>
                <th scope="col" class="px-5 py-3 font-medium text-gray-500 dark:text-gray-400 text-right">{{ t('admin.referral.availableBalance') }}</th>
                <th scope="col" class="px-5 py-3 font-medium text-gray-500 dark:text-gray-400 text-center">{{ t('admin.referral.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-200 dark:divide-dark-800">
              <tr v-for="(item, index) in overview.ranking" :key="item.user_id" class="hover:bg-gray-50 dark:hover:bg-dark-800/50 transition-colors">
                <td class="px-5 py-4 text-center">
                  <span class="inline-flex h-6 w-6 items-center justify-center rounded-full text-xs font-semibold"
                        :class="index < 3 ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'">
                    {{ index + 1 }}
                  </span>
                </td>
                <td class="px-5 py-4">
                  <div class="text-gray-900 dark:text-white font-medium">{{ item.email }}</div>
                  <div class="text-xs text-gray-500 dark:text-gray-400 mt-1">Code: {{ item.referral_code || t('admin.referral.notSet') }}</div>
                </td>
                <td class="px-5 py-4 text-center">
                  <div class="flex items-center justify-center text-xs">
                    <span class="text-gray-600 dark:text-gray-400">{{ t('admin.referral.directInvitees', '直接邀请') }} <strong class="text-gray-900 dark:text-gray-200">{{ item.direct_invitees }}</strong></span>
                  </div>
                </td>
                <td class="px-5 py-4 text-right">
                  <span class="text-gray-900 dark:text-white font-semibold">￥{{ formatMoney(item.total_commission) }}</span>
                </td>
                <td class="px-5 py-4 text-right">
                  <span class="text-primary-600 dark:text-primary-400 font-semibold">￥{{ formatMoney(item.available_commission) }}</span>
                </td>
                <td class="px-5 py-4 text-center">
                  <div class="flex justify-center space-x-2">
                    <button class="text-xs px-3 py-1.5 rounded bg-gray-100 hover:bg-gray-200 text-gray-700 dark:bg-dark-700 dark:hover:bg-dark-600 dark:text-gray-300 font-medium transition-colors" @click="emit('openTree', item)">
                      {{ t('admin.referral.structureTree') }}
                    </button>
                    <button class="text-xs px-3 py-1.5 rounded bg-primary-50 hover:bg-primary-100 text-primary-700 dark:bg-primary-900/20 dark:hover:bg-primary-900/40 dark:text-primary-400 font-medium transition-colors" @click="emit('openWorkspace', item)">
                      {{ t('admin.referral.workspace') }}
                    </button>
                  </div>
                </td>
              </tr>
              <tr v-if="!overview.ranking.length">
                <td colspan="6" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.referral.noRankingData') }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import ReferralAccountPicker from '@/components/admin/referral/ReferralAccountPicker.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type {
  AdminReferralOverview,
  AdminCommissionWithdrawal,
  AdminReferralAccountOption,
  AdminReferralRankingItem,
  BasePaginationResponse
} from '@/types'

// Setup chart.js
import {
  Chart as ChartJS,
  Title,
  Tooltip,
  Legend,
  BarElement,
  CategoryScale,
  LinearScale
} from 'chart.js'
import { Bar } from 'vue-chartjs'

ChartJS.register(CategoryScale, LinearScale, BarElement, Title, Tooltip, Legend)

const { t } = useI18n()

interface Props {
  loading: boolean
  overview: AdminReferralOverview | null
  pendingWithdrawals: BasePaginationResponse<AdminCommissionWithdrawal> | null
  quickAccountQuery: string
  quickAccountResults: AdminReferralAccountOption[]
  selectedQuickAccount: AdminReferralAccountOption | null
  quickAccountLoading: boolean
}

const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'refresh'): void
  (e: 'updateQuickAccountQuery', val: string): void
  (e: 'searchQuickAccounts', query: string): void
  (e: 'openWorkspaceFromAccount', item: AdminReferralAccountOption): void
  (e: 'clearQuickAccount'): void
  (e: 'openTree', item: any): void
  (e: 'openWorkspace', item: AdminReferralRankingItem): void
  (e: 'openWorkspaceFromWithdrawal', item: AdminCommissionWithdrawal): void
}>()

// Configure real Chart.js Data
const chartData = computed(() => {
  if (!props.overview || !props.overview.recent_trend) return null
  const trend = props.overview.recent_trend
  return {
    labels: trend.map(p => formatTrendDate(p.date)),
    datasets: [
      {
        label: t('admin.referral.rewardTrendLegend', '代理新增返佣'),
        backgroundColor: '#10b981', // emerald-500
        data: trend.map(p => p.reward_amount),
        borderRadius: 4,
        maxBarThickness: 40
      },
      {
        label: t('admin.referral.withdrawTrendLegend', '代理提现流出'),
        backgroundColor: '#6366f1', // indigo-500
        data: trend.map(p => p.withdrawal_amount),
        borderRadius: 4,
        maxBarThickness: 40
      }
    ]
  }
})

// Clean Chart.js Options for Dashboard
const chartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: {
      position: 'bottom' as const,
      labels: {
        usePointStyle: true,
        padding: 20
      }
    },
    tooltip: {
      mode: 'index' as const,
      intersect: false,
      callbacks: {
        label: function(context: any) {
          let label = context.dataset.label || '';
          if (label) {
            label += ': ';
          }
          if (context.parsed.y !== null) {
            label += '￥' + context.parsed.y.toFixed(2);
          }
          return label;
        }
      }
    }
  },
  scales: {
    y: {
      beginAtZero: true,
      border: { display: false },
      grid: {
        color: 'rgba(156, 163, 175, 0.1)',
        drawBorder: false,
      }
    },
    x: {
      border: { display: false },
      grid: {
        display: false
      }
    }
  },
  interaction: {
    mode: 'index' as const,
    intersect: false,
  }
}

function formatMoney(value: number | undefined) {
  return Number(value || 0).toFixed(2)
}

function formatTrendDate(value: string) {
  const parsed = new Date(`${value}T00:00:00`)
  return `${parsed.getMonth() + 1}/${parsed.getDate()}`
}
</script>
