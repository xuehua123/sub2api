<template>
  <AppLayout>
    <ReferralOverview
      :loading="loading"
      :overview="overview"
      :pendingWithdrawals="pendingWithdrawals"
      :quickAccountQuery="quickAccountQuery"
      :quickAccountResults="quickAccountResults"
      :quickAccountLoading="quickAccountLoading"
      :selectedQuickAccount="selectedQuickAccount"
      @refresh="loadAll"
      @updateQuickAccountQuery="quickAccountQuery = $event"
      @searchQuickAccounts="searchQuickAccounts"
      @openWorkspaceFromAccount="openWorkspaceFromAccount"
      @clearQuickAccount="clearQuickAccount"
      @openTree="openTreeDrawer"
      @openWorkspace="openWorkspaceFromRanking"
      @openWorkspaceFromWithdrawal="openWorkspaceFromWithdrawal"
    />

    <ReferralTreeDrawer
      :show="treeDrawerOpen"
      :loading="treeLoading"
      :treeRoot="treeRoot"
      @close="closeTreeDrawer"
      @openWorkspace="openWorkspaceFromTreeNode"
    />

    <AccountWorkbenchDrawer
      ref="workbenchDrawerRef"
      @submitSuccess="loadAll"
      @openTree="openTreeDrawer"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import referralAdminAPI from '@/api/admin/referral'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores'

import ReferralOverview from './referral-components/ReferralOverview.vue'
import ReferralTreeDrawer from './referral-components/ReferralTreeDrawer.vue'
import AccountWorkbenchDrawer from './referral-components/AccountWorkbenchDrawer.vue'

import type {
  AdminReferralAccountOption,
  AdminReferralOverview,
  AdminReferralRankingItem,
  AdminReferralTreeNode,
  AdminCommissionWithdrawal,
  BasePaginationResponse
} from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

// State
const loading = ref(true)
const overview = ref<AdminReferralOverview | null>(null)
const pendingWithdrawals = ref<BasePaginationResponse<AdminCommissionWithdrawal> | null>(null)

// Quick search
const quickAccountLoading = ref(false)
const quickAccountQuery = ref('')
const quickAccountResults = ref<AdminReferralAccountOption[]>([])
const selectedQuickAccount = ref<AdminReferralAccountOption | null>(null)

// Tree drawer
const treeDrawerOpen = ref(false)
const treeLoading = ref(false)
const treeRoot = ref<AdminReferralTreeNode | null>(null)

// Workbench drawer
const workbenchDrawerRef = ref<InstanceType<typeof AccountWorkbenchDrawer> | null>(null)

async function loadAll() {
  loading.value = true
  try {
    const [overviewData, pendingWithdrawalData] = await Promise.all([
      referralAdminAPI.getOverview(),
      referralAdminAPI.listWithdrawals(1, 5, { status: 'pending_review' })
    ])
    overview.value = overviewData
    pendingWithdrawals.value = pendingWithdrawalData
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '操作失败'))
  } finally {
    loading.value = false
  }
}

// Quick Search
async function searchQuickAccounts(query: string) {
  quickAccountLoading.value = true
  try {
    quickAccountResults.value = await referralAdminAPI.searchAccounts(query)
  } finally {
    quickAccountLoading.value = false
  }
}
function clearQuickAccount() {
  selectedQuickAccount.value = null
  quickAccountResults.value = []
  quickAccountQuery.value = ''
}

// Tree logic
async function openTreeDrawer(item: { user_id: number }) {
  treeDrawerOpen.value = true
  treeLoading.value = true
  treeRoot.value = null
  try {
    treeRoot.value = await referralAdminAPI.getTree(item.user_id)
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '操作失败'))
  } finally {
    treeLoading.value = false
  }
}
function closeTreeDrawer() {
  treeDrawerOpen.value = false
  treeRoot.value = null
}

// Workbench mappings
function findRankingItem(userId: number) {
  return overview.value?.ranking.find((i) => i.user_id === userId) || null
}

function openWorkspaceFromRanking(item: AdminReferralRankingItem) {
  workbenchDrawerRef.value?.open(item.user_id, item, 'relation')
}

function openWorkspaceFromAccount(option: AdminReferralAccountOption) {
  selectedQuickAccount.value = option
  const rankingItem = findRankingItem(option.user_id) || {
    user_id: option.user_id,
    email: option.email,
    username: option.username,
    referral_code: option.referral_code,
    direct_invitees: 0,
    second_level_invitees: 0,
    total_commission: 0,
    available_commission: 0,
    withdrawn_commission: 0,
    invitee_count: 0
  }
  workbenchDrawerRef.value?.open(option.user_id, rankingItem, 'relation')
}

function openWorkspaceFromWithdrawal(item: AdminCommissionWithdrawal) {
  const rankingItem = findRankingItem(item.user_id) || {
    user_id: item.user_id,
    email: item.user_email,
    username: item.username,
    referral_code: '', // Cannot know from withdrawal list, but enough for ID
    direct_invitees: 0,
    second_level_invitees: 0,
    total_commission: 0,
    available_commission: 0,
    withdrawn_commission: 0,
    invitee_count: 0
  }
  workbenchDrawerRef.value?.open(item.user_id, rankingItem, 'withdrawals')
}

function openWorkspaceFromTreeNode(item: AdminReferralTreeNode) {
  const rankingItem: AdminReferralRankingItem = {
    user_id: item.user_id,
    email: item.email,
    username: item.username,
    referral_code: item.referral_code,
    direct_invitees: item.direct_invitees,
    second_level_invitees: item.second_level_invitees,
    total_commission: item.total_commission,
    available_commission: item.available_commission,
    withdrawn_commission: 0,
    invitee_count: item.invitee_count
  }
  workbenchDrawerRef.value?.open(item.user_id, rankingItem, 'relation')
}

onMounted(loadAll)
</script>
