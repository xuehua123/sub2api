<template>
  <BaseDialog
    :show="show"
    :title="t('admin.referral.withdrawalItemsTitle', '提现审核')"
    width="wide"
    @close="emit('close')"
  >
    <div v-if="loading" class="flex flex-col items-center justify-center py-20 text-sm text-primary-500">
      <LoadingSpinner class="h-10 w-10 text-primary-500" />
      <span class="mt-4">{{ t('common.loading', '加载中') }}</span>
    </div>

    <div v-else class="space-y-5">
      <!-- Header: Withdrawal No + Status + Amount -->
      <div class="flex items-center justify-between rounded-2xl border border-gray-200 bg-gray-50/50 p-5 dark:border-dark-700 dark:bg-dark-800/50">
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.referral.withdrawalNo', '提现单号') }}</div>
          <div class="mt-1 font-mono text-sm font-semibold text-gray-900 dark:text-gray-200">{{ withdrawal?.withdrawal_no || '-' }}</div>
        </div>
        <div class="flex items-center gap-4">
          <span
            class="inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-semibold"
            :class="statusBadgeClass(withdrawal?.status || '')"
          >
            <span class="h-1.5 w-1.5 rounded-full" :class="statusDotClass(withdrawal?.status || '')" />
            {{ formatStatus(withdrawal?.status || '') }}
          </span>
          <div class="text-right">
            <div class="text-xs text-gray-500">{{ t('admin.referral.withdrawalNet', '实付金额') }}</div>
            <div class="mt-0.5 font-mono text-xl font-bold text-primary-600 dark:text-primary-400">
              {{ formatCurrency(withdrawal?.net_amount || 0) }}
            </div>
          </div>
        </div>
      </div>

      <!-- User Info -->
      <div class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.referral.userInfo', '用户信息') }}</h3>
        <div class="mt-3 grid grid-cols-2 gap-4 text-sm">
          <div>
            <div class="text-xs text-gray-500">{{ t('common.email', '邮箱') }}</div>
            <div class="mt-1 font-medium text-gray-900 dark:text-white">{{ withdrawal?.user_email || '-' }}</div>
          </div>
          <div>
            <div class="text-xs text-gray-500">{{ t('admin.referral.username', '用户名') }}</div>
            <div class="mt-1 font-medium text-gray-900 dark:text-white">{{ withdrawal?.username || '-' }}</div>
          </div>
        </div>
      </div>

      <!-- Withdrawal Details -->
      <div class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.referral.withdrawalDetails', '提现详情') }}</h3>
        <div class="mt-3 grid grid-cols-3 gap-4 text-sm">
          <div>
            <div class="text-xs text-gray-500">{{ t('admin.referral.withdrawalAmount', '申请金额') }}</div>
            <div class="mt-1 font-mono font-medium text-gray-900 dark:text-white">{{ formatCurrency(withdrawal?.amount || 0) }}</div>
          </div>
          <div>
            <div class="text-xs text-gray-500">{{ t('admin.referral.withdrawalFee', '手续费') }}</div>
            <div class="mt-1 font-mono text-red-500">{{ formatCurrency(withdrawal?.fee_amount || 0) }}</div>
          </div>
          <div>
            <div class="text-xs text-gray-500">{{ t('admin.referral.withdrawalNet', '实付金额') }}</div>
            <div class="mt-1 font-mono font-semibold text-green-600 dark:text-green-400">{{ formatCurrency(withdrawal?.net_amount || 0) }}</div>
          </div>
        </div>

        <!-- Payout method + account -->
        <div class="mt-4 border-t border-gray-100 pt-4 dark:border-dark-700">
          <div class="grid grid-cols-2 gap-4 text-sm">
            <div>
              <div class="text-xs text-gray-500">{{ t('referral.payoutMethod', '收款方式') }}</div>
              <div class="mt-1 font-medium text-gray-900 dark:text-white">{{ formatPayoutMethod(withdrawal?.payout_method || '') }}</div>
            </div>
            <div v-if="payoutSnapshot?.account_name">
              <div class="text-xs text-gray-500">{{ t('referral.accountName', '收款人名称') }}</div>
              <div class="mt-1 font-medium text-gray-900 dark:text-white">{{ payoutSnapshot.account_name }}</div>
            </div>
            <div v-if="payoutSnapshot?.account_no_encrypted">
              <div class="text-xs text-gray-500">{{ t('admin.referral.payoutAccountNo', '收款账号') }}</div>
              <div class="mt-1 font-mono text-gray-900 dark:text-white">{{ payoutSnapshot.account_no_encrypted }}</div>
            </div>
            <div v-if="payoutSnapshot?.bank_name">
              <div class="text-xs text-gray-500">{{ t('referral.bankName', '开户银行') }}</div>
              <div class="mt-1 text-gray-900 dark:text-white">{{ payoutSnapshot.bank_name }}</div>
            </div>
          </div>
          <div v-if="payoutSnapshot?.qr_image_url" class="mt-3 text-sm">
            <div class="text-xs text-gray-500">{{ t('referral.qrCode', '收款二维码') }}</div>
            <div class="mt-1 break-all text-primary-600 dark:text-primary-400">{{ payoutSnapshot.qr_image_url }}</div>
          </div>
        </div>

        <!-- Timeline: submit / review / payment -->
        <div class="mt-4 border-t border-gray-100 pt-4 dark:border-dark-700">
          <div class="grid grid-cols-3 gap-4 text-sm">
            <div>
              <div class="text-xs text-gray-500">{{ t('admin.referral.submitTime', '申请时间') }}</div>
              <div class="mt-1 text-gray-700 dark:text-gray-300">{{ formatDate(withdrawal?.created_at) }}</div>
            </div>
            <div>
              <div class="text-xs text-gray-500">{{ t('admin.referral.reviewTime', '审核时间') }}</div>
              <div class="mt-1 text-gray-700 dark:text-gray-300">{{ formatDate(withdrawal?.reviewed_at) }}</div>
            </div>
            <div>
              <div class="text-xs text-gray-500">{{ t('admin.referral.paymentTime', '打款时间') }}</div>
              <div class="mt-1 text-gray-700 dark:text-gray-300">{{ formatDate(withdrawal?.paid_at) }}</div>
            </div>
          </div>
        </div>

        <!-- Reject reason -->
        <div v-if="withdrawal?.reject_reason" class="mt-4 rounded-xl border border-red-200 bg-red-50/50 p-3 dark:border-red-900/30 dark:bg-red-900/10">
          <div class="text-xs font-medium text-red-600 dark:text-red-400">{{ t('admin.referral.rejectReason', '驳回原因') }}</div>
          <div class="mt-1 text-sm text-red-700 dark:text-red-300">{{ withdrawal.reject_reason }}</div>
        </div>
      </div>

      <!-- Commission Source Table -->
      <div class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.referral.commissionSource', '佣金来源明细') }}</h3>
        <div class="mt-3 overflow-x-auto">
          <table v-if="items.length" class="w-full text-sm">
            <thead>
              <tr class="border-b border-gray-200 dark:border-dark-700">
                <th class="px-3 py-2 text-left text-xs font-medium text-gray-500">{{ t('referral.sourceUser', '来源用户') }}</th>
                <th class="px-3 py-2 text-left text-xs font-medium text-gray-500">{{ t('referral.orderNo', '订单号') }}</th>
                <th class="px-3 py-2 text-right text-xs font-medium text-gray-500">{{ t('referral.orderAmount', '充值金额') }}</th>
                <th class="px-3 py-2 text-right text-xs font-medium text-gray-500">{{ t('referral.commissionRate', '比例') }}</th>
                <th class="px-3 py-2 text-right text-xs font-medium text-gray-500">{{ t('admin.referral.allocatedAmount', '分配金额') }}</th>
                <th class="px-3 py-2 text-right text-xs font-medium text-gray-500">{{ t('admin.referral.feeAllocatedAmount', '手续费') }}</th>
                <th class="px-3 py-2 text-right text-xs font-medium text-gray-500">{{ t('admin.referral.netAllocatedAmount', '净额') }}</th>
                <th class="px-3 py-2 text-left text-xs font-medium text-gray-500">{{ t('admin.referral.submitTime', '充值时间') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
              <tr v-for="item in items" :key="item.id" class="text-gray-700 dark:text-gray-300">
                <td class="px-3 py-2.5 text-xs">{{ item.source_user_email || '-' }}</td>
                <td class="px-3 py-2.5 font-mono text-xs text-gray-500">{{ item.external_order_id || item.recharge_order_id }}</td>
                <td class="px-3 py-2.5 text-right font-mono">
                  <template v-if="item.order_paid_amount">{{ formatCurrency(item.order_paid_amount) }}</template>
                  <template v-else>-</template>
                </td>
                <td class="px-3 py-2.5 text-right text-xs">
                  <template v-if="item.reward_rate_snapshot">{{ (item.reward_rate_snapshot * 100).toFixed(1) }}%</template>
                  <template v-else>-</template>
                </td>
                <td class="px-3 py-2.5 text-right font-mono">{{ formatCurrency(item.allocated_amount) }}</td>
                <td class="px-3 py-2.5 text-right font-mono text-red-500/80">{{ formatCurrency(item.fee_allocated_amount) }}</td>
                <td class="px-3 py-2.5 text-right font-mono font-medium text-green-600 dark:text-green-400">{{ formatCurrency(item.net_allocated_amount) }}</td>
                <td class="px-3 py-2.5 text-xs text-gray-500">{{ item.order_paid_at ? formatDate(item.order_paid_at) : '-' }}</td>
              </tr>
            </tbody>
          </table>
          <div v-else class="flex flex-col items-center py-8">
            <svg class="mb-2 h-8 w-8 text-gray-300 dark:text-gray-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4" />
            </svg>
            <span class="text-xs text-gray-400">{{ t('common.noData', '暂无明细数据') }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Action Buttons -->
    <template #footer>
      <div class="flex flex-col gap-3 w-full">
        <!-- Reject reason input (shown when rejecting) -->
        <div v-if="showRejectInput" class="space-y-2">
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.referral.rejectReason', '驳回原因') }}</label>
          <textarea
            v-model="rejectReason"
            class="w-full rounded-xl border border-gray-300 bg-white px-3 py-2 text-sm focus:border-primary-500 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-800 dark:text-white"
            rows="2"
            :placeholder="t('admin.referral.rejectReasonPlaceholder', '请输入驳回原因（必填）')"
          />
          <div class="flex gap-2">
            <button
              class="flex-1 rounded-xl border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
              @click="showRejectInput = false; rejectReason = ''"
            >{{ t('common.cancel', '取消') }}</button>
            <button
              class="flex-1 rounded-xl bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-500 disabled:opacity-50"
              :disabled="acting || !rejectReason.trim()"
              @click="confirmReject"
            >{{ t('admin.referral.confirmReject', '确认驳回') }}</button>
          </div>
        </div>

        <!-- Normal action buttons -->
        <div v-else-if="withdrawal?.status === 'pending_review' || withdrawal?.status === 'approved'" class="flex gap-3">
          <button
            v-if="withdrawal?.status === 'pending_review'"
            class="flex-1 inline-flex items-center justify-center rounded-xl px-4 py-2.5 text-sm font-semibold text-red-700 bg-red-100 hover:bg-red-200 transition dark:bg-red-500/20 dark:text-red-400 dark:hover:bg-red-500/30"
            :disabled="acting"
            @click="showRejectInput = true"
          >
            {{ t('admin.referral.reject', '驳回申请') }}
          </button>
          <button
            v-if="withdrawal?.status === 'pending_review'"
            class="flex-1 inline-flex items-center justify-center rounded-xl bg-primary-600 px-4 py-2.5 text-sm font-semibold text-white shadow-sm transition hover:bg-primary-500"
            :disabled="acting"
            @click="approveWithdrawal"
          >
            {{ t('admin.referral.approve', '通过审核') }}
          </button>
          <button
            v-if="withdrawal?.status === 'approved'"
            class="w-full inline-flex items-center justify-center rounded-xl bg-green-600 px-4 py-2.5 text-sm font-semibold text-white shadow-sm transition hover:bg-green-500"
            :disabled="acting"
            @click="markPaid"
          >
            <svg class="mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
            </svg>
            {{ t('admin.referral.markPaid', '确认已打款') }}
          </button>
        </div>
        <div v-else class="text-center text-sm font-medium text-gray-400 dark:text-gray-500 py-1">
          {{ t('admin.referral.auditNoActions', '已完结') }}
        </div>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import referralAdminAPI from '@/api/admin/referral'
import { useAppStore } from '@/stores'
import type { AdminCommissionWithdrawal, CommissionWithdrawalItem } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

interface Props {
  show: boolean
  loading: boolean
  withdrawal: AdminCommissionWithdrawal | null
  items: CommissionWithdrawalItem[]
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'actionSuccess'): void
}>()

const acting = ref(false)
const showRejectInput = ref(false)
const rejectReason = ref('')

const payoutSnapshot = computed(() => {
  const raw = props.withdrawal?.payout_account_snapshot_json
  if (!raw) return null
  try {
    return JSON.parse(raw) as {
      method?: string
      account_name?: string
      account_no_encrypted?: string
      bank_name?: string
      qr_image_url?: string
    }
  } catch {
    return null
  }
})

async function approveWithdrawal() {
  if (!props.withdrawal) return
  acting.value = true
  try {
    await referralAdminAPI.approveWithdrawal(props.withdrawal.id)
    appStore.showSuccess(t('admin.referral.approveSuccess', '提现单审核通过'))
    emit('actionSuccess')
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '操作失败'))
  } finally {
    acting.value = false
  }
}

async function confirmReject() {
  if (!props.withdrawal || !rejectReason.value.trim()) return
  acting.value = true
  try {
    await referralAdminAPI.rejectWithdrawal(props.withdrawal.id, rejectReason.value.trim())
    appStore.showSuccess(t('admin.referral.rejectSuccess', '提现单已驳回'))
    showRejectInput.value = false
    rejectReason.value = ''
    emit('actionSuccess')
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '操作失败'))
  } finally {
    acting.value = false
  }
}

async function markPaid() {
  if (!props.withdrawal) return
  acting.value = true
  try {
    await referralAdminAPI.markWithdrawalPaid(props.withdrawal.id)
    appStore.showSuccess(t('admin.referral.markPaidSuccess', '已确认打款完成'))
    emit('actionSuccess')
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '操作失败'))
  } finally {
    acting.value = false
  }
}

function formatCurrency(value: number) {
  return '\uFFE5' + Number(value || 0).toFixed(2)
}

function formatStatus(status: string): string {
  const map: Record<string, string> = {
    pending_review: '待审核',
    approved: '已通过',
    rejected: '已驳回',
    paid: '已打款',
    frozen: '冻结中'
  }
  return map[status] || status
}

function formatPayoutMethod(method: string): string {
  const map: Record<string, string> = {
    alipay: '支付宝',
    wechat: '微信',
    bank: '银行卡'
  }
  return map[method] || method || '-'
}

function statusBadgeClass(status: string): string {
  const map: Record<string, string> = {
    pending_review: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-500/10 dark:text-yellow-400',
    approved: 'bg-blue-100 text-blue-700 dark:bg-blue-500/10 dark:text-blue-400',
    paid: 'bg-green-100 text-green-700 dark:bg-green-500/10 dark:text-green-400',
    rejected: 'bg-red-100 text-red-700 dark:bg-red-500/10 dark:text-red-400',
    frozen: 'bg-gray-100 text-gray-700 dark:bg-gray-500/10 dark:text-gray-400'
  }
  return map[status] || 'bg-gray-100 text-gray-700 dark:bg-gray-500/10 dark:text-gray-400'
}

function statusDotClass(status: string): string {
  const map: Record<string, string> = {
    pending_review: 'bg-yellow-500 animate-pulse',
    approved: 'bg-blue-500',
    paid: 'bg-green-500',
    rejected: 'bg-red-500',
    frozen: 'bg-gray-500'
  }
  return map[status] || 'bg-gray-500'
}

function formatDate(value?: string | Date | null) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}
</script>
