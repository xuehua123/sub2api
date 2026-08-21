<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <!-- Top Toolbar: Left (search + filters) / Right (actions) -->
        <div class="flex flex-wrap items-start justify-between gap-4">
          <!-- Left: Fuzzy user search + filters (wrap to multiple lines) -->
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <!-- User Search -->
            <div
              class="relative w-full sm:w-64"
              data-filter-user-search
            >
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
              />
              <input
                v-model="filterUserKeyword"
                type="text"
                :placeholder="t('admin.users.searchUsers')"
                class="input pl-10 pr-8"
                @input="debounceSearchFilterUsers"
                @focus="showFilterUserDropdown = true"
              />
              <button
                v-if="selectedFilterUser"
                @click="clearFilterUser"
                type="button"
                class="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                :title="t('common.clear')"
              >
                <Icon name="x" size="sm" :stroke-width="2" />
              </button>

              <!-- User Dropdown -->
              <div
                v-if="showFilterUserDropdown && (filterUserResults.length > 0 || filterUserKeyword)"
                class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800"
              >
                <div
                  v-if="filterUserLoading"
                  class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
                >
                  {{ t('common.loading') }}
                </div>
                <div
                  v-else-if="filterUserResults.length === 0 && filterUserKeyword"
                  class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
                >
                  {{ t('common.noOptionsFound') }}
                </div>
                <button
                  v-for="user in filterUserResults"
                  :key="user.id"
                  type="button"
                  @click="selectFilterUser(user)"
                  class="w-full px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700"
                >
                  <span class="font-medium text-gray-900 dark:text-white">{{ user.email }}</span>
                  <span class="ml-2 text-gray-500 dark:text-gray-400">#{{ user.id }}</span>
                </button>
              </div>
            </div>

            <!-- Filters -->
            <div class="w-full sm:w-40">
              <Select
                v-model="filters.status"
                :options="statusOptions"
                :placeholder="t('admin.subscriptions.allStatus')"
                @change="applyFilters"
              />
            </div>
            <div class="w-full sm:w-48">
              <Select
                v-model="filters.plan_id"
                :options="planFilterOptions"
                :placeholder="t('admin.subscriptions.allPlans')"
                @change="applyFilters"
              />
            </div>
            <div class="w-full sm:w-40">
              <Select
                v-model="filters.platform"
                :options="platformFilterOptions"
                :placeholder="t('admin.subscriptions.allPlatforms')"
                @change="applyFilters"
              />
            </div>
          </div>

          <!-- Right: Actions -->
          <div class="ml-auto flex flex-wrap items-center justify-end gap-3">
            <button
              @click="loadSubscriptions"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <!-- Column Settings Dropdown -->
            <div class="relative" ref="columnDropdownRef">
              <button
                @click="showColumnDropdown = !showColumnDropdown"
                class="btn btn-secondary px-2 md:px-3"
                :title="t('admin.users.columnSettings')"
              >
                <svg class="h-4 w-4 md:mr-1.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9 4.5v15m6-15v15m-10.875 0h15.75c.621 0 1.125-.504 1.125-1.125V5.625c0-.621-.504-1.125-1.125-1.125H4.125C3.504 4.5 3 5.004 3 5.625v12.75c0 .621.504 1.125 1.125 1.125z" />
                </svg>
                <span class="hidden md:inline">{{ t('admin.users.columnSettings') }}</span>
              </button>
              <!-- Dropdown menu -->
              <div
                v-if="showColumnDropdown"
                class="absolute right-0 z-50 mt-2 w-48 origin-top-right rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800"
              >
                <div class="p-2">
                  <!-- User column mode selection -->
                  <div class="mb-2 border-b border-gray-200 pb-2 dark:border-dark-700">
                    <div class="px-3 py-1 text-xs font-medium text-gray-500 dark:text-gray-400">
                      {{ t('admin.subscriptions.columns.user') }}
                    </div>
                    <button
                      @click="setUserColumnMode('email')"
                      class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-dark-700"
                    >
                      <span>{{ t('admin.users.columns.email') }}</span>
                      <Icon v-if="userColumnMode === 'email'" name="check" size="sm" class="text-primary-500" />
                    </button>
                    <button
                      @click="setUserColumnMode('username')"
                      class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-dark-700"
                    >
                      <span>{{ t('admin.users.columns.username') }}</span>
                      <Icon v-if="userColumnMode === 'username'" name="check" size="sm" class="text-primary-500" />
                    </button>
                  </div>
                  <!-- Other columns toggle -->
                  <button
                    v-for="col in toggleableColumns"
                    :key="col.key"
                    @click="toggleColumn(col.key)"
                    class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-dark-700"
                  >
                    <span>{{ col.label }}</span>
                    <Icon v-if="isColumnVisible(col.key)" name="check" size="sm" class="text-primary-500" />
                  </button>
                </div>
              </div>
            </div>
            <button
              @click="showGuideModal = true"
              class="btn btn-secondary"
              :title="t('admin.subscriptions.guide.showGuide')"
            >
              <Icon name="questionCircle" size="md" />
            </button>
            <button @click="showAssignModal = true" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.subscriptions.assignSubscription') }}
            </button>
          </div>
        </div>
      </template>

      <!-- Subscriptions Table -->
      <template #table>
        <DataTable
          :columns="columns"
          :data="displaySubscriptions"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-user="{ row }">
            <div class="flex items-center gap-2">
              <div
                class="flex h-8 w-8 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30"
              >
                <span class="text-sm font-medium text-primary-700 dark:text-primary-300">
                  {{ userColumnMode === 'email'
                    ? (row.user?.email?.charAt(0).toUpperCase() || '?')
                    : (row.user?.username?.charAt(0).toUpperCase() || '?')
                  }}
                </span>
              </div>
              <span class="font-medium text-gray-900 dark:text-white">
                {{ userColumnMode === 'email'
                  ? (row.user?.email || t('admin.redeem.userPrefix', { id: row.user_id }))
                  : (row.user?.username || '-')
                }}
              </span>
            </div>
          </template>

          <template #cell-plan="{ row }">
            <div class="min-w-[180px]">
              <div class="font-medium text-gray-900 dark:text-white">
                {{ subscriptionDisplayName(row, subscriptionPlans) }}
              </div>
              <div
                v-if="resolveSubscriptionPlan(row)"
                class="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-gray-500 dark:text-dark-300"
              >
                <span>{{ t('admin.subscriptions.planId', { id: resolveSubscriptionPlan(row)?.id }) }}</span>
                <span>/</span>
                <span>{{ formatPlanValidity(getPlanValidityDays(resolveSubscriptionPlan(row)?.validity_days || 0, resolveSubscriptionPlan(row)?.validity_unit)) }}</span>
              </div>
              <div v-else class="mt-1 text-xs text-gray-500 dark:text-dark-300">
                {{ t('admin.subscriptions.legacyGroup') }}
              </div>
            </div>
          </template>

          <template #cell-usage="{ row }">
            <div class="min-w-[280px] space-y-2">
              <!-- Daily Usage -->
              <div v-if="getSubscriptionLimit(row, 'daily') !== null" class="usage-row">
                <div class="flex items-center gap-2">
                  <span class="usage-label">{{ t('admin.subscriptions.daily') }}</span>
                  <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="getProgressClass(row.daily_usage_usd, getSubscriptionLimit(row, 'daily'))"
                      :style="{
                        width: getProgressWidth(row.daily_usage_usd, getSubscriptionLimit(row, 'daily'))
                      }"
                    ></div>
                  </div>
                  <span class="usage-amount">
                    ${{ row.daily_usage_usd?.toFixed(2) || '0.00' }}
                    <span class="text-gray-400">/</span>
                    ${{ getSubscriptionLimit(row, 'daily')?.toFixed(2) }}
                  </span>
                </div>
                <div class="reset-info">
                  <svg
                    class="h-3 w-3"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  <span>{{ formatDailyUsageWindow(row) }}</span>
                </div>
              </div>

              <!-- Weekly Usage -->
              <div v-if="getSubscriptionLimit(row, 'weekly') !== null" class="usage-row">
                <div class="flex items-center gap-2">
                  <span class="usage-label">{{ t('admin.subscriptions.weekly') }}</span>
                  <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="getProgressClass(row.weekly_usage_usd, getSubscriptionLimit(row, 'weekly'))"
                      :style="{
                        width: getProgressWidth(row.weekly_usage_usd, getSubscriptionLimit(row, 'weekly'))
                      }"
                    ></div>
                  </div>
                  <span class="usage-amount">
                    ${{ row.weekly_usage_usd?.toFixed(2) || '0.00' }}
                    <span class="text-gray-400">/</span>
                    ${{ getSubscriptionLimit(row, 'weekly')?.toFixed(2) }}
                  </span>
                </div>
                <div class="reset-info">
                  <svg
                    class="h-3 w-3"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  <span>{{ formatResetTime(row.weekly_window_start, row.starts_at, 'weekly') }}</span>
                </div>
              </div>

              <!-- Monthly Usage -->
              <div v-if="getSubscriptionLimit(row, 'monthly') !== null" class="usage-row">
                <div class="flex items-center gap-2">
                  <span class="usage-label">{{ t('admin.subscriptions.monthly') }}</span>
                  <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="getProgressClass(row.monthly_usage_usd, getSubscriptionLimit(row, 'monthly'))"
                      :style="{
                        width: getProgressWidth(row.monthly_usage_usd, getSubscriptionLimit(row, 'monthly'))
                      }"
                    ></div>
                  </div>
                  <span class="usage-amount">
                    ${{ row.monthly_usage_usd?.toFixed(2) || '0.00' }}
                    <span class="text-gray-400">/</span>
                    ${{ getSubscriptionLimit(row, 'monthly')?.toFixed(2) }}
                  </span>
                </div>
                <div class="reset-info">
                  <svg
                    class="h-3 w-3"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  <span>{{ formatResetTime(row.monthly_window_start, row.starts_at, 'monthly') }}</span>
                </div>
              </div>

              <!-- No Limits - Unlimited badge -->
              <div
                v-if="
                  getSubscriptionLimit(row, 'daily') === null &&
                  getSubscriptionLimit(row, 'weekly') === null &&
                  getSubscriptionLimit(row, 'monthly') === null
                "
                class="flex items-center gap-2 rounded-lg bg-gradient-to-r from-emerald-50 to-teal-50 px-3 py-2 dark:from-emerald-900/20 dark:to-teal-900/20"
              >
                <span class="text-lg text-emerald-600 dark:text-emerald-400">∞</span>
                <span class="text-xs font-medium text-emerald-700 dark:text-emerald-300">
                  {{ t('admin.subscriptions.unlimited') }}
                </span>
              </div>
            </div>
          </template>

          <template #cell-expires_at="{ value }">
            <div v-if="value">
              <span
                class="text-sm"
                :class="
                  isExpiringSoon(value)
                    ? 'text-orange-600 dark:text-orange-400'
                    : 'text-gray-700 dark:text-gray-300'
                "
              >
                {{ formatDateTimeToMinute(value) }}
              </span>
              <template
                v-for="remainingExpiry in [formatRemainingExpiry(value)]"
                :key="remainingExpiry ?? 'expired'"
              >
                <div v-if="remainingExpiry" class="text-xs text-gray-500">
                  {{ remainingExpiry }}
                </div>
              </template>
            </div>
            <span v-else class="text-sm text-gray-500">{{
              t('admin.subscriptions.noExpiration')
            }}</span>
          </template>

          <template #cell-status="{ value }">
            <span
              :class="[
                'badge',
                value === 'active'
                  ? 'badge-success'
                  : value === 'expired'
                    ? 'badge-warning'
                    : 'badge-danger'
              ]"
            >
              {{ t(`admin.subscriptions.status.${value}`) }}
            </span>
          </template>

          <template #cell-actions="{ row }">
            <div v-if="canManageSubscriptionRow(row)" class="flex flex-wrap items-center gap-1">
              <span
                v-if="row.entitlement_only"
                class="mr-1 inline-flex items-center rounded-md bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300"
              >
                {{ t('admin.subscriptions.entitlementOnly') }}
              </span>
              <button
                v-if="row.status === 'active' || row.status === 'expired'"
                @click="handleExtend(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
              >
                <Icon name="calendar" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.adjust') }}</span>
              </button>
              <button
                v-if="row.status === 'active'"
                @click="handleCycleAdjustment(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-teal-50 hover:text-teal-600 dark:hover:bg-teal-900/20 dark:hover:text-teal-400"
              >
                <Icon name="clock" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.cycleAdjustment') }}</span>
              </button>
              <button
                v-if="row.status === 'active'"
                @click="handleResetQuota(row)"
                :disabled="resettingQuota && resettingSubscription?.id === row.id"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-orange-50 hover:text-orange-600 dark:hover:bg-orange-900/20 dark:hover:text-orange-400 disabled:cursor-not-allowed disabled:opacity-50"
              >
                <Icon name="refresh" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.resetQuota') }}</span>
              </button>
              <button
                v-if="row.status === 'active'"
                @click="handleRevoke(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              >
                <Icon name="ban" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.revoke') }}</span>
              </button>
              <button
                v-if="row.status === 'revoked'"
                @click="handleRestore(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-green-50 hover:text-green-600 dark:hover:bg-green-900/20 dark:hover:text-green-400"
              >
                <Icon name="refresh" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.restore') }}</span>
              </button>
            </div>
            <span
              v-else
              class="inline-flex items-center rounded-md bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300"
            >
              {{ t('admin.subscriptions.entitlementOnly') }}
            </span>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.subscriptions.noSubscriptionsYet')"
              :description="t('admin.subscriptions.assignFirstSubscription')"
              :action-text="t('admin.subscriptions.assignSubscription')"
              @action="showAssignModal = true"
            />
          </template>
        </DataTable>
      </template>

      <!-- Pagination -->
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

    <!-- Assign Subscription Modal -->
    <BaseDialog
      :show="showAssignModal"
      :title="t('admin.subscriptions.assignSubscription')"
      width="normal"
      @close="closeAssignModal"
    >
      <form
        id="assign-subscription-form"
        @submit.prevent="handleAssignSubscription"
        class="space-y-5"
      >
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.user') }}</label>
          <div class="relative" data-assign-user-search>
            <input
              v-model="userSearchKeyword"
              type="text"
              class="input pr-8"
              :placeholder="t('admin.usage.searchUserPlaceholder')"
              @input="debounceSearchUsers"
              @focus="showUserDropdown = true"
            />
            <button
              v-if="selectedUser"
              @click="clearUserSelection"
              type="button"
              class="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
            >
              <Icon name="x" size="sm" :stroke-width="2" />
            </button>
            <!-- User Dropdown -->
            <div
              v-if="showUserDropdown && (userSearchResults.length > 0 || userSearchKeyword)"
              class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800"
            >
              <div
                v-if="userSearchLoading"
                class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
              >
                {{ t('common.loading') }}
              </div>
              <div
                v-else-if="userSearchResults.length === 0 && userSearchKeyword"
                class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
              >
                {{ t('common.noOptionsFound') }}
              </div>
              <button
                v-for="user in userSearchResults"
                :key="user.id"
                type="button"
                @click="selectUser(user)"
                class="w-full px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700"
              >
                <span class="font-medium text-gray-900 dark:text-white">{{ user.email }}</span>
                <span class="ml-2 text-gray-500 dark:text-gray-400">#{{ user.id }}</span>
              </button>
            </div>
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.plan') }}</label>
          <Select
            v-model="assignForm.plan_id"
            :options="subscriptionPlanOptions"
            :placeholder="t('admin.subscriptions.selectPlan')"
            @change="handleAssignPlanChange"
          >
            <template #selected="{ option }">
              <div v-if="option" class="min-w-0">
                <div class="truncate font-medium">{{ (option as unknown as PlanOption).label }}</div>
                <div class="truncate text-xs text-gray-500 dark:text-dark-300">
                  {{ formatPlanValidity((option as unknown as PlanOption).validityDays) }}
                </div>
              </div>
              <span v-else class="text-gray-400">{{ t('admin.subscriptions.selectPlan') }}</span>
            </template>
            <template #option="{ option, selected }">
              <div class="flex w-full items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="truncate font-medium text-gray-900 dark:text-white">
                    {{ (option as unknown as PlanOption).label }}
                  </div>
                  <div class="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-gray-500 dark:text-dark-300">
                    <span>{{ t('admin.subscriptions.planId', { id: (option as unknown as PlanOption).planId }) }}</span>
                    <span>/</span>
                    <span>{{ formatPlanValidity((option as unknown as PlanOption).validityDays) }}</span>
                    <span v-if="(option as unknown as PlanOption).groupCount > 1">/</span>
                    <span v-if="(option as unknown as PlanOption).groupCount > 1">
                      {{ t('admin.subscriptions.planGroupCount', { count: (option as unknown as PlanOption).groupCount }) }}
                    </span>
                  </div>
                </div>
                <Icon v-if="selected" name="check" size="sm" class="mt-0.5 flex-shrink-0 text-primary-500" />
              </div>
            </template>
          </Select>
          <p class="input-hint">{{ t('admin.subscriptions.planHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.validityDays') }}</label>
          <input v-model.number="assignForm.validity_days" type="number" min="1" :max="MAX_PLAN_VALIDITY_DAYS" class="input" />
          <p class="input-hint">{{ t('admin.subscriptions.validityHint') }}</p>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeAssignModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
          <button
            type="submit"
            form="assign-subscription-form"
            :disabled="submitting"
            class="btn btn-primary"
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
            {{ submitting ? t('admin.subscriptions.assigning') : t('admin.subscriptions.assign') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Adjust Subscription Modal -->
    <BaseDialog
      :show="showExtendModal"
      :title="t('admin.subscriptions.adjustSubscription')"
      width="narrow"
      @close="closeExtendModal"
    >
      <form
        v-if="extendingSubscription"
        id="extend-subscription-form"
        @submit.prevent="handleExtendSubscription"
        class="space-y-5"
      >
        <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700">
          <p class="text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.subscriptions.adjustingFor') }}
            <span class="font-medium text-gray-900 dark:text-white">{{
              extendingSubscription.user?.email
            }}</span>
          </p>
          <p class="mt-1 text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.subscriptions.currentExpiration') }}:
            <span class="font-medium text-gray-900 dark:text-white">
              {{
                extendingSubscription.expires_at
                  ? formatDateTimeToMinute(extendingSubscription.expires_at)
                  : t('admin.subscriptions.noExpiration')
              }}
            </span>
          </p>
          <p v-if="extendingSubscription.expires_at" class="mt-1 text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.subscriptions.remainingDays') }}:
            <span class="font-medium text-gray-900 dark:text-white">
              {{ formatExpirationRemaining(extendingSubscription.expires_at) || '0m' }}
            </span>
          </p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.adjustDays') }}</label>
          <div class="flex items-center gap-2">
            <input
              v-model.number="extendForm.days"
              type="number"
              required
              class="input text-center"
              :placeholder="t('admin.subscriptions.adjustDaysPlaceholder')"
            />
          </div>
          <p class="input-hint">{{ t('admin.subscriptions.adjustHint') }}</p>
        </div>
      </form>
      <template #footer>
        <div v-if="extendingSubscription" class="flex justify-end gap-3">
          <button @click="closeExtendModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
          <button
            type="submit"
            form="extend-subscription-form"
            :disabled="submitting"
            class="btn btn-primary"
          >
            {{ submitting ? t('admin.subscriptions.adjusting') : t('admin.subscriptions.adjust') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Monthly Cycle Adjustment Modal -->
    <BaseDialog
      :show="showCycleAdjustmentModal"
      :title="t('admin.subscriptions.cycleAdjustmentTitle')"
      width="wide"
      @close="closeCycleAdjustmentModal"
    >
      <form
        v-if="cycleAdjustmentSubscription"
        id="cycle-adjustment-form"
        class="space-y-5"
        @submit.prevent="previewCycleAdjustment"
      >
        <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p class="text-sm text-gray-600 dark:text-gray-400">
                {{ t('admin.subscriptions.cycleAdjustmentFor') }}
                <span class="font-medium text-gray-900 dark:text-white">
                  {{ cycleAdjustmentSubscription.user?.email || t('admin.redeem.userPrefix', { id: cycleAdjustmentSubscription.user_id }) }}
                </span>
              </p>
              <p class="mt-1 text-sm text-gray-600 dark:text-gray-400">
                {{ subscriptionDisplayName(cycleAdjustmentSubscription, subscriptionPlans) }}
              </p>
            </div>
            <span
              v-if="cycleAdjustmentSubscription.entitlement_id || cycleAdjustmentSubscription.entitlement_only"
              class="inline-flex items-center rounded-md bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300"
            >
              {{ t('admin.subscriptions.entitlementOnly') }}
            </span>
          </div>
        </div>

        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.subscriptions.cycleAdjustmentMode') }}</label>
            <select
              v-model="cycleAdjustmentForm.mode"
              class="input"
              @change="handleCycleAdjustmentModeChange"
            >
              <option
                v-for="option in cycleAdjustmentModeOptions"
                :key="option.value"
                :value="option.value"
              >
                {{ option.label }}
              </option>
            </select>
            <p class="input-hint">{{ cycleAdjustmentModeHint }}</p>
          </div>

          <div v-if="cycleAdjustmentNeedsCycleCount">
            <label class="input-label">{{ t('admin.subscriptions.cycleCount') }}</label>
            <input
              v-model.number="cycleAdjustmentForm.cycle_count"
              type="number"
              min="1"
              :max="cycleAdjustmentMaxCycleCount"
              class="input"
              @input="clearCycleAdjustmentPreview"
            />
            <p class="input-hint">{{ t('admin.subscriptions.cycleCountHint') }}</p>
          </div>

          <div v-if="cycleAdjustmentNeedsCustomTimes">
            <label class="input-label">{{ t('admin.subscriptions.customMonthlyWindowStart') }}</label>
            <input
              v-model="cycleAdjustmentForm.custom_monthly_window_start"
              type="datetime-local"
              class="input"
              @input="clearCycleAdjustmentPreview"
            />
          </div>

          <div v-if="cycleAdjustmentNeedsCustomTimes">
            <label class="input-label">{{ t('admin.subscriptions.customExpiresAt') }}</label>
            <input
              v-model="cycleAdjustmentForm.custom_expires_at"
              type="datetime-local"
              class="input"
              @input="clearCycleAdjustmentPreview"
            />
          </div>

          <label class="flex items-start gap-3 rounded-lg border border-gray-200 bg-white p-3 text-sm dark:border-dark-600 dark:bg-dark-800">
            <input
              v-model="cycleAdjustmentForm.reset_monthly_usage"
              type="checkbox"
              class="mt-1 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              :disabled="cycleAdjustmentForcesUsageReset"
              @change="clearCycleAdjustmentPreview"
            />
            <span>
              <span class="block font-medium text-gray-900 dark:text-white">
                {{ t('admin.subscriptions.resetMonthlyUsage') }}
              </span>
              <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">
                {{ cycleAdjustmentForcesUsageReset ? t('admin.subscriptions.resetMonthlyUsageForcedHint') : t('admin.subscriptions.resetMonthlyUsageOptionalHint') }}
              </span>
            </span>
          </label>
        </div>

        <div>
          <label class="input-label">
            {{ t('admin.subscriptions.cycleAdjustmentReason') }}
            <span v-if="cycleAdjustmentReasonRequired" class="text-red-500">*</span>
          </label>
          <textarea
            v-model.trim="cycleAdjustmentForm.reason"
            rows="2"
            :maxlength="cycleAdjustmentReasonMaxLength"
            class="input resize-none"
            :placeholder="t('admin.subscriptions.cycleAdjustmentReasonPlaceholder')"
            @input="clearCycleAdjustmentPreview"
          ></textarea>
        </div>
      </form>

      <div
        v-if="cycleAdjustmentPreview"
        class="mt-5 space-y-4 rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800"
      >
        <div class="grid gap-3 md:grid-cols-2">
          <div class="space-y-3">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.subscriptions.beforeAdjustment') }}
            </h3>
            <div class="space-y-2 text-sm text-gray-600 dark:text-gray-300">
              <div class="flex justify-between gap-3">
                <span>{{ t('admin.subscriptions.expiresAt') }}</span>
                <span class="text-right font-medium text-gray-900 dark:text-white">{{ formatAdjustmentDateTime(cycleAdjustmentPreview.current_expires_at) }}</span>
              </div>
              <div class="flex justify-between gap-3">
                <span>{{ t('admin.subscriptions.monthlyWindowStart') }}</span>
                <span class="text-right font-medium text-gray-900 dark:text-white">{{ formatAdjustmentDateTime(cycleAdjustmentPreview.current_monthly_window_start) }}</span>
              </div>
              <div class="flex justify-between gap-3">
                <span>{{ t('admin.subscriptions.nextResetAt') }}</span>
                <span class="text-right font-medium text-gray-900 dark:text-white">{{ formatAdjustmentDateTime(cycleAdjustmentPreview.current_reset_at) }}</span>
              </div>
              <div class="flex justify-between gap-3">
                <span>{{ t('admin.subscriptions.monthlyUsage') }}</span>
                <span class="text-right font-medium text-gray-900 dark:text-white">
                  {{ formatAdjustmentUsage(cycleAdjustmentPreview.current_monthly_usage_usd, cycleAdjustmentPreview.monthly_limit_usd) }}
                </span>
              </div>
            </div>
          </div>

          <div class="space-y-3">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.subscriptions.afterAdjustment') }}
            </h3>
            <div class="space-y-2 text-sm text-gray-600 dark:text-gray-300">
              <div class="flex justify-between gap-3">
                <span>{{ t('admin.subscriptions.expiresAt') }}</span>
                <span class="text-right font-medium text-gray-900 dark:text-white">{{ formatAdjustmentDateTime(cycleAdjustmentPreview.new_expires_at) }}</span>
              </div>
              <div class="flex justify-between gap-3">
                <span>{{ t('admin.subscriptions.monthlyWindowStart') }}</span>
                <span class="text-right font-medium text-gray-900 dark:text-white">{{ formatAdjustmentDateTime(cycleAdjustmentPreview.new_monthly_window_start) }}</span>
              </div>
              <div class="flex justify-between gap-3">
                <span>{{ t('admin.subscriptions.nextResetAt') }}</span>
                <span class="text-right font-medium text-gray-900 dark:text-white">{{ formatAdjustmentDateTime(cycleAdjustmentPreview.new_reset_at) }}</span>
              </div>
              <div class="flex justify-between gap-3">
                <span>{{ t('admin.subscriptions.monthlyUsage') }}</span>
                <span class="text-right font-medium text-gray-900 dark:text-white">
                  {{ formatAdjustmentUsage(cycleAdjustmentPreview.new_monthly_usage_usd, cycleAdjustmentPreview.monthly_limit_usd) }}
                </span>
              </div>
            </div>
          </div>
        </div>

        <div class="grid gap-3 border-t border-gray-200 pt-3 text-sm dark:border-dark-600 md:grid-cols-3">
          <div>
            <span class="text-gray-500 dark:text-gray-400">{{ t('admin.subscriptions.deductedValidity') }}</span>
            <div class="mt-1 font-medium text-gray-900 dark:text-white">{{ formatAdjustmentDuration(cycleAdjustmentPreview.deducted_seconds) }}</div>
          </div>
          <div>
            <span class="text-gray-500 dark:text-gray-400">{{ t('admin.subscriptions.fullCycles') }}</span>
            <div class="mt-1 font-medium text-gray-900 dark:text-white">{{ cycleAdjustmentPreview.full_cycles }}</div>
          </div>
          <div>
            <span class="text-gray-500 dark:text-gray-400">{{ t('admin.subscriptions.tailDuration') }}</span>
            <div class="mt-1 font-medium text-gray-900 dark:text-white">{{ formatAdjustmentDuration(cycleAdjustmentPreview.tail_seconds) }}</div>
          </div>
        </div>

        <div
          v-if="!cycleAdjustmentPreview.can_apply"
          class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-900/20 dark:text-red-300"
        >
          {{ formatCycleAdjustmentUnavailable(cycleAdjustmentPreview.unavailable_reason) }}
        </div>

        <div
          v-if="cycleAdjustmentPreview.warnings?.length"
          class="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200"
        >
          <div v-for="warning in cycleAdjustmentPreview.warnings" :key="warning">
            {{ formatCycleAdjustmentWarning(warning) }}
          </div>
        </div>
      </div>

      <template #footer>
        <div v-if="cycleAdjustmentSubscription" class="flex flex-wrap justify-end gap-3">
          <button @click="closeCycleAdjustmentModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
          <button
            type="submit"
            form="cycle-adjustment-form"
            :disabled="cycleAdjustmentLoading || cycleAdjustmentSubmitting"
            class="btn btn-secondary"
          >
            {{ cycleAdjustmentLoading ? t('admin.subscriptions.previewing') : t('admin.subscriptions.previewAdjustment') }}
          </button>
          <button
            type="button"
            :disabled="cycleAdjustmentSubmitting || !cycleAdjustmentPreview?.can_apply"
            class="btn btn-primary"
            @click="applyCycleAdjustment"
          >
            {{ cycleAdjustmentSubmitting ? t('admin.subscriptions.applyingCycleAdjustment') : t('admin.subscriptions.applyCycleAdjustment') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Revoke Confirmation Dialog -->
    <ConfirmDialog
      :show="showRevokeDialog"
      :title="t('admin.subscriptions.revokeSubscription')"
      :message="t('admin.subscriptions.revokeConfirm', { user: revokingSubscription?.user?.email })"
      :confirm-text="t('admin.subscriptions.revoke')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmRevoke"
      @cancel="showRevokeDialog = false"
    />

    <!-- Restore Confirmation Dialog -->
    <ConfirmDialog
      :show="showRestoreDialog"
      :title="t('admin.subscriptions.restoreSubscription')"
      :message="t('admin.subscriptions.restoreConfirm', { user: restoringSubscription?.user?.email })"
      :confirm-text="t('admin.subscriptions.restore')"
      :cancel-text="t('common.cancel')"
      @confirm="confirmRestore"
      @cancel="showRestoreDialog = false"
    />

    <!-- Reset Quota Confirmation Dialog -->
    <ConfirmDialog
      :show="showResetQuotaConfirm"
      :title="t('admin.subscriptions.resetQuotaTitle')"
      :message="t('admin.subscriptions.resetQuotaConfirm', { user: resettingSubscription?.user?.email })"
      :confirm-text="t('admin.subscriptions.resetQuota')"
      :cancel-text="t('common.cancel')"
      @confirm="confirmResetQuota"
      @cancel="showResetQuotaConfirm = false"
    />
    <!-- Subscription Guide Modal -->
    <teleport to="body">
      <transition name="modal">
        <div v-if="showGuideModal" class="fixed inset-0 z-50 flex items-center justify-center p-4" @mousedown.self="showGuideModal = false">
          <div class="fixed inset-0 bg-black/50" @click="showGuideModal = false"></div>
          <div class="relative max-h-[85vh] w-full max-w-2xl overflow-y-auto rounded-xl bg-white p-6 shadow-2xl dark:bg-dark-800">
            <button type="button" class="absolute right-4 top-4 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200" @click="showGuideModal = false">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
            </button>

            <h2 class="mb-4 text-lg font-bold text-gray-900 dark:text-white">{{ t('admin.subscriptions.guide.title') }}</h2>
            <p class="mb-5 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.subscriptions.guide.subtitle') }}</p>

            <!-- Step 1 -->
            <div class="mb-5">
              <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">1</span>
                {{ t('admin.subscriptions.guide.step1.title') }}
              </h3>
              <ol class="ml-8 list-decimal space-y-1 text-sm text-gray-600 dark:text-gray-300">
                <li>{{ t('admin.subscriptions.guide.step1.line1') }}</li>
                <li>{{ t('admin.subscriptions.guide.step1.line2') }}</li>
                <li>{{ t('admin.subscriptions.guide.step1.line3') }}</li>
              </ol>
              <div class="ml-8 mt-2">
                <router-link
                  to="/admin/groups"
                  @click="showGuideModal = false"
                  class="inline-flex items-center gap-1 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                >
                  {{ t('admin.subscriptions.guide.step1.link') }}
                  <Icon name="arrowRight" size="xs" />
                </router-link>
              </div>
            </div>

            <!-- Step 2 -->
            <div class="mb-5">
              <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">2</span>
                {{ t('admin.subscriptions.guide.step2.title') }}
              </h3>
              <ol class="ml-8 list-decimal space-y-1 text-sm text-gray-600 dark:text-gray-300">
                <li>{{ t('admin.subscriptions.guide.step2.line1') }}</li>
                <li>{{ t('admin.subscriptions.guide.step2.line2') }}</li>
                <li>{{ t('admin.subscriptions.guide.step2.line3') }}</li>
              </ol>
            </div>

            <!-- Step 3 -->
            <div class="mb-5">
              <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">3</span>
                {{ t('admin.subscriptions.guide.step3.title') }}
              </h3>
              <div class="ml-8 overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
                <table class="w-full text-sm">
                  <tbody>
                    <tr v-for="(row, i) in guideActionRows" :key="i" class="border-b border-gray-100 dark:border-dark-700 last:border-0">
                      <td class="whitespace-nowrap bg-gray-50 px-3 py-2 font-medium text-gray-700 dark:bg-dark-700 dark:text-gray-300">{{ row.action }}</td>
                      <td class="px-3 py-2 text-gray-600 dark:text-gray-400">{{ row.desc }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>

            <!-- Tip -->
            <div class="rounded-lg bg-blue-50 p-3 text-xs text-blue-700 dark:bg-blue-900/20 dark:text-blue-300">
              {{ t('admin.subscriptions.guide.tip') }}
            </div>

            <div class="mt-4 text-right">
              <button type="button" class="btn btn-primary btn-sm" @click="showGuideModal = false">{{ t('common.close') }}</button>
            </div>
          </div>
        </div>
      </transition>
    </teleport>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type {
  MonthlyCycleAdjustmentMode,
  MonthlyCycleAdjustmentPreview,
  MonthlyCycleAdjustmentRequest,
  UserSubscription
} from '@/types'
import type { SubscriptionPlan } from '@/types/payment'
import type { SimpleUser } from '@/api/admin/usage'
import type { Column } from '@/components/common/types'
import {
	formatDateTime,
	formatDateTimeLocalInput,
	formatDateTimeToMinute,
	parseDateTimeLocalInput
} from '@/utils/format'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  getRemainingDurationParts,
  getRemainingExpiryDuration,
  isOneTimeDailyQuota,
  type RemainingDurationParts
} from '@/utils/subscriptionQuota'
import { formatRemainingDurationCompact, getCycleResetAt, getPlanValidityDays, getRemainingHours, MAX_PLAN_VALIDITY_DAYS } from '@/utils/subscriptionTime'
import {
  normalizeSubscriptionPlans,
  subscriptionDisplayName,
  subscriptionMatchesPlan,
  subscriptionPlanDisplayName,
  subscriptionPlanGroupIDs,
  type RawSubscriptionPlan
} from '@/utils/subscriptionPlanDisplay'
import { GROUP_PLATFORM_OPTIONS } from '@/constants/platforms'

const { t } = useI18n()
const appStore = useAppStore()

interface PlanOption extends Record<string, unknown> {
  value: number
  label: string
  planId: number
  validityDays: number
  groupCount: number
  disabled?: boolean
}

const PLAN_FILTER_FETCH_PAGE_SIZE = 200
type SubscriptionListFilters = NonNullable<Parameters<typeof adminAPI.subscriptions.list>[2]>
type SubscriptionStatusFilter = NonNullable<SubscriptionListFilters['status']>

// Guide modal state
const showGuideModal = ref(false)

const guideActionRows = computed(() => [
  { action: t('admin.subscriptions.guide.actions.adjust'), desc: t('admin.subscriptions.guide.actions.adjustDesc') },
  { action: t('admin.subscriptions.guide.actions.resetQuota'), desc: t('admin.subscriptions.guide.actions.resetQuotaDesc') },
  { action: t('admin.subscriptions.guide.actions.revoke'), desc: t('admin.subscriptions.guide.actions.revokeDesc') }
])

// User column display mode: 'email' or 'username'
const userColumnMode = ref<'email' | 'username'>('email')
const USER_COLUMN_MODE_KEY = 'subscription-user-column-mode'

const loadUserColumnMode = () => {
  try {
    const saved = localStorage.getItem(USER_COLUMN_MODE_KEY)
    if (saved === 'email' || saved === 'username') {
      userColumnMode.value = saved
    }
  } catch (e) {
    console.error('Failed to load user column mode:', e)
  }
}

const saveUserColumnMode = () => {
  try {
    localStorage.setItem(USER_COLUMN_MODE_KEY, userColumnMode.value)
  } catch (e) {
    console.error('Failed to save user column mode:', e)
  }
}

const setUserColumnMode = (mode: 'email' | 'username') => {
  userColumnMode.value = mode
  saveUserColumnMode()
}

// All available columns
const allColumns = computed<Column[]>(() => [
  {
    key: 'user',
    label: userColumnMode.value === 'email'
      ? t('admin.subscriptions.columns.user')
      : t('admin.users.columns.username'),
    sortable: false
  },
  { key: 'plan', label: t('admin.subscriptions.columns.plan'), sortable: false },
  { key: 'usage', label: t('admin.subscriptions.columns.usage'), sortable: false },
  { key: 'expires_at', label: t('admin.subscriptions.columns.expires'), sortable: true },
  { key: 'status', label: t('admin.subscriptions.columns.status'), sortable: true },
  { key: 'actions', label: t('admin.subscriptions.columns.actions'), sortable: false }
])

// Columns that can be toggled (exclude user and actions which are always visible)
const toggleableColumns = computed(() =>
  allColumns.value.filter(col => col.key !== 'user' && col.key !== 'actions')
)

// Hidden columns set
const hiddenColumns = reactive<Set<string>>(new Set())

// Default hidden columns
const DEFAULT_HIDDEN_COLUMNS: string[] = []

// localStorage key
const HIDDEN_COLUMNS_KEY = 'subscription-hidden-columns'

// Load saved column settings
const loadSavedColumns = () => {
  try {
    const saved = localStorage.getItem(HIDDEN_COLUMNS_KEY)
    if (saved) {
      const parsed = JSON.parse(saved) as string[]
      parsed.forEach(key => hiddenColumns.add(key))
    } else {
      DEFAULT_HIDDEN_COLUMNS.forEach(key => hiddenColumns.add(key))
    }
  } catch (e) {
    console.error('Failed to load saved columns:', e)
    DEFAULT_HIDDEN_COLUMNS.forEach(key => hiddenColumns.add(key))
  }
}

// Save column settings to localStorage
const saveColumnsToStorage = () => {
  try {
    localStorage.setItem(HIDDEN_COLUMNS_KEY, JSON.stringify([...hiddenColumns]))
  } catch (e) {
    console.error('Failed to save columns:', e)
  }
}

// Toggle column visibility
const toggleColumn = (key: string) => {
  if (hiddenColumns.has(key)) {
    hiddenColumns.delete(key)
  } else {
    hiddenColumns.add(key)
  }
  saveColumnsToStorage()
}

// Check if column is visible
const isColumnVisible = (key: string) => !hiddenColumns.has(key)

// Filtered columns for display
const columns = computed<Column[]>(() =>
  allColumns.value.filter(col =>
    col.key === 'user' || col.key === 'actions' || !hiddenColumns.has(col.key)
  )
)

// Column dropdown state
const showColumnDropdown = ref(false)
const columnDropdownRef = ref<HTMLElement | null>(null)

// Filter options
const statusOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allStatus') },
  { value: 'active', label: t('admin.subscriptions.status.active') },
  { value: 'expired', label: t('admin.subscriptions.status.expired') },
  { value: 'suspended', label: t('admin.subscriptions.status.suspended') },
  { value: 'revoked', label: t('admin.subscriptions.status.revoked') }
])

const subscriptions = ref<UserSubscription[]>([])
const subscriptionPlans = ref<SubscriptionPlan[]>([])
const loading = ref(false)
let abortController: AbortController | null = null

type SubscriptionQuotaPeriod = 'daily' | 'weekly' | 'monthly'
const MONTHLY_CYCLE_MS = 30 * 24 * 60 * 60 * 1000

// Toolbar user filter (fuzzy search -> select user_id)
const filterUserKeyword = ref('')
const filterUserResults = ref<SimpleUser[]>([])
const filterUserLoading = ref(false)
const showFilterUserDropdown = ref(false)
const selectedFilterUser = ref<SimpleUser | null>(null)
let filterUserSearchTimeout: ReturnType<typeof setTimeout> | null = null

// User search state
const userSearchKeyword = ref('')
const userSearchResults = ref<SimpleUser[]>([])
const userSearchLoading = ref(false)
const showUserDropdown = ref(false)
const selectedUser = ref<SimpleUser | null>(null)
let userSearchTimeout: ReturnType<typeof setTimeout> | null = null

const filters = reactive({
  status: 'active',
  plan_id: '',
  platform: '',
  user_id: null as number | null
})

// Sorting state
const sortState = reactive({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc'
})

const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
})

const showAssignModal = ref(false)
const showExtendModal = ref(false)
const showRevokeDialog = ref(false)
const showRestoreDialog = ref(false)
const showResetQuotaConfirm = ref(false)
const submitting = ref(false)
const resettingSubscription = ref<UserSubscription | null>(null)
const resettingQuota = ref(false)
const extendingSubscription = ref<UserSubscription | null>(null)
const revokingSubscription = ref<UserSubscription | null>(null)
const restoringSubscription = ref<UserSubscription | null>(null)
const showCycleAdjustmentModal = ref(false)
const cycleAdjustmentSubscription = ref<UserSubscription | null>(null)
const cycleAdjustmentPreview = ref<MonthlyCycleAdjustmentPreview | null>(null)
const cycleAdjustmentLoading = ref(false)
const cycleAdjustmentSubmitting = ref(false)
const cycleAdjustmentMaxCycleCount = 1217
const cycleAdjustmentReasonMaxLength = 500

const assignForm = reactive({
  user_id: null as number | null,
  plan_id: null as number | null,
  group_id: null as number | null,
  validity_days: 30
})

const extendForm = reactive({
  days: 30
})

const cycleAdjustmentForm = reactive({
  mode: 'advance_next_cycle' as MonthlyCycleAdjustmentMode,
  cycle_count: 1,
  custom_monthly_window_start: '',
  custom_expires_at: '',
  reset_monthly_usage: true,
  reason: ''
})

const planFilterOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allPlans') },
  ...saleSubscriptionPlans.value.map((plan) => ({
    value: plan.id.toString(),
    label: getPlanDisplayName(plan)
  }))
])

const platformFilterOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allPlatforms') },
  ...GROUP_PLATFORM_OPTIONS
])

const cycleAdjustmentModeOptions = computed<Array<{ value: MonthlyCycleAdjustmentMode; label: string }>>(() => [
  { value: 'advance_next_cycle', label: t('admin.subscriptions.cycleAdjustmentModes.advanceNextCycle') },
  { value: 'compensate_reset', label: t('admin.subscriptions.cycleAdjustmentModes.compensateReset') },
  { value: 'align_to_reset', label: t('admin.subscriptions.cycleAdjustmentModes.alignToReset') },
  { value: 'align_to_expiry', label: t('admin.subscriptions.cycleAdjustmentModes.alignToExpiry') },
  { value: 'custom', label: t('admin.subscriptions.cycleAdjustmentModes.custom') }
])

const cycleAdjustmentNeedsCycleCount = computed(() =>
  cycleAdjustmentForm.mode === 'align_to_reset' || cycleAdjustmentForm.mode === 'align_to_expiry'
)

const cycleAdjustmentNeedsCustomTimes = computed(() => cycleAdjustmentForm.mode === 'custom')
const cycleAdjustmentForcesUsageReset = computed(() =>
  cycleAdjustmentForm.mode === 'advance_next_cycle' || cycleAdjustmentForm.mode === 'compensate_reset'
)
const cycleAdjustmentReasonRequired = computed(() => cycleAdjustmentForm.mode === 'compensate_reset')
const cycleAdjustmentModeHint = computed(() =>
  t(`admin.subscriptions.cycleAdjustmentModeHints.${cycleAdjustmentForm.mode}`)
)

const subscriptionPlanOptions = computed<PlanOption[]>(() =>
  saleSubscriptionPlans.value.map((plan) => {
    const groupIds = getPlanGroupIDs(plan)
    return {
      value: plan.id,
      label: getPlanDisplayName(plan),
      planId: plan.id,
      validityDays: getPlanValidityDays(plan.validity_days, plan.validity_unit),
      groupCount: groupIds.length,
      disabled: groupIds.length === 0
    }
  })
)

const saleSubscriptionPlans = computed(() =>
  subscriptionPlans.value.filter((plan) => plan.for_sale)
)

const planByID = computed(() => {
  const map = new Map<number, SubscriptionPlan>()
  subscriptionPlans.value.forEach((plan) => map.set(plan.id, plan))
  return map
})

const plansByGroupID = computed(() => {
  const map = new Map<number, SubscriptionPlan[]>()
  subscriptionPlans.value.forEach((plan) => {
    getPlanGroupIDs(plan).forEach((groupID) => {
      const existing = map.get(groupID) || []
      existing.push(plan)
      map.set(groupID, existing)
    })
  })
  return map
})

const getPlanDisplayName = (plan: SubscriptionPlan | null | undefined): string => {
  return subscriptionPlanDisplayName(plan) || '-'
}

const getPlanGroupIDs = (plan: SubscriptionPlan | null | undefined): number[] => {
  return subscriptionPlanGroupIDs(plan)
}

const getPlanPrimaryGroupID = (plan: SubscriptionPlan | null | undefined): number | null => {
  return getPlanGroupIDs(plan)[0] || null
}

const listMatchingPlansForSubscription = (subscription: UserSubscription): SubscriptionPlan[] => {
  if (subscription.plan_id) {
    const exactPlan = planByID.value.get(subscription.plan_id)
    return exactPlan ? [exactPlan] : []
  }

  return plansByGroupID.value.get(subscription.group_id) || []
}

const resolveSubscriptionPlan = (subscription: UserSubscription): SubscriptionPlan | null => {
  const matchingPlans = listMatchingPlansForSubscription(subscription)
  return matchingPlans.length === 1 ? matchingPlans[0] : null
}

const pickPreferredPlanRow = (
  rows: UserSubscription[],
  plan: SubscriptionPlan
): UserSubscription => {
  const primaryGroupID = getPlanPrimaryGroupID(plan)
  return [...rows].sort((left, right) => {
    const leftPrimary = left.group_id === primaryGroupID ? 1 : 0
    const rightPrimary = right.group_id === primaryGroupID ? 1 : 0
    if (leftPrimary !== rightPrimary) return rightPrimary - leftPrimary

    const leftExpires = Date.parse(left.expires_at || '') || 0
    const rightExpires = Date.parse(right.expires_at || '') || 0
    if (leftExpires !== rightExpires) return rightExpires - leftExpires

    return right.id - left.id
  })[0]
}

const displaySubscriptions = computed(() => {
  if (subscriptions.value.length <= 1) return subscriptions.value

  const rowsByUser = new Map<number, UserSubscription[]>()
  subscriptions.value.forEach((subscription) => {
    const existing = rowsByUser.get(subscription.user_id) || []
    existing.push(subscription)
    rowsByUser.set(subscription.user_id, existing)
  })

  const visibleIDs = new Set<number>()

  rowsByUser.forEach((userRows) => {
    const planBuckets = new Map<number, { plan: SubscriptionPlan; rows: UserSubscription[] }>()
    const unresolvedRows: UserSubscription[] = []

    userRows.forEach((subscription) => {
      const plan = resolveSubscriptionPlan(subscription)
      if (!plan) {
        unresolvedRows.push(subscription)
        return
      }

      const bucket = planBuckets.get(plan.id)
      if (bucket) {
        bucket.rows.push(subscription)
        return
      }

      planBuckets.set(plan.id, { plan, rows: [subscription] })
    })

    const keptPlanRows = [...planBuckets.values()].map(({ plan, rows }) => {
      const preferred = pickPreferredPlanRow(rows, plan)
      visibleIDs.add(preferred.id)
      return { plan, subscription: preferred }
    })

    unresolvedRows.forEach((subscription) => {
      const coveredByPlanRow = keptPlanRows.some(({ plan }) =>
        getPlanGroupIDs(plan).includes(subscription.group_id)
      )
      if (!coveredByPlanRow) {
        visibleIDs.add(subscription.id)
      }
    })
  })

  return subscriptions.value.filter((subscription) => visibleIDs.has(subscription.id))
})

const getSubscriptionLimit = (
  subscription: UserSubscription,
  period: SubscriptionQuotaPeriod
): number | null => {
  const entitlementLimit = period === 'daily'
    ? subscription.daily_limit_usd
    : period === 'weekly'
      ? subscription.weekly_limit_usd
      : subscription.monthly_limit_usd
  if (typeof entitlementLimit === 'number' && entitlementLimit > 0) return entitlementLimit

  const plan = resolveSubscriptionPlan(subscription)
  const planLimit = period === 'daily'
    ? plan?.daily_limit_usd
    : period === 'weekly'
      ? plan?.weekly_limit_usd
      : plan?.monthly_limit_usd
  if (typeof planLimit === 'number' && planLimit > 0) return planLimit
  if (subscription.entitlement_id != null || subscription.entitlement_only === true || subscription.plan_id != null) return null

  const groupLimit = period === 'daily'
    ? subscription.group?.daily_limit_usd
    : period === 'weekly'
      ? subscription.group?.weekly_limit_usd
      : subscription.group?.monthly_limit_usd

  return typeof groupLimit === 'number' && groupLimit > 0 ? groupLimit : null
}

const formatPlanValidity = (days: number | null | undefined): string => {
  return t('admin.subscriptions.planValidityDays', { days: days || 0 })
}

const getStatusFilter = (): SubscriptionStatusFilter | undefined => {
  return filters.status === 'active' || filters.status === 'expired' || filters.status === 'suspended' || filters.status === 'revoked'
    ? filters.status
    : undefined
}

const canManageSubscriptionRow = (subscription: UserSubscription): boolean => {
  return (
    subscription.id > 0 ||
    (subscription.entitlement_only === true && subscription.id < 0 && subscription.entitlement_id != null)
  )
}

const applyFilters = () => {
  pagination.page = 1
  loadSubscriptions()
}

const loadSubscriptions = async () => {
  if (abortController) {
    abortController.abort()
  }
  const requestController = new AbortController()
  abortController = requestController
  const { signal } = requestController

  loading.value = true
  try {
    const filterPlan = filters.plan_id ? planByID.value.get(parseInt(filters.plan_id)) : null
    if (filterPlan) {
      await loadSubscriptionsByPlan(filterPlan, signal)
      return
    }

    const listFilters: Parameters<typeof adminAPI.subscriptions.list>[2] = {
      status: getStatusFilter(),
      platform: filters.platform || undefined,
      user_id: filters.user_id || undefined,
      sort_by: sortState.sort_by,
      sort_order: sortState.sort_order
    }

    const response = await adminAPI.subscriptions.list(
      pagination.page,
      pagination.page_size,
      listFilters,
      {
        signal
      }
    )
    if (signal.aborted || abortController !== requestController) return
    subscriptions.value = response.items
    pagination.total = response.total
    pagination.pages = response.pages
  } catch (error: any) {
    if (signal.aborted || error?.name === 'AbortError' || error?.code === 'ERR_CANCELED') {
      return
    }
    appStore.showError(t('admin.subscriptions.failedToLoad'))
    console.error('Error loading subscriptions:', error)
  } finally {
    if (abortController === requestController) {
      loading.value = false
      abortController = null
    }
  }
}

const loadSubscriptionsByPlan = async (plan: SubscriptionPlan, signal: AbortSignal) => {
  const groupIDs = getPlanGroupIDs(plan)
  if (groupIDs.length === 0) {
    subscriptions.value = []
    pagination.total = 0
    pagination.pages = 0
    return
  }

  const baseFilters: Omit<NonNullable<Parameters<typeof adminAPI.subscriptions.list>[2]>, 'group_id'> = {
    status: getStatusFilter(),
    platform: filters.platform || undefined,
    user_id: filters.user_id || undefined,
    sort_by: sortState.sort_by,
    sort_order: sortState.sort_order
  }

  const batches = await Promise.all(
    groupIDs.map((groupID) => loadAllSubscriptionsForGroup(groupID, baseFilters, signal))
  )
  if (signal.aborted) return

  const byID = new Map<number, UserSubscription>()
  batches.flat().forEach((subscription) => {
    if (subscriptionMatchesPlan(subscription, plan)) {
      byID.set(subscription.id, subscription)
    }
  })

  const sortedItems = sortSubscriptionsClientSide([...byID.values()])
  const start = (pagination.page - 1) * pagination.page_size

  subscriptions.value = sortedItems.slice(start, start + pagination.page_size)
  pagination.total = sortedItems.length
  pagination.pages = Math.ceil(sortedItems.length / pagination.page_size)
}

const loadAllSubscriptionsForGroup = async (
  groupID: number,
  baseFilters: Omit<NonNullable<Parameters<typeof adminAPI.subscriptions.list>[2]>, 'group_id'>,
  signal: AbortSignal
): Promise<UserSubscription[]> => {
  const firstPage = await adminAPI.subscriptions.list(
    1,
    PLAN_FILTER_FETCH_PAGE_SIZE,
    { ...baseFilters, group_id: groupID },
    { signal }
  )
  if (signal.aborted || firstPage.pages <= 1) return firstPage.items

  const rest = await Promise.all(
    Array.from({ length: firstPage.pages - 1 }, (_, index) =>
      adminAPI.subscriptions.list(
        index + 2,
        PLAN_FILTER_FETCH_PAGE_SIZE,
        { ...baseFilters, group_id: groupID },
        { signal }
      )
    )
  )

  return [firstPage, ...rest].flatMap((page) => page.items)
}

const sortSubscriptionsClientSide = (items: UserSubscription[]): UserSubscription[] => {
  const sortKey = sortState.sort_by as keyof UserSubscription
  const direction = sortState.sort_order === 'asc' ? 1 : -1

  return [...items].sort((a, b) => compareValues(a[sortKey], b[sortKey]) * direction)
}

const compareValues = (left: unknown, right: unknown): number => {
  if (left === right) return 0
  if (left === null || left === undefined) return 1
  if (right === null || right === undefined) return -1

  if (typeof left === 'number' && typeof right === 'number') {
    return left - right
  }

  const leftTime = typeof left === 'string' ? Date.parse(left) : Number.NaN
  const rightTime = typeof right === 'string' ? Date.parse(right) : Number.NaN
  if (!Number.isNaN(leftTime) && !Number.isNaN(rightTime)) {
    return leftTime - rightTime
  }

  return String(left).localeCompare(String(right))
}

const loadSubscriptionPlans = async () => {
  try {
    const response = await adminAPI.payment.getPlans()
    subscriptionPlans.value = normalizeSubscriptionPlans((response.data || []) as RawSubscriptionPlan[])
  } catch (error) {
    console.error('Error loading subscription plans:', error)
  }
}

// Toolbar user filter search with debounce
const debounceSearchFilterUsers = () => {
  if (filterUserSearchTimeout) {
    clearTimeout(filterUserSearchTimeout)
  }
  filterUserSearchTimeout = setTimeout(searchFilterUsers, 300)
}

const searchFilterUsers = async () => {
  const keyword = filterUserKeyword.value.trim()

  // Clear active user filter if user modified the search keyword
  if (selectedFilterUser.value && keyword !== selectedFilterUser.value.email) {
    selectedFilterUser.value = null
    filters.user_id = null
    applyFilters()
  }

  if (!keyword) {
    filterUserResults.value = []
    return
  }

  filterUserLoading.value = true
  try {
    filterUserResults.value = await adminAPI.usage.searchUsers(keyword)
  } catch (error) {
    console.error('Failed to search users:', error)
    filterUserResults.value = []
  } finally {
    filterUserLoading.value = false
  }
}

const selectFilterUser = (user: SimpleUser) => {
  selectedFilterUser.value = user
  filterUserKeyword.value = user.email
  showFilterUserDropdown.value = false
  filters.user_id = user.id
  applyFilters()
}

const clearFilterUser = () => {
  selectedFilterUser.value = null
  filterUserKeyword.value = ''
  filterUserResults.value = []
  showFilterUserDropdown.value = false
  filters.user_id = null
  applyFilters()
}

// User search with debounce
const debounceSearchUsers = () => {
  if (userSearchTimeout) {
    clearTimeout(userSearchTimeout)
  }
  userSearchTimeout = setTimeout(searchUsers, 300)
}

const searchUsers = async () => {
  const keyword = userSearchKeyword.value.trim()

  // Clear selection if user modified the search keyword
  if (selectedUser.value && keyword !== selectedUser.value.email) {
    selectedUser.value = null
    assignForm.user_id = null
  }

  if (!keyword) {
    userSearchResults.value = []
    return
  }

  userSearchLoading.value = true
  try {
    userSearchResults.value = await adminAPI.usage.searchUsers(keyword)
  } catch (error) {
    console.error('Failed to search users:', error)
    userSearchResults.value = []
  } finally {
    userSearchLoading.value = false
  }
}

const selectUser = (user: SimpleUser) => {
  selectedUser.value = user
  userSearchKeyword.value = user.email
  showUserDropdown.value = false
  assignForm.user_id = user.id
}

const clearUserSelection = () => {
  selectedUser.value = null
  userSearchKeyword.value = ''
  userSearchResults.value = []
  assignForm.user_id = null
}

const handlePageChange = (page: number) => {
  pagination.page = page
  loadSubscriptions()
}

const handlePageSizeChange = (pageSize: number) => {
  pagination.page_size = pageSize
  pagination.page = 1
  loadSubscriptions()
}

const handleSort = (key: string, order: 'asc' | 'desc') => {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  loadSubscriptions()
}

const closeAssignModal = () => {
  showAssignModal.value = false
  assignForm.user_id = null
  assignForm.plan_id = null
  assignForm.group_id = null
  assignForm.validity_days = 30
  // Clear user search state
  selectedUser.value = null
  userSearchKeyword.value = ''
  userSearchResults.value = []
  showUserDropdown.value = false
}

const handleAssignPlanChange = (value: string | number | boolean | null) => {
  const planID = typeof value === 'number' ? value : Number(value)
  const plan = Number.isFinite(planID) ? planByID.value.get(planID) : null
  assignForm.group_id = getPlanPrimaryGroupID(plan)
  assignForm.validity_days = plan
    ? getPlanValidityDays(plan.validity_days, plan.validity_unit)
    : 30
}

const handleAssignSubscription = async () => {
  if (!assignForm.user_id) {
    appStore.showError(t('admin.subscriptions.pleaseSelectUser'))
    return
  }
  if (!assignForm.plan_id) {
    appStore.showError(t('admin.subscriptions.pleaseSelectPlan'))
    return
  }
  if (!assignForm.group_id) {
    appStore.showError(t('admin.subscriptions.selectedPlanUnavailable'))
    return
  }
  if (!assignForm.validity_days || assignForm.validity_days < 1) {
    appStore.showError(t('admin.subscriptions.validityDaysRequired'))
    return
  }
  if (assignForm.validity_days > MAX_PLAN_VALIDITY_DAYS) {
    appStore.showError(t('admin.subscriptions.validityDaysTooLong', { days: MAX_PLAN_VALIDITY_DAYS }))
    return
  }

  submitting.value = true
  try {
    await adminAPI.subscriptions.assign({
      user_id: assignForm.user_id,
      group_id: assignForm.group_id,
      plan_id: assignForm.plan_id,
      validity_days: assignForm.validity_days
    })
    appStore.showSuccess(t('admin.subscriptions.subscriptionAssigned'))
    closeAssignModal()
    loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToAssign'))
    console.error('Error assigning subscription:', error)
  } finally {
    submitting.value = false
  }
}

const handleExtend = (subscription: UserSubscription) => {
  extendingSubscription.value = subscription
  extendForm.days = 30
  showExtendModal.value = true
}

const closeExtendModal = () => {
  showExtendModal.value = false
  extendingSubscription.value = null
}

const handleExtendSubscription = async () => {
  if (!extendingSubscription.value) return

  // 前端验证：调整后的过期时间必须在未来
  if (extendingSubscription.value.expires_at) {
    const expiresAt = new Date(extendingSubscription.value.expires_at)
    const newExpiresAt = new Date(expiresAt.getTime() + extendForm.days * 24 * 60 * 60 * 1000)
    if (newExpiresAt <= new Date()) {
      appStore.showError(t('admin.subscriptions.adjustWouldExpire'))
      return
    }
  }

  submitting.value = true
  try {
    await adminAPI.subscriptions.extend(extendingSubscription.value.id, {
      days: extendForm.days
    })
    appStore.showSuccess(t('admin.subscriptions.subscriptionAdjusted'))
    closeExtendModal()
    loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToAdjust'))
    console.error('Error adjusting subscription:', error)
  } finally {
    submitting.value = false
  }
}

const handleRevoke = (subscription: UserSubscription) => {
  revokingSubscription.value = subscription
  showRevokeDialog.value = true
}

const confirmRevoke = async () => {
  if (!revokingSubscription.value) return

  try {
    await adminAPI.subscriptions.revoke(revokingSubscription.value.id)
    appStore.showSuccess(t('admin.subscriptions.subscriptionRevoked'))
    showRevokeDialog.value = false
    revokingSubscription.value = null
    loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToRevoke'))
    console.error('Error revoking subscription:', error)
  }
}

const handleRestore = (subscription: UserSubscription) => {
  restoringSubscription.value = subscription
  showRestoreDialog.value = true
}

const confirmRestore = async () => {
  if (!restoringSubscription.value) return

  try {
    await adminAPI.subscriptions.restore(restoringSubscription.value.id)
    appStore.showSuccess(t('admin.subscriptions.subscriptionRestored'))
    showRestoreDialog.value = false
    restoringSubscription.value = null
    loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToRestore'))
    console.error('Error restoring subscription:', error)
  }
}

const handleResetQuota = (subscription: UserSubscription) => {
  resettingSubscription.value = subscription
  showResetQuotaConfirm.value = true
}

const confirmResetQuota = async () => {
  if (!resettingSubscription.value) return
  if (resettingQuota.value) return
  resettingQuota.value = true
  try {
    await adminAPI.subscriptions.resetQuota(resettingSubscription.value.id, { daily: true, weekly: true, monthly: true })
    appStore.showSuccess(t('admin.subscriptions.quotaResetSuccess'))
    showResetQuotaConfirm.value = false
    resettingSubscription.value = null
    await loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToResetQuota'))
    console.error('Error resetting quota:', error)
  } finally {
    resettingQuota.value = false
  }
}

const handleCycleAdjustment = (subscription: UserSubscription) => {
  cycleAdjustmentSubscription.value = subscription
  cycleAdjustmentForm.mode = 'advance_next_cycle'
  cycleAdjustmentForm.cycle_count = inferCycleCountFromSubscription(subscription)
  cycleAdjustmentForm.custom_monthly_window_start = toDateTimeLocalValue(subscription.monthly_window_start || subscription.starts_at)
  cycleAdjustmentForm.custom_expires_at = toDateTimeLocalValue(subscription.expires_at)
  cycleAdjustmentForm.reset_monthly_usage = true
  cycleAdjustmentForm.reason = ''
  cycleAdjustmentPreview.value = null
  showCycleAdjustmentModal.value = true
}

const closeCycleAdjustmentModal = () => {
  if (cycleAdjustmentLoading.value || cycleAdjustmentSubmitting.value) return
  showCycleAdjustmentModal.value = false
  cycleAdjustmentSubscription.value = null
  cycleAdjustmentPreview.value = null
  cycleAdjustmentForm.mode = 'advance_next_cycle'
  cycleAdjustmentForm.cycle_count = 1
  cycleAdjustmentForm.custom_monthly_window_start = ''
  cycleAdjustmentForm.custom_expires_at = ''
  cycleAdjustmentForm.reset_monthly_usage = true
  cycleAdjustmentForm.reason = ''
}

const clearCycleAdjustmentPreview = () => {
  cycleAdjustmentPreview.value = null
}

const handleCycleAdjustmentModeChange = () => {
  cycleAdjustmentForm.reset_monthly_usage = cycleAdjustmentForcesUsageReset.value
  clearCycleAdjustmentPreview()
}

const previewCycleAdjustment = async () => {
  if (!cycleAdjustmentSubscription.value) return
  const request = buildCycleAdjustmentRequest()
  if (!request) return

  cycleAdjustmentLoading.value = true
  try {
    cycleAdjustmentPreview.value = await adminAPI.subscriptions.previewMonthlyCycleAdjustment(
      cycleAdjustmentSubscription.value.id,
      request
    )
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToPreviewCycleAdjustment'))
    console.error('Error previewing monthly cycle adjustment:', error)
  } finally {
    cycleAdjustmentLoading.value = false
  }
}

const applyCycleAdjustment = async () => {
  if (!cycleAdjustmentSubscription.value || !cycleAdjustmentPreview.value?.can_apply) return
  const request = buildCycleAdjustmentRequest()
  if (!request) return

  cycleAdjustmentSubmitting.value = true
  try {
    await adminAPI.subscriptions.applyMonthlyCycleAdjustment(cycleAdjustmentSubscription.value.id, request)
    appStore.showSuccess(t('admin.subscriptions.cycleAdjustmentApplied'))
    showCycleAdjustmentModal.value = false
    cycleAdjustmentSubscription.value = null
    cycleAdjustmentPreview.value = null
    await loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToApplyCycleAdjustment'))
    console.error('Error applying monthly cycle adjustment:', error)
  } finally {
    cycleAdjustmentSubmitting.value = false
  }
}

const buildCycleAdjustmentRequest = (): MonthlyCycleAdjustmentRequest | null => {
  const request: MonthlyCycleAdjustmentRequest = {
    mode: cycleAdjustmentForm.mode,
    reset_monthly_usage: cycleAdjustmentForcesUsageReset.value || cycleAdjustmentForm.reset_monthly_usage
  }

  if (cycleAdjustmentNeedsCycleCount.value) {
    const cycleCount = Number(cycleAdjustmentForm.cycle_count)
    if (!Number.isFinite(cycleCount) || cycleCount < 1) {
      appStore.showError(t('admin.subscriptions.cycleCountRequired'))
      return null
    }
    if (cycleCount > cycleAdjustmentMaxCycleCount) {
      appStore.showError(t('admin.subscriptions.cycleCountOutOfRange', { max: cycleAdjustmentMaxCycleCount }))
      return null
    }
    request.cycle_count = Math.floor(cycleCount)
  }

  if (cycleAdjustmentNeedsCustomTimes.value) {
    const windowStart = dateTimeLocalToISOString(cycleAdjustmentForm.custom_monthly_window_start)
    const expiresAt = dateTimeLocalToISOString(cycleAdjustmentForm.custom_expires_at)
    if (!windowStart || !expiresAt) {
      appStore.showError(t('admin.subscriptions.customTimesRequired'))
      return null
    }
    request.custom_monthly_window_start = windowStart
    request.custom_expires_at = expiresAt
  }

  const reason = cycleAdjustmentForm.reason.trim()
  if (reason.length > cycleAdjustmentReasonMaxLength) {
    appStore.showError(t('admin.subscriptions.cycleAdjustmentReasonTooLong', { max: cycleAdjustmentReasonMaxLength }))
    return null
  }
  if (reason) {
    request.reason = reason
  } else if (cycleAdjustmentReasonRequired.value) {
    appStore.showError(t('admin.subscriptions.cycleAdjustmentReasonRequired'))
    return null
  }

  return request
}

// Helper functions
const formatExpirationRemaining = (expiresAt: string): string | null => {
  return formatRemainingDurationCompact(expiresAt)
}

const formatRemainingExpiry = (expiresAt: string): string | null => {
  const duration = getRemainingExpiryDuration(expiresAt)
  if (!duration) return null
  if (duration.unit === 'days') {
    return t('admin.subscriptions.daysRemaining', { days: duration.days })
  }
  if (duration.hours) {
    return t('admin.subscriptions.hoursMinutesRemaining', {
      hours: duration.hours,
      minutes: duration.minutes
    })
  }
  return t('admin.subscriptions.minutesRemaining', { minutes: duration.minutes })
}

const isExpiringSoon = (expiresAt: string): boolean => {
  const remainingHours = getRemainingHours(expiresAt)
  return remainingHours !== null && remainingHours <= 7 * 24
}

const getProgressWidth = (used: number | null | undefined, limit: number | null): string => {
  if (!limit || limit === 0) return '0%'
  const usedValue = used ?? 0
  const percentage = Math.min((usedValue / limit) * 100, 100)
  return `${percentage}%`
}

const getProgressClass = (used: number | null | undefined, limit: number | null): string => {
  if (!limit || limit === 0) return 'bg-gray-400'
  const usedValue = used ?? 0
  const percentage = (usedValue / limit) * 100
  if (percentage >= 90) return 'bg-red-500'
  if (percentage >= 70) return 'bg-orange-500'
  return 'bg-green-500'
}

const formatResetDuration = (parts: RemainingDurationParts): string => {
  if (parts.days > 0) {
    return t('admin.subscriptions.resetInDaysHours', { days: parts.days, hours: parts.hours })
  }

  if (parts.hours > 0) {
    return t('admin.subscriptions.resetInHoursMinutes', { hours: parts.hours, minutes: parts.minutes })
  }

  return t('admin.subscriptions.resetInMinutes', { minutes: parts.minutes })
}

const formatQuotaEndDuration = (parts: RemainingDurationParts): string => {
  if (parts.days > 0) {
    return t('admin.subscriptions.quotaEndsInDaysHours', { days: parts.days, hours: parts.hours })
  }

  if (parts.hours > 0) {
    return t('admin.subscriptions.quotaEndsInHoursMinutes', { hours: parts.hours, minutes: parts.minutes })
  }

  return t('admin.subscriptions.quotaEndsInMinutes', { minutes: parts.minutes })
}

const formatDailyUsageWindow = (subscription: UserSubscription): string => {
  if (isOneTimeDailyQuota(subscription) && subscription.expires_at) {
    const parts = getRemainingDurationParts(subscription.expires_at)
    return parts ? formatQuotaEndDuration(parts) : t('admin.subscriptions.windowNotActive')
  }

  if (subscription.daily_resets_at) {
    const parts = getRemainingDurationParts(subscription.daily_resets_at)
    return parts ? formatResetDuration(parts) : t('admin.subscriptions.windowNotActive')
  }

  return formatResetTime(subscription.daily_window_start, subscription.starts_at, 'daily')
}

// Format reset time based on window start and period type
const formatResetTime = (
  windowStart: string | null,
  startsAt: string,
  period: 'daily' | 'weekly' | 'monthly'
): string => {
  const cycleHours = period === 'daily' ? 24 : period === 'weekly' ? 24 * 7 : 24 * 30
  const resetTime = getCycleResetAt(windowStart, startsAt, cycleHours)
  if (!resetTime) return t('admin.subscriptions.windowNotActive')

  const parts = getRemainingDurationParts(resetTime, new Date())

  return parts ? formatResetDuration(parts) : t('admin.subscriptions.windowNotActive')
}

const toDateTimeLocalValue = (value: string | null | undefined): string => {
  if (!value) return ''
  const timestamp = Date.parse(value)
  if (!Number.isFinite(timestamp)) return ''
  return formatDateTimeLocalInput(Math.floor(timestamp / 1000))
}

const dateTimeLocalToISOString = (value: string): string | undefined => {
  const timestampSeconds = parseDateTimeLocalInput(value)
  if (!timestampSeconds) return undefined
  return new Date(timestampSeconds * 1000).toISOString()
}

const inferCycleCountFromSubscription = (subscription: UserSubscription): number => {
  const start = Date.parse(subscription.monthly_window_start || subscription.starts_at || '')
  const expires = Date.parse(subscription.expires_at || '')
  if (!Number.isFinite(start) || !Number.isFinite(expires) || expires <= start) return 1
  return Math.max(1, Math.ceil((expires - start) / MONTHLY_CYCLE_MS))
}

const formatAdjustmentDateTime = (value: string | null | undefined): string => {
  return value ? formatDateTime(value) : '-'
}

const formatAdjustmentUsage = (usage: number, limit?: number | null): string => {
  const used = `$${(usage || 0).toFixed(2)}`
  return typeof limit === 'number' && limit > 0 ? `${used} / $${limit.toFixed(2)}` : used
}

const formatAdjustmentDuration = (seconds: number | null | undefined): string => {
  const totalSeconds = Math.max(0, Math.floor(seconds || 0))
  if (totalSeconds === 0) return t('admin.subscriptions.durationZero')

  const days = Math.floor(totalSeconds / 86400)
  const hours = Math.floor((totalSeconds % 86400) / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  if (days > 0) {
    return t('admin.subscriptions.durationDaysHours', { days, hours })
  }
  if (hours > 0) {
    return t('admin.subscriptions.durationHoursMinutes', { hours, minutes })
  }
  return t('admin.subscriptions.durationMinutes', { minutes: Math.max(1, minutes) })
}

const formatCycleAdjustmentUnavailable = (reason?: string): string => {
  if (!reason) return t('admin.subscriptions.cycleAdjustmentUnavailable.generic')
  return t(`admin.subscriptions.cycleAdjustmentUnavailable.${reason}`)
}

const formatCycleAdjustmentWarning = (warning: string): string => {
  return t(`admin.subscriptions.cycleAdjustmentWarnings.${warning}`)
}

// Handle click outside to close dropdowns
const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (!target.closest('[data-assign-user-search]')) showUserDropdown.value = false
  if (!target.closest('[data-filter-user-search]')) showFilterUserDropdown.value = false
  if (columnDropdownRef.value && !columnDropdownRef.value.contains(target)) {
    showColumnDropdown.value = false
  }
}

onMounted(() => {
  loadUserColumnMode()
  loadSavedColumns()
  loadSubscriptionPlans().finally(loadSubscriptions)
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  if (filterUserSearchTimeout) {
    clearTimeout(filterUserSearchTimeout)
  }
  if (userSearchTimeout) {
    clearTimeout(userSearchTimeout)
  }
})
</script>

<style scoped>
.usage-row {
  @apply space-y-1;
}

.usage-label {
  @apply w-10 flex-shrink-0 text-xs font-medium text-gray-500 dark:text-gray-400;
}

.usage-amount {
  @apply whitespace-nowrap text-xs tabular-nums text-gray-600 dark:text-gray-300;
}

.reset-info {
  @apply flex items-center gap-1 pl-12 text-[10px] text-blue-600 dark:text-blue-400;
}
</style>
