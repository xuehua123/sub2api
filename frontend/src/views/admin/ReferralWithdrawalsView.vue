<template>
  <AppLayout>
    <TablePageLayout>
      <!-- Header + Actions -->
      <template #actions>
        <div class="flex flex-col gap-4">
          <div class="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
            <div>
              <h1 class="text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
                {{ t('admin.referral.withdrawalReviewTitle', '提现管理') }}
              </h1>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.referral.withdrawalReviewDescription', '审核现金提现，并查看用户转余额记录（转余额无需审核）。') }}
              </p>
            </div>
            <button
              class="inline-flex items-center justify-center rounded-xl bg-primary-600 px-4 py-2.5 text-sm font-semibold text-white shadow-sm transition hover:bg-primary-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-600 dark:bg-primary-500 dark:hover:bg-primary-400"
              :disabled="loading"
              @click="loadAll"
            >
              {{ loading ? t('common.loading', '加载中') : t('common.refresh', '刷新') }}
            </button>
          </div>
          <nav class="flex space-x-4 border-b border-gray-200 dark:border-dark-800 pb-px" aria-label="Tabs">
            <router-link
              to="/admin/referral"
              class="whitespace-nowrap border-b-2 border-transparent px-1 py-3 text-sm font-medium text-gray-500 transition-colors hover:border-gray-300 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
            >
              {{ t('admin.referral.menuCommission', '数据总览') }}
            </router-link>
            <router-link
              to="/admin/referral-rewards"
              class="whitespace-nowrap border-b-2 border-transparent px-1 py-3 text-sm font-medium text-gray-500 transition-colors hover:border-gray-300 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
            >
              {{ t('admin.referral.menuRewards', '佣金明细') }}
            </router-link>
            <router-link
              to="/admin/referral-withdrawals"
              class="whitespace-nowrap border-b-2 border-primary-500 px-1 py-3 text-sm font-medium text-primary-600 dark:text-primary-400"
            >
              {{ t('admin.referral.menuWithdrawals', '批量提现审核') }}
            </router-link>
          </nav>
        </div>
      </template>

      <template #filters>
        <div class="flex flex-wrap items-end gap-3">
          <label class="block">
            <span class="mb-1 block text-xs font-medium text-gray-500">类型</span>
            <select v-model="filters.kind" class="input h-10 min-w-[9rem]" @change="reloadFromFirstPage">
              <option value="">全部类型</option>
              <option value="cash">现金提现</option>
              <option value="credit">转平台余额</option>
            </select>
          </label>
          <label class="block">
            <span class="mb-1 block text-xs font-medium text-gray-500">状态</span>
            <select v-model="filters.status" class="input h-10 min-w-[9rem]" @change="reloadFromFirstPage">
              <option value="">全部状态</option>
              <option value="pending_review">待审核</option>
              <option value="approved">已通过·待打款</option>
              <option value="paid">已完成 (含打款/转余额)</option>
              <option value="rejected">已驳回</option>
            </select>
          </label>
          <label class="block min-w-[14rem] flex-1">
            <span class="mb-1 block text-xs font-medium text-gray-500">搜索用户</span>
            <input
              v-model="filters.search"
              class="input h-10 w-full"
              type="search"
              placeholder="邮箱 / 用户名"
              @keyup.enter="reloadFromFirstPage"
            />
          </label>
          <button class="btn btn-secondary h-10" type="button" @click="reloadFromFirstPage">查询</button>
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
                <th class="text-right">{{ t('admin.referral.withdrawalNet', '实付/到账') }}</th>
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
                    :class="withdrawalStatusBadgeClass(record.status, record.payout_method)"
                  >
                    <span class="h-1.5 w-1.5 rounded-full" :class="withdrawalStatusDotClass(record.status, record.payout_method)" />
                    {{ formatWithdrawalStatus(record.status, record.payout_method) }}
                  </span>
                </td>
                <td class="text-gray-500">{{ formatDate(record.created_at) }}</td>
                <td>
                  <button
                    class="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium text-primary-600 transition hover:bg-primary-50 dark:text-primary-400 dark:hover:bg-primary-900/20"
                    @click="openWithdrawalDrawer(record)"
                  >
                    {{ isCreditConversion(record.payout_method) ? t('common.view', '查看') : t('admin.referral.viewDetails', '审核') }}
                  </button>
                </td>
              </tr>
              <tr v-if="!withdrawals.items.length">
                <td colspan="9" class="py-16 text-center">
                  <div class="flex flex-col items-center">
                    <h3 class="text-sm font-medium text-gray-900 dark:text-white">{{ t('common.noData', '暂无数据') }}</h3>
                    <p class="mt-1 text-xs text-gray-500">{{ t('admin.referral.noWithdrawalText', '当前没有匹配的记录') }}</p>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>

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
import { onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
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
import {
  formatPayoutMethod,
  formatWithdrawalStatus,
  isCreditConversion,
  withdrawalStatusBadgeClass,
  withdrawalStatusDotClass
} from '@/utils/referralWithdrawalDisplay'

const { t } = useI18n()
const appStore = useAppStore()
const route = useRoute()
const router = useRouter()

const loading = ref(true)
const currentPage = ref(1)
const pageSize = ref(20)
const withdrawals = ref<BasePaginationResponse<AdminCommissionWithdrawal>>({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
const filters = reactive({ kind: '', status: '', search: '' })

const drawerOpen = ref(false)
const drawerLoading = ref(false)
const selectedWithdrawal = ref<AdminCommissionWithdrawal | null>(null)
const withdrawalItems = ref<CommissionWithdrawalItem[]>([])

const allowedKinds = new Set(['', 'cash', 'credit'])
const allowedStatuses = new Set(['', 'pending_review', 'approved', 'paid', 'rejected'])
/** Drop stale list responses when a newer load supersedes an in-flight one. */
let loadSeq = 0

function normalizeRouteFilters(query: typeof route.query): { kind: string; status: string; search: string } {
  const kind = String(query.kind ?? '').trim()
  const status = String(query.status ?? '').trim()
  const search = String(query.search ?? '').trim()
  return {
    kind: allowedKinds.has(kind) ? kind : '',
    status: allowedStatuses.has(status) ? status : '',
    search
  }
}

function seedFiltersFromRoute(): void {
  const next = normalizeRouteFilters(route.query)
  filters.kind = next.kind
  filters.status = next.status
  filters.search = next.search
}

function buildQueryFromFilters(): Record<string, string> {
  const query: Record<string, string> = {}
  if (filters.kind) query.kind = filters.kind
  if (filters.status) query.status = filters.status
  if (filters.search.trim()) query.search = filters.search.trim()
  return query
}

function filtersMatchRoute(): boolean {
  const next = normalizeRouteFilters(route.query)
  return next.kind === filters.kind && next.status === filters.status && next.search === filters.search.trim()
}

async function loadAll() {
  const seq = ++loadSeq
  loading.value = true
  try {
    const data = await referralAdminAPI.listWithdrawals(currentPage.value, pageSize.value, {
      kind: filters.kind || undefined,
      status: filters.status || undefined,
      search: filters.search.trim() || undefined
    })
    if (seq !== loadSeq) return
    withdrawals.value = data
  } catch (error) {
    if (seq !== loadSeq) return
    appStore.showError((error as Error).message || t('common.operationFailed', '操作失败'))
  } finally {
    if (seq === loadSeq) loading.value = false
  }
}

/**
 * Single load owner for filter changes:
 * - route differs → only router.replace; watch seeds + loads once
 * - route already matches → load here (replace would be a no-op and watch may not fire)
 */
function reloadFromFirstPage() {
  currentPage.value = 1
  if (filtersMatchRoute()) {
    void loadAll()
    return
  }
  void router.replace({ query: buildQueryFromFilters() })
}

function handlePageChange(page: number) {
  currentPage.value = page
  void loadAll()
}

function handlePageSizeChange(size: number) {
  pageSize.value = size
  currentPage.value = 1
  void loadAll()
}

async function openWithdrawalDrawer(record: AdminCommissionWithdrawal) {
  selectedWithdrawal.value = record
  drawerOpen.value = true
  drawerLoading.value = true
  try {
    // Detail fetch decrypts payout account for audit; list responses stay masked-only.
    const [detail, items] = await Promise.all([
      referralAdminAPI.getWithdrawal(record.id),
      referralAdminAPI.getWithdrawalItems(record.id)
    ])
    selectedWithdrawal.value = detail
    withdrawalItems.value = items
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

function formatDate(value?: string | Date | null) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

onMounted(() => {
  seedFiltersFromRoute()
  void loadAll()
})

// External navigation (overview deep-links, browser back/forward) re-seeds and loads once.
// Local filter apply uses router.replace only when the query changes, so this is the sole load path then.
watch(
  () => route.query,
  () => {
    seedFiltersFromRoute()
    currentPage.value = 1
    void loadAll()
  }
)
</script>
