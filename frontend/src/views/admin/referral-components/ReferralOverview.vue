<template>
  <div class="space-y-6">
    <!-- Header -->
    <div class="flex flex-col gap-4 border-b border-gray-200/80 pb-6 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
      <div class="min-w-0">
        <div class="inline-flex items-center gap-2 rounded-full bg-primary-50 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">
          <span class="h-1.5 w-1.5 rounded-full bg-primary-500" />
          {{ t('admin.referral.menuCommission', '数据总览') }}
        </div>
        <h1 class="mt-2 text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
          {{ t('admin.referral.title', '返佣管理总览') }}
        </h1>
        <p class="mt-1 max-w-xl text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.referral.description', '查看全局分销账号、佣金趋势并处理提现。') }}
        </p>
      </div>

      <div class="flex w-full flex-col gap-3 sm:flex-row sm:items-center lg:w-auto">
        <div class="w-full sm:w-[320px]">
          <ReferralAccountPicker
            :label="''"
            :placeholder="t('admin.referral.quickOpenAccountPlaceholder', '搜索账号邮箱直达工作台...')"
            :query="quickAccountQuery"
            :model-value="selectedQuickAccount"
            :options="quickAccountResults"
            :loading="quickAccountLoading"
            input-test-id="quick-account-input"
            @update:query="emit('updateQuickAccountQuery', $event)"
            @search="emit('searchQuickAccounts', $event)"
            @select="emit('openWorkspaceFromAccount', $event)"
            @clear="emit('clearQuickAccount')"
          />
        </div>
        <button
          type="button"
          class="inline-flex h-10 shrink-0 items-center justify-center gap-2 rounded-xl border border-gray-200 bg-white px-4 text-sm font-semibold text-gray-700 shadow-sm transition hover:bg-gray-50 disabled:opacity-60 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
          :disabled="loading"
          @click="emit('refresh')"
        >
          <svg class="h-4 w-4" :class="loading ? 'animate-spin' : ''" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          {{ t('common.refresh', '刷新数据') }}
        </button>
      </div>
    </div>

    <!-- Tabs -->
    <nav class="flex gap-1 overflow-x-auto rounded-xl border border-gray-200 bg-gray-50/80 p-1 dark:border-dark-700 dark:bg-dark-800/60" aria-label="Tabs">
      <router-link
        to="/admin/referral"
        class="whitespace-nowrap rounded-lg bg-white px-4 py-2 text-sm font-semibold text-primary-700 shadow-sm ring-1 ring-black/5 dark:bg-dark-900 dark:text-primary-300 dark:ring-white/10"
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
        class="whitespace-nowrap rounded-lg px-4 py-2 text-sm font-medium text-gray-600 transition hover:bg-white/80 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-900/60 dark:hover:text-gray-200"
      >
        {{ t('admin.referral.menuWithdrawals', '批量提现审核') }}
      </router-link>
    </nav>

    <div v-if="loading && !overview" class="flex flex-col items-center justify-center py-24">
      <LoadingSpinner class="mb-3 h-8 w-8 text-primary-500" />
      <span class="text-sm text-gray-500">{{ t('common.loading', '加载中...') }}</span>
    </div>

    <template v-else-if="overview">
      <!-- Metric cards -->
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <div class="group relative overflow-hidden rounded-2xl border border-emerald-100 bg-gradient-to-br from-emerald-50/90 via-white to-white p-5 shadow-sm dark:border-emerald-900/40 dark:from-emerald-950/40 dark:via-dark-900 dark:to-dark-900">
          <div class="flex items-start justify-between gap-3">
            <div>
              <p class="text-xs font-semibold uppercase tracking-wide text-emerald-700/80 dark:text-emerald-300/80">
                {{ t('admin.referral.availableCommission', '当前可提现总量') }}
              </p>
              <p class="mt-3 text-3xl font-bold tabular-nums tracking-tight text-gray-900 dark:text-white">
                <span class="text-lg font-semibold text-gray-400">￥</span>{{ formatMoney(overview.available_commission) }}
              </p>
              <p class="mt-2 text-xs font-medium text-emerald-700 dark:text-emerald-300">
                {{ t('admin.referral.fromActiveRelations', { count: overview.total_bound_users }) }}
              </p>
            </div>
            <span class="inline-flex h-10 w-10 items-center justify-center rounded-xl bg-emerald-500/10 text-emerald-600 dark:text-emerald-300">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8" d="M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8V7m0 10v1m8-6a8 8 0 11-16 0 8 8 0 0116 0z" /></svg>
            </span>
          </div>
        </div>

        <div class="group relative overflow-hidden rounded-2xl border border-sky-100 bg-gradient-to-br from-sky-50/90 via-white to-white p-5 shadow-sm dark:border-sky-900/40 dark:from-sky-950/40 dark:via-dark-900 dark:to-dark-900">
          <div class="flex items-start justify-between gap-3">
            <div>
              <p class="text-xs font-semibold uppercase tracking-wide text-sky-700/80 dark:text-sky-300/80">
                {{ t('admin.referral.pendingCommission', '未出安全期总计') }}
              </p>
              <p class="mt-3 text-3xl font-bold tabular-nums tracking-tight text-gray-900 dark:text-white">
                <span class="text-lg font-semibold text-gray-400">￥</span>{{ formatMoney(overview.pending_commission) }}
              </p>
              <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.referral.pendingCommissionHint', '未结转的待生效佣金') }}
              </p>
            </div>
            <span class="inline-flex h-10 w-10 items-center justify-center rounded-xl bg-sky-500/10 text-sky-600 dark:text-sky-300">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
            </span>
          </div>
        </div>

        <div class="group relative overflow-hidden rounded-2xl border border-amber-100 bg-gradient-to-br from-amber-50/90 via-white to-white p-5 shadow-sm dark:border-amber-900/40 dark:from-amber-950/40 dark:via-dark-900 dark:to-dark-900">
          <div class="flex items-start justify-between gap-3">
            <div>
              <p class="text-xs font-semibold uppercase tracking-wide text-amber-700/80 dark:text-amber-300/80">
                {{ t('admin.referral.frozenCommissionTitle', '提现冻结中') }}
              </p>
              <p class="mt-3 text-3xl font-bold tabular-nums tracking-tight text-gray-900 dark:text-white">
                <span class="text-lg font-semibold text-gray-400">￥</span>{{ formatMoney(overview.frozen_commission) }}
              </p>
              <div class="mt-2 flex flex-wrap gap-1.5">
                <span class="inline-flex items-center rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-medium text-amber-800 dark:bg-amber-900/40 dark:text-amber-200">
                  {{ t('admin.referral.chipPendingReview', '待审') }} {{ overview.pending_withdrawal_count || 0 }}
                </span>
                <span class="inline-flex items-center rounded-full bg-blue-100 px-2 py-0.5 text-[11px] font-medium text-blue-800 dark:bg-blue-900/40 dark:text-blue-200">
                  {{ t('admin.referral.chipPendingPayout', '待打款') }} {{ overview.approved_withdrawal_count || 0 }}
                </span>
              </div>
            </div>
            <span class="inline-flex h-10 w-10 items-center justify-center rounded-xl bg-amber-500/10 text-amber-600 dark:text-amber-300">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" /></svg>
            </span>
          </div>
        </div>

        <div class="group relative overflow-hidden rounded-2xl border border-indigo-100 bg-gradient-to-br from-indigo-50/90 via-white to-white p-5 shadow-sm dark:border-indigo-900/40 dark:from-indigo-950/40 dark:via-dark-900 dark:to-dark-900">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0 flex-1">
              <p class="text-xs font-semibold uppercase tracking-wide text-indigo-700/80 dark:text-indigo-300/80">
                {{ t('admin.referral.withdrawnCommission', '已清算佣金') }}
              </p>
              <p class="mt-3 text-3xl font-bold tabular-nums tracking-tight text-gray-900 dark:text-white">
                <span class="text-lg font-semibold text-gray-400">￥</span>{{ formatMoney(overview.withdrawn_commission) }}
              </p>
              <div class="mt-3 grid grid-cols-2 gap-2">
                <div class="rounded-xl bg-white/80 px-2.5 py-2 ring-1 ring-emerald-100 dark:bg-dark-800/80 dark:ring-emerald-900/40">
                  <p class="text-[10px] font-medium text-gray-500 dark:text-gray-400">{{ t('admin.referral.cashNetLabel', '现金净额') }}</p>
                  <p class="mt-0.5 text-sm font-bold tabular-nums text-emerald-700 dark:text-emerald-300">￥{{ formatMoney(overview.cash_paid_commission || 0) }}</p>
                </div>
                <div class="rounded-xl bg-white/80 px-2.5 py-2 ring-1 ring-indigo-100 dark:bg-dark-800/80 dark:ring-indigo-900/40">
                  <p class="text-[10px] font-medium text-gray-500 dark:text-gray-400">转余额扣佣</p>
                  <p class="mt-0.5 text-sm font-bold tabular-nums text-indigo-700 dark:text-indigo-300">￥{{ formatMoney(overview.credit_converted_commission || 0) }}</p>
                </div>
              </div>
              <p
                v-if="(overview.negative_commission_debt || 0) > 0"
                class="mt-2 inline-flex items-center rounded-full bg-red-50 px-2 py-0.5 text-[11px] font-semibold text-red-700 dark:bg-red-900/30 dark:text-red-300"
              >
                {{ t('admin.referral.negativeDebtLabel', '负佣金欠账') }} ￥{{ formatMoney(overview.negative_commission_debt || 0) }}
              </p>
            </div>
          </div>
        </div>
      </div>

      <!-- Chart + side rails -->
      <div class="grid grid-cols-1 gap-5 xl:grid-cols-3">
        <div class="flex min-h-[360px] flex-col rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900 xl:col-span-2">
          <div class="mb-4 flex items-center justify-between gap-3">
            <div>
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.referral.trendTitle', '近 7 天收支走势') }}</h2>
              <p class="mt-0.5 text-xs text-gray-500">{{ t('admin.referral.trendSubtitle', '返佣入账 vs 提现/转余额流出') }}</p>
            </div>
          </div>
          <div class="min-h-0 flex-1">
            <Bar v-if="chartData" :data="chartData" :options="chartOptions" />
          </div>
        </div>

        <div class="flex flex-col gap-5">
          <!-- Pending cash -->
          <section class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <div class="flex items-center justify-between gap-2 border-b border-gray-100 px-4 py-3.5 dark:border-dark-700">
              <div class="flex items-center gap-2">
                <span class="inline-flex h-7 w-7 items-center justify-center rounded-lg bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300">
                  <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" /></svg>
                </span>
                <div>
                  <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.referral.pendingWithdrawalsTitle', '待审现金提现') }}</h2>
                  <p class="text-[11px] text-gray-500">{{ t('admin.referral.pendingWithdrawalsHint', '需人工审核') }}</p>
                </div>
              </div>
              <router-link
                to="/admin/referral-withdrawals?kind=cash&status=pending_review"
                class="text-xs font-semibold text-primary-600 hover:text-primary-700 dark:text-primary-400"
              >
                {{ t('admin.referral.viewAll', '全部 →') }}
              </router-link>
            </div>
            <div class="max-h-[220px] overflow-y-auto">
              <div v-if="pendingWithdrawals?.items?.length" class="divide-y divide-gray-100 dark:divide-dark-800">
                <div
                  v-for="item in pendingWithdrawals.items"
                  :key="item.id"
                  class="flex items-center justify-between gap-3 px-4 py-3.5 transition hover:bg-amber-50/50 dark:hover:bg-amber-950/20"
                >
                  <div class="min-w-0">
                    <p class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ item.user_email }}</p>
                    <p class="mt-0.5 font-mono text-[11px] text-gray-400">{{ item.withdrawal_no }}</p>
                  </div>
                  <div class="shrink-0 text-right">
                    <p class="text-sm font-bold tabular-nums text-gray-900 dark:text-white">￥{{ formatMoney(item.net_amount) }}</p>
                    <button
                      type="button"
                      class="mt-1.5 rounded-lg bg-primary-600 px-2.5 py-1 text-[11px] font-semibold text-white shadow-sm transition hover:bg-primary-500"
                      @click="emit('openWorkspaceFromWithdrawal', item)"
                    >
                      {{ t('admin.referral.goToProcess', '去处理') }}
                    </button>
                  </div>
                </div>
              </div>
              <div v-else class="flex flex-col items-center px-4 py-10 text-center">
                <div class="mb-2 flex h-12 w-12 items-center justify-center rounded-2xl bg-gray-50 text-gray-300 dark:bg-dark-800">
                  <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M5 13l4 4L19 7" /></svg>
                </div>
                <p class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.referral.noPendingCash', '暂无待审提现') }}</p>
                <p class="mt-1 text-xs text-gray-400">{{ t('admin.referral.noPendingRecords', '队列空闲') }}</p>
              </div>
            </div>
          </section>

          <!-- Recent credit -->
          <section class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <div class="flex items-center justify-between gap-2 border-b border-gray-100 px-4 py-3.5 dark:border-dark-700">
              <div class="flex items-center gap-2">
                <span class="inline-flex h-7 w-7 items-center justify-center rounded-lg bg-indigo-100 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300">
                  <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" /></svg>
                </span>
                <div>
                  <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.referral.recentCreditTitle', '最近转余额') }}</h2>
                  <p class="text-[11px] text-gray-500">{{ t('admin.referral.recentCreditHint', '自助完成 · 无需审核') }}</p>
                </div>
              </div>
              <router-link
                to="/admin/referral-withdrawals?kind=credit"
                class="text-xs font-semibold text-indigo-600 hover:text-indigo-700 dark:text-indigo-400"
              >
                {{ t('admin.referral.viewAll', '全部 →') }}
              </router-link>
            </div>
            <div class="max-h-[180px] overflow-y-auto">
              <div v-if="overview.recent_credit_conversions?.length" class="divide-y divide-gray-100 dark:divide-dark-800">
                <div
                  v-for="item in overview.recent_credit_conversions"
                  :key="item.id"
                  class="flex items-center justify-between gap-3 px-4 py-3 transition hover:bg-indigo-50/40 dark:hover:bg-indigo-950/20"
                >
                  <div class="min-w-0">
                    <p class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ item.user_email || item.username || '-' }}</p>
                    <p class="mt-0.5 text-[11px] text-gray-400">{{ formatDateTime(item.paid_at || item.created_at) }}</p>
                  </div>
                  <p class="shrink-0 text-sm font-bold tabular-nums text-indigo-600 dark:text-indigo-300">￥{{ formatMoney(item.amount) }}</p>
                </div>
              </div>
              <div v-else class="flex flex-col items-center px-4 py-8 text-center">
                <p class="text-sm text-gray-500">{{ t('admin.referral.noCreditConversionRecords', '暂无转余额记录') }}</p>
              </div>
            </div>
          </section>
        </div>
      </div>

      <!-- Ranking -->
      <section class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div class="flex items-center justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.referral.rankingTitle', '返佣代理排行榜') }}</h2>
            <p class="mt-0.5 text-xs text-gray-500">{{ t('admin.referral.rankingSubtitle', '按累计佣金排序 · 二级人数仅统计不产生返利') }}</p>
          </div>
        </div>
        <div class="overflow-x-auto">
          <table class="min-w-full text-left text-sm">
            <thead>
              <tr class="border-b border-gray-100 bg-gray-50/80 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:border-dark-700 dark:bg-dark-800/50 dark:text-gray-400">
                <th class="w-16 px-5 py-3 text-center">{{ t('admin.referral.rankColumn', '排名') }}</th>
                <th class="px-5 py-3">{{ t('admin.referral.agentAccount', '代理账号') }}</th>
                <th class="px-5 py-3 text-center">{{ t('admin.referral.userScale', '用户规模') }}</th>
                <th class="px-5 py-3 text-right">{{ t('admin.referral.totalEarnings', '累计佣金') }}</th>
                <th class="px-5 py-3 text-right">{{ t('admin.referral.availableBalance', '可提现') }}</th>
                <th class="px-5 py-3 text-center">{{ t('admin.referral.actions', '操作') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
              <tr
                v-for="(item, index) in overview.ranking"
                :key="item.user_id"
                class="transition hover:bg-gray-50/80 dark:hover:bg-dark-800/40"
              >
                <td class="px-5 py-4 text-center">
                  <span
                    class="inline-flex h-7 w-7 items-center justify-center rounded-full text-xs font-bold"
                    :class="rankBadgeClass(index)"
                  >
                    {{ index + 1 }}
                  </span>
                </td>
                <td class="px-5 py-4">
                  <div class="font-medium text-gray-900 dark:text-white">{{ item.email }}</div>
                  <div class="mt-1 inline-flex items-center rounded-md bg-gray-100 px-1.5 py-0.5 font-mono text-[11px] text-gray-500 dark:bg-dark-700 dark:text-gray-400">
                    {{ item.referral_code || t('admin.referral.notSet', '未设置') }}
                  </div>
                </td>
                <td class="px-5 py-4 text-center">
                  <div class="inline-flex flex-col items-center gap-1">
                    <span class="rounded-full bg-primary-50 px-2.5 py-0.5 text-xs font-semibold text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">
                      {{ t('admin.referral.directInviteesShort', '直邀') }} {{ item.direct_invitees }}
                    </span>
                    <span class="text-[11px] text-gray-400" :title="t('admin.referral.secondLevelNoRebateHint', '二级仅统计人数，不产生返利')">
                      {{ t('admin.referral.secondLevelShort', '二级') }} {{ item.second_level_invitees || 0 }} · {{ t('admin.referral.noRebateTag', '无返利') }}
                    </span>
                  </div>
                </td>
                <td class="px-5 py-4 text-right">
                  <span class="font-semibold tabular-nums text-gray-900 dark:text-white">￥{{ formatMoney(item.total_commission) }}</span>
                </td>
                <td class="px-5 py-4 text-right">
                  <span class="font-semibold tabular-nums text-emerald-600 dark:text-emerald-400">￥{{ formatMoney(item.available_commission) }}</span>
                </td>
                <td class="px-5 py-4">
                  <div class="flex items-center justify-center gap-2">
                    <button
                      type="button"
                      class="rounded-lg border border-gray-200 bg-white px-3 py-1.5 text-xs font-semibold text-gray-700 shadow-sm transition hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
                      @click="emit('openTree', item)"
                    >
                      {{ t('admin.referral.structureTree', '结构树') }}
                    </button>
                    <button
                      type="button"
                      class="rounded-lg bg-primary-600 px-3 py-1.5 text-xs font-semibold text-white shadow-sm transition hover:bg-primary-500"
                      @click="emit('openWorkspace', item)"
                    >
                      {{ t('admin.referral.workspace', '工作台') }}
                    </button>
                  </div>
                </td>
              </tr>
              <tr v-if="!overview.ranking.length">
                <td colspan="6" class="px-5 py-16 text-center">
                  <p class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.referral.noRankingData', '暂无排行数据') }}</p>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import ReferralAccountPicker from '@/components/admin/referral/ReferralAccountPicker.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { formatDateTime } from '@/utils/format'
import type {
  AdminReferralOverview,
  AdminCommissionWithdrawal,
  AdminReferralAccountOption,
  AdminReferralRankingItem,
  BasePaginationResponse
} from '@/types'

import {
  Chart as ChartJS,
  Title,
  Tooltip,
  Legend,
  BarElement,
  CategoryScale,
  LinearScale
} from 'chart.js'
import { Bar } from 'vue-chartjs'

ChartJS.register(CategoryScale, LinearScale, BarElement, Title, Tooltip, Legend)

const { t } = useI18n()

interface Props {
  loading: boolean
  overview: AdminReferralOverview | null
  pendingWithdrawals: BasePaginationResponse<AdminCommissionWithdrawal> | null
  quickAccountQuery: string
  quickAccountResults: AdminReferralAccountOption[]
  selectedQuickAccount: AdminReferralAccountOption | null
  quickAccountLoading: boolean
}

const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'refresh'): void
  (e: 'updateQuickAccountQuery', val: string): void
  (e: 'searchQuickAccounts', query: string): void
  (e: 'openWorkspaceFromAccount', item: AdminReferralAccountOption): void
  (e: 'clearQuickAccount'): void
  (e: 'openTree', item: any): void
  (e: 'openWorkspace', item: AdminReferralRankingItem): void
  (e: 'openWorkspaceFromWithdrawal', item: AdminCommissionWithdrawal): void
}>()

const chartData = computed(() => {
  if (!props.overview || !props.overview.recent_trend) return null
  const trend = props.overview.recent_trend
  return {
    labels: trend.map(p => formatTrendDate(p.date)),
    datasets: [
      {
        label: t('admin.referral.rewardTrendLegend', '代理新增返佣'),
        backgroundColor: 'rgba(16, 185, 129, 0.85)',
        data: trend.map(p => p.reward_amount),
        borderRadius: 6,
        maxBarThickness: 36
      },
      {
        label: t('admin.referral.withdrawTrendLegend', '代理提现流出'),
        backgroundColor: 'rgba(99, 102, 241, 0.85)',
        data: trend.map(p => p.withdrawal_amount),
        borderRadius: 6,
        maxBarThickness: 36
      }
    ]
  }
})

const chartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: {
      position: 'bottom' as const,
      labels: {
        usePointStyle: true,
        padding: 20,
        boxWidth: 8
      }
    },
    tooltip: {
      mode: 'index' as const,
      intersect: false,
      callbacks: {
        label: function (context: any) {
          let label = context.dataset.label || ''
          if (label) label += ': '
          if (context.parsed.y !== null) {
            label += '￥' + context.parsed.y.toFixed(2)
          }
          return label
        }
      }
    }
  },
  scales: {
    y: {
      beginAtZero: true,
      border: { display: false },
      grid: {
        color: 'rgba(156, 163, 175, 0.12)',
        drawBorder: false
      }
    },
    x: {
      border: { display: false },
      grid: { display: false }
    }
  },
  interaction: {
    mode: 'index' as const,
    intersect: false
  }
}

function formatMoney(value: number | undefined) {
  return Number(value || 0).toFixed(2)
}

function formatTrendDate(value: string) {
  const parsed = new Date(`${value}T00:00:00`)
  return `${parsed.getMonth() + 1}/${parsed.getDate()}`
}

function rankBadgeClass(index: number): string {
  if (index === 0) return 'bg-amber-400 text-white shadow-sm shadow-amber-200 dark:shadow-none'
  if (index === 1) return 'bg-slate-300 text-slate-800 dark:bg-slate-500 dark:text-white'
  if (index === 2) return 'bg-orange-300 text-orange-900 dark:bg-orange-700 dark:text-orange-50'
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
}
</script>
