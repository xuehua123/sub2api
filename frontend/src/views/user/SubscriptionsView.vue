<template>
  <AppLayout>
    <div class="mx-auto w-full max-w-[1760px] space-y-5">
      <!-- Loading State -->
      <div v-if="loading" class="flex justify-center py-12">
        <div
          class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
        ></div>
      </div>

      <!-- Empty State -->
      <div v-else-if="!hasAnySubscriptionDisplay" class="card p-12 text-center">
        <div
          class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700"
        >
          <Icon name="creditCard" size="xl" class="text-gray-400" />
        </div>
        <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('userSubscriptions.noActiveSubscriptions') }}
        </h3>
        <p class="text-gray-500 dark:text-dark-400">
          {{ t('userSubscriptions.noActiveSubscriptionsDesc') }}
        </p>
      </div>

      <template v-else>
        <section v-if="displayEntitlements.length > 0" class="space-y-3" data-testid="entitlement-section">
          <div class="flex flex-wrap items-end justify-between gap-3">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('userSubscriptions.entitlements.title') }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
                {{ t('userSubscriptions.entitlements.sharedQuota') }}
              </p>
            </div>
          </div>

          <div class="grid gap-4 lg:grid-cols-2 xl:grid-cols-3">
            <article
              v-for="entitlement in displayEntitlements"
              :key="entitlement.id"
              class="overflow-hidden rounded-xl border border-indigo-100 bg-white shadow-sm dark:border-indigo-900/50 dark:bg-dark-800"
              data-testid="entitlement-card"
            >
              <div class="border-b border-gray-100 p-3.5 dark:border-dark-700">
                <div class="flex flex-wrap items-start justify-between gap-3">
                  <div class="min-w-0">
                    <div class="flex flex-wrap items-center gap-2">
                      <h3 class="text-base font-semibold text-gray-900 dark:text-white">
                        {{ entitlement.plan_name || entitlement.name || `Entitlement #${entitlement.id}` }}
                      </h3>
                      <span
                        :class="[
                          'rounded-full px-2 py-0.5 text-xs font-medium',
                          entitlementStatusClass(entitlement)
                        ]"
                      >
                        {{ entitlementStatusLabel(entitlement) }}
                      </span>
                    </div>
                    <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                      {{ t('userSubscriptions.entitlements.validity', {
                        start: formatDateTime(entitlement.starts_at),
                        end: formatDateTime(entitlement.expires_at)
                      }) }}
                    </p>
                    <p
                      v-if="entitlement.legacy_subscription_id"
                      class="mt-1 text-xs text-gray-500 dark:text-dark-400"
                    >
                      {{ t('userSubscriptions.entitlements.legacyCompatible', {
                        id: entitlement.legacy_subscription_id
                      }) }}
                    </p>
                  </div>
                  <div class="flex flex-col items-end gap-1.5 text-right text-xs text-gray-500 dark:text-dark-400">
                    <span>{{ t('userSubscriptions.entitlements.id', { id: entitlement.id }) }}</span>
                    <button
                      type="button"
                      class="rounded-md px-2 py-1 font-medium text-red-600 transition-colors hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
                      @click="confirmDeleteEntitlement(entitlement)"
                    >
                      {{ t('common.delete') }}
                    </button>
                  </div>
                </div>

                <div class="mt-3">
                  <div class="mb-2 text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-dark-400">
                    {{ t('userSubscriptions.entitlements.authorizedGroups') }}
                  </div>
                  <div class="space-y-1.5">
                    <div
                      v-for="group in visibleEntitlementGroups(entitlement)"
                      :key="group.id"
                      :class="[
                        'grid grid-cols-[minmax(0,1fr)_auto] items-center gap-2 rounded-lg border px-2.5 py-1.5 text-xs font-medium',
                        platformBadgeClass(group.platform || '')
                      ]"
                    >
                      <div class="min-w-0">
                        <div class="truncate font-semibold">{{ group.name || `Group #${group.id}` }}</div>
                        <div class="mt-0.5 truncate text-[10px] opacity-75">{{ platformLabel(group.platform || '') }}</div>
                      </div>
                      <div class="flex shrink-0 items-center gap-1.5 text-right">
                        <span class="rounded bg-black/10 px-1.5 py-0.5 text-[10px] font-bold dark:bg-white/10">
                          x{{ formatRateMultiplier(group.rate_multiplier) }}
                        </span>
                        <span class="w-[74px] truncate text-[10px] font-semibold">
                          {{ entitlementGroupEstimatedCost(entitlement, group) }}
                        </span>
                      </div>
                    </div>
                    <button
                      v-if="hiddenEntitlementGroupCount(entitlement) > 0"
                      type="button"
                      class="flex w-full items-center justify-between rounded-lg border border-primary-200 bg-primary-50 px-2.5 py-2 text-left text-xs font-bold text-primary-700 transition hover:border-primary-300 hover:bg-primary-100 dark:border-primary-500/20 dark:bg-primary-500/10 dark:text-primary-300 dark:hover:border-primary-400 dark:hover:bg-primary-500/15"
                      @click="openEntitlementGroupsModal(entitlement)"
                    >
                      <span>+{{ hiddenEntitlementGroupCount(entitlement) }} 个分组</span>
                      <span class="text-[10px] font-semibold opacity-80">查看全部</span>
                    </button>
                  </div>
                </div>
              </div>

              <div class="space-y-2.5 p-3.5">
                <div
                  :class="[
                    'grid gap-2',
                    visibleEntitlementUsageWindows(entitlement).length > 1 ? 'sm:grid-cols-2 2xl:grid-cols-3' : 'grid-cols-1'
                  ]"
                >
                  <div
                    v-for="window in visibleEntitlementUsageWindows(entitlement)"
                    :key="window.key"
                    class="rounded-lg border border-gray-100 p-2.5 dark:border-dark-700"
                    :data-testid="`entitlement-${window.key}-quota`"
                  >
                    <div class="mb-1.5 flex items-center justify-between gap-2">
                      <span class="text-xs font-medium text-gray-700 dark:text-gray-300">
                        {{ window.label }}
                      </span>
                      <span class="text-xs text-gray-500 dark:text-dark-400">
                        {{ formatEntitlementUsage(window.used, window.limit) }}
                      </span>
                    </div>
                    <div class="relative h-1.5 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                      <div
                        class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                        :class="getProgressBarClass(window.used, window.limit)"
                        :style="{ width: getProgressWidth(window.used, window.limit) }"
                      ></div>
                    </div>
                    <p class="mt-1.5 truncate text-[10px] text-gray-500 dark:text-dark-400">
                      {{ t('userSubscriptions.entitlements.nextReset') }}:
                      {{ formatEntitlementReset(window, entitlement) }}
                    </p>
                  </div>
                </div>

                <div class="flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2 text-xs dark:bg-dark-900">
                  <span class="text-gray-500 dark:text-dark-400">
                    {{ t('userSubscriptions.entitlements.overagePolicy') }}
                  </span>
                  <span class="font-medium text-gray-800 dark:text-gray-200">
                    {{ entitlementOveragePolicyLabel(entitlement.overage_policy) }}
                  </span>
                </div>

                <div class="rounded-lg border border-gray-100 bg-gray-50/70 p-2.5 dark:border-dark-700 dark:bg-dark-900/70">
                  <button
                    type="button"
                    data-testid="entitlement-advance-monthly-cycle"
                    class="flex w-full items-center justify-center rounded-md border border-primary-200 bg-primary-50 px-3 py-2 text-xs font-semibold text-primary-700 transition-colors hover:bg-primary-100 disabled:cursor-not-allowed disabled:border-gray-200 disabled:bg-gray-100 disabled:text-gray-400 dark:border-primary-900/60 dark:bg-primary-900/20 dark:text-primary-200 dark:hover:bg-primary-900/30 dark:disabled:border-dark-700 dark:disabled:bg-dark-800 dark:disabled:text-dark-500"
                    :disabled="advancingEntitlementId === entitlement.id || !canAdvanceEntitlementMonthlyCycle(entitlement)"
                    :title="advanceEntitlementMonthlyCycleHint(entitlement)"
                    @click="advanceEntitlementMonthlyCycle(entitlement)"
                  >
                    {{
                      advancingEntitlementId === entitlement.id
                        ? t('common.processing')
                        : t('userSubscriptions.advanceEntitlementMonthlyCycle')
                    }}
                  </button>
                  <p
                    class="mt-1.5 text-[10px] leading-snug text-gray-500 dark:text-dark-400"
                    data-testid="entitlement-advance-monthly-hint"
                  >
                    {{ advanceEntitlementMonthlyCycleHint(entitlement) }}
                  </p>
                </div>
              </div>
            </article>
          </div>
        </section>

        <div
          v-if="switchPreferences.length > 1 && displayEntitlements.length === 0"
          class="rounded-2xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800"
        >
          <div class="mb-3 flex items-center justify-between gap-3">
            <div>
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('userSubscriptions.switchPriority') }}
              </h3>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                {{ t('userSubscriptions.switchPriorityHint') }}
              </p>
            </div>
            <button
              class="btn btn-secondary text-sm"
              :disabled="savingPreferences"
              @click="saveSwitchPreferences"
            >
              {{ savingPreferences ? t('common.saving') : t('common.save') }}
            </button>
          </div>
          <div class="space-y-2">
            <div
              v-for="(pref, index) in switchPreferences"
              :key="pref.group_id"
              class="flex cursor-move items-center gap-3 rounded-lg border border-gray-100 bg-gray-50 px-3 py-2 dark:border-dark-700 dark:bg-dark-900"
              draggable="true"
              @dragstart="onPreferenceDragStart(pref.group_id)"
              @dragover.prevent
              @drop="onPreferenceDrop(pref.group_id)"
            >
              <span class="w-6 text-xs font-semibold text-gray-400">#{{ index + 1 }}</span>
              <div class="min-w-0 flex-1">
                <div class="truncate text-sm font-medium text-gray-900 dark:text-white">
                  {{ subscriptionByGroupID(pref.group_id)
                    ? subscriptionDisplayName(subscriptionByGroupID(pref.group_id)!, subscriptionPlans)
                    : `Group #${pref.group_id}` }}
                </div>
                <div class="flex flex-wrap items-center gap-1 text-xs text-gray-500 dark:text-dark-400">
                  <span>{{ platformLabel(subscriptionByGroupID(pref.group_id)?.group?.platform || '') }}</span>
                  <span
                    v-if="subscriptionSwitchModeLabel(subscriptionByGroupID(pref.group_id))"
                    class="rounded border border-gray-200 px-1.5 py-0.5 text-[11px] text-gray-600 dark:border-dark-600 dark:text-gray-300"
                  >
                    {{ subscriptionSwitchModeLabel(subscriptionByGroupID(pref.group_id)) }}
                  </span>
                </div>
              </div>
              <button
                type="button"
                class="rounded-md px-2 py-1 text-xs text-gray-500 hover:bg-gray-200 dark:text-gray-300 dark:hover:bg-dark-700"
                :disabled="index === 0"
                @click="movePreference(index, -1)"
              >
                {{ t('userSubscriptions.moveUp') }}
              </button>
              <button
                type="button"
                class="rounded-md px-2 py-1 text-xs text-gray-500 hover:bg-gray-200 dark:text-gray-300 dark:hover:bg-dark-700"
                :disabled="index === switchPreferences.length - 1"
                @click="movePreference(index, 1)"
              >
                {{ t('userSubscriptions.moveDown') }}
              </button>
              <button
                type="button"
                :aria-label="t('userSubscriptions.switchCandidateToggleHint')"
                :title="t('userSubscriptions.switchCandidateToggleHint')"
                @click="pref.enabled = !pref.enabled"
                :class="[
                  'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                  pref.enabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
                ]"
              >
                <span
                  :class="[
                    'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                    pref.enabled ? 'translate-x-4' : 'translate-x-0'
                  ]"
                />
              </button>
            </div>
          </div>
        </div>

        <!-- Subscriptions Grid -->
        <div class="grid gap-4 lg:grid-cols-2 xl:grid-cols-3">
          <div
            v-for="subscription in displayLegacySubscriptions"
            :key="subscription.id"
            class="overflow-hidden rounded-xl border bg-white shadow-sm dark:bg-dark-800"
            :class="platformBorderClass(subscription.group?.platform || '')"
          >
          <!-- Header -->
          <div
            class="flex items-center justify-between border-b border-gray-100 p-4 dark:border-dark-700"
          >
            <div class="flex items-center gap-3">
              <div :class="['h-1.5 w-1.5 shrink-0 rounded-full', platformAccentDotClass(subscription.group?.platform || '')]" />
              <div>
                <div class="flex items-center gap-2">
                  <h3 class="font-semibold text-gray-900 dark:text-white">
                    {{ subscriptionDisplayName(subscription, subscriptionPlans) }}
                  </h3>
                  <span :class="['rounded-md border px-2 py-0.5 text-[11px] font-medium', platformBadgeClass(subscription.group?.platform || '')]">
                    {{ platformLabel(subscription.group?.platform || '') }}
                  </span>
                  <span
                    v-if="subscriptionSwitchModeLabel(subscription)"
                    class="rounded-md border border-gray-200 px-2 py-0.5 text-[11px] font-medium text-gray-600 dark:border-dark-600 dark:text-gray-300"
                  >
                    {{ subscriptionSwitchModeLabel(subscription) }}
                  </span>
                </div>
                <p v-if="subscription.group?.description" class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
                  {{ subscription.group.description }}
                </p>
                <div class="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-gray-400 dark:text-gray-500">
                  <span>{{ t('payment.planCard.rate') }}: ×{{ subscription.group?.rate_multiplier ?? 1 }}</span>
                  <span v-if="subscriptionHasPeakRate(subscription)" class="text-amber-700 dark:text-amber-300">
                    {{ t('payment.planCard.peakRate') }}: {{ subscriptionPeakRateLabel(subscription) }}
                  </span>
                </div>
              </div>
            </div>
            <div class="flex items-center gap-2">
              <span
                :class="[
                  'rounded-full px-2 py-0.5 text-xs font-medium',
                  subscription.status === 'active'
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
                    : subscription.status === 'expired'
                      ? 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-400'
                      : 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
                ]"
              >
                {{ t(`userSubscriptions.status.${subscription.status}`) }}
              </span>
              <button
                v-if="subscription.status === 'active'"
                :class="['rounded-lg px-3 py-1.5 text-xs font-semibold text-white transition-colors', platformButtonClass(subscription.group?.platform || '')]"
                @click="router.push({ path: '/purchase', query: { tab: 'subscription', group: String(subscription.group_id) } })"
              >
                {{ t('payment.renewNow') }}
              </button>
              <button
                type="button"
                class="rounded-lg px-3 py-1.5 text-xs font-semibold text-red-600 transition-colors hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
                @click="confirmDeleteSubscription(subscription)"
              >
                {{ t('common.delete') }}
              </button>
            </div>
          </div>

          <!-- Usage Progress -->
          <div class="space-y-4 p-4">
            <!-- Expiration Info -->
            <div v-if="subscription.expires_at" class="flex items-center justify-between text-sm">
              <span class="text-gray-500 dark:text-dark-400">{{
                t('userSubscriptions.expires')
              }}</span>
              <span :class="getExpirationClass(subscription.expires_at)">
                {{ formatExpirationDate(subscription.expires_at) }}
              </span>
            </div>
            <div v-else class="flex items-center justify-between text-sm">
              <span class="text-gray-500 dark:text-dark-400">{{
                t('userSubscriptions.expires')
              }}</span>
              <span class="text-gray-700 dark:text-gray-300">{{
                t('userSubscriptions.noExpiration')
              }}</span>
            </div>

            <!-- Daily Usage -->
            <div v-if="subscriptionDailyLimit(subscription)" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('userSubscriptions.daily') }}
                </span>
                <span class="text-sm text-gray-500 dark:text-dark-400">
                  ${{ (subscription.daily_usage_usd || 0).toFixed(2) }} / ${{
                    subscriptionDailyLimit(subscription)?.toFixed(2)
                  }}
                </span>
              </div>
              <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="
                    getProgressBarClass(
                      subscription.daily_usage_usd,
                      subscriptionDailyLimit(subscription)
                    )
                  "
                  :style="{
                    width: getProgressWidth(
                      subscription.daily_usage_usd,
                      subscriptionDailyLimit(subscription)
                    )
                  }"
                ></div>
              </div>
              <p class="text-xs text-gray-500 dark:text-dark-400">
                {{ formatDailyUsageWindow(subscription) }}
              </p>
            </div>

            <!-- Weekly Usage -->
            <div v-if="subscriptionWeeklyLimit(subscription)" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('userSubscriptions.weekly') }}
                </span>
                <span class="text-sm text-gray-500 dark:text-dark-400">
                  ${{ (subscription.weekly_usage_usd || 0).toFixed(2) }} / ${{
                    subscriptionWeeklyLimit(subscription)?.toFixed(2)
                  }}
                </span>
              </div>
              <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="
                    getProgressBarClass(
                      subscription.weekly_usage_usd,
                      subscriptionWeeklyLimit(subscription)
                    )
                  "
                  :style="{
                    width: getProgressWidth(
                      subscription.weekly_usage_usd,
                      subscriptionWeeklyLimit(subscription)
                    )
                  }"
                ></div>
              </div>
              <p class="text-xs text-gray-500 dark:text-dark-400">
                {{
                  t('userSubscriptions.resetIn', {
                    time: formatResetTime(subscription.weekly_window_start, 168, subscription.starts_at)
                  })
                }}
              </p>
            </div>

            <!-- Monthly Usage -->
            <div v-if="subscriptionMonthlyLimit(subscription)" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('userSubscriptions.monthly') }}
                </span>
                <span class="text-sm text-gray-500 dark:text-dark-400">
                  ${{ (subscription.monthly_usage_usd || 0).toFixed(2) }} / ${{
                    subscriptionMonthlyLimit(subscription)?.toFixed(2)
                  }}
                </span>
              </div>
              <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="
                    getProgressBarClass(
                      subscription.monthly_usage_usd,
                      subscriptionMonthlyLimit(subscription)
                    )
                  "
                  :style="{
                    width: getProgressWidth(
                      subscription.monthly_usage_usd,
                      subscriptionMonthlyLimit(subscription)
                    )
                  }"
                ></div>
              </div>
              <p class="text-xs text-gray-500 dark:text-dark-400">
                {{
                  t('userSubscriptions.resetIn', {
                    time: formatResetTime(subscription.monthly_window_start, 720, subscription.starts_at)
                  })
                }}
              </p>
              <button
                type="button"
                class="btn btn-secondary mt-1 text-xs disabled:cursor-not-allowed disabled:opacity-50"
                :disabled="advancingSubscriptionId === subscription.id || !canAdvanceMonthlyCycle(subscription)"
                :title="advanceMonthlyCycleHint(subscription)"
                @click="advanceMonthlyCycle(subscription)"
              >
                {{
                  advancingSubscriptionId === subscription.id
                    ? t('common.processing')
                    : t('userSubscriptions.advanceMonthlyCycle')
                }}
              </button>
              <p
                v-if="!canAdvanceMonthlyCycle(subscription)"
                class="text-xs text-gray-500 dark:text-dark-400"
              >
                {{ advanceMonthlyCycleHint(subscription) }}
              </p>
            </div>

            <!-- No limits configured - Unlimited badge -->
            <div
              v-if="
                !subscriptionDailyLimit(subscription) &&
                !subscriptionWeeklyLimit(subscription) &&
                !subscriptionMonthlyLimit(subscription)
              "
              class="flex items-center justify-center rounded-xl bg-gradient-to-r from-emerald-50 to-teal-50 py-6 dark:from-emerald-900/20 dark:to-teal-900/20"
            >
              <div class="flex items-center gap-3">
                <span class="text-4xl text-emerald-600 dark:text-emerald-400">∞</span>
                <div>
                  <p class="text-sm font-medium text-emerald-700 dark:text-emerald-300">
                    {{ t('userSubscriptions.unlimited') }}
                  </p>
                  <p class="text-xs text-emerald-600/70 dark:text-emerald-400/70">
                    {{ t('userSubscriptions.unlimitedDesc') }}
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
        </div>
      </template>
    </div>
    <ConfirmDialog
      :show="deleteTarget !== null"
      :title="t('userSubscriptions.deleteTitle')"
      :message="deleteTarget ? t('userSubscriptions.deleteConfirm', { name: deleteTarget.name }) : ''"
      :confirm-text="deletingSubscription ? t('common.processing') : t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="deleteSelectedSubscription"
      @cancel="cancelDeleteSubscription"
    />
    <Teleport to="body">
      <Transition name="modal">
        <div
          v-if="groupsModalEntitlement"
          class="fixed inset-0 z-[65] flex items-center justify-center bg-slate-950/70 p-4 backdrop-blur-sm"
          @click.self="closeEntitlementGroupsModal"
        >
          <div class="relative flex max-h-[82vh] w-full max-w-2xl flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-900">
            <div class="flex items-start justify-between gap-4 border-b border-gray-100 p-5 dark:border-dark-700">
              <div class="min-w-0">
                <p class="text-xs font-bold uppercase tracking-wide text-primary-600 dark:text-primary-300">授权分组</p>
                <h3 class="mt-1 truncate text-lg font-semibold text-gray-900 dark:text-white">
                  {{ entitlementDisplayName(groupsModalEntitlement) }}
                </h3>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                  共 {{ groupsModalEntitlement.groups.length }} 个可用分组，按分组倍率共享套餐额度
                </p>
              </div>
              <button
                type="button"
                class="rounded-lg p-1.5 text-gray-500 transition hover:bg-gray-100 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white"
                @click="closeEntitlementGroupsModal"
              >
                <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            <div class="flex-1 space-y-2 overflow-y-auto p-5">
              <div
                v-for="group in entitlementGroupsForDisplay(groupsModalEntitlement)"
                :key="group.id"
                :class="[
                  'grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 rounded-xl border px-3 py-2.5 text-sm',
                  platformBadgeClass(group.platform || '')
                ]"
              >
                <div class="min-w-0">
                  <div class="truncate font-semibold">{{ group.name || `Group #${group.id}` }}</div>
                  <div class="mt-0.5 truncate text-[11px] opacity-75">{{ platformLabel(group.platform || '') }}</div>
                </div>
                <div class="flex shrink-0 items-center gap-2 text-right">
                  <span class="rounded bg-black/10 px-2 py-1 text-xs font-bold dark:bg-white/10">
                    x{{ formatRateMultiplier(group.rate_multiplier) }}
                  </span>
                  <span class="w-[86px] truncate text-xs font-semibold">
                    {{ entitlementGroupEstimatedCost(groupsModalEntitlement, group) }}
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import subscriptionsAPI from '@/api/subscriptions'
import { paymentAPI } from '@/api/payment'
import type { SubscriptionGroupPreference, UserEntitlement, UserSubscription } from '@/types'
import type { SubscriptionPlan } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatCurrency, formatDateTime, formatDateTimeToMinute } from '@/utils/format'
import { hasPeakRate, formatPeakRateWindow, serverTimezoneLabel } from '@/utils/peak-rate'
import { platformBorderClass, platformBadgeClass, platformButtonClass, platformLabel } from '@/utils/platformColors'
import {
  getExpirationDateRelation,
  getRemainingDurationParts,
  isOneTimeDailyQuota,
  type RemainingDurationParts
} from '@/utils/subscriptionQuota'
import { getCycleResetAt } from '@/utils/subscriptionTime'
import { normalizeSubscriptionPlans, subscriptionDisplayName } from '@/utils/subscriptionPlanDisplay'
import { sortGroupsForDisplay } from '@/utils/groupDisplayOrder'

function platformAccentDotClass(p: string): string {
  switch (p) {
    case 'anthropic': return 'bg-orange-500'
    case 'openai': return 'bg-emerald-500'
    case 'antigravity': return 'bg-purple-500'
    case 'gemini': return 'bg-blue-500'
    default: return 'bg-gray-400'
  }
}

const { t, locale } = useI18n()
const router = useRouter()
const appStore = useAppStore()

const subscriptions = ref<UserSubscription[]>([])
const entitlements = ref<UserEntitlement[]>([])
const subscriptionPlans = ref<SubscriptionPlan[]>([])
const loading = ref(true)
const switchPreferences = ref<SubscriptionGroupPreference[]>([])
const savingPreferences = ref(false)
const advancingSubscriptionId = ref<number | null>(null)
const advancingEntitlementId = ref<number | null>(null)
const draggedPreferenceGroupID = ref<number | null>(null)
const deletingSubscription = ref(false)
const monthlyCycleAdvanceThreshold = 0.9
const monthlyCycleDurationMs = 30 * 24 * 60 * 60 * 1000
const monthlyCycleAdvanceValidityGraceMs = 60 * 1000
const entitlementGroupPreviewLimit = 4

type DeleteSubscriptionTarget = {
  type: 'entitlement' | 'subscription'
  id: number
  name: string
}

const deleteTarget = ref<DeleteSubscriptionTarget | null>(null)
const groupsModalEntitlement = ref<UserEntitlement | null>(null)

const displayEntitlements = computed(() => {
  return [...entitlements.value].sort((a, b) => {
    const statusDiff = entitlementStatusRank(a) - entitlementStatusRank(b)
    if (statusDiff !== 0) return statusDiff
    return new Date(a.expires_at).getTime() - new Date(b.expires_at).getTime()
  })
})

const displayLegacySubscriptions = computed(() => {
  const visibleEntitlementIDs = new Set(entitlements.value.map((entitlement) => entitlement.id))
  const legacySubscriptionIDsCoveredByEntitlements = new Set(
    entitlements.value
      .map((entitlement) => entitlement.legacy_subscription_id)
      .filter((id): id is number => typeof id === 'number' && id > 0)
  )
  return subscriptions.value.filter((subscription) => (
    !(subscription.entitlement_id && visibleEntitlementIDs.has(subscription.entitlement_id)) &&
    !legacySubscriptionIDsCoveredByEntitlements.has(subscription.id)
  ))
})

const hasAnySubscriptionDisplay = computed(() => (
  displayLegacySubscriptions.value.length > 0 || displayEntitlements.value.length > 0
))

function subscriptionHasPeakRate(subscription: UserSubscription): boolean {
  return hasPeakRate(subscription.group)
}

function subscriptionPeakRateLabel(subscription: UserSubscription): string {
  return formatPeakRateWindow(subscription.group, serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset))
}

async function loadSubscriptions() {
  try {
    loading.value = true
    const entitlementRecordsPromise = subscriptionsAPI.getEntitlements().catch((error) => {
      console.warn('Failed to load entitlements:', error)
      return [] as UserEntitlement[]
    })
    const subs = await subscriptionsAPI.getMySubscriptions()
    subscriptions.value = subs
    void loadSubscriptionPlansForSubscriptions(subs)

    const entitlementRecords = await entitlementRecordsPromise
    entitlements.value = entitlementRecords

    if (entitlementRecords.length > 0) {
      switchPreferences.value = []
    } else {
      const prefs = await subscriptionsAPI.getGroupPreferences()
      switchPreferences.value = buildSwitchPreferences(subs, prefs)
    }
  } catch (error) {
    console.error('Failed to load subscriptions:', error)
    appStore.showError(t('userSubscriptions.failedToLoad'))
  } finally {
    loading.value = false
  }
}

async function loadSubscriptionPlansForSubscriptions(records: UserSubscription[]) {
  if (!records.some((subscription) => subscription.plan_id)) {
    subscriptionPlans.value = []
    return
  }

  try {
    const response = await paymentAPI.getPlans()
    subscriptionPlans.value = normalizeSubscriptionPlans(response.data || [])
  } catch (error) {
    console.warn('Failed to load subscription plans:', error)
  }
}

function subscriptionByGroupID(groupID: number): UserSubscription | undefined {
  return subscriptions.value.find((sub) => sub.group_id === groupID)
}

function subscriptionSwitchModeLabel(subscription: UserSubscription | undefined): string {
  const group = subscription?.group
  if (!group) return ''
  if (group.platform === 'openai') {
    return group.allow_messages_dispatch
      ? t('userSubscriptions.switchMode.openaiMessages')
      : t('userSubscriptions.switchMode.openaiCodex')
  }
  if (group.platform === 'antigravity') {
    return t('userSubscriptions.switchMode.antigravity')
  }
  return ''
}

function buildSwitchPreferences(
  subs: UserSubscription[],
  prefs: SubscriptionGroupPreference[]
): SubscriptionGroupPreference[] {
  const prefMap = new Map(prefs.map((pref) => [pref.group_id, pref]))
  return subs
    .filter((sub) => sub.status === 'active' && sub.group_id)
    .map((sub, index) => {
      const saved = prefMap.get(sub.group_id)
      return {
        group_id: sub.group_id,
        sort_order: saved?.sort_order ?? index,
        enabled: saved?.enabled ?? true
      }
    })
    .sort((a, b) => a.sort_order - b.sort_order)
    .map((pref, index) => ({ ...pref, sort_order: index }))
}

function movePreference(index: number, direction: -1 | 1) {
  const targetIndex = index + direction
  if (targetIndex < 0 || targetIndex >= switchPreferences.value.length) return
  const next = [...switchPreferences.value]
  const [item] = next.splice(index, 1)
  next.splice(targetIndex, 0, item)
  switchPreferences.value = next.map((pref, i) => ({ ...pref, sort_order: i }))
}

function onPreferenceDragStart(groupID: number) {
  draggedPreferenceGroupID.value = groupID
}

function onPreferenceDrop(targetGroupID: number) {
  const sourceGroupID = draggedPreferenceGroupID.value
  draggedPreferenceGroupID.value = null
  if (!sourceGroupID || sourceGroupID === targetGroupID) return
  const next = [...switchPreferences.value]
  const from = next.findIndex((pref) => pref.group_id === sourceGroupID)
  const to = next.findIndex((pref) => pref.group_id === targetGroupID)
  if (from < 0 || to < 0) return
  const [item] = next.splice(from, 1)
  next.splice(to, 0, item)
  switchPreferences.value = next.map((pref, index) => ({ ...pref, sort_order: index }))
}

async function saveSwitchPreferences() {
  try {
    savingPreferences.value = true
    switchPreferences.value = await subscriptionsAPI.saveGroupPreferences(
      switchPreferences.value.map((pref, index) => ({ ...pref, sort_order: index }))
    )
    appStore.showSuccess(t('userSubscriptions.switchPrioritySaved'))
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('userSubscriptions.switchPrioritySaveFailed'))
  } finally {
    savingPreferences.value = false
  }
}

function positiveSubscriptionLimit(value: number | null | undefined): number | null {
  return typeof value === 'number' && value > 0 ? value : null
}

function isEntitlementBackedSubscription(subscription: UserSubscription): boolean {
  return subscription.entitlement_id != null || subscription.plan_id != null
}

function subscriptionDailyLimit(subscription: UserSubscription): number | null {
  const subscriptionLimit = positiveSubscriptionLimit(subscription.daily_limit_usd)
  if (isEntitlementBackedSubscription(subscription)) return subscriptionLimit
  return subscriptionLimit ?? positiveSubscriptionLimit(subscription.group?.daily_limit_usd)
}

function subscriptionWeeklyLimit(subscription: UserSubscription): number | null {
  const subscriptionLimit = positiveSubscriptionLimit(subscription.weekly_limit_usd)
  if (isEntitlementBackedSubscription(subscription)) return subscriptionLimit
  return subscriptionLimit ?? positiveSubscriptionLimit(subscription.group?.weekly_limit_usd)
}

function subscriptionMonthlyLimit(subscription: UserSubscription): number | null {
  const subscriptionLimit = positiveSubscriptionLimit(subscription.monthly_limit_usd)
  if (isEntitlementBackedSubscription(subscription)) return subscriptionLimit
  return subscriptionLimit ?? positiveSubscriptionLimit(subscription.group?.monthly_limit_usd)
}

function getProgressWidth(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return '0%'
  const percentage = Math.min(((used || 0) / limit) * 100, 100)
  return `${percentage}%`
}

function getProgressBarClass(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return 'bg-gray-400'
  const percentage = ((used || 0) / limit) * 100
  if (percentage >= 90) return 'bg-red-500'
  if (percentage >= 70) return 'bg-orange-500'
  return 'bg-green-500'
}

function formatExpirationDate(expiresAt: string): string {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))
  const relation = getExpirationDateRelation(expires, now)

  if (relation === null) return ''

  if (relation === 'expired') {
    return t('userSubscriptions.status.expired')
  }

  const dateStr = formatDateTimeToMinute(expires)

  if (relation === 'today') {
    return `${dateStr} (${t('common.today')})`
  }
  if (relation === 'tomorrow') {
    return `${dateStr} (${t('common.tomorrow')})`
  }

  return t('userSubscriptions.daysRemaining', { days }) + ` (${dateStr})`
}

function getExpirationClass(expiresAt: string): string {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))

  if (diff <= 0) return 'text-red-600 dark:text-red-400 font-medium'
  if (days <= 3) return 'text-red-600 dark:text-red-400'
  if (days <= 7) return 'text-orange-600 dark:text-orange-400'
  return 'text-gray-700 dark:text-gray-300'
}

function formatDurationParts(parts: RemainingDurationParts): string {
  if (parts.days > 0) {
    return `${parts.days}d ${parts.hours}h`
  }

  if (parts.hours > 0) {
    return `${parts.hours}h ${parts.minutes}m`
  }

  return `${parts.minutes}m`
}

function formatDailyUsageWindow(subscription: UserSubscription): string {
  if (isOneTimeDailyQuota(subscription) && subscription.expires_at) {
    const parts = getRemainingDurationParts(subscription.expires_at)
    if (!parts) return t('userSubscriptions.windowNotActive')
    return t('userSubscriptions.quotaEndsIn', { time: formatDurationParts(parts) })
  }

  return t('userSubscriptions.resetIn', {
    time: formatResetTime(subscription.daily_window_start, 24, subscription.starts_at)
  })
}

function formatResetTime(windowStart: string | null, windowHours: number, startsAt?: string): string {
  const end = getCycleResetAt(windowStart, startsAt, windowHours)
  if (!end) return t('userSubscriptions.windowNotActive')
  const parts = getRemainingDurationParts(end)

  return parts ? formatDurationParts(parts) : t('userSubscriptions.windowNotActive')
}

type EntitlementWindowKey = 'daily' | 'weekly' | 'monthly'

interface EntitlementUsageWindow {
  key: EntitlementWindowKey
  label: string
  used: number
  limit: number | null
  windowStart: string | null
  cycleHours: number
  resetsAt: string | null
  resetsInSeconds: number | null
}

function entitlementStatusRank(entitlement: UserEntitlement): number {
  const statusKey = entitlementStatusKey(entitlement)
  if (statusKey === 'active') return 0
  if (statusKey === 'future') return 1
  if (statusKey === 'expired') return 2
  return 3
}

function entitlementStatusKey(entitlement: UserEntitlement): string {
  if (entitlement.status === 'active') {
    const startsAt = new Date(entitlement.starts_at)
    const expiresAt = new Date(entitlement.expires_at)
    const now = new Date()
    if (isValidDate(startsAt) && startsAt.getTime() > now.getTime()) return 'future'
    if (isValidDate(expiresAt) && expiresAt.getTime() <= now.getTime()) return 'expired'
    return 'active'
  }
  return entitlement.status || 'unknown'
}

function entitlementStatusLabel(entitlement: UserEntitlement): string {
  return t(`userSubscriptions.entitlements.status.${entitlementStatusKey(entitlement)}`)
}

function entitlementStatusClass(entitlement: UserEntitlement): string {
  const statusKey = entitlementStatusKey(entitlement)
  if (statusKey === 'active') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
  if (statusKey === 'future') return 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
  if (statusKey === 'expired') return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-400'
  return 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
}

function entitlementUsageWindows(entitlement: UserEntitlement): EntitlementUsageWindow[] {
  return [
    {
      key: 'daily',
      label: t('userSubscriptions.entitlements.dailyQuota'),
      used: entitlement.daily_usage_usd || 0,
      limit: entitlement.daily_limit_usd,
      windowStart: entitlement.daily_window_start,
      cycleHours: 24,
      resetsAt: entitlement.daily_resets_at,
      resetsInSeconds: entitlement.daily_resets_in_seconds
    },
    {
      key: 'weekly',
      label: t('userSubscriptions.entitlements.weeklyQuota'),
      used: entitlement.weekly_usage_usd || 0,
      limit: entitlement.weekly_limit_usd,
      windowStart: entitlement.weekly_window_start,
      cycleHours: 168,
      resetsAt: entitlement.weekly_resets_at,
      resetsInSeconds: entitlement.weekly_resets_in_seconds
    },
    {
      key: 'monthly',
      label: t('userSubscriptions.entitlements.monthlyQuota'),
      used: entitlement.monthly_usage_usd || 0,
      limit: entitlement.monthly_limit_usd,
      windowStart: entitlement.monthly_window_start,
      cycleHours: 720,
      resetsAt: entitlement.monthly_resets_at,
      resetsInSeconds: entitlement.monthly_resets_in_seconds
    }
  ]
}

function visibleEntitlementUsageWindows(entitlement: UserEntitlement): EntitlementUsageWindow[] {
  const windows = entitlementUsageWindows(entitlement)
  const meaningfulWindows = windows.filter((window) => window.limit != null || window.used > 0)
  return meaningfulWindows.length > 0 ? meaningfulWindows : windows.slice(0, 1)
}

function formatEntitlementUsage(used: number, limit: number | null | undefined): string {
  const usedValue = `$${(used || 0).toFixed(2)}`
  if (limit == null) {
    return `${usedValue} / ${t('userSubscriptions.entitlements.unlimitedQuota')}`
  }
  return `${usedValue} / $${limit.toFixed(2)}`
}

function formatEntitlementReset(window: EntitlementUsageWindow, entitlement: UserEntitlement): string {
  if (window.limit == null || window.limit <= 0) {
    return t('userSubscriptions.windowNotActive')
  }
  if (window.resetsAt) {
    return formatDateTime(window.resetsAt)
  }
  if (typeof window.resetsInSeconds === 'number') {
    if (window.resetsInSeconds <= 0) {
      return t('userSubscriptions.entitlements.resetNow')
    }
    return formatPreciseDuration(window.resetsInSeconds)
  }
  const fallbackResetAt = getCycleResetAt(window.windowStart, entitlement.starts_at, window.cycleHours)
  if (fallbackResetAt) {
    const seconds = Math.ceil((fallbackResetAt.getTime() - Date.now()) / 1000)
    if (seconds <= 0) {
      return t('userSubscriptions.entitlements.resetNow')
    }
    return formatDateTime(fallbackResetAt)
  }
  return t('userSubscriptions.windowNotActive')
}

function entitlementOveragePolicyLabel(policy: string): string {
  if (policy === 'balance_fallback') {
    return t('userSubscriptions.entitlements.overage.balanceFallback')
  }
  if (policy === 'block') {
    return t('userSubscriptions.entitlements.overage.block')
  }
  return policy || '-'
}

function formatRateMultiplier(rate: number | null | undefined): string {
  const normalized = rate && rate > 0 ? rate : 1
  return Number(normalized.toPrecision(10)).toString()
}

function entitlementGroupEstimatedCost(
  entitlement: UserEntitlement,
  group: UserEntitlement['groups'][number]
): string {
  const unitCost = entitlement.unit_cost_per_usd
  if (!unitCost || unitCost <= 0) {
    return t('userSubscriptions.entitlements.priceUnavailable')
  }
  const rate = group.rate_multiplier && group.rate_multiplier > 0 ? group.rate_multiplier : 1
  return t('userSubscriptions.entitlements.groupUnitCost', {
    amount: formatCurrency(unitCost * rate, 'CNY')
  })
}

function entitlementDisplayName(entitlement: UserEntitlement): string {
  return entitlement.plan_name || entitlement.name || `Entitlement #${entitlement.id}`
}

function visibleEntitlementGroups(entitlement: UserEntitlement): UserEntitlement['groups'] {
  return entitlementGroupsForDisplay(entitlement).slice(0, entitlementGroupPreviewLimit)
}

function hiddenEntitlementGroupCount(entitlement: UserEntitlement): number {
  return Math.max(entitlement.groups.length - entitlementGroupPreviewLimit, 0)
}

function entitlementGroupsForDisplay(entitlement: UserEntitlement): UserEntitlement['groups'] {
  return sortGroupsForDisplay(entitlement.groups)
}

function openEntitlementGroupsModal(entitlement: UserEntitlement) {
  groupsModalEntitlement.value = entitlement
}

function closeEntitlementGroupsModal() {
  groupsModalEntitlement.value = null
}

function confirmDeleteEntitlement(entitlement: UserEntitlement) {
  deleteTarget.value = {
    type: 'entitlement',
    id: entitlement.id,
    name: entitlementDisplayName(entitlement)
  }
}

function confirmDeleteSubscription(subscription: UserSubscription) {
  deleteTarget.value = {
    type: 'subscription',
    id: subscription.id,
    name: subscriptionDisplayName(subscription, subscriptionPlans.value)
  }
}

function cancelDeleteSubscription() {
  if (deletingSubscription.value) return
  deleteTarget.value = null
}

async function deleteSelectedSubscription() {
  const target = deleteTarget.value
  if (!target || deletingSubscription.value) return
  try {
    deletingSubscription.value = true
    if (target.type === 'entitlement') {
      await subscriptionsAPI.deleteEntitlement(target.id)
    } else {
      await subscriptionsAPI.deleteSubscription(target.id)
    }
    appStore.showSuccess(t('userSubscriptions.deleteSuccess'))
    deleteTarget.value = null
    await loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('userSubscriptions.deleteFailed'))
  } finally {
    deletingSubscription.value = false
  }
}

function canAdvanceMonthlyCycle(subscription: UserSubscription): boolean {
  if (subscription.entitlement_id) {
    return false
  }
  const limit = subscription.group?.monthly_limit_usd
  if (
    subscription.status !== 'active' ||
    !subscription.expires_at ||
    !limit ||
    limit <= 0 ||
    !hasReachedMonthlyCycleAdvanceThreshold(subscription)
  ) {
    return false
  }

  const now = new Date()
  const resetAt = getMonthlyResetAt(subscription, now)
  const expiresAt = new Date(subscription.expires_at)
  if (!isValidDate(resetAt) || !isValidDate(expiresAt) || resetAt.getTime() <= now.getTime()) {
    return false
  }
  if (!hasFullNextMonthlyCycle(subscription, resetAt, expiresAt)) {
    return false
  }

  const deductedSeconds = estimateDeductedSeconds(subscription, now)
  const newExpiresAt = new Date(expiresAt)
  newExpiresAt.setSeconds(newExpiresAt.getSeconds() - deductedSeconds)
  return newExpiresAt.getTime() > now.getTime()
}

function hasReachedMonthlyCycleAdvanceThreshold(subscription: UserSubscription): boolean {
  const limit = subscription.group?.monthly_limit_usd
  if (!limit || limit <= 0) return false
  return (subscription.monthly_usage_usd || 0) >= limit * monthlyCycleAdvanceThreshold
}

function advanceMonthlyCycleHint(subscription: UserSubscription): string {
  if (subscription.entitlement_id) {
    return t('userSubscriptions.advanceMonthlyUnavailableAlias')
  }
  const limit = subscription.group?.monthly_limit_usd
  if (subscription.status !== 'active') {
    return t('userSubscriptions.advanceMonthlyUnavailableInactive')
  }
  if (!subscription.expires_at) {
    return t('userSubscriptions.advanceMonthlyUnavailableNoExpiration')
  }
  if (!limit || limit <= 0) {
    return t('userSubscriptions.advanceMonthlyUnavailableNoMonthlyLimit')
  }
  if (!hasReachedMonthlyCycleAdvanceThreshold(subscription)) {
    return t('userSubscriptions.advanceMonthlyThresholdHint', {
      percent: Math.round((1 - monthlyCycleAdvanceThreshold) * 100)
    })
  }
  const now = new Date()
  const resetAt = getMonthlyResetAt(subscription, now)
  const expiresAt = new Date(subscription.expires_at)
  if (!isValidDate(resetAt) || !isValidDate(expiresAt) || resetAt.getTime() <= now.getTime()) {
    return t('userSubscriptions.advanceMonthlyUnavailableWindow')
  }
  if (!hasFullNextMonthlyCycle(subscription, resetAt, expiresAt)) {
    return t('userSubscriptions.advanceMonthlyUnavailableValidity')
  }
  const deductedSeconds = estimateDeductedSeconds(subscription, now)
  const newExpiresAt = new Date(expiresAt)
  newExpiresAt.setSeconds(newExpiresAt.getSeconds() - deductedSeconds)
  if (newExpiresAt.getTime() <= now.getTime()) {
    return t('userSubscriptions.advanceMonthlyUnavailableValidity')
  }
  return t('userSubscriptions.advanceMonthlyAvailableHint', { duration: formatPreciseDuration(deductedSeconds) })
}

function canAdvanceEntitlementMonthlyCycle(entitlement: UserEntitlement): boolean {
  const limit = entitlement.monthly_limit_usd
  if (
    entitlementStatusKey(entitlement) !== 'active' ||
    !entitlement.expires_at ||
    !limit ||
    limit <= 0 ||
    !hasReachedEntitlementMonthlyCycleAdvanceThreshold(entitlement)
  ) {
    return false
  }

  const now = new Date()
  const resetAt = getEntitlementMonthlyResetAt(entitlement, now)
  const expiresAt = new Date(entitlement.expires_at)
  if (!isValidDate(resetAt) || !isValidDate(expiresAt) || resetAt.getTime() <= now.getTime()) {
    return false
  }
  if (!hasFullNextEntitlementMonthlyCycle(entitlement, resetAt, expiresAt)) {
    return false
  }

  const deductedSeconds = estimateEntitlementDeductedSeconds(entitlement, now)
  const newExpiresAt = new Date(expiresAt)
  newExpiresAt.setSeconds(newExpiresAt.getSeconds() - deductedSeconds)
  return newExpiresAt.getTime() > now.getTime()
}

function hasReachedEntitlementMonthlyCycleAdvanceThreshold(entitlement: UserEntitlement): boolean {
  const limit = entitlement.monthly_limit_usd
  if (!limit || limit <= 0) return false
  return (entitlement.monthly_usage_usd || 0) >= limit * monthlyCycleAdvanceThreshold
}

function advanceEntitlementMonthlyCycleHint(entitlement: UserEntitlement): string {
  const limit = entitlement.monthly_limit_usd
  if (entitlementStatusKey(entitlement) !== 'active') {
    return t('userSubscriptions.advanceEntitlementMonthlyUnavailableInactive')
  }
  if (!entitlement.expires_at) {
    return t('userSubscriptions.advanceEntitlementMonthlyUnavailableNoExpiration')
  }
  if (!limit || limit <= 0) {
    return t('userSubscriptions.advanceEntitlementMonthlyUnavailableNoMonthlyLimit')
  }
  if (!hasReachedEntitlementMonthlyCycleAdvanceThreshold(entitlement)) {
    return t('userSubscriptions.advanceEntitlementMonthlyThresholdHint', {
      percent: Math.round((1 - monthlyCycleAdvanceThreshold) * 100)
    })
  }
  const now = new Date()
  const resetAt = getEntitlementMonthlyResetAt(entitlement, now)
  const expiresAt = new Date(entitlement.expires_at)
  if (!isValidDate(resetAt) || !isValidDate(expiresAt) || resetAt.getTime() <= now.getTime()) {
    return t('userSubscriptions.advanceEntitlementMonthlyUnavailableWindow')
  }
  if (!hasFullNextEntitlementMonthlyCycle(entitlement, resetAt, expiresAt)) {
    return t('userSubscriptions.advanceEntitlementMonthlyUnavailableValidity')
  }
  const deductedSeconds = estimateEntitlementDeductedSeconds(entitlement, now)
  const newExpiresAt = new Date(expiresAt)
  newExpiresAt.setSeconds(newExpiresAt.getSeconds() - deductedSeconds)
  if (newExpiresAt.getTime() <= now.getTime()) {
    return t('userSubscriptions.advanceEntitlementMonthlyUnavailableValidity')
  }
  return t('userSubscriptions.advanceEntitlementMonthlyAvailableHint', { duration: formatPreciseDuration(deductedSeconds) })
}

function getEntitlementMonthlyResetAt(entitlement: UserEntitlement, now = new Date()): Date {
  return getCycleResetAt(entitlement.monthly_window_start, entitlement.starts_at, 720, now)
    || new Date(now.getTime() + monthlyCycleDurationMs)
}

function hasFullNextEntitlementMonthlyCycle(
  entitlement: UserEntitlement,
  resetAt: Date,
  expiresAt: Date
): boolean {
  const startsAt = new Date(entitlement.starts_at)
  if (!isValidDate(startsAt)) return false
  if (expiresAt.getTime() <= startsAt.getTime() + monthlyCycleDurationMs) return false
  return expiresAt.getTime() + monthlyCycleAdvanceValidityGraceMs >= resetAt.getTime() + monthlyCycleDurationMs
}

function estimateEntitlementDeductedSeconds(entitlement: UserEntitlement, now = new Date()): number {
  const resetAt = getEntitlementMonthlyResetAt(entitlement, now)
  const diff = resetAt.getTime() - now.getTime()
  return Math.max(Math.ceil(diff / 1000), 1)
}

function getMonthlyResetAt(subscription: UserSubscription, now = new Date()): Date {
  return getCycleResetAt(subscription.monthly_window_start, subscription.starts_at, 720, now)
    || new Date(now.getTime() + monthlyCycleDurationMs)
}

function isValidDate(value: Date): boolean {
  return !Number.isNaN(value.getTime())
}

function hasFullNextMonthlyCycle(
  subscription: UserSubscription,
  resetAt: Date,
  expiresAt: Date
): boolean {
  const startsAt = new Date(subscription.starts_at)
  if (!isValidDate(startsAt)) return false
  if (expiresAt.getTime() <= startsAt.getTime() + monthlyCycleDurationMs) return false
  return expiresAt.getTime() + monthlyCycleAdvanceValidityGraceMs >= resetAt.getTime() + monthlyCycleDurationMs
}

function estimateDeductedSeconds(subscription: UserSubscription, now = new Date()): number {
  const resetAt = getMonthlyResetAt(subscription, now)
  const diff = resetAt.getTime() - now.getTime()
  return Math.max(Math.ceil(diff / 1000), 1)
}

function formatPreciseDuration(totalSeconds: number): string {
  const seconds = Math.max(Math.floor(totalSeconds), 0)
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const remainingSeconds = seconds % 60
  const zh = locale.value.toLowerCase().startsWith('zh')

  if (zh) {
    const parts: string[] = []
    if (days) parts.push(`${days}天`)
    if (hours) parts.push(`${hours}小时`)
    if (minutes) parts.push(`${minutes}分钟`)
    if (remainingSeconds || parts.length === 0) parts.push(`${remainingSeconds}秒`)
    return parts.join(' ')
  }

  const parts: string[] = []
  if (days) parts.push(`${days}d`)
  if (hours) parts.push(`${hours}h`)
  if (minutes) parts.push(`${minutes}m`)
  if (remainingSeconds || parts.length === 0) parts.push(`${remainingSeconds}s`)
  return parts.join(' ')
}

async function advanceMonthlyCycle(subscription: UserSubscription) {
  if (!canAdvanceMonthlyCycle(subscription)) return
  const groupName = subscriptionDisplayName(subscription, subscriptionPlans.value)
  const deductedSeconds = estimateDeductedSeconds(subscription)
  const limit = subscription.group?.monthly_limit_usd || 0
  const used = subscription.monthly_usage_usd || 0
  const remaining = Math.max(limit - used, 0)
  if (!window.confirm(t('userSubscriptions.advanceMonthlyConfirm', {
    group: groupName,
    duration: formatPreciseDuration(deductedSeconds),
    used: used.toFixed(2),
    limit: limit.toFixed(2),
    remaining: remaining.toFixed(2)
  }))) {
    return
  }
  try {
    advancingSubscriptionId.value = subscription.id
    const result = await subscriptionsAPI.advanceMonthlyCycle(subscription.id)
    appStore.showSuccess(
      t('userSubscriptions.advanceMonthlySuccess', { duration: formatPreciseDuration(result.deducted_seconds) })
    )
    await loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('userSubscriptions.advanceMonthlyFailed'))
  } finally {
    advancingSubscriptionId.value = null
  }
}

async function advanceEntitlementMonthlyCycle(entitlement: UserEntitlement) {
  if (!canAdvanceEntitlementMonthlyCycle(entitlement)) return
  const entitlementName = entitlementDisplayName(entitlement)
  const deductedSeconds = estimateEntitlementDeductedSeconds(entitlement)
  const limit = entitlement.monthly_limit_usd || 0
  const used = entitlement.monthly_usage_usd || 0
  const remaining = Math.max(limit - used, 0)
  if (!window.confirm(t('userSubscriptions.advanceEntitlementMonthlyConfirm', {
    entitlement: entitlementName,
    duration: formatPreciseDuration(deductedSeconds),
    used: used.toFixed(2),
    limit: limit.toFixed(2),
    remaining: remaining.toFixed(2)
  }))) {
    return
  }
  try {
    advancingEntitlementId.value = entitlement.id
    const result = await subscriptionsAPI.advanceEntitlementMonthlyCycle(entitlement.id)
    appStore.showSuccess(
      t('userSubscriptions.advanceEntitlementMonthlySuccess', { duration: formatPreciseDuration(result.deducted_seconds) })
    )
    await loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('userSubscriptions.advanceEntitlementMonthlyFailed'))
  } finally {
    advancingEntitlementId.value = null
  }
}

onMounted(() => {
  loadSubscriptions()
})
</script>
