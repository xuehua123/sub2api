<template>
  <AppLayout>
    <div class="space-y-4">
      <!-- Actions -->
      <div class="flex items-center justify-end gap-2">
        <label class="inline-flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-gray-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300">
          <input
            v-model="showHiddenPlans"
            type="checkbox"
            class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
          <span>{{ t('payment.admin.showHiddenPlans') }}</span>
        </label>
        <button @click="loadPlans" :disabled="plansLoading" class="btn btn-secondary" :title="t('common.refresh')">
          <Icon name="refresh" size="md" :class="plansLoading ? 'animate-spin' : ''" />
        </button>
        <button @click="openPlanEdit(null)" class="btn btn-primary">{{ t('payment.admin.createPlan') }}</button>
      </div>

      <!-- Plans Table -->
      <DataTable :columns="planColumns" :data="visiblePlans" :loading="plansLoading">
        <template #cell-name="{ value, row }">
          <span class="text-sm font-medium" :class="getPlanNameClass(row)">{{ value }}</span>
        </template>
        <template #cell-access_scope="{ value }">
          <span class="text-sm text-gray-700 dark:text-gray-300">{{ getAccessScopeLabel(value) }}</span>
        </template>
        <template #cell-groups="{ row }">
          <div v-if="row.groups && row.groups.length > 0" class="flex flex-wrap items-center gap-1">
            <GroupBadge
              v-for="group in row.groups.slice(0, 3)"
              :key="group.id"
              :name="group.name"
              :platform="group.platform"
              :rate-multiplier="group.rate_multiplier"
            />
            <span v-if="row.groups.length > 3" class="text-xs text-gray-500 dark:text-gray-400">+{{ row.groups.length - 3 }}</span>
          </div>
          <span v-else-if="row.group_ids && row.group_ids.length > 0" class="text-sm">
            <span class="text-gray-400">#{{ row.group_ids.join(', #') }}</span>
            <span class="ml-1 badge badge-danger">{{ t('payment.admin.groupMissing') }}</span>
          </span>
          <span v-else class="text-sm text-gray-400">-</span>
        </template>
        <template #cell-price="{ value, row }">
          <div class="text-sm">
            <span class="font-medium text-gray-900 dark:text-white">${{ (value ?? 0).toFixed(2) }}</span>
            <span v-if="row.currency" class="ml-1 text-xs text-gray-400">{{ row.currency }}</span>
            <span v-if="row.original_price" class="ml-1 text-xs text-gray-400 line-through">${{ row.original_price.toFixed(2) }}</span>
          </div>
        </template>
        <template #cell-validity_days="{ value, row }">
          <span class="text-sm">{{ value }} {{ getValidityUnitLabel(row.validity_unit) }}</span>
        </template>
        <template #cell-for_sale="{ value, row }">
          <button
            type="button"
            :class="[
              'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              value ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'
            ]"
            @click="toggleForSale(row)"
          >
            <span :class="[
              'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              value ? 'translate-x-4' : 'translate-x-0'
            ]" />
          </button>
        </template>
        <template #cell-actions="{ row }">
          <div class="flex items-center gap-2">
            <button @click="openPlanEdit(row)" class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400">
              <Icon name="edit" size="sm" />
              <span class="text-xs">{{ t('common.edit') }}</span>
            </button>
            <button @click="confirmDeletePlan(row)" class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400">
              <Icon name="trash" size="sm" />
              <span class="text-xs">{{ t('common.delete') }}</span>
            </button>
          </div>
        </template>
      </DataTable>
    </div>

    <!-- Plan Edit Dialog -->
    <PlanEditDialog :show="showPlanDialog" :plan="editingPlan" :groups="visibleGroupsForPlanEdit" :payment-config="paymentConfig" @close="showPlanDialog = false" @saved="loadPlans" />

    <ConfirmDialog :show="showDeletePlanDialog" :title="t('payment.admin.deletePlan')" :message="t('payment.admin.deletePlanConfirm')" :confirm-text="t('common.delete')" danger @confirm="handleDeletePlan" @cancel="showDeletePlanDialog = false" />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import type { AdminPaymentConfig } from '@/api/admin/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import adminAPI from '@/api/admin'
import type { SubscriptionPlan } from '@/types/payment'
import type { AdminGroup } from '@/types'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import PlanEditDialog from './PlanEditDialog.vue'
import { platformTextClass } from '@/utils/platformColors'
import { parsePlanValidityUnit } from '@/utils/subscriptionTime'

const { t } = useI18n()
const appStore = useAppStore()

// ==================== Groups ====================

const groups = ref<AdminGroup[]>([])
const paymentConfig = ref<AdminPaymentConfig | null>(null)

async function loadGroups() {
  try {
    groups.value = await adminAPI.groups.getAll()
  } catch { /* ignore */ }
}

async function loadPaymentConfig() {
  try {
    const res = await adminPaymentAPI.getConfig()
    paymentConfig.value = res.data
  } catch { /* preview only */ }
}

function getGroup(id: number): AdminGroup | undefined {
  return groups.value.find(g => g.id === id)
}

function getPlanNameClass(plan: SubscriptionPlan): string {
  const firstGroup = plan.groups?.[0]
  if (firstGroup?.platform) return platformTextClass(firstGroup.platform)
  const group = getGroup(plan.group_id)
  return group ? platformTextClass(group.platform) : 'text-gray-900 dark:text-white'
}

function getValidityUnitLabel(unit: string | null | undefined): string {
  const parsedUnit = parsePlanValidityUnit(unit)
  if (!parsedUnit) {
    return t('payment.admin.invalidValidityUnitLabel', { unit: unit || '-' })
  }

  switch (parsedUnit) {
    case 'week':
      return t('payment.admin.weeks')
    case 'month':
      return t('payment.admin.months')
    case 'year':
      return t('payment.years')
    default:
      return t('payment.admin.days')
  }
}


// ==================== Plans ====================

const plansLoading = ref(false)
const plans = ref<SubscriptionPlan[]>([])
const showHiddenPlans = ref(false)
const showPlanDialog = ref(false)
const showDeletePlanDialog = ref(false)
const editingPlan = ref<SubscriptionPlan | null>(null)
const deletingPlanId = ref<number | null>(null)

const visiblePlans = computed(() =>
  showHiddenPlans.value ? plans.value : plans.value.filter(plan => plan.for_sale)
)

const visibleGroupsForPlanEdit = computed(() =>
  groups.value.filter(group =>
    group.status === 'active' &&
    (group.subscription_enabled ?? group.subscription_type === 'subscription')
  )
)

const planColumns = computed((): Column[] => [
  { key: 'id', label: 'ID' },
  { key: 'name', label: t('payment.admin.planName') },
  { key: 'access_scope', label: t('payment.admin.accessScope') },
  { key: 'groups', label: t('payment.admin.authorizedGroups') },
  { key: 'price', label: t('payment.admin.price') },
  { key: 'validity_days', label: t('payment.admin.validityDays') },
  { key: 'for_sale', label: t('payment.admin.forSale') },
  { key: 'sort_order', label: t('payment.admin.sortOrder') },
  { key: 'actions', label: t('common.actions') },
])

async function loadPlans() {
  plansLoading.value = true
  try {
    const res = await adminPaymentAPI.getPlans()
    // Backend returns features as newline-separated string; parse to array
    plans.value = (res.data || []).map((p: Omit<SubscriptionPlan, 'features'> & { features: string | string[] }) => ({
      ...p,
      access_scope: p.access_scope || 'explicit',
      group_ids: p.group_ids && p.group_ids.length > 0 ? p.group_ids : (p.group_id ? [p.group_id] : []),
      groups: p.groups || [],
      allowed_platforms: p.allowed_platforms || [],
      overage_policy: p.overage_policy || 'block',
      features: typeof p.features === 'string'
        ? p.features.split('\n').map((f: string) => f.trim()).filter(Boolean)
        : (p.features || []),
    }))
  }
  catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
  finally { plansLoading.value = false }
}

function openPlanEdit(plan: SubscriptionPlan | null) {
  editingPlan.value = plan
  showPlanDialog.value = true
}

function getAccessScopeLabel(scope: string | null | undefined): string {
  switch (scope) {
    case 'platform_subscription_groups':
      return t('payment.admin.platformSubscriptionGroups')
    case 'all_subscription_groups':
      return t('payment.admin.allSubscriptionGroups')
    default:
      return t('payment.admin.explicitGroups')
  }
}


/** Quick toggle for_sale from the list */
async function toggleForSale(plan: SubscriptionPlan) {
  try {
    await adminPaymentAPI.updatePlan(plan.id, { for_sale: !plan.for_sale })
    plan.for_sale = !plan.for_sale
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  }
}

function confirmDeletePlan(plan: SubscriptionPlan) { deletingPlanId.value = plan.id; showDeletePlanDialog.value = true }
async function handleDeletePlan() {
  if (!deletingPlanId.value) return
  try { await adminPaymentAPI.deletePlan(deletingPlanId.value); appStore.showSuccess(t('common.deleted')); showDeletePlanDialog.value = false; loadPlans() }
  catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
}

// ==================== Lifecycle ====================

onMounted(() => {
  loadGroups()
  loadPaymentConfig()
  loadPlans()
})
</script>
