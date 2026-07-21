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
            <button
              class="btn btn-secondary"
              :title="t('admin.upstreamConnections.runtime.refreshTitle')"
              :disabled="loading || runtimeRefreshing"
              @click="handlePageRuntimeRefresh"
            >
              <Icon name="refresh" size="md" :class="runtimeRefreshing ? 'animate-spin' : ''" />
              <span class="ml-2">{{ t('admin.upstreamConnections.runtime.refresh') }}</span>
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
            <button class="row-name text-left" :title="t('admin.upstreamConnections.detail.connectionAndUsage')" @click="openDetails(row)">
              <span class="row-name-label block font-medium text-gray-900 hover:text-primary-600 dark:text-white dark:hover:text-primary-400">
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
          <template #cell-runtime="{ row }">
            <!-- Compact list cell: accounts|concurrency + group rows with cost / 5m volume / rate -->
            <div class="w-[292px] min-w-[292px]">
              <div
                v-if="row.runtime_available"
                class="rounded-md border border-gray-200/90 bg-white dark:border-dark-600 dark:bg-dark-800/70"
              >
                <button
                  type="button"
                  class="flex w-full items-center gap-1 border-b border-gray-100 px-2 py-1 text-left transition hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/50"
                  :title="runtimeSummaryTitle(row)"
                  @click="openDetails(row)"
                >
                  <div class="min-w-0 flex-1 truncate text-[11px] tabular-nums text-gray-700 dark:text-gray-200">
                    <span class="font-semibold text-gray-900 dark:text-gray-100">{{ row.binding_count }}</span>
                    <span class="text-gray-500 dark:text-gray-400"> {{ t('admin.upstreamConnections.runtime.accountsUnit') }}</span>
                    <span class="mx-1 text-gray-300 dark:text-dark-500">|</span>
                    <span class="text-gray-600 dark:text-gray-300">{{ runtimeCompactConcurrencyLabel(row) }}</span>
                  </div>
                  <Icon name="chevronRight" size="sm" class="shrink-0 text-gray-400" />
                </button>

                <div v-if="runtimeGroups(row).length" class="px-1.5 py-0.5">
                  <button
                    v-for="group in runtimeSummaryGroups(row)"
                    :key="group.group_id"
                    type="button"
                    data-testid="runtime-group-row"
                    class="grid w-full grid-cols-[minmax(0,1fr)_auto] items-baseline gap-x-2 rounded px-1 py-0.5 text-left transition hover:bg-gray-50 dark:hover:bg-dark-700/40"
                    :title="runtimeGroupTitle(group)"
                    @click="openAccountGroup(row.id, group.group_id)"
                  >
                    <span class="flex min-w-0 items-center gap-1.5">
                      <span class="h-1.5 w-1.5 shrink-0 rounded-full" :class="runtimeSuccessDotClass(group)" />
                      <span class="truncate text-[11px] font-medium text-gray-800 dark:text-gray-100">
                        {{ runtimeGroupDisplayName(group) }}
                      </span>
                    </span>
                    <span class="flex shrink-0 items-center gap-1.5 text-[10px] tabular-nums">
                      <span class="font-medium text-emerald-600 dark:text-emerald-400">{{ runtimeGroupCostLabel(group) }}</span>
                      <span class="text-gray-300 dark:text-dark-500">·</span>
                      <!-- 5m volume must stay list-visible (1/1 vs 1000/1000). -->
                      <span class="font-semibold text-gray-600 dark:text-gray-300" data-testid="runtime-group-5m-count">
                        {{ runtimeGroupRequestLabel(group) }}
                      </span>
                      <span class="text-gray-300 dark:text-dark-500">·</span>
                      <span class="font-bold" :class="runtimeSuccessTextClass(group)">{{ runtimeGroupRateLabel(group) }}</span>
                    </span>
                  </button>
                  <button
                    v-if="runtimeHiddenGroupCount(row) > 0"
                    type="button"
                    class="w-full rounded px-1 py-0.5 text-left text-[10px] font-medium text-sky-600 hover:text-sky-700 dark:text-sky-400"
                    @click="openDetails(row)"
                  >
                    {{ t('admin.upstreamConnections.runtime.moreGroups', { count: runtimeHiddenGroupCount(row) }) }} →
                  </button>
                </div>
                <button
                  v-else
                  type="button"
                  class="w-full px-2 py-1 text-left text-[11px] text-gray-400 hover:bg-gray-50 dark:text-gray-500 dark:hover:bg-dark-700/40"
                  @click="openDetails(row)"
                >
                  {{ t('admin.upstreamConnections.runtime.noTraffic') }}
                </button>
              </div>

              <div
                v-else
                class="flex items-center gap-1.5 rounded-md border border-amber-200/80 bg-amber-50/70 px-2 py-1 text-xs text-amber-700 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-300"
              >
                <span class="min-w-0 flex-1 truncate" :title="row.runtime_error">{{ t('admin.upstreamConnections.runtime.unavailable') }}</span>
                <button
                  type="button"
                  class="rounded p-0.5 hover:bg-amber-100 dark:hover:bg-amber-900/40"
                  :title="t('admin.upstreamConnections.runtime.retryRow')"
                  data-testid="runtime-row-retry"
                  @click="handleRowRuntimeRefresh(row.id)"
                >
                  <Icon name="refresh" size="sm" :class="runtimeRefreshing ? 'animate-spin' : ''" />
                </button>
              </div>
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

        <section class="rounded-xl border border-gray-200 bg-slate-50/60 p-4 dark:border-dark-600 dark:bg-dark-800/50">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.upstreamConnections.runtime.detailTitle') }}</h3>
              <p class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">{{ t('admin.upstreamConnections.runtime.detailHint') }}</p>
            </div>
            <div class="flex items-center gap-2">
              <span
                v-if="details.runtime_fetched_at"
                class="rounded-full bg-white px-2.5 py-1 text-[11px] text-gray-500 shadow-sm dark:bg-dark-700 dark:text-gray-400"
              >
                {{ t('admin.upstreamConnections.runtime.updatedAt', { time: formatDateTime(details.runtime_fetched_at) }) }}
              </span>
              <button
                type="button"
                class="inline-flex items-center gap-1 rounded-lg border border-gray-200 bg-white px-2.5 py-1 text-[11px] font-medium text-gray-700 shadow-sm transition hover:bg-gray-50 disabled:opacity-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
                :title="t('admin.upstreamConnections.runtime.refreshTitle')"
                :disabled="runtimeRefreshing"
                data-testid="runtime-detail-refresh"
                @click="handleDetailRuntimeRefresh"
              >
                <Icon name="refresh" size="sm" :class="runtimeRefreshing ? 'animate-spin' : ''" />
                {{ t('admin.upstreamConnections.runtime.refresh') }}
              </button>
            </div>
          </div>
          <div class="mt-3 grid gap-3 sm:grid-cols-4">
            <div class="rounded-xl border border-gray-200/80 bg-white px-3 py-2.5 shadow-sm dark:border-dark-600 dark:bg-dark-800">
              <p class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('admin.upstreamConnections.runtime.boundAccounts', { count: details.binding_count }) }}</p>
              <p class="mt-1 text-xl font-semibold tabular-nums text-gray-900 dark:text-white">{{ details.binding_count }}</p>
            </div>
            <div class="rounded-xl border border-gray-200/80 bg-white px-3 py-2.5 shadow-sm dark:border-dark-600 dark:bg-dark-800">
              <p class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('admin.upstreamConnections.runtime.currentConcurrency') }}</p>
              <p class="mt-1 text-xl font-semibold tabular-nums text-gray-900 dark:text-white">{{ runtimeConcurrencyTotal(details) }}</p>
            </div>
            <div class="rounded-xl border border-gray-200/80 bg-white px-3 py-2.5 shadow-sm dark:border-dark-600 dark:bg-dark-800">
              <p class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('admin.upstreamConnections.runtime.waiting') }}</p>
              <p class="mt-1 text-xl font-semibold tabular-nums text-gray-900 dark:text-white">{{ runtimeWaitingTotal(details) }}</p>
            </div>
            <div class="rounded-xl border border-emerald-200/70 bg-emerald-50/50 px-3 py-2.5 shadow-sm dark:border-emerald-900/40 dark:bg-emerald-950/20">
              <p class="text-[11px] text-emerald-700/80 dark:text-emerald-300/80">{{ t('admin.upstreamConnections.runtime.todayUsageLabel') }}</p>
              <p class="mt-1 text-sm font-semibold tabular-nums text-emerald-700 dark:text-emerald-300">{{ runtimeTodayUsageLabel(details) }}</p>
            </div>
          </div>
          <div class="mt-4 overflow-hidden rounded-xl border border-gray-200 dark:border-dark-600">
            <div class="border-b border-gray-100 bg-gray-50 px-3 py-2 text-xs font-semibold text-gray-600 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300">
              {{ t('admin.upstreamConnections.runtime.groupRuntime') }}
            </div>
            <div class="overflow-x-auto">
              <table class="min-w-full divide-y divide-gray-100 text-sm dark:divide-dark-700">
                <thead class="bg-white text-left text-[11px] uppercase tracking-wide text-gray-400 dark:bg-dark-900 dark:text-gray-500">
                  <tr>
                    <th class="px-3 py-2 font-medium">{{ t('admin.upstreamConnections.detail.groupName') }}</th>
                    <th class="px-3 py-2 text-right font-medium">{{ t('admin.upstreamConnections.runtime.fiveMinuteRequests') }}</th>
                    <th class="px-3 py-2 text-right font-medium">{{ t('admin.upstreamConnections.runtime.successFailure') }}</th>
                    <th class="px-3 py-2 text-right font-medium">{{ t('admin.upstreamConnections.runtime.successRate') }}</th>
                    <th class="px-3 py-2 text-right font-medium">{{ t('admin.upstreamConnections.runtime.todayCost') }}</th>
                    <th class="px-3 py-2 text-right font-medium">{{ t('admin.upstreamConnections.runtime.todayRequests') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                  <tr v-for="group in runtimeGroups(details)" :key="group.group_id" class="bg-white hover:bg-slate-50 dark:bg-dark-900 dark:hover:bg-dark-800">
                    <td class="px-3 py-2.5">
                      <button class="inline-flex items-center gap-2 font-medium text-primary-700 hover:underline dark:text-primary-300" @click="openAccountGroup(details.id, group.group_id)">
                        <span class="h-2 w-2 rounded-full" :class="runtimeSuccessDotClass(group)" />
                        {{ runtimeGroupDisplayName(group) }}
                      </button>
                    </td>
                    <td class="px-3 py-2.5 text-right tabular-nums">{{ group.five_minute_requests.toLocaleString() }}</td>
                    <td class="px-3 py-2.5 text-right tabular-nums">{{ group.five_minute_success_count.toLocaleString() }} / {{ group.five_minute_error_count.toLocaleString() }}</td>
                    <td class="px-3 py-2.5 text-right">
                      <span class="inline-flex rounded-md px-1.5 py-0.5 text-xs font-bold tabular-nums" :class="runtimeSuccessBadgeClass(group)">{{ runtimeGroupRateLabel(group) }}</span>
                    </td>
                    <td class="px-3 py-2.5 text-right tabular-nums font-medium text-emerald-700 dark:text-emerald-300">${{ formatCost(group.today.account_cost) }}</td>
                    <td class="px-3 py-2.5 text-right tabular-nums">{{ group.today.requests.toLocaleString() }}</td>
                  </tr>
                  <tr v-if="runtimeGroups(details).length === 0">
                    <td colspan="6" class="px-3 py-8 text-center text-gray-500">{{ t('admin.upstreamConnections.runtime.noTraffic') }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
          <div class="mt-4 overflow-hidden rounded-xl border border-gray-200 dark:border-dark-600">
            <div class="border-b border-gray-100 bg-gray-50 px-3 py-2 text-xs font-semibold text-gray-600 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300">
              {{ t('admin.upstreamConnections.runtime.accountRuntime') }}
            </div>
            <div class="overflow-x-auto">
              <table class="min-w-full divide-y divide-gray-100 text-sm dark:divide-dark-700">
                <thead class="bg-white text-left text-[11px] uppercase tracking-wide text-gray-400 dark:bg-dark-900 dark:text-gray-500">
                  <tr>
                    <th class="px-3 py-2 font-medium">{{ t('admin.upstreamConnections.runtime.account') }}</th>
                    <th class="px-3 py-2 text-right font-medium">{{ t('admin.upstreamConnections.runtime.currentConcurrency') }}</th>
                    <th class="px-3 py-2 text-right font-medium">{{ t('admin.upstreamConnections.runtime.waiting') }}</th>
                    <th class="px-3 py-2 font-medium">{{ t('admin.upstreamConnections.runtime.activeGroups') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                  <tr v-for="account in (details.runtime_accounts || [])" :key="account.account_id" class="bg-white dark:bg-dark-900">
                    <td class="px-3 py-2.5 font-medium text-gray-900 dark:text-white">{{ account.account_name || `#${account.account_id}` }}</td>
                    <td class="px-3 py-2.5 text-right tabular-nums">{{ account.current_concurrency ?? '-' }}</td>
                    <td class="px-3 py-2.5 text-right tabular-nums">{{ account.waiting_count ?? '-' }}</td>
                    <td class="px-3 py-2.5 text-xs text-gray-600 dark:text-gray-300">{{ runtimeAccountGroupsLabel(account) }}</td>
                  </tr>
                  <tr v-if="!(details.runtime_accounts || []).length">
                    <td colspan="4" class="px-3 py-8 text-center text-gray-500">{{ t('admin.upstreamConnections.runtime.noBoundAccounts') }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </section>

        <section>
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.upstreamConnections.detail.groups') }}</h3>
          <div class="mt-2 overflow-x-auto border-y border-gray-200 dark:border-dark-600">
            <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
              <thead class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-700">
                <tr><th class="px-3 py-2">{{ t('admin.upstreamConnections.detail.groupName') }}</th><th class="px-3 py-2">{{ t('admin.upstreamConnections.detail.multiplier') }}</th><th class="px-3 py-2">{{ t('admin.upstreamConnections.detail.source') }}</th></tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="group in (details.groups || [])" :key="group.id"><td class="px-3 py-2">{{ group.name }}</td><td class="px-3 py-2">{{ group.rate_multiplier === null ? t('admin.upstreamConnections.unknown') : `${group.rate_multiplier}x` }}</td><td class="px-3 py-2 text-xs text-gray-500">{{ group.source }}</td></tr>
                <tr v-if="!(details.groups || []).length"><td colspan="3" class="px-3 py-6 text-center text-gray-500">{{ t('admin.upstreamConnections.detail.noGroups') }}</td></tr>
              </tbody>
            </table>
          </div>
        </section>

        <UpstreamConnectionUsagePanel
          :usage="detailsUsage"
          :bindings="details.bindings || []"
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
import { useRouter } from 'vue-router'
import { adminAPI } from '@/api/admin'
import type {
  CreateUpstreamConnectionRequest,
  UpstreamConnection,
  UpstreamConnectionAuthMode,
  UpstreamConnectionProvider,
  UpstreamConnectionRuntimeAccount,
  UpstreamConnectionRuntimeGroup,
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
const router = useRouter()
const appStore = useAppStore()
const loading = ref(false)
const saving = ref(false)
type UpstreamConnectionRow = UpstreamConnection & {
  today_requests: number | null
  today_cost: number | null
  runtime_available: boolean
  runtime_error: string
  runtime_fetched_at: string | null
  runtime_accounts: UpstreamConnectionRuntimeAccount[]
}
const allConnections = ref<UpstreamConnectionRow[]>([])
const connections = ref<UpstreamConnectionRow[]>([])
const todayStatsAvailable = ref(true)
const runtimeRefreshing = ref(false)
const proxies = ref<Proxy[]>([])
const probingIds = ref(new Set<number>())
const details = ref<UpstreamConnectionRow | null>(null)
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
  { key: 'runtime', label: t('admin.upstreamConnections.columns.runtime'), class: 'w-[292px] min-w-[292px]' },
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
function runtimeAccountUsage(account: UpstreamConnectionRuntimeAccount): { cost: number; requests: number } {
  return (account.groups ?? []).reduce((total, group) => ({
    cost: total.cost + Number(group.today.account_cost || 0),
    requests: total.requests + Number(group.today.requests || 0)
  }), { cost: 0, requests: 0 })
}
function runtimeCompactConcurrencyLabel(row: UpstreamConnectionRow): string {
  const runtimeAccounts = row.runtime_accounts ?? []
  const known = runtimeAccounts.filter(account => account.current_concurrency !== null && account.current_concurrency !== undefined)
  if (known.length === 0) return t('admin.upstreamConnections.runtime.compactConcurrencyUnavailable')
  const total = known.reduce((sum, account) => sum + Number(account.current_concurrency || 0), 0)
  // Treat missing bound accounts or null concurrency fields as partial visibility.
  const partial =
    known.length < runtimeAccounts.length ||
    runtimeAccounts.length < Number(row.binding_count || 0)
  return partial
    ? t('admin.upstreamConnections.runtime.compactPartialConcurrency', { count: total })
    : t('admin.upstreamConnections.runtime.compactConcurrency', { count: total })
}
function runtimeConcurrencyTotal(row: UpstreamConnectionRow): string {
  const known = (row.runtime_accounts ?? []).filter(account => account.current_concurrency !== null)
  if (known.length === 0) return '-'
  return known.reduce((sum, account) => sum + Number(account.current_concurrency || 0), 0).toLocaleString()
}
function runtimeWaitingTotal(row: UpstreamConnectionRow): string {
  const known = (row.runtime_accounts ?? []).filter(account => account.waiting_count !== null)
  if (known.length === 0) return '-'
  return known.reduce((sum, account) => sum + Number(account.waiting_count || 0), 0).toLocaleString()
}
function runtimeUsage(row: UpstreamConnectionRow): { cost: number; requests: number } {
  return (row.runtime_accounts ?? []).reduce((total, account) => {
    const accountUsage = runtimeAccountUsage(account)
    return { cost: total.cost + accountUsage.cost, requests: total.requests + accountUsage.requests }
  }, { cost: 0, requests: 0 })
}
function runtimeTodayUsageLabel(row: UpstreamConnectionRow): string {
  const usage = runtimeUsage(row)
  return t('admin.upstreamConnections.runtime.todayUsage', { cost: formatCost(usage.cost), count: usage.requests.toLocaleString() })
}
function runtimeGroups(row: UpstreamConnectionRow): UpstreamConnectionRuntimeGroup[] {
  const groupsByID = new Map<number, UpstreamConnectionRuntimeGroup>()
  for (const account of row.runtime_accounts ?? []) {
    for (const group of account.groups ?? []) {
      const existing = groupsByID.get(group.group_id)
      if (!existing) {
        groupsByID.set(group.group_id, { ...group, today: { ...group.today } })
        continue
      }
      existing.today.requests += group.today.requests
      existing.today.tokens += group.today.tokens
      existing.today.account_cost += group.today.account_cost
      existing.today.standard_cost += group.today.standard_cost
      existing.today.user_cost += group.today.user_cost
      existing.five_minute_requests += group.five_minute_requests
      existing.five_minute_success_count += group.five_minute_success_count
      existing.five_minute_error_count += group.five_minute_error_count
      existing.five_minute_success_rate = existing.five_minute_requests > 0
        ? existing.five_minute_success_count * 100 / existing.five_minute_requests
        : null
    }
  }
  return [...groupsByID.values()].sort((left, right) => {
    if (left.today.account_cost !== right.today.account_cost) return right.today.account_cost - left.today.account_cost
    if (left.five_minute_requests !== right.five_minute_requests) return right.five_minute_requests - left.five_minute_requests
    if (left.today.requests !== right.today.requests) return right.today.requests - left.today.requests
    return left.group_name.localeCompare(right.group_name)
  })
}
function runtimeGroupDisplayName(group: UpstreamConnectionRuntimeGroup): string {
  const name = String(group.group_name || '').trim()
  if (name) return name
  // group_id=0 is log-level ungrouped traffic; positive missing join is a deleted group.
  if (Number(group.group_id) === 0) return t('admin.upstreamConnections.runtime.ungroupedTraffic')
  return t('admin.upstreamConnections.runtime.deletedGroup')
}
function runtimeCompactGroupLabel(group: UpstreamConnectionRuntimeGroup): string {
  const name = runtimeGroupDisplayName(group)
  return group.five_minute_requests > 0
    ? t('admin.upstreamConnections.runtime.compactGroupSuccessRate', { name, rate: (group.five_minute_success_count * 100 / group.five_minute_requests).toFixed(1) })
    : t('admin.upstreamConnections.runtime.compactGroupNoRecentRequests', { name })
}
function runtimeSummaryGroups(row: UpstreamConnectionRow): UpstreamConnectionRuntimeGroup[] {
  return runtimeGroups(row).slice(0, 2)
}
function runtimeHiddenGroupCount(row: UpstreamConnectionRow): number {
  return Math.max(0, runtimeGroups(row).length - runtimeSummaryGroups(row).length)
}
function runtimeGroupRateLabel(group: UpstreamConnectionRuntimeGroup): string {
  if (group.five_minute_requests <= 0) return '—'
  return `${(group.five_minute_success_count * 100 / group.five_minute_requests).toFixed(1)}%`
}
function runtimeGroupRequestLabel(group: UpstreamConnectionRuntimeGroup): string {
  // Keep short so list column always shows 5m volume next to success rate.
  return group.five_minute_requests > 0
    ? t('admin.upstreamConnections.runtime.fiveMinuteRequestsCompact', {
        count: group.five_minute_requests.toLocaleString()
      })
    : t('admin.upstreamConnections.runtime.noRecentRequests')
}
function runtimeGroupCostLabel(group: UpstreamConnectionRuntimeGroup): string {
  return `$${formatCost(group.today.account_cost)}`
}
function runtimeSuccessTier(group: UpstreamConnectionRuntimeGroup): 'none' | 'good' | 'warn' | 'bad' {
  if (group.five_minute_requests <= 0) return 'none'
  const rate = group.five_minute_success_count * 100 / group.five_minute_requests
  if (rate >= 98) return 'good'
  if (rate >= 90) return 'warn'
  return 'bad'
}
function runtimeSuccessDotClass(group: UpstreamConnectionRuntimeGroup): string {
  switch (runtimeSuccessTier(group)) {
    case 'good': return 'bg-emerald-500'
    case 'warn': return 'bg-amber-500'
    case 'bad': return 'bg-red-500'
    default: return 'bg-gray-300 dark:bg-dark-500'
  }
}
function runtimeSuccessBadgeClass(group: UpstreamConnectionRuntimeGroup): string {
  switch (runtimeSuccessTier(group)) {
    case 'good': return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
    case 'warn': return 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300'
    case 'bad': return 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
    default: return 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
  }
}
function runtimeSuccessTextClass(group: UpstreamConnectionRuntimeGroup): string {
  switch (runtimeSuccessTier(group)) {
    case 'good': return 'text-emerald-600 dark:text-emerald-400'
    case 'warn': return 'text-amber-600 dark:text-amber-400'
    case 'bad': return 'text-red-600 dark:text-red-400'
    default: return 'text-gray-400 dark:text-gray-500'
  }
}
function runtimeAccountGroupsLabel(account: UpstreamConnectionRuntimeAccount): string {
  const groups = account.groups ?? []
  if (groups.length === 0) return t('admin.upstreamConnections.runtime.noTraffic')
  return groups.map(group => runtimeGroupDisplayName(group)).join(' · ')
}
function runtimeGroupTitle(group: UpstreamConnectionRuntimeGroup): string {
  return `${runtimeCompactGroupLabel(group)} · ${t('admin.upstreamConnections.runtime.todayUsage', { cost: formatCost(group.today.account_cost), count: group.today.requests.toLocaleString() })}`
}
function runtimeSummaryTitle(row: UpstreamConnectionRow): string {
  if (!row.runtime_available) return row.runtime_error || t('admin.upstreamConnections.runtime.unavailable')
  const groupSummary = runtimeGroups(row).map(runtimeGroupTitle).join('\n') || t('admin.upstreamConnections.runtime.noTraffic')
  return [
    t('admin.upstreamConnections.runtime.boundAccounts', { count: row.binding_count }),
    runtimeCompactConcurrencyLabel(row),
    runtimeTodayUsageLabel(row),
    groupSummary,
    row.runtime_fetched_at ? t('admin.upstreamConnections.runtime.updatedAt', { time: formatDateTime(row.runtime_fetched_at) }) : '',
    t('admin.upstreamConnections.runtime.fiveMinuteWindowHint')
  ].filter(Boolean).join('\n')
}
function openAccountGroup(connectionID: number, groupID: number): void {
  // Runtime groups come from today's usage/error logs. Account list filters by
  // current membership — they can diverge after rebinding. Flag the source so
  // Accounts can show an explainable chip.
  // group_id=0 is log-level ungrouped traffic, NOT membership "ungrouped".
  const query: Record<string, string> = {
    upstream_connection_id: String(connectionID),
    runtime_traffic: '1'
  }
  if (groupID > 0) query.group = String(groupID)
  void router.push({ name: 'AdminAccounts', query })
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
      today_cost: row.today_cost,
      runtime_available: row.runtime_available,
      runtime_error: row.runtime_error,
      runtime_fetched_at: row.runtime_fetched_at,
      runtime_accounts: row.runtime_accounts
    }
  }
  allConnections.value = allConnections.value.map(merge)
  // Re-sort/page so balance / last-sync ordered lists move the refreshed row immediately.
  applySortAndPage(pagination.page)
}

function withRuntimeSnapshot(connection: UpstreamConnection, snapshot: UpstreamConnectionRow): UpstreamConnectionRow {
  return {
    ...connection,
    id: snapshot.id,
    wallet_amount: connection.wallet_amount ?? snapshot.wallet_amount,
    wallet_currency: connection.wallet_currency || snapshot.wallet_currency,
    wallet_usd: connection.wallet_usd ?? snapshot.wallet_usd,
    wallet_unlimited: connection.wallet_unlimited ?? snapshot.wallet_unlimited,
    wallet_reliability: connection.wallet_reliability || snapshot.wallet_reliability,
    groups: connection.groups ?? snapshot.groups ?? [],
    bindings: connection.bindings ?? snapshot.bindings ?? [],
    today_requests: snapshot.today_requests,
    today_cost: snapshot.today_cost,
    runtime_available: snapshot.runtime_available,
    runtime_error: snapshot.runtime_error,
    runtime_fetched_at: snapshot.runtime_fetched_at,
    runtime_accounts: snapshot.runtime_accounts
  }
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
      today_cost: remote.today_cost,
      runtime_available: remote.runtime_available,
      runtime_error: remote.runtime_error,
      runtime_fetched_at: remote.runtime_fetched_at,
      runtime_accounts: remote.runtime_accounts
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
    let runtimeAvailable = true
    let runtimeError = ''
    let runtimeFetchedAt: string | null = null
    let runtimeAccountsByID = new Map<number, UpstreamConnectionRuntimeAccount>()
    if (accountIds.length > 0) {
      const [statsResult, runtimeResult] = await Promise.allSettled([
        adminAPI.accounts.getBatchTodayStats(accountIds),
        adminAPI.upstreamConnections.getRuntimeOverview(accountIds)
      ])
      if (statsResult.status === 'fulfilled') {
        stats = statsResult.value.stats
      } else {
        statsAvailable = false
      }
      if (runtimeResult.status === 'fulfilled') {
        runtimeAccountsByID = new Map(runtimeResult.value.accounts.map(account => [account.account_id, account]))
        runtimeFetchedAt = new Date().toISOString()
      } else {
        runtimeAvailable = false
        runtimeError = errorMessage(runtimeResult.reason, t('admin.upstreamConnections.runtime.unavailable'))
      }
    }
    const rows: UpstreamConnectionRow[] = items.map(item => ({
      ...item,
      today_requests: statsAvailable
        ? (item.bound_account_ids ?? []).reduce((total, accountId) => total + Number(stats[String(accountId)]?.requests ?? 0), 0)
        : null,
      today_cost: statsAvailable
        ? (item.bound_account_ids ?? []).reduce((total, accountId) => total + Number(stats[String(accountId)]?.cost ?? 0), 0)
        : null,
        runtime_available: runtimeAvailable,
        runtime_error: runtimeError,
        runtime_fetched_at: runtimeFetchedAt,
        runtime_accounts: (item.bound_account_ids ?? [])
        .map(accountID => runtimeAccountsByID.get(accountID))
        .filter((account): account is UpstreamConnectionRuntimeAccount => account !== undefined)
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
function boundRuntimeAccountIDs(rows: UpstreamConnectionRow[]): number[] {
  return [...new Set(rows.flatMap(row => row.bound_account_ids ?? []))]
}

/** Only runtime snapshot fields — never overwrite groups/bindings from full get(). */
function mergeRuntimeSnapshot(
  row: UpstreamConnectionRow,
  snapshot: Pick<UpstreamConnectionRow, 'runtime_available' | 'runtime_error' | 'runtime_fetched_at' | 'runtime_accounts'>
): UpstreamConnectionRow {
  return {
    ...row,
    runtime_available: snapshot.runtime_available,
    runtime_error: snapshot.runtime_error,
    runtime_fetched_at: snapshot.runtime_fetched_at,
    runtime_accounts: snapshot.runtime_accounts
  }
}

function syncOpenDetailsRuntime(): void {
  // Keep the open dialog's concurrency/traffic in sync without wiping groups/bindings.
  if (!details.value) return
  const listRow = allConnections.value.find(row => row.id === details.value!.id)
  if (!listRow) return
  details.value = mergeRuntimeSnapshot(details.value, {
    runtime_available: listRow.runtime_available,
    runtime_error: listRow.runtime_error,
    runtime_fetched_at: listRow.runtime_fetched_at,
    runtime_accounts: listRow.runtime_accounts
  })
}

/** Click wrappers — never pass native Event into options-typed refresh. */
function handlePageRuntimeRefresh(): void {
  void refreshRuntimeOverview()
}
function handleRowRuntimeRefresh(connectionId: number): void {
  void refreshRuntimeOverview({ connectionId })
}
function handleDetailRuntimeRefresh(): void {
  if (!details.value) return
  void refreshRuntimeOverview({ connectionId: details.value.id })
}

async function refreshRuntimeOverview(options?: { connectionId?: number }): Promise<void> {
  if (runtimeRefreshing.value) return

  // Page-level refresh: all listed connections. Detail button: only the open connection.
  const connectionId = options?.connectionId
  const targetRows =
    connectionId != null
      ? allConnections.value.filter(row => row.id === connectionId)
      : allConnections.value
  const accountIDs = boundRuntimeAccountIDs(targetRows)
  if (accountIDs.length === 0) return

  const targetIds = new Set(targetRows.map(row => row.id))
  runtimeRefreshing.value = true
  try {
    const overview = await adminAPI.upstreamConnections.getRuntimeOverview(accountIDs)
    const accountsByID = new Map(overview.accounts.map(account => [account.account_id, account]))
    const fetchedAt = new Date().toISOString()
    allConnections.value = allConnections.value.map(row => {
      if (!targetIds.has(row.id)) return row
      return mergeRuntimeSnapshot(row, {
        runtime_available: true,
        runtime_error: '',
        runtime_fetched_at: fetchedAt,
        runtime_accounts: (row.bound_account_ids ?? [])
          .map(accountID => accountsByID.get(accountID))
          .filter((account): account is UpstreamConnectionRuntimeAccount => account !== undefined)
      })
    })
    applySortAndPage(pagination.page)
    syncOpenDetailsRuntime()
  } catch (error: unknown) {
    const message = errorMessage(error, t('admin.upstreamConnections.runtime.unavailable'))
    allConnections.value = allConnections.value.map(row => {
      if (!targetIds.has(row.id)) return row
      return mergeRuntimeSnapshot(row, {
        runtime_available: false,
        runtime_error: message,
        runtime_fetched_at: null,
        runtime_accounts: []
      })
    })
    applySortAndPage(pagination.page)
    syncOpenDetailsRuntime()
    appStore.showError(t('admin.upstreamConnections.runtime.refreshFailed'))
  } finally {
    runtimeRefreshing.value = false
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
        ...(form.remote_user_id !== editing.value.remote_user_id ? { remote_user_id: form.remote_user_id } : {}),
        sync_enabled: form.sync_enabled,
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
      details.value = withRuntimeSnapshot(result, details.value)
      await loadDetailsUsage(result.id)
    }
  } catch (error: unknown) {
    appStore.showError(errorMessage(error, t('admin.upstreamConnections.probeFailed')))
  } finally {
    setProbing(connection.id, false)
  }
}
async function openDetails(connection: UpstreamConnectionRow): Promise<void> {
  const generation = ++detailsGeneration
  const connectionId = connection.id
  details.value = withRuntimeSnapshot(connection, connection)
  detailsUsage.value = null
  detailsUsageError.value = ''
  detailsUsageLoading.value = true
  try {
    const [connectionResult, usageResult] = await Promise.allSettled([
      adminAPI.upstreamConnections.get(connectionId),
      adminAPI.upstreamConnections.getTodayUsage(connectionId)
    ])
    if (generation !== detailsGeneration) return
    if (connectionResult.status === 'rejected') throw connectionResult.reason
    // Prefer the latest list-row runtime snapshot so a mid-flight "刷新运行"
    // is not overwritten by the GET that started at open time.
    const latestRuntime =
      allConnections.value.find(row => row.id === connectionId) ?? connection
    details.value = withRuntimeSnapshot(connectionResult.value, latestRuntime)
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
