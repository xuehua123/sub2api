<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-2 md:flex-row md:items-end md:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">
            {{ t('referral.title', '邀请中心') }}
          </h1>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('referral.description', '查看推广码、邀请关系、返佣流水和提现记录。') }}
          </p>
        </div>
        <button class="btn btn-secondary" @click="loadAll" :disabled="loading">
          {{ t('common.refresh', '刷新') }}
        </button>
      </div>

      <div v-if="loading" class="flex items-center justify-center py-12 text-sm text-gray-500">
        <LoadingSpinner />
        <span class="ml-2">{{ t('common.loading', '加载中') }}</span>
      </div>

      <template v-else-if="overview">
        <!-- Dashboard Stats -->
        <div class="rounded-3xl border border-gray-200 bg-gradient-to-br from-primary-50 to-white p-5 shadow-sm dark:from-primary-900/10 dark:to-dark-900">
          <div class="flex flex-wrap items-start justify-between gap-4">
            <div>
              <div class="text-sm font-medium text-gray-500 dark:text-gray-400">{{ t('referral.totalCommission', '累计已结算佣金') }}</div>
              <div class="mt-2 text-3xl font-bold tracking-tight text-gray-900 dark:text-white">
                <span class="text-xl font-normal text-gray-500">￥</span>{{ formatMoney(overview.total_commission) }}
              </div>
            </div>
            <div class="inline-flex items-center gap-1.5 rounded-full bg-white/80 px-3 py-1.5 text-sm font-medium text-gray-700 dark:bg-dark-800/80 dark:text-gray-300">
              <svg viewBox="0 0 20 20" fill="currentColor" class="h-4 w-4 text-primary-500"><path d="M10 9a3 3 0 100-6 3 3 0 000 6zM6 8a2 2 0 11-4 0 2 2 0 014 0zM1.49 15.326a.78.78 0 01-.358-.442 3 3 0 014.308-3.516 6.484 6.484 0 00-1.905 3.959c-.023.222-.014.442.025.654a4.97 4.97 0 01-2.07-.655zM16.44 15.98a4.97 4.97 0 002.07-.654.78.78 0 00.357-.442 3 3 0 00-4.308-3.517 6.484 6.484 0 011.907 3.96 2.32 2.32 0 01-.026.654zM18 8a2 2 0 11-4 0 2 2 0 014 0zM5.304 16.19a.844.844 0 01-.277-.71 5 5 0 019.947 0 .843.843 0 01-.277.71A6.975 6.975 0 0110 18a6.974 6.974 0 01-4.696-1.81z"/></svg>
              {{ t('referral.directInvitees', '邀请用户') }}: {{ overview.direct_invitees || 0 }} {{ t('referral.peopleCount', '人') }}
            </div>
          </div>
          <div class="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-3">
            <button
              type="button"
              class="group flex flex-col items-start rounded-2xl border border-gray-100 bg-white/60 p-3 text-left transition hover:border-primary-300 hover:shadow-sm dark:border-dark-700 dark:bg-dark-800/60 dark:hover:border-primary-600"
              @click="openBucketDetail('available', t('referral.availableCommission', '可提现佣金明细'))"
            >
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('referral.availableCommission', '可提现') }}</span>
              <span class="mt-1 text-lg font-semibold text-primary-600 dark:text-primary-400">￥{{ formatMoney(overview.available_commission) }}</span>
              <span class="mt-1 text-[10px] text-gray-400 group-hover:text-primary-500">{{ t('referral.clickToViewDetail', '点击查看明细 →') }}</span>
            </button>
            <button
              type="button"
              class="group flex flex-col items-start rounded-2xl border border-gray-100 bg-white/60 p-3 text-left transition hover:border-primary-300 hover:shadow-sm dark:border-dark-700 dark:bg-dark-800/60 dark:hover:border-primary-600"
              @click="openBucketDetail('processing', t('referral.processingCommission', '处理中佣金明细'))"
            >
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('referral.processingCommission', '处理中') }}</span>
              <span class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">￥{{ formatMoney((overview.pending_commission || 0) + (overview.frozen_commission || 0)) }}</span>
              <span class="mt-1 text-[10px] text-gray-400 group-hover:text-primary-500">{{ t('referral.clickToViewDetail', '点击查看明细 →') }}</span>
            </button>
            <button
              type="button"
              class="group flex flex-col items-start rounded-2xl border border-gray-100 bg-white/60 p-3 text-left transition hover:border-primary-300 hover:shadow-sm dark:border-dark-700 dark:bg-dark-800/60 dark:hover:border-primary-600"
              @click="openBucketDetail('settled', t('referral.withdrawnCommission', '已提现佣金明细'))"
            >
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('referral.withdrawnCommission', '已提现') }}</span>
              <span class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">￥{{ formatMoney(overview.withdrawn_commission) }}</span>
              <span class="mt-1 text-[10px] text-gray-400 group-hover:text-primary-500">{{ t('referral.clickToViewDetail', '点击查看明细 →') }}</span>
            </button>
          </div>
          <!-- Convert to Credit Action -->
          <div v-if="withdrawEnabled" class="mt-4 border-t border-gray-200 pt-4 dark:border-dark-800">
            <button
              class="btn btn-primary btn-sm flex items-center gap-1 bg-gradient-to-r from-primary-500 to-indigo-500 hover:from-primary-600 hover:to-indigo-600 border-none px-4"
              @click="showConvertModal = true"
              :disabled="!maxWithdrawable"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" class="h-4 w-4" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 3v18"/><path d="M3 17l4 4 4-4"/><path d="M13 7l4-4 4 4"/><path d="M7 21V3"/></svg>
              {{ t('referral.convertToCredit', '将佣金转储为平台余额') }}
            </button>
          </div>
        </div>

        <!-- 邀请码及关系 -->
        <section class="rounded-3xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <div class="flex items-start justify-between gap-4">
            <div>
              <h2 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('referral.myInviteCodeTitle', '我的推广入口') }}</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('referral.myInviteCodeDesc', '将您的专属链接或固定推广码发给朋友。') }}</p>
            </div>
          </div>

          <div class="mt-6 grid grid-cols-1 gap-4 lg:grid-cols-2">
            <div class="rounded-2xl border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800">
              <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('referral.defaultCode', '固定推广码') }}</div>
              <div class="mt-2 flex items-center justify-between">
                <div class="text-xl font-semibold tracking-wider text-gray-900 dark:text-white">
                  {{ overview.default_code?.code || '-' }}
                </div>
                <button class="btn btn-secondary btn-sm" @click="copy(overview.default_code?.code || '')" :disabled="!overview.default_code?.code">
                  {{ t('common.copy', '复制') }}
                </button>
              </div>
            </div>

            <div class="rounded-2xl border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800">
              <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('referral.inviteLink', '邀请注册链接') }}</div>
              <div class="mt-2 flex items-center gap-3">
                <input
                  class="block w-full rounded-xl border border-transparent bg-transparent py-1.5 text-sm outline-none focus:border-transparent dark:text-gray-300"
                  :value="inviteLink"
                  readonly
                />
                <button class="btn btn-primary btn-sm shrink-0" @click="copy(inviteLink)" :disabled="!inviteLink">
                  {{ t('referral.copyInviteLink', '复制链接') }}
                </button>
              </div>
            </div>
          </div>
        </section>

        <div v-if="withdrawEnabled" id="withdrawal-form-section" class="grid grid-cols-1 gap-6 xl:grid-cols-[1fr_1.5fr]">
          <!-- 收款账户组件 -->
          <PayoutAccountBinder
            :accounts="payoutAccounts"
            :enabled-methods="withdrawMethods"
            @refresh="loadPayoutAccountsAndOverview"
          />

          <!-- 提现申请 -->
          <section class="rounded-3xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('referral.withdrawal', '申请提现') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('referral.withdrawalHint', '可提现佣金将进行打款处理，发起后锁定金额，后台审核后即时打款。') }}</p>
            
            <div class="mt-5 rounded-2xl bg-gray-50 p-4 dark:bg-dark-800">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-500 dark:text-gray-400">{{ t('referral.availableToWithdraw', '当前最大可提现额度') }}</span>
                <span class="text-xl font-bold tracking-tight text-primary-600 dark:text-primary-400">￥{{ formatMoney(maxWithdrawable) }}</span>
              </div>
            </div>

            <form class="mt-6 space-y-4" data-test="withdrawal-form" @submit.prevent="handleCreateWithdrawal">
              <div>
                <label class="mb-1 block text-sm text-gray-600 dark:text-gray-400">{{ t('referral.withdrawAmount', '提现金额') }}</label>
                <div class="relative">
                  <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3">
                    <span class="text-gray-500 sm:text-sm">￥</span>
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
                    <button type="button" class="rounded bg-white px-2 py-1 text-xs font-medium text-primary-600 outline-none hover:bg-primary-50 dark:bg-dark-900 dark:text-primary-400 dark:hover:bg-primary-900/20" @click="withdrawForm.amount = maxWithdrawable">
                      {{ t('referral.withdrawAll', '全部提现') }}
                    </button>
                  </div>
                </div>
              </div>
              
              <div>
                <label class="mb-1 block text-sm text-gray-600 dark:text-gray-400">{{ t('referral.selectPayoutAccount', '入账账户') }}</label>
                <select v-model="withdrawForm.payout_account_id" class="input" :disabled="!payoutAccounts.length">
                  <option v-if="!payoutAccounts.length" :value="0">{{ t('referral.noPayoutAccountHint', '请先在左侧添加收款账户') }}</option>
                  <option v-for="account in payoutAccounts" :key="account.id" :value="account.id">
                    {{ account.account_name }} ({{ account.method }}) - {{ account.account_no_masked || account.bank_name || t('referral.hasQrCode', '收款二维码') }}
                  </option>
                </select>
              </div>

              <div>
                <label class="mb-1 block text-sm text-gray-600 dark:text-gray-400">{{ t('referral.withdrawRemark', '留言备注 (选填)') }}</label>
                <textarea v-model="withdrawForm.remark" class="input min-h-24"></textarea>
              </div>
              
              <button class="btn btn-primary w-full py-2.5" :disabled="creatingWithdrawal || !withdrawForm.amount || !withdrawForm.payout_account_id || withdrawForm.amount > maxWithdrawable">
                {{ creatingWithdrawal ? t('common.saving', '处理中...') : t('referral.submitWithdrawal', '确认发起提现') }}
              </button>
            </form>
          </section>
        </div>

        <section
          v-else
          class="rounded-3xl border border-dashed border-gray-200 bg-gray-50 p-6 text-sm text-gray-500 shadow-sm dark:border-dark-700 dark:bg-dark-900 dark:text-gray-400"
        >
          {{ t('referral.withdrawDisabledMessage', '推广佣金提现当前未开启，因此暂不展示转余额和提现入口。') }}
        </section>

        <section id="withdrawal-records-section" class="rounded-3xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <div class="mb-4">
            <h2 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('referral.withdrawalRecords', '提现记录') }}</h2>
          </div>
          <div class="overflow-x-auto rounded-2xl border border-gray-100 dark:border-dark-800">
            <table class="min-w-full text-sm">
              <thead class="bg-gray-50 dark:bg-dark-800/50">
                <tr class="text-left text-gray-500 dark:text-gray-400">
                  <th class="px-4 py-3 font-medium">{{ t('referral.withdrawalNo', '提现单号') }}</th>
                  <th class="px-4 py-3 font-medium">{{ t('common.amount', '金额') }}</th>
                  <th class="px-4 py-3 font-medium">{{ t('common.status', '状态') }}</th>
                  <th class="px-4 py-3 font-medium">{{ t('common.createdAt', '申请时间') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                <tr v-for="record in withdrawals.items" :key="record.id" class="text-gray-700 dark:text-gray-300">
                  <td class="px-4 py-4 font-mono text-xs">{{ record.withdrawal_no }}</td>
                  <td class="px-4 py-4 font-medium text-gray-900 dark:text-white">￥{{ formatMoney(record.net_amount) }}</td>
                  <td class="px-4 py-4">
                    <span class="inline-flex items-center rounded-full bg-gray-100 px-2.5 py-0.5 text-xs font-medium text-gray-800 dark:bg-dark-800 dark:text-gray-300">
                      {{ formatStatus(record.status) }}
                    </span>
                  </td>
                  <td class="px-4 py-4 text-gray-500">{{ formatDate(record.created_at) }}</td>
                </tr>
                <tr v-if="!withdrawals.items.length">
                  <td colspan="4" class="px-4 py-8 text-center text-gray-500">
                    {{ t('common.noData', '暂无提现记录') }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-if="withdrawals.pages > 1" class="mt-4 flex items-center justify-between text-sm">
            <span class="text-gray-500 dark:text-gray-400">{{ t('common.totalPrefix', '共') }} {{ withdrawals.total }} {{ t('common.totalSuffix', '条') }}</span>
            <div class="flex gap-1">
              <button class="btn btn-secondary btn-sm" :disabled="withdrawals.page <= 1" @click="loadWithdrawalsPage(withdrawals.page - 1)">{{ t('common.prevPage', '上一页') }}</button>
              <span class="px-3 py-1.5 text-gray-700 dark:text-gray-300">{{ withdrawals.page }}/{{ withdrawals.pages }}</span>
              <button class="btn btn-secondary btn-sm" :disabled="withdrawals.page >= withdrawals.pages" @click="loadWithdrawalsPage(withdrawals.page + 1)">{{ t('common.nextPage', '下一页') }}</button>
            </div>
          </div>
        </section>

        <!-- Logs section -->
        <div class="grid grid-cols-1 gap-6 xl:grid-cols-2">
          <!-- 邀请记录 -->
          <section class="rounded-3xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <div class="mb-4">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('referral.invitees', '我邀请的用户列表') }}</h2>
            </div>
            <div class="overflow-x-auto rounded-2xl border border-gray-100 dark:border-dark-800">
              <table class="min-w-full text-sm">
                <thead class="bg-gray-50 dark:bg-dark-800/50">
                  <tr class="text-left text-gray-500 dark:text-gray-400">
                    <th class="w-8 px-2 py-3"></th>
                    <th class="px-4 py-3 font-medium">{{ t('common.username', '用户') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.totalRecharge', '充值总额') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.orderCount', '订单数') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.totalCommissionAmount', '返佣总额') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.latestPaidAt', '最近付款') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.bindTime', '绑定时间') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                  <template v-for="invitee in invitees.items" :key="invitee.user_id">
                    <tr class="cursor-pointer transition-colors hover:bg-gray-50 dark:hover:bg-dark-800/50" @click="toggleInviteeExpand(invitee.user_id)">
                      <td class="px-2 py-3 text-center text-gray-400">
                        <svg class="inline-block h-4 w-4 transition-transform" :class="{ 'rotate-90': expandedInviteeId === invitee.user_id }" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6"/></svg>
                      </td>
                      <td class="px-4 py-3">
                        <div class="font-medium text-gray-900 dark:text-white">{{ invitee.username || '-' }}</div>
                        <div class="text-xs text-gray-500">{{ invitee.email }}</div>
                      </td>
                      <td class="px-4 py-3 font-medium" :class="invitee.total_recharge > 0 ? 'text-green-600 dark:text-green-400' : 'text-gray-400 dark:text-gray-500'">
                        {{ formatMoney(invitee.total_recharge) }}
                      </td>
                      <td class="px-4 py-3">
                        <span class="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-700 dark:bg-dark-800 dark:text-gray-300">
                          {{ invitee.order_count || 0 }}
                        </span>
                      </td>
                      <td class="px-4 py-3 font-medium text-gray-900 dark:text-white">
                        {{ formatMoney(invitee.total_commission) }}
                      </td>
                      <td class="px-4 py-3 text-gray-500">
                        {{ invitee.latest_paid_at ? formatDate(invitee.latest_paid_at) : t('referral.notPaid', '未充值') }}
                      </td>
                      <td class="px-4 py-3 text-gray-500">{{ formatDate(invitee.bound_at) }}</td>
                    </tr>
                    <!-- Expanded reward details -->
                    <tr v-if="expandedInviteeId === invitee.user_id">
                      <td :colspan="7" class="bg-gray-50/50 px-4 py-3 dark:bg-dark-800/30">
                        <div v-if="inviteeRewardsLoading" class="flex items-center justify-center py-4 text-sm text-gray-500">
                          <LoadingSpinner />
                          <span class="ml-2">{{ t('common.loading', '加载中') }}</span>
                        </div>
                        <div v-else-if="inviteeRewards.length === 0" class="py-4 text-center text-sm text-gray-500">
                          {{ t('referral.noRewards', '暂无返佣明细') }}
                        </div>
                        <table v-else class="min-w-full text-xs">
                          <thead>
                            <tr class="text-left text-gray-500 dark:text-gray-400">
                              <th class="px-3 py-2 font-medium">{{ t('referral.rewardDate', '返佣时间') }}</th>
                              <th class="px-3 py-2 font-medium">{{ t('referral.orderNo', '订单号') }}</th>
                              <th class="px-3 py-2 font-medium">{{ t('referral.orderAmount', '订单金额') }}</th>
                              <th class="px-3 py-2 font-medium">{{ t('referral.commissionRate', '返佣比例') }}</th>
                              <th class="px-3 py-2 font-medium">{{ t('referral.commission', '佣金') }}</th>
                              <th class="px-3 py-2 font-medium">{{ t('common.status', '状态') }}</th>
                            </tr>
                          </thead>
                          <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                            <tr v-for="reward in inviteeRewards" :key="reward.id">
                              <td class="px-3 py-2 text-gray-500">{{ formatDate(reward.created_at) }}</td>
                              <td class="px-3 py-2 font-mono text-gray-700 dark:text-gray-300">{{ reward.external_order_id || '-' }}</td>
                              <td class="px-3 py-2 text-gray-700 dark:text-gray-300">{{ formatMoney(reward.order_paid_amount) }}</td>
                              <td class="px-3 py-2 text-gray-700 dark:text-gray-300">{{ (reward.rate_snapshot * 100).toFixed(1) }}%</td>
                              <td class="px-3 py-2 font-medium text-green-600 dark:text-green-400">+{{ formatMoney(reward.reward_amount) }}</td>
                              <td class="px-3 py-2">
                                <span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
                                  :class="reward.status === 'settled' ? 'bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400' : reward.status === 'pending' ? 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-400' : 'bg-gray-100 text-gray-800 dark:bg-dark-800 dark:text-gray-300'">
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
                    <td colspan="7" class="px-4 py-6 text-center text-gray-500">
                      {{ t('common.noData', '暂无数据') }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div v-if="invitees.pages > 1" class="mt-4 flex items-center justify-between text-sm">
              <span class="text-gray-500 dark:text-gray-400">{{ t('common.totalPrefix', '共') }} {{ invitees.total }} {{ t('common.totalSuffix', '条') }}</span>
              <div class="flex gap-1">
                <button class="btn btn-secondary btn-sm" :disabled="invitees.page <= 1" @click="loadInviteesPage(invitees.page - 1)">{{ t('common.prevPage', '上一页') }}</button>
                <span class="px-3 py-1.5 text-gray-700 dark:text-gray-300">{{ invitees.page }}/{{ invitees.pages }}</span>
                <button class="btn btn-secondary btn-sm" :disabled="invitees.page >= invitees.pages" @click="loadInviteesPage(invitees.page + 1)">{{ t('common.nextPage', '下一页') }}</button>
              </div>
            </div>
          </section>

          <!-- 佣金流水 -->
          <section id="ledger-section" class="rounded-3xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <div class="mb-4">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('referral.ledger', '佣金入账流水') }}</h2>
            </div>
            <div class="overflow-x-auto rounded-2xl border border-gray-100 dark:border-dark-800">
              <table class="min-w-full text-sm">
                <thead class="bg-gray-50 dark:bg-dark-800/50">
                  <tr class="text-left text-gray-500 dark:text-gray-400">
                    <th class="px-4 py-3 font-medium">{{ t('referral.entryType', '类型') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.sourceUser', '来源') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.orderAmount', '订单金额') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.commissionRate', '比例') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('referral.commission', '佣金') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('common.createdAt', '时间') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                  <tr v-for="entry in ledger.items" :key="entry.id">
                    <td class="px-4 py-3">
                      <div class="font-medium text-gray-900 dark:text-white">{{ formatEntryType(entry.entry_type) }}</div>
                      <div class="text-xs text-gray-500">{{ formatStatus(entry.bucket) }}</div>
                      <div v-if="entry.external_order_id" class="mt-0.5 text-xs font-mono text-gray-400 dark:text-gray-500">{{ entry.external_order_id }}</div>
                    </td>
                    <td class="px-4 py-3 text-gray-700 dark:text-gray-300">
                      {{ entry.source_user_email || entry.source_user_username || '-' }}
                    </td>
                    <td class="px-4 py-3 text-gray-700 dark:text-gray-300">
                      <template v-if="entry.order_paid_amount && entry.order_paid_amount > 0">
                        {{ formatMoney(entry.order_paid_amount) }}
                      </template>
                      <template v-else>-</template>
                    </td>
                    <td class="px-4 py-3 text-gray-700 dark:text-gray-300">
                      <template v-if="entry.reward_rate_snapshot && entry.reward_rate_snapshot > 0">
                        {{ (entry.reward_rate_snapshot * 100).toFixed(1) }}%
                      </template>
                      <template v-else>-</template>
                    </td>
                    <td class="px-4 py-3 font-medium" :class="entry.amount > 0 ? 'text-green-600 dark:text-green-400' : entry.amount < 0 ? 'text-red-600 dark:text-red-400' : 'text-gray-900 dark:text-white'">
                      {{ entry.amount > 0 ? '+' : '' }}{{ formatMoney(entry.amount) }}
                    </td>
                    <td class="px-4 py-3 text-gray-500">{{ formatDate(entry.created_at) }}</td>
                  </tr>
                  <tr v-if="!ledger.items.length">
                    <td colspan="6" class="px-4 py-6 text-center text-gray-500">
                      {{ t('common.noData', '暂无数据') }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div v-if="ledger.pages > 1" class="mt-4 flex items-center justify-between text-sm">
              <span class="text-gray-500 dark:text-gray-400">{{ t('common.totalPrefix', '共') }} {{ ledger.total }} {{ t('common.totalSuffix', '条') }}</span>
              <div class="flex gap-1">
                <button class="btn btn-secondary btn-sm" :disabled="ledger.page <= 1" @click="loadLedgerPage(ledger.page - 1)">{{ t('common.prevPage', '上一页') }}</button>
                <span class="px-3 py-1.5 text-gray-700 dark:text-gray-300">{{ ledger.page }}/{{ ledger.pages }}</span>
                <button class="btn btn-secondary btn-sm" :disabled="ledger.page >= ledger.pages" @click="loadLedgerPage(ledger.page + 1)">{{ t('common.nextPage', '下一页') }}</button>
              </div>
            </div>
          </section>
        </div>
      </template>
    </div>

    <!-- Convert Modal -->
    <div v-if="showConvertModal" class="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/50 backdrop-blur-sm" @click.self="showConvertModal = false" @keydown.esc="showConvertModal = false" tabindex="0" ref="convertModalBackdrop">
      <div class="w-full max-w-sm rounded-3xl border border-gray-200 bg-white p-6 shadow-xl dark:border-dark-700 dark:bg-dark-900">
        <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('referral.convertToCredit', '将佣金转储为平台余额') }}</h3>
        <p class="mt-2 text-sm text-gray-500">{{ t('referral.convertDesc', '实时到账平台账户余额，可用于抵扣消费。') }}</p>
        
        <div class="mt-4">
          <label class="mb-1 block text-sm text-gray-600 dark:text-gray-400">{{ t('referral.convertAmount', '转入金额') }}</label>
          <div class="relative">
            <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3">
              <span class="text-gray-500 sm:text-sm">￥</span>
            </div>
            <input
              v-model="convertAmount"
              type="number"
              min="0"
              :max="maxWithdrawable"
              step="0.01"
              class="input pl-8 w-full"
              placeholder="0.00"
            />
            <div class="absolute inset-y-0 right-1 flex items-center">
               <button type="button" class="rounded bg-gray-100 px-2 py-1 text-xs font-medium text-gray-700 outline-none hover:bg-gray-200 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700" @click="convertAmount = String(maxWithdrawable)">
                 {{ t('referral.withdrawAll', '全部转出') }}
               </button>
            </div>
          </div>
        </div>

        <div class="mt-6 flex gap-3">
          <button class="btn btn-secondary flex-1" @click="showConvertModal = false">{{ t('common.cancel', '取消') }}</button>
          <button 
            class="btn btn-primary flex-1 bg-gradient-to-r from-primary-500 to-indigo-500 border-none" 
            :disabled="converting || !Number(convertAmount) || Number(convertAmount) <= 0 || Number(convertAmount) > maxWithdrawable"
            @click="handleConvertToCredit"
          >
            {{ converting ? t('common.processing', '转换中...') : t('common.confirm', '确认转换') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Bucket Detail Modal -->
    <div v-if="bucketDetailVisible" class="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/50 backdrop-blur-sm" @click.self="bucketDetailVisible = false" @keydown.esc="bucketDetailVisible = false" tabindex="0">
      <div class="w-full max-w-2xl max-h-[80vh] flex flex-col rounded-3xl border border-gray-200 bg-white shadow-xl dark:border-dark-700 dark:bg-dark-900">
        <div class="flex items-center justify-between border-b border-gray-100 px-6 py-4 dark:border-dark-800">
          <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ bucketDetailTitle }}</h3>
          <button class="rounded-full p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-800" @click="bucketDetailVisible = false">
            <svg viewBox="0 0 20 20" fill="currentColor" class="h-5 w-5"><path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z"/></svg>
          </button>
        </div>
        <div class="flex-1 overflow-y-auto px-6 py-4">
          <div v-if="bucketDetailLoading" class="flex items-center justify-center py-8 text-sm text-gray-500">
            <LoadingSpinner /><span class="ml-2">{{ t('common.loading', '加载中') }}</span>
          </div>
          <div v-else-if="!bucketDetailItems.length" class="py-12 text-center text-sm text-gray-500">
            {{ t('common.noData', '暂无数据') }}
          </div>
          <div v-else class="space-y-3">
            <div v-for="entry in bucketDetailItems" :key="entry.id" class="rounded-2xl border border-gray-100 p-4 dark:border-dark-800">
              <div class="flex items-start justify-between">
                <div>
                  <div class="font-medium text-gray-900 dark:text-white">{{ formatEntryType(entry.entry_type) }}</div>
                  <div class="mt-0.5 text-xs text-gray-500">{{ formatStatus(entry.bucket) }}</div>
                  <div v-if="entry.external_order_id" class="mt-0.5 text-xs font-mono text-gray-400">{{ entry.external_order_id }}</div>
                </div>
                <div class="text-right">
                  <div class="font-semibold" :class="entry.amount > 0 ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'">
                    {{ entry.amount > 0 ? '+' : '' }}￥{{ formatMoney(entry.amount) }}
                  </div>
                  <div class="mt-0.5 text-xs text-gray-500">{{ formatDate(entry.created_at) }}</div>
                </div>
              </div>
              <div v-if="entry.source_user_email || entry.order_paid_amount" class="mt-2 flex flex-wrap gap-3 text-xs text-gray-500">
                <span v-if="entry.source_user_email">{{ t('referral.sourceUser', '来源') }}: {{ entry.source_user_email }}</span>
                <span v-if="entry.order_paid_amount">{{ t('referral.orderAmount', '订单金额') }}: ￥{{ formatMoney(entry.order_paid_amount) }}</span>
                <span v-if="entry.reward_rate_snapshot">{{ t('referral.commissionRate', '比例') }}: {{ (entry.reward_rate_snapshot * 100).toFixed(1) }}%</span>
              </div>
            </div>
          </div>
        </div>
        <div v-if="bucketDetailItems.length" class="border-t border-gray-100 px-6 py-3 text-center text-xs text-gray-500 dark:border-dark-800">
          {{ t('common.totalPrefix', '共') }} {{ bucketDetailItems.length }} {{ t('common.totalSuffix', '条') }}
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import PayoutAccountBinder from './referral-components/PayoutAccountBinder.vue'
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

const { t } = useI18n()
const appStore = useAppStore()

// State
const loading = ref(true)
const creatingWithdrawal = ref(false)

const overview = ref<ReferralCenterOverview | null>(null)
const invitees = ref<BasePaginationResponse<ReferralInvitee>>({ items: [], total: 0, page: 1, page_size: 10, pages: 1 })
const ledger = ref<BasePaginationResponse<CommissionLedgerEntry>>({ items: [], total: 0, page: 1, page_size: 15, pages: 1 })
const withdrawals = ref<BasePaginationResponse<CommissionWithdrawal>>({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
const payoutAccounts = ref<CommissionPayoutAccount[]>([])

// Invitee expand
const expandedInviteeId = ref<number | null>(null)
const inviteeRewards = ref<UserInviteeReward[]>([])
const inviteeRewardsLoading = ref(false)

// Convert Modal
const showConvertModal = ref(false)
const converting = ref(false)
const convertAmount = ref('')
const convertModalBackdrop = ref<HTMLDivElement | null>(null)

// Bucket detail modal
const bucketDetailVisible = ref(false)
const bucketDetailTitle = ref('')
const bucketDetailLoading = ref(false)
const bucketDetailItems = ref<CommissionLedgerEntry[]>([])

async function openBucketDetail(bucket: string, title: string) {
  bucketDetailTitle.value = title
  bucketDetailVisible.value = true
  bucketDetailLoading.value = true
  bucketDetailItems.value = []
  try {
    // Load a large page of ledger entries and filter by bucket on the client side
    const data = await referralAPI.getLedger(1, 200)
    const bucketMap: Record<string, string[]> = {
      available: ['available'],
      processing: ['pending', 'frozen'],
      settled: ['settled']
    }
    const matchBuckets = bucketMap[bucket] || [bucket]
    bucketDetailItems.value = data.items.filter((entry) => matchBuckets.includes(entry.bucket))
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '加载失败'))
  } finally {
    bucketDetailLoading.value = false
  }
}

watch(showConvertModal, (val) => {
  if (val) {
    nextTick(() => convertModalBackdrop.value?.focus())
  }
})

// Withdraw Form
const withdrawForm = reactive({
  amount: 0 as number,
  payout_account_id: 0,
  remark: ''
})

const withdrawMethods = computed(() => overview.value?.withdraw_methods_enabled?.length ? overview.value.withdraw_methods_enabled : ['alipay', 'wechat', 'bank'])
const maxWithdrawable = computed(() => Number(overview.value?.available_commission || 0))
const withdrawEnabled = computed(() => Boolean(overview.value?.referral_withdraw_enabled))

const inviteLink = computed(() => {
  if (!overview.value?.default_code?.code) return ''
  return `${window.location.origin}/register?ref=${overview.value.default_code.code}`
})

async function loadPayoutAccountsAndOverview() {
  try {
    const [overviewData, payoutAccountsData] = await Promise.all([
      referralAPI.getOverview(),
      referralAPI.getPayoutAccounts()
    ])
    overview.value = overviewData
    payoutAccounts.value = payoutAccountsData

    if (payoutAccounts.value.length > 0 && (!withdrawForm.payout_account_id || !payoutAccounts.value.find(a => a.id === withdrawForm.payout_account_id))) {
      withdrawForm.payout_account_id = payoutAccounts.value.find((item) => item.is_default)?.id || payoutAccounts.value[0].id
    }
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '操作失败'))
  }
}

async function loadAll() {
  loading.value = true
  try {
    await loadPayoutAccountsAndOverview()

    const [inviteesData, ledgerData, withdrawalsData] = await Promise.all([
      referralAPI.getInvitees(1, 10),
      referralAPI.getLedger(1, 15),
      referralAPI.getWithdrawals()
    ])
    invitees.value = inviteesData
    ledger.value = ledgerData
    withdrawals.value = withdrawalsData
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '加载失败'))
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
    withdraw_paid: t('referral.withdrawPaid', '提现打款'),
    admin_add: t('referral.adminAdd', '管理员调增'),
    admin_subtract: t('referral.adminSubtract', '管理员扣减')
  }
  return map[entryType] || entryType
}

function formatStatus(status: string): string {
  const map: Record<string, string> = {
    'pending': '待处理',
    'pending_review': '待审核',
    'approved': '已批准',
    'rejected': '已拒绝',
    'paid': '已打款',
    'available': '可用',
    'frozen': '已冻结',
    'reversed': '已撤销',
    'partially_reversed': '部分撤销',
    'partially_frozen': '部分冻结',
    'partially_paid': '部分打款',
    'credited': '已入账',
    'refunded': '已退款',
    'active': '正常',
    'settled': '已结算',
    'withdrawn': '已提现',
  }
  return map[status] || status
}

function formatDate(value?: string | Date | null) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

onMounted(loadAll)
</script>
