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
          <div class="flex flex-1 items-center justify-end gap-2">
            <button class="btn btn-secondary" :title="t('common.refresh')" :disabled="loading" @click="loadConnections()">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button class="btn btn-secondary" :disabled="migrationLoading" @click="openLegacyMigration">
              <Icon name="database" size="md" class="mr-2" />
              {{ t('admin.upstreamConnections.migration.open') }}
            </button>
            <button class="btn btn-primary" @click="openCreate">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.upstreamConnections.create') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="connections" :loading="loading">
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
            <div class="flex flex-col">
              <span class="font-medium text-gray-800 dark:text-gray-100">{{ formatWallet(row) }}</span>
              <span class="text-xs text-gray-500">{{ reliabilityLabel(row.wallet_reliability) }}</span>
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
          @update:page="loadConnections"
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

    <BaseDialog :show="Boolean(details)" :title="details?.name || ''" width="wide" @close="details = null">
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

        <section>
          <div class="flex items-center justify-between gap-3">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.upstreamConnections.detail.bindings') }}</h3>
            <span class="text-xs text-gray-500">{{ t('admin.upstreamConnections.detail.observeOnly') }}</span>
          </div>
          <div class="mt-2 overflow-x-auto border-y border-gray-200 dark:border-dark-600">
            <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
              <thead class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-700">
                <tr><th class="px-3 py-2">{{ t('admin.upstreamConnections.detail.accountId') }}</th><th class="px-3 py-2">{{ t('admin.upstreamConnections.detail.token') }}</th><th class="px-3 py-2">{{ t('admin.upstreamConnections.detail.groupName') }}</th><th class="px-3 py-2">{{ t('admin.upstreamConnections.detail.multiplier') }}</th><th class="px-3 py-2">{{ t('admin.upstreamConnections.detail.status') }}</th></tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="binding in details.bindings" :key="binding.id"><td class="px-3 py-2">#{{ binding.account_id }}</td><td class="px-3 py-2">{{ binding.remote_token_name || binding.remote_token_id || '-' }}</td><td class="px-3 py-2">{{ binding.remote_group_name || binding.resolution_kind }}</td><td class="px-3 py-2">{{ binding.observed_multiplier === null ? '-' : `${binding.observed_multiplier}x` }}</td><td class="px-3 py-2"><span class="badge" :class="binding.status === 'ready' ? 'badge-success' : 'badge-warning'">{{ statusLabel(binding.status) }}</span></td></tr>
                <tr v-if="details.bindings.length === 0"><td colspan="5" class="px-3 py-6 text-center text-gray-500">{{ t('admin.upstreamConnections.detail.noBindings') }}</td></tr>
              </tbody>
            </table>
          </div>
        </section>
      </div>
      <template #footer><div class="flex justify-end"><button class="btn btn-secondary" @click="details = null">{{ t('common.close') }}</button></div></template>
    </BaseDialog>

    <BaseDialog
      :show="showLegacyMigration"
      :title="t('admin.upstreamConnections.migration.title')"
      width="wide"
      @close="closeLegacyMigration"
    >
      <div v-if="migrationLoading" class="flex min-h-48 items-center justify-center text-gray-500">
        <Icon name="refresh" size="lg" class="mr-3 animate-spin" />
        {{ t('admin.upstreamConnections.migration.loading') }}
      </div>
      <div v-else-if="migrationResult" class="space-y-5">
        <div class="border-y border-gray-200 bg-gray-50 px-4 py-3 text-sm text-gray-700 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200">
          <p>{{ t('admin.upstreamConnections.migration.safety') }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.upstreamConnections.migration.compatibility') }}</p>
        </div>

        <div class="grid grid-cols-2 divide-x divide-y border-y border-gray-200 text-center sm:grid-cols-4 dark:divide-dark-600 dark:border-dark-600">
          <div class="px-3 py-3">
            <p class="text-lg font-semibold text-gray-900 dark:text-white">{{ migrationResult.summary.eligible_accounts }}</p>
            <p class="text-xs text-gray-500">{{ t('admin.upstreamConnections.migration.summary.eligible') }}</p>
          </div>
          <div class="px-3 py-3">
            <p class="text-lg font-semibold text-gray-900 dark:text-white">{{ migrationResult.summary.unique_connections }}</p>
            <p class="text-xs text-gray-500">{{ t('admin.upstreamConnections.migration.summary.connections') }}</p>
          </div>
          <div class="px-3 py-3">
            <p class="text-lg font-semibold text-gray-900 dark:text-white">{{ migrationResult.dry_run ? migrationResult.summary.planned_accounts : migrationResult.summary.migrated_accounts }}</p>
            <p class="text-xs text-gray-500">{{ t(migrationResult.dry_run ? 'admin.upstreamConnections.migration.summary.planned' : 'admin.upstreamConnections.migration.summary.migrated') }}</p>
          </div>
          <div class="px-3 py-3">
            <p class="text-lg font-semibold text-gray-900 dark:text-white">{{ migrationResult.summary.skipped_accounts + migrationResult.summary.failed_accounts }}</p>
            <p class="text-xs text-gray-500">{{ t('admin.upstreamConnections.migration.summary.skipped') }}</p>
          </div>
        </div>

        <div class="max-h-[420px] overflow-auto border-y border-gray-200 dark:border-dark-600">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
            <thead class="sticky top-0 bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-700">
              <tr>
                <th class="px-3 py-2">{{ t('admin.upstreamConnections.migration.columns.account') }}</th>
                <th class="px-3 py-2">{{ t('admin.upstreamConnections.migration.columns.upstream') }}</th>
                <th class="px-3 py-2">{{ t('admin.upstreamConnections.migration.columns.group') }}</th>
                <th class="px-3 py-2">{{ t('admin.upstreamConnections.migration.columns.action') }}</th>
                <th class="px-3 py-2">{{ t('admin.upstreamConnections.migration.columns.note') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="item in migrationResult.items" :key="item.account_id">
                <td class="px-3 py-2">
                  <span class="block font-medium text-gray-900 dark:text-white">{{ item.account_name || `#${item.account_id}` }}</span>
                  <span class="text-xs text-gray-500">#{{ item.account_id }}</span>
                </td>
                <td class="max-w-[220px] px-3 py-2">
                  <span class="block">{{ providerLabel(item.provider) }}</span>
                  <span class="block truncate text-xs text-gray-500" :title="item.management_base_url">{{ item.management_base_url || '-' }}</span>
                </td>
                <td class="px-3 py-2">{{ item.legacy_group || '-' }}</td>
                <td class="px-3 py-2"><span class="badge" :class="migrationActionClass(item.action)">{{ migrationActionLabel(item.action) }}</span></td>
                <td class="max-w-[280px] px-3 py-2 text-xs text-gray-600 dark:text-gray-300">{{ item.message }}</td>
              </tr>
              <tr v-if="migrationResult.items.length === 0">
                <td colspan="5" class="px-3 py-8 text-center text-gray-500">{{ t('admin.upstreamConnections.migration.empty') }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div v-if="migrationResult.warnings.length" class="border-l-2 border-amber-500 pl-3 text-sm text-amber-700 dark:text-amber-300">
          <p v-for="warning in migrationResult.warnings" :key="warning">{{ warning }}</p>
        </div>
      </div>
      <template #footer>
        <div class="flex w-full flex-wrap justify-end gap-3">
          <button type="button" class="btn btn-secondary" :disabled="migrationApplying" @click="closeLegacyMigration">{{ t('common.close') }}</button>
          <button
            v-if="migrationResult?.dry_run"
            type="button"
            class="btn btn-primary"
            :disabled="migrationApplying || migrationResult.summary.planned_accounts === 0"
            @click="applyLegacyMigration"
          >
            <Icon v-if="migrationApplying" name="refresh" size="sm" class="mr-2 animate-spin" />
            <Icon v-else name="database" size="sm" class="mr-2" />
            {{ t('admin.upstreamConnections.migration.apply') }}
          </button>
        </div>
      </template>
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
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  CreateUpstreamConnectionRequest,
  UpstreamConnection,
  UpstreamConnectionAuthMode,
  UpstreamLegacyMigrationResult,
  UpstreamConnectionProvider,
  UpdateUpstreamConnectionRequest
} from '@/api/admin/upstreamConnections'
import type { Column } from '@/components/common/types'
import type { Proxy } from '@/types'
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

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const saving = ref(false)
const connections = ref<UpstreamConnection[]>([])
const proxies = ref<Proxy[]>([])
const probingIds = ref(new Set<number>())
const details = ref<UpstreamConnection | null>(null)
const deleting = ref<UpstreamConnection | null>(null)
const editing = ref<UpstreamConnection | null>(null)
const showForm = ref(false)
const showLegacyMigration = ref(false)
const migrationLoading = ref(false)
const migrationApplying = ref(false)
const migrationResult = ref<UpstreamLegacyMigrationResult | null>(null)
let searchTimer: ReturnType<typeof setTimeout> | null = null
let loadGeneration = 0

const pagination = reactive({ page: 1, page_size: 20, total: 0 })
const filters = reactive({ search: '', provider: '', status: '' })
const form = reactive({
  name: '', provider: 'auto' as UpstreamConnectionProvider,
  auth_mode: 'password' as UpstreamConnectionAuthMode,
  management_base_url: '', forwarding_base_url: '', remote_user_id: '',
  username: '', password: '', access_token: '', refresh_token: '', proxy_id: null as number | null,
  sync_enabled: true, sync_interval_seconds: 300
})

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.upstreamConnections.columns.name') },
  { key: 'provider', label: t('admin.upstreamConnections.columns.provider') },
  { key: 'wallet', label: t('admin.upstreamConnections.columns.wallet') },
  { key: 'observations', label: t('admin.upstreamConnections.columns.observations') },
  { key: 'last_synced_at', label: t('admin.upstreamConnections.columns.lastSync') },
  { key: 'status', label: t('admin.upstreamConnections.columns.status') },
  { key: 'actions', label: t('admin.upstreamConnections.columns.actions') }
])

const providerValues: UpstreamConnectionProvider[] = ['auto', 'newapi', 'sub2api', 'rixapi', 'shellapi', 'oneapi', 'veloera', 'onehub', 'donehub']
const providerOptions = computed(() => providerValues.map(value => ({ value, label: providerLabel(value) })))
const providerFilterOptions = computed(() => [{ value: '', label: t('admin.upstreamConnections.allProviders') }, ...providerOptions.value])
const statusValues = ['pending', 'ready', 'degraded', 'auth_error', 'needs_input', 'disabled']
const statusFilterOptions = computed(() => [{ value: '', label: t('admin.upstreamConnections.allStatuses') }, ...statusValues.map(value => ({ value, label: statusLabel(value) }))])
const authModeOptions = computed(() => [
  { value: 'password', label: t('admin.upstreamConnections.authModes.password') },
  { value: 'access_token', label: t('admin.upstreamConnections.authModes.access_token') }
])
const showsRemoteUserId = computed(() => form.auth_mode === 'access_token' && form.provider !== 'sub2api')
const requiresRemoteUserId = computed(() => form.auth_mode === 'access_token' && ['newapi', 'rixapi', 'shellapi', 'veloera'].includes(form.provider))

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
function migrationActionLabel(action: string): string {
  return t(`admin.upstreamConnections.migration.actions.${action}`, action)
}
function migrationActionClass(action: string): string {
  if (['migrated', 'reused_and_bound', 'already_migrated'].includes(action)) return 'badge-success'
  if (action === 'failed') return 'badge-danger'
  if (action.startsWith('skip_')) return 'badge-warning'
  return 'badge-gray'
}
function formatWallet(connection: UpstreamConnection): string {
  if (connection.wallet_unlimited) return t('admin.upstreamConnections.unlimited')
  if (connection.wallet_amount === null) return t('admin.upstreamConnections.unknown')
  const currency = connection.wallet_currency || ''
  return `${connection.wallet_amount.toLocaleString(undefined, { maximumFractionDigits: 4 })} ${currency}`.trim()
}
function errorMessage(error: unknown, fallback: string): string {
  const response = (error as { response?: { data?: { message?: string; detail?: string } }; message?: string })
  return response.response?.data?.message || response.response?.data?.detail || response.message || fallback
}

async function loadConnections(page = pagination.page): Promise<void> {
  const generation = ++loadGeneration
  loading.value = true
  try {
    const result = await adminAPI.upstreamConnections.list(page, pagination.page_size, {
      search: filters.search || undefined, provider: filters.provider || undefined, status: filters.status || undefined
    })
    if (generation === loadGeneration) {
      connections.value = result.items
      pagination.page = result.page
      pagination.total = result.total
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
function changePageSize(size: number): void { pagination.page_size = size; void loadConnections(1) }
function resetForm(): void {
  Object.assign(form, { name: '', provider: 'auto', auth_mode: 'password', management_base_url: '', forwarding_base_url: '', remote_user_id: '', username: '', password: '', access_token: '', refresh_token: '', proxy_id: null, sync_enabled: true, sync_interval_seconds: 300 })
}
function openCreate(): void { editing.value = null; resetForm(); showForm.value = true }
function openEdit(connection: UpstreamConnection): void {
  editing.value = connection
  Object.assign(form, { name: connection.name, provider: connection.provider, auth_mode: connection.auth_mode, management_base_url: connection.management_base_url, forwarding_base_url: connection.forwarding_base_url, remote_user_id: connection.remote_user_id, username: '', password: '', access_token: '', refresh_token: '', proxy_id: connection.proxy_id, sync_enabled: connection.sync_enabled, sync_interval_seconds: connection.sync_interval_seconds })
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
  probingIds.value.add(connection.id)
  try {
    const result = await adminAPI.upstreamConnections.probe(connection.id)
    appStore.showSuccess(t('admin.upstreamConnections.probeSuccess'))
    if (details.value?.id === result.id) details.value = result
    await loadConnections()
  } catch (error: unknown) {
    appStore.showError(errorMessage(error, t('admin.upstreamConnections.probeFailed')))
  } finally {
    probingIds.value.delete(connection.id)
  }
}
async function openDetails(connection: UpstreamConnection): Promise<void> {
  try { details.value = await adminAPI.upstreamConnections.get(connection.id) }
  catch (error: unknown) { appStore.showError(errorMessage(error, t('admin.upstreamConnections.loadFailed'))) }
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

async function openLegacyMigration(): Promise<void> {
  showLegacyMigration.value = true
  migrationLoading.value = true
  migrationResult.value = null
  try {
    migrationResult.value = await adminAPI.upstreamConnections.previewLegacyMigration()
  } catch (error: unknown) {
    appStore.showError(errorMessage(error, t('admin.upstreamConnections.migration.previewFailed')))
    showLegacyMigration.value = false
  } finally {
    migrationLoading.value = false
  }
}
function closeLegacyMigration(): void {
  if (migrationApplying.value) return
  showLegacyMigration.value = false
  migrationResult.value = null
}
async function applyLegacyMigration(): Promise<void> {
  if (!migrationResult.value?.dry_run || migrationResult.value.summary.planned_accounts === 0) return
  migrationApplying.value = true
  try {
    migrationResult.value = await adminAPI.upstreamConnections.migrateLegacy()
    appStore.showSuccess(t('admin.upstreamConnections.migration.applied', {
      count: migrationResult.value.summary.migrated_accounts
    }))
    await loadConnections(1)
  } catch (error: unknown) {
    appStore.showError(errorMessage(error, t('admin.upstreamConnections.migration.applyFailed')))
  } finally {
    migrationApplying.value = false
  }
}

onMounted(async () => {
  await Promise.all([loadConnections(1), adminAPI.proxies.getAll().then(items => { proxies.value = items }).catch(() => undefined)])
})
onBeforeUnmount(() => {
  loadGeneration++
  if (searchTimer) clearTimeout(searchTimer)
})
</script>
