<template>
  <AppLayout>
    <div class="keys-market">
      <TablePageLayout>
      <template #filters>
        <div class="flex flex-col gap-3">
          <div class="flex flex-wrap items-center gap-3">
            <SearchInput
              v-model="filterSearch"
              :placeholder="t('keys.searchPlaceholder')"
              class="w-full sm:w-64"
              @search="onFilterChange"
            />
            <Select
              :model-value="filterGroupId"
              class="w-40"
              :options="groupFilterOptions"
              @update:model-value="onGroupFilterChange"
            />
            <Select
              :model-value="filterStatus"
              class="w-40"
              :options="statusFilterOptions"
              @update:model-value="onStatusFilterChange"
            />
          </div>
          <EndpointPopover
            v-if="publicSettings?.api_base_url || (publicSettings?.custom_endpoints?.length ?? 0) > 0"
            :api-base-url="publicSettings?.api_base_url || ''"
            :custom-endpoints="publicSettings?.custom_endpoints || []"
          />
        </div>
      </template>

      <template #actions>
        <div class="flex justify-end gap-3">
        <button
          @click="loadApiKeys"
          :disabled="loading"
          class="btn btn-secondary"
          :title="t('common.refresh')"
        >
          <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
        </button>
        <div class="relative" ref="columnDropdownRef">
          <button
            @click="showColumnDropdown = !showColumnDropdown"
            class="btn btn-secondary px-2 md:px-3"
            :title="t('keys.columnSettings')"
          >
            <svg
              class="h-4 w-4 md:mr-1.5"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
              stroke-width="1.5"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M9 4.5v15m6-15v15m-10.875 0h15.75c.621 0 1.125-.504 1.125-1.125V5.625c0-.621-.504-1.125-1.125-1.125H4.125C3.504 4.5 3 5.004 3 5.625v12.75c0 .621.504 1.125 1.125 1.125z"
              />
            </svg>
            <span class="hidden md:inline">{{ t('keys.columnSettings') }}</span>
          </button>
          <div
            v-if="showColumnDropdown"
            class="absolute right-0 top-full z-50 mt-1 max-h-80 w-48 overflow-y-auto rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-dark-600 dark:bg-dark-800"
          >
            <button
              v-for="col in toggleableColumns"
              :key="col.key"
              @click="toggleColumn(col.key)"
              class="flex w-full items-center justify-between px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
            >
              <span>{{ col.label }}</span>
              <Icon
                v-if="isColumnVisible(col.key)"
                name="check"
                size="sm"
                class="text-primary-500"
                :stroke-width="2"
              />
            </button>
          </div>
        </div>
        <button @click="handleCreateAction" class="btn btn-primary" data-tour="keys-create-btn">
          <Icon name="plus" size="md" class="mr-2" />
          {{ t('keys.createKey') }}
        </button>
      </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="apiKeys"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-id="{ value }">
            <span class="font-mono text-xs text-gray-500 dark:text-gray-400">#{{ value }}</span>
          </template>

          <template #cell-key="{ value, row }">
            <div class="flex items-center gap-2">
              <code class="code text-xs">
                {{ maskApiKey(value) }}
              </code>
              <button
                @click="copyToClipboard(value, row.id)"
                class="rounded-lg p-1 transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
                :class="
                  copiedKeyId === row.id
                    ? 'text-green-500'
                    : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300'
                "
                :title="copiedKeyId === row.id ? t('keys.copied') : t('keys.copyToClipboard')"
              >
                <Icon
                  v-if="copiedKeyId === row.id"
                  name="check"
                  size="sm"
                  :stroke-width="2"
                />
                <Icon v-else name="clipboard" size="sm" />
              </button>
            </div>
          </template>

          <template #cell-name="{ value, row }">
            <div class="flex items-center gap-1.5">
              <span class="font-medium text-gray-900 dark:text-white">{{ value }}</span>
              <Icon
                v-if="row.ip_whitelist?.length > 0 || row.ip_blacklist?.length > 0"
                name="shield"
                size="sm"
                class="text-blue-500"
                :title="t('keys.ipRestrictionEnabled')"
              />
            </div>
          </template>

          <template #cell-entitlement="{ row }">
            <button
              v-if="rowAccessSource(row) === 'entitlement' && row.subscription_entitlement_id"
              type="button"
              class="entitlement-card-btn plan"
              :title="t('keys.clickToChangeEntitlement')"
              @click="editKey(row)"
            >
              <div class="flex w-full items-center gap-2">
                <span class="badge-tag plan-badge">
                  {{ t('keys.planCardBadge') }}
                </span>
                <span class="ml-auto text-[11px] font-medium text-gray-500 dark:text-gray-400">
                  {{ entitlementQuotaPeriodTextByID(row.subscription_entitlement_id) }}
                </span>
              </div>
              <div class="mt-1 w-full truncate text-sm font-semibold text-gray-900 dark:text-white">
                {{ entitlementLabelByID(row.subscription_entitlement_id) }}
              </div>
              <div class="mt-1.5 w-full">
                <div class="flex min-w-0 items-center justify-between gap-2 text-xs">
                  <span
                    :class="[
                      'min-w-0 truncate font-semibold',
                      entitlementQuotaTextClassByID(row.subscription_entitlement_id)
                    ]"
                  >
                    {{ entitlementQuotaRemainingTextByID(row.subscription_entitlement_id) }}
                  </span>
                  <span class="action-trigger-text">
                    {{ t('keys.switchCardShort') }}
                  </span>
                </div>
              </div>
            </button>
            <button
              v-else
              type="button"
              class="entitlement-card-btn balance"
              :title="t('keys.clickToChangeEntitlement')"
              @click="editKey(row)"
            >
              <span class="badge-tag balance-badge">
                {{ t('keys.accessSourceBalanceShort') }}
              </span>
              <span class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('keys.accessSourceBalance') }}
              </span>
              <div class="mt-1.5 grid w-full min-w-0 grid-cols-[1fr_auto] items-center gap-2 text-xs">
                <span class="min-w-0 truncate text-gray-500 dark:text-dark-300">
                  {{ t('keys.balanceDeductionHint') }}
                </span>
                <span class="action-trigger-text">
                  {{ t('keys.switchAccessSourceShort') }}
                </span>
              </div>
            </button>
          </template>

          <template #cell-group="{ row }">
            <div class="group/dropdown relative">
              <button
                :ref="(el) => setGroupButtonRef(row.id, el)"
                @click="openGroupSelector(row)"
                class="entitlement-card-btn group-select-card"
                :title="t('keys.clickToChangeGroup')"
              >
                <div class="flex w-full items-center gap-2">
                  <span class="badge-tag group-badge">
                    {{ t('keys.currentCardGroupBadge') }}
                  </span>
                  <span
                    v-if="row.group"
                    class="ml-auto text-[11px] font-medium text-gray-500 dark:text-gray-400"
                  >
                    {{ t('keys.rateMultiplierBadge', { rate: formatRateMultiplier(effectiveGroupRate(row.group.id, row.group.rate_multiplier)) }) }}
                  </span>
                </div>
                <div class="mt-1.5 flex max-w-full items-center gap-2">
                  <GroupBadge
                    v-if="row.group"
                    :name="row.group.name"
                    :platform="row.group.platform"
                    :subscription-type="row.group.subscription_type"
                    :rate-multiplier="row.group.rate_multiplier"
                    :user-rate-multiplier="userGroupRates[row.group.id]"
                    :peak-rate-enabled="row.group.peak_rate_enabled"
                    :peak-start="row.group.peak_start"
                    :peak-end="row.group.peak_end"
                    :peak-rate-multiplier="row.group.peak_rate_multiplier"
                    :show-rate="false"
                  />
                  <span v-else class="text-sm text-gray-400 dark:text-dark-500">{{
                    t('keys.noGroup')
                  }}</span>
                </div>
                <div
                  v-if="row.group"
                  class="mt-1.5 grid w-full min-w-0 grid-cols-[1fr_auto] items-center gap-2 text-xs"
                >
                  <span class="min-w-0 truncate font-semibold text-gray-900 dark:text-white">
                    {{ rowGroupActualCostText(row) }}
                  </span>
                  <span class="action-trigger-text">
                    {{ t('keys.changeGroup') }}
                  </span>
                </div>
              </button>
            </div>
          </template>

          <template #cell-current_concurrency="{ value }">
            <span
              :class="[
                'inline-flex min-w-8 items-center justify-center rounded px-2 py-1 text-sm font-semibold tabular-nums',
                (value ?? 0) > 0
                  ? 'bg-emerald-50 text-emerald-700 ring-1 ring-emerald-200 dark:bg-emerald-900/25 dark:text-emerald-300 dark:ring-emerald-800'
                  : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-400'
              ]"
            >
              {{ value ?? 0 }}
            </span>
          </template>

          <template #cell-usage="{ row }">
            <div class="text-sm">
              <div>
                <div class="flex items-center gap-1.5">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('keys.today') }}:</span>
                  <div class="flex flex-col items-start">
                    <span class="font-medium text-gray-900 dark:text-white">
                      {{ formatUsageUSD(apiKeyUsageCost(row, 'today')) }}
                    </span>
                    <span
                      v-if="apiKeyUsageCnyText(row, 'today')"
                      class="text-[10px] text-orange-500/80 dark:text-orange-400/70"
                    >
                      ≈ {{ apiKeyUsageCnyText(row, 'today') }}
                    </span>
                  </div>
                </div>
              </div>
              <div class="mt-1.5">
                <div class="flex items-center gap-1.5">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('keys.total') }}:</span>
                  <div class="flex flex-col items-start">
                    <span class="font-medium text-gray-900 dark:text-white">
                      {{ formatUsageUSD(apiKeyUsageCost(row, 'total')) }}
                    </span>
                    <span
                      v-if="apiKeyUsageCnyText(row, 'total')"
                      class="text-[10px] text-orange-500/80 dark:text-orange-400/70"
                    >
                      ≈ {{ apiKeyUsageCnyText(row, 'total') }}
                    </span>
                  </div>
                </div>
              </div>
              <!-- Quota progress (if quota is set) -->
              <div v-if="row.quota > 0" class="mt-1.5">
                <div class="flex items-center gap-1.5 text-xs">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('keys.quota') }}:</span>
                  <span :class="[
                    'font-medium',
                    row.quota_used >= row.quota ? 'text-red-500' :
                    row.quota_used >= row.quota * 0.8 ? 'text-yellow-500' :
                    'text-gray-900 dark:text-white'
                  ]">
                    ${{ row.quota_used?.toFixed(2) || '0.00' }} / ${{ row.quota?.toFixed(2) }}
                  </span>
                </div>
                <div class="progress-bar-track mt-1">
                  <div
                    class="progress-bar-fill"
                    :class="[
                      row.quota_used >= row.quota ? 'danger' :
                      row.quota_used >= row.quota * 0.8 ? 'warning' :
                      'primary'
                    ]"
                    :style="{ width: Math.min((row.quota_used / row.quota) * 100, 100) + '%' }"
                  />
                </div>
              </div>
            </div>
          </template>

          <template #cell-rate_limit="{ row }">
            <div v-if="row.rate_limit_5h > 0 || row.rate_limit_1d > 0 || row.rate_limit_7d > 0" class="space-y-1.5 min-w-[140px]">
              <!-- 5h window -->
              <div v-if="row.rate_limit_5h > 0">
                <div class="flex items-center justify-between text-[11px]">
                  <span class="text-gray-500 dark:text-gray-400">5h</span>
                  <span :class="[
                    'font-semibold tabular-nums',
                    row.usage_5h >= row.rate_limit_5h ? 'text-red-500' :
                    row.usage_5h >= row.rate_limit_5h * 0.8 ? 'text-yellow-500' :
                    'text-gray-700 dark:text-gray-300'
                  ]">
                    ${{ row.usage_5h?.toFixed(2) || '0.00' }}/${{ row.rate_limit_5h?.toFixed(2) }}
                  </span>
                </div>
                <div class="progress-bar-track thin">
                  <div
                    class="progress-bar-fill"
                    :class="[
                      row.usage_5h >= row.rate_limit_5h ? 'danger' :
                      row.usage_5h >= row.rate_limit_5h * 0.8 ? 'warning' :
                      'success'
                    ]"
                    :style="{ width: Math.min((row.usage_5h / row.rate_limit_5h) * 100, 100) + '%' }"
                  />
                </div>
                <div v-if="row.reset_5h_at && formatResetTime(row.reset_5h_at)" class="text-[10px] text-gray-400 dark:text-gray-500 tabular-nums">
                  ⟳ {{ formatResetTime(row.reset_5h_at) }}
                </div>
              </div>
              <!-- 1d window -->
              <div v-if="row.rate_limit_1d > 0">
                <div class="flex items-center justify-between text-[11px]">
                  <span class="text-gray-500 dark:text-gray-400">1d</span>
                  <span :class="[
                    'font-semibold tabular-nums',
                    row.usage_1d >= row.rate_limit_1d ? 'text-red-500' :
                    row.usage_1d >= row.rate_limit_1d * 0.8 ? 'text-yellow-500' :
                    'text-gray-700 dark:text-gray-300'
                  ]">
                    ${{ row.usage_1d?.toFixed(2) || '0.00' }}/${{ row.rate_limit_1d?.toFixed(2) }}
                  </span>
                </div>
                <div class="progress-bar-track thin">
                  <div
                    class="progress-bar-fill"
                    :class="[
                      row.usage_1d >= row.rate_limit_1d ? 'danger' :
                      row.usage_1d >= row.rate_limit_1d * 0.8 ? 'warning' :
                      'success'
                    ]"
                    :style="{ width: Math.min((row.usage_1d / row.rate_limit_1d) * 100, 100) + '%' }"
                  />
                </div>
                <div v-if="row.reset_1d_at && formatResetTime(row.reset_1d_at)" class="text-[10px] text-gray-400 dark:text-gray-500 tabular-nums">
                  ⟳ {{ formatResetTime(row.reset_1d_at) }}
                </div>
              </div>
              <!-- 7d window -->
              <div v-if="row.rate_limit_7d > 0">
                <div class="flex items-center justify-between text-[11px]">
                  <span class="text-gray-500 dark:text-gray-400">7d</span>
                  <span :class="[
                    'font-semibold tabular-nums',
                    row.usage_7d >= row.rate_limit_7d ? 'text-red-500' :
                    row.usage_7d >= row.rate_limit_7d * 0.8 ? 'text-yellow-500' :
                    'text-gray-700 dark:text-gray-300'
                  ]">
                    ${{ row.usage_7d?.toFixed(2) || '0.00' }}/${{ row.rate_limit_7d?.toFixed(2) }}
                  </span>
                </div>
                <div class="progress-bar-track thin">
                  <div
                    class="progress-bar-fill"
                    :class="[
                      row.usage_7d >= row.rate_limit_7d ? 'danger' :
                      row.usage_7d >= row.rate_limit_7d * 0.8 ? 'warning' :
                      'success'
                    ]"
                    :style="{ width: Math.min((row.usage_7d / row.rate_limit_7d) * 100, 100) + '%' }"
                  />
                </div>
                <div v-if="row.reset_7d_at && formatResetTime(row.reset_7d_at)" class="text-[10px] text-gray-400 dark:text-gray-500 tabular-nums">
                  ⟳ {{ formatResetTime(row.reset_7d_at) }}
                </div>
              </div>
              <!-- Reset button -->
              <button
                v-if="row.usage_5h > 0 || row.usage_1d > 0 || row.usage_7d > 0"
                @click.stop="confirmResetRateLimitFromTable(row)"
                class="mt-0.5 inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-xs text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                :title="t('keys.resetRateLimitUsage')"
              >
                <Icon name="refresh" size="xs" />
                {{ t('keys.resetUsage') }}
              </button>
            </div>
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>

          <template #cell-expires_at="{ value }">
            <span v-if="value" :class="[
              'text-sm',
              new Date(value) < new Date() ? 'text-red-500 dark:text-red-400' : 'text-gray-500 dark:text-dark-400'
            ]">
              {{ formatDateTime(value) }}
            </span>
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">{{ t('keys.noExpiration') }}</span>
          </template>

          <template #cell-status="{ value }">
            <span :class="[
              'badge',
              value === 'active' ? 'badge-success' :
              value === 'quota_exhausted' ? 'badge-warning' :
              value === 'expired' ? 'badge-danger' :
              'badge-gray'
            ]">
              {{ t('keys.status.' + value) }}
            </span>
          </template>

          <template #cell-last_used_at="{ value }">
            <span v-if="value" class="text-sm text-gray-500 dark:text-dark-400">
              {{ formatDateTime(value) }}
            </span>
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>

          <template #cell-last_used_ip="{ value }">
            <span v-if="value" class="text-sm text-gray-500 dark:text-dark-400">
              {{ value }}
            </span>
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatDateTime(value) }}</span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <!-- Use Key Button -->
              <button
                @click="openUseKeyModal(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-green-50 hover:text-green-600 dark:hover:bg-green-900/20 dark:hover:text-green-400"
              >
                <Icon name="terminal" size="sm" />
                <span class="text-xs">{{ t('keys.useKey') }}</span>
              </button>
              <!-- Import to CC Switch Button -->
              <button
                v-if="!publicSettings?.hide_ccs_import_button"
                @click="importToCcswitch(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
              >
                <Icon name="upload" size="sm" />
                <span class="text-xs">{{ t('keys.importToCcSwitch') }}</span>
              </button>
              <!-- Open in LobeHub Button -->
              <button
                v-if="canOpenRowInLobeHub(row)"
                @click="openInLobeHub(row)"
                :disabled="openingLobeHubKeyId === row.id"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-purple-50 hover:text-purple-600 disabled:cursor-not-allowed disabled:opacity-60 dark:hover:bg-purple-900/20 dark:hover:text-purple-400"
              >
                <Icon name="globe" size="sm" :class="openingLobeHubKeyId === row.id ? 'animate-spin' : ''" />
                <span class="text-xs">{{ openingLobeHubKeyId === row.id ? t('keys.openingLobeHub') : t('keys.openInLobeHub') }}</span>
              </button>
              <!-- Toggle Status Button -->
              <button
                @click="toggleKeyStatus(row)"
                :class="[
                  'flex flex-col items-center gap-0.5 rounded-lg p-1.5 transition-colors',
                  row.status === 'active'
                    ? 'text-gray-500 hover:bg-yellow-50 hover:text-yellow-600 dark:hover:bg-yellow-900/20 dark:hover:text-yellow-400'
                    : 'text-gray-500 hover:bg-green-50 hover:text-green-600 dark:hover:bg-green-900/20 dark:hover:text-green-400'
                ]"
              >
                <Icon v-if="row.status === 'active'" name="ban" size="sm" />
                <Icon v-else name="checkCircle" size="sm" />
                <span class="text-xs">{{ row.status === 'active' ? t('keys.disable') : t('keys.enable') }}</span>
              </button>
              <!-- Edit Button -->
              <button
                @click="editKey(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              >
                <Icon name="edit" size="sm" />
                <span class="text-xs">{{ t('common.edit') }}</span>
              </button>
              <!-- Delete Button -->
              <button
                @click="confirmDelete(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              >
                <Icon name="trash" size="sm" />
                <span class="text-xs">{{ t('common.delete') }}</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('keys.noKeysYet')"
              :description="t('keys.createFirstKey')"
              :action-text="t('keys.createKey')"
              @action="handleCreateAction"
            />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>
    </div>

    <!-- Create/Edit Modal -->
    <BaseDialog
      :show="showCreateModal || showEditModal"
      :title="showEditModal ? t('keys.editKey') : t('keys.createKey')"
      width="normal"
      @close="closeModals"
    >
      <form id="key-form" @submit.prevent="handleSubmit" class="space-y-5">
        <div>
          <label class="input-label">{{ t('keys.nameLabel') }}</label>
          <input
            v-model="formData.name"
            type="text"
            required
            class="input"
            :placeholder="t('keys.namePlaceholder')"
            data-tour="key-form-name"
          />
        </div>

        <div class="space-y-2" data-testid="access-source-selector">
          <label class="input-label">{{ t('keys.accessSourceLabel') }}</label>
          <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <button
              type="button"
              class="access-source-card balance order-2"
              :class="{ active: formData.access_source === 'balance' }"
              data-testid="access-source-balance"
              @click="selectAccessSource('balance')"
            >
              <Icon name="dollar" size="sm" class="mt-0.5 shrink-0 icon" />
              <span class="min-w-0">
                <span class="block text-sm font-semibold">{{ t('keys.accessSourceBalance') }}</span>
                <span class="mt-0.5 block text-xs desc-text">
                  {{ t('keys.accessSourceBalanceHint') }}
                </span>
              </span>
            </button>
            <button
              type="button"
              class="access-source-card entitlement order-1"
              :class="{ active: formData.access_source === 'entitlement' }"
              :disabled="availableEntitlements.length === 0"
              data-testid="access-source-entitlement"
              @click="selectAccessSource('entitlement')"
            >
              <Icon name="badge" size="sm" class="mt-0.5 shrink-0 icon" />
              <span class="min-w-0">
                <span class="block text-sm font-semibold">{{ t('keys.accessSourceEntitlement') }}</span>
                <span class="mt-0.5 block text-xs desc-text">
                  {{ availableEntitlements.length > 0 ? t('keys.accessSourceEntitlementHint') : t('keys.accessSourceNoEntitlementHint') }}
                </span>
              </span>
            </button>
          </div>
        </div>

        <div
          v-if="formData.access_source === 'entitlement' && entitlementOptions.length > 0"
          class="rounded-lg border border-primary-100 bg-primary-50/60 p-3 dark:border-primary-900/40 dark:bg-primary-900/10"
          data-testid="entitlement-selector"
        >
          <label class="input-label mb-1">{{ t('keys.entitlementLabel') }}</label>
          <Select
            v-if="entitlementOptions.length > 1"
            v-model="formData.subscription_entitlement_id"
            :options="entitlementOptions"
            :placeholder="t('keys.entitlementSelectPlaceholder')"
            :searchable="true"
            data-testid="entitlement-select"
          >
            <template #selected="{ option }">
              <span v-if="option" class="flex min-w-0 items-center justify-between gap-2">
                <span class="min-w-0 truncate text-sm font-medium text-gray-900 dark:text-white">
                  {{ entitlementOptionLabel(option) }}
                </span>
                <span
                  :class="[
                    'shrink-0 rounded-full px-2 py-0.5 text-[11px] font-semibold',
                    entitlementOptionQuotaPillClass(option)
                  ]"
                >
                  {{ entitlementOptionQuotaCompactText(option) }}
                </span>
              </span>
              <span v-else class="text-gray-400">{{ t('keys.entitlementSelectPlaceholder') }}</span>
            </template>
            <template #option="{ option, selected }">
              <div class="w-full">
                <div class="flex w-full items-start justify-between gap-3">
                  <div class="min-w-0 flex-1 text-left">
                    <div class="flex min-w-0 items-center gap-2">
                      <span class="inline-flex shrink-0 items-center gap-1 rounded-md bg-primary-50 px-2 py-0.5 text-[11px] font-semibold text-primary-700 ring-1 ring-primary-200 dark:bg-primary-900/20 dark:text-primary-300 dark:ring-primary-800/70">
                        <Icon name="badge" size="xs" />
                        {{ t('keys.planCardBadge') }}
                      </span>
                      <span class="min-w-0 truncate text-sm font-semibold text-gray-900 dark:text-white">
                        {{ entitlementOptionLabel(option) }}
                      </span>
                    </div>
                    <div
                      v-if="entitlementOptionDescription(option)"
                      class="mt-1 text-xs text-gray-500 dark:text-gray-400"
                    >
                      {{ entitlementOptionDescription(option) }}
                    </div>
                  </div>
                  <div class="flex shrink-0 items-center gap-2 pt-0.5">
                    <span
                      :class="[
                        'inline-flex items-center whitespace-nowrap rounded-full px-3 py-1 text-xs font-semibold',
                        entitlementOptionQuotaPillClass(option)
                      ]"
                    >
                      {{ entitlementOptionQuotaCompactText(option) }}
                    </span>
                    <Icon
                      v-if="selected"
                      name="check"
                      size="sm"
                      class="shrink-0 text-primary-500"
                    />
                  </div>
                </div>
              </div>
            </template>
          </Select>
          <div
            v-else
            class="rounded-lg border border-primary-100 bg-white/70 p-3 text-sm text-gray-700 dark:border-primary-900/40 dark:bg-dark-800/70 dark:text-gray-300"
            data-testid="entitlement-auto-selected"
          >
            <div class="flex min-w-0 items-start justify-between gap-3">
              <div class="min-w-0">
                <div class="flex min-w-0 items-center gap-2">
                  <span class="inline-flex shrink-0 items-center gap-1 rounded-md bg-primary-50 px-2 py-0.5 text-[11px] font-semibold text-primary-700 ring-1 ring-primary-200 dark:bg-primary-900/20 dark:text-primary-300 dark:ring-primary-800/70">
                    <Icon name="badge" size="xs" />
                    {{ t('keys.entitlementAutoSelected') }}
                  </span>
                  <span class="min-w-0 truncate font-semibold text-gray-900 dark:text-white">
                    {{ formatEntitlementLabel(entitlementOptions[0].entitlement) }}
                  </span>
                </div>
                <div
                  v-if="formatEntitlementDescription(entitlementOptions[0].entitlement)"
                  class="mt-1 text-xs text-gray-500 dark:text-gray-400"
                >
                  {{ formatEntitlementDescription(entitlementOptions[0].entitlement) }}
                </div>
              </div>
              <span
                :class="[
                  'shrink-0 rounded-full px-3 py-1 text-xs font-semibold',
                  entitlementQuotaPillClass(entitlementOptions[0].entitlement)
                ]"
              >
                {{ entitlementQuotaCompactText(entitlementOptions[0].entitlement) }}
              </span>
            </div>
          </div>
          <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
            {{ formData.subscription_entitlement_id ? t('keys.entitlementSelectedHint') : t('keys.entitlementChooseFirstHint') }}
          </p>
        </div>

        <div>
          <label class="input-label">{{ t('keys.currentGroupLabel') }}</label>
          <Select
            v-model="formData.group_id"
            :options="formGroupOptions"
            :placeholder="t('keys.selectGroup')"
            :searchable="true"
            :search-placeholder="t('keys.searchGroup')"
            data-tour="key-form-group"
          >
            <template #selected="{ option }">
              <GroupBadge
                v-if="option"
                :name="(option as unknown as GroupOption).label"
                :platform="(option as unknown as GroupOption).platform"
                :subscription-type="(option as unknown as GroupOption).subscriptionType"
                :rate-multiplier="(option as unknown as GroupOption).rate"
                :user-rate-multiplier="(option as unknown as GroupOption).userRate"
                :peak-rate-enabled="(option as unknown as GroupOption).peakRateEnabled"
                :peak-start="(option as unknown as GroupOption).peakStart"
                :peak-end="(option as unknown as GroupOption).peakEnd"
                :peak-rate-multiplier="(option as unknown as GroupOption).peakRateMultiplier"
                :always-show-rate="true"
              />
              <span v-else class="text-gray-400">{{ t('keys.selectGroup') }}</span>
            </template>
            <template #option="{ option, selected }">
              <div class="w-full">
                <GroupOptionItem
                  :name="(option as unknown as GroupOption).label"
                  :platform="(option as unknown as GroupOption).platform"
                  :subscription-type="(option as unknown as GroupOption).subscriptionType"
                  :rate-multiplier="(option as unknown as GroupOption).rate"
                  :user-rate-multiplier="(option as unknown as GroupOption).userRate"
                  :peak-rate-enabled="(option as unknown as GroupOption).peakRateEnabled"
                  :peak-start="(option as unknown as GroupOption).peakStart"
                  :peak-end="(option as unknown as GroupOption).peakEnd"
                  :peak-rate-multiplier="(option as unknown as GroupOption).peakRateMultiplier"
                  :description="(option as unknown as GroupOption).description"
                  :selected="selected"
                />
                <div class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-left text-xs">
                  <span
                    v-if="groupEntitlementScopeText(option as unknown as GroupOption, formData.subscription_entitlement_id)"
                    class="text-primary-600 dark:text-primary-400"
                  >
                    {{ groupEntitlementScopeText(option as unknown as GroupOption, formData.subscription_entitlement_id) }}
                  </span>
                  <span class="font-medium text-gray-600 dark:text-gray-300">
                    {{ groupOptionActualCostText(option as unknown as GroupOption, formData.subscription_entitlement_id) }}
                  </span>
                </div>
              </div>
            </template>
          </Select>
          <p v-if="selectedGroupOption" class="input-hint">
            {{ groupOptionActualCostText(selectedGroupOption, formData.subscription_entitlement_id) }}
            ·
            {{ t('keys.currentGroupRateHint', { rate: formatRateMultiplier(effectiveGroupOptionRate(selectedGroupOption)) }) }}
          </p>
          <p v-if="formData.subscription_entitlement_id" class="input-hint">
            {{ t('keys.currentGroupFilteredHint') }}
          </p>
        </div>

        <!-- Custom Key Section (only for create) -->
        <div v-if="!showEditModal" class="space-y-3">
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{ t('keys.customKeyLabel') }}</label>
            <button
              type="button"
              @click="formData.use_custom_key = !formData.use_custom_key"
              :class="[
                'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                formData.use_custom_key ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  formData.use_custom_key ? 'translate-x-4' : 'translate-x-0'
                ]"
              />
            </button>
          </div>
          <div v-if="formData.use_custom_key">
            <input
              v-model="formData.custom_key"
              type="text"
              class="input font-mono"
              :placeholder="t('keys.customKeyPlaceholder')"
              :class="{ 'border-red-500 dark:border-red-500': customKeyError }"
            />
            <p v-if="customKeyError" class="mt-1 text-sm text-red-500">{{ customKeyError }}</p>
            <p v-else class="input-hint">{{ t('keys.customKeyHint') }}</p>
          </div>
        </div>

        <div v-if="showEditModal">
          <label class="input-label">{{ t('keys.statusLabel') }}</label>
          <Select
            v-model="formData.status"
            :options="statusOptions"
            :placeholder="t('keys.selectStatus')"
          />
        </div>

        <!-- IP Restriction Section -->
        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{ t('keys.ipRestriction') }}</label>
            <button
              type="button"
              @click="formData.enable_ip_restriction = !formData.enable_ip_restriction"
              :class="[
                'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                formData.enable_ip_restriction ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  formData.enable_ip_restriction ? 'translate-x-4' : 'translate-x-0'
                ]"
              />
            </button>
          </div>

          <div v-if="formData.enable_ip_restriction" class="space-y-4 pt-2">
            <div>
              <label class="input-label">{{ t('keys.ipWhitelist') }}</label>
              <textarea
                v-model="formData.ip_whitelist"
                rows="3"
                class="input font-mono text-sm"
                :placeholder="t('keys.ipWhitelistPlaceholder')"
              />
              <p class="input-hint">{{ t('keys.ipWhitelistHint') }}</p>
            </div>

            <div>
              <label class="input-label">{{ t('keys.ipBlacklist') }}</label>
              <textarea
                v-model="formData.ip_blacklist"
                rows="3"
                class="input font-mono text-sm"
                :placeholder="t('keys.ipBlacklistPlaceholder')"
              />
              <p class="input-hint">{{ t('keys.ipBlacklistHint') }}</p>
            </div>
          </div>
        </div>

        <!-- Quota Limit Section -->
        <div class="space-y-3">
          <label class="input-label">{{ t('keys.quotaLimit') }}</label>
          <!-- Switch commented out - always show input, 0 = unlimited
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{ t('keys.quotaLimit') }}</label>
            <button
              type="button"
              @click="formData.enable_quota = !formData.enable_quota"
              :class="[
                'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                formData.enable_quota ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  formData.enable_quota ? 'translate-x-4' : 'translate-x-0'
                ]"
              />
            </button>
          </div>
          -->

          <div class="space-y-4">
            <div>
              <div class="relative">
                <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500">$</span>
                <input
                  v-model.number="formData.quota"
                  type="number"
                  step="0.01"
                  min="0"
                  class="input pl-7"
                  :placeholder="t('keys.quotaAmountPlaceholder')"
                />
              </div>
              <p class="input-hint">{{ t('keys.quotaAmountHint') }}</p>
            </div>

            <!-- Quota used display (only in edit mode) -->
            <div v-if="showEditModal && selectedKey && selectedKey.quota > 0">
              <label class="input-label">{{ t('keys.quotaUsed') }}</label>
              <div class="flex items-center gap-2">
                <div class="flex-1 rounded-lg bg-gray-100 px-3 py-2 dark:bg-dark-700">
                  <span class="font-medium text-gray-900 dark:text-white">
                    ${{ selectedKey.quota_used?.toFixed(4) || '0.0000' }}
                  </span>
                  <span class="mx-2 text-gray-400">/</span>
                  <span class="text-gray-500 dark:text-gray-400">
                    ${{ selectedKey.quota?.toFixed(2) || '0.00' }}
                  </span>
                </div>
                <button
                  type="button"
                  @click="confirmResetQuota"
                  class="btn btn-secondary text-sm"
                  :title="t('keys.resetQuotaUsed')"
                >
                  {{ t('keys.reset') }}
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- Rate Limit Section -->
        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{ t('keys.rateLimitSection') }}</label>
            <button
              type="button"
              @click="formData.enable_rate_limit = !formData.enable_rate_limit"
              :class="[
                'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                formData.enable_rate_limit ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  formData.enable_rate_limit ? 'translate-x-4' : 'translate-x-0'
                ]"
              />
            </button>
          </div>

          <div v-if="formData.enable_rate_limit" class="space-y-4 pt-2">
            <p class="input-hint -mt-2">{{ t('keys.rateLimitHint') }}</p>
            <!-- 5-Hour Limit -->
            <div>
              <label class="input-label">{{ t('keys.rateLimit5h') }}</label>
              <div class="relative">
                <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500">$</span>
                <input
                  v-model.number="formData.rate_limit_5h"
                  type="number"
                  step="0.01"
                  min="0"
                  class="input pl-7"
                  :placeholder="'0'"
                />
              </div>
              <!-- Usage info (edit mode only) -->
              <div v-if="showEditModal && selectedKey && selectedKey.rate_limit_5h > 0" class="mt-2">
                <div class="flex items-center gap-2">
                  <div class="flex-1 rounded-lg bg-gray-100 px-3 py-2 dark:bg-dark-700 text-sm">
                    <span :class="[
                      'font-medium',
                      selectedKey.usage_5h >= selectedKey.rate_limit_5h ? 'text-red-500' :
                      selectedKey.usage_5h >= selectedKey.rate_limit_5h * 0.8 ? 'text-yellow-500' :
                      'text-gray-900 dark:text-white'
                    ]">
                      ${{ selectedKey.usage_5h?.toFixed(4) || '0.0000' }}
                    </span>
                    <span class="mx-2 text-gray-400">/</span>
                    <span class="text-gray-500 dark:text-gray-400">
                      ${{ selectedKey.rate_limit_5h?.toFixed(2) || '0.00' }}
                    </span>
                  </div>
                </div>
                <div class="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                  <div
                    :class="[
                      'h-full rounded-full transition-all',
                      selectedKey.usage_5h >= selectedKey.rate_limit_5h ? 'bg-red-500' :
                      selectedKey.usage_5h >= selectedKey.rate_limit_5h * 0.8 ? 'bg-yellow-500' :
                      'bg-green-500'
                    ]"
                    :style="{ width: Math.min((selectedKey.usage_5h / selectedKey.rate_limit_5h) * 100, 100) + '%' }"
                  />
                </div>
              </div>
            </div>

            <!-- Daily Limit -->
            <div>
              <label class="input-label">{{ t('keys.rateLimit1d') }}</label>
              <div class="relative">
                <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500">$</span>
                <input
                  v-model.number="formData.rate_limit_1d"
                  type="number"
                  step="0.01"
                  min="0"
                  class="input pl-7"
                  :placeholder="'0'"
                />
              </div>
              <!-- Usage info (edit mode only) -->
              <div v-if="showEditModal && selectedKey && selectedKey.rate_limit_1d > 0" class="mt-2">
                <div class="flex items-center gap-2">
                  <div class="flex-1 rounded-lg bg-gray-100 px-3 py-2 dark:bg-dark-700 text-sm">
                    <span :class="[
                      'font-medium',
                      selectedKey.usage_1d >= selectedKey.rate_limit_1d ? 'text-red-500' :
                      selectedKey.usage_1d >= selectedKey.rate_limit_1d * 0.8 ? 'text-yellow-500' :
                      'text-gray-900 dark:text-white'
                    ]">
                      ${{ selectedKey.usage_1d?.toFixed(4) || '0.0000' }}
                    </span>
                    <span class="mx-2 text-gray-400">/</span>
                    <span class="text-gray-500 dark:text-gray-400">
                      ${{ selectedKey.rate_limit_1d?.toFixed(2) || '0.00' }}
                    </span>
                  </div>
                </div>
                <div class="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                  <div
                    :class="[
                      'h-full rounded-full transition-all',
                      selectedKey.usage_1d >= selectedKey.rate_limit_1d ? 'bg-red-500' :
                      selectedKey.usage_1d >= selectedKey.rate_limit_1d * 0.8 ? 'bg-yellow-500' :
                      'bg-green-500'
                    ]"
                    :style="{ width: Math.min((selectedKey.usage_1d / selectedKey.rate_limit_1d) * 100, 100) + '%' }"
                  />
                </div>
              </div>
            </div>

            <!-- 7-Day Limit -->
            <div>
              <label class="input-label">{{ t('keys.rateLimit7d') }}</label>
              <div class="relative">
                <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500">$</span>
                <input
                  v-model.number="formData.rate_limit_7d"
                  type="number"
                  step="0.01"
                  min="0"
                  class="input pl-7"
                  :placeholder="'0'"
                />
              </div>
              <!-- Usage info (edit mode only) -->
              <div v-if="showEditModal && selectedKey && selectedKey.rate_limit_7d > 0" class="mt-2">
                <div class="flex items-center gap-2">
                  <div class="flex-1 rounded-lg bg-gray-100 px-3 py-2 dark:bg-dark-700 text-sm">
                    <span :class="[
                      'font-medium',
                      selectedKey.usage_7d >= selectedKey.rate_limit_7d ? 'text-red-500' :
                      selectedKey.usage_7d >= selectedKey.rate_limit_7d * 0.8 ? 'text-yellow-500' :
                      'text-gray-900 dark:text-white'
                    ]">
                      ${{ selectedKey.usage_7d?.toFixed(4) || '0.0000' }}
                    </span>
                    <span class="mx-2 text-gray-400">/</span>
                    <span class="text-gray-500 dark:text-gray-400">
                      ${{ selectedKey.rate_limit_7d?.toFixed(2) || '0.00' }}
                    </span>
                  </div>
                </div>
                <div class="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                  <div
                    :class="[
                      'h-full rounded-full transition-all',
                      selectedKey.usage_7d >= selectedKey.rate_limit_7d ? 'bg-red-500' :
                      selectedKey.usage_7d >= selectedKey.rate_limit_7d * 0.8 ? 'bg-yellow-500' :
                      'bg-green-500'
                    ]"
                    :style="{ width: Math.min((selectedKey.usage_7d / selectedKey.rate_limit_7d) * 100, 100) + '%' }"
                  />
                </div>
              </div>
            </div>

            <!-- Reset Rate Limit button (edit mode only) -->
            <div v-if="showEditModal && selectedKey && (selectedKey.rate_limit_5h > 0 || selectedKey.rate_limit_1d > 0 || selectedKey.rate_limit_7d > 0)">
              <button
                type="button"
                @click="confirmResetRateLimit"
                class="btn btn-secondary text-sm"
              >
                {{ t('keys.resetRateLimitUsage') }}
              </button>
            </div>
          </div>
        </div>

        <!-- Expiration Section -->
        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{ t('keys.expiration') }}</label>
            <button
              type="button"
              @click="formData.enable_expiration = !formData.enable_expiration"
              :class="[
                'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                formData.enable_expiration ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  formData.enable_expiration ? 'translate-x-4' : 'translate-x-0'
                ]"
              />
            </button>
          </div>

          <div v-if="formData.enable_expiration" class="space-y-4 pt-2">
            <!-- Quick select buttons (for both create and edit mode) -->
            <div class="flex flex-wrap gap-2">
              <button
                v-for="days in ['7', '30', '90']"
                :key="days"
                type="button"
                @click="setExpirationDays(parseInt(days))"
                :class="[
                  'rounded-lg px-3 py-1.5 text-sm transition-colors',
                  formData.expiration_preset === days
                    ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-400 dark:hover:bg-dark-600'
                ]"
              >
                {{ showEditModal ? t('keys.extendDays', { days }) : t('keys.expiresInDays', { days }) }}
              </button>
              <button
                type="button"
                @click="formData.expiration_preset = 'custom'"
                :class="[
                  'rounded-lg px-3 py-1.5 text-sm transition-colors',
                  formData.expiration_preset === 'custom'
                    ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-400 dark:hover:bg-dark-600'
                ]"
              >
                {{ t('keys.customDate') }}
              </button>
            </div>

            <!-- Date picker (always show for precise adjustment) -->
            <div>
              <label class="input-label">{{ t('keys.expirationDate') }}</label>
              <input
                v-model="formData.expiration_date"
                type="datetime-local"
                class="input"
              />
              <p class="input-hint">{{ t('keys.expirationDateHint') }}</p>
            </div>

            <!-- Current expiration display (only in edit mode) -->
            <div v-if="showEditModal && selectedKey?.expires_at" class="text-sm">
              <span class="text-gray-500 dark:text-gray-400">{{ t('keys.currentExpiration') }}: </span>
              <span class="font-medium text-gray-900 dark:text-white">
                {{ formatDateTime(selectedKey.expires_at) }}
              </span>
            </div>
          </div>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeModals" type="button" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
          <button
            form="key-form"
            type="submit"
            :disabled="submitting"
            class="btn btn-primary"
            data-tour="key-form-submit"
          >
            <svg
              v-if="submitting"
              class="-ml-1 mr-2 h-4 w-4 animate-spin"
              fill="none"
              viewBox="0 0 24 24"
            >
              <circle
                class="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                stroke-width="4"
              ></circle>
              <path
                class="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
              ></path>
            </svg>
            {{
              submitting
                ? t('keys.saving')
                : showEditModal
                  ? t('common.update')
                  : t('common.create')
            }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Delete Confirmation Dialog -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('keys.deleteKey')"
      :message="t('keys.deleteConfirmMessage', { name: selectedKey?.name })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="handleDelete"
      @cancel="showDeleteDialog = false"
    />

    <!-- Reset Quota Confirmation Dialog -->
    <ConfirmDialog
      :show="showResetQuotaDialog"
      :title="t('keys.resetQuotaTitle')"
      :message="t('keys.resetQuotaConfirmMessage', { name: selectedKey?.name, used: selectedKey?.quota_used?.toFixed(4) })"
      :confirm-text="t('keys.reset')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="resetQuotaUsed"
      @cancel="showResetQuotaDialog = false"
    />

    <!-- Reset Rate Limit Confirmation Dialog -->
    <ConfirmDialog
      :show="showResetRateLimitDialog"
      :title="t('keys.resetRateLimitTitle')"
      :message="t('keys.resetRateLimitConfirmMessage', { name: selectedKey?.name })"
      :confirm-text="t('keys.reset')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="resetRateLimitUsage"
      @cancel="showResetRateLimitDialog = false"
    />

    <!-- Use Key Modal -->
    <UseKeyModal
      :show="showUseKeyModal"
      :api-key="selectedKey?.key || ''"
      :base-url="publicSettings?.api_base_url || ''"
      :platform="selectedKey?.group?.platform || null"
      :allow-messages-dispatch="selectedKey?.group?.allow_messages_dispatch || false"
      @close="closeUseKeyModal"
    />

    <!-- CCS Client Selection Dialog for Antigravity -->
    <BaseDialog
      :show="showCcsClientSelect"
      :title="t('keys.ccsClientSelect.title')"
      width="narrow"
      @close="closeCcsClientSelect"
    >
      <div class="space-y-4">
        <p class="text-sm text-gray-600 dark:text-gray-400">
          {{ t('keys.ccsClientSelect.description') }}
	        </p>
	        <div class="grid grid-cols-2 gap-3">
	          <button
	            @click="handleCcsClientSelect('claude')"
	            class="flex flex-col items-center gap-2 p-4 rounded-xl border-2 border-gray-200 dark:border-dark-600 hover:border-primary-500 dark:hover:border-primary-500 hover:bg-primary-50 dark:hover:bg-primary-900/20 transition-all"
	          >
	            <Icon name="terminal" size="xl" class="text-gray-600 dark:text-gray-400" />
	            <span class="font-medium text-gray-900 dark:text-white">{{
	              t('keys.ccsClientSelect.claudeCode')
	            }}</span>
	            <span class="text-xs text-gray-500 dark:text-gray-400">{{
	              t('keys.ccsClientSelect.claudeCodeDesc')
	            }}</span>
	          </button>
	          <button
	            @click="handleCcsClientSelect('gemini')"
	            class="flex flex-col items-center gap-2 p-4 rounded-xl border-2 border-gray-200 dark:border-dark-600 hover:border-primary-500 dark:hover:border-primary-500 hover:bg-primary-50 dark:hover:bg-primary-900/20 transition-all"
	          >
	            <Icon name="sparkles" size="xl" class="text-gray-600 dark:text-gray-400" />
	            <span class="font-medium text-gray-900 dark:text-white">{{
	              t('keys.ccsClientSelect.geminiCli')
	            }}</span>
	            <span class="text-xs text-gray-500 dark:text-gray-400">{{
	              t('keys.ccsClientSelect.geminiCliDesc')
	            }}</span>
	          </button>
	        </div>
	      </div>
      <template #footer>
        <div class="flex justify-end">
          <button @click="closeCcsClientSelect" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Group Selector Dropdown (Teleported to body to avoid overflow clipping) -->
    <Teleport to="body">
      <div
        v-if="groupSelectorKeyId !== null && dropdownPosition"
        ref="dropdownRef"
        class="animate-in fade-in slide-in-from-top-2 fixed z-[100000020] w-max min-w-[380px] overflow-hidden rounded-xl bg-white shadow-lg ring-1 ring-black/5 duration-200 dark:bg-dark-800 dark:ring-white/10"
        style="pointer-events: auto !important;"
        :style="{
          top: dropdownPosition.top !== undefined ? dropdownPosition.top + 'px' : undefined,
          bottom: dropdownPosition.bottom !== undefined ? dropdownPosition.bottom + 'px' : undefined,
          left: dropdownPosition.left + 'px'
        }"
      >
        <!-- Search box -->
        <div class="border-b border-gray-100 p-2 dark:border-dark-700">
          <div class="relative">
            <svg class="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
            <input
              v-model="groupSearchQuery"
              type="text"
              class="w-full rounded-lg border border-gray-200 bg-gray-50 py-1.5 pl-8 pr-3 text-sm text-gray-900 placeholder-gray-400 outline-none focus:border-primary-300 focus:ring-1 focus:ring-primary-300 dark:border-dark-600 dark:bg-dark-700 dark:text-white dark:placeholder-gray-500 dark:focus:border-primary-600 dark:focus:ring-primary-600"
              :placeholder="t('keys.searchGroup')"
              @click.stop
            />
          </div>
        </div>
        <!-- Group list -->
        <div class="max-h-80 overflow-y-auto p-1.5">
          <button
            v-for="option in filteredGroupOptions"
            :key="option.value ?? 'null'"
            @click="changeGroup(selectedKeyForGroup!, option.value)"
            :class="[
              'flex w-full items-center justify-between rounded-lg px-3 py-2.5 text-sm transition-colors',
              'border-b border-gray-100 last:border-0 dark:border-dark-700',
              selectedKeyForGroup?.group_id === option.value ||
              (!selectedKeyForGroup?.group_id && option.value === null)
                ? 'bg-primary-50 dark:bg-primary-900/20'
                : 'hover:bg-gray-100 dark:hover:bg-dark-700'
            ]"
            :title="option.description || undefined"
          >
            <div class="w-full">
              <GroupOptionItem
                :name="option.label"
                :platform="option.platform"
                :subscription-type="option.subscriptionType"
                :rate-multiplier="option.rate"
                :user-rate-multiplier="option.userRate"
                :peak-rate-enabled="option.peakRateEnabled"
                :peak-start="option.peakStart"
                :peak-end="option.peakEnd"
                :peak-rate-multiplier="option.peakRateMultiplier"
                :description="option.description"
                :selected="
                  selectedKeyForGroup?.group_id === option.value ||
                  (!selectedKeyForGroup?.group_id && option.value === null)
                "
              />
              <div
                v-if="groupEntitlementScopeText(option, selectedKeyForGroup?.subscription_entitlement_id, selectedKeyForGroup ? rowAccessSource(selectedKeyForGroup) : undefined)"
                class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-left text-xs"
              >
                <span class="text-primary-600 dark:text-primary-400">
                  {{ groupEntitlementScopeText(option, selectedKeyForGroup?.subscription_entitlement_id, selectedKeyForGroup ? rowAccessSource(selectedKeyForGroup) : undefined) }}
                </span>
                <span class="font-medium text-gray-600 dark:text-gray-300">
                  {{ groupOptionActualCostText(option, selectedKeyForGroup?.subscription_entitlement_id, selectedKeyForGroup ? rowAccessSource(selectedKeyForGroup) : undefined) }}
                </span>
              </div>
            </div>
          </button>
          <!-- Empty state when search has no results -->
          <div v-if="filteredGroupOptions.length === 0" class="py-4 text-center text-sm text-gray-400 dark:text-gray-500">
            {{ t('keys.noGroupFound') }}
          </div>
        </div>
      </div>
    </Teleport>
  </AppLayout>
</template>

<script setup lang="ts">
	import { ref, reactive, computed, onMounted, onUnmounted, watch, type ComponentPublicInstance } from 'vue'
	import { useRoute, useRouter } from 'vue-router'
	import { useI18n } from 'vue-i18n'
	import { useAppStore } from '@/stores/app'
	import { useOnboardingStore } from '@/stores/onboarding'
	import { useClipboard } from '@/composables/useClipboard'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
import { keysAPI, authAPI, usageAPI, userGroupsAPI, lobehubAPI } from '@/api'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
	import DataTable from '@/components/common/DataTable.vue'
	import Pagination from '@/components/common/Pagination.vue'
	import BaseDialog from '@/components/common/BaseDialog.vue'
	import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
	import EmptyState from '@/components/common/EmptyState.vue'
	import Select from '@/components/common/Select.vue'
	import SearchInput from '@/components/common/SearchInput.vue'
	import Icon from '@/components/icons/Icon.vue'
	import UseKeyModal from '@/components/keys/UseKeyModal.vue'
	import EndpointPopover from '@/components/keys/EndpointPopover.vue'
	import GroupBadge from '@/components/common/GroupBadge.vue'
	import GroupOptionItem from '@/components/common/GroupOptionItem.vue'
	import type { ApiKey, AvailableGroup, AvailableGroupAccessSource, AvailableGroupEntitlement, CreateApiKeyRequest, PublicSettings, SubscriptionType, GroupPlatform, UpdateApiKeyRequest } from '@/types'
import type { Column } from '@/components/common/types'
import type { BatchApiKeyUsageStats } from '@/api/usage'
import { formatDateTime } from '@/utils/format'
import { maskApiKey } from '@/utils/maskApiKey'
import {
  buildCcSwitchImportDeeplink,
  type CcSwitchClientType
} from '@/utils/ccswitchImport'
import { useCurrencyResolver } from '@/composables/useCurrencyResolver'
import { sortGroupsForDisplay } from '@/utils/groupDisplayOrder'
const { convertUsdToCnyForLog, formatCny } = useCurrencyResolver()
// Helper to format date for datetime-local input
const formatDateTimeLocal = (isoDate: string): string => {
  const date = new Date(isoDate)
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

interface GroupOption {
  value: number
  label: string
  description: string | null
  rate: number
  userRate: number | null
  peakRateEnabled: boolean
  peakStart: string
  peakEnd: string
  peakRateMultiplier: number
  sortOrder: number | null
  subscriptionType: SubscriptionType
  balanceEnabled?: boolean
  subscriptionEnabled?: boolean
  planAutoGrantEnabled?: boolean
  platform: GroupPlatform
  entitlements: AvailableGroupEntitlement[]
  accessSources: AvailableGroupAccessSource[]
}

type AccessSource = 'balance' | 'entitlement'

interface EntitlementSelectOption extends Record<string, unknown> {
  value: number
  label: string
  description: string
  entitlement: AvailableGroupEntitlement
}

const appStore = useAppStore()
const onboardingStore = useOnboardingStore()
const { copyToClipboard: clipboardCopy } = useClipboard()

const allColumns = computed<Column[]>(() => [
  { key: 'name', label: t('common.name'), sortable: true },
  { key: 'id', label: t('keys.id'), sortable: true },
  { key: 'key', label: t('keys.apiKey'), sortable: false },
  { key: 'entitlement', label: t('keys.accessSourceColumn'), sortable: false },
  { key: 'group', label: t('keys.currentGroupLabel'), sortable: false },
  { key: 'current_concurrency', label: t('keys.currentConcurrency'), sortable: true },
  { key: 'usage', label: t('keys.usage'), sortable: false },
  { key: 'rate_limit', label: t('keys.rateLimitColumn'), sortable: false },
  { key: 'expires_at', label: t('keys.expiresAt'), sortable: true },
  { key: 'status', label: t('common.status'), sortable: true },
  { key: 'last_used_at', label: t('keys.lastUsedAt'), sortable: true },
  { key: 'last_used_ip', label: t('keys.lastUsedIP'), sortable: false },
  { key: 'created_at', label: t('keys.created'), sortable: true },
  { key: 'actions', label: t('common.actions'), sortable: false }
])

const ALWAYS_VISIBLE_COLUMNS = new Set(['name', 'actions'])
const DEFAULT_HIDDEN_COLUMNS = ['id', 'rate_limit', 'last_used_at', 'last_used_ip']
const HIDDEN_COLUMNS_KEY = 'api-key-hidden-columns'
const COLUMN_SETTINGS_VERSION_KEY = 'api-key-column-settings-version'
const COLUMN_SETTINGS_VERSION = 3
const VERSION_NEW_HIDDEN_COLUMNS: Record<number, string[]> = {
  2: ['last_used_ip'],
  3: ['id']
}

const toggleableColumns = computed(() =>
  allColumns.value.filter((col) => !ALWAYS_VISIBLE_COLUMNS.has(col.key))
)

const hiddenColumns = reactive<Set<string>>(new Set())

const saveColumnsToStorage = () => {
  try {
    localStorage.setItem(HIDDEN_COLUMNS_KEY, JSON.stringify([...hiddenColumns]))
    localStorage.setItem(COLUMN_SETTINGS_VERSION_KEY, String(COLUMN_SETTINGS_VERSION))
  } catch (error) {
    console.error('Failed to save API key table columns:', error)
  }
}

const loadSavedColumns = () => {
  hiddenColumns.clear()
  try {
    const saved = localStorage.getItem(HIDDEN_COLUMNS_KEY)
    if (saved) {
      const parsed = JSON.parse(saved) as string[]
      const validColumnKeys = new Set(allColumns.value.map((col) => col.key))
      parsed
        .filter((key) =>
          typeof key === 'string' &&
          validColumnKeys.has(key) &&
          !ALWAYS_VISIBLE_COLUMNS.has(key)
        )
        .forEach((key) => hiddenColumns.add(key))
      const storedVersion = Number(localStorage.getItem(COLUMN_SETTINGS_VERSION_KEY) ?? '1')
      if (storedVersion < COLUMN_SETTINGS_VERSION) {
        for (let v = storedVersion + 1; v <= COLUMN_SETTINGS_VERSION; v++) {
          for (const key of VERSION_NEW_HIDDEN_COLUMNS[v] ?? []) {
            if (validColumnKeys.has(key) && !ALWAYS_VISIBLE_COLUMNS.has(key)) {
              hiddenColumns.add(key)
            }
          }
        }
        saveColumnsToStorage()
      } else {
        localStorage.setItem(COLUMN_SETTINGS_VERSION_KEY, String(COLUMN_SETTINGS_VERSION))
      }
    } else {
      DEFAULT_HIDDEN_COLUMNS.forEach((key) => hiddenColumns.add(key))
      localStorage.setItem(COLUMN_SETTINGS_VERSION_KEY, String(COLUMN_SETTINGS_VERSION))
    }
  } catch (error) {
    console.error('Failed to load API key table columns:', error)
    DEFAULT_HIDDEN_COLUMNS.forEach((key) => hiddenColumns.add(key))
  }
}

const toggleColumn = (key: string) => {
  if (ALWAYS_VISIBLE_COLUMNS.has(key)) return
  if (hiddenColumns.has(key)) {
    hiddenColumns.delete(key)
  } else {
    hiddenColumns.add(key)
  }
  saveColumnsToStorage()
}

const isColumnVisible = (key: string) => !hiddenColumns.has(key)

const columns = computed<Column[]>(() =>
  allColumns.value.filter((col) => ALWAYS_VISIBLE_COLUMNS.has(col.key) || !hiddenColumns.has(col.key))
)
const apiKeys = ref<ApiKey[]>([])
const groups = ref<AvailableGroup[]>([])
const loading = ref(false)
const submitting = ref(false)
const now = ref(new Date())
let resetTimer: ReturnType<typeof setInterval> | null = null
const usageStats = ref<Record<string, BatchApiKeyUsageStats>>({})
const userGroupRates = ref<Record<number, number>>({})

const pagination = ref({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
})
const sortState = ref({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc'
})

// Filter state
const filterSearch = ref('')
const filterStatus = ref('')
const filterGroupId = ref<string | number>('')

const showCreateModal = ref(false)
const showEditModal = ref(false)
const showDeleteDialog = ref(false)
const showResetQuotaDialog = ref(false)
const showResetRateLimitDialog = ref(false)
const showUseKeyModal = ref(false)
const showCcsClientSelect = ref(false)
const showColumnDropdown = ref(false)
const pendingCcsRow = ref<ApiKey | null>(null)
const selectedKey = ref<ApiKey | null>(null)
const copiedKeyId = ref<number | null>(null)
const groupSelectorKeyId = ref<number | null>(null)
const publicSettings = ref<PublicSettings | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const columnDropdownRef = ref<HTMLElement | null>(null)
const dropdownPosition = ref<{ top?: number; bottom?: number; left: number } | null>(null)
const groupButtonRefs = ref<Map<number, HTMLElement>>(new Map())
const openingLobeHubKeyId = ref<number | null>(null)
let abortController: AbortController | null = null

// Get the currently selected key for group change
const selectedKeyForGroup = computed(() => {
  if (groupSelectorKeyId.value === null) return null
  return apiKeys.value.find((k) => k.id === groupSelectorKeyId.value) || null
})

const setGroupButtonRef = (keyId: number, el: Element | ComponentPublicInstance | null) => {
  if (el instanceof HTMLElement) {
    groupButtonRefs.value.set(keyId, el)
  } else {
    groupButtonRefs.value.delete(keyId)
  }
}

const formData = ref({
  name: '',
  access_source: 'balance' as AccessSource,
  group_id: null as number | null,
  subscription_entitlement_id: null as number | null,
  auto_switch_group_enabled: false,
  status: 'active' as 'active' | 'inactive',
  use_custom_key: false,
  custom_key: '',
  enable_ip_restriction: false,
  ip_whitelist: '',
  ip_blacklist: '',
  // Quota settings (empty = unlimited)
  enable_quota: false,
  quota: null as number | null,
  // Rate limit settings
  enable_rate_limit: false,
  rate_limit_5h: null as number | null,
  rate_limit_1d: null as number | null,
  rate_limit_7d: null as number | null,
  enable_expiration: false,
  expiration_preset: '30' as '7' | '30' | '90' | 'custom',
  expiration_date: ''
})

// 自定义Key验证
const customKeyError = computed(() => {
  if (!formData.value.use_custom_key || !formData.value.custom_key) {
    return ''
  }
  const key = formData.value.custom_key
  if (key.length < 16) {
    return t('keys.customKeyTooShort')
  }
  // 检查字符：只允许字母、数字、下划线、连字符
  if (!/^[a-zA-Z0-9_-]+$/.test(key)) {
    return t('keys.customKeyInvalidChars')
  }
  return ''
})

const statusOptions = computed(() => [
  { value: 'active', label: t('common.active') },
  { value: 'inactive', label: t('common.inactive') }
])

const sortedGroups = computed(() =>
  sortGroupsForDisplay(groups.value, {
    getRateMultiplier: (group) => effectiveGroupRate(group.id, group.rate_multiplier),
  })
)

// Filter dropdown options
const groupFilterOptions = computed(() => [
  { value: '', label: t('keys.allGroups') },
  { value: 0, label: t('keys.noGroup') },
  ...sortedGroups.value.map((g) => ({ value: g.id, label: g.name }))
])

const statusFilterOptions = computed(() => [
  { value: '', label: t('keys.allStatus') },
  { value: 'active', label: t('keys.status.active') },
  { value: 'inactive', label: t('keys.status.inactive') },
  { value: 'quota_exhausted', label: t('keys.status.quota_exhausted') },
  { value: 'expired', label: t('keys.status.expired') }
])

const canOpenInLobeHub = computed(() => Boolean(
  publicSettings.value?.lobehub_enabled && !publicSettings.value?.hide_lobehub_import_button
))

const canOpenRowInLobeHub = (row: ApiKey) => canOpenInLobeHub.value && row.status === 'active'

const onFilterChange = () => {
  pagination.value.page = 1
  loadApiKeys()
}

const onGroupFilterChange = (value: string | number | boolean | null) => {
  filterGroupId.value = value as string | number
  onFilterChange()
}

const onStatusFilterChange = (value: string | number | boolean | null) => {
  filterStatus.value = value as string
  onFilterChange()
}

// Convert groups to Select options format with rate multiplier and subscription type
const groupOptions = computed(() =>
  sortedGroups.value.map((group) => ({
    value: group.id,
    label: group.name,
    description: group.description,
    rate: group.rate_multiplier,
    userRate: userGroupRates.value[group.id] ?? null,
    peakRateEnabled: group.peak_rate_enabled,
    peakStart: group.peak_start,
    peakEnd: group.peak_end,
    peakRateMultiplier: group.peak_rate_multiplier,
    sortOrder: group.sort_order ?? null,
    subscriptionType: group.subscription_type,
    balanceEnabled: group.balance_enabled,
    subscriptionEnabled: group.subscription_enabled,
    planAutoGrantEnabled: group.plan_auto_grant_enabled,
    platform: group.platform,
    entitlements: group.entitlements ?? [],
    accessSources: group.access_sources ?? []
  }))
)

const selectedGroupOption = computed(() =>
  groupOptions.value.find((option) => option.value === formData.value.group_id) ?? null
)

function activeAccessSourcesForGroup(option: GroupOption, type?: AccessSource): AvailableGroupAccessSource[] {
  const sources = option.accessSources.filter((source) => {
    if (source.disabled) return false
    if (type && source.type !== type) return false
    return source.type === 'balance' || source.type === 'entitlement'
  })
  return sources as AvailableGroupAccessSource[]
}

function entitlementAccessSourcesForGroup(option: GroupOption): AvailableGroupAccessSource[] {
  return activeAccessSourcesForGroup(option, 'entitlement').filter((source) => Number(source.entitlement_id) > 0)
}

function entitlementHasUsableStructuredSource(option: GroupOption, entitlementID: number): boolean {
  const matchingSources = option.accessSources.filter((source) =>
    source.type === 'entitlement' && Number(source.entitlement_id) === entitlementID
  )
  // Older / partially rolled-out servers can return entitlement metadata before
  // they include the corresponding access source. Missing metadata must not
  // hide a valid plan group; an explicit disabled source still takes priority.
  return matchingSources.length === 0 || matchingSources.some((source) => !source.disabled)
}

function accessSourceHasStructuredSources(option: GroupOption): boolean {
  return option.accessSources.length > 0
}

function entitlementFromAccessSource(source: AvailableGroupAccessSource): AvailableGroupEntitlement | null {
  const id = Number(source.entitlement_id)
  if (!Number.isFinite(id) || id <= 0) return null
  return {
    id,
    name: source.name || source.label || t('keys.entitlementFallbackName', { id }),
    plan_id: source.plan_id ?? null,
    starts_at: '',
    expires_at: source.expires_at ?? '',
    purchase_price: source.purchase_price ?? null,
    purchase_currency: source.purchase_currency ?? null,
    quota_usd: source.quota_usd ?? null,
    quota_used_usd: source.quota_used_usd ?? null,
    quota_period: source.quota_period ?? null,
    unit_cost_per_usd: source.unit_cost_per_usd ?? null,
    overage_policy: source.overage_policy ?? null,
  }
}

function entitlementsForGroupOption(option: GroupOption): AvailableGroupEntitlement[] {
  const byID = new Map<number, AvailableGroupEntitlement>()
  for (const entitlement of option.entitlements) {
    if (entitlementHasUsableStructuredSource(option, entitlement.id)) {
      byID.set(entitlement.id, entitlement)
    }
  }
  for (const source of entitlementAccessSourcesForGroup(option)) {
    const entitlement = entitlementFromAccessSource(source)
    if (entitlement && !byID.has(entitlement.id)) {
      byID.set(entitlement.id, entitlement)
    }
  }
  return Array.from(byID.values()).sort((a, b) => a.id - b.id)
}

const availableEntitlements = computed(() => {
  const byID = new Map<number, AvailableGroupEntitlement>()
  for (const option of groupOptions.value) {
    for (const entitlement of entitlementsForGroupOption(option)) {
      if (!byID.has(entitlement.id)) {
        byID.set(entitlement.id, entitlement)
      }
    }
  }
  return Array.from(byID.values()).sort((a, b) => a.id - b.id)
})

const entitlementOptions = computed<EntitlementSelectOption[]>(() =>
  availableEntitlements.value.map((entitlement) => ({
    value: entitlement.id,
    label: formatEntitlementLabel(entitlement),
    description: formatEntitlementDescription(entitlement),
    entitlement
  }))
)

function normalizeAccessSource(source?: string | null, entitlementID?: number | null): AccessSource {
  if (source === 'entitlement') return 'entitlement'
  if (source === 'balance') return 'balance'
  return entitlementID ? 'entitlement' : 'balance'
}

function rowAccessSource(key: ApiKey): AccessSource {
  return normalizeAccessSource(key.access_source, key.subscription_entitlement_id)
}

function groupSupportsBalanceSource(option: GroupOption): boolean {
  if (accessSourceHasStructuredSources(option)) {
    return activeAccessSourcesForGroup(option, 'balance').length > 0
  }
  if (option.balanceEnabled !== undefined) return option.balanceEnabled
  return option.subscriptionType !== 'subscription' || option.entitlements.length === 0
}

function groupSupportsEntitlementSource(option: GroupOption): boolean {
  if (accessSourceHasStructuredSources(option)) {
    return entitlementsForGroupOption(option).length > 0
  }
  if (option.subscriptionEnabled !== undefined) return option.subscriptionEnabled && option.entitlements.length > 0
  return option.subscriptionType === 'subscription' && option.entitlements.length > 0
}

const formGroupOptions = computed(() => {
  if (formData.value.access_source === 'balance') {
    return groupOptions.value.filter(groupSupportsBalanceSource)
  }

  const entitlementID = formData.value.subscription_entitlement_id
  if (!entitlementID) return groupOptions.value.filter(groupSupportsEntitlementSource)
  return groupOptions.value.filter((option) => {
    if (!groupSupportsEntitlementSource(option)) return false
    return entitlementsForGroupOption(option).some((entitlement) => entitlement.id === entitlementID)
  })
})

const requiresEntitlementSelection = computed(() => {
  const group = selectedGroupOption.value
  return Boolean(
    group &&
    formData.value.access_source === 'entitlement' &&
    groupSupportsEntitlementSource(group) &&
    entitlementsForGroupOption(group).length > 0 &&
    !formData.value.subscription_entitlement_id
  )
})

function entitlementOptionLabel(option: unknown): string {
  if (typeof option === 'object' && option !== null && 'label' in option) {
    return String((option as { label?: unknown }).label ?? '')
  }
  return ''
}

function entitlementOptionDescription(option: unknown): string {
  if (typeof option === 'object' && option !== null && 'description' in option) {
    return String((option as { description?: unknown }).description ?? '')
  }
  return ''
}

function effectiveGroupRate(groupID: number, defaultRate: number): number {
  return userGroupRates.value[groupID] ?? defaultRate
}

function effectiveGroupOptionRate(option: GroupOption): number {
  return option.userRate ?? option.rate
}

function formatRateMultiplier(rate: number): string {
  return `${Number(rate.toFixed(3))}x`
}

function positiveNumber(value: unknown): number | null {
  const numberValue = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(numberValue) && numberValue > 0 ? numberValue : null
}

function normalizeCurrencyCode(currency: string | null | undefined): string {
  return currency?.trim().toUpperCase() || 'CNY'
}

function currencySymbol(currency: string | null | undefined): string {
  const code = normalizeCurrencyCode(currency)
  switch (code) {
    case 'CNY':
    case 'RMB':
      return '\u00A5'
    case 'USD':
      return '$'
    case 'EUR':
      return '\u20AC'
    case 'GBP':
      return '\u00A3'
    case 'JPY':
      return '\u00A5'
    default:
      return `${code} `
  }
}

function formatMoneyAmount(value: number, currency: string | null | undefined): string {
  const fractionDigits = Math.abs(value) >= 1 ? 2 : 4
  return `${currencySymbol(currency)}${new Intl.NumberFormat('zh-CN', {
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  }).format(value)}`
}

function formatQuotaUSD(value: number): string {
  const absolute = Math.abs(value)
  const fractionDigits = absolute >= 100 ? 0 : 2
  return `$${new Intl.NumberFormat('en-US', {
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  }).format(value)}`
}

function formatUsageUSD(value: number): string {
  return `$${new Intl.NumberFormat('en-US', {
    minimumFractionDigits: 4,
    maximumFractionDigits: 4,
  }).format(value)}`
}

function entitlementQuotaTotal(entitlement: AvailableGroupEntitlement | null | undefined): number | null {
  return positiveNumber(entitlement?.quota_usd)
}

function entitlementQuotaRemainingText(entitlement: AvailableGroupEntitlement | null | undefined): string {
  const total = entitlementQuotaTotal(entitlement)
  if (total === null) return t('keys.entitlementQuotaUnavailable')
  return t('keys.entitlementQuotaTotal', {
    total: formatQuotaUSD(total),
  })
}

function entitlementQuotaCompactText(entitlement: AvailableGroupEntitlement | null | undefined): string {
  const total = entitlementQuotaTotal(entitlement)
  if (total === null) return t('keys.entitlementQuotaUnavailable')
  return t('keys.entitlementQuotaTotal', {
    total: formatQuotaUSD(total),
  })
}

function entitlementQuotaTextClass(entitlement: AvailableGroupEntitlement | null | undefined): string {
  return entitlementQuotaTotal(entitlement) === null
    ? 'text-gray-500 dark:text-gray-400'
    : 'text-primary-700 dark:text-primary-300'
}

function entitlementQuotaPillClass(entitlement: AvailableGroupEntitlement | null | undefined): string {
  return entitlementQuotaTotal(entitlement) === null
    ? 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
    : 'bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300'
}

function entitlementQuotaRemainingTextByID(id: number | null | undefined): string {
  return entitlementQuotaRemainingText(entitlementByID(id))
}

function entitlementQuotaTextClassByID(id: number | null | undefined): string {
  return entitlementQuotaTextClass(entitlementByID(id))
}

function entitlementFromSelectOption(option: unknown): AvailableGroupEntitlement | null {
  if (typeof option === 'object' && option !== null && 'entitlement' in option) {
    return (option as { entitlement?: AvailableGroupEntitlement }).entitlement ?? null
  }
  return null
}

function entitlementOptionQuotaCompactText(option: unknown): string {
  return entitlementQuotaCompactText(entitlementFromSelectOption(option))
}

function entitlementOptionQuotaPillClass(option: unknown): string {
  return entitlementQuotaPillClass(entitlementFromSelectOption(option))
}

function entitlementUnitCost(entitlement: AvailableGroupEntitlement | null | undefined): number | null {
  if (!entitlement) return null
  const provided = positiveNumber(entitlement.unit_cost_per_usd)
  if (provided !== null) return provided

  const purchasePrice = positiveNumber(entitlement.purchase_price)
  const quotaUSD = positiveNumber(entitlement.quota_usd)
  if (purchasePrice === null || quotaUSD === null) return null
  return purchasePrice / quotaUSD
}

function entitlementQuotaPeriodText(entitlement: AvailableGroupEntitlement | null | undefined): string {
  switch (entitlement?.quota_period) {
    case 'daily':
      return t('keys.quotaPeriod.daily')
    case 'weekly':
      return t('keys.quotaPeriod.weekly')
    case 'monthly':
      return t('keys.quotaPeriod.monthly')
    default:
      return t('keys.quotaPeriod.default')
  }
}

function entitlementQuotaPeriodTextByID(id: number | null | undefined): string {
  return entitlementQuotaPeriodText(entitlementByID(id))
}

function entitlementActualCostText(
  entitlement: AvailableGroupEntitlement | null | undefined,
  rate: number
): string {
  const unitCost = entitlementUnitCost(entitlement)
  if (unitCost === null) return t('keys.priceUnavailable')
  const multiplier = positiveNumber(rate) ?? 1
  return t('keys.actualRmbCostHint', {
    rate: formatRateMultiplier(multiplier),
    amount: formatMoneyAmount(unitCost * multiplier, entitlement?.purchase_currency),
  })
}

function entitlementActualCostTextByID(id: number | null | undefined, rate: number): string {
  return entitlementActualCostText(entitlementByID(id), rate)
}

function entitlementForGroupOption(
  option: GroupOption | null,
  entitlementID: number | null | undefined
): AvailableGroupEntitlement | null {
  if (!option || !groupSupportsEntitlementSource(option)) return null
  const entitlements = entitlementsForGroupOption(option)
  if (entitlementID) {
    return entitlements.find((entitlement) => entitlement.id === entitlementID) ?? entitlementByID(entitlementID)
  }
  if (entitlements.length === 1) return entitlements[0]
  return null
}

function rowGroupActualCostText(key: ApiKey): string {
  if (rowAccessSource(key) === 'balance') return t('keys.balanceDeductionHint')
  if (!key.group) return t('keys.priceUnavailable')
  return entitlementActualCostTextByID(
    key.subscription_entitlement_id,
    effectiveGroupRate(key.group.id, key.group.rate_multiplier)
  )
}

type ApiKeyUsageScope = 'today' | 'total'

function apiKeyUsageCost(key: ApiKey, scope: ApiKeyUsageScope): number {
  const stats = usageStats.value[key.id]
  const cost = scope === 'today' ? stats?.today_actual_cost : stats?.total_actual_cost
  const value = typeof cost === 'number' ? cost : Number(cost)
  return Number.isFinite(value) ? value : 0
}

function apiKeyUsageCnyText(key: ApiKey, scope: ApiKeyUsageScope): string {
  if (rowAccessSource(key) !== 'entitlement' || !key.subscription_entitlement_id) return ''
  const amount = convertUsdToCnyForLog(apiKeyUsageCost(key, scope), {
    entitlement_id: key.subscription_entitlement_id,
  })
  return formatCny(amount)
}

function groupOptionActualCostText(
  option: GroupOption | null,
  entitlementID: number | null | undefined,
  accessSource: AccessSource = formData.value.access_source
): string {
  if (accessSource === 'balance') return t('keys.balanceDeductionHint')
  return entitlementActualCostText(
    entitlementForGroupOption(option, entitlementID),
    option ? effectiveGroupOptionRate(option) : 1
  )
}

function normalizeEntitlementCardName(name: string): string {
  return name.replace(/^(?:\u5957\u9910\u6743\u76ca|\u6743\u76ca)[-\uFF0D\s]*/u, '').trim()
}

function formatEntitlementLabel(entitlement: AvailableGroupEntitlement): string {
  const name = normalizeEntitlementCardName(entitlement.name?.trim() || '')
  return name || t('keys.entitlementFallbackName', { id: entitlement.id })
}

function formatEntitlementDescription(entitlement: AvailableGroupEntitlement): string {
  const parts: string[] = []
  const groupCount = entitlementGroupOptions(entitlement.id).length
  if (entitlement.plan_id) {
    parts.push(t('keys.entitlementPlan', { id: entitlement.plan_id }))
  }
  if (groupCount > 0) {
    parts.push(t('keys.entitlementIncludedGroups', { count: groupCount }))
  }
  if (entitlement.expires_at) {
    parts.push(t('keys.entitlementExpires', { date: formatDateTime(entitlement.expires_at) }))
  }
  return parts.join(' / ')
}

function entitlementByID(id: number | null | undefined): AvailableGroupEntitlement | null {
  if (!id) return null
  return availableEntitlements.value.find((entitlement) => entitlement.id === id) ?? null
}

function entitlementLabelByID(id: number | null | undefined): string {
  const entitlement = entitlementByID(id)
  return entitlement ? formatEntitlementLabel(entitlement) : t('keys.entitlementFallbackName', { id })
}

function entitlementGroupOptions(entitlementID: number): GroupOption[] {
  return groupOptions.value.filter((option) => {
    if (!groupSupportsEntitlementSource(option)) return false
    return entitlementsForGroupOption(option).some((entitlement) => entitlement.id === entitlementID)
  })
}

function entitlementCountText(option: GroupOption): string {
  const entitlements = entitlementsForGroupOption(option)
  if (entitlements.length <= 0) return ''
  if (entitlements.length === 1) return t('keys.entitlementSingleAvailable')
  return t('keys.entitlementMultipleAvailable', { count: entitlements.length })
}

function groupEntitlementScopeText(
  option: GroupOption,
  entitlementID: number | null | undefined,
  accessSource: AccessSource = formData.value.access_source
): string {
  if (accessSource === 'balance') {
    return t('keys.accessSourceBalanceShort')
  }
  if (entitlementID && entitlementsForGroupOption(option).some((entitlement) => entitlement.id === entitlementID)) {
    return t('keys.entitlementCurrentCardIncludesGroup')
  }
  return entitlementCountText(option)
}

function reconcileFormEntitlementSelection() {
  if (groups.value.length === 0) return

  const selectedEntitlementID = formData.value.subscription_entitlement_id
  const group = selectedGroupOption.value

  if (formData.value.access_source === 'balance') {
    formData.value.subscription_entitlement_id = null
    if (group && !groupSupportsBalanceSource(group)) {
      formData.value.group_id = null
    }
    return
  }

  if (
    formData.value.group_id === null &&
    !selectedEntitlementID &&
    formGroupOptions.value.length > 0 &&
    availableEntitlements.value.length === 1
  ) {
    formData.value.subscription_entitlement_id = availableEntitlements.value[0].id
  }

  if (formData.value.group_id === null) {
    return
  }

  if (!group || !groupSupportsEntitlementSource(group)) {
    formData.value.subscription_entitlement_id = null
    formData.value.group_id = null
    return
  }

  const entitlements = entitlementsForGroupOption(group)
  if (entitlements.length === 0) {
    formData.value.subscription_entitlement_id = null
    return
  }

  if (selectedEntitlementID && entitlements.some((entitlement) => entitlement.id === selectedEntitlementID)) {
    return
  }

  if (selectedEntitlementID) {
    formData.value.group_id = null
    return
  }

  if (entitlements.length === 1) {
    formData.value.subscription_entitlement_id = entitlements[0].id
  }
}

watch(
  () => [formData.value.access_source, formData.value.group_id, formData.value.subscription_entitlement_id, groups.value] as const,
  () => reconcileFormEntitlementSelection()
)

function selectAccessSource(source: AccessSource) {
  if (formData.value.access_source === source) return

  formData.value.access_source = source
  if (source === 'balance') {
    formData.value.subscription_entitlement_id = null
    const group = selectedGroupOption.value
    if (group && !groupSupportsBalanceSource(group)) {
      formData.value.group_id = null
    }
  } else {
    const group = selectedGroupOption.value
    if (group && !groupSupportsEntitlementSource(group)) {
      formData.value.group_id = null
    }
  }
  reconcileFormEntitlementSelection()
}

// Group dropdown search
const groupSearchQuery = ref('')
const quickSwitchGroupOptions = computed(() => {
  const selectedKey = selectedKeyForGroup.value
  if (!selectedKey) return groupOptions.value
  if (rowAccessSource(selectedKey) === 'balance') {
    return groupOptions.value.filter(groupSupportsBalanceSource)
  }

  const entitlementID = selectedKey.subscription_entitlement_id
  if (!entitlementID) return []
  return groupOptions.value.filter((option) => {
    if (!groupSupportsEntitlementSource(option)) return false
    return entitlementsForGroupOption(option).some((entitlement) => entitlement.id === entitlementID)
  })
})
const filteredGroupOptions = computed(() => {
  const query = groupSearchQuery.value.trim().toLowerCase()
  const options = quickSwitchGroupOptions.value
  if (!query) return options
  return options.filter((opt) => {
    return opt.label.toLowerCase().includes(query) ||
      (opt.description && opt.description.toLowerCase().includes(query))
  })
})

const copyToClipboard = async (text: string, keyId: number) => {
  const success = await clipboardCopy(text, t('keys.copied'))
  if (success) {
    copiedKeyId.value = keyId
    setTimeout(() => {
      copiedKeyId.value = null
    }, 800)
  }
}

const isAbortError = (error: unknown) => {
  if (!error || typeof error !== 'object') return false
  const { name, code } = error as { name?: string; code?: string }
  return name === 'AbortError' || code === 'ERR_CANCELED'
}

const loadApiKeys = async () => {
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller
  const { signal } = controller
  loading.value = true
  try {
    // Build filters
    const filters: {
      search?: string
      status?: string
      group_id?: number | string
      sort_by?: string
      sort_order?: 'asc' | 'desc'
    } = {}
    if (filterSearch.value) filters.search = filterSearch.value
    if (filterStatus.value) filters.status = filterStatus.value
    if (filterGroupId.value !== '') filters.group_id = filterGroupId.value
    filters.sort_by = sortState.value.sort_by
    filters.sort_order = sortState.value.sort_order

    const response = await keysAPI.list(pagination.value.page, pagination.value.page_size, filters, {
      signal
    })
    if (signal.aborted) return
    apiKeys.value = response.items
    pagination.value.total = response.total
    pagination.value.pages = response.pages

    // Load usage stats for all API keys in the list
    if (response.items.length > 0) {
      const keyIds = response.items.map((k) => k.id)
      try {
        const usageResponse = await usageAPI.getDashboardApiKeysUsage(keyIds, { signal })
        if (signal.aborted) return
        usageStats.value = usageResponse.stats
      } catch (e) {
        if (!isAbortError(e)) {
          console.error('Failed to load usage stats:', e)
        }
      }
    }
  } catch (error) {
    if (isAbortError(error)) {
      return
    }
    appStore.showError(t('keys.failedToLoad'))
  } finally {
    if (abortController === controller) {
      loading.value = false
    }
  }
}

const loadGroups = async () => {
  try {
    groups.value = await userGroupsAPI.getAvailable()
  } catch (error) {
    console.error('Failed to load groups:', error)
  }
}

const loadUserGroupRates = async () => {
  try {
    userGroupRates.value = await userGroupsAPI.getUserGroupRates()
  } catch (error) {
    console.error('Failed to load user group rates:', error)
  }
}

const loadPublicSettings = async () => {
  try {
    publicSettings.value = await authAPI.getPublicSettings()
  } catch (error) {
    console.error('Failed to load public settings:', error)
  }
}

const openUseKeyModal = (key: ApiKey) => {
  selectedKey.value = key
  showUseKeyModal.value = true
}

const closeUseKeyModal = () => {
  showUseKeyModal.value = false
  selectedKey.value = null
}

function safeRouteQueryString(value: unknown): string {
  if (Array.isArray(value)) {
    return typeof value[0] === 'string' ? value[0].trim() : ''
  }
  return typeof value === 'string' ? value.trim() : ''
}

function getRedirectQueryPath(): string {
  const redirect = safeRouteQueryString(route.query.redirect)
  if (!redirect.startsWith('/') || redirect.startsWith('//') || redirect.includes('://') || redirect.includes('\n') || redirect.includes('\r')) {
    return ''
  }
  return redirect
}

function shouldOpenCreateFromQuery(): boolean {
  return safeRouteQueryString(route.query.openCreate) === '1'
}

async function closeCreateQuery() {
  if (!shouldOpenCreateFromQuery()) return
  const query = { ...route.query }
  delete query.openCreate
  await router.replace({ path: route.path, query })
}

const handleCreateAction = () => {
  if (availableEntitlements.value.length > 0) {
    formData.value.access_source = 'entitlement'
    formData.value.subscription_entitlement_id = availableEntitlements.value.length === 1
      ? availableEntitlements.value[0].id
      : null
    reconcileFormEntitlementSelection()
  }
  showCreateModal.value = true
}

const openInLobeHub = async (row: ApiKey) => {
  if (openingLobeHubKeyId.value !== null) return

  const popup = window.open('', '_blank')
  if (popup) {
    try {
      popup.opener = null
    } catch {
      // ignore
    }
  }

  openingLobeHubKeyId.value = row.id
  try {
    const response = await lobehubAPI.createLaunchTicket(row.id)
    const bridgeURL = new URL(response.bridge_url, window.location.origin).toString()
    if (popup?.location) {
      popup.location.replace(bridgeURL)
    } else {
      window.location.assign(bridgeURL)
    }
  } catch (error) {
    if (popup && !popup.closed) popup.close()
    console.error('Failed to open LobeHub:', error)
    appStore.showError(t('keys.failedToOpenLobeHub'))
  } finally {
    openingLobeHubKeyId.value = null
  }
}

const handlePageChange = (page: number) => {
  pagination.value.page = page
  loadApiKeys()
}

const handlePageSizeChange = (pageSize: number) => {
  pagination.value.page_size = pageSize
  pagination.value.page = 1
  loadApiKeys()
}

const handleSort = (key: string, order: 'asc' | 'desc') => {
  sortState.value.sort_by = key
  sortState.value.sort_order = order
  pagination.value.page = 1
  loadApiKeys()
}

const editKey = (key: ApiKey) => {
  selectedKey.value = key
  const hasIPRestriction = (key.ip_whitelist?.length > 0) || (key.ip_blacklist?.length > 0)
  const hasExpiration = !!key.expires_at
  formData.value = {
    name: key.name,
    access_source: rowAccessSource(key),
    group_id: key.group_id,
    subscription_entitlement_id: key.subscription_entitlement_id ?? null,
    auto_switch_group_enabled: false,
    status: key.status === 'quota_exhausted' || key.status === 'expired' ? 'inactive' : key.status,
    use_custom_key: false,
    custom_key: '',
    enable_ip_restriction: hasIPRestriction,
    ip_whitelist: (key.ip_whitelist || []).join('\n'),
    ip_blacklist: (key.ip_blacklist || []).join('\n'),
    enable_quota: key.quota > 0,
    quota: key.quota > 0 ? key.quota : null,
    enable_rate_limit: (key.rate_limit_5h > 0) || (key.rate_limit_1d > 0) || (key.rate_limit_7d > 0),
    rate_limit_5h: key.rate_limit_5h || null,
    rate_limit_1d: key.rate_limit_1d || null,
    rate_limit_7d: key.rate_limit_7d || null,
    enable_expiration: hasExpiration,
    expiration_preset: 'custom',
    expiration_date: key.expires_at ? formatDateTimeLocal(key.expires_at) : ''
  }
  reconcileFormEntitlementSelection()
  showEditModal.value = true
}

const toggleKeyStatus = async (key: ApiKey) => {
  const newStatus = key.status === 'active' ? 'inactive' : 'active'
  try {
    await keysAPI.toggleStatus(key.id, newStatus)
    appStore.showSuccess(
      newStatus === 'active' ? t('keys.keyEnabledSuccess') : t('keys.keyDisabledSuccess')
    )
    loadApiKeys()
  } catch (error) {
    appStore.showError(t('keys.failedToUpdateStatus'))
  }
}

const openGroupSelector = (key: ApiKey) => {
  if (groupSelectorKeyId.value === key.id) {
    groupSelectorKeyId.value = null
    dropdownPosition.value = null
  } else {
    const buttonEl = groupButtonRefs.value.get(key.id)
    if (buttonEl) {
      const rect = buttonEl.getBoundingClientRect()
      const dropdownEstHeight = 400 // estimated max dropdown height
      const spaceBelow = window.innerHeight - rect.bottom
      const spaceAbove = rect.top

      if (spaceBelow < dropdownEstHeight && spaceAbove > spaceBelow) {
        // Not enough space below, pop upward
        dropdownPosition.value = {
          bottom: window.innerHeight - rect.top + 4,
          left: rect.left
        }
      } else {
        // Default: pop downward
        dropdownPosition.value = {
          top: rect.bottom + 4,
          left: rect.left
        }
      }
    }
    groupSelectorKeyId.value = key.id
    groupSearchQuery.value = ''
  }
}

function entitlementIDForGroupOption(option: GroupOption | null, key?: ApiKey): number | null | undefined {
  if (!option) return null
  if (!key || rowAccessSource(key) === 'balance') return null
  if (!groupSupportsEntitlementSource(option)) return null
  const currentEntitlementID = key?.subscription_entitlement_id
  const entitlements = entitlementsForGroupOption(option)
  if (currentEntitlementID && entitlements.some((entitlement) => entitlement.id === currentEntitlementID)) {
    return currentEntitlementID
  }
  if (entitlements.length === 1) return entitlements[0].id
  if (entitlements.length > 1) return undefined
  return undefined
}

function subscriptionEntitlementPayloadForForm(includeClear: boolean): number | null | undefined {
  if (formData.value.access_source === 'balance') return includeClear ? null : null
  const option = selectedGroupOption.value
  if (formData.value.group_id === null) return includeClear ? null : undefined
  if (!option) return undefined
  if (!groupSupportsEntitlementSource(option)) return includeClear ? null : undefined
  if (entitlementsForGroupOption(option).length > 0) return formData.value.subscription_entitlement_id
  return undefined
}

const changeGroup = async (key: ApiKey, newGroupId: number | null) => {
  groupSelectorKeyId.value = null
  dropdownPosition.value = null
  if (key.group_id === newGroupId) return

  const option = groupOptions.value.find((item) => item.value === newGroupId) ?? null
  const accessSource = rowAccessSource(key)
  if (option && accessSource === 'balance' && !groupSupportsBalanceSource(option)) {
    appStore.showInfo(t('keys.accessSourceSwitchRequired'))
    editKey(key)
    return
  }
  if (option && accessSource === 'entitlement' && !groupSupportsEntitlementSource(option)) {
    appStore.showInfo(t('keys.accessSourceSwitchRequired'))
    editKey(key)
    return
  }

  const entitlementID = entitlementIDForGroupOption(option, key)
  if (entitlementID === undefined && option && entitlementsForGroupOption(option).length > 0) {
    editKey(key)
    formData.value.access_source = 'entitlement'
    formData.value.group_id = newGroupId
    formData.value.subscription_entitlement_id = null
    appStore.showInfo(t('keys.entitlementSelectionRequired'))
    return
  }

  const payload: { group_id: number | null; access_source?: AccessSource; subscription_entitlement_id?: number | null } = {
    group_id: newGroupId
  }
  payload.access_source = accessSource
  if (entitlementID !== undefined) {
    payload.subscription_entitlement_id = entitlementID
  }

  try {
    await keysAPI.update(key.id, payload)
    appStore.showSuccess(t('keys.groupChangedSuccess'))
    loadApiKeys()
  } catch (error) {
    appStore.showError(t('keys.failedToChangeGroup'))
  }
}

const closeGroupSelector = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  // Check if click is inside the dropdown or the trigger button
  if (!target.closest('.group\\/dropdown') && !dropdownRef.value?.contains(target)) {
    groupSelectorKeyId.value = null
    dropdownPosition.value = null
  }
  if (columnDropdownRef.value && !columnDropdownRef.value.contains(target)) {
    showColumnDropdown.value = false
  }
}

const confirmDelete = (key: ApiKey) => {
  selectedKey.value = key
  showDeleteDialog.value = true
}

const handleSubmit = async () => {
  // Validate group_id is required
  if (formData.value.group_id === null) {
    appStore.showError(t('keys.groupRequired'))
    return
  }
  if (requiresEntitlementSelection.value && !formData.value.subscription_entitlement_id) {
    appStore.showError(t('keys.entitlementRequired'))
    return
  }
  if (formData.value.access_source === 'entitlement' && !formData.value.subscription_entitlement_id) {
    appStore.showError(t('keys.entitlementRequired'))
    return
  }

  // Validate custom key if enabled
  if (!showEditModal.value && formData.value.use_custom_key) {
    if (!formData.value.custom_key) {
      appStore.showError(t('keys.customKeyRequired'))
      return
    }
    if (customKeyError.value) {
      appStore.showError(customKeyError.value)
      return
    }
  }

  // Parse IP lists only if IP restriction is enabled
  const parseIPList = (text: string): string[] =>
    text.split('\n').map(ip => ip.trim()).filter(ip => ip.length > 0)
  const ipWhitelist = formData.value.enable_ip_restriction ? parseIPList(formData.value.ip_whitelist) : []
  const ipBlacklist = formData.value.enable_ip_restriction ? parseIPList(formData.value.ip_blacklist) : []

  // Calculate quota value (null/empty/0 = unlimited, stored as 0)
  const quota = formData.value.quota && formData.value.quota > 0 ? formData.value.quota : 0

  // Calculate expiration
  let expiresInDays: number | undefined
  let expiresAt: string | null | undefined
  if (formData.value.enable_expiration && formData.value.expiration_date) {
    if (!showEditModal.value) {
      // Create mode: calculate days from date
      const expDate = new Date(formData.value.expiration_date)
      const now = new Date()
      const diffDays = Math.ceil((expDate.getTime() - now.getTime()) / (1000 * 60 * 60 * 24))
      expiresInDays = diffDays > 0 ? diffDays : 1
    } else {
      // Edit mode: use custom date directly
      expiresAt = new Date(formData.value.expiration_date).toISOString()
    }
  } else if (showEditModal.value) {
    // Edit mode: if expiration disabled or date cleared, send empty string to clear
    expiresAt = ''
  }

  // Calculate rate limit values (send 0 when toggle is off)
  const rateLimitData = formData.value.enable_rate_limit ? {
    rate_limit_5h: formData.value.rate_limit_5h && formData.value.rate_limit_5h > 0 ? formData.value.rate_limit_5h : 0,
    rate_limit_1d: formData.value.rate_limit_1d && formData.value.rate_limit_1d > 0 ? formData.value.rate_limit_1d : 0,
    rate_limit_7d: formData.value.rate_limit_7d && formData.value.rate_limit_7d > 0 ? formData.value.rate_limit_7d : 0,
  } : { rate_limit_5h: 0, rate_limit_1d: 0, rate_limit_7d: 0 }

  submitting.value = true
  try {
    if (showEditModal.value && selectedKey.value) {
      const entitlementPatch = subscriptionEntitlementPayloadForForm(true)
      const updatePayload: UpdateApiKeyRequest = {
        name: formData.value.name,
        group_id: formData.value.group_id,
        access_source: formData.value.access_source,
        auto_switch_group_enabled: formData.value.auto_switch_group_enabled,
        status: formData.value.status,
        ip_whitelist: ipWhitelist,
        ip_blacklist: ipBlacklist,
        quota: quota,
        expires_at: expiresAt,
        rate_limit_5h: rateLimitData.rate_limit_5h,
        rate_limit_1d: rateLimitData.rate_limit_1d,
        rate_limit_7d: rateLimitData.rate_limit_7d,
      }
      if (entitlementPatch !== undefined) {
        updatePayload.subscription_entitlement_id = entitlementPatch
      }
      await keysAPI.update(selectedKey.value.id, updatePayload)
      appStore.showSuccess(t('keys.keyUpdatedSuccess'))
    } else {
      const customKey = formData.value.use_custom_key ? formData.value.custom_key : undefined
      const createPayload: CreateApiKeyRequest = {
        name: formData.value.name,
        group_id: formData.value.group_id,
        access_source: formData.value.access_source,
        auto_switch_group_enabled: formData.value.auto_switch_group_enabled,
        ...(customKey ? { custom_key: customKey } : {})
      }
      if (ipWhitelist.length > 0) createPayload.ip_whitelist = ipWhitelist
      if (ipBlacklist.length > 0) createPayload.ip_blacklist = ipBlacklist
      if (quota > 0) createPayload.quota = quota
      if (expiresInDays !== undefined && expiresInDays > 0) createPayload.expires_in_days = expiresInDays
      if (rateLimitData.rate_limit_5h > 0) createPayload.rate_limit_5h = rateLimitData.rate_limit_5h
      if (rateLimitData.rate_limit_1d > 0) createPayload.rate_limit_1d = rateLimitData.rate_limit_1d
      if (rateLimitData.rate_limit_7d > 0) createPayload.rate_limit_7d = rateLimitData.rate_limit_7d
      const entitlementPatch = subscriptionEntitlementPayloadForForm(false)
      if (entitlementPatch !== undefined) {
        createPayload.subscription_entitlement_id = entitlementPatch
      }
      await keysAPI.createWithPayload(createPayload)
      appStore.showSuccess(t('keys.keyCreatedSuccess'))
      // Only advance tour if active, on submit step, and creation succeeded
      if (onboardingStore.isCurrentStep('[data-tour="key-form-submit"]')) {
        onboardingStore.nextStep(500)
      }
      const redirectPath = getRedirectQueryPath()
      if (redirectPath) {
        closeModals()
        await loadApiKeys()
        await router.push(redirectPath)
        return
      }
    }
    closeModals()
    await closeCreateQuery()
    loadApiKeys()
  } catch (error: any) {
    const errorMsg = error.response?.data?.detail || t('keys.failedToSave')
    appStore.showError(errorMsg)
    // Don't advance tour on error
  } finally {
    submitting.value = false
  }
}

/**
 * 处理删除 API Key 的操作
 * 优化：错误处理改进，优先显示后端返回的具体错误消息（如权限不足等），
 * 若后端未返回消息则显示默认的国际化文本
 */
const handleDelete = async () => {
  if (!selectedKey.value) return

  try {
    await keysAPI.delete(selectedKey.value.id)
    appStore.showSuccess(t('keys.keyDeletedSuccess'))
    showDeleteDialog.value = false
    loadApiKeys()
  } catch (error: any) {
    // 优先使用后端返回的错误消息，提供更具体的错误信息给用户
    const errorMsg = error?.message || t('keys.failedToDelete')
    appStore.showError(errorMsg)
  }
}

const closeModals = () => {
  showCreateModal.value = false
  showEditModal.value = false
  selectedKey.value = null
  formData.value = {
    name: '',
    access_source: 'balance',
    group_id: null,
    subscription_entitlement_id: null,
    auto_switch_group_enabled: false,
    status: 'active',
    use_custom_key: false,
    custom_key: '',
    enable_ip_restriction: false,
    ip_whitelist: '',
    ip_blacklist: '',
    enable_quota: false,
    quota: null,
    enable_rate_limit: false,
    rate_limit_5h: null,
    rate_limit_1d: null,
    rate_limit_7d: null,
    enable_expiration: false,
    expiration_preset: '30',
    expiration_date: ''
  }
}

// Show reset quota confirmation dialog
const confirmResetQuota = () => {
  showResetQuotaDialog.value = true
}

// Set expiration date based on quick select days
const setExpirationDays = (days: number) => {
  formData.value.expiration_preset = days.toString() as '7' | '30' | '90'
  const expDate = new Date()
  expDate.setDate(expDate.getDate() + days)
  formData.value.expiration_date = formatDateTimeLocal(expDate.toISOString())
}

// Reset quota used for an API key
const resetQuotaUsed = async () => {
  if (!selectedKey.value) return
  showResetQuotaDialog.value = false
  try {
    await keysAPI.update(selectedKey.value.id, { reset_quota: true })
    appStore.showSuccess(t('keys.quotaResetSuccess'))
    // Update local state
    if (selectedKey.value) {
      selectedKey.value.quota_used = 0
    }
  } catch (error: any) {
    const errorMsg = error.response?.data?.detail || t('keys.failedToResetQuota')
    appStore.showError(errorMsg)
  }
}

// Show reset rate limit confirmation dialog (from edit modal)
const confirmResetRateLimit = () => {
  showResetRateLimitDialog.value = true
}

// Show reset rate limit confirmation dialog (from table row)
const confirmResetRateLimitFromTable = (row: ApiKey) => {
  selectedKey.value = row
  showResetRateLimitDialog.value = true
}

// Reset rate limit usage for an API key
const resetRateLimitUsage = async () => {
  if (!selectedKey.value) return
  showResetRateLimitDialog.value = false
  try {
    await keysAPI.update(selectedKey.value.id, { reset_rate_limit_usage: true })
    appStore.showSuccess(t('keys.rateLimitResetSuccess'))
    // Refresh key data
    await loadApiKeys()
    // Update the editing key with fresh data
    const refreshedKey = apiKeys.value.find(k => k.id === selectedKey.value!.id)
    if (refreshedKey) {
      selectedKey.value = refreshedKey
    }
  } catch (error: any) {
    const errorMsg = error.response?.data?.detail || t('keys.failedToResetRateLimit')
    appStore.showError(errorMsg)
  }
}

const importToCcswitch = (row: ApiKey) => {
  const platform = row.group?.platform || 'anthropic'

  // For antigravity platform, show client selection dialog
  if (platform === 'antigravity') {
    pendingCcsRow.value = row
    showCcsClientSelect.value = true
    return
  }

  // For other platforms, execute directly
  executeCcsImport(row, platform === 'gemini' ? 'gemini' : 'claude')
}

const executeCcsImport = (row: ApiKey, clientType: CcSwitchClientType) => {
  const baseUrl = publicSettings.value?.api_base_url || window.location.origin
  const platform = row.group?.platform || 'anthropic'

  const usageScript = `({
    request: {
      url: "{{baseUrl}}/v1/usage",
      method: "GET",
      headers: { "Authorization": "Bearer {{apiKey}}" }
    },
    extractor: function(response) {
      const remaining = response?.remaining ?? response?.quota?.remaining ?? response?.balance;
      const unit = response?.unit ?? response?.quota?.unit ?? "USD";
      return {
        isValid: response?.is_active ?? response?.isValid ?? true,
        remaining,
        unit
      };
    }
  })`
  const providerName = (publicSettings.value?.site_name || 'sub2api').trim() || 'sub2api'
  const deeplink = buildCcSwitchImportDeeplink({
    baseUrl,
    platform,
    clientType,
    providerName,
    apiKey: row.key,
    usageScript
  })

  try {
    window.open(deeplink, '_self')

    // Check if the protocol handler worked by detecting if we're still focused
    setTimeout(() => {
      if (document.hasFocus()) {
        // Still focused means the protocol handler likely failed
        appStore.showError(t('keys.ccSwitchNotInstalled'))
      }
    }, 100)
  } catch (error) {
    appStore.showError(t('keys.ccSwitchNotInstalled'))
  }
}

const handleCcsClientSelect = (clientType: CcSwitchClientType) => {
  if (pendingCcsRow.value) {
    executeCcsImport(pendingCcsRow.value, clientType)
  }
  showCcsClientSelect.value = false
  pendingCcsRow.value = null
}

const closeCcsClientSelect = () => {
  showCcsClientSelect.value = false
  pendingCcsRow.value = null
}

function formatResetTime(resetAt: string | null): string {
  if (!resetAt) return ''
  const diff = new Date(resetAt).getTime() - now.value.getTime()
  if (diff <= 0) return t('keys.resetNow')
  const days = Math.floor(diff / 86400000)
  const hours = Math.floor((diff % 86400000) / 3600000)
  const mins = Math.floor((diff % 3600000) / 60000)
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${mins}m`
  return `${mins}m`
}

onMounted(() => {
  loadSavedColumns()
  if (shouldOpenCreateFromQuery()) {
    showCreateModal.value = true
  }
  loadApiKeys()
  loadGroups()
  loadUserGroupRates()
  loadPublicSettings()
  document.addEventListener('click', closeGroupSelector)
  resetTimer = setInterval(() => { now.value = new Date() }, 60000)
})

onUnmounted(() => {
  document.removeEventListener('click', closeGroupSelector)
  if (resetTimer) clearInterval(resetTimer)
})
</script>

<style scoped>
.keys-market {
  --bg-card-glow: rgba(59, 130, 246, 0.03);
  --border-glow: #3b82f6;

  --plan-bg: rgba(239, 246, 255, 0.55);
  --plan-border: rgba(191, 219, 254, 0.6);
  --plan-text: #1d4ed8;

  --balance-bg: rgba(240, 253, 244, 0.55);
  --balance-border: rgba(187, 247, 208, 0.6);
  --balance-text: #15803d;

  --group-bg: rgba(254, 243, 199, 0.5);
  --group-border: rgba(253, 230, 138, 0.6);
  --group-text: #b45309;
}

/* Card Button styles for cells */
.keys-market :deep(.entitlement-card-btn) {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  min-width: 250px;
  max-width: 320px;
  border-radius: 10px;
  border: 1px solid rgba(226, 232, 240, 0.8);
  background: #ffffff;
  padding: 10px 14px;
  text-align: left;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.02);
  transition: transform 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease, background 0.2s ease;
}

.keys-market :deep(.entitlement-card-btn):hover {
  transform: translateY(-1.5px);
  box-shadow: 0 4px 12px rgba(59, 130, 246, 0.05);
}

.keys-market :deep(.entitlement-card-btn.plan) {
  background: var(--plan-bg);
  border-color: var(--plan-border);
}

.keys-market :deep(.entitlement-card-btn.plan):hover {
  border-color: var(--border-glow);
  box-shadow: 0 4px 12px rgba(59, 130, 246, 0.08);
}

.keys-market :deep(.entitlement-card-btn.balance) {
  border-style: dashed;
  background: var(--balance-bg);
  border-color: var(--balance-border);
}

.keys-market :deep(.entitlement-card-btn.balance):hover {
  border-style: solid;
  border-color: #10b981;
  box-shadow: 0 4px 12px rgba(16, 185, 129, 0.08);
}

.keys-market :deep(.entitlement-card-btn.group-select-card) {
  background: var(--group-bg);
  border-color: var(--group-border);
}

.keys-market :deep(.entitlement-card-btn.group-select-card):hover {
  border-color: #f59e0b;
  box-shadow: 0 4px 12px rgba(245, 158, 11, 0.08);
}

/* Badge style inside card */
.keys-market :deep(.badge-tag) {
  display: inline-flex;
  align-items: center;
  border-radius: 4px;
  padding: 1.5px 6px;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.02em;
}

.keys-market :deep(.plan-badge) {
  background: rgba(59, 130, 246, 0.12);
  color: var(--plan-text);
}

.keys-market :deep(.balance-badge) {
  background: rgba(16, 185, 129, 0.12);
  color: var(--balance-text);
}

.keys-market :deep(.group-badge) {
  background: rgba(245, 158, 11, 0.12);
  color: var(--group-text);
}

/* Action Trigger Text */
.keys-market :deep(.action-trigger-text) {
  flex-shrink: 0;
  font-size: 11px;
  font-weight: 700;
  color: var(--border-glow);
  opacity: 0.85;
  transition: opacity 0.2s;
}

.keys-market :deep(.entitlement-card-btn):hover .action-trigger-text {
  opacity: 1;
  text-decoration: underline;
}

/* Progress bar styles */
.keys-market :deep(.progress-bar-track) {
  position: relative;
  height: 6px;
  width: 100%;
  overflow: hidden;
  border-radius: 9999px;
  background: #e2e8f0;
}

.keys-market :deep(.progress-bar-track.thin) {
  height: 4px;
}

.keys-market :deep(.progress-bar-fill) {
  height: 100%;
  border-radius: 9999px;
  transition: width 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.keys-market :deep(.progress-bar-fill.primary) {
  background: linear-gradient(90deg, #6366f1, #8b5cf6);
}

.keys-market :deep(.progress-bar-fill.success) {
  background: linear-gradient(90deg, #10b981, #059669);
}

.keys-market :deep(.progress-bar-fill.warning) {
  background: linear-gradient(90deg, #f59e0b, #d97706);
}

.keys-market :deep(.progress-bar-fill.danger) {
  background: linear-gradient(90deg, #ef4444, #dc2626);
}

/* Dialog: Access Source Cards styling */
.access-source-card {
  display: flex;
  min-width: 0;
  align-items: start;
  gap: 12px;
  border-radius: 10px;
  border: 1.5px solid #e2e8f0;
  background: #ffffff;
  padding: 14px;
  text-align: left;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.02);
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
}

.access-source-card .icon {
  color: #64748b;
  transition: color 0.2s;
}

.access-source-card .desc-text {
  color: #64748b;
}

.access-source-card:hover:not(:disabled) {
  border-color: #cbd5e1;
  background: #f8fafc;
}

.access-source-card:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.access-source-card.active {
  border-color: #3b82f6;
  background: rgba(59, 130, 246, 0.03);
  box-shadow: 0 4px 12px rgba(59, 130, 246, 0.05);
}

.access-source-card.active .icon {
  color: #3b82f6;
}

.access-source-card.balance.active {
  border-color: #10b981;
  background: rgba(16, 185, 129, 0.03);
  box-shadow: 0 4px 12px rgba(16, 185, 129, 0.05);
}

.access-source-card.balance.active .icon {
  color: #10b981;
}
</style>

<style>
/* Unscoped dark mode overrides for KeysView specifically prefixed to avoid Vue compiler stripping */
.dark .keys-market {
  --bg-card-glow: rgba(56, 189, 248, 0.02);
  --border-glow: #38bdf8;

  --plan-bg: rgba(56, 189, 248, 0.04);
  --plan-border: rgba(56, 189, 248, 0.18);
  --plan-text: #38bdf8;

  --balance-bg: rgba(52, 211, 153, 0.04);
  --balance-border: rgba(52, 211, 153, 0.18);
  --balance-text: #34d399;

  --group-bg: rgba(251, 191, 36, 0.04);
  --group-border: rgba(251, 191, 36, 0.18);
  --group-text: #fbbf24;
}

.dark .keys-market .entitlement-card-btn {
  border-color: rgba(255, 255, 255, 0.05);
  background: #14161b;
}

.dark .keys-market .progress-bar-track {
  background: #1f2937;
}

/* Dialog dark overrides */
.dark .access-source-card {
  border-color: rgba(255, 255, 255, 0.07);
  background: #14161b;
}

.dark .access-source-card:hover:not(:disabled) {
  border-color: rgba(255, 255, 255, 0.14);
  background: #181b21;
}

.dark .access-source-card.active {
  border-color: #38bdf8;
  background: rgba(56, 189, 248, 0.06);
}

.dark .access-source-card.balance.active {
  border-color: #34d399;
  background: rgba(52, 211, 153, 0.06);
}

.dark .access-source-card .desc-text {
  color: #94a3b8;
}
</style>
