<template>
  <BaseDialog
    :show="drawerOpen"
    :title="t('admin.referral.workspaceTitle', '用户分销档案')"
    width="extra-wide"
    @close="closeDrawer"
  >
    <div v-if="loading" class="flex flex-col items-center justify-center py-16 text-sm text-gray-500">
      <LoadingSpinner class="h-8 w-8 text-primary-500" />
      <span class="mt-4">{{ t('common.loading', '正在调取档案库...') }}</span>
    </div>

    <div v-else-if="account" class="flex flex-col md:flex-row gap-6 pb-4">
      <!-- 左侧：基础档案与导航 -->
      <div class="w-full md:w-1/3 xl:w-1/4 shrink-0 space-y-4">
        <div class="rounded-2xl bg-gray-50 p-5 dark:bg-dark-800">
          <div class="flex h-16 w-16 items-center justify-center rounded-full bg-primary-100 text-xl font-bold text-primary-700 dark:bg-primary-900/50 dark:text-primary-300 mx-auto">
            {{ account.email.charAt(0).toUpperCase() }}
          </div>
          <div class="mt-4 text-center">
            <h3 class="text-lg font-bold text-gray-900 dark:text-white truncate" :title="account.email">
              {{ account.email }}
            </h3>
            <div class="mt-1 flex items-center justify-center gap-2 text-xs text-gray-500">
              UID: {{ account.user_id }}
            </div>
            <div v-if="account.referral_code" class="mt-3 inline-block rounded bg-white px-3 py-1 font-mono text-sm font-semibold text-gray-700 shadow-sm border border-gray-100 dark:bg-dark-900 dark:border-dark-700 dark:text-gray-300">
              {{ t('admin.referral.promoCode') }}: {{ account.referral_code }}
            </div>
          </div>
        </div>

        <div class="rounded-2xl border border-gray-100 p-4 dark:border-dark-700">
          <div class="space-y-3">
            <div class="flex justify-between items-center text-sm">
              <span class="text-gray-500">{{ t('admin.referral.directDownline') }}</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ account.direct_invitees }} <span class="font-normal text-xs text-gray-400">{{ t('admin.referral.peopleSuffix') }}</span></span>
            </div>
            <div class="flex justify-between items-center text-sm">
              <span class="text-gray-500">{{ t('admin.referral.viralNetwork') }}</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ account.second_level_invitees }} <span class="font-normal text-xs text-gray-400">{{ t('admin.referral.peopleSuffix') }}</span></span>
            </div>
            <div class="flex justify-between items-center text-sm pt-2 border-t border-gray-100 dark:border-dark-700">
              <span class="text-gray-500">{{ t('admin.referral.totalCommissionOutput') }}</span>
              <span class="font-bold text-green-600">￥{{ formatMoney(account.total_commission) }}</span>
            </div>
          </div>
        </div>

        <!-- 垂直导航菜单 -->
        <nav class="flex flex-col gap-1">
          <button 
            v-for="tab in processedTabs" 
            :key="tab.value"
            @click="activeTab = tab.value"
            :data-test="`workspace-tab-${tab.value}`"
            class="flex items-center gap-3 rounded-xl px-4 py-3 text-sm font-medium transition-colors text-left"
            :class="activeTab === tab.value ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'text-gray-600 hover:bg-gray-50 dark:text-gray-400 dark:hover:bg-dark-800'"
          >
            <component :is="tab.icon" class="h-5 w-5" :class="activeTab === tab.value ? 'text-primary-500' : 'text-gray-400'" />
            {{ tab.label }}
          </button>
        </nav>
      </div>

      <!-- 右侧：功能面板 -->
      <div class="flex-1 min-w-0">
        
        <!-- 面板：更换上级 -->
        <div v-if="activeTab === 'relation'" class="space-y-6">
          <div class="rounded-2xl border border-gray-200 p-6 dark:border-dark-700">
            <h3 class="text-lg font-bold text-gray-900 dark:text-white mb-2">{{ t('admin.referral.changeReferrerTitle') }}</h3>
            <p class="text-sm text-gray-500 mb-6">{{ t('admin.referral.changeReferrerDesc') }}</p>
            
            <div class="mb-6 flex items-center justify-between rounded-xl bg-gray-50 p-4 dark:bg-dark-800">
              <div>
                <div class="text-xs text-gray-500 mb-1">{{ t('admin.referral.currentUpstream') }}</div>
                <div class="font-semibold text-gray-900 dark:text-white">
                  {{ relation?.referrer_email || t('admin.referral.organicSignup') }}
                </div>
              </div>
              <button class="btn btn-secondary btn-sm" @click="emit('openTree', account)">
                {{ t('admin.referral.viewHierarchy') }}
              </button>
            </div>

            <form class="space-y-4" data-test="workspace-relation-form" @submit.prevent="submitRelation">
              <div>
                <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">{{ t('admin.referral.searchNewUpstream') }}</label>
                <ReferralAccountPicker
                  label=""
                  input-test-id="workspace-upstream-input"
                  :placeholder="t('admin.referral.searchByEmailPlaceholder')"
                  :query="upstreamQuery"
                  :model-value="selectedUpstream"
                  :options="upstreamResults"
                  :loading="upstreamLoading"
                  :show-referral-code="true"
                  @update:query="upstreamQuery = $event"
                  @search="searchUpstreamAccounts"
                  @select="selectUpstream"
                  @clear="clearUpstream"
                />
              </div>

              <div v-if="selectedUpstream" class="rounded-xl border border-green-200 bg-green-50 p-3 text-sm text-green-700 dark:border-green-900/40 dark:bg-green-900/20 dark:text-green-400">
                {{ t('admin.referral.willBindTo') }}: <span class="font-bold">{{ selectedUpstream.email }}</span>
              </div>

              <div>
                <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">{{ t('admin.referral.changeReasonLabel') }}</label>
                <textarea
                  v-model="relationForm.reason"
                  class="input w-full min-h-[100px]"
                  :placeholder="t('admin.referral.changeReasonPlaceholder')"
                ></textarea>
              </div>

              <button
                type="submit"
                class="btn btn-primary w-full"
                :disabled="savingRelation || !upstreamResultCode"
              >
                <LoadingSpinner v-if="savingRelation" class="mr-2 h-4 w-4" />
                {{ t('admin.referral.confirmChangeUpstream') }}
              </button>
            </form>
          </div>
        </div>

        <!-- 面板：佣金明细与补偿 -->
        <div v-else-if="activeTab === 'adjustment'" class="space-y-6">
          <div class="rounded-2xl border border-gray-200 p-6 dark:border-dark-700">
            <h3 class="text-lg font-bold text-gray-900 dark:text-white mb-2">{{ t('admin.referral.rewardTrackingTitle') }}</h3>
            <p class="text-sm text-gray-500 mb-6">{{ t('admin.referral.rewardTrackingDesc') }}</p>
            
            <div class="grid grid-cols-1 xl:grid-cols-2 gap-6">
              <!-- 左侧列表 -->
              <div class="space-y-3 max-h-[400px] overflow-y-auto pr-2 custom-scrollbar">
                <div
                  v-for="reward in rewards.items"
                  :key="reward.id"
                  :data-test="`select-reward-${reward.id}`"
                  class="cursor-pointer rounded-xl border p-3 transition"
                  :class="selectedReward?.id === reward.id ? 'border-primary-500 bg-primary-50 ring-1 ring-primary-500 dark:border-primary-500/50 dark:bg-primary-900/20' : 'border-gray-200 hover:border-primary-300 dark:border-dark-700'"
                  @click="selectedReward = reward"
                >
                  <div class="flex justify-between items-start mb-2">
                    <div class="font-semibold text-gray-900 dark:text-white text-sm truncate pr-2 w-48" :title="reward.source_user_email">{{ reward.source_user_email }}</div>
                    <div class="font-mono text-sm font-bold text-green-600 dark:text-green-400">￥{{ formatMoney(reward.reward_amount) }}</div>
                  </div>
                  <div class="flex items-center gap-2 text-xs text-gray-500 flex-wrap">
                    <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-dark-800">LVL {{ reward.level }}</span>
                    <span v-if="reward.rate_snapshot" class="rounded bg-blue-50 px-1.5 py-0.5 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400">{{ (reward.rate_snapshot * 100).toFixed(1) }}%</span>
                    <span>{{ reward.status }}</span>
                    <span>ID:{{ reward.id }}</span>
                  </div>
                  <div v-if="reward.external_order_id || reward.base_amount_snapshot" class="mt-1.5 flex items-center gap-2 text-xs text-gray-400">
                    <span v-if="reward.external_order_id" class="truncate" :title="reward.external_order_id">{{ t('referral.orderAmount', '订单金额') }}: {{ reward.external_order_id }}</span>
                    <span v-if="reward.base_amount_snapshot">￥{{ formatMoney(reward.base_amount_snapshot) }}</span>
                  </div>
                </div>
                <div v-if="!rewards.items.length" class="text-center py-8 text-sm text-gray-500">{{ t('admin.referral.noRewardRecords') }}</div>
              </div>

              <!-- 右侧操作表单 -->
              <div class="rounded-xl bg-gray-50 p-5 dark:bg-dark-800/50">
                <h4 class="font-medium text-gray-900 dark:text-white text-sm mb-4">{{ t('admin.referral.manualAdjustTitle') }}</h4>
                <div v-if="selectedReward">
                  <div class="text-xs text-gray-500 mb-1">{{ t('admin.referral.currentSelectedOrder') }}</div>
                  <div class="font-mono text-sm font-bold mb-4 bg-white dark:bg-dark-900 rounded p-2 text-gray-700 dark:text-gray-300">#{{ selectedReward.id }} - {{ selectedReward.source_user_email }}</div>
                  
                  <form class="space-y-4" data-test="workspace-adjustment-form" @submit.prevent="submitAdjustment">
                    <div>
                      <label class="block text-xs font-medium text-gray-700 dark:text-gray-400 mb-1">{{ t('admin.referral.adjustAmountLabel', '干预金额 (￥)') }}</label>
                      <input v-model="adjustmentForm.amount" data-test="workspace-adjustment-amount" type="number" step="0.01" class="input w-full" :placeholder="t('admin.referral.positiveAddNegativeDeduct')" />
                    </div>
                    <div>
                      <label class="block text-xs font-medium text-gray-700 dark:text-gray-400 mb-1">{{ t('admin.referral.adjustRemarkLabel') }}</label>
                      <textarea v-model="adjustmentForm.remark" class="input w-full min-h-[80px]" :placeholder="t('admin.referral.adjustReasonPlaceholder')"></textarea>
                    </div>
                    <button type="submit" class="btn btn-primary w-full" :disabled="savingAdjustment || !Number(adjustmentForm.amount)">
                      {{ t('admin.referral.executeAdjustment') }}
                    </button>
                  </form>
                </div>
                <div v-else class="flex h-40 items-center justify-center text-sm text-gray-400">
                  {{ t('admin.referral.selectOrderFirst') }}
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 面板：提现明细 -->
        <div v-else-if="activeTab === 'withdrawals'" class="space-y-6">
          <div class="rounded-2xl border border-gray-200 p-6 dark:border-dark-700">
            <h3 class="text-lg font-bold text-gray-900 dark:text-white mb-6">{{ t('admin.referral.withdrawalRequestRecords') }}</h3>
            
            <div class="space-y-3">
              <div v-for="withdrawal in withdrawals.items" :key="withdrawal.id" :data-test="`workspace-withdrawal-${withdrawal.id}`" class="rounded-xl border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800">
                <div class="flex items-center justify-between mb-3">
                  <div class="font-mono text-sm font-bold text-gray-900 dark:text-white">{{ withdrawal.withdrawal_no }}</div>
                  <div class="text-lg font-bold text-primary-600 dark:text-primary-400">￥{{ formatMoney(withdrawal.net_amount) }}</div>
                </div>
                <div class="flex items-center justify-between text-xs text-gray-500">
                  <span class="inline-flex rounded-full bg-white px-2 py-0.5 border border-gray-200 dark:bg-dark-900 dark:border-dark-600">{{ withdrawal.status }}</span>
                  <span>{{ formatDate(withdrawal.created_at) }}</span>
                </div>
              </div>
              <div v-if="!withdrawals.items.length" class="text-center py-10 text-sm text-gray-500">{{ t('admin.referral.noWithdrawalActions') }}</div>
            </div>
          </div>
        </div>

        <!-- 面板：操作日志 -->
        <div v-else-if="activeTab === 'history'" class="space-y-6">
          <div class="rounded-2xl border border-gray-200 p-6 dark:border-dark-700">
            <h3 class="text-lg font-bold text-gray-900 dark:text-white mb-6">{{ t('admin.referral.accountTrailTitle') }}</h3>
            
            <div class="relative border-l border-gray-200 dark:border-dark-700 ml-3 space-y-8">
              <div v-for="item in relationHistories.items" :key="item.id" class="relative pl-6">
                <div class="absolute -left-[5px] top-1.5 h-2.5 w-2.5 rounded-full bg-primary-500 ring-4 ring-white dark:ring-dark-900"></div>
                <div class="mb-1 text-sm font-bold text-gray-900 dark:text-white">{{ item.change_source || t('admin.referral.systemChange') }}</div>
                <div class="mb-2 text-xs text-gray-500">{{ formatDate(item.created_at) }}</div>
                <div class="rounded-xl bg-gray-50 p-3 text-sm dark:bg-dark-800">
                  <div class="mb-2 text-gray-600 dark:text-gray-300">{{ t('admin.referral.reasonLabel') }}: {{ item.reason || t('admin.referral.noDescription') }}</div>
                  <div class="font-mono text-xs mt-1">
                    <span class="text-red-500 line-through">{{ item.old_bind_code || t('admin.referral.none') }}</span>
                    <span class="mx-2 text-gray-400">➔</span>
                    <span class="text-green-600 font-bold">{{ item.new_bind_code || t('admin.referral.none') }}</span>
                  </div>
                </div>
              </div>
              <div v-if="!relationHistories.items.length" class="pl-6 text-sm text-gray-500">{{ t('admin.referral.noChangeRecords') }}</div>
            </div>
          </div>
        </div>

      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, reactive, h } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import ReferralAccountPicker from '@/components/admin/referral/ReferralAccountPicker.vue'
import referralAdminAPI from '@/api/admin/referral'
import { useAppStore } from '@/stores'
import type {
  AdminCommissionReward,
  AdminCommissionWithdrawal,
  AdminReferralAccountOption,
  AdminReferralRankingItem,
  AdminReferralRelation,
  BasePaginationResponse,
  ReferralRelationHistoryEntry
} from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

const emit = defineEmits<{
  (e: 'submitSuccess'): void
  (e: 'openTree', account: any): void
}>()

const drawerOpen = ref(false)
const loading = ref(false)
const activeTab = ref('relation')
const account = ref<AdminReferralRankingItem | null>(null)

// Data states
const relation = ref<AdminReferralRelation | null>(null)
const relationHistories = ref<BasePaginationResponse<ReferralRelationHistoryEntry>>({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
const rewards = ref<BasePaginationResponse<AdminCommissionReward>>({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
const withdrawals = ref<BasePaginationResponse<AdminCommissionWithdrawal>>({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })

// 构造内联 SVG 图标用于菜单
const IconUser = () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor' }, [h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2', d: 'M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z' })])
const IconCoin = () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor' }, [h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2', d: 'M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8c1.11 0 2.08.402 2.599 1M12 8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1M21 12a9 9 0 11-18 0 9 9 0 0118 0z' })])
const IconBank = () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor' }, [h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2', d: 'M8 14v3m4-3v3m4-3v3M3 21h18M3 10h18M3 7l9-4 9 4M4 10h16v11H4V10z' })])
const IconDocument = () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor' }, [h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2', d: 'M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z' })])

const processedTabs = computed(() => [
  { value: 'relation', label: t('admin.referral.tabChangeReferrer'), icon: IconUser },
  { value: 'adjustment', label: t('admin.referral.tabCommissionAdjust'), icon: IconCoin },
  { value: 'withdrawals', label: t('admin.referral.tabWithdrawalRecords'), icon: IconBank },
  { value: 'history', label: t('admin.referral.tabChangeLog'), icon: IconDocument }
])

// Forms
const relationForm = reactive({ reason: '' })
const adjustmentForm = reactive({ amount: '', remark: '' })
const savingRelation = ref(false)
const savingAdjustment = ref(false)

// Selectors
const upstreamQuery = ref('')
const upstreamResults = ref<AdminReferralAccountOption[]>([])
const selectedUpstream = ref<AdminReferralAccountOption | null>(null)
const upstreamLoading = ref(false)
const selectedReward = ref<AdminCommissionReward | null>(null)

const upstreamResultCode = computed(() => selectedUpstream.value?.referral_code || '')

function closeDrawer() {
  drawerOpen.value = false
  account.value = null
  resetState()
}

function resetState() {
  relation.value = null
  selectedUpstream.value = null
  upstreamQuery.value = ''
  upstreamResults.value = []
  relationForm.reason = ''
  adjustmentForm.amount = ''
  adjustmentForm.remark = ''
  selectedReward.value = null
}

async function open(userId: number, initialAccount: AdminReferralRankingItem, defaultTab = 'relation') {
  drawerOpen.value = true
  loading.value = true
  account.value = initialAccount
  activeTab.value = defaultTab
  resetState()

  try {
    const [relRes, histRes, rwdsRes, wthdRes] = await Promise.all([
      referralAdminAPI.listRelations(1, 20, initialAccount.email),
      referralAdminAPI.listRelationHistories(1, 20, userId),
      referralAdminAPI.listCommissionRewards(1, 20, { user_id: userId }),
      referralAdminAPI.listWithdrawals(1, 20, { user_id: userId })
    ])
    relation.value = relRes.items.find((i) => i.user_id === userId) || null
    relationHistories.value = histRes
    rewards.value = rwdsRes
    withdrawals.value = wthdRes
    if (rewards.value.items.length) {
      selectedReward.value = rewards.value.items[0]
    }
  } catch (err) {
    appStore.showError((err as Error).message || 'Load failed')
  } finally {
    loading.value = false
  }
}

// Interacting
async function searchUpstreamAccounts(query: string) {
  upstreamLoading.value = true
  try {
    const res = await referralAdminAPI.searchAccounts(query)
    upstreamResults.value = res.filter(i => i.user_id !== account.value?.user_id)
  } finally {
    upstreamLoading.value = false
  }
}
function selectUpstream(item: AdminReferralAccountOption) {
  selectedUpstream.value = item
  upstreamQuery.value = item.email
  upstreamResults.value = []
}
function clearUpstream() {
  selectedUpstream.value = null
  upstreamQuery.value = ''
  upstreamResults.value = []
}

async function submitRelation() {
  if (!account.value || !upstreamResultCode.value) return
  savingRelation.value = true
  try {
    await referralAdminAPI.updateRelation(account.value.user_id, {
      code: upstreamResultCode.value,
      reason: relationForm.reason.trim()
    })
    appStore.showSuccess(t('admin.referral.relationSaved', '上级已更换成功'))
    emit('submitSuccess')
    open(account.value.user_id, account.value, activeTab.value)
  } catch (err) {
    appStore.showError((err as Error).message)
  } finally {
    savingRelation.value = false
  }
}

async function submitAdjustment() {
  if (!selectedReward.value) return
  savingAdjustment.value = true
  try {
    await referralAdminAPI.createCommissionAdjustment({
      reward_id: selectedReward.value.id,
      amount: Number(adjustmentForm.amount),
      remark: adjustmentForm.remark.trim()
    })
    appStore.showSuccess(t('admin.referral.adjustmentSaved', '调账操作已完成'))
    emit('submitSuccess')
    if (account.value) open(account.value.user_id, account.value, activeTab.value)
  } catch (err) {
    appStore.showError((err as Error).message)
  } finally {
    savingAdjustment.value = false
  }
}

function formatMoney(value: number | undefined) {
  return Number(value || 0).toFixed(2)
}
function formatDate(value?: string | null) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

defineExpose({ open, closeDrawer })
</script>
