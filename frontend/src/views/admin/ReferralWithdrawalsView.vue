<template>
  <AppLayout>
    <TablePageLayout>
      <!-- Header + Actions -->
      <template #actions>
        <div class="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
          <div>
            <h1 class="text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
              {{ t('admin.referral.withdrawalReviewTitle', '提现管理') }}
            </h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.referral.withdrawalReviewDescription', '审核和处理用户的佣金提现申请') }}
            </p>
          </div>
          <div class="flex items-center gap-3">
            <router-link
              to="/admin/referral"
              class="inline-flex items-center justify-center rounded-xl bg-white px-4 py-2.5 text-sm font-semibold text-gray-700 shadow-sm ring-1 ring-inset ring-gray-300 transition hover:bg-gray-50 dark:bg-dark-800 dark:text-gray-200 dark:ring-dark-600 dark:hover:bg-dark-700"
            >
              <svg class="mr-2 h-4 w-4 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 19l-7-7m0 0l7-7m-7 7h18" />
              </svg>
              {{ t('admin.referral.menuCommission', '返回全景概览') }}
            </router-link>
            <button
              class="inline-flex items-center justify-center rounded-xl bg-primary-600 px-4 py-2.5 text-sm font-semibold text-white shadow-sm transition hover:bg-primary-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-600 dark:bg-primary-500 dark:hover:bg-primary-400"
              :disabled="loading"
              @click="loadAll"
            >
              <svg v-if="loading" class="mr-2 h-4 w-4 animate-spin text-white" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
              </svg>
              <svg v-else class="mr-2 h-4 w-4 text-primary-100" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
              {{ loading ? t('common.loading', '加载中') : t('common.refresh', '刷新') }}
            </button>
          </div>
        </div>
      </template>

      <!-- Table -->
      <template #table>
        <div v-if="loading" class="flex flex-col items-center justify-center py-20 text-sm text-gray-500">
          <LoadingSpinner class="h-10 w-10 text-primary-500" />
          <span class="mt-4">{{ t('common.loading', '加载中') }}</span>
        </div>

        <div v-else class="table-wrapper">
          <table class="w-full">
            <thead>
              <tr>
                <th>{{ t('admin.referral.withdrawalNo', '提现单号') }}</th>
                <th>{{ t('admin.referral.withdrawalUser', '用户') }}</th>
                <th class="text-right">{{ t('admin.referral.withdrawalAmount', '申请金额') }}</th>
                <th class="text-right">{{ t('admin.referral.withdrawalFee', '手续费') }}</th>
                <th class="text-right">{{ t('admin.referral.withdrawalNet', '实付金额') }}</th>
                <th>{{ t('admin.referral.withdrawalPayoutMethod', '收款方式') }}</th>
                <th>{{ t('common.status', '状态') }}</th>
                <th>{{ t('common.createdAt', '申请时间') }}</th>
                <th>{{ t('common.actions', '操作') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
              <tr v-for="record in withdrawals.items" :key="record.id" class="hover:bg-gray-50/50 dark:hover:bg-dark-800/30">
                <td class="font-mono text-xs text-gray-600 dark:text-gray-400">{{ record.withdrawal_no }}</td>
                <td>
                  <div class="font-medium text-gray-900 dark:text-white">{{ record.username || '-' }}</div>
                  <div class="text-xs text-gray-500">{{ record.user_email }}</div>
                </td>
                <td class="text-right font-mono">{{ formatCurrency(record.amount) }}</td>
                <td class="text-right font-mono text-red-500/80">{{ formatCurrency(record.fee_amount) }}</td>
                <td class="text-right font-mono font-medium text-gray-900 dark:text-white">{{ formatCurrency(record.net_amount) }}</td>
                <td>{{ formatPayoutMethod(record.payout_method) }}</td>
                <td>
                  <span
                    class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium"
                    :class="statusBadgeClass(record.status)"
                  >
                    <span class="h-1.5 w-1.5 rounded-full" :class="statusDotClass(record.status)" />
                    {{ formatStatus(record.status) }}
                  </span>
                </td>
                <td class="text-gray-500">{{ formatDate(record.created_at) }}</td>
                <td>
                  <button
                    class="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium text-primary-600 transition hover:bg-primary-50 dark:text-primary-400 dark:hover:bg-primary-900/20"
                    @click="openWithdrawalDrawer(record)"
                  >
                    <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
                    </svg>
                    {{ t('admin.referral.viewDetails', '审核') }}
                  </button>
                </td>
              </tr>
              <tr v-if="!withdrawals.items.length">
                <td colspan="9" class="py-16 text-center">
                  <div class="flex flex-col items-center">
                    <svg class="mb-3 h-10 w-10 text-gray-300 dark:text-gray-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                    </svg>
                    <h3 class="text-sm font-medium text-gray-900 dark:text-white">{{ t('common.noData', '暂无数据') }}</h3>
                    <p class="mt-1 text-xs text-gray-500">{{ t('admin.referral.noWithdrawalText', '当前没有提现申请需要处理') }}</p>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>

      <!-- Pagination -->
      <template #pagination>
        <Pagination
          v-if="withdrawals.total > 0"
          :total="withdrawals.total"
          :page="currentPage"
          :page-size="pageSize"
          @update:page="handlePageChange"
          @update:page-size="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <!-- Drawer -->
    <WithdrawalAuditDrawer
      :show="drawerOpen"
      :loading="drawerLoading"
      :withdrawal="selectedWithdrawal"
      :items="withdrawalItems"
      @close="drawerOpen = false"
      @actionSuccess="handleActionSuccess"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useAppStore } from '@/stores'
import referralAdminAPI from '@/api/admin/referral'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import WithdrawalAuditDrawer from './referral-components/WithdrawalAuditDrawer.vue'
import type {
  AdminCommissionWithdrawal,
  BasePaginationResponse,
  CommissionWithdrawalItem
} from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

// State
const loading = ref(true)
const currentPage = ref(1)
const pageSize = ref(20)
const withdrawals = ref<BasePaginationResponse<AdminCommissionWithdrawal>>({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })

// Drawer details
const drawerOpen = ref(false)
const drawerLoading = ref(false)
const selectedWithdrawal = ref<AdminCommissionWithdrawal | null>(null)
const withdrawalItems = ref<CommissionWithdrawalItem[]>([])

async function loadAll() {
  loading.value = true
  try {
    withdrawals.value = await referralAdminAPI.listWithdrawals(currentPage.value, pageSize.value)
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '操作失败'))
  } finally {
    loading.value = false
  }
}

function handlePageChange(page: number) {
  currentPage.value = page
  loadAll()
}

function handlePageSizeChange(size: number) {
  pageSize.value = size
  currentPage.value = 1
  loadAll()
}

async function openWithdrawalDrawer(record: AdminCommissionWithdrawal) {
  selectedWithdrawal.value = record
  drawerOpen.value = true
  drawerLoading.value = true
  try {
    withdrawalItems.value = await referralAdminAPI.getWithdrawalItems(record.id)
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '操作失败'))
  } finally {
    drawerLoading.value = false
  }
}

async function handleActionSuccess() {
  drawerOpen.value = false
  await loadAll()
}

function formatCurrency(value: number) {
  return '\uFFE5' + Number(value || 0).toFixed(2)
}

function formatStatus(status: string): string {
  const map: Record<string, string> = {
    pending_review: '待审核',
    approved: '已通过',
    rejected: '已驳回',
    paid: '已打款',
    frozen: '冻结中'
  }
  return map[status] || status
}

function formatPayoutMethod(method: string): string {
  const map: Record<string, string> = {
    alipay: '支付宝',
    wechat: '微信',
    bank: '银行卡'
  }
  return map[method] || method || '-'
}

function statusBadgeClass(status: string): string {
  const map: Record<string, string> = {
    pending_review: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-500/10 dark:text-yellow-400',
    approved: 'bg-blue-100 text-blue-700 dark:bg-blue-500/10 dark:text-blue-400',
    paid: 'bg-green-100 text-green-700 dark:bg-green-500/10 dark:text-green-400',
    rejected: 'bg-red-100 text-red-700 dark:bg-red-500/10 dark:text-red-400',
    frozen: 'bg-gray-100 text-gray-700 dark:bg-gray-500/10 dark:text-gray-400'
  }
  return map[status] || 'bg-gray-100 text-gray-700 dark:bg-gray-500/10 dark:text-gray-400'
}

function statusDotClass(status: string): string {
  const map: Record<string, string> = {
    pending_review: 'bg-yellow-500 animate-pulse',
    approved: 'bg-blue-500',
    paid: 'bg-green-500',
    rejected: 'bg-red-500',
    frozen: 'bg-gray-500'
  }
  return map[status] || 'bg-gray-500'
}

function formatDate(value?: string | Date | null) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

onMounted(loadAll)
</script>
