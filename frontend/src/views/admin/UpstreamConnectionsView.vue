<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="relative w-full sm:w-64">
            <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input
              v-model="filters.search"
              class="input pl-10"
              :placeholder="t('admin.upstreamConnections.search')"
              @input="scheduleSearch"
            />
          </div>
          <div class="w-full sm:w-40">
            <Select v-model="filters.provider" :options="providerFilterOptions" @change="loadConnections(1)" />
          </div>
          <div class="w-full sm:w-40">
            <Select v-model="filters.status" :options="statusFilterOptions" @change="loadConnections(1)" />
          </div>
          <div class="w-full sm:w-48">
            <Select v-model="filters.sort" :options="sortOptions" @change="applySortAndPage(1)" />
          </div>
          <label class="flex h-10 items-center gap-2 border border-gray-300 bg-white px-3 text-sm text-gray-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300">
            <span class="whitespace-nowrap">{{ t('admin.upstreamConnections.walletHighlightThreshold') }}</span>
            <input
              v-model.number="walletHighlightThreshold"
              class="w-16 bg-transparent text-right tabular-nums outline-none"
              type="number"
              min="0"
              max="1000000"
              step="1"
              inputmode="decimal"
              :aria-label="t('admin.upstreamConnections.walletHighlightThreshold')"
            />
            <span class="text-xs text-gray-400">USD</span>
          </label>
          <div class="flex flex-1 items-center justify-end gap-2">
            <button class="btn btn-secondary" :title="t('common.refresh')" :disabled="loading" @click="loadConnections()">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button class="btn btn-primary" @click="openCreate">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.upstreamConnections.create') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <div class="mb-3 grid grid-cols-2 divide-x divide-gray-200 border-y border-gray-200 bg-white lg:grid-cols-4 dark:divide-dark-700 dark:border-dark-700 dark:bg-dark-800">
          <div class="px-4 py-3">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.upstreamConnections.summary.connections') }}</p>
            <p class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white">{{ connectionSummary.total }}</p>
          </div>
          <div class="px-4 py-3">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.upstreamConnections.lowBalanceSummary', { threshold: walletHighlightThresholdValue }) }}</p>
            <p class="mt-1 text-lg font-semibold tabular-nums" :class="connectionSummary.lowBalance > 0 ? 'text-amber-600 dark:text-amber-300' : 'text-gray-900 dark:text-white'">{{ connectionSummary.lowBalance }}</p>
          </div>
          <div class="px-4 py-3">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.upstreamConnections.summary.todayAccountCost') }}</p>
            <p data-testid="today-cost-summary" class="mt-1 text-lg font-semibold tabular-nums text-emerald-700 dark:text-emerald-300">{{ todayStatsAvailable ? `$${formatCost(connectionSummary.todayCost)}` : '-' }}</p>
          </div>
          <div class="px-4 py-3">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.upstreamConnections.summary.todayRequests') }}</p>
            <p data-testid="today-requests-summary" class="mt-1 text-lg font-semibold tabular-nums text-sky-600 dark:text-sky-300">{{ todayStatsAvailable ? connectionSummary.todayRequests.toLocaleString() : '-' }}</p>
          </div>
        </div>
        <DataTable
          :key="filters.sort"
          :columns="columns"
          :data="connections"
          :loading="loading"
          server-side-sort
          :default-sort-key="tableSort.key"
          :default-sort-order="tableSort.order"
          @sort="handleTableSort"
        >
          <template #cell-name="{ row }">
            <button class="text-left" @click="openDetails(row)">
              <span class="block font-medium text-gray-900 hover:text-primary-600 dark:text-white dark:hover:text-primary-400">
                {{ row.name }}
              </span>
              <span class="block max-w-[280px] truncate text-xs text-gray-500 dark:text-gray-400">
                {{ row.management_base_url }}
              </span>
            </button>
          </template>
          <template #cell-provider="{ row }">
            <div class="flex flex-col gap-1">
              <span class="badge badge-gray w-fit">{{ providerLabel(row.provider) }}</span>
              <span class="text-xs text-gray-500">{{ authModeLabel(row.auth_mode) }}</span>
            </div>
          </template>
          <template #cell-wallet="{ row }">
            <div class="flex flex-col" :class="isLowWallet(row) ? 'border-l-2 border-amber-500 pl-2' : ''">
              <span class="font-medium" :class="isLowWallet(row) ? 'text-amber-700 dark:text-amber-300' : 'text-gray-800 dark:text-gray-100'">{{ formatWallet(row) }}</span>
              <span class="text-xs" :class="isLowWallet(row) ? 'text-amber-600 dark:text-amber-400' : 'text-gray-500'">
                {{ isLowWallet(row) ? t('admin.upstreamConnections.walletHighlightHint', { threshold: walletHighlightThresholdValue }) : reliabilityLabel(row.wallet_reliability) }}
              </span>
            </div>
          </template>
          <template #cell-today_requests="{ row }">
            <div class="flex flex-col">
              <span class="font-medium tabular-nums text-emerald-700 dark:text-emerald-300">
                {{ row.today_cost === null ? '-' : `$${formatCost(row.today_cost)}` }}
              </span>
              <span class="text-xs tabular-nums text-gray-500 dark:text-gray-400">
                {{ row.today_requests === null ? '-' : t('admin.upstreamConnections.usage.requestCount', { count: row.today_requests.toLocaleString() }) }}
              </span>
            </div>
          </template>
          <template #cell-observations="{ row }">
            <div class="flex flex-col gap-1 text-xs text-gray-600 dark:text-gray-300">
              <span>{{ t('admin.upstreamConnections.groupsCount', { count: row.group_count }) }}</span>
              <span>{{ t('admin.upstreamConnections.bindingsCount', { count: row.binding_count }) }}</span>
            </div>
          </template>
          <template #cell-last_synced_at="{ value }">
            <span class="text-xs text-gray-600 dark:text-gray-300">{{ value ? formatDateTime(value) : '-' }}</span>
          </template>
          <template #cell-status="{ row }">
            <span class="badge" :class="statusClass(row.status)">{{ statusLabel(row.status) }}</span>
            <p v-if="row.last_error" class="mt-1 max-w-[260px] truncate text-xs text-amber-700 dark:text-amber-300" :title="row.last_error">
              {{ row.last_error }}
            </p>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <a
                class="rounded p-1.5 text-gray-500 hover:bg-sky-50 hover:text-sky-600 dark:hover:bg-sky-900/20"
                :href="homepageUrl(row.management_base_url)"
                target="_blank"
                rel="noopener noreferrer"
                :title="t('admin.upstreamConnections.openHomepage')"
                :aria-label="t('admin.upstreamConnections.openHomepage')"
                @click.stop
              >
                <Icon name="globe" size="sm" />
              </a>
              <button class="rounded p-1.5 text-gray-500 hover:bg-emerald-50 hover:text-emerald-600 dark:hover:bg-emerald-900/20" :title="t('admin.upstreamConnections.probe')" :disabled="probingIds.has(row.id)" @click="probeConnection(row)">
                <Icon name="refresh" size="sm" :class="probingIds.has(row.id) ? 'animate-spin' : ''" />
              </button>
              <button class="rounded p-1.5 text-gray-500 hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700" :title="t('common.view')" @click="openDetails(row)">
                <Icon name="eye" size="sm" />
              </button>
              <button class="rounded p-1.5 text-gray-500 hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700" :title="t('common.edit')" @click="openEdit(row)">
                <Icon name="edit" size="sm" />
              </button>
              <button class="rounded p-1.5 text-gray-500 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20" :title="t('common.delete')" @click="deleting = row">
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="applySortAndPage"
          @update:pageSize="changePageSize"
        />
      </template>
    </TablePageLayout>

    <BaseDialog :show="showForm" :title="editing ? t('admin.upstreamConnections.edit') : t('admin.upstreamConnections.create')" width="wide" @close="closeForm">
      <form id="upstream-connection-form" class="space-y-5" @submit.prevent="saveConnection">
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.upstreamConnections.fields.name') }}</label>
            <input v-model.trim="form.name" data-testid="upstream-name" class="input" required maxlength="100" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.upstreamConnections.fields.provider') }}</label>
            <Select v-model="form.provider" data-testid="upstream-provider-select" :options="providerOptions" />
          </div>
        </div>

        <div>
          <label class="input-label">{{ t('admin.upstreamConnections.fields.managementBaseUrl') }}</label>
          <input v-model.trim="form.management_base_url" data-testid="upstream-management-url" type="url" class="input" required placeholder="https://console.example.com" />
          <p class="input-hint">{{ t('admin.upstreamConnections.fields.managementBaseUrlHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.upstreamConnections.fields.forwardingBaseUrl') }}</label>
          <input v-model.trim="form.forwarding_base_url" type="url" class="input" placeholder="https://api.example.com" />
        </div>

        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.upstreamConnections.fields.authMode') }}</label>
            <Select v-model="form.auth_mode" data-testid="upstream-auth-mode-select" :options="authModeOptions" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.upstreamConnections.fields.proxy') }}</label>
            <select v-model="form.proxy_id" class="input">
              <option :value="null">{{ t('admin.upstreamConnections.fields.noProxy') }}</option>
              <option v-for="proxy in proxies" :key="proxy.id" :value="proxy.id">{{ proxy.name }}</option>
            </select>
          </div>
        </div>

        <div v-if="form.auth_mode === 'password'" class="grid gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.upstreamConnections.fields.username') }}</label>
            <input v-model.trim="form.username" class="input" autocomplete="username" :required="!editing || form.auth_mode !== editing.auth_mode" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.upstreamConnections.fields.password') }}</label>
            <input v-model="form.password" type="password" class="input" autocomplete="new-password" :required="!editing || form.auth_mode !== editing.auth_mode" />
          </div>
          <p class="sm:col-span-2 input-hint">{{ t('admin.upstreamConnections.fields.passwordModeHint') }}</p>
          <label class="sm:col-span-2 flex items-start gap-3 border-y border-gray-200 py-3 text-sm text-gray-700 dark:border-dark-600 dark:text-gray-200">
            <input
              v-model="form.not_in_cn_confirmed"
              data-testid="upstream-not-in-cn-confirmed"
              type="checkbox"
              class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600"
            />
            <span>
              <span class="block font-medium">{{ t('admin.upstreamConnections.fields.notInCNConfirmed') }}</span>
              <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('admin.upstreamConnections.fields.notInCNConfirmedHint') }}</span>
            </span>
          </label>
        </div>
        <div v-else class="space-y-4">
          <div>
            <label class="input-label">{{ t('admin.upstreamConnections.fields.accessToken') }}</label>
            <input v-model.trim="form.access_token" data-testid="upstream-access-token" type="password" class="input" autocomplete="new-password" :required="!editing || form.auth_mode !== editing.auth_mode" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.upstreamConnections.fields.refreshToken') }}</label>
            <input v-model.trim="form.refresh_token" data-testid="upstream-refresh-token" type="password" class="input" autocomplete="new-password" />
            <p class="input-hint">{{ t('admin.upstreamConnections.fields.refreshTokenHint') }}</p>
          </div>
          <div v-if="showsRemoteUserId">
            <label class="input-label">{{ t('admin.upstreamConnections.fields.remoteUserId') }}</label>
            <input v-model.trim="form.remote_user_id" data-testid="upstream-remote-user-id" class="input" inputmode="numeric" :required="requiresRemoteUserId" />
            <p class="input-hint">{{ t('admin.upstreamConnections.fields.remoteUserIdHint') }}</p>
          </div>
        </div>

        <div class="grid gap-4 sm:grid-cols-2">
          <label class="flex items-center gap-3 text-sm text-gray-700 dark:text-gray-200">
            <input v-model="form.sync_enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600" />
            {{ t('admin.upstreamConnections.fields.syncEnabled') }}
          </label>
          <div>
            <label class="input-label">{{ t('admin.upstreamConnections.fields.syncInterval') }}</label>
            <input v-model.number="form.sync_interval_seconds" type="number" min="30" max="86400" class="input" />
          </div>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeForm">{{ t('common.cancel') }}</button>
          <button type="submit" form="upstream-connection-form" class="btn btn-primary" :disabled="saving">
            <Icon v-if="saving" name="refresh" size="sm" class="mr-2 animate-spin" />
            {{ t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog :show="Boolean(details)" :title="details?.name || ''" width="extra-wide" @close="closeDetails">
      <div v-if="details" class="space-y-6">
        <div class="grid gap-4 sm:grid-cols-3">
          <div>
            <p class="text-xs text-gray-500">{{ t('admin.upstreamConnections.detail.wallet') }}</p>
            <p class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ formatWallet(details) }}</p>
          </div>
          <div>
            <p class="text-xs text-gray-500">{{ t('admin.upstreamConnections.detail.status') }}</p>
            <span class="mt-2 badge" :class="statusClass(details.status)">{{ statusLabel(details.status) }}</span>
          </div>
          <div>
            <p class="text-xs text-gray-500">{{ t('admin.upstreamConnections.detail.lastSync') }}</p>
            <p class="mt-1 text-sm text-gray-800 dark:text-gray-100">{{ details.last_synced_at ? formatDateTime(details.last_synced_at) : '-' }}</p>
          </div>
        </div>

        <section>
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.upstreamConnections.detail.groups') }}</h3>
          <div class="mt-2 overflow-x-auto border-y border-gray-200 dark:border-dark-600">
            <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
              <thead class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-700">
                <tr><th class="px-3 py-2">{{ t('admin.upstreamConnections.detail.groupName') }}</th><th class="px-3 py-2">{{ t('admin.upstreamConnections.detail.multiplier') }}</th><th class="px-3 py-2">{{ t('admin.upstreamConnections.detail.source') }}</th></tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="group in details.groups" :key="group.id"><td class="px-3 py-2">{{ group.name }}</td><td class="px-3 py-2">{{ group.rate_multiplier === null ? t('admin.upstreamConnections.unknown') : `${group.rate_multiplier}x` }}</td><td class="px-3 py-2 text-xs text-gray-500">{{ group.source }}</td></tr>
                <tr v-if="details.groups.length === 0"><td colspan="3" class="px-3 py-6 text-center text-gray-500">{{ t('admin.upstreamConnections.detail.noGroups') }}</td></tr>
              </tbody>
            </table>
          </div>
        </section>

        <UpstreamConnectionUsagePanel
          :usage="detailsUsage"
          :bindings="details.bindings"
          :loading="detailsUsageLoading"
          :error="detailsUsageError"
          @retry="loadDetailsUsage(details.id)"
        />
      </div>
      <template #footer><div class="flex justify-end"><button class="btn btn-secondary" @click="closeDetails">{{ t('common.close') }}</button></div></template>
    </BaseDialog>

    <ConfirmDialog
      :show="Boolean(deleting)"
      :title="t('admin.upstreamConnections.delete')"
      :message="t('admin.upstreamConnections.deleteConfirm', { name: deleting?.name || '' })"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="deleting = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  CreateUpstreamConnectionRequest,
  UpstreamConnection,
  UpstreamConnectionAuthMode,
  UpstreamConnectionProvider,
  UpstreamConnectionTodayUsage,
  UpdateUpstreamConnectionRequest
} from '@/api/admin/upstreamConnections'
import type { Column } from '@/components/common/types'
import type { Proxy, WindowStats } from '@/types'
import { useAppStore } from '@/stores/app'
import { formatDateTime } from '@/utils/format'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import UpstreamConnectionUsagePanel from '@/components/admin/UpstreamConnectionUsagePanel.vue'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const saving = ref(false)
type UpstreamConnectionRow = UpstreamConnection & {
  today_requests: number | null
  today_cost: number | null
}
const allConnections = ref<UpstreamConnectionRow[]>([])
const connections = ref<UpstreamConnectionRow[]>([])
const todayStatsAvailable = ref(true)
const proxies = ref<Proxy[]>([])
const probingIds = ref(new Set<number>())
const details = ref<UpstreamConnection | null>(null)
const detailsUsage = ref<UpstreamConnectionTodayUsage | null>(null)
const detailsUsageLoading = ref(false)
const detailsUsageError = ref('')
const deleting = ref<UpstreamConnection | null>(null)
const editing = ref<UpstreamConnection | null>(null)
const showForm = ref(false)
let searchTimer: ReturnType<typeof setTimeout> | null = null
let loadGeneration = 0
let detailsGeneration = 0

const pagination = reactive({ page: 1, page_size: 20, total: 0 })
const filters = reactive({ search: '', provider: '', status: '', sort: 'today_requests_desc' })
const walletHighlightStorageKey = 'admin.upstreamConnections.walletHighlightThreshold'
const walletHighlightThreshold = ref(readWalletHighlightThreshold())
const form = reactive({
  name: '', provider: 'auto' as UpstreamConnectionProvider,
  auth_mode: 'password' as UpstreamConnectionAuthMode,
  management_base_url: '', forwarding_base_url: '', remote_user_id: '',
  username: '', password: '', access_token: '', refresh_token: '', proxy_id: null as number | null,
  not_in_cn_confirmed: false,
  sync_enabled: true, sync_interval_seconds: 300
})

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.upstreamConnections.columns.name'), sortable: true },
  { key: 'provider', label: t('admin.upstreamConnections.columns.provider') },
  { key: 'wallet', label: t('admin.upstreamConnections.columns.wallet'), sortable: true },
  { key: 'today_requests', label: t('admin.upstreamConnections.columns.todayUsage'), sortable: true },
  { key: 'observations', label: t('admin.upstreamConnections.columns.observations'), sortable: true },
  { key: 'last_synced_at', label: t('admin.upstreamConnections.columns.lastSync'), sortable: true },
  { key: 'status', label: t('admin.upstreamConnections.columns.status') },
  { key: 'actions', label: t('admin.upstreamConnections.columns.actions') }
])

const providerValues: UpstreamConnectionProvider[] = ['auto', 'newapi', 'sub2api', 'rixapi', 'shellapi', 'oneapi', 'veloera', 'onehub', 'donehub']
const providerOptions = computed(() => providerValues.map(value => ({ value, label: providerLabel(value) })))
const providerFilterOptions = computed(() => [{ value: '', label: t('admin.upstreamConnections.allProviders') }, ...providerOptions.value])
const statusValues = ['pending', 'ready', 'degraded', 'auth_error', 'needs_input', 'disabled']
const statusFilterOptions = computed(() => [{ value: '', label: t('admin.upstreamConnections.allStatuses') }, ...statusValues.map(value => ({ value, label: statusLabel(value) }))])
const sortOptions = computed(() => [
  { value: 'today_requests_desc', label: t('admin.upstreamConnections.sort.todayRequestsDesc') },
  { value: 'today_requests_asc', label: t('admin.upstreamConnections.sort.todayRequestsAsc') },
  { value: 'today_cost_desc', label: t('admin.upstreamConnections.sort.todayCostDesc') },
  { value: 'today_cost_asc', label: t('admin.upstreamConnections.sort.todayCostAsc') },
  { value: 'balance_asc', label: t('admin.upstreamConnections.sort.balanceAsc') },
  { value: 'balance_desc', label: t('admin.upstreamConnections.sort.balanceDesc') },
  { value: 'group_count_desc', label: t('admin.upstreamConnections.sort.groupCountDesc') },
  { value: 'group_count_asc', label: t('admin.upstreamConnections.sort.groupCountAsc') },
  { value: 'binding_count_desc', label: t('admin.upstreamConnections.sort.bindingCountDesc') },
  { value: 'binding_count_asc', label: t('admin.upstreamConnections.sort.bindingCountAsc') },
  { value: 'last_sync_desc', label: t('admin.upstreamConnections.sort.lastSyncDesc') },
  { value: 'last_sync_asc', label: t('admin.upstreamConnections.sort.lastSyncAsc') },
  { value: 'name_asc', label: t('admin.upstreamConnections.sort.nameAsc') },
  { value: 'name_desc', label: t('admin.upstreamConnections.sort.nameDesc') }
])
const authModeOptions = computed(() => [
  { value: 'password', label: t('admin.upstreamConnections.authModes.password') },
  { value: 'access_token', label: t('admin.upstreamConnections.authModes.access_token') }
])
const showsRemoteUserId = computed(() => form.auth_mode === 'access_token' && form.provider !== 'sub2api')
const requiresRemoteUserId = computed(() => form.auth_mode === 'access_token' && ['newapi', 'rixapi', 'shellapi', 'veloera'].includes(form.provider))
const connectionSummary = computed(() => ({
  total: allConnections.value.length,
  lowBalance: allConnections.value.filter(isLowWallet).length,
  todayCost: allConnections.value.reduce((total, row) => total + (row.today_cost ?? 0), 0),
  todayRequests: allConnections.value.reduce((total, row) => total + (row.today_requests ?? 0), 0)
}))
const walletHighlightThresholdValue = computed(() => normalizeWalletHighlightThreshold(walletHighlightThreshold.value))
const tableSort = computed(() => tableSortFor(filters.sort))

function providerLabel(provider: string): string { return provider ? t(`admin.upstreamConnections.providers.${provider}`, provider) : '-' }
function statusLabel(status: string): string { return t(`admin.upstreamConnections.statuses.${status}`, status) }
function authModeLabel(mode: string): string { return t(`admin.upstreamConnections.authModes.${mode}`, mode) }
function reliabilityLabel(value: string): string { return value ? t(`admin.upstreamConnections.reliability.${value}`, value) : '-' }
function statusClass(status: string): string {
  if (status === 'ready') return 'badge-success'
  if (status === 'pending') return 'badge-gray'
  if (status === 'disabled') return 'badge-gray'
  if (status === 'auth_error') return 'badge-danger'
  return 'badge-warning'
}
function formatWallet(connection: UpstreamConnection): string {
  if (connection.wallet_unlimited) return t('admin.upstreamConnections.unlimited')
  if (connection.wallet_amount === null) return t('admin.upstreamConnections.unknown')
  const currency = connection.wallet_currency || ''
  return `${connection.wallet_amount.toLocaleString(undefined, { maximumFractionDigits: 4 })} ${currency}`.trim()
}
function formatCost(value: number): string {
  return Number(value || 0).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 4 })
}
function isLowWallet(connection: UpstreamConnection): boolean {
  return !connection.wallet_unlimited && connection.wallet_usd !== null && connection.wallet_usd < walletHighlightThresholdValue.value
}
function readWalletHighlightThreshold(): number {
  if (typeof window === 'undefined') return 50
  const stored = window.localStorage.getItem(walletHighlightStorageKey)
  return stored === null ? 50 : normalizeWalletHighlightThreshold(Number(stored))
}
function normalizeWalletHighlightThreshold(value: number): number {
  return Number.isFinite(value) && value >= 0 && value <= 1_000_000 ? value : 50
}
function tableSortFor(sort: string): { key: string; order: 'asc' | 'desc' } {
  const [field, order] = sort.split(/_(?=[^_]+$)/)
  const keyByField: Record<string, string> = {
    today_requests: 'today_requests',
    balance: 'wallet',
    group_count: 'observations',
    last_sync: 'last_synced_at',
    name: 'name'
  }
  return { key: keyByField[field] || 'today_requests', order: order === 'asc' ? 'asc' : 'desc' }
}
function handleTableSort(key: string, order: 'asc' | 'desc'): void {
  const fieldByKey: Record<string, string> = {
    name: 'name',
    wallet: 'balance',
    today_requests: 'today_requests',
    observations: 'group_count',
    last_synced_at: 'last_sync'
  }
  const field = fieldByKey[key]
  if (!field) return
  filters.sort = `${field}_${order}`
  applySortAndPage(1)
}
function compareNullableNumber(left: number | null, right: number | null, order: 'asc' | 'desc'): number {
  if (left === null && right === null) return 0
  if (left === null) return 1
  if (right === null) return -1
  return order === 'asc' ? left - right : right - left
}
function homepageUrl(value: string): string {
  try {
    return new URL(value).origin
  } catch {
    return value
  }
}
function errorMessage(error: unknown, fallback: string): string {
  const response = (error as { response?: { data?: { message?: string; detail?: string } }; message?: string })
  return response.response?.data?.message || response.response?.data?.detail || response.message || fallback
}

function setProbing(connectionId: number, probing: boolean): void {
  // Reassign Set so Vue reliably re-renders row refresh spinners.
  const next = new Set(probingIds.value)
  if (probing) next.add(connectionId)
  else next.delete(connectionId)
  probingIds.value = next
}

/** Merge a probe/get payload into the current table rows without wiping today usage stats. */
function patchConnectionRow(updated: UpstreamConnection): void {
  const merge = (row: UpstreamConnectionRow): UpstreamConnectionRow => {
    if (row.id !== updated.id) return row
    return {
      ...row,
      ...updated,
      today_requests: row.today_requests,
      today_cost: row.today_cost
    }
  }
  allConnections.value = allConnections.value.map(merge)
  // Re-sort/page so balance / last-sync ordered lists move the refreshed row immediately.
  applySortAndPage(pagination.page)
}

/** True when the local row is a newer probe/sync snapshot than the list payload. */
function isLocalConnectionFresher(local: UpstreamConnection, remote: UpstreamConnection): boolean {
  const localVersion = Number(local.version) || 0
  const remoteVersion = Number(remote.version) || 0
  if (localVersion !== remoteVersion) return localVersion > remoteVersion
  const localSync = local.last_synced_at ? Date.parse(local.last_synced_at) : NaN
  const remoteSync = remote.last_synced_at ? Date.parse(remote.last_synced_at) : NaN
  return Number.isFinite(localSync) && Number.isFinite(remoteSync) && localSync > remoteSync
}

/**
 * Apply list rows without letting a stale in-flight list wipe a just-probed local snapshot.
 * Today usage always comes from the list path (client-joined stats).
 */
function mergeListRowsWithLocal(remoteRows: UpstreamConnectionRow[]): UpstreamConnectionRow[] {
  if (allConnections.value.length === 0) return remoteRows
  const localById = new Map(allConnections.value.map(row => [row.id, row]))
  return remoteRows.map(remote => {
    const local = localById.get(remote.id)
    if (!local || !isLocalConnectionFresher(local, remote)) return remote
    return {
      ...local,
      today_requests: remote.today_requests,
      today_cost: remote.today_cost
    }
  })
}

async function loadConnections(page = pagination.page): Promise<void> {
  const generation = ++loadGeneration
  loading.value = true
  try {
    const items = await adminAPI.upstreamConnections.listAll({
      search: filters.search || undefined, provider: filters.provider || undefined, status: filters.status || undefined
    })
    const accountIds = [...new Set(items.flatMap(item => item.bound_account_ids ?? []))]
    let stats: Record<string, WindowStats> = {}
    let statsAvailable = true
    if (accountIds.length > 0) {
      try {
        stats = (await adminAPI.accounts.getBatchTodayStats(accountIds)).stats
      } catch {
        statsAvailable = false
      }
    }
    const rows: UpstreamConnectionRow[] = items.map(item => ({
      ...item,
      today_requests: statsAvailable
        ? (item.bound_account_ids ?? []).reduce((total, accountId) => total + Number(stats[String(accountId)]?.requests ?? 0), 0)
        : null,
      today_cost: statsAvailable
        ? (item.bound_account_ids ?? []).reduce((total, accountId) => total + Number(stats[String(accountId)]?.cost ?? 0), 0)
        : null
    }))
    if (generation === loadGeneration) {
      todayStatsAvailable.value = statsAvailable
      allConnections.value = mergeListRowsWithLocal(rows)
      pagination.total = rows.length
      applySortAndPage(page)
    }
  } catch (error: unknown) {
    if (generation === loadGeneration) {
      appStore.showError(errorMessage(error, t('admin.upstreamConnections.loadFailed')))
    }
  } finally {
    if (generation === loadGeneration) loading.value = false
  }
}
function scheduleSearch(): void {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => void loadConnections(1), 250)
}
function changePageSize(size: number): void { pagination.page_size = size; applySortAndPage(1) }
function applySortAndPage(page = pagination.page): void {
  const rows = [...allConnections.value]
  rows.sort((a, b) => {
    const [field, sortOrder] = filters.sort.split(/_(?=[^_]+$)/)
    const order = sortOrder === 'asc' ? 'asc' : 'desc'
    let comparison = 0
    if (field === 'balance') {
      comparison = compareNullableNumber(a.wallet_unlimited ? null : a.wallet_usd, b.wallet_unlimited ? null : b.wallet_usd, order)
    } else if (field === 'today_cost') {
      comparison = compareNullableNumber(a.today_cost, b.today_cost, order)
    } else if (field === 'today_requests') {
      comparison = compareNullableNumber(a.today_requests, b.today_requests, order)
    } else if (field === 'group_count') {
      comparison = order === 'asc' ? a.group_count - b.group_count : b.group_count - a.group_count
    } else if (field === 'binding_count') {
      comparison = order === 'asc' ? a.binding_count - b.binding_count : b.binding_count - a.binding_count
    } else if (field === 'last_sync') {
      comparison = compareNullableNumber(a.last_synced_at ? Date.parse(a.last_synced_at) : null, b.last_synced_at ? Date.parse(b.last_synced_at) : null, order)
    } else if (field === 'name') {
      comparison = order === 'asc' ? a.name.localeCompare(b.name) : b.name.localeCompare(a.name)
    }
    if (comparison !== 0) return comparison
    return b.id - a.id
  })
  const maxPage = Math.max(1, Math.ceil(rows.length / pagination.page_size))
  pagination.page = Math.min(Math.max(1, page), maxPage)
  const start = (pagination.page - 1) * pagination.page_size
  connections.value = rows.slice(start, start + pagination.page_size)
}

watch(walletHighlightThreshold, value => {
  const normalized = normalizeWalletHighlightThreshold(value)
  if (value !== normalized) walletHighlightThreshold.value = normalized
  if (typeof window !== 'undefined') window.localStorage.setItem(walletHighlightStorageKey, String(normalized))
})
function resetForm(): void {
  Object.assign(form, { name: '', provider: 'auto', auth_mode: 'password', management_base_url: '', forwarding_base_url: '', remote_user_id: '', username: '', password: '', access_token: '', refresh_token: '', proxy_id: null, not_in_cn_confirmed: false, sync_enabled: true, sync_interval_seconds: 300 })
}
function openCreate(): void { editing.value = null; resetForm(); showForm.value = true }
function openEdit(connection: UpstreamConnection): void {
  editing.value = connection
  Object.assign(form, { name: connection.name, provider: connection.provider, auth_mode: connection.auth_mode, management_base_url: connection.management_base_url, forwarding_base_url: connection.forwarding_base_url, remote_user_id: connection.remote_user_id, username: '', password: '', access_token: '', refresh_token: '', proxy_id: connection.proxy_id, not_in_cn_confirmed: connection.not_in_cn_confirmed ?? false, sync_enabled: connection.sync_enabled, sync_interval_seconds: connection.sync_interval_seconds })
  showForm.value = true
}
function closeForm(): void { showForm.value = false; editing.value = null }

function credentialPayload(): CreateUpstreamConnectionRequest['credential'] | undefined {
  if (form.auth_mode === 'password') {
    if (!form.username && !form.password) return undefined
    return { username: form.username, password: form.password }
  }
  if (!form.access_token && !form.refresh_token) return undefined
  return { access_token: form.access_token, refresh_token: form.refresh_token || undefined }
}
async function saveConnection(): Promise<void> {
  const credential = credentialPayload()
  if ((!editing.value || form.auth_mode !== editing.value.auth_mode) && !credential) {
    appStore.showError(t('admin.upstreamConnections.credentialsRequired'))
    return
  }
  if (form.auth_mode === 'password' && credential && (!form.username || !form.password)) {
    appStore.showError(t('admin.upstreamConnections.credentialsRequired'))
    return
  }
  if (form.auth_mode === 'access_token' && credential && !form.access_token) {
    appStore.showError(t('admin.upstreamConnections.credentialsRequired'))
    return
  }
  if (requiresRemoteUserId.value && !/^\d+$/.test(form.remote_user_id)) {
    appStore.showError(t('admin.upstreamConnections.remoteUserIdRequired'))
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      const payload: UpdateUpstreamConnectionRequest = {
        expected_version: editing.value.version,
        name: form.name, provider: form.provider, auth_mode: form.auth_mode,
        management_base_url: form.management_base_url, forwarding_base_url: form.forwarding_base_url,
        remote_user_id: form.remote_user_id, sync_enabled: form.sync_enabled,
        sync_interval_seconds: form.sync_interval_seconds,
        ...(form.auth_mode === 'password' && form.not_in_cn_confirmed !== (editing.value.not_in_cn_confirmed ?? false)
          ? { not_in_cn_confirmed: form.not_in_cn_confirmed }
          : {}),
        ...(form.proxy_id ? { proxy_id: form.proxy_id } : { clear_proxy: true })
      }
      if (credential) payload.credential = credential
      await adminAPI.upstreamConnections.update(editing.value.id, payload)
      appStore.showSuccess(t('admin.upstreamConnections.updated'))
    } else {
      const payload: CreateUpstreamConnectionRequest = {
        name: form.name, provider: form.provider, auth_mode: form.auth_mode,
        management_base_url: form.management_base_url, forwarding_base_url: form.forwarding_base_url || undefined,
        credential: credential!, remote_user_id: form.remote_user_id || undefined,
        ...(form.auth_mode === 'password' ? { not_in_cn_confirmed: form.not_in_cn_confirmed } : {}),
        proxy_id: form.proxy_id, sync_enabled: form.sync_enabled, sync_interval_seconds: form.sync_interval_seconds
      }
      const created = await adminAPI.upstreamConnections.create(payload)
      appStore.showSuccess(t('admin.upstreamConnections.created'))
      try { await adminAPI.upstreamConnections.probe(created.id) } catch { appStore.showError(t('admin.upstreamConnections.createdProbeFailed')) }
    }
    closeForm()
    await loadConnections(1)
  } catch (error: unknown) {
    appStore.showError(errorMessage(error, t('admin.upstreamConnections.saveFailed')))
  } finally {
    saving.value = false
  }
}
async function probeConnection(connection: UpstreamConnection): Promise<void> {
  if (probingIds.value.has(connection.id)) return
  setProbing(connection.id, true)
  try {
    const result = await adminAPI.upstreamConnections.probe(connection.id)
    // Apply probe payload immediately. Do NOT call loadConnections() here — a follow-up
    // list GET can race or return a cached/stale page and wipe the just-probed wallet/status.
    // Probe Get already returns the full connection (counts, groups, bindings, wallet).
    patchConnectionRow(result)
    appStore.showSuccess(t('admin.upstreamConnections.probeSuccess'))
    if (details.value?.id === result.id) {
      details.value = result
      await loadDetailsUsage(result.id)
    }
  } catch (error: unknown) {
    appStore.showError(errorMessage(error, t('admin.upstreamConnections.probeFailed')))
  } finally {
    setProbing(connection.id, false)
  }
}
async function openDetails(connection: UpstreamConnection): Promise<void> {
  const generation = ++detailsGeneration
  details.value = connection
  detailsUsage.value = null
  detailsUsageError.value = ''
  detailsUsageLoading.value = true
  try {
    const [connectionResult, usageResult] = await Promise.allSettled([
      adminAPI.upstreamConnections.get(connection.id),
      adminAPI.upstreamConnections.getTodayUsage(connection.id)
    ])
    if (generation !== detailsGeneration) return
    if (connectionResult.status === 'rejected') throw connectionResult.reason
    details.value = connectionResult.value
    if (usageResult.status === 'fulfilled') {
      detailsUsage.value = usageResult.value
    } else {
      detailsUsageError.value = errorMessage(usageResult.reason, t('admin.upstreamConnections.usage.loadFailed'))
    }
  } catch (error: unknown) {
    if (generation === detailsGeneration) {
      details.value = null
      appStore.showError(errorMessage(error, t('admin.upstreamConnections.loadFailed')))
    }
  } finally {
    if (generation === detailsGeneration) detailsUsageLoading.value = false
  }
}
async function loadDetailsUsage(connectionId: number): Promise<void> {
  const generation = ++detailsGeneration
  detailsUsageLoading.value = true
  detailsUsageError.value = ''
  try {
    const usage = await adminAPI.upstreamConnections.getTodayUsage(connectionId)
    if (generation === detailsGeneration && details.value?.id === connectionId) {
      detailsUsage.value = usage
    }
  } catch (error: unknown) {
    if (generation === detailsGeneration && details.value?.id === connectionId) {
      detailsUsageError.value = errorMessage(error, t('admin.upstreamConnections.usage.loadFailed'))
    }
  } finally {
    if (generation === detailsGeneration) detailsUsageLoading.value = false
  }
}
function closeDetails(): void {
  detailsGeneration++
  details.value = null
  detailsUsage.value = null
  detailsUsageError.value = ''
  detailsUsageLoading.value = false
}
async function confirmDelete(): Promise<void> {
  if (!deleting.value) return
  try {
    await adminAPI.upstreamConnections.remove(deleting.value.id)
    appStore.showSuccess(t('admin.upstreamConnections.deleted'))
    deleting.value = null
    await loadConnections(1)
  } catch (error: unknown) {
    appStore.showError(errorMessage(error, t('admin.upstreamConnections.deleteFailed')))
  }
}

onMounted(async () => {
  await Promise.all([loadConnections(1), adminAPI.proxies.getAll().then(items => { proxies.value = items }).catch(() => undefined)])
})
onBeforeUnmount(() => {
  loadGeneration++
  detailsGeneration++
  if (searchTimer) clearTimeout(searchTimer)
})
</script>
