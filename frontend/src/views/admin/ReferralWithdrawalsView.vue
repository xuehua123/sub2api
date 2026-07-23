<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="flex flex-col gap-5">
          <div class="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
            <div>
              <div class="inline-flex items-center gap-2 rounded-full bg-primary-50 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">
                <span class="h-1.5 w-1.5 rounded-full bg-primary-500" />
                {{ t('admin.referral.payoutReviewBadge', '出金审核') }}
              </div>
              <h1 class="mt-2 text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
                {{ t('admin.referral.withdrawalReviewTitle', '提现管理') }}
              </h1>
              <p class="mt-1 max-w-2xl text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.referral.withdrawalReviewDescription', '审核现金提现，并查看用户转余额记录（转余额无需审核）。') }}
              </p>
            </div>
            <button
              type="button"
              class="inline-flex h-10 items-center justify-center gap-2 rounded-xl bg-primary-600 px-4 text-sm font-semibold text-white shadow-sm transition hover:bg-primary-500 disabled:opacity-60"
              :disabled="loading"
              @click="loadAll"
            >
              <svg class="h-4 w-4" :class="loading ? 'animate-spin' : ''" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
              {{ loading ? t('common.loading', '加载中') : t('common.refresh', '刷新') }}
            </button>
          </div>

          <nav class="flex gap-1 overflow-x-auto rounded-xl border border-gray-200 bg-gray-50/80 p-1 dark:border-dark-700 dark:bg-dark-800/60" aria-label="Tabs">
            <router-link
              to="/admin/referral"
              class="whitespace-nowrap rounded-lg px-4 py-2 text-sm font-medium text-gray-600 transition hover:bg-white/80 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-900/60 dark:hover:text-gray-200"
            >
              {{ t('admin.referral.menuCommission', '数据总览') }}
            </router-link>
            <router-link
              to="/admin/referral-rewards"
              class="whitespace-nowrap rounded-lg px-4 py-2 text-sm font-medium text-gray-600 transition hover:bg-white/80 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-900/60 dark:hover:text-gray-200"
            >
              {{ t('admin.referral.menuRewards', '佣金明细') }}
            </router-link>
            <router-link
              to="/admin/referral-withdrawals"
              class="whitespace-nowrap rounded-lg bg-white px-4 py-2 text-sm font-semibold text-primary-700 shadow-sm ring-1 ring-black/5 dark:bg-dark-900 dark:text-primary-300 dark:ring-white/10"
            >
              {{ t('admin.referral.menuWithdrawals', '批量提现审核') }}
            </router-link>
          </nav>
        </div>
      </template>

      <template #filters>
        <div class="space-y-4 rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <!-- Kind chips -->
          <div>
            <p class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">{{ t('admin.referral.filterKindLabel', '类型') }}</p>
            <div class="flex flex-wrap gap-2">
              <button
                v-for="opt in kindOptions"
                :key="opt.value || 'all'"
                type="button"
                class="rounded-full px-3.5 py-1.5 text-xs font-semibold transition"
                :class="filters.kind === opt.value
                  ? 'bg-primary-600 text-white shadow-sm shadow-primary-200 dark:shadow-none'
                  : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700'"
                @click="setKind(opt.value)"
              >
                {{ opt.label }}
              </button>
            </div>
          </div>

          <!-- Status chips -->
          <div>
            <p class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">{{ t('admin.referral.filterStatusLabel', '状态') }}</p>
            <div class="flex flex-wrap gap-2">
              <button
                v-for="opt in statusOptions"
                :key="opt.value || 'all-status'"
                type="button"
                class="rounded-full px-3.5 py-1.5 text-xs font-semibold transition"
                :class="filters.status === opt.value
                  ? opt.activeClass
                  : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700'"
                @click="setStatus(opt.value)"
              >
                {{ opt.label }}
              </button>
            </div>
          </div>

          <div class="flex flex-col gap-3 sm:flex-row sm:items-center">
            <div class="relative min-w-0 flex-1">
              <svg class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-4.35-4.35M11 18a7 7 0 100-14 7 7 0 000 14z" />
              </svg>
              <input
                v-model="filters.search"
                class="input h-11 w-full rounded-xl border-gray-200 pl-10 dark:border-dark-600"
                type="search"
                :placeholder="t('admin.referral.searchPlaceholder', '搜索邮箱 / 用户名')"
                @keyup.enter="reloadFromFirstPage"
              />
            </div>
            <button
              type="button"
              class="inline-flex h-11 items-center justify-center rounded-xl bg-gray-900 px-5 text-sm font-semibold text-white transition hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-100"
              @click="reloadFromFirstPage"
            >
              {{ t('common.search', '查询') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <div v-if="loading" class="flex flex-col items-center justify-center py-24 text-sm text-gray-500">
          <LoadingSpinner class="h-10 w-10 text-primary-500" />
          <span class="mt-4">{{ t('common.loading', '加载中') }}</span>
        </div>

        <div v-else class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <div class="overflow-x-auto">
            <table class="w-full min-w-[960px] text-left text-sm">
              <thead>
                <tr class="border-b border-gray-100 bg-gray-50/90 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:border-dark-700 dark:bg-dark-800/60 dark:text-gray-400">
                  <th class="px-4 py-3.5">{{ t('admin.referral.withdrawalNo', '提现单号') }}</th>
                  <th class="px-4 py-3.5">{{ t('admin.referral.withdrawalUser', '用户') }}</th>
                  <th class="px-4 py-3.5 text-right">{{ t('admin.referral.withdrawalAmount', '申请') }}</th>
                  <th class="px-4 py-3.5 text-right">{{ t('admin.referral.withdrawalFee', '手续费') }}</th>
                  <th class="px-4 py-3.5 text-right">{{ t('admin.referral.withdrawalNet', '实付/到账') }}</th>
                  <th class="px-4 py-3.5">{{ t('admin.referral.withdrawalMethod', '方式') }}</th>
                  <th class="px-4 py-3.5">{{ t('common.status', '状态') }}</th>
                  <th class="px-4 py-3.5">{{ t('common.createdAt', '时间') }}</th>
                  <th class="px-4 py-3.5 text-right">{{ t('common.actions', '操作') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                <tr
                  v-for="record in withdrawals.items"
                  :key="record.id"
                  class="transition hover:bg-primary-50/30 dark:hover:bg-primary-950/10"
                >
                  <td class="px-4 py-4">
                    <span class="rounded-md bg-gray-100 px-2 py-1 font-mono text-xs text-gray-600 dark:bg-dark-800 dark:text-gray-300">
                      {{ record.withdrawal_no }}
                    </span>
                  </td>
                  <td class="px-4 py-4">
                    <div class="font-semibold text-gray-900 dark:text-white">{{ record.username || '-' }}</div>
                    <div class="mt-0.5 text-xs text-gray-500">{{ record.user_email }}</div>
                  </td>
                  <td class="px-4 py-4 text-right font-mono tabular-nums text-gray-700 dark:text-gray-200">
                    {{ formatCurrency(record.amount) }}
                  </td>
                  <td class="px-4 py-4 text-right font-mono tabular-nums text-red-500/90">
                    {{ formatCurrency(record.fee_amount) }}
                  </td>
                  <td class="px-4 py-4 text-right font-mono text-base font-bold tabular-nums text-gray-900 dark:text-white">
                    {{ formatCurrency(record.net_amount) }}
                  </td>
                  <td class="px-4 py-4">
                    <span
                      class="inline-flex items-center rounded-full px-2.5 py-1 text-xs font-semibold"
                      :class="methodPillClass(record.payout_method)"
                    >
                      {{ formatPayoutMethod(record.payout_method) }}
                    </span>
                  </td>
                  <td class="px-4 py-4">
                    <span
                      class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-semibold"
                      :class="withdrawalStatusBadgeClass(record.status, record.payout_method)"
                    >
                      <span class="h-1.5 w-1.5 rounded-full" :class="withdrawalStatusDotClass(record.status, record.payout_method)" />
                      {{ formatWithdrawalStatus(record.status, record.payout_method) }}
                    </span>
                  </td>
                  <td class="px-4 py-4 text-xs text-gray-500">
                    {{ formatDate(record.created_at) }}
                  </td>
                  <td class="px-4 py-4 text-right">
                    <button
                      type="button"
                      class="inline-flex items-center gap-1.5 rounded-xl px-3 py-1.5 text-xs font-semibold transition"
                      :class="isCreditConversion(record.payout_method)
                        ? 'bg-indigo-50 text-indigo-700 hover:bg-indigo-100 dark:bg-indigo-900/30 dark:text-indigo-300'
                        : 'bg-primary-600 text-white shadow-sm hover:bg-primary-500'"
                      @click="openWithdrawalDrawer(record)"
                    >
                      {{ isCreditConversion(record.payout_method) ? t('common.view', '查看') : t('admin.referral.viewDetails', '审核') }}
                    </button>
                  </td>
                </tr>
                <tr v-if="!withdrawals.items.length">
                  <td colspan="9" class="px-4 py-20 text-center">
                    <div class="mx-auto flex max-w-sm flex-col items-center">
                      <div class="mb-3 flex h-14 w-14 items-center justify-center rounded-2xl bg-gray-50 text-gray-300 dark:bg-dark-800">
                        <svg class="h-7 w-7" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                        </svg>
                      </div>
                      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('common.noData', '暂无数据') }}</h3>
                      <p class="mt-1 text-xs text-gray-500">{{ t('admin.referral.noWithdrawalText', '当前没有匹配的记录') }}</p>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
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
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useAppStore } from '@/stores'
import referralAdminAPI from '@/api/admin/referral'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import WithdrawalAuditDrawer from './referral-components/WithdrawalAuditDrawer.vue'
import { formatDateTime } from '@/utils/format'
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

const kindOptions = computed(() => [
  { value: '', label: t('admin.referral.filterKindAll', '全部类型') },
  { value: 'cash', label: t('admin.referral.filterKindCash', '现金提现') },
  { value: 'credit', label: t('admin.referral.filterKindCredit', '转平台余额') }
])

const statusOptions = computed(() => [
  { value: '', label: t('admin.referral.filterStatusAll', '全部状态'), activeClass: 'bg-gray-900 text-white dark:bg-white dark:text-gray-900' },
  { value: 'pending_review', label: t('admin.referral.filterStatusPending', '待审核'), activeClass: 'bg-amber-500 text-white' },
  { value: 'approved', label: t('admin.referral.filterStatusApproved', '待打款'), activeClass: 'bg-blue-600 text-white' },
  { value: 'paid', label: t('admin.referral.filterStatusPaid', '已完成'), activeClass: 'bg-emerald-600 text-white' },
  { value: 'rejected', label: t('admin.referral.filterStatusRejected', '已驳回'), activeClass: 'bg-red-600 text-white' }
])

const allowedKinds = new Set(['', 'cash', 'credit'])
const allowedStatuses = new Set(['', 'pending_review', 'approved', 'paid', 'rejected'])
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

function setKind(value: string) {
  filters.kind = value
  reloadFromFirstPage()
}

function setStatus(value: string) {
  filters.status = value
  reloadFromFirstPage()
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
  return formatDateTime(value) || new Date(value).toLocaleString()
}

function methodPillClass(method?: string | null): string {
  if (isCreditConversion(method)) {
    return 'bg-indigo-50 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300'
  }
  const map: Record<string, string> = {
    alipay: 'bg-sky-50 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300',
    wechat: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
    bank: 'bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-300'
  }
  return map[String(method || '')] || 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
}

onMounted(() => {
  seedFiltersFromRoute()
  void loadAll()
})

watch(
  () => route.query,
  () => {
    seedFiltersFromRoute()
    currentPage.value = 1
    void loadAll()
  }
)
</script>
