<template>
  <AppLayout>
    <div class="mx-auto w-full max-w-[1080px] space-y-5 px-1 sm:px-0">
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-[28px] font-semibold tracking-tight text-[#1d1d1f] dark:text-white">
            {{ t('referral.title', '邀请') }}
          </h1>
          <p class="mt-0.5 text-[14px] text-[#86868b]">
            {{ t('referral.description', '分享链接，好友充值获得佣金') }}
          </p>
        </div>
        <button
          type="button"
          class="inline-flex h-9 items-center justify-center gap-1.5 rounded-full bg-[#f5f5f7] px-4 text-[13px] font-medium text-[#1d1d1f] transition hover:bg-[#e8e8ed] disabled:opacity-50 dark:bg-white/10 dark:text-white dark:hover:bg-white/15"
          :disabled="loading"
          @click="loadAll"
        >
          <svg class="h-3.5 w-3.5" :class="loading ? 'animate-spin' : ''" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          {{ t('common.refresh', '刷新') }}
        </button>
      </div>

      <div v-if="loading && !overview" class="flex items-center justify-center py-20 text-sm text-slate-500">
        <LoadingSpinner />
        <span class="ml-2">{{ t('common.loading', '加载中') }}</span>
      </div>

      <template v-else-if="overview && !overview.referral_enabled">
        <section class="rounded-2xl border border-dashed border-slate-200 bg-slate-50 p-10 text-center dark:border-dark-700 dark:bg-dark-900">
          <h2 class="text-xl font-semibold text-slate-900 dark:text-white">
            {{ t('referral.disabledTitle', '邀请功能未开启') }}
          </h2>
          <p class="mt-2 text-sm text-slate-500 dark:text-slate-400">
            {{ t('referral.disabledDescription', '当前账号未开启邀请功能。') }}
          </p>
        </section>
      </template>

      <template v-else-if="overview">
        <!-- Rate-first marketing banner -->
        <ReferralRateBanner
          :level1-rate="displayLevel1Rate"
          :level1-enabled="displayLevel1Enabled"
          :reward-mode="displayRewardMode"
          :settlement-delay-days="displaySettlementDelayDays"
          :withdraw-enabled="withdrawEnabled"
          :credit-conversion-enabled="creditConversionEnabled"
          :credit-conversion-rate="creditConversionRate"
        />

        <!-- Dual primary cards -->
        <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <ReferralShareCard
            :code="overview.default_code?.code || ''"
            :invite-link="inviteLink"
            :invite-count="overview.direct_invitees || 0"
            :level1-rate="displayLevel1Rate"
            :level1-enabled="displayLevel1Enabled"
            :reward-mode="displayRewardMode"
            @copy="copy"
          />
          <ReferralWalletCard
            :available="overview.available_commission || 0"
            :processing="(overview.pending_commission || 0) + (overview.frozen_commission || 0)"
            :withdrawn="overview.withdrawn_commission || 0"
            :total="overview.total_commission || 0"
            :withdraw-enabled="withdrawEnabled"
            :credit-conversion-enabled="creditConversionEnabled"
            :credit-conversion-rate="creditConversionRate"
            :level1-rate="displayLevel1Rate"
            @open-bucket="openBucketDetail"
            @open-convert="showConvertModal = true"
            @scroll-withdraw="scrollToWithdraw"
          />
        </div>

        <ReferralHowItWorks
          :level1-rate="displayLevel1Rate"
          :level1-enabled="displayLevel1Enabled"
          :reward-mode="displayRewardMode"
          :settlement-delay-days="displaySettlementDelayDays"
          :withdraw-enabled="withdrawEnabled"
          :credit-conversion-enabled="creditConversionEnabled"
          :credit-conversion-rate="creditConversionRate"
        />

        <!-- Withdrawal zone -->
        <div v-if="withdrawEnabled" id="withdrawal-form-section" class="grid grid-cols-1 gap-5 xl:grid-cols-[1fr_1.35fr]">
          <PayoutAccountBinder
            :accounts="payoutAccounts"
            :enabled-methods="withdrawMethods"
            @refresh="loadPayoutAccountsAndOverview"
          />

          <section class="rounded-[28px] border border-black/[0.06] bg-white p-7 dark:border-white/10 dark:bg-[#1c1c1e]">
            <h2 class="text-[17px] font-semibold tracking-tight text-[#1d1d1f] dark:text-white">
              {{ t('referral.withdrawal', '申请提现') }}
            </h2>
            <p class="mt-1 text-[14px] text-[#6e6e73] dark:text-[#a1a1a6]">
              {{ t('referral.withdrawalHint', '发起后锁定金额，审核通过后打款。') }}
            </p>

            <div class="mt-5 rounded-[16px] bg-[#f5f5f7] px-4 py-3.5 dark:bg-[#2c2c2e]">
              <div class="flex items-center justify-between">
                <span class="text-[13px] font-medium text-[#86868b]">
                  {{ t('referral.availableToWithdraw', '可提现额度') }}
                </span>
                <span class="text-[20px] font-semibold tabular-nums tracking-tight text-[#1d1d1f] dark:text-white">
                  ¥{{ formatMoney(maxWithdrawable) }}
                </span>
              </div>
            </div>

            <form class="mt-6 space-y-4" data-test="withdrawal-form" @submit.prevent="handleCreateWithdrawal">
              <div>
                <label class="mb-1 block text-sm text-slate-600 dark:text-slate-400">
                  {{ t('referral.withdrawAmount', '提现金额') }}
                </label>
                <div class="relative">
                  <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3">
                    <span class="text-slate-500 sm:text-sm">￥</span>
                  </div>
                  <input
                    v-model.number="withdrawForm.amount"
                    name="withdraw_amount"
                    type="number"
                    min="0"
                    :max="maxWithdrawable"
                    step="0.01"
                    class="input pl-8"
                    placeholder="0.00"
                  />
                  <div class="absolute inset-y-0 right-1 flex items-center">
                    <button
                      type="button"
                      class="rounded bg-white px-2 py-1 text-xs font-medium text-primary-600 outline-none hover:bg-primary-50 dark:bg-dark-900 dark:text-primary-400 dark:hover:bg-primary-900/20"
                      @click="withdrawForm.amount = maxWithdrawable"
                    >
                      {{ t('referral.withdrawAll', '全部提现') }}
                    </button>
                  </div>
                </div>
              </div>

              <div>
                <label class="mb-1 block text-sm text-slate-600 dark:text-slate-400">
                  {{ t('referral.selectPayoutAccount', '入账账户') }}
                </label>
                <select v-model="withdrawForm.payout_account_id" class="input" :disabled="!payoutAccounts.length">
                  <option v-if="!payoutAccounts.length" :value="0">
                    {{ t('referral.noPayoutAccountHint', '请先在左侧添加收款账户') }}
                  </option>
                  <option v-for="account in payoutAccounts" :key="account.id" :value="account.id">
                    {{ account.account_name }} ({{ account.method }}) -
                    {{ account.account_no_masked || account.bank_name || t('referral.hasQrCode', '收款二维码') }}
                  </option>
                </select>
              </div>

              <div>
                <label class="mb-1 block text-sm text-slate-600 dark:text-slate-400">
                  {{ t('referral.withdrawRemark', '留言备注 (选填)') }}
                </label>
                <textarea v-model="withdrawForm.remark" class="input min-h-24" />
              </div>

              <button
                class="h-12 w-full rounded-full bg-[#0071e3] text-[15px] font-medium text-white transition hover:bg-[#0077ed] disabled:opacity-40"
                :disabled="creatingWithdrawal || !withdrawForm.amount || !withdrawForm.payout_account_id || withdrawForm.amount > maxWithdrawable"
              >
                {{ creatingWithdrawal ? t('common.saving', '处理中...') : t('referral.submitWithdrawal', '确认提现') }}
              </button>
            </form>
          </section>
        </div>

        <section
          v-if="!withdrawEnabled && !creditConversionEnabled"
          class="rounded-2xl border border-dashed border-slate-200 bg-slate-50 p-6 text-sm text-slate-500 dark:border-dark-700 dark:bg-dark-900 dark:text-slate-400"
        >
          {{ t('referral.monetizationDisabledMessage', '推广佣金转余额和提现当前均未开启。') }}
        </section>
        <section
          v-else-if="!withdrawEnabled"
          class="rounded-2xl border border-dashed border-slate-200 bg-slate-50 p-6 text-sm text-slate-500 dark:border-dark-700 dark:bg-dark-900 dark:text-slate-400"
        >
          {{ t('referral.withdrawDisabledMessage', '推广佣金提现当前未开启。') }}
        </section>

        <!-- Records tabs -->
        <section class="rounded-[28px] border border-black/[0.06] bg-white p-6 dark:border-white/10 dark:bg-[#1c1c1e] sm:p-7">
          <div class="mb-5 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h2 class="text-[17px] font-semibold tracking-tight text-[#1d1d1f] dark:text-white">
                {{ t('referral.recordsTitle', '明细') }}
              </h2>
              <p class="mt-0.5 text-[13px] text-[#86868b]">
                {{ t('referral.recordsSubtitle', '邀请、返佣、流水与提现') }}
              </p>
            </div>
            <nav class="flex gap-1 overflow-x-auto rounded-full bg-[#f5f5f7] p-1 dark:bg-[#2c2c2e]">
              <button
                v-for="tab in recordTabs"
                :key="tab.key"
                type="button"
                class="whitespace-nowrap rounded-full px-3.5 py-1.5 text-[13px] font-medium transition"
                :class="
                  activeRecordTab === tab.key
                    ? 'bg-white text-[#1d1d1f] shadow-sm dark:bg-[#3a3a3c] dark:text-white'
                    : 'text-[#6e6e73] hover:text-[#1d1d1f] dark:text-[#a1a1a6] dark:hover:text-white'
                "
                @click="activeRecordTab = tab.key"
              >
                {{ tab.label }}
                <span v-if="tab.count != null" class="ml-0.5 tabular-nums text-[#86868b]">{{ tab.count }}</span>
              </button>
            </nav>
          </div>

          <!-- Invitees -->
          <div v-show="activeRecordTab === 'invitees'" id="invitees-section">
            <!-- Desktop table -->
            <div class="hidden overflow-x-auto rounded-xl border border-slate-100 dark:border-dark-800 md:block">
              <table class="min-w-full text-sm">
                <thead class="bg-slate-50 dark:bg-dark-800/50">
                  <tr class="text-left text-slate-500 dark:text-slate-400">
                    <th class="w-8 px-2 py-3" />
                    <th class="px-4 py-3 font-medium">{{ t('common.username', '用户') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.totalRecharge', '充值总额') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.orderCount', '订单数') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.totalCommissionAmount', '返佣总额') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.latestPaidAt', '最近付款') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.bindTime', '绑定时间') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-slate-100 dark:divide-dark-800">
                  <template v-for="invitee in invitees.items" :key="invitee.user_id">
                    <tr
                      class="cursor-pointer transition-colors hover:bg-slate-50 dark:hover:bg-dark-800/50"
                      @click="toggleInviteeExpand(invitee.user_id)"
                    >
                      <td class="px-2 py-3 text-center text-slate-400">
                        <svg
                          class="inline-block h-4 w-4 transition-transform"
                          :class="{ 'rotate-90': expandedInviteeId === invitee.user_id }"
                          viewBox="0 0 24 24"
                          fill="none"
                          stroke="currentColor"
                          stroke-width="2"
                        >
                          <path d="M9 18l6-6-6-6" />
                        </svg>
                      </td>
                      <td class="px-4 py-3">
                        <div class="font-medium text-slate-900 dark:text-white">{{ invitee.username || '-' }}</div>
                        <div class="text-xs text-slate-500">{{ invitee.email }}</div>
                      </td>
                      <td
                        class="px-4 py-3 font-medium"
                        :class="invitee.total_recharge > 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-slate-400'"
                      >
                        {{ formatMoney(invitee.total_recharge) }}
                      </td>
                      <td class="px-4 py-3">
                        <span class="inline-flex items-center rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-700 dark:bg-dark-800 dark:text-slate-300">
                          {{ invitee.order_count || 0 }}
                        </span>
                      </td>
                      <td class="px-4 py-3 font-medium text-slate-900 dark:text-white">
                        {{ formatMoney(invitee.total_commission) }}
                      </td>
                      <td class="px-4 py-3 text-slate-500">
                        {{ invitee.latest_paid_at ? formatDate(invitee.latest_paid_at) : t('referral.notPaid', '未充值') }}
                      </td>
                      <td class="px-4 py-3 text-slate-500">{{ formatDate(invitee.bound_at) }}</td>
                    </tr>
                    <tr v-if="expandedInviteeId === invitee.user_id">
                      <td colspan="7" class="bg-slate-50/50 px-4 py-3 dark:bg-dark-800/30">
                        <div v-if="inviteeRewardsLoading" class="flex items-center justify-center py-4 text-sm text-slate-500">
                          <LoadingSpinner />
                          <span class="ml-2">{{ t('common.loading', '加载中') }}</span>
                        </div>
                        <div v-else-if="inviteeRewards.length === 0" class="py-4 text-center text-sm text-slate-500">
                          {{ t('referral.noRewards', '暂无返佣明细') }}
                        </div>
                        <table v-else class="min-w-full text-xs">
                          <thead>
                            <tr class="text-left text-slate-500">
                              <th class="px-3 py-2 font-medium">{{ t('referral.rewardDate', '返佣时间') }}</th>
                              <th class="px-3 py-2 font-medium">{{ t('referral.orderNo', '订单号') }}</th>
                              <th class="px-3 py-2 font-medium">{{ t('referral.orderAmount', '订单金额') }}</th>
                              <th class="px-3 py-2 font-medium">{{ t('referral.commissionRate', '返佣比例') }}</th>
                              <th class="px-3 py-2 font-medium">{{ t('referral.commission', '佣金') }}</th>
                              <th class="px-3 py-2 font-medium">{{ t('common.status', '状态') }}</th>
                            </tr>
                          </thead>
                          <tbody class="divide-y divide-slate-100 dark:divide-dark-800">
                            <tr v-for="reward in inviteeRewards" :key="reward.id">
                              <td class="px-3 py-2 text-slate-500">{{ formatDate(reward.created_at) }}</td>
                              <td class="px-3 py-2 font-mono">{{ reward.external_order_id || '-' }}</td>
                              <td class="px-3 py-2">{{ formatMoney(reward.order_paid_amount) }}</td>
                              <td class="px-3 py-2">{{ (reward.rate_snapshot * 100).toFixed(1) }}%</td>
                              <td class="px-3 py-2 font-medium text-emerald-600">+{{ formatMoney(reward.reward_amount) }}</td>
                              <td class="px-3 py-2">
                                <span class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium" :class="statusBadgeClass(reward.status)">
                                  {{ formatStatus(reward.status) }}
                                </span>
                              </td>
                            </tr>
                          </tbody>
                        </table>
                      </td>
                    </tr>
                  </template>
                  <tr v-if="!invitees.items.length">
                    <td colspan="7" class="px-4 py-10 text-center">
                      <EmptyInviteState @copy-link="copy(inviteLink)" />
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <!-- Mobile cards -->
            <div class="space-y-3 md:hidden">
              <button
                v-for="invitee in invitees.items"
                :key="invitee.user_id"
                type="button"
                class="w-full rounded-xl border border-slate-100 bg-slate-50/50 p-4 text-left dark:border-dark-800 dark:bg-dark-800/40"
                @click="toggleInviteeExpand(invitee.user_id)"
              >
                <div class="flex items-start justify-between gap-2">
                  <div>
                    <p class="font-semibold text-slate-900 dark:text-white">{{ invitee.username || invitee.email }}</p>
                    <p class="text-xs text-slate-500">{{ invitee.email }}</p>
                  </div>
                  <span class="text-sm font-bold text-emerald-600">￥{{ formatMoney(invitee.total_commission) }}</span>
                </div>
                <div class="mt-2 flex flex-wrap gap-2 text-[11px] text-slate-500">
                  <span>{{ t('referral.totalRecharge', '充值') }} ￥{{ formatMoney(invitee.total_recharge) }}</span>
                  <span>·</span>
                  <span>{{ invitee.order_count || 0 }} {{ t('referral.orderCount', '单') }}</span>
                </div>
              </button>
              <div v-if="!invitees.items.length" class="py-8">
                <EmptyInviteState @copy-link="copy(inviteLink)" />
              </div>
            </div>
            <PaginationBar
              v-if="invitees.pages > 1"
              :page="invitees.page"
              :pages="invitees.pages"
              :total="invitees.total"
              @prev="loadInviteesPage(invitees.page - 1)"
              @next="loadInviteesPage(invitees.page + 1)"
            />
          </div>

          <!-- Rewards -->
          <div v-show="activeRecordTab === 'rewards'" id="rewards-section">
            <p class="mb-3 text-xs text-slate-500 dark:text-slate-400">{{ t('referral.rewardsDescription') }}</p>
            <div class="hidden overflow-x-auto rounded-xl border border-slate-100 dark:border-dark-800 md:block">
              <table class="min-w-full text-sm">
                <thead class="bg-slate-50 dark:bg-dark-800/50">
                  <tr class="text-left text-slate-500 dark:text-slate-400">
                    <th class="px-4 py-3 font-medium">{{ t('referral.rewardDate') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.sourceUser') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.orderNo') }}</th>
                    <th class="px-4 py-3 font-medium text-right">{{ t('referral.orderAmount') }}</th>
                    <th class="px-4 py-3 font-medium text-right">{{ t('referral.commissionRate') }}</th>
                    <th class="px-4 py-3 font-medium text-right">{{ t('referral.commission') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('common.status') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.availableAt') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-slate-100 dark:divide-dark-800">
                  <tr v-for="reward in rewards.items" :key="reward.id" class="text-slate-700 dark:text-slate-300">
                    <td class="px-4 py-3 text-slate-500">{{ formatDate(reward.created_at) }}</td>
                    <td class="px-4 py-3">
                      <div class="font-medium text-slate-900 dark:text-white">
                        {{ reward.source_user_username || reward.invitee_email || reward.source_user_email || '-' }}
                      </div>
                      <div v-if="reward.source_user_email || reward.invitee_email" class="text-xs text-slate-500">
                        {{ reward.source_user_email || reward.invitee_email }}
                      </div>
                    </td>
                    <td class="px-4 py-3 font-mono text-xs">
                      {{ reward.external_order_id || (reward.recharge_order_id ? '#' + reward.recharge_order_id : '-') }}
                    </td>
                    <td class="px-4 py-3 text-right tabular-nums">{{ formatMoney(reward.order_paid_amount) }}</td>
                    <td class="px-4 py-3 text-right tabular-nums">
                      {{ reward.rate_snapshot != null ? (Number(reward.rate_snapshot) * 100).toFixed(1) + '%' : '-' }}
                    </td>
                    <td class="px-4 py-3 text-right font-medium text-emerald-600 dark:text-emerald-400">
                      +{{ formatMoney(reward.reward_amount) }}
                    </td>
                    <td class="px-4 py-3">
                      <span class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium" :class="statusBadgeClass(reward.status)">
                        {{ formatStatus(reward.status) }}
                      </span>
                    </td>
                    <td class="px-4 py-3 text-slate-500">
                      <template v-if="reward.status === 'pending' && reward.available_at">{{ formatDate(reward.available_at) }}</template>
                      <template v-else>-</template>
                    </td>
                  </tr>
                  <tr v-if="!rewards.items.length">
                    <td colspan="8" class="px-4 py-10 text-center text-slate-500">{{ t('referral.noRewards') }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div class="space-y-3 md:hidden">
              <div
                v-for="reward in rewards.items"
                :key="reward.id"
                class="rounded-xl border border-slate-100 bg-slate-50/50 p-4 dark:border-dark-800 dark:bg-dark-800/40"
              >
                <div class="flex items-start justify-between">
                  <div>
                    <p class="font-medium text-slate-900 dark:text-white">
                      {{ reward.source_user_username || reward.invitee_email || reward.source_user_email || '-' }}
                    </p>
                    <p class="text-xs text-slate-500">{{ formatDate(reward.created_at) }}</p>
                  </div>
                  <span class="font-bold text-emerald-600">+{{ formatMoney(reward.reward_amount) }}</span>
                </div>
                <div class="mt-2 flex flex-wrap gap-2 text-[11px] text-slate-500">
                  <span>{{ reward.external_order_id || '—' }}</span>
                  <span class="rounded-full px-2 py-0.5" :class="statusBadgeClass(reward.status)">{{ formatStatus(reward.status) }}</span>
                </div>
              </div>
              <p v-if="!rewards.items.length" class="py-8 text-center text-sm text-slate-500">{{ t('referral.noRewards') }}</p>
            </div>
            <PaginationBar
              v-if="rewards.pages > 1"
              :page="rewards.page"
              :pages="rewards.pages"
              :total="rewards.total"
              @prev="loadRewardsPage(rewards.page - 1)"
              @next="loadRewardsPage(rewards.page + 1)"
            />
          </div>

          <!-- Ledger -->
          <div v-show="activeRecordTab === 'ledger'" id="ledger-section">
            <div class="hidden overflow-x-auto rounded-xl border border-slate-100 dark:border-dark-800 md:block">
              <table class="min-w-full text-sm">
                <thead class="bg-slate-50 dark:bg-dark-800/50">
                  <tr class="text-left text-slate-500 dark:text-slate-400">
                    <th class="px-4 py-3 font-medium">{{ t('referral.entryType', '类型') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.sourceUser', '来源') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.orderAmount', '订单金额') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.commissionRate', '比例') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.commission', '佣金') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('common.createdAt', '时间') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-slate-100 dark:divide-dark-800">
                  <tr v-for="entry in ledger.items" :key="entry.id">
                    <td class="px-4 py-3">
                      <div class="font-medium text-slate-900 dark:text-white">{{ formatEntryType(entry.entry_type) }}</div>
                      <div class="text-xs text-slate-500">{{ formatStatus(entry.bucket) }}</div>
                      <div v-if="entry.external_order_id" class="mt-0.5 font-mono text-xs text-slate-400">{{ entry.external_order_id }}</div>
                    </td>
                    <td class="px-4 py-3 text-slate-700 dark:text-slate-300">
                      {{ entry.source_user_email || entry.source_user_username || '-' }}
                    </td>
                    <td class="px-4 py-3 text-slate-700 dark:text-slate-300">
                      <template v-if="entry.order_paid_amount && entry.order_paid_amount > 0">
                        {{ formatMoney(entry.order_paid_amount) }}
                      </template>
                      <template v-else>-</template>
                    </td>
                    <td class="px-4 py-3 text-slate-700 dark:text-slate-300">
                      <template v-if="entry.reward_rate_snapshot && entry.reward_rate_snapshot > 0">
                        {{ (entry.reward_rate_snapshot * 100).toFixed(1) }}%
                      </template>
                      <template v-else>-</template>
                    </td>
                    <td
                      class="px-4 py-3 font-medium"
                      :class="entry.amount > 0 ? 'text-emerald-600 dark:text-emerald-400' : entry.amount < 0 ? 'text-red-600' : 'text-slate-900 dark:text-white'"
                    >
                      {{ entry.amount > 0 ? '+' : '' }}{{ formatMoney(entry.amount) }}
                    </td>
                    <td class="px-4 py-3 text-slate-500">{{ formatDate(entry.created_at) }}</td>
                  </tr>
                  <tr v-if="!ledger.items.length">
                    <td colspan="6" class="px-4 py-10 text-center text-slate-500">{{ t('common.noData', '暂无数据') }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div class="space-y-3 md:hidden">
              <div
                v-for="entry in ledger.items"
                :key="entry.id"
                class="rounded-xl border border-slate-100 bg-slate-50/50 p-4 dark:border-dark-800 dark:bg-dark-800/40"
              >
                <div class="flex justify-between gap-2">
                  <div>
                    <p class="font-medium text-slate-900 dark:text-white">{{ formatEntryType(entry.entry_type) }}</p>
                    <p class="text-xs text-slate-500">{{ formatDate(entry.created_at) }}</p>
                  </div>
                  <span
                    class="font-bold"
                    :class="entry.amount > 0 ? 'text-emerald-600' : entry.amount < 0 ? 'text-red-600' : 'text-slate-700'"
                  >
                    {{ entry.amount > 0 ? '+' : '' }}{{ formatMoney(entry.amount) }}
                  </span>
                </div>
              </div>
              <p v-if="!ledger.items.length" class="py-8 text-center text-sm text-slate-500">{{ t('common.noData', '暂无数据') }}</p>
            </div>
            <PaginationBar
              v-if="ledger.pages > 1"
              :page="ledger.page"
              :pages="ledger.pages"
              :total="ledger.total"
              @prev="loadLedgerPage(ledger.page - 1)"
              @next="loadLedgerPage(ledger.page + 1)"
            />
          </div>

          <!-- Withdrawals -->
          <div v-show="activeRecordTab === 'withdrawals'" id="withdrawal-records-section">
            <p class="mb-3 text-xs text-slate-500">
              {{ t('referral.withdrawalRecordsHint', '含现金提现与「转平台余额」；转余额显示为已转余额，不是银行卡打款。') }}
            </p>
            <div class="hidden overflow-x-auto rounded-xl border border-slate-100 dark:border-dark-800 md:block">
              <table class="min-w-full text-sm">
                <thead class="bg-slate-50 dark:bg-dark-800/50">
                  <tr class="text-left text-slate-500 dark:text-slate-400">
                    <th class="px-4 py-3 font-medium">{{ t('referral.withdrawalNo', '单号') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.typeLabel', '类型') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('common.amount', '金额') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('common.status', '状态') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('common.createdAt', '时间') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-slate-100 dark:divide-dark-800">
                  <tr v-for="record in withdrawals.items" :key="record.id" class="text-slate-700 dark:text-slate-300">
                    <td class="px-4 py-4 font-mono text-xs">{{ record.withdrawal_no }}</td>
                    <td class="px-4 py-4">{{ formatPayoutMethod(record.payout_method || record.method) }}</td>
                    <td class="px-4 py-4 font-medium text-slate-900 dark:text-white">￥{{ formatMoney(record.net_amount) }}</td>
                    <td class="px-4 py-4">
                      <span class="inline-flex items-center rounded-full bg-slate-100 px-2.5 py-0.5 text-xs font-medium text-slate-800 dark:bg-dark-800 dark:text-slate-300">
                        {{ formatWithdrawalStatus(record.status, record.payout_method || record.method) }}
                      </span>
                    </td>
                    <td class="px-4 py-4 text-slate-500">{{ formatDate(record.created_at) }}</td>
                  </tr>
                  <tr v-if="!withdrawals.items.length">
                    <td colspan="5" class="px-4 py-10 text-center text-slate-500">{{ t('common.noData', '暂无记录') }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div class="space-y-3 md:hidden">
              <div
                v-for="record in withdrawals.items"
                :key="record.id"
                class="rounded-xl border border-slate-100 bg-slate-50/50 p-4 dark:border-dark-800 dark:bg-dark-800/40"
              >
                <div class="flex justify-between">
                  <span class="font-mono text-xs text-slate-500">{{ record.withdrawal_no }}</span>
                  <span class="font-bold text-slate-900 dark:text-white">￥{{ formatMoney(record.net_amount) }}</span>
                </div>
                <div class="mt-2 flex flex-wrap gap-2 text-xs text-slate-500">
                  <span>{{ formatPayoutMethod(record.payout_method || record.method) }}</span>
                  <span>{{ formatWithdrawalStatus(record.status, record.payout_method || record.method) }}</span>
                </div>
              </div>
              <p v-if="!withdrawals.items.length" class="py-8 text-center text-sm text-slate-500">{{ t('common.noData', '暂无记录') }}</p>
            </div>
            <PaginationBar
              v-if="withdrawals.pages > 1"
              :page="withdrawals.page"
              :pages="withdrawals.pages"
              :total="withdrawals.total"
              @prev="loadWithdrawalsPage(withdrawals.page - 1)"
              @next="loadWithdrawalsPage(withdrawals.page + 1)"
            />
          </div>
        </section>
      </template>
    </div>

    <!-- Convert Modal -->
    <div
      v-if="showConvertModal"
      ref="convertModalBackdrop"
      class="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/50 backdrop-blur-sm"
      tabindex="0"
      @click.self="showConvertModal = false"
      @keydown.esc="showConvertModal = false"
    >
      <div class="w-full max-w-sm rounded-[24px] border border-black/[0.06] bg-white p-7 shadow-2xl dark:border-white/10 dark:bg-[#1c1c1e]">
        <h3 class="text-[20px] font-semibold tracking-tight text-[#1d1d1f] dark:text-white">
          {{ t('referral.convertToCredit', '转入平台余额') }}
        </h3>
        <p class="mt-2 text-[14px] leading-relaxed text-[#6e6e73] dark:text-[#a1a1a6]">
          {{ convertModalRateHint }}
        </p>

        <div class="mt-5 rounded-[14px] bg-[#f5f5f7] px-4 py-3 dark:bg-[#2c2c2e]">
          <p class="text-[12px] font-medium text-[#86868b]">{{ t('referral.convertModalRateTitle') }}</p>
          <p class="mt-0.5 text-[15px] font-semibold text-[#1d1d1f] dark:text-white">
            {{ convertModalRateLine }}
          </p>
        </div>

        <div class="mt-4">
          <label class="mb-1.5 block text-[13px] font-medium text-[#86868b]">
            {{ t('referral.convertAmount') }}
          </label>
          <div class="relative">
            <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3.5">
              <span class="text-[15px] text-[#86868b]">¥</span>
            </div>
            <input
              v-model="convertAmount"
              type="number"
              min="0"
              :max="maxWithdrawable"
              step="0.01"
              class="input w-full rounded-[14px] pl-8"
              placeholder="0.00"
              data-test="convert-amount-input"
            />
            <div class="absolute inset-y-0 right-1.5 flex items-center">
              <button
                type="button"
                class="rounded-full bg-white px-2.5 py-1 text-[12px] font-medium text-[#0071e3] outline-none dark:bg-[#3a3a3c]"
                @click="convertAmount = String(maxWithdrawable)"
              >
                {{ t('referral.convertModalAll') }}
              </button>
            </div>
          </div>
          <p class="mt-2 text-[13px] text-[#6e6e73] dark:text-[#a1a1a6]" data-test="convert-expected-credit">
            {{ t('referral.convertModalExpected') }}
            <span class="font-semibold tabular-nums text-[#1d1d1f] dark:text-white">¥{{ formatMoney(convertCreditAmount) }}</span>
          </p>
        </div>

        <div class="mt-6 flex gap-2.5">
          <button
            class="h-11 flex-1 rounded-full bg-[#f5f5f7] text-[15px] font-medium text-[#1d1d1f] dark:bg-white/10 dark:text-white"
            @click="showConvertModal = false"
          >
            {{ t('common.cancel', '取消') }}
          </button>
          <button
            class="h-11 flex-1 rounded-full bg-[#0071e3] text-[15px] font-medium text-white transition hover:bg-[#0077ed] disabled:opacity-40"
            :disabled="converting || !Number(convertAmount) || Number(convertAmount) <= 0 || Number(convertAmount) > maxWithdrawable"
            @click="handleConvertToCredit"
          >
            {{ converting ? t('common.processing', '处理中...') : t('common.confirm', '确认转入') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Bucket Detail Modal -->
    <div
      v-if="bucketDetailVisible"
      class="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/50 backdrop-blur-sm"
      tabindex="0"
      @click.self="bucketDetailVisible = false"
      @keydown.esc="bucketDetailVisible = false"
    >
      <div class="flex max-h-[80vh] w-full max-w-2xl flex-col rounded-2xl border border-slate-200 bg-white shadow-xl dark:border-dark-700 dark:bg-dark-900">
        <div class="flex items-center justify-between border-b border-slate-100 px-6 py-4 dark:border-dark-800">
          <h3 class="text-lg font-semibold text-slate-900 dark:text-white">{{ bucketDetailTitle }}</h3>
          <button class="rounded-full p-1.5 text-slate-400 hover:bg-slate-100 dark:hover:bg-dark-800" @click="bucketDetailVisible = false">
            <svg viewBox="0 0 20 20" fill="currentColor" class="h-5 w-5">
              <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
            </svg>
          </button>
        </div>
        <div class="flex-1 overflow-y-auto px-6 py-4">
          <div v-if="bucketDetailLoading" class="flex items-center justify-center py-8 text-sm text-slate-500">
            <LoadingSpinner /><span class="ml-2">{{ t('common.loading', '加载中') }}</span>
          </div>
          <div v-else-if="!bucketDetailItems.length" class="py-12 text-center text-sm text-slate-500">
            {{ t('common.noData', '暂无数据') }}
          </div>
          <div v-else class="space-y-3">
            <div v-for="entry in bucketDetailItems" :key="entry.id" class="rounded-2xl border border-slate-100 p-4 dark:border-dark-800">
              <div class="flex items-start justify-between">
                <div>
                  <div class="font-medium text-slate-900 dark:text-white">{{ formatEntryType(entry.entry_type) }}</div>
                  <div class="mt-0.5 text-xs text-slate-500">{{ formatStatus(entry.bucket) }}</div>
                  <div v-if="entry.external_order_id" class="mt-0.5 font-mono text-xs text-slate-400">{{ entry.external_order_id }}</div>
                </div>
                <div class="text-right">
                  <div
                    class="font-semibold"
                    :class="entry.amount > 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'"
                  >
                    {{ entry.amount > 0 ? '+' : '' }}￥{{ formatMoney(entry.amount) }}
                  </div>
                  <div class="mt-0.5 text-xs text-slate-500">{{ formatDate(entry.created_at) }}</div>
                </div>
              </div>
            </div>
          </div>
        </div>
        <div v-if="bucketDetailItems.length" class="border-t border-slate-100 px-6 py-3 text-center text-xs text-slate-500 dark:border-dark-800">
          {{ t('common.totalPrefix', '共') }} {{ bucketDetailItems.length }} {{ t('common.totalSuffix', '条') }}
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import PayoutAccountBinder from './referral-components/PayoutAccountBinder.vue'
import ReferralShareCard from './referral-components/ReferralShareCard.vue'
import ReferralWalletCard from './referral-components/ReferralWalletCard.vue'
import ReferralHowItWorks from './referral-components/ReferralHowItWorks.vue'
import ReferralRateBanner from './referral-components/ReferralRateBanner.vue'
import { useAppStore } from '@/stores'
import referralAPI from '@/api/referral'
import type {
  BasePaginationResponse,
  CommissionLedgerEntry,
  CommissionPayoutAccount,
  CommissionWithdrawal,
  CreateReferralWithdrawalRequest,
  ReferralCenterOverview,
  ReferralInvitee,
  UserInviteeReward
} from '@/types'
import { formatPayoutMethod, formatWithdrawalStatus } from '@/utils/referralWithdrawalDisplay'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(true)
const creatingWithdrawal = ref(false)
const activeRecordTab = ref<'invitees' | 'rewards' | 'ledger' | 'withdrawals'>('invitees')

const overview = ref<ReferralCenterOverview | null>(null)
const invitees = ref<BasePaginationResponse<ReferralInvitee>>({ items: [], total: 0, page: 1, page_size: 10, pages: 1 })
const rewards = ref<BasePaginationResponse<UserInviteeReward>>({ items: [], total: 0, page: 1, page_size: 15, pages: 1 })
const ledger = ref<BasePaginationResponse<CommissionLedgerEntry>>({ items: [], total: 0, page: 1, page_size: 15, pages: 1 })
const withdrawals = ref<BasePaginationResponse<CommissionWithdrawal>>({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
const payoutAccounts = ref<CommissionPayoutAccount[]>([])

const expandedInviteeId = ref<number | null>(null)
const inviteeRewards = ref<UserInviteeReward[]>([])
const inviteeRewardsLoading = ref(false)

const showConvertModal = ref(false)
const converting = ref(false)
const convertAmount = ref('')
const convertModalBackdrop = ref<HTMLDivElement | null>(null)

const bucketDetailVisible = ref(false)
const bucketDetailTitle = ref('')
const bucketDetailLoading = ref(false)
const bucketDetailItems = ref<CommissionLedgerEntry[]>([])

const withdrawForm = reactive({
  amount: 0 as number,
  payout_account_id: 0,
  remark: ''
})

const withdrawMethods = computed(() =>
  overview.value?.withdraw_methods_enabled?.length ? overview.value.withdraw_methods_enabled : ['alipay', 'wechat', 'bank']
)
const maxWithdrawable = computed(() => Number(overview.value?.available_commission || 0))
const withdrawEnabled = computed(() => Boolean(overview.value?.referral_withdraw_enabled))
const creditConversionEnabled = computed(() => Boolean(overview.value?.referral_credit_conversion_enabled))
/**
 * 转余额倍数来源（唯一）：
 *   DB settings 键 referral_credit_conversion_rate
 *   → SettingService → GET /user/referral/overview
 *     字段 overview.referral_credit_conversion_rate
 *   实际入账：credit = commission * rate（见 referral_withdrawal_service ConvertToCredit）
 * 管理端可在「系统设置 → 邀请返佣 → 转余额倍数」修改。
 */
const creditConversionRate = computed(() => {
  const rate = Number(overview.value?.referral_credit_conversion_rate || 1)
  return rate > 0 ? rate : 1
})
/** Match backend roundMoney (8 decimal places). */
function roundMoney8(value: number): number {
  return Math.round(value * 1e8) / 1e8
}

/** Format rate for display: up to 8 decimals, trim trailing zeros. */
function formatRateDisplay(rate: number): string {
  if (rate % 1 === 0) return String(rate)
  return rate
    .toFixed(8)
    .replace(/\.?0+$/, '')
}

const convertCreditAmount = computed(() =>
  roundMoney8(Number(convertAmount.value || 0) * creditConversionRate.value)
)

const convertModalRateLine = computed(() => {
  const m = creditConversionRate.value
  const mText = formatRateDisplay(m)
  if (m === 1) return t('referral.creditConversionRateOneToOne')
  return t('referral.creditConversionRateMulti', { rate: mText })
})

const convertModalRateHint = computed(() => {
  return `${t('referral.convertDesc')} ${convertModalRateLine.value} ${t('referral.convertPrecisionNote')}`
})

const inviteLink = computed(() => {
  if (!overview.value?.default_code?.code) return ''
  return `${window.location.origin}/register?ref=${overview.value.default_code.code}`
})

const displayLevel1Enabled = computed(() => {
  if (overview.value?.level1_enabled === false) return false
  if (overview.value?.level1_enabled === true) return true
  if (appStore.cachedPublicSettings?.referral_level1_enabled === false) return false
  // Default true when unset (matches backend).
  return true
})

/**
 * Prefer overview; fall back to public settings.
 * Rate is 0 when level-1 is disabled (backend also zeros it).
 */
const displayLevel1Rate = computed(() => {
  if (!displayLevel1Enabled.value) return 0
  const fromOverview = Number(overview.value?.level1_rate || 0)
  if (fromOverview > 0) return fromOverview
  const fromPublic = Number(appStore.cachedPublicSettings?.referral_level1_rate || 0)
  return fromPublic > 0 ? fromPublic : 0
})

const displayRewardMode = computed(() => {
  const fromOverview = (overview.value?.reward_mode || '').trim()
  if (fromOverview) return fromOverview
  const fromPublic = (appStore.cachedPublicSettings?.referral_reward_mode || '').trim()
  return fromPublic || 'first_paid_order'
})

const displaySettlementDelayDays = computed(() => {
  const fromOverview = overview.value?.settlement_delay_days
  if (fromOverview != null && Number(fromOverview) >= 0) return Number(fromOverview)
  const fromPublic = Number(appStore.cachedPublicSettings?.referral_settlement_delay_days)
  return Number.isFinite(fromPublic) && fromPublic >= 0 ? fromPublic : 0
})

const recordTabs = computed(() => [
  { key: 'invitees' as const, label: t('referral.tabs.invitees', '邀请'), count: invitees.value.total },
  { key: 'rewards' as const, label: t('referral.tabs.rewards', '返佣'), count: rewards.value.total },
  { key: 'ledger' as const, label: t('referral.tabs.ledger', '流水'), count: ledger.value.total },
  { key: 'withdrawals' as const, label: t('referral.tabs.withdrawals', '提现'), count: withdrawals.value.total }
])

const EmptyInviteState = defineComponent({
  name: 'EmptyInviteState',
  emits: ['copyLink'],
  setup(_, { emit }) {
    return () =>
      h('div', { class: 'mx-auto max-w-sm space-y-3 py-2 text-center' }, [
        h('p', { class: 'text-sm font-semibold text-slate-800 dark:text-slate-100' }, t('referral.empty.inviteTitle', '还没有邀请记录')),
        h('p', { class: 'text-xs text-slate-500' }, t('referral.empty.inviteHint', '复制上方邀请链接发给好友，完成注册充值后这里会出现明细。')),
        h(
          'button',
          {
            type: 'button',
            class:
              'inline-flex items-center justify-center rounded-xl bg-gradient-to-r from-primary-500 to-cyan-500 px-4 py-2 text-xs font-bold text-slate-950 shadow-md',
            onClick: () => emit('copyLink')
          },
          t('referral.empty.inviteCta', '去复制邀请链接')
        )
      ])
  }
})

const PaginationBar = defineComponent({
  name: 'PaginationBar',
  props: {
    page: { type: Number, required: true },
    pages: { type: Number, required: true },
    total: { type: Number, required: true }
  },
  emits: ['prev', 'next'],
  setup(props, { emit }) {
    return () =>
      h('div', { class: 'mt-4 flex items-center justify-between text-sm' }, [
        h(
          'span',
          { class: 'text-slate-500 dark:text-slate-400' },
          `${t('common.totalPrefix', '共')} ${props.total} ${t('common.totalSuffix', '条')}`
        ),
        h('div', { class: 'flex gap-1' }, [
          h(
            'button',
            {
              class: 'btn btn-secondary btn-sm',
              disabled: props.page <= 1,
              onClick: () => emit('prev')
            },
            t('common.prevPage', '上一页')
          ),
          h('span', { class: 'px-3 py-1.5 text-slate-700 dark:text-slate-300' }, `${props.page}/${props.pages}`),
          h(
            'button',
            {
              class: 'btn btn-secondary btn-sm',
              disabled: props.page >= props.pages,
              onClick: () => emit('next')
            },
            t('common.nextPage', '下一页')
          )
        ])
      ])
  }
})

async function openBucketDetail(bucket: string, title: string) {
  bucketDetailTitle.value = title
  bucketDetailVisible.value = true
  bucketDetailLoading.value = true
  bucketDetailItems.value = []
  try {
    const bucketMap: Record<string, string[]> = {
      available: ['available'],
      processing: ['pending', 'frozen'],
      settled: ['settled'],
      // All buckets that sum into total_commission on overview.
      all: ['pending', 'available', 'frozen', 'settled']
    }
    const matchBuckets = bucketMap[bucket] || [bucket]
    // Page through ledger until we cover all matching rows (cap pages to avoid runaway).
    const pageSize = 100
    const maxPages = 20
    const collected: CommissionLedgerEntry[] = []
    let page = 1
    let totalPages = 1
    while (page <= totalPages && page <= maxPages) {
      const data = await referralAPI.getLedger(page, pageSize)
      totalPages = Math.max(1, Number(data.pages || 1))
      for (const entry of data.items) {
        if (matchBuckets.includes(entry.bucket)) {
          collected.push(entry)
        }
      }
      // Early exit when a full page has no matches and we're past known data — still
      // continue if pages remain because buckets are not sorted server-side.
      page += 1
    }
    bucketDetailItems.value = collected
    if (totalPages > maxPages) {
      appStore.showInfo(
        t('referral.bucketDetailTruncated', {
          max: maxPages * pageSize
        })
      )
    }
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '加载失败'))
  } finally {
    bucketDetailLoading.value = false
  }
}

function scrollToWithdraw() {
  document.getElementById('withdrawal-form-section')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

watch(showConvertModal, (val) => {
  if (val) {
    nextTick(() => convertModalBackdrop.value?.focus())
  }
})

async function loadPayoutAccountsAndOverview() {
  try {
    const overviewData = await referralAPI.getOverview()
    overview.value = overviewData

    if (!overviewData.referral_enabled) {
      payoutAccounts.value = []
      withdrawForm.payout_account_id = 0
      return
    }

    const payoutAccountsData = await referralAPI.getPayoutAccounts()
    payoutAccounts.value = payoutAccountsData

    if (
      payoutAccounts.value.length > 0 &&
      (!withdrawForm.payout_account_id || !payoutAccounts.value.find((a) => a.id === withdrawForm.payout_account_id))
    ) {
      withdrawForm.payout_account_id =
        payoutAccounts.value.find((item) => item.is_default)?.id || payoutAccounts.value[0].id
    }
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '操作失败'))
  }
}

async function loadAll() {
  loading.value = true
  try {
    await loadPayoutAccountsAndOverview()

    if (!overview.value?.referral_enabled) {
      invitees.value = { items: [], total: 0, page: 1, page_size: 10, pages: 1 }
      rewards.value = { items: [], total: 0, page: 1, page_size: 15, pages: 1 }
      ledger.value = { items: [], total: 0, page: 1, page_size: 15, pages: 1 }
      withdrawals.value = { items: [], total: 0, page: 1, page_size: 20, pages: 1 }
      expandedInviteeId.value = null
      inviteeRewards.value = []
      return
    }

    const results = await Promise.allSettled([
      referralAPI.getInvitees(1, 10),
      referralAPI.getRewards(1, 15),
      referralAPI.getLedger(1, 15),
      referralAPI.getWithdrawals()
    ])
    const labels = [
      t('referral.invitees'),
      t('referral.rewardsTitle'),
      t('referral.ledger'),
      t('referral.withdrawalRecords')
    ]
    const failures: string[] = []
    if (results[0].status === 'fulfilled') invitees.value = results[0].value
    else failures.push(labels[0])
    if (results[1].status === 'fulfilled') rewards.value = results[1].value
    else failures.push(labels[1])
    if (results[2].status === 'fulfilled') ledger.value = results[2].value
    else failures.push(labels[2])
    if (results[3].status === 'fulfilled') withdrawals.value = results[3].value
    else failures.push(labels[3])
    if (failures.length) {
      appStore.showError(t('referral.partialLoadFailed', { sections: failures.join(', ') }))
    }
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed'))
  } finally {
    loading.value = false
  }
}

async function loadInviteesPage(page: number) {
  try {
    expandedInviteeId.value = null
    inviteeRewards.value = []
    invitees.value = await referralAPI.getInvitees(page, 10)
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '加载失败'))
  }
}

async function loadLedgerPage(page: number) {
  try {
    ledger.value = await referralAPI.getLedger(page, 15)
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '加载失败'))
  }
}

async function loadRewardsPage(page: number) {
  try {
    rewards.value = await referralAPI.getRewards(page, 15)
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '加载失败'))
  }
}

async function loadWithdrawalsPage(page: number) {
  try {
    withdrawals.value = await referralAPI.getWithdrawals(page)
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '加载失败'))
  }
}

async function toggleInviteeExpand(userId: number) {
  if (expandedInviteeId.value === userId) {
    expandedInviteeId.value = null
    inviteeRewards.value = []
    return
  }
  expandedInviteeId.value = userId
  inviteeRewardsLoading.value = true
  inviteeRewards.value = []
  try {
    inviteeRewards.value = await referralAPI.getInviteeRewards(userId)
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '加载失败'))
  } finally {
    inviteeRewardsLoading.value = false
  }
}

async function handleConvertToCredit() {
  const amountToConvert = Number(convertAmount.value)
  if (amountToConvert <= 0 || amountToConvert > maxWithdrawable.value) return

  converting.value = true
  try {
    await referralAPI.convertToCredit(amountToConvert)
    showConvertModal.value = false
    convertAmount.value = ''
    appStore.showSuccess(t('referral.convertSuccess', '已成功转入平台余额'))
    await loadAll()
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '操作失败'))
  } finally {
    converting.value = false
  }
}

async function handleCreateWithdrawal() {
  const amount = withdrawForm.amount
  if (amount <= 0 || amount > maxWithdrawable.value) {
    appStore.showError(t('referral.withdrawInvalid', '提现金额无效'))
    return
  }
  creatingWithdrawal.value = true
  try {
    const account = payoutAccounts.value.find((item) => item.id === withdrawForm.payout_account_id)
    const payload: CreateReferralWithdrawalRequest = {
      amount,
      payout_method: account?.method || withdrawMethods.value[0],
      payout_account_id: withdrawForm.payout_account_id,
      remark: withdrawForm.remark
    }
    await referralAPI.createWithdrawal(payload)
    withdrawForm.amount = 0
    withdrawForm.remark = ''
    appStore.showSuccess(t('referral.withdrawalCreated', '提现申请已提交'))
    await loadAll()
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '操作失败'))
  } finally {
    creatingWithdrawal.value = false
  }
}

async function copy(text: string) {
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    appStore.showSuccess(t('common.copySuccess', '复制成功'))
  } catch {
    appStore.showError(t('common.copyFailed', '复制失败，请手动复制'))
  }
}

function formatMoney(value: number) {
  return `${Number(value || 0).toFixed(2)}`
}

function formatEntryType(entryType: string): string {
  const map: Record<string, string> = {
    reward_pending_credit: t('referral.rewardPendingCredit', '返佣入账'),
    reward_pending_to_available: t('referral.rewardSettled', '佣金结算'),
    withdraw_freeze: t('referral.withdrawFreeze', '提现冻结'),
    withdraw_return: t('referral.withdrawReturn', '提现退回'),
    withdraw_paid: t('referral.withdrawPaid', '提现/转余额完成'),
    admin_add: t('referral.adminAdd', '管理员调增'),
    admin_subtract: t('referral.adminSubtract', '管理员扣减')
  }
  return map[entryType] || entryType
}

function formatStatus(status: string): string {
  if (!status) return '-'
  const key = `referral.status.${status}`
  const translated = t(key)
  return translated === key ? status : translated
}

function statusBadgeClass(status: string): string {
  if (status === 'available' || status === 'paid' || status === 'settled') {
    return 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/20 dark:text-emerald-400'
  }
  if (status === 'pending') {
    return 'bg-amber-100 text-amber-800 dark:bg-amber-900/20 dark:text-amber-400'
  }
  return 'bg-slate-100 text-slate-800 dark:bg-dark-800 dark:text-slate-300'
}

function formatDate(value?: string | Date | null) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

onMounted(loadAll)
</script>
