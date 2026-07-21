<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="flex flex-col gap-4">
          <div class="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
            <div>
              <h1 class="text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
                {{ t('admin.referral.rewardsTitle', '佣金记账明细') }}
              </h1>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.referral.rewardsDescription', '邀请用户充值成功后产生的返佣记账，一行对应一笔订单返佣。') }}
              </p>
            </div>
            <button
              class="inline-flex items-center justify-center rounded-xl bg-primary-600 px-4 py-2.5 text-sm font-semibold text-white shadow-sm transition hover:bg-primary-500 disabled:opacity-60"
              :disabled="loading"
              @click="refresh"
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
              class="whitespace-nowrap border-b-2 border-primary-500 px-1 py-3 text-sm font-medium text-primary-600 dark:text-primary-400"
            >
              {{ t('admin.referral.menuRewards', '佣金明细') }}
            </router-link>
            <router-link
              to="/admin/referral-withdrawals"
              class="whitespace-nowrap border-b-2 border-transparent px-1 py-3 text-sm font-medium text-gray-500 transition-colors hover:border-gray-300 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
            >
              {{ t('admin.referral.menuWithdrawals', '批量提现审核') }}
            </router-link>
          </nav>
        </div>
      </template>

      <template #filters>
        <div class="flex flex-wrap items-end gap-3">
          <label class="block">
            <span class="mb-1 block text-xs font-medium text-gray-500">{{ t('common.status', '状态') }}</span>
            <select v-model="filters.status" class="input h-10 min-w-[10rem]" @change="reloadFromFirstPage">
              <option value="">{{ t('common.all', '全部') }}</option>
              <option value="pending">{{ t('admin.referral.rewardStatus.pending', '待生效') }}</option>
              <option value="available">{{ t('admin.referral.rewardStatus.available', '可提现') }}</option>
              <option value="frozen">{{ t('admin.referral.rewardStatus.frozen', '冻结中') }}</option>
              <option value="partially_frozen">{{ t('admin.referral.rewardStatus.partially_frozen', '部分冻结') }}</option>
              <option value="paid">{{ t('admin.referral.rewardStatus.paid', '已提现/转出') }}</option>
              <option value="partially_paid">{{ t('admin.referral.rewardStatus.partially_paid', '部分提现') }}</option>
              <option value="reversed">{{ t('admin.referral.rewardStatus.reversed', '已冲回') }}</option>
              <option value="partially_reversed">{{ t('admin.referral.rewardStatus.partially_reversed', '部分冲回') }}</option>
            </select>
          </label>
          <label class="block min-w-[16rem] flex-1">
            <span class="mb-1 block text-xs font-medium text-gray-500">{{ t('admin.referral.rewardsSearch', '搜索邀请人/被邀请人') }}</span>
            <input
              v-model="filters.search"
              class="input h-10 w-full"
              type="search"
              :placeholder="t('admin.referral.rewardsSearchPlaceholder', '邮箱 / 用户名')"
              @keyup.enter="reloadFromFirstPage"
            />
          </label>
          <button class="btn btn-secondary h-10" type="button" @click="reloadFromFirstPage">
            {{ t('common.search', '查询') }}
          </button>
        </div>
      </template>

      <template #table>
        <div v-if="loading" class="flex flex-col items-center justify-center py-20 text-sm text-gray-500">
          <LoadingSpinner class="h-10 w-10 text-primary-500" />
          <span class="mt-4">{{ t('common.loading', '加载中') }}</span>
        </div>

        <div v-else class="table-wrapper">
          <table class="w-full">
            <thead>
              <tr>
                <th>{{ t('common.createdAt', '记账时间') }}</th>
                <th>{{ t('admin.referral.referrerUser', '邀请人（得佣）') }}</th>
                <th>{{ t('admin.referral.inviteeUser', '被邀请人（充值）') }}</th>
                <th>{{ t('referral.orderNo', '订单号') }}</th>
                <th class="text-right">{{ t('referral.orderAmount', '订单金额') }}</th>
                <th class="text-right">{{ t('referral.commissionRate', '比例') }}</th>
                <th class="text-right">{{ t('referral.commission', '佣金') }}</th>
                <th>{{ t('admin.referral.level', '层级') }}</th>
                <th>{{ t('common.status', '状态') }}</th>
                <th>{{ t('admin.referral.availableAt', '可结算时间') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
              <tr
                v-for="row in rewards.items"
                :key="row.id"
                class="hover:bg-gray-50/50 dark:hover:bg-dark-800/30"
              >
                <td class="whitespace-nowrap text-gray-500">{{ formatDate(row.created_at) }}</td>
                <td>
                  <div class="font-medium text-gray-900 dark:text-white">{{ row.username || '-' }}</div>
                  <div class="text-xs text-gray-500">{{ row.user_email || '-' }}</div>
                </td>
                <td>
                  <div class="font-medium text-gray-900 dark:text-white">{{ row.source_username || '-' }}</div>
                  <div class="text-xs text-gray-500">{{ row.source_user_email || '-' }}</div>
                </td>
                <td class="max-w-[12rem] truncate font-mono text-xs text-gray-600 dark:text-gray-400" :title="orderId(row)">
                  {{ orderId(row) || '-' }}
                </td>
                <td class="text-right font-mono">{{ formatCurrency(row.base_amount_snapshot) }}</td>
                <td class="text-right tabular-nums">
                  {{ row.rate_snapshot != null ? ((Number(row.rate_snapshot) * 100).toFixed(1) + '%') : '-' }}
                </td>
                <td class="text-right font-mono font-medium text-emerald-600 dark:text-emerald-400">
                  {{ formatCurrency(row.reward_amount) }}
                </td>
                <td>
                  <span class="rounded bg-gray-100 px-1.5 py-0.5 text-xs dark:bg-dark-800">L{{ row.level }}</span>
                </td>
                <td>
                  <span
                    class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium"
                    :class="statusBadgeClass(row.status)"
                  >
                    {{ formatRewardStatus(row.status) }}
                  </span>
                </td>
                <td class="whitespace-nowrap text-gray-500">
                  <template v-if="row.status === 'pending' && row.available_at">{{ formatDate(row.available_at) }}</template>
                  <template v-else>-</template>
                </td>
              </tr>
              <tr v-if="!rewards.items.length">
                <td colspan="10" class="py-16 text-center">
                  <div class="flex flex-col items-center">
                    <h3 class="text-sm font-medium text-gray-900 dark:text-white">{{ t('common.noData', '暂无数据') }}</h3>
                    <p class="mt-1 text-xs text-gray-500">
                      {{ t('admin.referral.noRewardRecords', '当前没有匹配的返佣记账') }}
                    </p>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>

      <template #pagination>
        <Pagination
          v-if="rewards.total > 0"
          :total="rewards.total"
          :page="currentPage"
          :page-size="pageSize"
          @update:page="handlePageChange"
          @update:page-size="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useAppStore } from '@/stores'
import referralAdminAPI from '@/api/admin/referral'
import type { AdminCommissionReward, BasePaginationResponse } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()
const route = useRoute()
const router = useRouter()

const loading = ref(true)
const currentPage = ref(1)
const pageSize = ref(20)
const rewards = ref<BasePaginationResponse<AdminCommissionReward>>({
  items: [],
  total: 0,
  page: 1,
  page_size: 20,
  pages: 1
})
const filters = reactive({ status: '', search: '' })

const allowedStatuses = new Set([
  '',
  'pending',
  'available',
  'frozen',
  'partially_frozen',
  'paid',
  'partially_paid',
  'reversed',
  'partially_reversed'
])

let loadSeq = 0

function normalizeRouteFilters(query: typeof route.query): { status: string; search: string } {
  const status = String(query.status ?? '').trim()
  const search = String(query.search ?? '').trim()
  return {
    status: allowedStatuses.has(status) ? status : '',
    search
  }
}

function seedFiltersFromRoute(): void {
  const next = normalizeRouteFilters(route.query)
  filters.status = next.status
  filters.search = next.search
}

function buildQueryFromFilters(): Record<string, string> {
  const query: Record<string, string> = {}
  if (filters.status) query.status = filters.status
  if (filters.search.trim()) query.search = filters.search.trim()
  return query
}

function filtersMatchRoute(): boolean {
  const next = normalizeRouteFilters(route.query)
  return next.status === filters.status && next.search === filters.search.trim()
}

async function loadAll() {
  const seq = ++loadSeq
  loading.value = true
  try {
    const data = await referralAdminAPI.listCommissionRewards(currentPage.value, pageSize.value, {
      status: filters.status || undefined,
      search: filters.search.trim() || undefined
    })
    if (seq !== loadSeq) return
    rewards.value = data
  } catch (error) {
    if (seq !== loadSeq) return
    appStore.showError((error as Error).message || t('common.operationFailed', '操作失败'))
  } finally {
    if (seq === loadSeq) loading.value = false
  }
}

/** Keep address bar filters in sync before any load that depends on in-memory filters. */
function syncFiltersToRoute(): boolean {
  if (filtersMatchRoute()) return true
  void router.replace({ query: buildQueryFromFilters() })
  return false
}

function reloadFromFirstPage() {
  currentPage.value = 1
  if (syncFiltersToRoute()) {
    void loadAll()
  }
  // else: route watch seeds filters and reloads
}

/** Refresh applies current filter inputs (not only last committed URL). */
function refresh() {
  if (!syncFiltersToRoute()) {
    currentPage.value = 1
    return
  }
  void loadAll()
}

function handlePageChange(page: number) {
  currentPage.value = page
  if (!syncFiltersToRoute()) {
    currentPage.value = 1
    return
  }
  void loadAll()
}

function handlePageSizeChange(size: number) {
  pageSize.value = size
  currentPage.value = 1
  if (!syncFiltersToRoute()) return
  void loadAll()
}

function orderId(row: AdminCommissionReward): string {
  const raw = row.external_order_id as string | null | undefined
  if (raw) return raw
  if (row.recharge_order_id) return `#${row.recharge_order_id}`
  return ''
}

function formatCurrency(value?: number | null) {
  return '￥' + Number(value || 0).toFixed(2)
}

function formatDate(value?: string | Date | null) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

function formatRewardStatus(status: string): string {
  const key = `admin.referral.rewardStatus.${status}`
  const translated = t(key)
  return translated === key ? status : translated
}

function statusBadgeClass(status: string): string {
  switch (status) {
    case 'available':
      return 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'pending':
      return 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300'
    case 'frozen':
    case 'partially_frozen':
      return 'bg-sky-100 text-sky-800 dark:bg-sky-900/30 dark:text-sky-300'
    case 'paid':
    case 'partially_paid':
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
    case 'reversed':
    case 'partially_reversed':
      return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  }
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
