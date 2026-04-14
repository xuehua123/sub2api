<template>
  <BaseDialog
    :show="show"
    :title="t('admin.referral.treeTitle', '推广结构树')"
    width="wide"
    @close="emit('close')"
  >
    <div v-if="loading" class="flex flex-col items-center justify-center py-16 text-sm text-gray-500">
      <LoadingSpinner class="h-8 w-8 text-primary-500" />
      <span class="mt-4">{{ t('common.loading', '正在加载结构...') }}</span>
    </div>

    <div v-else-if="treeRoot" class="py-4">
      <!-- 树形视图容器 -->
      <div class="relative">
        
        <!-- 根节点 (Root) -->
        <div class="relative z-10 flex items-start gap-4">
          <div class="flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br from-primary-500 to-indigo-600 text-white shadow-lg ring-4 ring-white dark:ring-dark-900">
            <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" /></svg>
          </div>
          <div class="flex-1 rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2">
              <div>
                <div class="flex items-center gap-2">
                  <span class="rounded bg-primary-100 px-2 py-0.5 text-xs font-bold text-primary-700 dark:bg-primary-900/50 dark:text-primary-300">{{ t('admin.referral.treeRoot') }}</span>
                  <span class="text-lg font-bold text-gray-900 dark:text-white">{{ treeRoot.email }}</span>
                </div>
                <div class="mt-1 text-sm text-gray-500 font-mono">{{ t('admin.referral.inviteCode') }}: {{ treeRoot.referral_code }}</div>
              </div>
              <div class="flex gap-4 text-sm">
                <div class="text-center">
                  <div class="text-gray-500">{{ t('admin.referral.directInvitees', '直接邀请') }}</div>
                  <div class="font-bold text-gray-900 dark:text-white">{{ treeRoot.direct_invitees }} <span class="font-normal text-xs text-gray-400">{{ t('admin.referral.peopleSuffix') }}</span></div>
                </div>
                <div class="text-center">
                  <div class="text-gray-500">{{ t('admin.referral.totalCommission') }}</div>
                  <div class="font-bold text-green-600">￥{{ formatMoney(treeRoot.total_commission) }}</div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 连线 (主干) -->
        <div v-if="treeRoot.children && treeRoot.children.length > 0" class="absolute left-6 top-12 bottom-0 w-px bg-gray-300 dark:bg-dark-600"></div>

        <!-- 一级子节点列表 -->
        <div class="mt-4 pl-12 space-y-6">
          <div v-for="(child, index) in treeRoot.children" :key="child.user_id" class="relative">
            <!-- 直接连线到卡片 -->
            <div class="absolute -left-6 top-6 h-px w-6 bg-gray-300 dark:bg-dark-600"></div>
            <!-- 如果是最后一个元素，遮盖掉主干多余的线 -->
            <div v-if="index === treeRoot.children.length - 1" class="absolute -left-[25px] top-[25px] bottom-0 w-[2px] bg-white dark:bg-dark-900"></div>

            <div class="flex-1 rounded-2xl border border-gray-200 bg-gray-50/50 p-4 transition hover:border-primary-300 hover:shadow-md dark:border-dark-700 dark:bg-dark-800/50">
              <div class="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-4">
                <div class="flex items-start gap-4">
                  <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-blue-100 text-blue-600 dark:bg-blue-900/50 dark:text-blue-400">
                    <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" /></svg>
                  </div>
                  <div>
                    <div class="flex items-center gap-2">
                      <span class="text-base font-bold text-gray-900 dark:text-white">{{ child.email }}</span>
                    </div>
                    <div class="mt-1 flex items-center gap-3 text-xs text-gray-500">
                      <span>ID: {{ child.user_id }}</span>
                      <span>{{ t('admin.referral.codeLabel') }}: <span class="font-mono">{{ child.referral_code }}</span></span>
                    </div>
                    <div class="mt-3 flex gap-4 text-sm">
                      <div>{{ t('admin.referral.directInvitees', '直接邀请') }}: <span class="font-bold text-gray-900 dark:text-white">{{ child.direct_invitees }}</span></div>
                      <div>{{ t('admin.referral.totalContribution') }}: <span class="font-bold text-green-600">￥{{ formatMoney(child.total_commission) }}</span></div>
                    </div>
                  </div>
                </div>
                
                <div>
                  <button
                    class="btn btn-primary btn-sm whitespace-nowrap"
                    @click="emit('openWorkspace', child)"
                  >
                    {{ t('admin.referral.manageThisPerson') }}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div v-if="!treeRoot.children || treeRoot.children.length === 0" class="mt-4 pl-12 text-sm text-gray-500">
          {{ t('admin.referral.noInviteesYet') }}
        </div>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { AdminReferralTreeNode } from '@/types'

const { t } = useI18n()

interface Props {
  show: boolean
  loading: boolean
  treeRoot: AdminReferralTreeNode | null
}

defineProps<Props>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'openWorkspace', node: any): void
}>()

function formatMoney(value: number | undefined) {
  return Number(value || 0).toFixed(2)
}
</script>
